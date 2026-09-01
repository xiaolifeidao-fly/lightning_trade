package trade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarketDataClientBinancePublicEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fapi/v1/klines":
			if got := r.URL.Query().Get("limit"); got != "1500" {
				t.Fatalf("kline limit = %s, want 1500", got)
			}
			_, _ = w.Write([]byte(`[[1710000000000,"100","110","90","105","2",1710000059999,"210",7]]`))
		case "/fapi/v1/ticker/24hr":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","lastPrice":"105","bidPrice":"104","askPrice":"106","closeTime":1710000060000}`))
		case "/fapi/v1/aggTrades":
			_, _ = w.Write([]byte(`[{"a":12,"p":"105","q":"2","T":1710000060000,"m":true}]`))
		case "/fapi/v1/fundingRate":
			_, _ = w.Write([]byte(`[{"fundingRate":"0.0001","fundingTime":1710000000000}]`))
		case "/fapi/v1/premiumIndex":
			_, _ = w.Write([]byte(`{"lastFundingRate":"0.0002","nextFundingTime":1710028800000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newMarketDataClient(server.Client(), server.URL, server.URL)
	ctx := context.Background()
	klines, err := client.getKlines(ctx, PlatformBinance, MarketKlineRequest{Symbol: "btc-usdt-swap", Interval: "1m", Limit: 2000})
	if err != nil {
		t.Fatalf("getKlines: %v", err)
	}
	if len(klines) != 1 || klines[0].Symbol != "BTCUSDT" || klines[0].ClosePrice != "105" || klines[0].TradeCount != 7 {
		t.Fatalf("unexpected kline: %#v", klines)
	}
	ticker, err := client.getTicker(ctx, PlatformBinance, MarketTickerRequest{Symbol: "BTCUSDT"})
	if err != nil || ticker.Price != "105" {
		t.Fatalf("getTicker: ticker=%#v err=%v", ticker, err)
	}
	trades, err := client.getRecentTrades(ctx, PlatformBinance, MarketTradeRequest{Symbol: "BTCUSDT", Limit: 1})
	if err != nil || len(trades) != 1 || trades[0].TradeID != "12" || !trades[0].IsBuyerMaker {
		t.Fatalf("getRecentTrades: trades=%#v err=%v", trades, err)
	}
	funding, err := client.getFundingRate(ctx, PlatformBinance, FundingRateRequest{Symbol: "BTCUSDT", Limit: 1})
	if err != nil || funding.LastRate != "0.0001" || funding.NextRate != "0.0002" {
		t.Fatalf("getFundingRate: funding=%#v err=%v", funding, err)
	}
}

func TestMarketDataClientDeepcoinKlinesAndTicker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deepcoin/market/candles":
			if got := r.URL.Query().Get("instId"); got != "BTC-USDT-SWAP" {
				t.Fatalf("instId = %s, want BTC-USDT-SWAP", got)
			}
			_, _ = w.Write([]byte(`{"code":"0","data":[[1710000000000,"100","110","90","105","2",1710000059999,"210"]]}`))
		case "/deepcoin/market/tickers":
			_, _ = w.Write([]byte(`{"code":"0","data":[{"instId":"BTC-USDT-SWAP","last":"105","ts":"1710000060000"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newMarketDataClient(server.Client(), server.URL, server.URL)
	klines, err := client.getKlines(context.Background(), PlatformDeepcoin, MarketKlineRequest{Symbol: "BTCUSDT"})
	if err != nil || len(klines) != 1 || klines[0].QuoteVolume != "210" {
		t.Fatalf("getKlines: klines=%#v err=%v", klines, err)
	}
	ticker, err := client.getTicker(context.Background(), PlatformDeepcoin, MarketTickerRequest{Symbol: "BTCUSDT"})
	if err != nil || ticker.Price != "105" || ticker.UpdateTime != 1710000060000 {
		t.Fatalf("getTicker: ticker=%#v err=%v", ticker, err)
	}
}

// DeepCoin 的 bar 走 OKX 口径：1h/1d 必须转成 1H/1D，否则多周期回填会被服务端判为非法周期。
func TestMarketDataClientDeepcoinBarInterval(t *testing.T) {
	var gotBars []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deepcoin/market/candles" {
			http.NotFound(w, r)
			return
		}
		gotBars = append(gotBars, r.URL.Query().Get("bar"))
		_, _ = w.Write([]byte(`{"code":"0","data":[[1710000000000,"100","110","90","105","2",1710000059999,"210"]]}`))
	}))
	defer server.Close()

	client := newMarketDataClient(server.Client(), server.URL, server.URL)
	for _, interval := range []string{"1m", "5m", "1h", "1d"} {
		if _, err := client.getKlines(context.Background(), PlatformDeepcoin, MarketKlineRequest{Symbol: "BTCUSDT", Interval: interval}); err != nil {
			t.Fatalf("getKlines(%s): %v", interval, err)
		}
	}
	want := []string{"1m", "5m", "1H", "1D"}
	for i := range want {
		if gotBars[i] != want[i] {
			t.Fatalf("bar[%d] = %q, want %q", i, gotBars[i], want[i])
		}
	}
}

func TestMarketDataClientRejectsUnsupportedPlatform(t *testing.T) {
	client := newMarketDataClient(nil, "https://example.invalid", "https://example.invalid")
	if got := marketDataPlatform(""); got != PlatformDeepcoin {
		t.Fatalf("empty platform = %q, want %q", got, PlatformDeepcoin)
	}
	if _, err := client.getKlines(context.Background(), "unsupported", MarketKlineRequest{Symbol: "BTCUSDT"}); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}
