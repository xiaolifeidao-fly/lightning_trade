package indicator

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	argusTrade "argus_single/pkg/trade"
	"oracle/pkg/collector"
)

// Metric 表示一个技术指标值及其有效性。Valid=false 表示数据不足，
// 不应把 Value(此时为 0) 当作真实指标值参与判断或展示。
type Metric struct {
	Value float64 `json:"value"`
	Valid bool    `json:"valid"`
}

func metric(v float64, ok bool) Metric { return Metric{Value: v, Valid: ok} }

// Features 主周期的技术特征。0 值字段统一用 Metric.Valid 区分"真实为0"与"数据不足"。
type Features struct {
	LastClose float64 `json:"last_close"`

	MA7  Metric `json:"ma7"`
	MA25 Metric `json:"ma25"`
	MA99 Metric `json:"ma99"`

	RSI14      Metric `json:"rsi14"`
	ATR14      Metric `json:"atr14"`
	Volatility Metric `json:"volatility"` // 收益率标准差（%）

	MACDDif  Metric `json:"macd_dif"`
	MACDDea  Metric `json:"macd_dea"`
	MACDHist Metric `json:"macd_hist"`

	BollUpper Metric `json:"boll_upper"`
	BollMid   Metric `json:"boll_mid"`
	BollLower Metric `json:"boll_lower"`

	RecentHigh Metric `json:"recent_high"` // 近 recentLookback 根最高价（阻力参考）
	RecentLow  Metric `json:"recent_low"`  // 近 recentLookback 根最低价（支撑参考）

	VolTrend Metric `json:"vol_trend"` // 近7根均量/近25根均量，>1 放量、<1 缩量

	// BuyVolume/SellVolume 由逐笔成交聚合的主动买/卖量。
	BuyVolume  float64 `json:"buy_volume"`
	SellVolume float64 `json:"sell_volume"`
}

// recentLookback 计算近端高低点(支撑/阻力参考)所用的 K 线根数。
const recentLookback = 30

// Compute 从采集快照计算主周期特征。
func Compute(snap *collector.Snapshot) Features {
	closes := closePrices(snap.Primary)
	f := Features{}
	if n := len(closes); n > 0 {
		f.LastClose = closes[n-1]
	}
	f.MA7 = metric(sma(closes, 7))
	f.MA25 = metric(sma(closes, 25))
	f.MA99 = metric(sma(closes, 99))
	f.RSI14 = metric(rsi(closes, 14))
	f.ATR14 = metric(atr(snap.Primary, 14))
	f.Volatility = metric(volatility(closes))

	dif, dea, hist, ok := macd(closes)
	f.MACDDif = metric(dif, ok)
	f.MACDDea = metric(dea, ok)
	f.MACDHist = metric(hist, ok)

	up, mid, low, ok := bollinger(closes, 20, 2)
	f.BollUpper = metric(up, ok)
	f.BollMid = metric(mid, ok)
	f.BollLower = metric(low, ok)

	high, lowP, ok := recentHighLow(snap.Primary, recentLookback)
	f.RecentHigh = metric(high, ok)
	f.RecentLow = metric(lowP, ok)

	f.VolTrend = metric(volumeTrend(snap.Primary, 7, 25))

	f.BuyVolume, f.SellVolume = tradePressure(snap.Trades)
	return f
}

// Summary 拼成给 AI 的特征文字（主周期 + 高周期 + 逐笔 + 资金费）。
// 数据不足的指标统一显示 N/A，避免被模型误读为真实数值（如 RSI=0）。
func Summary(snap *collector.Snapshot, f Features) string {
	var b strings.Builder
	fmt.Fprintf(&b, "主周期 %s 特征:\n", snap.Interval)
	fmt.Fprintf(&b, "  最新价=%.4f MA7=%s MA25=%s MA99=%s\n",
		f.LastClose, fnum(f.MA7, 4), fnum(f.MA25, 4), fnum(f.MA99, 4))
	fmt.Fprintf(&b, "  RSI14=%s ATR14=%s 收益率波动率=%s%%\n",
		fnum(f.RSI14, 2), fnum(f.ATR14, 4), fnum(f.Volatility, 3))
	fmt.Fprintf(&b, "  MACD: DIF=%s DEA=%s 柱=%s\n",
		fnum(f.MACDDif, 4), fnum(f.MACDDea, 4), fnum(f.MACDHist, 4))
	fmt.Fprintf(&b, "  布林带(20,2): 上轨=%s 中轨=%s 下轨=%s\n",
		fnum(f.BollUpper, 4), fnum(f.BollMid, 4), fnum(f.BollLower, 4))
	fmt.Fprintf(&b, "  近%d根高/低(支撑阻力参考): 高=%s 低=%s\n",
		recentLookback, fnum(f.RecentHigh, 4), fnum(f.RecentLow, 4))
	fmt.Fprintf(&b, "  成交量趋势(近7/近25均量): %s (>1放量,<1缩量)\n", fnum(f.VolTrend, 2))
	fmt.Fprintf(&b, "  均线排列: %s\n", maTrendM(f.MA7, f.MA25, f.MA99))

	if total := f.BuyVolume + f.SellVolume; total > 0 {
		fmt.Fprintf(&b, "  最近%d笔成交: 主动买量=%.4f 主动卖量=%.4f 买占比=%.1f%%\n",
			len(snap.Trades), f.BuyVolume, f.SellVolume, f.BuyVolume/total*100)
	} else {
		b.WriteString("  最近成交: N/A\n")
	}

	if len(snap.HighTF) > 0 {
		b.WriteString("高周期概览:\n")
		for tf, rows := range snap.HighTF {
			c := closePrices(rows)
			ma7 := metric(sma(c, 7))
			ma25 := metric(sma(c, 25))
			ma99 := metric(sma(c, 99))
			fmt.Fprintf(&b, "  [%s] 最新价=%s MA7=%s MA25=%s 排列=%s\n",
				tf, fnum(metric(lastOf(c), len(c) > 0), 4), fnum(ma7, 4), fnum(ma25, 4), maTrendM(ma7, ma25, ma99))
		}
	}

	if snap.Funding != nil {
		fmt.Fprintf(&b, "资金费率: 最新=%s 下期预测=%s\n", snap.Funding.LastRate, snap.Funding.NextRate)
	} else {
		b.WriteString("资金费率: N/A\n")
	}

	// 主周期最近若干根 OHLC 明细，供 AI 看细节（控制 token，仅保留近端）。
	b.WriteString("主周期最近K线(open,high,low,close,vol):\n")
	rows := snap.Primary
	start := 0
	if len(rows) > klineDetailRows {
		start = len(rows) - klineDetailRows
	}
	for _, k := range rows[start:] {
		fmt.Fprintf(&b, "  %s O=%s H=%s L=%s C=%s V=%s\n",
			fmtTime(k.OpenTime), k.OpenPrice, k.HighPrice, k.LowPrice, k.ClosePrice, k.Volume)
	}
	return b.String()
}

// klineDetailRows 注入 prompt 的 OHLC 明细根数（关键价位已由布林/近端高低汇总，明细控量即可）。
const klineDetailRows = 12

// fnum 格式化 Metric：无效时显示 N/A。
func fnum(m Metric, dec int) string {
	if !m.Valid {
		return "N/A"
	}
	return strconv.FormatFloat(m.Value, 'f', dec, 64)
}

// maTrendM 基于 Metric 判断均线排列，任一无效则返回"数据不足"。
func maTrendM(short, mid, long Metric) string {
	if !short.Valid || !mid.Valid || !long.Valid {
		return "数据不足"
	}
	return maTrend(short.Value, mid.Value, long.Value)
}

func maTrend(short, mid, long float64) string {
	if short >= mid && mid >= long {
		return "多头排列(short>=mid>=long)"
	}
	if short <= mid && mid <= long {
		return "空头排列(short<=mid<=long)"
	}
	return "震荡/纠缠"
}

func closePrices(rows []argusTrade.MarketKline) []float64 {
	out := make([]float64, 0, len(rows))
	for _, k := range rows {
		if v, err := strconv.ParseFloat(strings.TrimSpace(k.ClosePrice), 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// sma 简单移动平均；数据量不足 period 时返回 ok=false（不再用部分数据凑均值，避免误导排列判断）。
func sma(values []float64, period int) (float64, bool) {
	n := len(values)
	if n == 0 || period <= 0 || n < period {
		return 0, false
	}
	sum := 0.0
	for _, v := range values[n-period:] {
		sum += v
	}
	return sum / float64(period), true
}

// rsi 采用 Wilder 平滑(与 TradingView/交易所口径一致)：先取前 period 个涨跌的简单均值做种子，
// 其后逐根用 avg = (avg*(period-1)+当前)/period 平滑。
func rsi(values []float64, period int) (float64, bool) {
	n := len(values)
	if n <= period {
		return 0, false
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		change := values[i] - values[i-1]
		if change >= 0 {
			gain += change
		} else {
			loss -= change
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	for i := period + 1; i < n; i++ {
		change := values[i] - values[i-1]
		g, l := 0.0, 0.0
		if change >= 0 {
			g = change
		} else {
			l = -change
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
	}
	if avgLoss == 0 {
		return 100, true
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs), true
}

func atr(rows []argusTrade.MarketKline, period int) (float64, bool) {
	n := len(rows)
	if n < 2 {
		return 0, false
	}
	if period > n-1 {
		period = n - 1
	}
	sum := 0.0
	for i := n - period; i < n; i++ {
		high, _ := strconv.ParseFloat(rows[i].HighPrice, 64)
		low, _ := strconv.ParseFloat(rows[i].LowPrice, 64)
		prevClose, _ := strconv.ParseFloat(rows[i-1].ClosePrice, 64)
		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		sum += tr
	}
	return sum / float64(period), true
}

func volatility(values []float64) (float64, bool) {
	n := len(values)
	if n < 2 {
		return 0, false
	}
	returns := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		if values[i-1] != 0 {
			returns = append(returns, (values[i]-values[i-1])/values[i-1])
		}
	}
	if len(returns) == 0 {
		return 0, false
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns))
	return math.Sqrt(variance) * 100, true
}

// emaSeries 计算指数移动平均序列。数据足够时用前 period 个值的 SMA 做种子
// （TradingView/通用口径），不足 period 时退化为首元素种子。
func emaSeries(values []float64, period int) []float64 {
	n := len(values)
	if n == 0 || period <= 0 {
		return nil
	}
	k := 2.0 / (float64(period) + 1)
	out := make([]float64, n)
	if n < period {
		out[0] = values[0]
		for i := 1; i < n; i++ {
			out[i] = values[i]*k + out[i-1]*(1-k)
		}
		return out
	}
	// 种子区先填原值（不作为最终结果使用），out[period-1] 置为前 period 个值的 SMA。
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
		out[i] = values[i]
	}
	out[period-1] = sum / float64(period)
	for i := period; i < n; i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

// volumes 解析 K 线成交量序列。
func volumes(rows []argusTrade.MarketKline) []float64 {
	out := make([]float64, 0, len(rows))
	for _, k := range rows {
		if v, err := strconv.ParseFloat(strings.TrimSpace(k.Volume), 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// volumeTrend 近 shortN 根均量 / 近 longN 根均量，>1 放量、<1 缩量；数据不足或基准为 0 时 ok=false。
func volumeTrend(rows []argusTrade.MarketKline, shortN, longN int) (float64, bool) {
	vols := volumes(rows)
	shortAvg, ok1 := sma(vols, shortN)
	longAvg, ok2 := sma(vols, longN)
	if !ok1 || !ok2 || longAvg == 0 {
		return 0, false
	}
	return shortAvg / longAvg, true
}

// macd 计算 MACD(12,26,9)：DIF、DEA 与柱值(2*(DIF-DEA))；数据不足 26 根时 ok=false。
func macd(closes []float64) (dif, dea, hist float64, ok bool) {
	n := len(closes)
	if n < 26 {
		return 0, 0, 0, false
	}
	ema12 := emaSeries(closes, 12)
	ema26 := emaSeries(closes, 26)
	difSeries := make([]float64, n)
	for i := 0; i < n; i++ {
		difSeries[i] = ema12[i] - ema26[i]
	}
	deaSeries := emaSeries(difSeries, 9)
	dif = difSeries[n-1]
	dea = deaSeries[n-1]
	hist = (dif - dea) * 2
	return dif, dea, hist, true
}

// bollinger 计算布林带：中轨=SMA(period)，上下轨=中轨±k×标准差；数据不足 period 时 ok=false。
func bollinger(values []float64, period int, k float64) (upper, mid, lower float64, ok bool) {
	n := len(values)
	if period <= 0 || n < period {
		return 0, 0, 0, false
	}
	window := values[n-period:]
	sum := 0.0
	for _, v := range window {
		sum += v
	}
	mid = sum / float64(period)
	variance := 0.0
	for _, v := range window {
		variance += (v - mid) * (v - mid)
	}
	sd := math.Sqrt(variance / float64(period))
	return mid + k*sd, mid, mid - k*sd, true
}

// recentHighLow 取最近 n 根 K 线的最高/最低价，作为支撑/阻力参考。
func recentHighLow(rows []argusTrade.MarketKline, n int) (high, low float64, ok bool) {
	if len(rows) == 0 {
		return 0, 0, false
	}
	start := 0
	if len(rows) > n {
		start = len(rows) - n
	}
	high = math.Inf(-1)
	low = math.Inf(1)
	for _, k := range rows[start:] {
		if h, err := strconv.ParseFloat(k.HighPrice, 64); err == nil && h > high {
			high = h
		}
		if l, err := strconv.ParseFloat(k.LowPrice, 64); err == nil && l < low {
			low = l
		}
	}
	if math.IsInf(high, 0) || math.IsInf(low, 0) {
		return 0, 0, false
	}
	return high, low, true
}

func tradePressure(trades []argusTrade.MarketTrade) (buy, sell float64) {
	for _, t := range trades {
		qty, err := strconv.ParseFloat(strings.TrimSpace(t.Qty), 64)
		if err != nil {
			continue
		}
		// IsBuyerMaker=true 表示买方挂单，本笔为主动卖出。
		if t.IsBuyerMaker {
			sell += qty
		} else {
			buy += qty
		}
	}
	return buy, sell
}

func lastOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func fmtTime(openTimeMs int64) string {
	return time.UnixMilli(openTimeMs).Format("01-02 15:04")
}
