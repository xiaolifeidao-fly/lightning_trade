package backfill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Report 一次回灌/巡检的完整结果，渲染成 Markdown 交给人看。
//
// 报告本身就是交付物：3 个月双写期里每天跑一次，缺口栏一旦非零就说明
// 直写路径漏了事件（队列满 / DB 短暂不可用 / SIGKILL），此时用同一个
// 工具的回灌模式即可补齐，不需要另写脚本。
type Report struct {
	InstanceKey string
	GeneratedAt time.Time
	Mode        string // dry-run / import / audit
	Sources     []string
	Since       string
	Until       string
	Build       *BuildResult
	Writes      map[string]WriteStat // date → 写库结果
	Audits      []DayAudit           // 按日期升序
	Orphans     []OrphanDay          // 库里有整天数据但 JSONL 没有的日期
}

// Render 输出 Markdown。表格列顺序固定，方便两次巡检直接 diff。
func (r *Report) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# argus_single 事件一致性巡检报告 · %s\n\n", r.InstanceKey)
	fmt.Fprintf(&b, "| 项 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 实例 | `%s` |\n", r.InstanceKey)
	fmt.Fprintf(&b, "| 生成时间 | %s |\n", r.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| 运行模式 | %s |\n", r.Mode)
	fmt.Fprintf(&b, "| 日期范围 | %s |\n", dateRangeText(r.Since, r.Until))
	fmt.Fprintf(&b, "| 来源目录 | %s |\n", joinCode(r.Sources))
	if r.Build != nil {
		rawEvents, unique, duplicates, rows := r.Build.Totals()
		fmt.Fprintf(&b, "| JSONL 文件数 | %d |\n", len(r.Build.Files))
		fmt.Fprintf(&b, "| 解析成功事件 | %d |\n", rawEvents)
		fmt.Fprintf(&b, "| 去重后唯一事件 | %d |\n", unique)
		fmt.Fprintf(&b, "| 同哈希重复行 | %d |\n", duplicates)
		fmt.Fprintf(&b, "| 待入库行数 | %d |\n", rows)
	}
	b.WriteString("\n")

	r.renderFiles(&b)
	r.renderDays(&b)
	r.renderCoverage(&b)
	r.renderAudit(&b)
	r.renderAnomalies(&b)
	r.renderNotes(&b)
	return b.String()
}

func (r *Report) renderFiles(b *strings.Builder) {
	if r.Build == nil || len(r.Build.Files) == 0 {
		return
	}
	badTotal, offDateTotal, blankTotal := 0, 0, 0
	for _, file := range r.Build.Files {
		badTotal += file.BadLines
		offDateTotal += file.OffDate
		blankTotal += file.BlankLines
	}
	b.WriteString("## 1. 源文件读取\n\n")
	fmt.Fprintf(b, "共 %d 个 `events-<date>.jsonl`，坏行 %d、空行 %d、ts 日期与文件名不一致 %d。\n\n",
		len(r.Build.Files), badTotal, blankTotal, offDateTotal)
	if badTotal == 0 && offDateTotal == 0 {
		b.WriteString("> 全部文件逐行解析成功，无坏行。历史 JSONL 未被修改（本工具只读）。\n\n")
		return
	}
	b.WriteString("| 文件 | 非空行 | 解析成功 | 坏行 | 跨日 |\n|---|---:|---:|---:|---:|\n")
	for _, file := range r.Build.Files {
		if file.BadLines == 0 && file.OffDate == 0 {
			continue
		}
		fmt.Fprintf(b, "| `%s` | %d | %d | %d | %d |\n", file.Path, file.TotalLines, file.Events, file.BadLines, file.OffDate)
	}
	b.WriteString("\n")
}

func (r *Report) renderDays(b *strings.Builder) {
	if r.Build == nil || len(r.Build.Days) == 0 {
		return
	}
	b.WriteString("## 2. 逐日回灌\n\n")
	b.WriteString("| 日期 | 解析 | 去重后 | 重复 | strategy | balance | dev | 提交 | 新插入 | 已存在 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, day := range r.Build.Days {
		write := r.Writes[day.Date]
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			day.Date, day.RawEvents, len(day.Hashes), day.Duplicates,
			len(day.Rows.Strategy), len(day.Rows.Balance), len(day.Rows.Dev),
			write.Submitted, write.Inserted, write.Existed)
	}
	b.WriteString("\n")

	events := map[string]int{}
	for _, day := range r.Build.Days {
		for event, count := range day.ByEvent {
			events[event] += count
		}
	}
	if len(events) > 0 {
		b.WriteString("去重后按事件类型：\n\n| 事件 | 条数 |\n|---|---:|\n")
		for _, event := range sortedKeys(events) {
			fmt.Fprintf(b, "| `%s` | %d |\n", event, events[event])
		}
		b.WriteString("\n")
	}
}

func (r *Report) renderCoverage(b *strings.Builder) {
	if r.Build == nil || len(r.Build.Days) == 0 {
		return
	}
	b.WriteString("## 3. 字段可用性（设计文档 §2.4 的三个坑）\n\n")
	b.WriteString("字段缺失是这个系统的常态而不是老数据的兼容包袱：`equity`/`equityKnown` 2026-07-21 才上线，")
	b.WriteString("`balance.size` 同日，`peakPct` 2026-07-23，`sigLast`/`sigMark`/`gapBp` 2026-07-28。")
	b.WriteString("下游按天查历史前先看这张表，否则会把\"字段没上线\"读成\"那天没有信号\"。\n\n")
	b.WriteString("| 日期 | balance | equity 已知 | net_size 已知 | 信号事件 | 带 gapBp | trailing | 带 peakPct | 门控事件 | 已结构化 | variant 缺失 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, day := range r.Build.Days {
		c := day.Coverage
		fmt.Fprintf(b, "| %s | %d | %s | %s | %d | %s | %d | %s | %d | %s | %d |\n",
			day.Date, c.BalanceRows,
			ratio(c.EquityKnown, c.BalanceRows), ratio(c.NetSizeKnown, c.BalanceRows),
			c.SignalRows, ratio(c.GapBpPresent, c.SignalRows),
			c.TrailingRows, ratio(c.PeakPctPresent, c.TrailingRows),
			c.GateRows, ratio(c.GateKindResolved, c.GateRows),
			c.VariantMissing)
	}
	b.WriteString("\n")
}

func (r *Report) renderAudit(b *strings.Builder) {
	if len(r.Audits) == 0 {
		return
	}
	b.WriteString("## 4. JSONL ↔ MySQL 一致性\n\n")
	b.WriteString("按 `event_hash` 逐条比对。`缺失` = JSONL 有而库里没有，是双写期唯一需要动作的列；")
	b.WriteString("`多出` = 库里有而当前 JSONL 快照没有（该日 JSONL 已归档/未纳入本次来源目录时属正常）。\n\n")
	b.WriteString("| 日期 | 表 | JSONL | DB | 直写 | 回灌 | 缺失 | 多出 |\n|---|---|---:|---:|---:|---:|---:|---:|\n")
	for _, audit := range r.Audits {
		for _, table := range audit.Tables {
			if table.JSONLRows == 0 && table.DBRows == 0 {
				continue
			}
			fmt.Fprintf(b, "| %s | `%s` | %d | %d | %d | %d | %d | %d |\n",
				audit.Date, table.Table, table.JSONLRows, table.DBRows, table.DBLive, table.DBBackfill, table.Missing, table.Extra)
		}
	}
	b.WriteString("\n")

	missing, extra := 0, 0
	var samples []string
	for _, audit := range r.Audits {
		missing += audit.MissingTotal
		extra += audit.ExtraTotal
		if len(samples) < SampleLimit {
			samples = append(samples, audit.MissingHash...)
		}
	}
	if len(r.Orphans) > 0 {
		b.WriteString("库里有整天数据、本次来源目录却没有对应 JSONL 的日期（JSONL 落盘失败，或该天日志尚未归档进来）：\n\n")
		b.WriteString("| 日期 | 表 | 行数 |\n|---|---|---:|\n")
		for _, orphan := range r.Orphans {
			fmt.Fprintf(b, "| %s | `%s` | %d |\n", orphan.Date, orphan.Table, orphan.Rows)
		}
		b.WriteString("\n")
	}

	if missing == 0 && extra == 0 && len(r.Orphans) == 0 {
		b.WriteString("**结论：全部日期 JSONL 与 MySQL 逐条一致，零缺失零多余。**\n\n")
		return
	}
	fmt.Fprintf(b, "**结论：缺失 %d 行、多出 %d 行、JSONL 缺整天 %d 处，需处置。**\n\n", missing, extra, len(r.Orphans))
	if len(samples) > 0 {
		fmt.Fprintf(b, "缺失哈希样例：`%s`\n\n", strings.Join(samples, "`, `"))
	}
}

func (r *Report) renderAnomalies(b *strings.Builder) {
	if r.Build == nil {
		return
	}
	merged := map[string]*FieldAnomaly{}
	for _, day := range r.Build.Days {
		for _, anomaly := range day.Anomalies {
			key := anomaly.Event + "/" + anomaly.Field
			if entry, ok := merged[key]; ok {
				entry.Count += anomaly.Count
				continue
			}
			copied := anomaly
			merged[key] = &copied
		}
	}
	if len(merged) == 0 {
		return
	}
	b.WriteString("## 5. 字段适用性异常（设计文档 §6.6）\n\n")
	b.WriteString("矩阵说\"这类事件应该有这个字段\"但历史行里没有。只报数不拒绝导入：")
	b.WriteString("历史缺口是既成事实，拒绝导入等于把全部历史挡在库外。\n\n")
	b.WriteString("| 事件 | 字段 | 条数 | 首例 ts |\n|---|---|---:|---|\n")
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		anomaly := merged[key]
		fmt.Fprintf(b, "| `%s` | `%s` | %d | %s |\n", anomaly.Event, anomaly.Field, anomaly.Count, anomaly.Example)
	}
	b.WriteString("\n")
}

func (r *Report) renderNotes(b *strings.Builder) {
	b.WriteString("## 6. 口径备注\n\n")
	b.WriteString("- 回灌行 `source=2`、`config_version=0`：历史事件没有可考的配置版本，留 0 与直写的真实版本号区分开。\n")
	b.WriteString("- `uid` 由该实例 properties 的 `trade.accountN.name/uid` 映射而来；事件里从来没有 uid，未知标签一律拒绝导入而不猜。\n")
	b.WriteString("- `instrument` 归一化到 `BTCUSDT`，`inst_id_raw` 保留 JSONL 原值（信号侧 `BTCUSDT` / 持仓侧 `BTC-USDT-SWAP` 100% 分裂）。\n")
	b.WriteString("- `net_size_known` 按「天 × 账户」自适应判定：当天该账户出现过带 `size` 的 balance ⇒ 同日缺失即已知空仓；一条都没有 ⇒ 全天未知。\n")
	b.WriteString("- 历史 JSONL 只读，本工具不修改、不移动、不删除任何原文件。\n")
	if r.Build != nil && (r.Build.OutOfRange > 0 || r.Build.NoDate > 0) {
		fmt.Fprintf(b, "- 被日期范围过滤 %d 条，ts 无法归日 %d 条。\n", r.Build.OutOfRange, r.Build.NoDate)
	}
	b.WriteString("\n")
}

// Write 把报告写到文件；目录不存在时自动建。
func (r *Report) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("backfill: create report dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(r.Render()), 0o644); err != nil {
		return fmt.Errorf("backfill: write report %s: %w", path, err)
	}
	return nil
}

func ratio(part, total int) string {
	if total == 0 {
		return "—"
	}
	return fmt.Sprintf("%d (%.0f%%)", part, float64(part)*100/float64(total))
}

func dateRangeText(since, until string) string {
	switch {
	case since == "" && until == "":
		return "全部"
	case since == "":
		return "… ~ " + until
	case until == "":
		return since + " ~ …"
	default:
		return since + " ~ " + until
	}
}

func joinCode(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "`"+item+"`")
	}
	return strings.Join(quoted, "<br>")
}
