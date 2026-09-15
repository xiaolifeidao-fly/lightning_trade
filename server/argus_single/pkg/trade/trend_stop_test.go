package trade

import "testing"

// ResolveTrendStop 是趋势条件止损的判定纯函数：只有在【趋势顶着持仓】时才把
// 兜底止损从 S 收紧到 Y，其余时间一律沿用 S。
//
// 为什么不是全时段收紧（诊断 doc/module/收益诊断-2026-09-15 §三）：均值回归
// 策略的赢单本来就要先深亏——80 天样本里最低 ROI 到过 -338% 仍翻回 +77% 平仓，
// 7 天窗口上 S 从 400 收到任何更小值合计都下降。风控代码里那条
// ValidateRiskParams「catastrophe_stop_pct 必须 ≥250（松兜底护栏：紧止损杀死
// 均值回归 edge）」说的是同一件事。
//
// 判别式（231 笔平仓，同一文档 §四）：平仓时刻逆着持仓方向的 24h 动量，
// 赢单中位 -0.72%、兜底单中位 +4.09%；逆向动量 ≥3% 的占比 6% vs 62%。
// 10:1 的区分度，所以"只在逆向趋势里收紧"能把两者分开。

func TestTrendStopTightensShortInUptrend(t *testing.T) {
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	got, tightened := ResolveTrendStop("short", 4.09, true, p)
	if !tightened {
		t.Fatal("空仓遇 +4.09% 上行动量 ≥3%，应收紧")
	}
	if got != 250 {
		t.Fatalf("收紧后应为 250，得到 %v", got)
	}
}

func TestTrendStopTightensLongInDowntrend(t *testing.T) {
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if got, tightened := ResolveTrendStop("long", -3.2, true, p); !tightened || got != 250 {
		t.Fatalf("多仓遇 -3.2%% 下行动量应收紧到 250，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopKeepsBaseWithTrend(t *testing.T) {
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	// 顺势：空仓遇下跌、多仓遇上涨——趋势在帮它，不该动它的兜底线。
	if got, tightened := ResolveTrendStop("short", -6.0, true, p); tightened || got != 400 {
		t.Fatalf("空仓顺势不该收紧，得到 %v tightened=%v", got, tightened)
	}
	if got, tightened := ResolveTrendStop("long", 6.0, true, p); tightened || got != 400 {
		t.Fatalf("多仓顺势不该收紧，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopKeepsBaseBelowTrigger(t *testing.T) {
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if got, tightened := ResolveTrendStop("short", 2.99, true, p); tightened || got != 400 {
		t.Fatalf("2.99%% < 3%% 不该收紧，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopDisabledWhenTriggerUnset(t *testing.T) {
	// 与趋势闸同一部署安全语义：阈值 ≤0 = 未启用，缺省不生效。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 0, TightStopPct: 250}
	if got, tightened := ResolveTrendStop("short", 99, true, p); tightened || got != 400 {
		t.Fatalf("trigger=0 应视为未启用，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopDisabledWhenTightStopUnset(t *testing.T) {
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 0}
	if got, tightened := ResolveTrendStop("short", 99, true, p); tightened || got != 400 {
		t.Fatalf("tightStop=0 应视为未启用，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopFailsSafeWhenMomentumUnknown(t *testing.T) {
	// 重启后未回填 / 数据源断流 → 动量不可算。此时**绝不**收紧：
	// 拿不到证据就动别人的兜底线，等于凭空把赢单砍成亏损单。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if got, tightened := ResolveTrendStop("short", 9.9, false, p); tightened || got != 400 {
		t.Fatalf("momOK=false 必须沿用 S，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopRefusesToLoosen(t *testing.T) {
	// 配错成 Y ≥ S 时，本函数只能是"收紧或不动"，绝不能把安全网放宽。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 500}
	if got, tightened := ResolveTrendStop("short", 9.9, true, p); tightened || got != 400 {
		t.Fatalf("Y ≥ S 时不得放宽兜底线，得到 %v tightened=%v", got, tightened)
	}
	if got, _ := ResolveTrendStop("short", 9.9, true,
		TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 400}); got != 400 {
		t.Fatalf("Y == S 等于没收紧，得到 %v", got)
	}
}

func TestTrendStopKeepsBaseForUnknownSide(t *testing.T) {
	// 净仓方向读不到（空串）时同样按"没有证据"处理。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if got, tightened := ResolveTrendStop("", 9.9, true, p); tightened || got != 400 {
		t.Fatalf("方向未知必须沿用 S，得到 %v tightened=%v", got, tightened)
	}
}

func TestTrendStopAcceptsMixedCaseSide(t *testing.T) {
	// 持仓接口回的 posSide 大小写不保证（EvaluateTrendGate 用 EqualFold，这里对齐）。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if _, tightened := ResolveTrendStop("SHORT", 4.0, true, p); !tightened {
		t.Fatal("大写 SHORT 应与 short 同义")
	}
	if _, tightened := ResolveTrendStop("Long", -4.0, true, p); !tightened {
		t.Fatal("混合大小写 Long 应与 long 同义")
	}
}

func TestTrendStopReasonNamesNumbers(t *testing.T) {
	// 收紧会导致一次真实平仓，事件与 TG 告警必须能说清"为什么是 250 而不是 400"。
	p := TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	reason := TrendStopReason("short", 4.09, p)
	for _, want := range []string{"4.09", "3", "250", "400"} {
		if !contains(reason, want) {
			t.Errorf("reason 里应出现 %q，实际: %s", want, reason)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
