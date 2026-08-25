package monitor

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"common/utils"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
)

func TestBuildCloseEventFillsAuditFields(t *testing.T) {
	acc := trade.AccountConfig{Name: "账户A", Variant: "champion/S400_cap26_gate8"}
	pos := utils.PositionInfo{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "17",
		AvgPx: "63000.5", LastPx: "63220.1"}
	pct := decimal.NewFromFloat(44.5)
	pnl := decimal.NewFromFloat(3.88)

	// 已激活：peakPct 落字段
	ev := buildCloseEvent(acc, pos, eventlog.EvTrailingClose, pct, pnl, "移动止盈",
		TrailState{PeakPct: 61.2, Active: true, LastSize: 17})
	if ev.AvgPx != 63000.5 || ev.LastPx != 63220.1 {
		t.Fatalf("avgPx/lastPx 未解析: %+v", ev)
	}
	if ev.PeakPct != 61.2 {
		t.Fatalf("激活态应带 peakPct: %+v", ev)
	}
	if ev.Event != eventlog.EvTrailingClose || ev.Side != "long" || ev.Size != 17 ||
		ev.RoiPct != 44.5 || ev.Reason != "移动止盈" || ev.Account != "账户A" {
		t.Fatalf("基础字段错误: %+v", ev)
	}

	// 未激活（如从未到过激活线的兜底止损）：peakPct 省略
	ev2 := buildCloseEvent(acc, pos, eventlog.EvCatastropheStop, decimal.NewFromFloat(-405), pnl, "兜底止损",
		TrailState{PeakPct: 0, Active: false})
	if ev2.PeakPct != 0 {
		t.Fatalf("未激活不应带 peakPct: %+v", ev2)
	}
	if strings.Contains(eventlog.Marshal(ev2), "peakPct") {
		t.Fatalf("未激活序列化应省略 peakPct")
	}

	// 价格字段脏数据 → 0（omitempty 省略），不炸
	dirty := utils.PositionInfo{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "-3", AvgPx: "", LastPx: "abc"}
	ev3 := buildCloseEvent(acc, dirty, eventlog.EvFixedClose, pct, pnl, "固定止盈", TrailState{})
	if ev3.AvgPx != 0 || ev3.LastPx != 0 || ev3.Size != 3 {
		t.Fatalf("脏价格应为0且size取绝对值: %+v", ev3)
	}
}

