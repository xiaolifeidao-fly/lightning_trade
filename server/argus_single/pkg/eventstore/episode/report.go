package episode

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Report 一次重建的可读产物。与 backfill 的报告同一套写法：只有路径、日期与
// 计数，账户标签里的邮箱本地部分打码——logs/ 与 configs/ 含真实生产凭证，
// 任何落盘的报告都不得把它们带出来。
type Report struct {
	InstanceKey string
	GeneratedAt time.Time
	Mode        string
	Episodes    []*Episode
	Stats       Stats
	Write       WriteStat
}

var emailLocal = regexp.MustCompile(`[^-@\s]+@`)

// MaskAccount 把账户标签里的邮箱本地部分整段打码：
// 账户A-1394537246@qq.com → 账户A-***@qq.com。
//
// 报告是要提交进仓库、跟着任务走的文档，而 logs/ 与 configs/ 下的账户标签
// 带真实邮箱。区分账户靠"账户A/账户B"这个前缀就够了，邮箱本地部分对读报告
// 的人没有任何信息量，一律不落盘。
func MaskAccount(label string) string {
	if label == "" {
		return "(未标注)"
	}
	return emailLocal.ReplaceAllString(label, "***@")
}

// Render 渲染 Markdown 报告。
func (r *Report) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# argus_single episode 派生报告 · `%s`\n\n", r.InstanceKey)
	fmt.Fprintf(&b, "| 项 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 实例 | `%s` |\n", r.InstanceKey)
	fmt.Fprintf(&b, "| 生成时间 | %s |\n", r.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| 运行模式 | %s |\n", r.Mode)
	fmt.Fprintf(&b, "| 参与派生的事件 | %d |\n", r.Stats.Events)
	fmt.Fprintf(&b, "| 账本数 (实例×账户×合约) | %d |\n", r.Stats.Accounts)
	fmt.Fprintf(&b, "| episode | %d |\n", r.Stats.Episodes)
	fmt.Fprintf(&b, "| 建仓决策行 | %d |\n", r.entryCount())
	if first, last, ok := r.window(); ok {
		fmt.Fprintf(&b, "| 覆盖区间 | %s ~ %s |\n", first.Format("2006-01-02"), last.Format("2006-01-02"))
	}
	b.WriteString("\n")

	r.renderExit(&b)
	r.renderAccounts(&b)
	r.renderAttribution(&b)
	r.renderAttributionContrast(&b)
	r.renderDepth(&b)
	r.renderAnomalies(&b)
	r.renderEpisodes(&b)
	return b.String()
}

func (r *Report) renderExit(b *strings.Builder) {
	b.WriteString("## 1. 出场方式分布\n\n")
	b.WriteString("`reduce_to_zero` 没有任何平仓事件——反向减仓一张一张磨到 0。前端若只认平仓事件，这些 episode 会显示成「永远没结束」。\n\n")
	b.WriteString("| 出场方式 | episode 数 | 计入策略胜率 | 已知合计盈亏 |\n|---|---:|---|---:|\n")
	kinds := []string{ExitTrailingClose, ExitReduceToZero, ExitCatastropheStop, ExitFixedClose, ExitExternalClose, ExitManualClose, "open"}
	for _, kind := range kinds {
		count := r.Stats.ByExit[kind]
		if count == 0 {
			continue
		}
		var pnl float64
		attributable := "否"
		for _, ep := range r.Episodes {
			if exitLabel(ep) == kind {
				pnl += ep.Pnl
				if ep.StrategyAttributable == 1 {
					attributable = "是"
				}
			}
		}
		label := kind
		if kind == "open" {
			label = "`open`（仍持仓）"
		} else {
			label = "`" + kind + "`"
		}
		fmt.Fprintf(b, "| %s | %d | %s | %.4f |\n", label, count, attributable, pnl)
	}
	b.WriteString("\n")
}

func (r *Report) renderAccounts(b *strings.Builder) {
	type agg struct {
		episodes, closed, gaps int
		pnl, pnlStrategy       float64
		maxSize                int
	}
	// 按原始标签分组、只在打印时打码：打码后再分组会让两个不同账户在报告里
	// 被静默合并成一行，而"合并了"这件事本身看不出来。
	byAccount := map[string]*agg{}
	var order []string
	for _, ep := range r.Episodes {
		key := ep.AccountLabel
		entry, ok := byAccount[key]
		if !ok {
			entry = &agg{}
			byAccount[key] = entry
			order = append(order, key)
		}
		entry.episodes++
		if ep.ClosedAt != nil {
			entry.closed++
		}
		entry.gaps += ep.PositionGapCount
		entry.pnl += ep.Pnl
		entry.pnlStrategy += ep.PnlStrategy
		if ep.MaxSize > entry.maxSize {
			entry.maxSize = ep.MaxSize
		}
	}
	sort.Strings(order)
	b.WriteString("## 2. 逐账户\n\n")
	b.WriteString("| 账户 | episode | 已出场 | 峰值张数 | 已知盈亏 | 其中策略出场 | 仓位跳变 |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, key := range order {
		a := byAccount[key]
		fmt.Fprintf(b, "| %s | %d | %d | %d | %.4f | %.4f | %d |\n",
			MaskAccount(key), a.episodes, a.closed, a.maxSize, a.pnl, a.pnlStrategy, a.gaps)
	}
	b.WriteString("\n")
}

func (r *Report) renderAttribution(b *strings.Builder) {
	var attributed, strategyPnl float64
	incomplete := 0
	for _, ep := range r.Episodes {
		for _, entry := range ep.Entries {
			attributed += entry.AttributedPnl
			strategyPnl += entry.AttributedPnlStrategy
			if entry.PnlKnown == 0 {
				incomplete++
			}
		}
	}
	b.WriteString("## 3. 决策归集\n\n")
	b.WriteString("每笔已实现盈亏按各笔决策**在场张数**等比例摊回建仓时刻。按平仓时刻归集会把「更容易平仓的状态」误读成「更赚钱的状态」（波动率研究 §10.3 的伪影）。\n\n")
	fmt.Fprintf(b, "| 项 | 值 |\n|---|---:|\n")
	fmt.Fprintf(b, "| 建仓决策行 | %d |\n", r.entryCount())
	fmt.Fprintf(b, "| 已归集盈亏 | %.4f |\n", attributed)
	fmt.Fprintf(b, "| 其中策略出场部分 | %.4f |\n", strategyPnl)
	fmt.Fprintf(b, "| 归因不完整的决策行 | %d |\n", incomplete)
	fmt.Fprintf(b, "| 盈亏未知的实现次数 | %d |\n", r.Stats.MissingPnlEvents)
	fmt.Fprintf(b, "| 落不到任何决策上的盈亏 | %.4f |\n", r.Stats.UnattributedPnl)
	b.WriteString("\n> 「归因不完整」= 该决策仍有在场张数，或它在场期间发生过盈亏未知的实现（2026-07-24 之前的反向减仓只写 size 不写 pnl）。分层统计要么剔除这些行，要么显式标注。\n\n")
}

// contrastLimit 口径对比最多列几天。截断了就要写出来，否则一张只有 15 行的
// 表看上去像是"全部都在这里了"。
const contrastLimit = 15

// renderAttributionContrast 把同一份数据按两种口径分别按天汇总，直接摆出差异。
//
// 这是本任务最容易被下游忽略的一条：只要有人图省事按 closed_at 分桶，
// 波动率研究 §10.3 那个"高波动期收益是低波动期 8–11 倍、16/16 路径同号"的
// 伪影就会原样长回来。把差异印在报告里，比在文档里写一句提醒有用。
func (r *Report) renderAttributionContrast(b *strings.Builder) {
	decision := map[string]float64{}
	exit := map[string]float64{}
	days := map[string]struct{}{}
	for _, ep := range r.Episodes {
		if ep.ClosedAt != nil && ep.Pnl != 0 {
			day := ep.ClosedAt.Format("2006-01-02")
			exit[day] += ep.Pnl
			days[day] = struct{}{}
		}
		for _, entry := range ep.Entries {
			if entry.AttributedPnl == 0 {
				continue
			}
			day := entry.DecidedAt.Format("2006-01-02")
			decision[day] += entry.AttributedPnl
			days[day] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(days))
	for day := range days {
		ordered = append(ordered, day)
	}
	sort.Slice(ordered, func(i, j int) bool {
		di := math.Abs(decision[ordered[i]] - exit[ordered[i]])
		dj := math.Abs(decision[ordered[j]] - exit[ordered[j]])
		if di != dj {
			return di > dj
		}
		return ordered[i] < ordered[j]
	})

	b.WriteString("## 4. 决策归集 vs 平仓归集（差异最大的若干天）\n\n")
	if len(ordered) == 0 {
		b.WriteString("窗口内没有已实现盈亏，无从对比。\n\n")
		return
	}
	shown := ordered
	if len(shown) > contrastLimit {
		shown = shown[:contrastLimit]
	}
	sorted := append([]string(nil), shown...)
	sort.Strings(sorted)
	b.WriteString("| 日期 | 决策归集（按建仓日） | 平仓归集（按出场日） | 差 |\n|---|---:|---:|---:|\n")
	for _, day := range sorted {
		fmt.Fprintf(b, "| %s | %.4f | %.4f | %+.4f |\n", day, decision[day], exit[day], decision[day]-exit[day])
	}
	var decisionTotal, exitTotal float64
	for _, day := range ordered {
		decisionTotal += decision[day]
		exitTotal += exit[day]
	}
	fmt.Fprintf(b, "\n> 共 %d 天有已实现盈亏，上表按 |差| 取前 %d 天后再按日期排序。两列合计 %.4f vs %.4f——合计本就不该相等：右列不含仍持仓 episode 中途减仓的盈亏，左列不含建仓在数据窗口之前的那部分（见上一节「落不到任何决策上的盈亏」）。要看的是**逐日差异**：同一批钱换个归集口径就换了日子，§10.3 那个「高波动期收益是低波动期 8–11 倍」的伪影就是这么来的。**任何按状态/条件分层的收益归因都必须用左列。**\n\n",
		len(ordered), len(shown), decisionTotal, exitTotal)
}

func (r *Report) renderDepth(b *strings.Builder) {
	byDepth := map[string]int{}
	for _, ep := range r.Episodes {
		byDepth[ep.DepthFidelity]++
	}
	b.WriteString("## 5. 扛单深度保真度\n\n")
	b.WriteString("| 档位 | episode 数 | 含义 |\n|---|---:|---|\n")
	for _, kind := range []string{DepthMinute, DepthMixed, DepthAlertSampled} {
		if byDepth[kind] == 0 {
			continue
		}
		fmt.Fprintf(b, "| `%s` | %d | %s |\n", kind, byDepth[kind], depthMeaning(kind))
	}
	b.WriteString("\n> `min_roi_pct_observed` 在任何档位下都只是真实最深值的**下界**：loss_alert 的触发门槛是 ROI < −150%、告警冷却 5 分钟，极值经常落在两次告警之间。\n\n")
}

func depthMeaning(kind string) string {
	switch kind {
	case DepthMinute:
		return "全程有带 upl 的余额心跳，可画真正细的水下曲线"
	case DepthMixed:
		return "跨越 upl 上线时点或心跳有缺口，两段精度不同不得混排"
	default:
		return "只有 loss_alert 采样：0 ~ −150% 区间完全空白，深水区 5 分钟一采且为下界"
	}
}

func (r *Report) renderAnomalies(b *strings.Builder) {
	b.WriteString("## 6. 异常与边界\n\n")
	b.WriteString("| 项 | 次数 | 处理 |\n|---|---:|---|\n")
	fmt.Fprintf(b, "| 仓位跳变（\\|Δsize\\| != orderSize） | %d | 以仓位快照为准，记标记不断开 episode |\n", r.Stats.PositionGaps)
	fmt.Fprintf(b, "| 其中持仓快照滞后（Δsize = 0） | %d | 不凭 orderSize 猜真实仓位，猜错会让后面每笔归因整体错位 |\n", r.Stats.StaleSnapshot)
	fmt.Fprintf(b, "| 开头被窗口截断的 episode | %d | `opened_at` 留 NULL，不拿平仓时刻凑 |\n", r.Stats.TruncatedHead)
	fmt.Fprintf(b, "| 反向单反而做大仓位 | %d | 只记异常，不动仓位（reverse_gate 本应拦掉翻转单） |\n", r.Stats.SideFlipRejected)
	orphans := 0
	for _, count := range r.Stats.OrphanEvents {
		orphans += count
	}
	fmt.Fprintf(b, "| 无在场持仓时收到的观测事件 | %d | 计数并在下表列出，不静默丢弃 |\n", orphans)
	b.WriteString("\n")
	if len(r.Stats.SideFlipSamples) > 0 {
		b.WriteString("反向单做大仓位的样例（最多 5 条，可回 `strategy_event` 按时刻定位）：\n\n")
		for _, sample := range r.Stats.SideFlipSamples {
			fmt.Fprintf(b, "- `%s`\n", sample)
		}
		b.WriteString("\n")
	}
	if orphans > 0 {
		b.WriteString("| 事件类型 | 条数 |\n|---|---:|\n")
		kinds := make([]string, 0, len(r.Stats.OrphanEvents))
		for kind := range r.Stats.OrphanEvents {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			fmt.Fprintf(b, "| `%s` | %d |\n", kind, r.Stats.OrphanEvents[kind])
		}
		b.WriteString("\n")
		for _, sample := range r.Stats.OrphanSamples {
			fmt.Fprintf(b, "- `%s`\n", sample)
		}
		b.WriteString("\n")
	}
}

func (r *Report) renderEpisodes(b *strings.Builder) {
	b.WriteString("## 7. episode 明细\n\n")
	b.WriteString("| 建仓 | 出场 | 账户 | 方向 | 出场方式 | 加仓 | 减仓 | 峰值 | 观测最深 ROI% | 已知盈亏 | 深度 | 跳变 |\n")
	b.WriteString("|---|---|---|---|---|---:|---:|---:|---:|---:|---|---|\n")
	for _, ep := range r.Episodes {
		fmt.Fprintf(b, "| %s | %s | %s | %s | `%s` | %d | %d | %d | %s | %.4f | `%s` | %s |\n",
			tsOrDash(ep.OpenedAt), tsOrDash(ep.ClosedAt), MaskAccount(ep.AccountLabel), ep.Side,
			exitLabel(ep), ep.AddCount, ep.ReduceCount, ep.MaxSize,
			floatOrDash(ep.MinRoiPctObserved), ep.Pnl, ep.DepthFidelity, flag(ep.HasPositionGap))
	}
	b.WriteString("\n")
}

func exitLabel(ep *Episode) string {
	if ep.ExitKind == nil {
		return "open"
	}
	return *ep.ExitKind
}

func tsOrDash(ts *time.Time) string {
	if ts == nil {
		return "—"
	}
	return ts.Format("2006-01-02 15:04:05")
}

func floatOrDash(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *v)
}

func flag(v uint8) string {
	if v == 1 {
		return "⚠"
	}
	return ""
}

func (r *Report) entryCount() int {
	total := 0
	for _, ep := range r.Episodes {
		total += len(ep.Entries)
	}
	return total
}

func (r *Report) window() (time.Time, time.Time, bool) {
	if len(r.Episodes) == 0 {
		return time.Time{}, time.Time{}, false
	}
	first, last := r.Episodes[0].FirstEventAt, r.Episodes[0].LastEventAt
	for _, ep := range r.Episodes {
		if ep.FirstEventAt.Before(first) {
			first = ep.FirstEventAt
		}
		if ep.LastEventAt.After(last) {
			last = ep.LastEventAt
		}
	}
	return first, last, true
}

// WriteFile 落盘报告，自动建目录。
func (r *Report) WriteFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("episode: create report directory: %w", err)
	}
	return os.WriteFile(path, []byte(r.Render()), 0o644)
}
