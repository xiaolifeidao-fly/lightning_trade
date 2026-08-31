package eventlog

// AccountMetrics 单账户在一段事件上的聚合指标（对比报告用）。
type AccountMetrics struct {
	Account, Variant                                          string
	Opens, CapSkips, GateBlocks, TrendSkips                   int
	TrailingCloses, CatastropheStops, LossAlerts, FixedCloses int
	ExternalCloses, ManualCloses                              int // 非策略平仓：展示但不进胜率/ROI（P2-E）
	MaxSize                                                   int
	FirstBalance, LastBalance                                 float64
	MaxDrawdownPct                                            float64 // 权益口径回撤（仅权益已知样本；覆盖率<100% 时为真实回撤下界）
	BalanceDrawdownPct                                        float64 // 已实现余额口径回撤（全样本；权益全未知时报告回退用）
	LastEquity                                                float64 // 最近一次已知权益（是否过时见 LastEquityKnown）
	LastEquityKnown                                           bool    // 最后一条 balance 事件是否带 equity（false=当前权益未知）
	BalanceEvents, EquityKnownEvents                          int     // 权益覆盖率分母/分子
	CloseCount, CloseWins                                     int
	CloseRoiSum, CloseRoiMin, CloseRoiMax, ClosePnlSum        float64
}

// EquityCoveragePct 权益已知样本占比%（报告可信度标注；无 balance 事件时为 0）。
func (m *AccountMetrics) EquityCoveragePct() float64 {
	if m.BalanceEvents == 0 {
		return 0
	}
	return float64(m.EquityKnownEvents) / float64(m.BalanceEvents) * 100
}

// PnLPct 区间收益率%（基于 balance 序列首尾）。
func (m *AccountMetrics) PnLPct() float64 {
	if m.FirstBalance <= 0 {
		return 0
	}
	return (m.LastBalance - m.FirstBalance) / m.FirstBalance * 100
}

// CloseAvgRoi 平仓平均 ROI%。
func (m *AccountMetrics) CloseAvgRoi() float64 {
	if m.CloseCount == 0 {
		return 0
	}
	return m.CloseRoiSum / float64(m.CloseCount)
}

// CloseWinRate 平仓胜率（0~1）。
func (m *AccountMetrics) CloseWinRate() float64 {
	if m.CloseCount == 0 {
		return 0
	}
	return float64(m.CloseWins) / float64(m.CloseCount)
}

// Aggregate 把事件按账户聚合为指标。纯函数，可测。
func Aggregate(events []Event) map[string]*AccountMetrics {
	out := map[string]*AccountMetrics{}
	peak := map[string]float64{}    // 权益口径峰（仅权益已知样本）
	peakBal := map[string]float64{} // 已实现余额口径峰（全样本）
	for _, e := range events {
		if e.Event == EvDevSample {
			continue // 市场侧事件（无账户），否则会聚出一个空账户名的空条目
		}
		am := out[e.Account]
		if am == nil {
			am = &AccountMetrics{Account: e.Account}
			out[e.Account] = am
		}
		if e.Variant != "" {
			am.Variant = e.Variant
		}
		if e.Size > am.MaxSize {
			am.MaxSize = e.Size
		}
		switch e.Event {
		case EvOpen:
			am.Opens++
		case EvCapSkip:
			am.CapSkips++
		case EvTrendSkip:
			am.TrendSkips++
		case EvGateBlock:
			am.GateBlocks++
		case EvLossAlert:
			am.LossAlerts++
		case EvExternalClose:
			am.ExternalCloses++
		case EvManualClose:
			am.ManualCloses++
		case EvTrailingClose, EvCatastropheStop, EvFixedClose:
			switch e.Event {
			case EvTrailingClose:
				am.TrailingCloses++
			case EvCatastropheStop:
				am.CatastropheStops++
			case EvFixedClose:
				am.FixedCloses++
			}
			if am.CloseCount == 0 {
				am.CloseRoiMin, am.CloseRoiMax = e.RoiPct, e.RoiPct
			} else {
				if e.RoiPct < am.CloseRoiMin {
					am.CloseRoiMin = e.RoiPct
				}
				if e.RoiPct > am.CloseRoiMax {
					am.CloseRoiMax = e.RoiPct
				}
			}
			am.CloseCount++
			am.CloseRoiSum += e.RoiPct
			am.ClosePnlSum += e.Pnl
			if e.RoiPct > 0 {
				am.CloseWins++
			}
		case EvBalance:
			if am.FirstBalance == 0 {
				am.FirstBalance = e.Balance
			}
			am.LastBalance = e.Balance
			am.BalanceEvents++
			// 已实现余额口径回撤（全样本；权益全未知时报告回退用）
			if e.Balance > peakBal[e.Account] {
				peakBal[e.Account] = e.Balance
			}
			if peakBal[e.Account] > 0 {
				if dd := (peakBal[e.Account] - e.Balance) / peakBal[e.Account] * 100; dd > am.BalanceDrawdownPct {
					am.BalanceDrawdownPct = dd
				}
			}
			// 权益口径回撤只用权益已知样本。权益未知（老日志/启动首查/
			// UPL 陈旧被省略）——若回退 balance 会双向失真：
			// balance 假峰虚增回撤、接口故障期深水浮亏被隐藏。未知就是未知，
			// 由报告层标注下界与覆盖率，不在此伪造数据。
			// 已知性优先看 equityKnown 显式标记（0/负权益也是已知样本，
			// equity 的 omitempty 会省略 0 值）；老日志无此字段回退 Equity>0。
			known := e.EquityKnown || e.Equity > 0
			am.LastEquityKnown = known
			if known {
				am.EquityKnownEvents++
				am.LastEquity = e.Equity
				if e.Equity > peak[e.Account] {
					peak[e.Account] = e.Equity
				}
				if peak[e.Account] > 0 { // 无正峰则回撤无定义（首样本即 ≤0 时防除零）
					if dd := (peak[e.Account] - e.Equity) / peak[e.Account] * 100; dd > am.MaxDrawdownPct {
						am.MaxDrawdownPct = dd
					}
				}
			}
		}
	}
	return out
}
