package signal

import (
	"math"
	"testing"
	"time"
)

// 零值 Perturb 必须与不扰动逐字节一致——这是所有历史结论不被本次改动污染的前提。
func TestPerturbZeroValueIsIdentity(t *testing.T) {
	var sigs []Signal
	for i := 1; i <= 20; i++ {
		sigs = append(sigs, Signal{Ts: ts(i).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1})
	}
	a, err := Replay(Input{Params: baseParams(), Signals: sigs, Bars: flatBars(40, 60000)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Replay(Input{Params: baseParams(), Signals: sigs, Bars: flatBars(40, 60000), Perturb: Perturb{Seed: 7}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Net != b.Net || a.SignalReplayed != b.SignalReplayed || len(a.Episodes) != len(b.Episodes) ||
		b.PerturbDropped != 0 || b.ExitsDelayed != 0 {
		t.Fatalf("零值扰动应与未扰动一致: a=%+v b=%+v", a.Net, b.Net)
	}
}

// 丢信号：同 seed 可复现；概率 1 全丢且计数对得上（Total = Replayed + PerturbDropped）。
func TestSignalDropoutIsSeededAndAccounted(t *testing.T) {
	var sigs []Signal
	for i := 1; i <= 30; i++ {
		sigs = append(sigs, Signal{Ts: ts(i).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1})
	}
	in := Input{Params: baseParams(), Signals: sigs, Bars: flatBars(40, 60000)}

	in.Perturb = Perturb{Seed: 1, SignalDropPct: 1}
	all, _ := Replay(in)
	if all.PerturbDropped != 30 || all.SignalReplayed != 0 {
		t.Fatalf("概率 1 应全丢: dropped=%d replayed=%d", all.PerturbDropped, all.SignalReplayed)
	}

	in.Perturb = Perturb{Seed: 3, SignalDropPct: 0.4}
	x, _ := Replay(in)
	y, _ := Replay(in)
	if x.PerturbDropped != y.PerturbDropped || x.Net != y.Net {
		t.Fatalf("同 seed 必须复现: %d/%d %v/%v", x.PerturbDropped, y.PerturbDropped, x.Net, y.Net)
	}
	if x.PerturbDropped == 0 || x.PerturbDropped == 30 {
		t.Fatalf("概率 0.4 下 30 条信号不应全留或全丢: dropped=%d", x.PerturbDropped)
	}
	if x.SignalTotal != x.SignalReplayed+x.SignalDropped+x.PerturbDropped {
		t.Fatalf("对账: total=%d replayed=%d dropped=%d perturb=%d", x.SignalTotal, x.SignalReplayed, x.SignalDropped, x.PerturbDropped)
	}
}

// 出场晚一根：判定成立的那根不平；下一根里的信号先成交（这里是加仓 1→2），
// 再按下一根收盘平掉整个仓。这就是 09-17 03:23 实盘与回测分叉的机制。
func TestExitLateLetsNextBarSignalsFillBeforeClosing(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400 // 3.2% ⇒ 60000 多单在 58080 触发
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 58000, High: 58000, Low: 58000, Close: 58000}, // 穿越兜底
		{Ts: ts(3), Open: 57900, High: 57900, Low: 57900, Close: 57900},
		{Ts: ts(4), Open: 57900, High: 57900, Low: 57900, Close: 57900},
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(2).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}, // 落在 ts(3) 这根成交
	}
	// 对照：不扰动，ts(2) 那根就按兜底平 1 张
	base, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Episodes) < 1 || base.Episodes[0].Reason != ExitCatastrophe || base.Episodes[0].Contracts != 1 || !base.Episodes[0].ClosedAt.Equal(ts(2)) {
		t.Fatalf("对照应在 ts(2) 兜底平 1 张: %+v", base.Episodes)
	}

	late, err := Replay(Input{Params: p, Signals: sigs, Bars: bars, Perturb: Perturb{Seed: 1, ExitLateProb: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if late.ExitsDelayed != 1 {
		t.Fatalf("应记 1 次出场延迟, got %d", late.ExitsDelayed)
	}
	ep := late.Episodes[0]
	if ep.Reason != ExitCatastrophe || ep.Contracts != 2 || !ep.ClosedAt.Equal(ts(3)) || ep.ClosePx != 57900 {
		t.Fatalf("延迟后应在 ts(3) 按收盘 57900 平掉 2 张（下一根的加仓先成交）: %+v", ep)
	}
	if late.Net >= base.Net {
		t.Fatalf("多扛一根且多 1 张，亏损应更大: late=%.4f base=%.4f", late.Net, base.Net)
	}
}

func TestQuantileAndPaired(t *testing.T) {
	xs := []float64{5, 1, 3, 2, 4}
	if q := Quantile(xs, 0.5); q != 3 {
		t.Errorf("中位数应为 3, got %v", q)
	}
	if q := Quantile(xs, 0); q != 1 {
		t.Errorf("q=0 应为最小 1, got %v", q)
	}
	if q := Quantile(xs, 1); q != 5 {
		t.Errorf("q=1 应为最大 5, got %v", q)
	}
	if q := Quantile(xs, 0.25); math.Abs(q-2) > 1e-9 {
		t.Errorf("q=0.25 线性插值应为 2, got %v", q)
	}
	if xs[0] != 5 {
		t.Error("Quantile 不得改动入参顺序")
	}
	a := EnsembleStats{Runs: []EnsembleRun{{Net: 10, Catastrophes: 2}, {Net: 20, Catastrophes: 1}, {Net: 30, Catastrophes: 3}}}
	b := EnsembleStats{Runs: []EnsembleRun{{Net: 15, Catastrophes: 1}, {Net: 18, Catastrophes: 1}, {Net: 40, Catastrophes: 4}}}
	d := Paired(a, b)
	if d.N != 3 || d.Better != 2 || d.NetDeltaMedian != 5 || d.NetDeltaMin != -2 || d.CatNotWorse != 2 {
		t.Errorf("配对差错误: %+v", d)
	}
}

// Ensemble 并行跑，结果必须按 seed 有序且与串行单跑逐条一致——配对比较靠下标对齐。
func TestEnsembleIsOrderedAndReproducible(t *testing.T) {
	var sigs []Signal
	for i := 1; i <= 30; i++ {
		sigs = append(sigs, Signal{Ts: ts(i).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1})
	}
	in := Input{Params: baseParams(), Signals: sigs, Bars: flatBars(40, 60000)}
	pert := Perturb{SignalDropPct: 0.3}
	st, err := Ensemble(in, 6, pert)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Runs) != 6 {
		t.Fatalf("应有 6 条, got %d", len(st.Runs))
	}
	for i, r := range st.Runs {
		if r.Seed != int64(i+1) {
			t.Fatalf("第 %d 条 seed=%d，应按 seed 有序", i, r.Seed)
		}
		pert.Seed = r.Seed
		in.Perturb = pert
		single, _ := Replay(in)
		if single.PerturbDropped != r.Dropped || single.Net != r.Net {
			t.Fatalf("seed=%d 并行结果与单跑不一致: %d/%d %v/%v", r.Seed, r.Dropped, single.PerturbDropped, r.Net, single.Net)
		}
	}
}
