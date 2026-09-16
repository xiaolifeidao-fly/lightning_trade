package trade

import (
	"strings"

	"argus_single/pkg/eventlog"
)

// buildOpenEvent 造一条 open 事件（全新开仓 / 加仓 / 反向减仓共用）。
//
// 为什么提成独立纯函数：这段逻辑原先内联在 executeSignalTrades_From_WEB 的
// goroutine 里，没有测试缝。结果「avg_px 只在减仓分支被赋值」这个缺口在
// episode_entry 里躺了 3299 条决策行（100% NULL）才被发现——而排查时还得先
// 逐层排掉派生层、装载层的嫌疑。有了这个缝，口径可以逐条钉住。
//
// AvgPx/LastPx 取自 net：**下单前**的持仓快照，正是 episode_entry.avg_px 字段
// 注释写的"决策时刻的持仓均价"。写法照同包 gate_block 的既有参照
// （它在事件字面量里显式带 AvgPx/LastPx，所以覆盖率一直是 100%）。
//
// 全新开仓时 net.Size==0、均价为 0，落库经 nonZeroPtr 变 NULL。这是**正确语义
// 而非缺口**：第一张合约成交之前不存在"持仓均价"。netOK=false（净仓查询失败）
// 同样不落任何价格，与 applySignalQuote 的约定一致——不可算则整体省略，
// 不得落 0 冒充真实观测。
func buildOpenEvent(acc AccountConfig, instId, posSide string, net NetPosition, netOK bool,
	orderSize int, faceValue float64, q SignalQuote) eventlog.Event {

	// Size 记开仓后预计净仓张数（让"最大堆积"口径正确）；OrderSize 记本次下单张数。
	postNet := orderSize
	if netOK {
		if net.Size == 0 || strings.EqualFold(posSide, net.Side) {
			postNet = net.Size + orderSize // 全新 / 加仓
		} else {
			postNet = net.Size - orderSize // 反向减仓
			if postNet < 0 {
				postNet = 0
			}
		}
	}

	ev := eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: instId,
		Event: eventlog.EvOpen, Side: posSide, Size: postNet, OrderSize: orderSize}

	if netOK {
		ev.AvgPx, ev.LastPx = net.AvgPx, net.LastPx
	}
	if netOK && isReduction(posSide, net.Side, net.Size) {
		// P2-D：减仓锁利的已实现盈亏估算（lastPx=门控时点价，非成交价）
		ev.Pnl = estimateReducePnl(net.Side, net.AvgPx, net.LastPx, faceValue, orderSize)
		ev.RoiPct = netRoiPct(net.Side, net.AvgPx, net.LastPx, SignalLeverage)
		ev.NetSide = net.Side
		ev.Reason = "减仓锁利(pnl为估算)"
	}
	return applySignalQuote(ev, q)
}
