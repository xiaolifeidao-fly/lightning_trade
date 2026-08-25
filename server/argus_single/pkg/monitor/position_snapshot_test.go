package monitor

import (
	"testing"

	"common/utils"
)

func TestSnapshotFromPosition(t *testing.T) {
	pos := utils.PositionInfo{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "15",
		AvgPx: "63000.5", LastPx: "62500.0", UnrealizedProfit: "-7.5", UseMargin: "7.5",
		PosId: "p123", UTime: "1700000000000", CTime: "1700000100000"}
	snap, ok := snapshotFromPosition(pos)
	if !ok {
		t.Fatal("有效持仓应生成快照")
	}
	if snap.Size != 15 || snap.AvgPx != 63000.5 || snap.PosId != "p123" || snap.UTime != "1700000000000" {
		t.Fatalf("快照字段: %+v", snap)
	}
	if snap.Upl != -7.5 || snap.RoiPct != -100.0 {
		t.Fatalf("upl/roi 计算(long last<avg 为负, roi=upl/margin): %+v", snap)
	}
	// dead/unknown 不生成快照
	if _, ok := snapshotFromPosition(utils.PositionInfo{InstId: "x", PosSide: "long", Pos: "0"}); ok {
		t.Fatal("Pos=0 不应生成快照")
	}
	if _, ok := snapshotFromPosition(utils.PositionInfo{InstId: "x", PosSide: "long", Pos: ""}); ok {
		t.Fatal("Pos 空不应生成快照（unknown 保留旧快照由调用方处理）")
	}
}

func TestFindExternalCloses(t *testing.T) {
	prev := map[string]posSnapshot{
		"账户A:BTC-USDT-SWAP:long":  {InstId: "BTC-USDT-SWAP", PosSide: "long", Size: 15, Upl: -7.5, PosId: "p1"},
		"账户A:BTC-USDT-SWAP:short": {InstId: "BTC-USDT-SWAP", PosSide: "short", Size: 3, Upl: 1.2, PosId: "p2"},
		"账户A:ETH-USDT-SWAP:long":  {InstId: "ETH-USDT-SWAP", PosSide: "long", Size: 2, Upl: 0.5, PosId: "p3"},
	}
	live := map[string]bool{"账户A:BTC-USDT-SWAP:long": true}    // short 与 ETH 消失
	consumed := map[string]bool{"ETH-USDT-SWAP:long:p3": true} // ETH 是本机器人平的
	got := findExternalCloses(prev, live, func(instId, posSide, posId string) bool {
		return consumed[instId+":"+posSide+":"+posId]
	})
	if len(got) != 1 || got[0].PosSide != "short" || got[0].Size != 3 {
		t.Fatalf("应只报 short 为外部平仓: %+v", got)
	}
	// review fix#2: 标记的 posId 与消失仓位不同（旧仓标记 vs 新仓消失）→ 不消费 → 仍报 external
	stale := map[string]bool{"ETH-USDT-SWAP:long:旧仓posId": true}
	got2 := findExternalCloses(prev, live, func(instId, posSide, posId string) bool {
		return stale[instId+":"+posSide+":"+posId]
	})
	if len(got2) != 2 {
		t.Fatalf("旧仓标记不得吞掉新仓的 external_close, got %+v", got2)
	}
	// 全部存活 → 无外部平仓
	if n := len(findExternalCloses(prev, map[string]bool{
		"账户A:BTC-USDT-SWAP:long": true, "账户A:BTC-USDT-SWAP:short": true, "账户A:ETH-USDT-SWAP:long": true,
	}, func(string, string, string) bool { return false })); n != 0 {
		t.Fatalf("全部存活不应报外部平仓, got %d", n)
	}
	// 首轮（prev 空）→ 无
	if n := len(findExternalCloses(nil, map[string]bool{}, func(string, string, string) bool { return false })); n != 0 {
		t.Fatalf("首轮不应报外部平仓, got %d", n)
	}
}

func TestUpdateSnapshotsLifecycle(t *testing.T) {
	am := &AccountMonitor{snapshots: map[string]posSnapshot{
		"账户A:BTC-USDT-SWAP:long":  {InstId: "BTC-USDT-SWAP", PosSide: "long", Size: 10, PosId: "p1"},
		"账户A:BTC-USDT-SWAP:short": {InstId: "BTC-USDT-SWAP", PosSide: "short", Size: 5, PosId: "p2"},
	}}
	am.updateSnapshots("账户A", []utils.PositionInfo{
		// live: 覆盖（加仓 10→12）
		{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "12", AvgPx: "63000", LastPx: "63100",
			UnrealizedProfit: "1.2", UseMargin: "6.0", PosId: "p1"},
		// unknown(Pos 空): 保留旧快照不覆盖
		{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "", PosId: "垃圾"},
	})
	if am.snapshots["账户A:BTC-USDT-SWAP:long"].Size != 12 {
		t.Fatalf("live 应覆盖: %+v", am.snapshots)
	}
	if s := am.snapshots["账户A:BTC-USDT-SWAP:short"]; s.Size != 5 || s.PosId != "p2" {
		t.Fatalf("unknown 应保留旧快照: %+v", s)
	}
	// 整条消失 → 快照删除
	am.updateSnapshots("账户A", []utils.PositionInfo{})
	if len(am.snapshots) != 0 {
		t.Fatalf("消失的仓位快照应删除: %+v", am.snapshots)
	}
}

