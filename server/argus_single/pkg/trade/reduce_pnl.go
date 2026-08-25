package trade

import "strings"

// netRoiPct 净仓 ROI%（口径与移动止盈/门控一致：价格有利变动幅度 × 杠杆）。
// avgPx<=0 防御返回 0。纯函数（P2-D，从 EvaluateReverseGate 抽出复用）。
func netRoiPct(netSide string, avgPx, lastPx float64, leverage int) float64 {
	if avgPx <= 0 {
		return 0
	}
	if strings.EqualFold(netSide, "long") {
		return (lastPx - avgPx) / avgPx * float64(leverage) * 100
	}
	return (avgPx - lastPx) / avgPx * float64(leverage) * 100
}

// estimateReducePnl 反向减仓的已实现盈亏估算（P2-D）：
// long 净仓被减 = (lastPx−avgPx)×face×orderSize；short 反号。
// lastPx 是门控时点价而非实际成交价，误差≈滑点——事件 reason 标注"估算"。
// 任一输入非正防御返回 0。纯函数。
func estimateReducePnl(netSide string, avgPx, lastPx, face float64, orderSize int) float64 {
	if avgPx <= 0 || lastPx <= 0 || face <= 0 || orderSize <= 0 {
		return 0
	}
	diff := lastPx - avgPx
	if !strings.EqualFold(netSide, "long") {
		diff = -diff
	}
	return diff * face * float64(orderSize)
}

