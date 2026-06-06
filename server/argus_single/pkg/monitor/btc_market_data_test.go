package monitor

import (
	"math"
	"strconv"
	"testing"
)

// ─── calculateSMA ─────────────────────────────────────────────────────────────

func TestCalculateSMA_Basic(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5}
	got, err := calculateSMA(values, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3.0 {
		t.Fatalf("SMA(5) = %f, want 3.0", got)
	}
}

func TestCalculateSMA_Window(t *testing.T) {
	// SMA of last 3 from [1,2,3,4,5] = (3+4+5)/3 = 4
	values := []float64{1, 2, 3, 4, 5}
	got, err := calculateSMA(values, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 4.0 {
		t.Fatalf("SMA(3) = %f, want 4.0", got)
	}
}

func TestCalculateSMA_NotEnoughValues(t *testing.T) {
	_, err := calculateSMA([]float64{1, 2}, 5)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

func TestCalculateSMA_Single(t *testing.T) {
	got, err := calculateSMA([]float64{42}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("SMA(1) = %f, want 42", got)
	}
}

// ─── calculateEMA ─────────────────────────────────────────────────────────────

func TestCalculateEMA_Constant(t *testing.T) {
	// EMA of all-same values should equal that value
	values := make([]float64, 20)
	for i := range values {
		values[i] = 100
	}
	got, err := calculateEMA(values, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-100) > 0.001 {
		t.Fatalf("EMA of constant = %f, want 100", got)
	}
}

func TestCalculateEMA_NotEnoughValues(t *testing.T) {
	_, err := calculateEMA([]float64{1, 2}, 5)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

func TestCalculateEMA_GreaterThanSMAWhenRising(t *testing.T) {
	// Rising series: EMA should be > SMA because it weights recent more
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ema, err := calculateEMA(values, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sma, _ := calculateSMA(values, 5)
	if ema <= sma {
		t.Fatalf("EMA (%f) should be > SMA (%f) for rising series", ema, sma)
	}
}

// ─── calculateRSI ─────────────────────────────────────────────────────────────

func TestCalculateRSI_AllGains(t *testing.T) {
	// All prices increasing → RSI = 100
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	got, err := calculateRSI(values, 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 100 {
		t.Fatalf("RSI with all gains = %f, want 100", got)
	}
}

func TestCalculateRSI_AllLosses(t *testing.T) {
	// All prices decreasing → RSI = 0
	values := []float64{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	got, err := calculateRSI(values, 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("RSI with all losses = %f, want 0", got)
	}
}

func TestCalculateRSI_Range(t *testing.T) {
	values := []float64{44, 44.34, 44.09, 44.15, 43.61, 44.33, 44.83, 45.1, 45.15, 43.61, 44.33, 44.83, 45.1, 45.15, 45.98, 45.5}
	got, err := calculateRSI(values, 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got < 0 || got > 100 {
		t.Fatalf("RSI = %f, should be in [0,100]", got)
	}
}

func TestCalculateRSI_NotEnoughValues(t *testing.T) {
	_, err := calculateRSI([]float64{1, 2, 3}, 14)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

// ─── calculateBollingerBands ──────────────────────────────────────────────────

func TestCalculateBollingerBands_Constant(t *testing.T) {
	// Constant values → std=0 → upper=middle=lower
	values := make([]float64, 20)
	for i := range values {
		values[i] = 50
	}
	middle, upper, lower, width, err := calculateBollingerBands(values, 20, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(middle-50) > 0.001 {
		t.Fatalf("middle = %f, want 50", middle)
	}
	if math.Abs(upper-lower) > 0.001 {
		t.Fatalf("upper (%f) != lower (%f) for constant values", upper, lower)
	}
	if width != 0 {
		t.Fatalf("width = %f, want 0 for constant values", width)
	}
}

func TestCalculateBollingerBands_Ordering(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100,
		10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	middle, upper, lower, _, err := calculateBollingerBands(values, 20, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upper <= middle {
		t.Fatalf("upper (%f) should be > middle (%f)", upper, middle)
	}
	if lower >= middle {
		t.Fatalf("lower (%f) should be < middle (%f)", lower, middle)
	}
}

func TestCalculateBollingerBands_NotEnoughValues(t *testing.T) {
	_, _, _, _, err := calculateBollingerBands([]float64{1, 2}, 20, 2)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

// ─── calculateMACD ────────────────────────────────────────────────────────────

func TestCalculateMACD_HistogramIsMACSMinusSignal(t *testing.T) {
	// Generate 50 values
	values := make([]float64, 50)
	for i := range values {
		values[i] = float64(i + 1)
	}
	macd, signal, histogram, err := calculateMACD(values, 12, 26, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs((macd-signal)-histogram) > 0.0001 {
		t.Fatalf("histogram (%f) != macd (%f) - signal (%f)", histogram, macd, signal)
	}
}

func TestCalculateMACD_NotEnoughValues(t *testing.T) {
	_, _, _, err := calculateMACD([]float64{1, 2, 3}, 12, 26, 9)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

// ─── calculateATR ─────────────────────────────────────────────────────────────

func makeKlines(opens, highs, lows, closes []float64) []BTCKline {
	klines := make([]BTCKline, len(opens))
	for i := range opens {
		klines[i] = BTCKline{
			OpenPrice:  strconv.FormatFloat(opens[i], 'f', -1, 64),
			HighPrice:  strconv.FormatFloat(highs[i], 'f', -1, 64),
			LowPrice:   strconv.FormatFloat(lows[i], 'f', -1, 64),
			ClosePrice: strconv.FormatFloat(closes[i], 'f', -1, 64),
			Volume:     "1000",
		}
	}
	return klines
}

func TestCalculateATR_Positive(t *testing.T) {
	n := 20
	opens := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := range opens {
		opens[i] = float64(100 + i)
		highs[i] = float64(105 + i)
		lows[i] = float64(95 + i)
		closes[i] = float64(102 + i)
	}
	klines := makeKlines(opens, highs, lows, closes)
	got, err := calculateATR(klines, 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("ATR = %f, should be positive", got)
	}
}

func TestCalculateATR_NotEnoughKlines(t *testing.T) {
	klines := makeKlines([]float64{1}, []float64{2}, []float64{0.5}, []float64{1.5})
	_, err := calculateATR(klines, 14)
	if err == nil {
		t.Fatal("expected error for insufficient klines")
	}
}

// ─── trimKlines ───────────────────────────────────────────────────────────────

func TestTrimKlines_NoTrim(t *testing.T) {
	klines := []BTCKline{{OpenTime: "1"}, {OpenTime: "2"}}
	got := trimKlines(klines, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestTrimKlines_ExactMax(t *testing.T) {
	klines := []BTCKline{{OpenTime: "1"}, {OpenTime: "2"}, {OpenTime: "3"}}
	got := trimKlines(klines, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
}

func TestTrimKlines_OverMax(t *testing.T) {
	klines := make([]BTCKline, 10)
	for i := range klines {
		klines[i] = BTCKline{OpenTime: strconv.Itoa(i)}
	}
	got := trimKlines(klines, 5)
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}
	if got[0].OpenTime != "5" {
		t.Fatalf("expected first trimmed kline to be OpenTime=5, got %q", got[0].OpenTime)
	}
}

// ─── klineCloses ─────────────────────────────────────────────────────────────

func TestKlineCloses_Valid(t *testing.T) {
	klines := []BTCKline{
		{ClosePrice: "100"},
		{ClosePrice: "200"},
		{ClosePrice: "300"},
	}
	closes, err := klineCloses(klines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closes) != 3 || closes[1] != 200 {
		t.Fatalf("unexpected closes: %v", closes)
	}
}

func TestKlineCloses_InvalidPrice(t *testing.T) {
	klines := []BTCKline{{ClosePrice: "not-a-number"}}
	_, err := klineCloses(klines)
	if err == nil {
		t.Fatal("expected error for invalid close price")
	}
}

// ─── parseRESTKlineRow ────────────────────────────────────────────────────────

func TestParseRESTKlineRow_Valid(t *testing.T) {
	// Binance REST kline row: [openTime, open, high, low, close, vol, closeTime, quoteVol, trades, ...]
	row := []interface{}{
		float64(1700000000000), // 0: openTime
		"90000",                // 1: open
		"91000",                // 2: high
		"89000",                // 3: low
		"90500",                // 4: close
		"1000",                 // 5: volume
		float64(1700003599999), // 6: closeTime
		"90000000",             // 7: quoteVolume
		float64(500),           // 8: tradeCount
		"500",                  // 9: takerBuyVol
		"45000000",             // 10: takerBuyQuote
	}
	kline, err := parseRESTKlineRow(row, "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kline.ClosePrice != "90500" {
		t.Fatalf("ClosePrice = %q, want 90500", kline.ClosePrice)
	}
	if kline.Interval != "1h" {
		t.Fatalf("Interval = %q, want 1h", kline.Interval)
	}
	if kline.TradeCount != 500 {
		t.Fatalf("TradeCount = %d, want 500", kline.TradeCount)
	}
}

func TestParseRESTKlineRow_TooShort(t *testing.T) {
	_, err := parseRESTKlineRow([]interface{}{1, 2, 3}, "1h")
	if err == nil {
		t.Fatal("expected error for short row")
	}
}

// ─── extractRollingInterval ───────────────────────────────────────────────────

func TestExtractRollingInterval_Valid(t *testing.T) {
	cases := []struct {
		stream string
		want   string
	}{
		{"btcusdt@ticker_1h", "1h"},
		{"btcusdt@ticker_4h", "4h"},
		{"btcusdt@ticker_1d", "1d"},
	}
	for _, tc := range cases {
		got := extractRollingInterval(tc.stream)
		if got != tc.want {
			t.Errorf("extractRollingInterval(%q) = %q, want %q", tc.stream, got, tc.want)
		}
	}
}

func TestExtractRollingInterval_NoMatch(t *testing.T) {
	got := extractRollingInterval("btcusdt@ticker")
	if got != "" {
		t.Fatalf("expected empty for non-rolling stream, got %q", got)
	}
}

// ─── calculateEMASequence ─────────────────────────────────────────────────────

func TestCalculateEMASequence_Length(t *testing.T) {
	values := make([]float64, 30)
	for i := range values {
		values[i] = float64(i + 1)
	}
	seq, err := calculateEMASequence(values, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// sequence length = len(values) - period + 1
	expected := len(values) - 10 + 1
	if len(seq) != expected {
		t.Fatalf("sequence length = %d, want %d", len(seq), expected)
	}
}

func TestCalculateEMASequence_NotEnough(t *testing.T) {
	_, err := calculateEMASequence([]float64{1, 2}, 10)
	if err == nil {
		t.Fatal("expected error for insufficient values")
	}
}

// ─── formatFloat ─────────────────────────────────────────────────────────────

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{100.0, "100"},
		{0.5, "0.5"},
		{-1.23456, "-1.23456"},
	}
	for _, tc := range cases {
		got := formatFloat(tc.input)
		if got != tc.want {
			t.Errorf("formatFloat(%f) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
