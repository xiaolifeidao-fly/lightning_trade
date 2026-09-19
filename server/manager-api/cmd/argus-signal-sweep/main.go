// argus-signal-sweep 用真实触发流跑一批参数，对比结果。
//
// 为什么要这个 CLI：参数扫描的服务层（CreateSignalBacktestBatch / RunSignalBacktestBatch）
// 早就写好了，但只有 HTTP 入口，要带鉴权、还会把管理端进程的 CPU 吃满。诊断期需要
// 反复改网格重跑，走 CLI 直连库最省事，且与线上接口同一份代码、同一口径。
//
// 校准是前提：--dim baseline 先用【实例当前生产参数】跑一遍，与 episode 表里的
// 实盘结果对照。对不上就别看扫描结果——那说明回放口径有偏差，网格里的排序也不可信。
//
// 用法：
//
//	cd server/manager-api
//	go run ./cmd/argus-signal-sweep --account '账户B-mortypeng@gmail.com' \
//	  --start '2026-09-08 21:00:00' --end '2026-09-15 14:40:00' --dim baseline
//	go run ./cmd/argus-signal-sweep --account '账户B-...' --dim trend
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"common/middleware/db"
	"common/middleware/vipper"
	"service/trade"
	tradeDTO "service/trade/dto"
	"service/trade/strategy/signal"
)

func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }
func i32(v int) *int         { return &v }

// pinGate 把趋势闸钉在 2026-09-16 走前验证后上线的生产值 48h/3%。
// 不钉的话，各维会拿服务端基线解析器给的闸当参照——那可能是旧值，
// 测出来就是"本维效应 + 闸差异"的混合，排序不可信。
func pinGate(p tradeDTO.SignalBacktestParamsDTO) tradeDTO.SignalBacktestParamsDTO {
	p.TrendGateWindowHours = f64(48)
	p.TrendGateThresholdPct = f64(3)
	return p
}

// grid 返回某个维度要扫的参数组。组参数是【相对基线的增量】——没给的旋钮沿用
// 该实例当前生产值，所以 diff 里只有被扫的那一行。
func grid(dim string) []tradeDTO.SignalBacktestGroupDTO {
	g := func(label string, p tradeDTO.SignalBacktestParamsDTO) tradeDTO.SignalBacktestGroupDTO {
		return tradeDTO.SignalBacktestGroupDTO{Label: label, SignalBacktestParamsDTO: p}
	}
	switch dim {
	case "trend":
		// 趋势闸是唯一为「禁止逆势开/加仓」而生的机制，也是唯一**入场侧**的行情
		// 路由器——判断错只损失机会，不会把可回归的浮亏砍成实亏（趋势条件止损
		// 就是栽在这一点上：出场侧 + 抖动的指标 = 成交档位随机）。所以要调先调它。
		//
		// 网格必须包含这三个参考点，否则问题回答不了：
		//   off      基线（闸关掉），一切增益都要相对它衡量
		//   24h/3%   2026-09-15 起的生产值
		//   24h/5%   2026-09-15 之前的生产值——那次收紧至今**没在 80 天窗口上验过**
		// 窗口带到 48h 是为了看清平台形状：只有知道两侧都变差，中间那档才算平台
		// 而不是噪声里的一个尖峰。
		//
		// 事前预测（先写下来，免得事后挑一个好看的解释）：**窗口比阈值重要**。
		// 依据是 08-20（全样本最差日 −76.80）当天闸一次没响——当时阈值 5%，24h
		// 动量还没爬过线；到 08-21 才响了 150 次。那天的问题是**滞后**，不是档位。
		// 若如此，短窗口（4h/8h）该比降阈值（5%→3%）更有效。
		var out []tradeDTO.SignalBacktestGroupDTO
		out = append(out, g("trend_off", tradeDTO.SignalBacktestParamsDTO{
			TrendGateWindowHours: f64(24), TrendGateThresholdPct: f64(0),
		}))
		for _, w := range []float64{1, 2, 4, 8, 24, 48} {
			for _, t := range []float64{0.8, 1.5, 3.0, 5.0} {
				out = append(out, g(fmt.Sprintf("trend_%gh_%.1f%%", w, t),
					tradeDTO.SignalBacktestParamsDTO{
						TrendGateWindowHours:  f64(w),
						TrendGateThresholdPct: f64(t),
					}))
			}
		}
		return out
	case "cap":
		// 仓位上限：这套策略里唯一的硬敞口约束，而两个账户**全程顶格**
		// （B 上限跳过 1398 次 vs 成交 164 笔）。生产上 A 是公式约束
		// （N_formula=22.7 → 22，天花板 26 不咬）、B 是天花板约束（8）。
		// CapOverride 绕过公式直接固定上限，所以这一维问的是"敞口该多大"，
		// 与"改 risk_budget 还是改 max_contracts"解耦；结论要落到生产时再翻译。
		//
		// **判据必须事前说清，否则这一维会给出一个假结论**：策略是正期望的，
		// 净盈亏几乎必然随上限单调上升，"上限越大越赚"是算术恒等式、不是发现。
		// 所以要看的是风险调整后的结果：
		//   硬约束——最大回撤不得超过 risk_equity 的 40%（A 165.6 / B 66.5）；
		//            回撤接近本金就是破产，不管净盈亏多好看。
		//   在满足硬约束的格子里，再看净盈亏与 净盈亏/回撤。
		//   还要看走前两段的**形状**是否一致，而不是挑单点峰值。
		//
		// 事前预测：按 上限/本金 算，B 是 8/166.27=0.048、A 是 22/414=0.053，
		// B 略低于 A，所以 B 有小幅上调空间；但 08-20 那类尾部会同比放大，
		// 风险调整后的最优应落在现值附近或更低，而不是网格高端。
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, c := range []int{4, 6, 8, 12, 16, 22, 26, 34, 44} {
			out = append(out, g(fmt.Sprintf("cap_%d", c),
				pinGate(tradeDTO.SignalBacktestParamsDTO{CapOverride: i32(c)})))
		}
		return out
	case "stop":
		var out []tradeDTO.SignalBacktestGroupDTO
		// 钉闸（见 pinGate）。事前预测：曾到过 −100~−300% 的 71 笔全部回本、
		// −300 以下 31 笔 19 笔死——所以 300 是唯一可能好于 400 的格；<300 几乎
		// 必然更差（把回本单砍成实亏），这与 7 天窗口"收紧更差"的结论一致。
		for _, s := range []float64{250, 300, 350, 400, 500, 650} {
			out = append(out, g(fmt.Sprintf("stop_%.0f", s),
				pinGate(tradeDTO.SignalBacktestParamsDTO{CatastropheStopPct: f64(s)})))
		}
		return out
	case "gate":
		// 反向减仓门槛。生产 8%：只有净仓 ROI ≥ +8% 时反向信号才允许减 1 张，
		// 亏损中的反向信号一律拒绝（gate_block）。这一维原来只扫 0~40 的正值。
		//
		// 为什么要扫到负值：2026-09-17~18 的兜底（A −53.4 / B −19.3）是"慢磨"型——
		// 48h 动量 36 小时里没越过 3%，趋势闸看不见；而持仓期间收到 135 次反向
		// 信号，其中 47 次发生在 ROI > −100% 时，全被 8% 门槛拒绝。80 天 19 笔兜底
		// 无一例外：首个反向信号在开仓后 0~1.3h、ROI −2%~−38% 时就到了。
		// 负门槛 = "浮亏不深于 X% 时仍按反向信号减仓"，是与趋势闸正交的第二条
		// 尾部出口：趋势闸管入场（看行情），负门槛管出场（看信号）。
		//
		// **2026-09-19 扫过 −50~−300，结论：全部有害，且随负得越深越差。**
		// B 测试段 +123.95 → −38.61（−50）→ −99.34（−100）；A 留出段 +89.11 → −80.07（−50）。
		// 机理：反向信号每几分钟一次，负门槛把策略变成"跟信号来回翻"——笔数 ×4、
		// 手续费 ×2.5、胜率 92%→29%；而策略的利润恰恰来自**满仓扛过 −100~−300%
		// 再由移动止盈收割**（80 天里曾到过 −100~−300% 的 71 笔全部回本、零兜底）。
		// 任何让它在亏损中减仓的机制都在拆这个 edge——这是第三次得出同一结论
		// （收紧兜底、趋势条件止损、负门槛）。负值已从网格撤掉，实盘校验器仍拒绝负值。
		//
		// 事前判据（与趋势闸走前验证同一条）：训练段选格 → 该格在测试段净盈亏
		// 上升且兜底不增。0 与 8 在四段上互有胜负（A 测试段 0 大胜、留出段 0 大败），
		// 是路径噪声，不动。
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, v := range []float64{0, 4, 8, 15, 25, 40} {
			out = append(out, g(fmt.Sprintf("gate_%.0f", v),
				pinGate(tradeDTO.SignalBacktestParamsDTO{GateMinProfitPct: f64(v)})))
		}
		return out
	case "trendstop":
		// 趋势条件止损：X（触发阈值）× Y（收紧后的兜底线）。
		// Y < 250 会被实盘校验器拒（松兜底护栏），所以网格只到 250——
		// 要扫更紧的先改护栏语义并留审批痕迹，别用扫参绕过去。
		var out []tradeDTO.SignalBacktestGroupDTO
		out = append(out, g("trendstop_off", tradeDTO.SignalBacktestParamsDTO{
			TrendStopTriggerPct: f64(0), TrendStopPct: f64(0),
			TrendGateWindowHours: f64(24),
		}))
		for _, x := range []float64{2, 2.5, 3, 4, 5} {
			for _, y := range []float64{250, 300, 350} {
				out = append(out, g(fmt.Sprintf("ts_x%.1f_y%.0f", x, y),
					tradeDTO.SignalBacktestParamsDTO{
						TrendStopTriggerPct:  f64(x),
						TrendStopPct:         f64(y),
						TrendGateWindowHours: f64(24),
					}))
			}
		}
		return out
	case "regime":
		// 行情路由减仓（第 2 步）：前一日状态标签命中时，本日入场上限按系数压低。
		//
		// 只扫两件事——**减哪些状态**、**减多少**。日标签的阈值故意不扫，固定用
		// 金标准 1.5%/2.5%（backtest_capsf_study.py 的写死值）：80 天里只有 16 笔
		// 兜底、5 天主导亏损，多拟合一个阈值就是在噪声上找峰。少拟合一个参数，
		// 走前验证才有意义。
		//
		// 每组都显式钉住趋势闸 48h/3%——那是 2026-09-16 走前验证后上线的生产值。
		// 不钉的话这一维会拿旧闸当基线，测出来的是"路由 + 旧闸"的混合效应。
		var out []tradeDTO.SignalBacktestGroupDTO
		out = append(out, g("regime_off", pinGate(tradeDTO.SignalBacktestParamsDTO{
			RegimeScaleLabels: str(""),
		})))
		for _, labels := range []string{"trend", "trend,vol"} {
			for _, f := range []float64{0.5, 0.25, 0} {
				out = append(out, g(fmt.Sprintf("rg_%s_x%.2f", strings.ReplaceAll(labels, ",", "+"), f),
					pinGate(tradeDTO.SignalBacktestParamsDTO{
						RegimeScaleLabels: str(labels),
						RegimeScaleFactor: f64(f),
					})))
			}
		}
		return out
	case "trail":
		// 移动止盈的大档：决定那 86~92% 的小赢能留下多少。生产值是
		// large_activate=40 / large_giveback=0.20（两账户相同），**就在网格内**，
		// 所以每一格都能直接和现值比。
		//
		// 只扫大档是有意的：tier 边界是上限的 30%/65%，而两账户全程顶格，
		// 持仓始终落在大档（A 的日志写着"档位[小≤6/大≥15]"）。中/小档扫了也
		// 碰不到，还会把拟合参数从 2 个涨到 6 个。
		//
		// 判据（事前）：净盈亏上升 **且** 兜底次数不增加——与趋势闸同一条。
		// 移动止盈只管赢单的出场，理论上不该影响兜底次数；如果某格兜底变多，
		// 说明它把本该止盈的仓拖成了扛单，那是要拒绝的。
		//
		// 事前预测：现值 40/0.20 已经接近最优、曲面偏平。理由是这参数此前
		// 调过，而且策略的 TP 很小（价格约 +0.3%），把 activate 拉到 60/90
		// 的仓根本走不到，只会退化成"不止盈"。若预测错，应该表现为低 activate
		// （25）明显更好——那意味着现在止盈**太晚**、白白回吐。
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, a := range []float64{25, 40, 60, 90} {
			for _, gb := range []float64{0.15, 0.20, 0.30} {
				out = append(out, g(fmt.Sprintf("trail_a%.0f_gb%.2f", a, gb),
					pinGate(tradeDTO.SignalBacktestParamsDTO{
						LargeActivatePct: f64(a), LargeGiveback: f64(gb),
					})))
			}
		}
		return out
	}
	return nil
}

// waitBatch 轮询到批次终态。批次执行是创建时就地起的 goroutine，CLI 只能等。
func waitBatch(svc *trade.TradeService, batchID int64) error {
	for i := 0; i < 1200; i++ {
		d, err := svc.GetSignalBacktestBatchDetail(batchID)
		if err != nil {
			return err
		}
		switch d.Batch.Status {
		case "done", "partial":
			return nil
		case "failed":
			return fmt.Errorf("批次失败: %s", d.Batch.ErrorMsg)
		}
		if i%20 == 19 {
			log.Printf("  %d/%d 组完成…", d.Batch.DoneCount, d.Batch.GroupCount)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("等待批次 %d 超时", batchID)
}

func main() {
	instance := flag.String("instance", "argus-single-roc", "实例键")
	account := flag.String("account", "", "账户标签（strategy_event.account_label，必填）")
	start := flag.String("start", "2026-09-08 21:00:00", "窗口起")
	end := flag.String("end", "2026-09-15 14:40:00", "窗口止")
	dim := flag.String("dim", "baseline", "baseline|trend|trendstop|regime|cap|stop|gate|trail")
	platform := flag.String("platform", "deepcoin", "1m 路径回放平台")
	conc := flag.Int("concurrency", 4, "并发组数")
	// 显式基线旋钮。默认 0 = 用服务端的基线解析器；但解析器对 risk_equity 与
	// reverse_gate_min_profit_pct 仍按"DB 尚无列"回落代码缺省（注释已过期，
	// argus_account_risk 里这两列都有值），于是"对着生产基线回测"实际对的是
	// gate=20 / risk_equity=initial_balance。校准必须把生产值显式钉上。
	riskEquity := flag.Float64("risk-equity", 0, "覆盖 riskEquity（0=用解析器）")
	budgetPct := flag.Float64("budget-pct", 0, "覆盖 budgetPct")
	ceiling := flag.Int("ceiling", 0, "覆盖 ceiling")
	capOverride := flag.Int("cap", 0, "覆盖 capOverride（>0 固定上限，绕过 cap 公式）")
	stopPct := flag.Float64("stop", 0, "覆盖 catastropheStopPct")
	gatePct := flag.Float64("gate", -1, "覆盖 gateMinProfitPct（-1=用解析器）")
	// 集合评估（见 signal/perturb.go）：同一格跑 N 条扰动路径，报分布与配对差。
	// 单路径的 ±50 与 09-17 那种"出场晚 3 分钟、之后 37 小时反向"的分叉同量级，
	// 不做这一步，任何一维的结论都分不清是信号还是路径噪声。
	ensembleN := flag.Int("ensemble", 0, "每格扰动路径数（0=不做集合评估）")
	dropPct := flag.Float64("drop", 0.04, "扰动：每条信号被丢弃的概率")
	lateProb := flag.Float64("late", 0.5, "扰动：出场判定成立时晚一根 bar 成交的概率")
	ensembleJSON := flag.String("ensemble-json", "", "集合评估结果另存为 JSON 的路径（可选）")
	ensembleRef := flag.String("ensemble-ref", "", "配对参照组 label（空=基线；填生产格如 trend_48h_3.0% / stop_400）")
	flag.Parse()

	if strings.TrimSpace(*account) == "" {
		log.Fatal("--account 必填：同一实例有 champion/challenger 两个账户，少这一维会把两本仓算成一本")
	}

	vipper.Init()
	db.InitDB()
	if db.Db == nil {
		log.Fatal("数据库初始化失败")
	}
	svc := trade.NewTradeService()
	ctx := context.Background()

	groups := grid(*dim)
	if len(groups) == 0 {
		// baseline：只跑基线本身。IncludeBaselineRun 默认 true，
		// 但 Groups 是 binding:"required"，所以给一组「全同基线」占位。
		groups = []tradeDTO.SignalBacktestGroupDTO{{Label: "baseline_only"}}
	}

	var base *tradeDTO.SignalBacktestParamsDTO
	if *riskEquity > 0 || *budgetPct > 0 || *ceiling > 0 || *capOverride > 0 || *stopPct > 0 || *gatePct >= 0 {
		base = &tradeDTO.SignalBacktestParamsDTO{}
		if *riskEquity > 0 {
			base.RiskEquity = f64(*riskEquity)
		}
		if *budgetPct > 0 {
			base.BudgetPct = f64(*budgetPct)
		}
		if *ceiling > 0 {
			base.Ceiling = i32(*ceiling)
		}
		if *capOverride > 0 {
			base.CapOverride = i32(*capOverride)
		}
		if *stopPct > 0 {
			base.CatastropheStopPct = f64(*stopPct)
		}
		if *gatePct >= 0 {
			base.GateMinProfitPct = f64(*gatePct)
		}
	}

	yes := true
	batchID, err := svc.CreateSignalBacktestBatch(ctx, tradeDTO.CreateSignalBacktestBatchDTO{
		BaselineParams:     base,
		Name:               fmt.Sprintf("sweep-%s-%s", *dim, *account),
		PlatformCode:       *platform,
		Symbol:             "BTCUSDT",
		StartTime:          *start,
		EndTime:            *end,
		InstanceKey:        *instance,
		AccountLabel:       *account,
		Concurrency:        *conc,
		IncludeBaselineRun: &yes,
		Groups:             groups,
	})
	if err != nil {
		log.Fatalf("建批次失败：%v", err)
	}
	// CreateSignalBacktestBatch 内部已经 `go RunSignalBacktestBatch(batchID)`。
	// 这里**不能**再调一次：runBatchGroup 的"已 done 就跳过"是 check-then-act，
	// 两个执行器并发时都会看到 status != done，于是逐笔被 BatchCreate 写两份
	// （汇总仍对，因为指标来自 signal.Aggregate，但明细列表每笔重复）。
	log.Printf("批次 %d 已建：%d 组（含基线），等待回放完成…", batchID, len(groups))
	if err := waitBatch(svc, batchID); err != nil {
		log.Fatalf("回放失败：%v", err)
	}

	detail, err := svc.GetSignalBacktestBatchDetail(batchID)
	if err != nil {
		log.Fatalf("读结果失败：%v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "组\t净盈亏\t毛盈亏\t手续费\t笔数\t胜率\t盈亏比\t最大回撤\t回撤%本金\t兜底\t最大张数\t上限跳过\t门控拦\t趋势拦\t精度")
	for _, fg := range detail.Groups {
		for _, r := range fg.Rows {
			m := r.Metric
			if m == nil {
				fmt.Fprintf(w, "%s\t(无指标: %s %s)\n", r.GroupLabel, r.Status, r.ErrorMsg)
				continue
			}
			fmt.Fprintf(w, "%s\t%+.2f\t%+.2f\t%.2f\t%d\t%.0f%%\t%.2f\t%.2f\t%.1f%%\t%d\t%d\t%d\t%d\t%d\t%s\n",
				r.GroupLabel, m.NetPnl, m.GrossPnl, m.FeeTotal, m.FillCount, m.WinRate*100,
				m.ProfitFactor, m.MaxDrawdown, m.MaxDrawdownPct, m.SlCount, m.MaxStack, m.CapSkipCount, m.GateSkipCount,
				m.TrendSkipCount, m.Fidelity)
		}
	}
	w.Flush()
	for _, warn := range detail.Warnings {
		fmt.Printf("⚠️  %s\n", warn)
	}
	fmt.Printf("\n批次 id=%d（结果已落 trade_backtest_run/_trade/_metric，可在管理端「信号回测」里复核）\n", batchID)

	if *ensembleN > 0 {
		rep, err := svc.RunSignalEnsemble(ctx, trade.SignalEnsembleRequest{
			InstanceKey: *instance, AccountLabel: *account, Symbol: "BTCUSDT", PlatformCode: *platform,
			Start: *start, End: *end, BaselineParams: base, Groups: groups, N: *ensembleN,
			Perturb:  signal.Perturb{SignalDropPct: *dropPct, ExitLateProb: *lateProb},
			RefLabel: *ensembleRef,
		})
		if err != nil {
			log.Fatalf("集合评估失败：%v", err)
		}
		printEnsemble(rep)
		if *ensembleJSON != "" {
			if b, err := json.MarshalIndent(rep, "", "  "); err == nil {
				if err := os.WriteFile(*ensembleJSON, b, 0o644); err != nil {
					log.Printf("写 %s 失败：%v", *ensembleJSON, err)
				}
			}
		}
	}
}

// printEnsemble 分布表。读法：先看"同向"——某格要在 ≥ 3/4 的路径上都优于基线才算有信号；
// 再看 Δp10/最小差是否为负得离谱；最后才看中位数大小。兜底列看"不劣于基线"的路径数。
func printEnsemble(rep *trade.SignalEnsembleReport) {
	fmt.Printf("\n集合评估：N=%d 条扰动路径/格，丢信号 %.0f%%，出场晚一根 %.0f%%（信号 %d 条，K 线 %d 根）\n",
		rep.N, rep.Perturb.SignalDropPct*100, rep.Perturb.ExitLateProb*100, rep.Signals, rep.Bars)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "组\t单路径\t中位\tp10\t最小\t最大\t兜底中位/最大\tΔ中位vs %s\tΔ最小\t同向\t兜底不劣\n", rep.RefLabel)
	row := func(g trade.SignalEnsembleGroup, withDelta bool) {
		st := g.Stats
		d := "\t\t\t"
		if withDelta && g.Label != rep.RefLabel {
			d = fmt.Sprintf("%+.2f\t%+.2f\t%d/%d\t%d/%d", g.VsBaseline.NetDeltaMedian, g.VsBaseline.NetDeltaMin,
				g.VsBaseline.Better, g.VsBaseline.N, g.VsBaseline.CatNotWorse, g.VsBaseline.N)
		}
		fmt.Fprintf(w, "%s\t%+.2f\t%+.2f\t%+.2f\t%+.2f\t%+.2f\t%.0f/%d\t%s\n",
			g.Label, g.SinglePath, st.NetMedian, st.NetP10, st.NetMin, st.NetMax, st.CatMedian, st.CatMax, d)
	}
	row(rep.Baseline, false)
	for _, g := range rep.Groups {
		row(g, true)
	}
	w.Flush()
	for _, n := range rep.Notes {
		fmt.Printf("⚠️  %s\n", n)
	}
}
