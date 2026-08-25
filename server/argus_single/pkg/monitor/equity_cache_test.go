package monitor

import (
	"strings"
	"testing"
	"time"

	"common/utils"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
)

func TestSumAccountUpl(t *testing.T) {
	tests := []struct {
		name         string
		pos          []utils.PositionInfo
		want         float64
		wantComplete bool
	}{
		{"空仓=0且完整", nil, 0, true},
		{"多头直取", []utils.PositionInfo{
			{PosSide: "long", Pos: "15", AvgPx: "63000", LastPx: "63200", UnrealizedProfit: "-2.5"}}, 2.5, true},
		{"空头符号归一", []utils.PositionInfo{
			{PosSide: "short", Pos: "-6", AvgPx: "63459.2", LastPx: "63200", UnrealizedProfit: "-0.5268"}}, 0.5268, true},
		{"live仓UPL瞬空→不完整", []utils.PositionInfo{
			{PosSide: "long", Pos: "15", AvgPx: "63000", LastPx: "62500", UnrealizedProfit: ""}}, 0, false},
		{"Pos不可解析行→不完整", []utils.PositionInfo{
			{PosSide: "long", Pos: "abc", UnrealizedProfit: "1.0", AvgPx: "63000", LastPx: "63100"}}, 0, false},
		{"dead行跳过", []utils.PositionInfo{
			{PosSide: "long", Pos: "0", UnrealizedProfit: ""},
			{PosSide: "short", Pos: "-6", AvgPx: "63459.2", LastPx: "63200", UnrealizedProfit: "-0.5268"}}, 0.5268, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, complete := sumAccountUpl(tt.pos)
			if complete != tt.wantComplete {
				t.Fatalf("complete = %v, want %v", complete, tt.wantComplete)
			}
			if tt.wantComplete {
				if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
					t.Fatalf("sumAccountUpl = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestBuildBalanceEvent(t *testing.T) {
	acc := trade.AccountConfig{Name: "账户A", Variant: "champion/S400_cap26_gate8"}
	e := buildBalanceEvent(acc, 420.43, -18.93, true, 15)
	if e.Balance != 420.43 || e.Upl != -18.93 || e.Equity != 420.43-18.93 || e.Size != 15 {
		t.Fatalf("有缓存: %+v", e)
	}
	if e.Event != eventlog.EvBalance || e.Account != "账户A" {
		t.Fatalf("基础字段: %+v", e)
	}
	e2 := buildBalanceEvent(acc, 405.05, 0, false, 0)
	line := eventlog.Marshal(e2)
	if strings.Contains(line, "equity") || strings.Contains(line, "upl") {
		t.Fatalf("无缓存应省略 equity/upl: %s", line)
	}
	e3 := buildBalanceEvent(acc, 405.05, 0, true, 0)
	if e3.Equity != 405.05 {
		t.Fatalf("有缓存无仓: equity 应=balance: %+v", e3)
	}
	if strings.Contains(eventlog.Marshal(e3), "\"size\"") {
		t.Fatalf("空仓 size 应省略")
	}
	// codex round-4: equity=0（balance 恰被浮亏抵平）是已知的极端样本, 不是未知
	// ——equity 带 omitempty 会省略 0 值, 已知性由 equityKnown 显式携带
	e4 := buildBalanceEvent(acc, 100, -100, true, 3)
	if !e4.EquityKnown || e4.Equity != 0 || e4.Upl != -100 {
		t.Fatalf("零权益应标记 equityKnown: %+v", e4)
	}
	if e2.EquityKnown {
		t.Fatalf("无缓存不得标记 equityKnown: %+v", e2)
	}
}

// UPL 缓存新鲜度（codex review：持仓接口连续异常时, 数小时前的 UPL 会被当成
// 当前值持续写进 equity——缓存必须带采样时刻, 陈旧即诚实省略）。
// 阈值语义 = 最大允许年龄 3×轮询间隔(5s 轮询即 15s 上界): 给瞬断/抖动留冗余,
// 把陈旧上界钉死在秒级; 按年龄不按失败次数描述(边界相位下两者不一一对应)。
func TestUplFresh(t *testing.T) {
	t0 := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	s := uplSample{Value: -18.93, ObservedAt: t0}
	interval := 5 * time.Second
	tests := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"刚采样", 0, true},
		{"正常相位(一个周期内)", 4 * time.Second, true},
		{"单次瞬断后保留的旧值", 9 * time.Second, true},
		{"边界: 恰好3×interval", 15 * time.Second, true},
		{"超过最大年龄", 16 * time.Second, false},
		{"数小时持续故障", 3 * time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uplFresh(s, t0.Add(tt.age), interval); got != tt.want {
				t.Fatalf("uplFresh(age=%v, interval=%v) = %v, want %v", tt.age, interval, got, tt.want)
			}
		})
	}
}

func TestUplCacheFreshnessFlow(t *testing.T) {
	am := &AccountMonitor{uplByAccount: make(map[string]uplSample), posQueryInterval: 5 * time.Second}
	t0 := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	am.setUpl("账户A", -18.93, t0)

	s, ok := am.getUpl("账户A")
	if !ok || s.Value != -18.93 || !s.ObservedAt.Equal(t0) {
		t.Fatalf("样本应含值与采样时刻, got %+v ok=%v", s, ok)
	}
	if !uplFresh(s, t0.Add(14*time.Second), am.posQueryInterval) {
		t.Fatalf("14s(≤3×5s) 应新鲜: 值仍可用于 equity")
	}
	if uplFresh(s, t0.Add(2*time.Hour), am.posQueryInterval) {
		t.Fatalf("2h 旧值不得冒充实时 equity(持仓接口持续异常场景)")
	}
	if _, ok := am.getUpl("账户B"); ok {
		t.Fatalf("未采样账户应 ok=false")
	}
}

func TestAccountNetSizeFromSnapshots(t *testing.T) {
	am := &AccountMonitor{snapshots: map[string]posSnapshot{
		"账户A:BTC-USDT-SWAP:long":  {InstId: "BTC-USDT-SWAP", PosSide: "long", Size: 15},
		"账户B:BTC-USDT-SWAP:short": {InstId: "BTC-USDT-SWAP", PosSide: "short", Size: 6},
	}}
	if n := am.accountNetSize("账户A"); n != 15 {
		t.Fatalf("账户A size=15, got %d", n)
	}
	if n := am.accountNetSize("账户B"); n != 6 {
		t.Fatalf("账户B size=6, got %d", n)
	}
	if n := am.accountNetSize("账户C"); n != 0 {
		t.Fatalf("无仓账户=0, got %d", n)
	}
}

