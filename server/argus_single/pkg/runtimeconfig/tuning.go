package runtimeconfig

import (
	"encoding/json"
	"strings"
	"sync"

	"argus_single/pkg/trade"
	"common/middleware/vipper"
	"service/argus_config/repository"

	"github.com/sirupsen/logrus"
)

// properties 里那些既没有走 AccFloat、又被 pkg/monitor / pkg/trade 直接
// vipper.GetX 读取的全局标量键。它们的生效方式只能是把 DB 值写回 vipper。
const (
	keyMonitorIntervalSeconds = "position.monitor.interval_seconds"
	keyMonitorProfitThreshold = "position.monitor.profit_threshold"
	keyMonitorLossThreshold   = "position.monitor.loss_threshold"
	keyContractFace           = "position.risk.contract_face"
	keySignalDelaySeconds     = "trade.signal.delay_seconds"
	keySpreadMaxPriceAgeMs    = "monitor.spread.max_price_age_ms"
	keyTrendGateWindowHours   = "trade.trend_gate.window_hours"
	// 下面两个是 AccFloat 的全局键，走覆盖层而不是 vipper.Set。
	keyTrendGateThresholdPct   = "trade.trend_gate.threshold_pct"
	keyReverseGateMinProfitPct = "position.risk.reverse_gate_min_profit_pct"
)

// RuntimeTuning 是配置快照里的全局标量参数。
//
// 参数收敛分两条通道，取决于运行时怎么读它：
//   - 账户级参数与 AccFloat 的全局键 → trade.ParamOverrides 覆盖层，每次热加载
//     整体重建，取消配置能干净地退回 properties；
//   - 直接 vipper.GetX 的全局键 → 本结构体 + vipper.Set 写回。
//
// 字段为 0 一律表示「DB 未配置」，此时保留 properties 原值（首次写回前抓下来
// 的快照），不会把结构体零值当成显式配置压下去。
type RuntimeTuning struct {
	MonitorIntervalSecond int
	ProfitThreshold       float64
	LossThreshold         float64
	ContractFace          float64
	SignalDelaySecond     int
	SpreadMaxPriceAgeMs   int
	TrendGateWindowHour   float64
}

var (
	propertyFallbackMu sync.Mutex
	propertyFallback   = map[string]float64{}
)

// setTuningValue 把 DB 值写回 vipper。value <= 0 表示未配置，回填进程启动时
// properties 的原值——覆盖层可以整体重建，vipper.Set 不能撤销，只能自己记住兜底值。
func setTuningValue(key string, value float64) {
	propertyFallbackMu.Lock()
	fallback, ok := propertyFallback[key]
	if !ok {
		fallback = vipper.GetFloat64(key)
		propertyFallback[key] = fallback
	}
	propertyFallbackMu.Unlock()
	if value > 0 {
		vipper.Set(key, value)
		return
	}
	vipper.Set(key, fallback)
}

// applyTuning 把全局标量参数写回 vipper。必须早于 trade.ReplaceManager 与
// monitor.ReloadMonitor：这两者在构造时就会把值读走。
func applyTuning(tuning RuntimeTuning) {
	setTuningValue(keyMonitorIntervalSeconds, float64(tuning.MonitorIntervalSecond))
	setTuningValue(keyMonitorProfitThreshold, tuning.ProfitThreshold)
	setTuningValue(keyMonitorLossThreshold, tuning.LossThreshold)
	setTuningValue(keyContractFace, tuning.ContractFace)
	setTuningValue(keySignalDelaySeconds, float64(tuning.SignalDelaySecond))
	setTuningValue(keySpreadMaxPriceAgeMs, float64(tuning.SpreadMaxPriceAgeMs))
	setTuningValue(keyTrendGateWindowHours, tuning.TrendGateWindowHour)
}

func snapshotTuning(config repository.ArgusConfig) RuntimeTuning {
	return RuntimeTuning{
		MonitorIntervalSecond: config.MonitorIntervalSecond,
		ProfitThreshold:       config.ProfitThreshold,
		LossThreshold:         config.LossThreshold,
		ContractFace:          config.ContractFace,
		SignalDelaySecond:     config.SignalDelaySecond,
		SpreadMaxPriceAgeMs:   config.SpreadMaxPriceAgeMs,
		TrendGateWindowHour:   config.TrendGateWindowHour,
	}
}

// globalParamOverrides 收集 AccFloat 全局键的 DB 覆盖。
func globalParamOverrides(config repository.ArgusConfig, overrides *trade.ParamOverrides) {
	if config.TrendGateThresholdPct > 0 {
		overrides.SetGlobal(keyTrendGateThresholdPct, config.TrendGateThresholdPct)
	}
	if config.ReverseGateMinProfitPct > 0 {
		overrides.SetGlobal(keyReverseGateMinProfitPct, config.ReverseGateMinProfitPct)
	}
}

// accountParamOverrides 收集某账户的 DB 参数覆盖。
//
// trailing_stop_tiers_json 从 r1 起就在写，但 runtimeAccount 从来没读过：8 个
// 移动止盈档位一直是 properties 说了算，配置面改了没有任何效果。这里把它连同
// 其余账户级列一起接进覆盖层，键名与 AccFloat 的参数名一一对应。
func accountParamOverrides(index int, risk *repository.ArgusAccountRisk, accountName string, overrides *trade.ParamOverrides) {
	if risk == nil {
		return
	}
	if trimmed := strings.TrimSpace(risk.TrailingStopTiersJSON); trimmed != "" && trimmed != "null" {
		tiers := map[string]float64{}
		if err := json.Unmarshal([]byte(trimmed), &tiers); err != nil {
			logrus.Warnf("账户 %s 的 trailing_stop_tiers_json 解析失败，移动止盈档位回退 properties: %v", accountName, err)
		}
		for name, value := range tiers {
			overrides.SetAccount(index, name, value)
		}
	}
	if risk.RiskBudget > 0 {
		overrides.SetAccount(index, "budget_pct", risk.RiskBudget)
	}
	if risk.CatastrophicStopLoss > 0 {
		overrides.SetAccount(index, "catastrophe_stop_pct", risk.CatastrophicStopLoss)
	}
	if risk.MaxContracts > 0 {
		overrides.SetAccount(index, "max_contracts_ceiling", float64(risk.MaxContracts))
	}
	if risk.RiskEquity > 0 {
		overrides.SetAccount(index, "risk_equity", risk.RiskEquity)
	}
	if risk.ReverseGateMinProfitPct > 0 {
		overrides.SetAccount(index, "reverse_gate_min_profit_pct", risk.ReverseGateMinProfitPct)
	}
	if risk.TrendGateThresholdPct > 0 {
		overrides.SetAccount(index, "trend_gate_threshold_pct", risk.TrendGateThresholdPct)
	}
}

// accountExtraRisk 是 argus_account_risk.extra_risk_json 的结构。
// 这三项决定走哪套策略与事件归因，读不到就等于策略静默换人。
type accountExtraRisk struct {
	TradeDirection string `json:"tradeDirection"`
	TradeLogic     string `json:"tradeLogic"`
	Variant        string `json:"variant"`
}

func parseExtraRisk(raw, accountName string) (accountExtraRisk, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		// 空值不算错，但必须留痕：TradeLogic 会被归一化成 spread，
		// 盘口信号策略就此静默失效，运维只能靠这行日志发现。
		logrus.Warnf("账户 %s 的 extra_risk_json 为空，trade_logic/variant/trade_direction 将回退为默认值", accountName)
		return accountExtraRisk{}, nil
	}
	var extra accountExtraRisk
	if err := json.Unmarshal([]byte(trimmed), &extra); err != nil {
		return accountExtraRisk{}, err
	}
	return extra, nil
}
