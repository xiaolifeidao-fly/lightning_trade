package trade

import (
	"strings"
	"testing"

	argusTrade "argus_single/pkg/trade"
	tradeRepository "service/trade/repository"
)

// 批量回填的入参归一：复数字段优先，其次单数字段，最后默认值；顺带去重与大小写归一。
func TestNormalizeStringList(t *testing.T) {
	upper := func(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }

	cases := []struct {
		name      string
		list      []string
		single    string
		fallback  []string
		normalize func(string) string
		want      []string
	}{
		{"复数字段优先", []string{"btcusdt", "ethusdt"}, "solusdt", nil, upper, []string{"BTCUSDT", "ETHUSDT"}},
		{"回落单数字段", nil, " btcusdt ", nil, upper, []string{"BTCUSDT"}},
		{"回落默认值", nil, "", defaultBackfillIntervals, strings.TrimSpace, []string{"1m", "5m", "1h", "1d"}},
		{"去重与空项过滤", []string{"BTCUSDT", "btcusdt", "  "}, "", nil, upper, []string{"BTCUSDT"}},
		{"平台空值兜底币安", nil, "", []string{tradeRepository.KlinePlatformBinance}, tradeRepository.NormalizeKlinePlatform, []string{"binance"}},
		{"平台大小写归一", []string{"DeepCoin"}, "", nil, tradeRepository.NormalizeKlinePlatform, []string{"deepcoin"}},
		{"全空返回空列表", nil, "", nil, upper, []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeStringList(c.list, c.single, c.fallback, c.normalize)
			if len(got) != len(c.want) {
				t.Fatalf("长度不符: got=%v want=%v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("第%d项不符: got=%v want=%v", i, got, c.want)
				}
			}
		})
	}
}

// 入库模型必须带上平台代码，否则两个平台的同一根 K 线会撞唯一键互相覆盖。
func TestKlinesToModelsCarriesPlatform(t *testing.T) {
	rows := []argusTrade.MarketKline{{
		OpenTime: 1735689600000, CloseTime: 1735689659999,
		OpenPrice: "100.5", HighPrice: "101", LowPrice: "99.5", ClosePrice: "100",
		Volume: "12.5", QuoteVolume: "1250", TradeCount: 8,
	}}

	models, err := klinesToModels("DeepCoin", "btcusdt", "1m", rows)
	if err != nil {
		t.Fatalf("klinesToModels 失败: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("期望 1 条模型，实际 %d", len(models))
	}
	if models[0].PlatformCode != "deepcoin" {
		t.Fatalf("平台代码未归一: %q", models[0].PlatformCode)
	}
	if models[0].OpenPrice != 100.5 || models[0].ClosePrice != 100 || models[0].TradeCount != 8 {
		t.Fatalf("行情值映射不符: %+v", models[0])
	}

	// 平台留空时按币安兜底，与历史数据口径一致。
	models, err = klinesToModels("", "btcusdt", "1m", rows)
	if err != nil {
		t.Fatalf("klinesToModels 失败: %v", err)
	}
	if models[0].PlatformCode != tradeRepository.KlinePlatformBinance {
		t.Fatalf("空平台未兜底为币安: %q", models[0].PlatformCode)
	}
}
