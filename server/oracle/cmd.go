package main

import (
	"flag"
	"strings"
	"time"

	"oracle/initialization"
	"oracle/pkg/backfill"
)

func main() {
	// -backfill 模式：补跑历史长周期预测后退出；不传则进入常驻调度。
	backfillMode := flag.Bool("backfill", false, "运行历史预测回填后退出（不进入常驻调度）")
	coin := flag.String("coin", "", "回填币种，默认取配置首个(oracle.coins)")
	intervals := flag.String("intervals", "4h,12h,1d", "要补跑的预测周期，逗号分隔")
	sourceInterval := flag.String("source-interval", "1h", "时间点来源周期(取其发起时刻作锚点)")
	lookbackHours := flag.Int("lookback-hours", 72, "回看小时数")
	delayMs := flag.Int("delay-ms", 200, "每次 LLM 调用之间的间隔(毫秒)，缓解限流")
	dryRun := flag.Bool("dry-run", false, "只切片+打印，不调用 LLM、不落库")
	flag.Parse()

	if *backfillMode {
		initialization.RunBackfill(backfill.Options{
			Coin:           *coin,
			Intervals:      splitCSV(*intervals),
			SourceInterval: *sourceInterval,
			Lookback:       time.Duration(*lookbackHours) * time.Hour,
			DelayPerCall:   time.Duration(*delayMs) * time.Millisecond,
			DryRun:         *dryRun,
		})
		return
	}

	initialization.Init()
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
