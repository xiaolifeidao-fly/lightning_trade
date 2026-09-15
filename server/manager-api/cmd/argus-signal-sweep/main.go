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
)

func f64(v float64) *float64 { return &v }
func i32(v int) *int         { return &v }

// grid 返回某个维度要扫的参数组。组参数是【相对基线的增量】——没给的旋钮沿用
// 该实例当前生产值，所以 diff 里只有被扫的那一行。
func grid(dim string) []tradeDTO.SignalBacktestGroupDTO {
	g := func(label string, p tradeDTO.SignalBacktestParamsDTO) tradeDTO.SignalBacktestGroupDTO {
		return tradeDTO.SignalBacktestGroupDTO{Label: label, SignalBacktestParamsDTO: p}
	}
	switch dim {
	case "trend":
		// 趋势闸是唯一为「禁止逆势加仓」而生的机制，生产上 threshold=0 关着，
		// 且 trend_mom_pct 全为 NULL（阈值 0 时 tracker 根本不构造）。
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, w := range []float64{1, 2, 4, 8, 24} {
			for _, t := range []float64{0.8, 1.5, 3.0} {
				out = append(out, g(fmt.Sprintf("trend_%gh_%.1f%%", w, t),
					tradeDTO.SignalBacktestParamsDTO{
						TrendGateWindowHours:  f64(w),
						TrendGateThresholdPct: f64(t),
					}))
			}
		}
		return out
	case "cap":
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, c := range []int{6, 8, 12, 16, 22, 26, 34} {
			out = append(out, g(fmt.Sprintf("cap_%d", c),
				tradeDTO.SignalBacktestParamsDTO{CapOverride: i32(c)}))
		}
		return out
	case "stop":
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, s := range []float64{150, 200, 250, 300, 400, 500, 650} {
			out = append(out, g(fmt.Sprintf("stop_%.0f", s),
				tradeDTO.SignalBacktestParamsDTO{CatastropheStopPct: f64(s)}))
		}
		return out
	case "gate":
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, v := range []float64{0, 4, 8, 15, 25, 40} {
			out = append(out, g(fmt.Sprintf("gate_%.0f", v),
				tradeDTO.SignalBacktestParamsDTO{GateMinProfitPct: f64(v)}))
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
	case "trail":
		var out []tradeDTO.SignalBacktestGroupDTO
		for _, a := range []float64{25, 40, 60, 90} {
			for _, gb := range []float64{0.15, 0.20, 0.30} {
				out = append(out, g(fmt.Sprintf("trail_a%.0f_gb%.2f", a, gb),
					tradeDTO.SignalBacktestParamsDTO{
						LargeActivatePct: f64(a), LargeGiveback: f64(gb),
					}))
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
	dim := flag.String("dim", "baseline", "baseline|trend|trendstop|cap|stop|gate|trail")
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
	fmt.Fprintln(w, "组\t净盈亏\t毛盈亏\t手续费\t笔数\t胜率\t盈亏比\t最大回撤\t兜底\t上限跳过\t门控拦\t趋势拦\t精度")
	for _, fg := range detail.Groups {
		for _, r := range fg.Rows {
			m := r.Metric
			if m == nil {
				fmt.Fprintf(w, "%s\t(无指标: %s %s)\n", r.GroupLabel, r.Status, r.ErrorMsg)
				continue
			}
			fmt.Fprintf(w, "%s\t%+.2f\t%+.2f\t%.2f\t%d\t%.0f%%\t%.2f\t%.2f\t%d\t%d\t%d\t%d\t%s\n",
				r.GroupLabel, m.NetPnl, m.GrossPnl, m.FeeTotal, m.FillCount, m.WinRate*100,
				m.ProfitFactor, m.MaxDrawdown, m.SlCount, m.CapSkipCount, m.GateSkipCount,
				m.TrendSkipCount, m.Fidelity)
		}
	}
	w.Flush()
	for _, warn := range detail.Warnings {
		fmt.Printf("⚠️  %s\n", warn)
	}
	fmt.Printf("\n批次 id=%d（结果已落 trade_backtest_run/_trade/_metric，可在管理端「信号回测」里复核）\n", batchID)
}
