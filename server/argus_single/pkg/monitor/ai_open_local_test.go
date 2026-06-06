package monitor

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// 这些测试只覆盖「本地硬风控」纯逻辑，不发起任何 AI/网络请求。
// 运行：go test ./pkg/monitor/ -run TestAIOpen -v

func testOpenDecider() *Tu2doOpenDecider {
	return &Tu2doOpenDecider{
		minLiqDistancePercent: 30,
		minLiqDistanceUSD:     10000,
		maxBalancePercent:     50,
		maxTotalContracts:     100,
		cooldownMinutes:       30,
		liqSafetyFactor:       1.0, // 测试用 1.0 便于核对数值
		minOrderContracts:     1,
		maxOrderContracts:     100, // 风控测试用宽区间，避免夹取干扰 liq/cap/margin 场景
	}
}

func TestAIOpenClampOrderContracts(t *testing.T) {
	d := &Tu2doOpenDecider{minOrderContracts: 1, maxOrderContracts: 3}
	cases := []struct {
		in   string
		want string
	}{
		{"5", "3"},   // 超上限 → 3
		{"2", "2"},   // 区间内
		{"0.5", "1"}, // 向下取整=0，低于下限 → 1
		{"1", "1"},
		{"10", "3"},
	}
	for _, c := range cases {
		dec := &AIOpenDecision{}
		got := d.clampOrderContracts(decimal.RequireFromString(c.in), dec)
		if got.String() != c.want {
			t.Errorf("clampOrderContracts(%s)=%s, want %s", c.in, got.String(), c.want)
		}
		if dec.SuggestedSize != c.want+"张" {
			t.Errorf("SuggestedSize=%q, want %q", dec.SuggestedSize, c.want+"张")
		}
	}
}

func approxEqual(a decimal.Decimal, b float64) bool {
	diff := a.Sub(decimal.NewFromFloat(b)).Abs()
	return diff.LessThan(decimal.NewFromFloat(1))
}

func TestAIOpenParseContracts(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"2张", 2},
		{"5", 5},
		{" 3 张 ", 3},
		{"", 0},
		{"与当前仓位相同", 0},
		{"全部", 0},
	}
	for _, c := range cases {
		got := parseContracts(c.in)
		if !got.Equal(decimal.NewFromFloat(c.want)) {
			t.Errorf("parseContracts(%q)=%s, want %v", c.in, got.String(), c.want)
		}
	}
}

func TestAIOpenEstimatePostAddLiqDistance(t *testing.T) {
	// 现有多仓 10 张, 最新价 100000, 爆仓价 70000 → buffer=30000
	last, liq, size, side := "100000", "70000", "10", "long"

	// 同向加仓 5 张：净仓 15，newBuffer=30000*10/15=20000 → 20%
	distUSD, distPct, netQty, reduces, flipped, ok := estimatePostAddLiqDistance(last, liq, size, side, decimal.NewFromInt(5), true, 1.0)
	if !ok || reduces || flipped {
		t.Fatalf("同向加仓: ok=%v reduces=%v flipped=%v", ok, reduces, flipped)
	}
	if !netQty.Equal(decimal.NewFromInt(15)) || !approxEqual(distUSD, 20000) || !approxEqual(distPct, 20) {
		t.Errorf("同向加仓: netQty=%s distUSD=%s distPct=%s", netQty, distUSD, distPct)
	}

	// 反向减仓 3 张：净仓 7（未反转）→ reduces=true
	_, _, netQty, reduces, flipped, ok = estimatePostAddLiqDistance(last, liq, size, side, decimal.NewFromInt(3), false, 1.0)
	if !ok || !reduces || flipped || !netQty.Equal(decimal.NewFromInt(7)) {
		t.Errorf("反向减仓: ok=%v reduces=%v flipped=%v netQty=%s", ok, reduces, flipped, netQty)
	}

	// 反向反超 25 张：净仓变 short 15 → flipped=true, reduces=false
	_, _, netQty, reduces, flipped, ok = estimatePostAddLiqDistance(last, liq, size, side, decimal.NewFromInt(25), false, 1.0)
	if !ok || reduces || !flipped || !netQty.Equal(decimal.NewFromInt(15)) {
		t.Errorf("反向反超: ok=%v reduces=%v flipped=%v netQty=%s", ok, reduces, flipped, netQty)
	}

	// 反向 10 张：净仓归零 → 极大安全距离
	distUSD, distPct, netQty, reduces, _, ok = estimatePostAddLiqDistance(last, liq, size, side, decimal.NewFromInt(10), false, 1.0)
	if !ok || !reduces || !netQty.IsZero() || !approxEqual(distPct, 100) {
		t.Errorf("净仓归零: ok=%v reduces=%v netQty=%s distPct=%s", ok, reduces, netQty, distPct)
	}

	// 数据缺失
	if _, _, _, _, _, ok := estimatePostAddLiqDistance("", liq, size, side, decimal.NewFromInt(5), true, 1.0); ok {
		t.Error("缺少最新价应返回 ok=false")
	}
}

func TestAIOpenEstimateNewPositionLiqDistance(t *testing.T) {
	last := decimal.NewFromInt(100000)
	bal := decimal.NewFromInt(1000)

	// 1 张：qtyBTC=0.001, distUSD=1000/0.001=1,000,000 → 极远，安全
	distUSD, distPct, ok := estimateNewPositionLiqDistance(last, bal, decimal.NewFromInt(1), 1.0)
	if !ok || !approxEqual(distUSD, 1000000) {
		t.Errorf("新仓1张: ok=%v distUSD=%s distPct=%s", ok, distUSD, distPct)
	}

	// 200 张：qtyBTC=0.2, distUSD=5000 → 5%，不安全
	distUSD, distPct, ok = estimateNewPositionLiqDistance(last, bal, decimal.NewFromInt(200), 1.0)
	if !ok || !approxEqual(distUSD, 5000) || !approxEqual(distPct, 5) {
		t.Errorf("新仓200张: distUSD=%s distPct=%s", distUSD, distPct)
	}
}

func TestAIOpenLocalRiskGuard_Add(t *testing.T) {
	d := testOpenDecider()

	mkSnap := func(size, last, liq, side, lever, avail string) PositionSnapshot {
		s := PositionSnapshot{
			HasPosition:  true,
			PositionSize: size,
			LastPrice:    last,
			LiqPrice:     liq,
			PositionSide: side,
			AvailBal:     avail,
			TotalBal:     avail,
		}
		s.CurrentPosition.Leverage = lever
		return s
	}

	// 加仓后爆仓距离不足 → no_trade
	dec := &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "5张"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "70000", "long", "125", "100000"), dec)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("爆仓距离不足应拦截: passed=%v action=%s reason=%s", dec.RiskPassed, dec.FinalAction, dec.RiskBlockReason)
	}

	// 距离充足 → 放行
	dec = &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "1张"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "125", "100000"), dec)
	if !dec.RiskPassed {
		t.Errorf("距离充足应放行: reason=%s distPct=%s", dec.RiskBlockReason, dec.LocalLiqDistancePercent)
	}

	// 反向减仓 → 直接放行且标记 IsReduce
	dec = &AIOpenDecision{FinalAction: "open_short", SuggestedSize: "3张"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "125", "100000"), dec)
	if !dec.RiskPassed || !dec.IsReduce {
		t.Errorf("反向减仓应放行且 IsReduce: passed=%v isReduce=%v", dec.RiskPassed, dec.IsReduce)
	}

	// 张数解析不出 → no_trade
	dec = &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "与当前仓位相同"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "125", "100000"), dec)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("无效张数应拦截: passed=%v action=%s", dec.RiskPassed, dec.FinalAction)
	}

	// 余额预算超限（杠杆=1，所需保证金巨大）→ no_trade
	dec = &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "20张"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "1", "1000"), dec)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("余额预算超限应拦截: passed=%v reason=%s", dec.RiskPassed, dec.RiskBlockReason)
	}

	// 累计仓位上限：加 95 张 → 净仓 105 > 100 → no_trade
	dec = &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "95张"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "125", "100000"), dec)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("累计上限应拦截: passed=%v reason=%s", dec.RiskPassed, dec.RiskBlockReason)
	}

	// no_trade/wait 决策无需校验，直接 RiskPassed
	dec = &AIOpenDecision{FinalAction: "wait"}
	d.applyLocalRiskGuard(mkSnap("10", "100000", "50000", "long", "125", "100000"), dec)
	if !dec.RiskPassed {
		t.Errorf("wait 应视为通过")
	}
}

func TestAIOpenLocalRiskGuard_NewPosition(t *testing.T) {
	d := testOpenDecider()
	mkSnap := func(last, bal string) PositionSnapshot {
		s := PositionSnapshot{
			HasPosition:  false,
			PositionSize: "", // 空仓
			LastPrice:    last,
			AvailBal:     bal,
			TotalBal:     bal,
		}
		s.CurrentPosition.Leverage = "125"
		return s
	}

	// 空仓开 1 张：余额 1000 → 爆仓极远 → 放行
	dec := &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "1张"}
	d.applyLocalRiskGuard(mkSnap("100000", "1000"), dec)
	if !dec.RiskPassed {
		t.Errorf("空仓小仓应放行: reason=%s distPct=%s", dec.RiskBlockReason, dec.LocalLiqDistancePercent)
	}

	// 空仓开 90 张：distUSD≈11111(≥10000) 但 distPct≈11%(<30) → 爆仓距离不足拦截
	dec = &AIOpenDecision{FinalAction: "open_long", SuggestedSize: "90张"}
	d.applyLocalRiskGuard(mkSnap("100000", "1000"), dec)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("空仓大仓应拦截: passed=%v reason=%s", dec.RiskPassed, dec.RiskBlockReason)
	}
}

func TestAIOpenParseNextCheckInterval(t *testing.T) {
	// 夹在 [5m, 15m]
	cases := []struct {
		in     string
		want   time.Duration
		wantOk bool
	}{
		{"15m", 15 * time.Minute, true},
		{"10m", 10 * time.Minute, true},
		{"1h", 15 * time.Minute, true},  // 超上限 → 夹到 15m
		{"4h", 15 * time.Minute, true},  // 夹到 15m
		{"1d", 15 * time.Minute, true},  // 夹到 15m
		{"1-2h", 15 * time.Minute, true}, // 区间取较短端 1h，仍超上限 → 15m
		{"3m", 5 * time.Minute, true},   // 低于 5m 下限 → 夹到 5m
		{"8m", 8 * time.Minute, true},
		{"", 0, false},
		{"soon", 0, false},
	}
	for _, c := range cases {
		got, ok := parseNextCheckInterval(c.in, 5*time.Minute, 15*time.Minute)
		if ok != c.wantOk {
			t.Errorf("parseNextCheckInterval(%q) ok=%v, want %v", c.in, ok, c.wantOk)
			continue
		}
		if c.wantOk && got != c.want {
			t.Errorf("parseNextCheckInterval(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestAIOpenMinPositiveDuration(t *testing.T) {
	cases := []struct {
		a, b, want time.Duration
	}{
		{0, 0, 0},
		{0, 5 * time.Minute, 5 * time.Minute},
		{10 * time.Minute, 0, 10 * time.Minute},
		{10 * time.Minute, 5 * time.Minute, 5 * time.Minute},
		{5 * time.Minute, 10 * time.Minute, 5 * time.Minute},
	}
	for _, c := range cases {
		if got := minPositiveDuration(c.a, c.b); got != c.want {
			t.Errorf("minPositiveDuration(%v,%v)=%v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAIOpenCooldownGuard(t *testing.T) {
	d := testOpenDecider()
	now := time.Now()

	// 10 分钟前有一次已放行的同向加仓 → 未满 30 分钟冷却 → 拦截
	history := []AIOpenLastDecisionRecord{
		{SavedAt: now.Add(-10 * time.Minute).Format(time.RFC3339), FinalAction: "open_long", RiskPassed: true},
	}
	dec := &AIOpenDecision{FinalAction: "open_long", RiskPassed: true}
	d.applyCooldownGuard(dec, history, now)
	if dec.RiskPassed || dec.FinalAction != "no_trade" {
		t.Errorf("冷却内应拦截: passed=%v reason=%s", dec.RiskPassed, dec.RiskBlockReason)
	}

	// 40 分钟前 → 已过冷却 → 放行
	history = []AIOpenLastDecisionRecord{
		{SavedAt: now.Add(-40 * time.Minute).Format(time.RFC3339), FinalAction: "open_long", RiskPassed: true},
	}
	dec = &AIOpenDecision{FinalAction: "open_long", RiskPassed: true}
	d.applyCooldownGuard(dec, history, now)
	if !dec.RiskPassed {
		t.Errorf("已过冷却应放行: reason=%s", dec.RiskBlockReason)
	}

	// 反向减仓豁免冷却
	history = []AIOpenLastDecisionRecord{
		{SavedAt: now.Add(-5 * time.Minute).Format(time.RFC3339), FinalAction: "open_short", RiskPassed: true},
	}
	dec = &AIOpenDecision{FinalAction: "open_short", RiskPassed: true, IsReduce: true}
	d.applyCooldownGuard(dec, history, now)
	if !dec.RiskPassed {
		t.Errorf("反向减仓应免冷却: reason=%s", dec.RiskBlockReason)
	}
}
