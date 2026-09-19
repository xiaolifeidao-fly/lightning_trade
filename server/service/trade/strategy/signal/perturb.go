package signal

import (
	"math/rand"
	"runtime"
	"sort"
	"sync"
)

// Perturb 回放扰动：把"一条路径"变成"一族路径"。零值 = 不扰动，Replay 与从前逐字节一致。
//
// 为什么需要它：2026-09-19 用生产配置回测留出段，A 得 +89.11/0 兜底，实盘同期 +0.13/1 兜底。
// 逐笔对比找到分叉——09-17 03:23 的移动止盈实盘在 tick 上触发，回测按 1m bar 晚了 3 分钟，
// 这 3 分钟里的三次开空被并成了加仓，之后 37 小时方向相反。125x + 每几分钟一次信号 +
// 开/加/减由持仓状态决定，路径对出场时刻是混沌敏感的；单路径的 ±50 与这种分叉同量级。
// 两种扰动各对应一种实盘与回测之间真实存在的差异：
//
//   - SignalDropPct：实盘会因 5 秒冷却、下单失败、网络抖动漏掉个别触发；回放里按概率丢。
//   - ExitLateProb：实盘 5 秒 tick 监控 vs 回测 60 秒 bar 判定；出场成立时按概率晚一根 bar 成交，
//     **这一根里的信号照常成交**（加仓/减仓），再按这根的收盘价平掉整个仓——正是 09-17 的机制。
//
// 起点偏移不在这里：它只扰动窗口开头，两条路径一旦同时空仓就重新同步了；这两种扰动作用到窗口末尾。
type Perturb struct {
	Seed          int64
	SignalDropPct float64 // [0,1)：每条信号被独立丢弃的概率
	ExitLateProb  float64 // [0,1]：每次出场判定成立时，晚一根 bar 成交的概率
}

// On 是否有任何扰动生效。
func (p Perturb) On() bool { return p.SignalDropPct > 0 || p.ExitLateProb > 0 }

// rngs 两条独立随机流：丢信号与出场延迟互不影响，改一个概率不会重排另一个的抽样。
func (p Perturb) rngs() (drop, exit *rand.Rand) {
	if p.SignalDropPct > 0 {
		drop = rand.New(rand.NewSource(p.Seed))
	}
	if p.ExitLateProb > 0 {
		exit = rand.New(rand.NewSource(p.Seed*7919 + 1))
	}
	return
}

// EnsembleRun 一条扰动路径的摘要。
type EnsembleRun struct {
	Seed         int64
	Net          float64
	Catastrophes int
	MaxDrawdown  float64
	Dropped      int // 被扰动丢掉的信号
	ExitsDelayed int // 被扰动延迟的出场
}

// EnsembleStats 一组参数在 N 条扰动路径上的分布。
type EnsembleStats struct {
	Runs      []EnsembleRun
	NetMedian float64
	NetP10    float64
	NetMin    float64
	NetMax    float64
	CatMedian float64
	CatMax    int
}

// Ensemble 同一份输入、同一组参数，跑 seed=1..n 的 n 条扰动路径（并行，结果按 seed 排）。
// pert.Seed 被逐条覆盖；所有参数组用同一组 seed，配对比较才成立。
// Replay 是纯函数（Input 只读、每次新建 Engine），并行跑是安全的；
// 一条 80 天路径约 8 秒，25 格 × 8 seed 串行要半小时，并行才用得起。
func Ensemble(in Input, n int, pert Perturb) (EnsembleStats, error) {
	st := EnsembleStats{Runs: make([]EnsembleRun, n)}
	errs := make([]error, n)
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			seed := int64(i + 1)
			pin := in
			pp := pert
			pp.Seed = seed
			pin.Perturb = pp
			res, err := Replay(pin)
			if err != nil {
				errs[i] = err
				return
			}
			cats := 0
			for _, ep := range res.Episodes {
				if ep.Reason == ExitCatastrophe {
					cats++
				}
			}
			st.Runs[i] = EnsembleRun{Seed: seed, Net: res.Net, Catastrophes: cats,
				MaxDrawdown: res.MaxDrawdown, Dropped: res.PerturbDropped, ExitsDelayed: res.ExitsDelayed}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return EnsembleStats{}, err
		}
	}
	st.summarize()
	return st, nil
}

func (st *EnsembleStats) summarize() {
	if len(st.Runs) == 0 {
		return
	}
	nets := make([]float64, len(st.Runs))
	cats := make([]float64, len(st.Runs))
	for i, r := range st.Runs {
		nets[i] = r.Net
		cats[i] = float64(r.Catastrophes)
		if r.Catastrophes > st.CatMax {
			st.CatMax = r.Catastrophes
		}
	}
	st.NetMedian = Quantile(nets, 0.5)
	st.NetP10 = Quantile(nets, 0.1)
	st.NetMin = Quantile(nets, 0)
	st.NetMax = Quantile(nets, 1)
	st.CatMedian = Quantile(cats, 0.5)
}

// Quantile 线性插值分位数（q∈[0,1]）。不改动入参。
func Quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if q <= 0 {
		return s[0]
	}
	if q >= 1 {
		return s[len(s)-1]
	}
	pos := q * float64(len(s)-1)
	lo := int(pos)
	frac := pos - float64(lo)
	if lo+1 >= len(s) {
		return s[lo]
	}
	return s[lo]*(1-frac) + s[lo+1]*frac
}

// Paired 配对比较：同一 seed 下 b − a。返回净盈亏差的中位数、b 优于 a 的路径数、兜底次数差的逐条列表。
// 配对而不是比两个分布的中位数：两组吃的是同一组扰动，差值把共同的路径噪声抵掉了。
type PairedDelta struct {
	NetDeltaMedian float64
	NetDeltaMin    float64
	Better         int // b 净盈亏 > a 的路径数
	N              int
	CatDelta       []int // 逐 seed：b 兜底 − a 兜底
	CatNotWorse    int   // b 兜底 ≤ a 的路径数
}

func Paired(a, b EnsembleStats) PairedDelta {
	n := len(a.Runs)
	if len(b.Runs) < n {
		n = len(b.Runs)
	}
	var d PairedDelta
	d.N = n
	deltas := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		x := b.Runs[i].Net - a.Runs[i].Net
		deltas = append(deltas, x)
		if x > 0 {
			d.Better++
		}
		cd := b.Runs[i].Catastrophes - a.Runs[i].Catastrophes
		d.CatDelta = append(d.CatDelta, cd)
		if cd <= 0 {
			d.CatNotWorse++
		}
	}
	if n > 0 {
		d.NetDeltaMedian = Quantile(deltas, 0.5)
		d.NetDeltaMin = Quantile(deltas, 0)
	}
	return d
}
