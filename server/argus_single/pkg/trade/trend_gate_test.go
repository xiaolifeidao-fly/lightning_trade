package trade

import (
	"testing"

	"common/middleware/vipper"
)

// EvaluateTrendGate 是趋势闸（8/21 事故复盘）的判定纯函数：N 小时动量超阈时
// 禁止逆势方向的全新开仓与同向加仓（减仓在 manager 分流层就不会走到这里）。
// 回测依据 docs/backtest_aug1821_scan3.py：24h/5% 三窗口全部 ≥ baseline。

func TestTrendGateBlocksShortInUptrend(t *testing.T) {
	d := EvaluateTrendGate("short", 5.3, true, 5.0)
	if !d.Block {
		t.Fatal("+5.3% ≥ 5% 应禁开/加空")
	}
	if d.Reason == "" {
		t.Error("拦截必须给出 reason（进事件与 TG）")
	}
}

func TestTrendGateBlocksLongInDowntrend(t *testing.T) {
	if d := EvaluateTrendGate("long", -5.3, true, 5.0); !d.Block {
		t.Fatal("-5.3% ≤ -5% 应禁开/加多")
	}
}

func TestTrendGateAllowsWithTrend(t *testing.T) {
	if d := EvaluateTrendGate("long", 5.3, true, 5.0); d.Block {
		t.Fatal("顺势方向不拦")
	}
	if d := EvaluateTrendGate("short", -5.3, true, 5.0); d.Block {
		t.Fatal("顺势方向不拦")
	}
}

func TestTrendGateAllowsBelowThreshold(t *testing.T) {
	if d := EvaluateTrendGate("short", 4.9, true, 5.0); d.Block {
		t.Fatal("4.9% < 5% 不拦")
	}
	if d := EvaluateTrendGate("long", -4.9, true, 5.0); d.Block {
		t.Fatal("-4.9% > -5% 不拦")
	}
}

func TestTrendGateAllowsWhenMomentumUnknown(t *testing.T) {
	// 历史不足（重启后未满窗且未回填）→ 放行，与回测 warmup 缺失语义一致
	if d := EvaluateTrendGate("short", 99, false, 5.0); d.Block {
		t.Fatal("动量不可知时放行")
	}
}

func TestTrendGateDisabledByNonPositiveThreshold(t *testing.T) {
	if d := EvaluateTrendGate("short", 99, true, 0); d.Block {
		t.Fatal("threshold=0 表示未启用")
	}
	if d := EvaluateTrendGate("short", 99, true, -1); d.Block {
		t.Fatal("threshold<0 表示未启用")
	}
}

func TestTrendGateCaseInsensitiveSide(t *testing.T) {
	if d := EvaluateTrendGate("SHORT", 5.3, true, 5.0); !d.Block {
		t.Fatal("posSide 比较须大小写不敏感（与 manager 其余判定一致）")
	}
}

func TestResolveTrendGateThreshold(t *testing.T) {
	// 账户级覆盖 > 全局 > 默认 0（=关闭，部署安全：不配置绝不启用）
	vipper.Set("trade.trend_gate.threshold_pct", "5.0")
	vipper.Set("trade.account7.trend_gate_threshold_pct", "3.0")
	defer func() {
		vipper.Set("trade.trend_gate.threshold_pct", "") // pickFloat 视空串为未设置
		vipper.Set("trade.account7.trend_gate_threshold_pct", "")
	}()
	if v := resolveTrendGateThreshold(AccountConfig{Index: 7}); v != 3.0 {
		t.Errorf("账户级覆盖应生效, got %v", v)
	}
	if v := resolveTrendGateThreshold(AccountConfig{Index: 8}); v != 5.0 {
		t.Errorf("全局值应生效, got %v", v)
	}
}

func TestBuildTrendSkipEvent(t *testing.T) {
	dec := EvaluateTrendGate("short", 5.3, true, 5.0)
	q := NewSignalQuote(74500, 74450)
	q.TrendMomPct, q.TrendOK = 5.3, true
	ev := buildTrendSkipEvent(AccountConfig{Name: "账户A", Variant: "champion"}, "BTCUSDT",
		"short", NetPosition{Side: "short", Size: 3}, 1, dec, q)
	if ev.Event != "trend_skip" || ev.Account != "账户A" || ev.Side != "short" ||
		ev.NetSide != "short" || ev.Size != 3 || ev.OrderSize != 1 {
		t.Errorf("事件骨架不对: %+v", ev)
	}
	if ev.TrendMomPct != 5.3 || ev.Reason == "" {
		t.Errorf("动量与理由必须落盘: %+v", ev)
	}
	if ev.SigLast != 74500 || ev.GapBp == 0 {
		t.Errorf("应带 P7 报价字段（被拦信号的 gap 分布同样是研究数据）: %+v", ev)
	}
}
