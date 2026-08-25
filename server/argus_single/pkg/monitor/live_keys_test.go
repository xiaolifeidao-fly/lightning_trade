package monitor

import (
	"testing"

	"common/utils"
)

func TestClassifyPosLivenessThreeStates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want posLiveness
	}{
		{"有效非零多头", "12", posLive},
		{"有效非零空头(负数)", "-5", posLive},
		{"有效零=仓位不存在", "0", posDead},
		{"空串=瞬时脏数据", "", posUnknown},
		{"空白串=瞬时脏数据", "  ", posUnknown},
		{"非法串=瞬时脏数据", "abc", posUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPosLiveness(tt.raw); got != tt.want {
				t.Fatalf("classifyPosLiveness(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildLiveKeysRetainsUnknownAndLiveOnly(t *testing.T) {
	positions := []utils.PositionInfo{
		// 核心 bug 场景：UPL/UseMargin 瞬空但 Pos 有效非零 → 必须保留
		{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "15", UnrealizedProfit: "", UseMargin: ""},
		// Pos 本身非法 → unknown → 保留一轮
		{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "abc"},
		// Pos 有效零 → dead → 不保留
		{InstId: "ETH-USDT-SWAP", PosSide: "long", Pos: "0"},
		// key 组成字段缺失 → 无法构成有效 key，跳过
		{InstId: "", PosSide: "long", Pos: "3"},
	}
	got := buildLiveKeys("账户A", positions)

	if !got["账户A:BTC-USDT-SWAP:long"] {
		t.Fatalf("UPL 瞬空但 Pos 有效非零的仓位必须保留, got %v", got)
	}
	if !got["账户A:BTC-USDT-SWAP:short"] {
		t.Fatalf("Pos 非法(unknown)的仓位应保守保留一轮, got %v", got)
	}
	if got["账户A:ETH-USDT-SWAP:long"] {
		t.Fatalf("Pos 有效零(dead)不应保留, got %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("期望恰好 2 个存活 key, got %v", got)
	}
}

func TestGCKeepsTrailStateOnTransientEmptyFields(t *testing.T) {
	am := &AccountMonitor{trailStates: map[string]TrailState{
		"账户A:BTC-USDT-SWAP:long":  {PeakPct: 88.8, Active: true, LastSize: 15},
		"账户A:BTC-USDT-SWAP:short": {PeakPct: 10.0, Active: false, LastSize: 2},
	}}
	// 交易所瞬时脏响应：long 的 UPL/UseMargin 为空、short 整条消失
	dirty := []utils.PositionInfo{
		{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "15", UnrealizedProfit: "", UseMargin: ""},
	}
	am.gcTrailStates("账户A", buildLiveKeys("账户A", dirty))

	if st, ok := am.trailStates["账户A:BTC-USDT-SWAP:long"]; !ok || !st.Active || st.PeakPct != 88.8 {
		t.Fatalf("瞬空字段不得导致已激活峰值被 GC, states=%v", am.trailStates)
	}
	if _, ok := am.trailStates["账户A:BTC-USDT-SWAP:short"]; ok {
		t.Fatalf("整条消失的仓位应照常 GC, states=%v", am.trailStates)
	}
}

