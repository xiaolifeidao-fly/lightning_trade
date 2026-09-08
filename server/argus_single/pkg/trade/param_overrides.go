package trade

import "sync"

// ParamOverrides 是配置快照（DB）下发的参数覆盖层。
//
// 收敛前，账户级风险参数只能从 properties 读，配置面改了 DB 也不生效；收敛后
// DB 成为唯一真源，properties 退化成兜底。覆盖层的语义是「这个键在 DB 里被显式
// 配置过」——只有出现在这里的键才会遮蔽 properties，缺失的键仍走原来的
// 账户键 → 全局键 → 默认值链路，因此老部署不改配置也不会行为漂移。
//
// Accounts 的 key 是账户序号（AccountConfig.Index，1-based），内层 key 是
// AccFloat 的参数名（如 small_activate）；Global 的 key 是 AccFloat 的全局键
// （如 position.risk.reverse_gate_min_profit_pct）。
type ParamOverrides struct {
	Global   map[string]float64
	Accounts map[int]map[string]float64
}

// SetGlobal 记录一个全局键覆盖。
func (o *ParamOverrides) SetGlobal(globalKey string, value float64) {
	if globalKey == "" {
		return
	}
	if o.Global == nil {
		o.Global = make(map[string]float64)
	}
	o.Global[globalKey] = value
}

// SetAccount 记录一个账户级参数覆盖。
func (o *ParamOverrides) SetAccount(index int, name string, value float64) {
	if index <= 0 || name == "" {
		return
	}
	if o.Accounts == nil {
		o.Accounts = make(map[int]map[string]float64)
	}
	if o.Accounts[index] == nil {
		o.Accounts[index] = make(map[string]float64)
	}
	o.Accounts[index][name] = value
}

func (o ParamOverrides) clone() ParamOverrides {
	result := ParamOverrides{}
	for key, value := range o.Global {
		result.SetGlobal(key, value)
	}
	for index, values := range o.Accounts {
		for name, value := range values {
			result.SetAccount(index, name, value)
		}
	}
	return result
}

var (
	paramOverridesMu sync.RWMutex
	paramOverrides   ParamOverrides
)

// SetParamOverrides 安装配置快照的参数覆盖层。热加载时先于 ReplaceManager 调用，
// 否则新 TradeManager 会用旧参数解析 cap。传空值即清空覆盖、全部回退 properties。
func SetParamOverrides(overrides ParamOverrides) {
	snapshot := overrides.clone()
	paramOverridesMu.Lock()
	paramOverrides = snapshot
	paramOverridesMu.Unlock()
}

// lookupParamOverride 按「账户级 → 全局键」查覆盖层，都没有则交回 properties。
func lookupParamOverride(index int, name, globalKey string) (float64, bool) {
	paramOverridesMu.RLock()
	defer paramOverridesMu.RUnlock()
	if values, ok := paramOverrides.Accounts[index]; ok {
		if value, ok := values[name]; ok {
			return value, true
		}
	}
	if globalKey != "" {
		if value, ok := paramOverrides.Global[globalKey]; ok {
			return value, true
		}
	}
	return 0, false
}
