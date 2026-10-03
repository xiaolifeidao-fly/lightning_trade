package monitor

import (
	"fmt"

	"argus_single/pkg/trade"

	"github.com/shopspring/decimal"
)

// 本金回撤兜底（权益线）。
//
// ROI 兜底按均价定义：摊平把均价拉向现价、把 ROI 拉浅，于是满仓 8 张的 −400% 对 B
// 是"离开仓价 3.2%"，三周里 4 笔兜底全是 3.2~3.5% 的针或磨、随后回头。权益线按未实现
// 亏损 USDT 定义，加仓只改变敞口不改变止损线；20% × risk_equity 在满仓时 ≈ 价格 −4.85%。
// 启用后 ROI 兜底（含趋势条件止损收紧后的线）改为永不触发——替换，不叠加。
//
// 研究依据与判据见 doc/module/收益诊断-2026-09-15/诊断报告.md（09-19 下午、10-03 追补）。

// equityModeRoiStop 权益线模式下 ROI 兜底的哨兵值：永不触发。
const equityModeRoiStop = 1e9

// resolveEquityStopLine 权益线（USDT）= equity_stop_pct% × risk_equity；未配置返回 (0,false)。
func resolveEquityStopLine(acc trade.AccountConfig) (float64, bool) {
	pct := trade.ResolveEquityStopPct(acc)
	if pct <= 0 {
		return 0, false
	}
	e := trade.ResolveRiskEquity(acc)
	if e <= 0 {
		return 0, false
	}
	return pct / 100 * e, true
}

// applyEquityStop 权益线模式：ROI 兜底改为永不触发，移动止盈分档原样保留。
func applyEquityStop(tp TrailParams) TrailParams {
	tp.CatastropheStopPct = equityModeRoiStop
	return tp
}

// equityStopHit 未实现亏损 ≥ 线即触发（pnl 为负数）。
func equityStopHit(pnl decimal.Decimal, line float64) bool {
	return line > 0 && pnl.LessThanOrEqual(decimal.NewFromFloat(-line))
}

// equityStopReason 平仓理由；含"兜底"二字 → closeTrailing 归为 catastrophe_stop 事件。
func equityStopReason(line float64) string {
	return fmt.Sprintf("兜底止损(权益线 %.1fU)", line)
}
