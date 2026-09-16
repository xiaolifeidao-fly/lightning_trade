package trade

import (
	"testing"

	"argus_single/pkg/eventlog"
)

// open 事件必须带上决策时刻的持仓均价。
//
// 修的是这个 bug：episode_entry.avg_px 100% 为 NULL。根因不在派生层——
// derive.go 明确写了 AvgPx: e.AvgPx，装载也没做 Select 限制——而在这里：
// open 事件的字面量原本不带 AvgPx，只有 isReduction 分支里作为减仓盈亏估算的
// 副产品才赋值。而减仓**不产生建仓决策行**，所以 3299 条真正的开仓/加仓事件
// 一条都没有 avg_px（带 reason='减仓锁利' 的 626 条则 626 条全有，1:1 吻合）。
//
// 工作参照是同文件的 gate_block：它在字面量里显式写了 AvgPx/LastPx，所以 100% 有值。

func quoteOK() SignalQuote {
	return SignalQuote{Last: 60000, Mark: 60010, GapBp: 1.5} // OK() 判据是 Mark>0
}

// 加仓：net 是下单前的持仓快照，net.AvgPx 正是字段注释说的"决策时刻的持仓均价"。
func TestBuildOpenEventCarriesAvgPxOnAdd(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试", Variant: "champion/default"}
	net := NetPosition{Side: "long", Size: 5, AvgPx: 59500, LastPx: 60000}

	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "long", net, true, 1, 0.001, quoteOK())

	if ev.AvgPx != 59500 {
		t.Errorf("加仓应带上下单前均价 59500，实际 %v", ev.AvgPx)
	}
	if ev.LastPx != 60000 {
		t.Errorf("加仓应带上最新价 60000，实际 %v", ev.LastPx)
	}
	if ev.Size != 6 {
		t.Errorf("Size 记开仓后净仓（5+1），实际 %d", ev.Size)
	}
	if ev.OrderSize != 1 {
		t.Errorf("OrderSize 记本次下单张数，实际 %d", ev.OrderSize)
	}
	if ev.Reason != "" {
		t.Errorf("加仓不该带减仓的 reason，实际 %q", ev.Reason)
	}
}

// 全新开仓：下单前没有持仓，所以"持仓均价"不存在 → 留 0（落库经 nonZeroPtr 变 NULL）。
// 这不是缺口，是正确语义：第一张合约成交之前没有均价可言。
func TestBuildOpenEventLeavesAvgPxZeroOnFreshOpen(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试"}
	net := NetPosition{Size: 0} // 空仓快照

	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "long", net, true, 2, 0.001, quoteOK())

	if ev.AvgPx != 0 {
		t.Errorf("全新开仓不存在持仓均价，应留 0 让落库成 NULL，实际 %v", ev.AvgPx)
	}
	if ev.Size != 2 {
		t.Errorf("全新开仓 Size = 本次张数 2，实际 %d", ev.Size)
	}
}

// 净仓查询失败：不得落任何价格字段，与 applySignalQuote 的
// "不可算：整体省略，不得落 0 冒充真实观测"同口径。
func TestBuildOpenEventOmitsPricesWhenNetUnknown(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试"}
	net := NetPosition{Side: "long", Size: 5, AvgPx: 59500, LastPx: 60000}

	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "long", net, false, 1, 0.001, quoteOK())

	if ev.AvgPx != 0 || ev.LastPx != 0 {
		t.Errorf("netOK=false 时不得落价格，实际 avg=%v last=%v", ev.AvgPx, ev.LastPx)
	}
	if ev.Size != 1 {
		t.Errorf("净仓不可知时 Size 退化为本次张数，实际 %d", ev.Size)
	}
}

// 反向减仓：原有行为必须一字不变——带 NetSide、reason、估算的 pnl 与 roi。
// 这一条是回归护栏：修 avg_px 时很容易把减仓分支的语义一起改掉。
func TestBuildOpenEventPreservesReductionSemantics(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试"}
	net := NetPosition{Side: "long", Size: 5, AvgPx: 59000, LastPx: 60000}

	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "short", net, true, 2, 0.001, quoteOK())

	if ev.NetSide != "long" {
		t.Errorf("减仓必须写 NetSide=真实净仓方向，实际 %q", ev.NetSide)
	}
	if ev.Reason == "" {
		t.Error("减仓必须带 reason（下游靠它识别这是减仓而非建仓）")
	}
	if ev.Size != 3 {
		t.Errorf("减仓后净仓 5-2=3，实际 %d", ev.Size)
	}
	if ev.AvgPx != 59000 || ev.LastPx != 60000 {
		t.Errorf("减仓仍应带价格，实际 avg=%v last=%v", ev.AvgPx, ev.LastPx)
	}
	if ev.Pnl == 0 {
		t.Error("减仓应带估算的已实现盈亏")
	}
	if ev.RoiPct == 0 {
		t.Error("减仓应带 ROI")
	}
}

// 减仓削到 0 不能出现负的净仓。
func TestBuildOpenEventFloorsNetAtZero(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试"}
	net := NetPosition{Side: "long", Size: 2, AvgPx: 59000, LastPx: 60000}

	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "short", net, true, 5, 0.001, quoteOK())

	if ev.Size != 0 {
		t.Errorf("减仓超出持仓时净仓应为 0 而非负数，实际 %d", ev.Size)
	}
}

// 信号报价要照既有约定一并带上（applySignalQuote）。
func TestBuildOpenEventAppliesSignalQuote(t *testing.T) {
	acc := AccountConfig{Name: "账户A-测试"}
	ev := buildOpenEvent(acc, "BTC-USDT-SWAP", "long", NetPosition{Size: 0}, true, 1, 0.001, quoteOK())
	if ev.SigLast != 60000 || ev.SigMark != 60010 {
		t.Errorf("应带信号报价，实际 last=%v mark=%v", ev.SigLast, ev.SigMark)
	}
	if ev.Event != eventlog.EvOpen {
		t.Errorf("事件类型应为 open，实际 %q", ev.Event)
	}
}
