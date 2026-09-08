package signal

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 校准金标准：docs/argus_single/study_fine.csv —— 2026-07-02 那轮研究用
// backtest_capsf_study.py 跑出来的 10 个精算格（1459 信号 / 23 天 / 每格 16 路径）。
// 需求大纲 §6.1 第 7 条把"逐格吻合"列为本任务的验收口径。
//
// 输入同样是仓库内的真实数据：
//
//	docs/argus_single/signals_canonical.csv          （ts, side, ref_px, source）
//	docs/argus_single/backtest_klines_1m_study.json  （DeepCoin 1m 缓存，33k 根）
//
// 这份测试是本任务最硬的一条证据：它同时钉住了引擎口径（r8）、降噪协议的
// 路径展开顺序、5% 丢弃的随机流、λ_bear 的分母口径、p90 的取分位方式、
// 以及情景 bootstrap 的重采样序列。任何一处漂移它都会红。
//
// 数据缺失时跳过（CI 上可能没有 docs/），本地必须绿。

var (
	calibWinStart = time.Date(2026, 6, 9, 17, 0, 0, 0, time.UTC)
	calibWinEnd   = time.Date(2026, 7, 2, 18, 15, 0, 0, time.UTC)
)

// calibBaseParams 脚本里锁定的那组常量：E=375.73U、有效 taker 0.012%/边、
// 兜底过冲 +5 ROI 点、order_size=1、trail 固定 champion 值、无趋势闸。
func calibBaseParams() Params {
	p := DefaultParams()
	p.OrderSize = 1
	p.RiskEquity = 375.73
	p.TakerFee = 0.00012
	p.CatastropheOvershootRoiPts = 5
	p.EntryPx = EntryBarClose
	return p
}

// 时间戳一律按 UTC 标签承载"上海墙钟"（口径同 parity_test.go）：
// 缓存里的 ms 是 UTC，脚本 +8h 当本地时刻用，这样测试不依赖跑测机器的时区。
func calibLoadBars(t *testing.T, path string) []Bar {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("缺少 1m K 线缓存 %s：跳过金标准校准", path)
	}
	var cache map[string][]float64
	if err := json.Unmarshal(raw, &cache); err != nil {
		t.Fatalf("解析 K 线缓存失败: %v", err)
	}
	out := make([]Bar, 0, len(cache))
	for ms, ohlc := range cache {
		n, err := strconv.ParseInt(ms, 10, 64)
		if err != nil || len(ohlc) < 4 {
			continue
		}
		ts := time.UnixMilli(n).UTC().Add(8 * time.Hour)
		if ts.Before(calibWinStart) || ts.After(calibWinEnd) {
			continue
		}
		out = append(out, Bar{Ts: ts, Open: ohlc[0], High: ohlc[1], Low: ohlc[2], Close: ohlc[3]})
	}
	return SortBars(out)
}

func calibLoadSignals(t *testing.T, path string) []Signal {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("缺少 canonical 信号流 %s：跳过金标准校准", path)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatalf("解析 signals_canonical.csv 失败: %v", err)
	}
	out := make([]Signal, 0, len(rows)-1)
	for _, r := range rows[1:] {
		if len(r) < 2 {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(r[0]), time.UTC)
		if err != nil {
			continue
		}
		// canonical 流只有 ts+side：这一段历史早于事件双写，没有 gapBp/orderSize。
		// 回放不需要它们（阈值不变 ⇒ 不做门限筛；order_size 由参数给 1）。
		out = append(out, Signal{Ts: ts, Side: strings.ToLower(strings.TrimSpace(r[1])), Event: EvOpen, OrderSize: 1})
	}
	return SortSignals(out)
}

type goldenCell struct {
	Mode     string
	Cap      int
	S        float64
	Gate     float64
	MedPnl28 float64
	Sign     float64
	LamBear  float64
	MeanLoss float64
	Budget   float64
	BearP10  float64
	BearP50  float64
	P90Dd    float64
	Stack    int
	Fee      float64
}

func calibLoadGolden(t *testing.T, path string) []goldenCell {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("缺少金标准 %s：跳过校准", path)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatalf("解析 study_fine.csv 失败: %v", err)
	}
	num := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return v
	}
	out := make([]goldenCell, 0, len(rows)-1)
	for _, r := range rows[1:] {
		if len(r) < 14 {
			continue
		}
		out = append(out, goldenCell{
			Mode: r[0], Cap: int(num(r[1])), S: num(r[2]), Gate: num(r[3]),
			MedPnl28: num(r[4]), Sign: num(r[5]), LamBear: num(r[6]), MeanLoss: num(r[7]),
			Budget: num(r[8]), BearP10: num(r[9]), BearP50: num(r[10]), P90Dd: num(r[11]),
			Stack: int(num(r[12])), Fee: num(r[13]),
		})
	}
	return out
}

func calibRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// .../server/service/trade/strategy/signal → 上溯 5 层
	return filepath.Clean(filepath.Join(wd, "..", "..", "..", "..", ".."))
}

// TestStudyMatchesGoldenFineGrid 精算 10 格逐格比对 study_fine.csv。
func TestStudyMatchesGoldenFineGrid(t *testing.T) {
	root := calibRepoRoot(t)
	docs := filepath.Join(root, "docs", "argus_single")
	golden := calibLoadGolden(t, filepath.Join(docs, "study_fine.csv"))
	bars := calibLoadBars(t, filepath.Join(docs, "backtest_klines_1m_study.json"))
	signals := calibLoadSignals(t, filepath.Join(docs, "signals_canonical.csv"))

	// 脚本自报：信号=1459 bars=33196 天标签={trend:11, vol:7, quiet:6}
	if len(signals) != 1459 {
		t.Fatalf("canonical 信号数 %d，期望 1459（数据被改动过？）", len(signals))
	}
	if len(bars) != 33196 {
		t.Fatalf("窗口内 1m 根数 %d，期望 33196", len(bars))
	}
	proto := FineProtocol()
	labels := DayLabels(bars, proto.Regime)
	if got := CountLabel(labels, RegimeTrend); got != 11 {
		t.Fatalf("单边日 %d 天，期望 11 天（日标签口径漂移）", got)
	}
	if got := CountLabel(labels, RegimeVol); got != 7 {
		t.Fatalf("震荡日 %d 天，期望 7 天", got)
	}
	if got := len(proto.Paths()); got != 16 {
		t.Fatalf("精算协议展开 %d 条路径，期望 16 条", got)
	}

	base := calibBaseParams()
	for _, g := range golden {
		spec := CellSpec{Mode: g.Mode, Cap: g.Cap, StopPct: g.S, GatePct: g.Gate}
		in := Input{Params: spec.Params(base), Signals: signals, Bars: bars}
		outcomes := make([]PathOutcome, 0, 16)
		for _, ps := range proto.Paths() {
			o, err := RunPath(in, ps, calibWinStart, labels, proto)
			if err != nil {
				t.Fatalf("%s 路径 %s 失败: %v", spec.Key(), ps.Label(), err)
			}
			outcomes = append(outcomes, *o)
		}
		st := AggregateCell(spec, outcomes, labels, proto)

		// 这些量是纯确定性的（不含 bootstrap），必须精确到 1e-6。
		assertClose(t, spec.Key()+" med_pnl28", st.MedPnl28, g.MedPnl28, 1e-6)
		assertClose(t, spec.Key()+" sign", st.SignRatio, g.Sign, 1e-9)
		assertClose(t, spec.Key()+" lam_bear", st.LambdaBearPerMonth, g.LamBear, 1e-9)
		assertClose(t, spec.Key()+" mean_loss", st.MeanStopLoss, g.MeanLoss, 1e-6)
		assertClose(t, spec.Key()+" budget", st.StopBudget, g.Budget, 1e-6)
		assertClose(t, spec.Key()+" p90dd", st.P90MaxDrawdown, g.P90Dd, 1e-6)
		assertClose(t, spec.Key()+" fee", st.MedFee, g.Fee, 1e-6)
		if st.MaxStack != g.Stack {
			t.Errorf("%s max_stack: got %d want %d", spec.Key(), st.MaxStack, g.Stack)
		}
		// bootstrap 分位是同一条 MT19937 流上的重采样，同样应精确吻合；
		// 放到 1e-6 只为容忍浮点求和顺序（脚本自身重跑也有 ~1e-13 的抖动）。
		bear := st.Scenario(ScenarioBear)
		if bear == nil || bear.PoolSize == 0 {
			t.Fatalf("%s 熊市月情景池为空", spec.Key())
		}
		assertClose(t, spec.Key()+" bear_p10", bear.P10, g.BearP10, 1e-6)
		assertClose(t, spec.Key()+" bear_p50", bear.P50, g.BearP50, 1e-6)
	}
}

// TestGoldenFineGridThreeGateVerdict 三关判定必须与设计文档 §10.2 一致：
// 0 格通过，走无解分支；现行 champion net(15,300) 被脊线格全面支配。
func TestGoldenFineGridThreeGateVerdict(t *testing.T) {
	root := calibRepoRoot(t)
	docs := filepath.Join(root, "docs", "argus_single")
	golden := calibLoadGolden(t, filepath.Join(docs, "study_fine.csv"))
	if len(golden) == 0 {
		t.Skip("缺少金标准")
	}
	gates := DefaultGates().Normalize()
	// −56.36 / 93.93 —— §10.2 记作 −56U / 94U
	assertClose(t, "bearNetP10Min", gates.BearNetP10Min, -56.3595, 1e-3)
	assertClose(t, "maxDrawdownMax", gates.MaxDrawdownMax, 93.9325, 1e-3)

	passed := 0
	for _, g := range golden {
		st := CellStats{Spec: CellSpec{Mode: g.Mode, Cap: g.Cap, StopPct: g.S, GatePct: g.Gate},
			MedPnl28: g.MedPnl28, SignRatio: g.Sign, P90MaxDrawdown: g.P90Dd,
			LambdaBearPerMonth: g.LamBear, MeanStopLoss: g.MeanLoss, StopBudget: g.Budget,
			Scenarios: []ScenarioResult{{Scenario: ScenarioBear, PoolSize: 176, P10: g.BearP10, P50: g.BearP50}}}
		st.Key = st.Spec.Key()
		if Judge(st, gates).Passed {
			passed++
			t.Errorf("%s 通过了三关，但 §10.2 的结论是 0 格通过", st.Key)
		}
	}
	if passed != 0 {
		t.Fatalf("通过三关的格子数 %d，期望 0（无解分支）", passed)
	}
}

func assertClose(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.10f want %.10f (差 %.3g)", what, got, want, got-want)
	}
}
