package trade

import (
	"fmt"
	"strings"
)

// TrendStopParams 趋势条件止损的参数。
//
//	BaseStopPct  S：常规兜底线（position.monitor.catastrophe_stop_pct）
//	TriggerPct   X：逆向窗口动量达到多少个百分点才收紧；≤0 = 未启用
//	TightStopPct Y：收紧后的兜底线；≤0 = 未启用，≥S 视为没收紧
type TrendStopParams struct {
	BaseStopPct  float64
	TriggerPct   float64
	TightStopPct float64
}

// adverseMomentumPct 把窗口动量换算成「顶着这个持仓的幅度」：空仓怕涨、多仓怕跌。
// 与 EvaluateTrendGate 的逆势判定同源——两处若各写一遍符号，改一边必然漏另一边，
// 于是"闸门认为逆势"和"止损认为逆势"会对不上。方向读不到时返回 ok=false。
func adverseMomentumPct(posSide string, momPct float64) (float64, bool) {
	switch {
	case strings.EqualFold(posSide, "short"):
		return momPct, true
	case strings.EqualFold(posSide, "long"):
		return -momPct, true
	}
	return 0, false
}

// ResolveTrendStop 决定本次判定该用哪条兜底线，返回 (生效值, 是否被收紧)。
//
// 为什么是条件收紧而不是全时段收紧：均值回归策略的赢单本来就要先深亏
// （80 天样本里最低 ROI 到过 -338% 仍翻回 +77% 平仓），一刀切收紧会把赢单砍成
// 亏损单——ValidateRiskParams 里那条「catastrophe_stop_pct 必须 ≥250（松兜底
// 护栏：紧止损杀死均值回归 edge）」就是这个意思。而 231 笔平仓的判别式显示，
// 平仓时刻的逆向 24h 动量在赢单是中位 -0.72%、在兜底单是中位 +4.09%，
// ≥3% 的占比 6% vs 62%：趋势环境能把两者分开，所以只在逆向趋势里收紧。
// 依据见 doc/module/收益诊断-2026-09-15。
//
// 三条 fail-safe，方向一致——**没有证据就不动别人的兜底线**：
//   - 未启用（X 或 Y ≤0）：沿用 S。缺省不生效，部署安全。
//   - Y ≥ S：配错了，沿用 S。本函数只能收紧，绝不放宽安全网。
//   - momOK=false（重启后未回填 / 数据断流）或方向读不到：沿用 S。
func ResolveTrendStop(posSide string, momPct float64, momOK bool, p TrendStopParams) (float64, bool) {
	if p.TriggerPct <= 0 || p.TightStopPct <= 0 {
		return p.BaseStopPct, false
	}
	if p.TightStopPct >= p.BaseStopPct {
		return p.BaseStopPct, false
	}
	if !momOK {
		return p.BaseStopPct, false
	}
	adverse, ok := adverseMomentumPct(posSide, momPct)
	if !ok {
		return p.BaseStopPct, false
	}
	if adverse >= p.TriggerPct {
		return p.TightStopPct, true
	}
	return p.BaseStopPct, false
}

// TrendStopReason 收紧时写进事件与 Telegram 告警的说明。
// 必须同时给出动量、阈值和两条兜底线——否则事后看到一笔 -250% 的平仓，
// 没人能判断它是配置改了还是趋势闸收紧了。
func TrendStopReason(posSide string, momPct float64, p TrendStopParams) string {
	adverse, _ := adverseMomentumPct(posSide, momPct)
	return fmt.Sprintf("趋势条件止损: 逆向动量%+.2f%% ≥ %.1f%%, 兜底线 %.0f%% → %.0f%%",
		adverse, p.TriggerPct, p.BaseStopPct, p.TightStopPct)
}
