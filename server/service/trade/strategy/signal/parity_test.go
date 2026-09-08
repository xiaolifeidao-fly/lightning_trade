package signal

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 金标准对照：docs/argus_single/backtest_dual_side.py。
//
// 输入用仓库内真实数据（logs/argus_single/events-0702 的 5 天事件流 +
// docs/argus_single/backtest_klines_1m.json 的 DeepCoin 1m 缓存），
// 期望值来自把该脚本的 EVDIR/KCACHE 指到这两份数据后跑出来的输出：
//
//	python3 -c "import sys; sys.path.insert(0,'.'); import backtest_dual_side as bd; \
//	  bd.EVDIR='<repo>/logs/argus_single/events-0702'; \
//	  bd.KCACHE='<repo>/docs/argus_single/backtest_klines_1m.json'; bd.main()"
//
// 数据缺失时跳过（CI 上可能没有 logs/），但本地必须绿——它是"回测引擎与
// 历史全部研究结论口径一致"的唯一硬证据。

const (
	parityAccountA = "账户A-1394537246@qq.com"
	parityAccountB = "账户B-mortypeng@gmail.com"
)

// parityParams 脚本里写死的那组常量：LEV=125 FACE=0.001 TAKER=0.0006
// CAT=300 GATE_MIN=20 trail=[小150/0.35 中90/0.28 大40/0.20] tier=[0.30,0.65]，
// 上限固定 cap（不走 cap 公式），无过冲、无趋势闸。
func parityParams(mode string, cap int, pess bool) Params {
	p := DefaultParams()
	p.Mode = mode
	p.CapOverride = cap
	p.OrderSize = 1
	p.TakerFee = 0.0006
	p.CatastropheStopPct = 300
	p.GateMinProfitPct = 20
	p.CatastropheOvershootRoiPts = 0
	if pess {
		p.EvalMode = EvalPessimistic
	}
	return p
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	// .../server/service/trade/strategy/signal/parity_test.go → 上溯 5 层到 server 的父目录
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", ".."))
}

type rawEvent struct {
	Ts        string  `json:"ts"`
	Account   string  `json:"account"`
	Event     string  `json:"event"`
	Side      string  `json:"side"`
	NetSide   string  `json:"netSide"`
	Size      int     `json:"size"`
	OrderSize int     `json:"orderSize"`
	AvgPx     float64 `json:"avgPx"`
	LastPx    float64 `json:"lastPx"`
	RoiPct    float64 `json:"roiPct"`
	GapBp     float64 `json:"gapBp"`
	SigLast   float64 `json:"sigLast"`
}

// loadParityEvents 读 JSONL 事件流。刻意直接解析 JSONL 而不经 eventstore：
// 本测试要验的是"引擎口径与 python 一致"，多一层转换会把失败原因搅浑。
func loadParityEvents(t *testing.T, dir string) []rawEvent {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if err != nil || len(files) == 0 {
		return nil
	}
	var out []rawEvent
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s: %v", f, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var e rawEvent
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatalf("解析 %s: %v", f, err)
			}
			out = append(out, e)
		}
	}
	return out
}

// parityLocal 事件 ts 是无时区本地时间串，脚本用 naive datetime，
// 这里统一用 UTC 承载同样的字面量，保证两侧比较的是同一个时刻。
func parityLocal(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC)
	if err != nil {
		t.Fatalf("解析 ts %q: %v", s, err)
	}
	return ts
}

func paritySignals(t *testing.T, evs []rawEvent, account string) []Signal {
	t.Helper()
	var out []Signal
	for _, e := range evs {
		if e.Account != account {
			continue
		}
		switch e.Event {
		case EvOpen, EvCapSkip, EvGateBlock, EvTrendSkip:
		default:
			continue
		}
		out = append(out, Signal{
			Ts: parityLocal(t, e.Ts), Side: e.Side, Event: e.Event,
			OrderSize: e.OrderSize, GapBp: e.GapBp, SigLast: e.SigLast,
			NetSide: e.NetSide, NetSize: e.Size, AvgPx: e.AvgPx, LastPx: e.LastPx,
		})
	}
	return SortSignals(out)
}

// parityBars 读脚本的 1m 缓存：key 是 UTC 毫秒，value 是 [o,h,l,c]，
// 落到本地时区口径要 +8h（脚本 fetch_klines 的同一处理）。窗口与脚本一致。
func parityBars(t *testing.T, path string) []Bar {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string][]float64
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("解析 K 线缓存: %v", err)
	}
	winStart := time.Date(2026, 6, 28, 22, 0, 0, 0, time.UTC).Add(-5 * time.Minute)
	winEnd := time.Date(2026, 7, 2, 15, 35, 0, 0, time.UTC).Add(5 * time.Minute)
	out := make([]Bar, 0, len(raw))
	for ms, ohlc := range raw {
		n, err := strconv.ParseInt(ms, 10, 64)
		if err != nil || len(ohlc) < 4 {
			continue
		}
		ts := time.UnixMilli(n).UTC().Add(8 * time.Hour)
		if ts.Before(winStart) || ts.After(winEnd) {
			continue
		}
		out = append(out, Bar{Ts: ts, Open: ohlc[0], High: ohlc[1], Low: ohlc[2], Close: ohlc[3]})
	}
	return SortBars(out)
}

func parityFixture(t *testing.T) ([]Signal, []Bar, SeedPosition) {
	t.Helper()
	root := repoRoot(t)
	evs := loadParityEvents(t, filepath.Join(root, "logs", "argus_single", "events-0702"))
	bars := parityBars(t, filepath.Join(root, "docs", "argus_single", "backtest_klines_1m.json"))
	if len(evs) == 0 || len(bars) == 0 {
		t.Skip("缺少金标准数据（logs/argus_single/events-0702 或 docs/argus_single/backtest_klines_1m.json）")
	}
	sigs := paritySignals(t, evs, parityAccountA)
	// 种子仓口径同脚本 extract_seed：第一条带 avgPx+size+netSide 的事件。
	var seed SeedPosition
	for _, e := range evs {
		if e.Account == parityAccountA && e.AvgPx > 0 && e.Size > 0 && e.NetSide != "" {
			seed = SeedPosition{At: parityLocal(t, e.Ts), Side: strings.ToLower(e.NetSide), Size: e.Size, AvgPx: e.AvgPx}
			break
		}
	}
	if !seed.OK() {
		t.Fatal("金标准数据里找不到种子仓事件")
	}
	return sigs, bars, seed
}

// TestParityFixtureMatchesScript 先钉住输入本身：脚本打印
// 「信号数(A)=334 种子仓: 2026-06-28 22:37:18 long 14张 @60258.1」。
// 输入对不上时，后面的数值比较全都没有意义。
func TestParityFixtureMatchesScript(t *testing.T) {
	sigs, bars, seed := parityFixture(t)
	if len(sigs) != 334 {
		t.Errorf("信号数(A) = %d, 金标准 334", len(sigs))
	}
	if len(bars) != 5383 {
		t.Errorf("K 线根数 = %d, 金标准 5383", len(bars))
	}
	if seed.Side != "long" || seed.Size != 14 || math.Abs(seed.AvgPx-60258.1) > 0.05 {
		t.Errorf("种子仓 = %s %d张 @%.1f, 金标准 long 14张 @60258.1", seed.Side, seed.Size, seed.AvgPx)
	}
	if got := seed.At.Format("2006-01-02 15:04:05"); got != "2026-06-28 22:37:18" {
		t.Errorf("种子时刻 = %s, 金标准 2026-06-28 22:37:18", got)
	}
	// A/B 两账户的信号流必须一致（脚本的交叉校验：334/334 全匹配）——
	// 它证明"信号是市场侧事件、按账户过滤不会丢触发"。
	root := repoRoot(t)
	evs := loadParityEvents(t, filepath.Join(root, "logs", "argus_single", "events-0702"))
	if b := paritySignals(t, evs, parityAccountB); len(b) != len(sigs) {
		t.Errorf("账户B 信号数 = %d, 账户A = %d，两者应相等", len(b), len(sigs))
	}
}

// TestParityAgainstDualSideScript 六个口径逐项对齐 backtest_dual_side.py 的
// 「=== 对比运行 ===」输出。脚本按 2 位小数打印，容差取 0.01。
func TestParityAgainstDualSideScript(t *testing.T) {
	sigs, bars, seed := parityFixture(t)

	cases := []struct {
		name     string
		mode     string
		cap      int
		pess     bool
		realized float64
		fees     float64
		floating float64
		net      float64
		mdd      float64
		trail    int
		cat      int
		reduces  int
		skipCap  int
		skipGate int
		maxStack int
	}{
		{"S0 净仓+门控 close评估", ModeNet, 15, false, 3.78, 10.46, -2.89, -9.56, 37.98, 8, 2, 18, 40, 132, 15},
		{"S1a 双向 cap15/侧 close评估", ModeDual, 15, false, 28.77, 18.33, -0.92, 9.52, 16.94, 19, 2, 0, 76, 0, 30},
		{"S1b 双向 cap7/侧 close评估", ModeDual, 7, false, 16.57, 11.86, -0.92, 3.79, 14.33, 23, 2, 0, 167, 0, 21},
		{"S0 净仓+门控 悲观bar", ModeNet, 15, true, 3.18, 8.00, -1.63, -6.44, 34.37, 8, 1, 8, 66, 152, 15},
		{"S1a 双向 cap15/侧 悲观bar", ModeDual, 15, true, 30.02, 18.82, -3.71, 7.49, 17.43, 24, 2, 0, 73, 0, 30},
		{"S1b 双向 cap7/侧 悲观bar", ModeDual, 7, true, 18.83, 13.52, -3.71, 1.59, 13.72, 28, 2, 0, 147, 0, 21},
	}

	const tol = 0.011
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Replay(Input{Params: parityParams(c.mode, c.cap, c.pess), Signals: sigs, Bars: bars, Seed: seed})
			if err != nil {
				t.Fatalf("回放失败: %v", err)
			}
			m := Aggregate(res)
			checkFloat(t, "已实现PnL", res.Realized, c.realized, tol)
			checkFloat(t, "手续费", res.Fees, c.fees, tol)
			checkFloat(t, "期末浮动", res.Floating, c.floating, tol)
			checkFloat(t, "净结果", res.Net, c.net, tol)
			checkFloat(t, "MTM最大回撤U", res.MaxDrawdown, c.mdd, tol)
			checkInt(t, "trailing平仓", m.TrailCount, c.trail)
			checkInt(t, "兜底", m.SlCount, c.cat)
			checkInt(t, "盈利减仓", res.Reduces, c.reduces)
			checkInt(t, "cap跳过", res.SkipCap, c.skipCap)
			checkInt(t, "gate跳过", res.SkipGate, c.skipGate)
			checkInt(t, "最大堆积", res.MaxStack, c.maxStack)

			// 事件级精度 + 零丢失零重复：回放数 + 丢弃数必须等于输入总数。
			if res.Fidelity.Level != FidelityEvent {
				t.Errorf("精度等级 = %s, 期望 %s", res.Fidelity.Level, FidelityEvent)
			}
			if res.SignalReplayed+res.SignalDropped != res.SignalTotal {
				t.Errorf("信号守恒被破坏: 回放%d + 丢弃%d ≠ 总数%d",
					res.SignalReplayed, res.SignalDropped, res.SignalTotal)
			}
			// 逐 episode 的手续费之和必须等于全局手续费（per-book 归集无遗漏）。
			var feeSum float64
			for _, ep := range res.Episodes {
				feeSum += ep.Fee
			}
			if math.Abs(feeSum-res.Fees) > 1e-6 {
				t.Errorf("episode 手续费合计 %.6f ≠ 全局 %.6f", feeSum, res.Fees)
			}
		})
	}
}

// TestParityFinalPositionS0 脚本的 S0 期末持仓：{'long': (7, 60514.1, -85.2)}。
// 期末浮动对得上但均价/张数对不上，说明中间路径走错却在末尾凑巧抵消。
func TestParityFinalPositionS0(t *testing.T) {
	sigs, bars, seed := parityFixture(t)
	res, err := Replay(Input{Params: parityParams(ModeNet, 15, false), Signals: sigs, Bars: bars, Seed: seed})
	if err != nil {
		t.Fatalf("回放失败: %v", err)
	}
	if len(res.OpenPositions) != 1 {
		t.Fatalf("期末未平仓位数 = %d, 期望 1", len(res.OpenPositions))
	}
	ep := res.OpenPositions[0]
	if ep.Side != "long" || ep.Contracts != 7 {
		t.Errorf("期末持仓 = %s %d张, 期望 long 7张", ep.Side, ep.Contracts)
	}
	if math.Abs(ep.AvgPx-60514.1) > 0.05 {
		t.Errorf("期末均价 = %.1f, 期望 60514.1", ep.AvgPx)
	}
	if math.Abs(ep.RoiPct-(-85.2)) > 0.1 {
		t.Errorf("期末 ROI = %.1f%%, 期望 -85.2%%", ep.RoiPct)
	}
}

// TestParityCloseSequenceS0 逐笔平仓序列（脚本「平仓明细（close评估口径）」的 S0 段）：
// 时刻 + 归因 + 方向 + 张数 + ROI 全对上，才能说"每一笔都走对了"。
func TestParityCloseSequenceS0(t *testing.T) {
	sigs, bars, seed := parityFixture(t)
	res, err := Replay(Input{Params: parityParams(ModeNet, 15, false), Signals: sigs, Bars: bars, Seed: seed})
	if err != nil {
		t.Fatalf("回放失败: %v", err)
	}
	want := []struct {
		at        string
		kind      string
		side      string
		contracts int
		roi       float64
		pnl       float64
	}{
		{"2026-06-29 20:08:00", ExitTrailing, "long", 15, 35.2, 2.55},
		{"2026-06-29 21:59:00", ExitTrailing, "short", 14, 72.8, 4.88},
		{"2026-06-29 23:40:00", ExitTrailing, "long", 11, 67.6, 3.54},
		{"2026-06-30 01:21:00", ExitTrailing, "long", 15, 94.5, 6.80},
		{"2026-06-30 20:58:00", ExitCatastrophe, "long", 13, -300.0, -18.66},
		{"2026-06-30 23:07:00", ExitTrailing, "short", 15, 38.9, 2.74},
	}
	got := make([]Episode, 0, len(res.Episodes))
	for _, ep := range res.Episodes {
		if ep.Reason == ExitTrailing || ep.Reason == ExitCatastrophe {
			got = append(got, ep)
		}
	}
	if len(got) < len(want) {
		t.Fatalf("平仓笔数 = %d, 至少应有 %d 笔", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if at := g.ClosedAt.Format("2006-01-02 15:04:05"); at != w.at {
			t.Errorf("第%d笔时刻 = %s, 期望 %s", i+1, at, w.at)
		}
		if g.Reason != w.kind || g.Side != w.side || g.Contracts != w.contracts {
			t.Errorf("第%d笔 = %s %s %d张, 期望 %s %s %d张", i+1, g.Reason, g.Side, g.Contracts, w.kind, w.side, w.contracts)
		}
		if math.Abs(g.RoiPct-w.roi) > 0.06 {
			t.Errorf("第%d笔 ROI = %+.1f%%, 期望 %+.1f%%", i+1, g.RoiPct, w.roi)
		}
		if math.Abs(g.Pnl-w.pnl) > 0.011 {
			t.Errorf("第%d笔 pnl = %+.2fU, 期望 %+.2fU", i+1, g.Pnl, w.pnl)
		}
	}
}

func checkFloat(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.4f, 金标准 %.2f（差 %.4f）", name, got, want, got-want)
	}
}

func checkInt(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, 金标准 %d", name, got, want)
	}
}
