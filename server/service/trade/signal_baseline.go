package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	argusConfig "service/argus_config"
	argusDTO "service/argus_config/dto"
	"service/trade/strategy/signal"
)

// 本文件解决参数组批量扫描（r11）的一个前置问题：**基线是谁**。
//
// 任务口径是"基线默认取所选实例当前生产参数"。这件事必须显式做，不能拿
// signal.DefaultParams() 糊过去——生产缺省与三个实例的实际参数差得很远
// （需求大纲 §4.3：实例1 cap 15 / 实例2 cap 26+8 / 实例3 cap 246，S 300 vs 400，
// 趋势闸有的开有的关）。用缺省当基线，"改一个参数看差异"会变成"同时改了十几个
// 参数看差异"，diff 直接失去意义。
//
// 数据来源是 argus_config 的**已发布版本**快照（service/argus_config 的
// GetPublished），而不是 properties 文件：管理端能读到的只有 DB，且参数热更新
// 链路（r5/r7）以 DB 为唯一来源。
//
// ⚠️ 配置面收敛（r5）尚未完成，需求大纲 §3.2 列出的 C 类字段
// （risk_equity / order_size 账户级 / reverse_gate_min_profit_pct / contract_face /
// trend_gate.*）**DB 里根本没有列**。这些字段只能按实盘的解析口径兜底，并把每一处
// 兜底写进 BaselineNote 落库——否则页面上会出现"基线趋势闸=关闭"这种看起来是
// 事实、其实是缺列的读数，而实例2/3 生产上明明开着 24h/5%。

// 基线来源标记，落在 trade_backtest_batch.baseline_source 上。
const (
	// BaselineSourceInstancePublished 基线取自所选实例 argus_config 的已发布版本。
	BaselineSourceInstancePublished = "instance_published"
	// BaselineSourceRequest 基线由请求体显式给出（配置还没导入 DB 时的逃生口）。
	BaselineSourceRequest = "request"
)

// SignalBaseline 一次批量扫描的基线：参数 + 每个字段从哪来。
type SignalBaseline struct {
	Params signal.Params
	Source string
	// Notes 逐条说明"哪些字段不是从 DB 取到的、用了什么兜底"。它必须随批次落库
	// 并随对比结果回给前端：基线不可信时，与基线的 diff 也不可信。
	Notes []string
	// FromDB 实际从 DB 取到值的配置键，供页面标出"这些是真的生产值"。
	FromDB []string
}

// Note 把 Notes 拼成一行，供 varchar 列落库（口径同 signal.Fidelity.Note）。
func (b SignalBaseline) Note() string { return strings.Join(b.Notes, " | ") }

// ResolveInstanceBaseline 读所选实例当前已发布的生产配置，映射成回测参数基线。
//
// accountLabel 必须精确命中 argus_account.account_name——它与
// strategy_event.account_label 同源（都是 properties 的 trade.accountN.name，
// 见 eventstore/convert.go 的 AccountLabel: e.Account）。命中不了直接报错而不是
// 退到第一个账户：实例2 有 champion(cap26) 与 challenger(cap8) 两个账户，
// 猜错账户会让整批扫描的基线错一倍仓位，且没有任何外部现象能提示这件事。
func (s *TradeService) ResolveInstanceBaseline(ctx context.Context, instanceKey, accountLabel, symbol string) (*SignalBaseline, error) {
	instanceKey = strings.TrimSpace(instanceKey)
	accountLabel = strings.TrimSpace(accountLabel)
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if instanceKey == "" || accountLabel == "" {
		return nil, fmt.Errorf("instanceKey 与 accountLabel 都必填才能定位基线参数")
	}
	snapshot, err := argusConfig.NewArgusConfigService().GetPublished(ctx, instanceKey)
	if err != nil {
		return nil, fmt.Errorf("读实例 %s 的已发布配置失败: %w", instanceKey, err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("实例 %s 还没有已发布的配置版本：先在参数页发布一次，或在请求里显式给 baselineParams", instanceKey)
	}
	return baselineFromSnapshot(snapshot, accountLabel, symbol)
}

// baselineFromSnapshot 纯函数：已发布快照 → 基线参数。抽出来是为了能不连库单测
// 全部映射与兜底分支。
func baselineFromSnapshot(snapshot *argusDTO.ConfigSnapshotDTO, accountLabel, symbol string) (*SignalBaseline, error) {
	account, err := matchAccount(snapshot.Accounts, accountLabel)
	if err != nil {
		return nil, err
	}
	var risk *argusDTO.AccountRiskDTO
	for i := range snapshot.AccountRisks {
		if snapshot.AccountRisks[i].AccountID == account.ID {
			risk = &snapshot.AccountRisks[i]
			break
		}
	}
	if risk == nil {
		return nil, fmt.Errorf("账户 %s 在已发布版本 v%d 里没有风险配置行（argus_account_risk），基线无法成立",
			account.AccountName, snapshot.Version.Version)
	}

	b := &SignalBaseline{Params: signal.DefaultParams(), Source: BaselineSourceInstancePublished}
	b.Notes = append(b.Notes, fmt.Sprintf("基线取自实例 %s 的已发布配置 v%d（账户 %s）",
		snapshot.InstanceKey, snapshot.Version.Version, account.AccountName))

	// ① 有 DB 列、能直接取到的字段。
	if risk.RiskBudget > 0 {
		b.Params.BudgetPct = risk.RiskBudget
		b.FromDB = append(b.FromDB, "position.risk.budget_pct")
	} else {
		b.Notes = append(b.Notes, fmt.Sprintf("budget_pct 在 DB 里为 0，按生产缺省 %.1f%% 兜底", signal.DefaultBudgetPct))
	}
	if risk.CatastrophicStopLoss > 0 {
		b.Params.CatastropheStopPct = risk.CatastrophicStopLoss
		b.FromDB = append(b.FromDB, "position.monitor.catastrophe_stop_pct")
	} else {
		b.Notes = append(b.Notes, fmt.Sprintf("catastrophe_stop_pct 在 DB 里为 0，按生产缺省 %.0f 兜底", signal.DefaultCatastropheStopPct))
	}
	if risk.MaxContracts > 0 {
		b.Params.Ceiling = risk.MaxContracts
		b.FromDB = append(b.FromDB, "position.risk.max_contracts_ceiling")
	} else {
		b.Notes = append(b.Notes, fmt.Sprintf("max_contracts_ceiling 在 DB 里为 0，按生产缺省 %d 兜底", signal.DefaultCeiling))
	}
	applyTrailTiers(b, risk.TrailingStopTiersJSON)

	// order_size：DB 只有全局 argus_config.default_order_size，账户级
	// trade.accountN.order_size 还没有列（需求大纲 §3.2 C）。全局值与实例3 的
	// order_size=10 恰好一致、与实例1/2 的 1 也一致，但这是巧合不是保证。
	if snapshot.Config.DefaultOrderSize > 0 {
		b.Params.OrderSize = snapshot.Config.DefaultOrderSize
		b.FromDB = append(b.FromDB, "trade.order_size(全局)")
		b.Notes = append(b.Notes,
			"order_size 取的是全局 argus_config.default_order_size；账户级 trade.accountN.order_size 尚无 DB 列（r5 待收敛），多账户实例上可能与该账户实际值不同")
	}

	// risk_equity：制度性护栏是"只能来自配置"，DB 尚无列，实盘的解析口径是
	// trade.accountN.risk_equity 缺省回落 InitialBalance（ResolveRiskEquity），
	// 这里照同一个口径用 initial_balance。
	if account.InitialBalance > 0 {
		b.Params.RiskEquity = account.InitialBalance
		b.FromDB = append(b.FromDB, "argus_account.initial_balance")
		b.Notes = append(b.Notes,
			"risk_equity 尚无 DB 列（r5 待收敛），按实盘 ResolveRiskEquity 的口径回落 InitialBalance")
	} else {
		b.Notes = append(b.Notes,
			"⚠️ 该账户 initial_balance 为 0 且 risk_equity 无 DB 列：cap 公式无本金输入，基线必须在请求里显式给 riskEquity 或 capOverride")
	}

	// signal_threshold：DB 存的是小数（0.0005），引擎按 bp 表达。
	if th := matchSymbolThreshold(snapshot.MonitorSymbols, symbol); th > 0 {
		bp := th * 10000
		b.Params.BaselineThresholdBp = bp
		b.Params.SignalThresholdBp = bp // 基线自身不改阈值 ⇒ 保持事件级精度
		b.FromDB = append(b.FromDB, fmt.Sprintf("monitor.symbols.%s.signal_threshold", symbol))
	} else {
		b.Notes = append(b.Notes, fmt.Sprintf(
			"未在已发布版本里找到 %s 的 signal_threshold：频率级扫描需要 θ0，请在组参数里显式给 baselineThresholdBp", symbol))
	}

	// ② DB 完全没有列、只能按实盘缺省兜底的字段（需求大纲 §3.2 C 类）。
	//    每一条都要写进 Notes，否则页面上的"关闭"会被当成生产事实。
	b.Notes = append(b.Notes, fmt.Sprintf(
		"reverse_gate_min_profit_pct 尚无 DB 列（r5 待收敛），按实盘缺省 %.0f%% 兜底", signal.DefaultGateMinProfitPct))
	if risk.ReverseGateEnabled == 0 {
		// 引擎的净仓形态（ModeNet）总带反向门控，没有开关；生产上关掉门控时
		// 反向单会直接减仓。这个差异必须说出来而不是让 diff 里凭空多一道门控。
		b.Notes = append(b.Notes,
			"⚠️ 该账户生产上 reverse_gate=off，而回测的净仓形态恒带反向门控：基线在这一点上比生产更严，反向减仓类差异不可直接外推到实盘")
	}
	b.Notes = append(b.Notes,
		"trend_gate.window_hours / threshold_pct 尚无 DB 列（r5 待收敛），基线按关闭处理；实例2/3 生产上是 24h/5%，需要对齐时在组参数里显式给")
	b.Notes = append(b.Notes, fmt.Sprintf(
		"position.risk.contract_face 尚无 DB 列，按 BTC 合约面值 %.3f 兜底", signal.DefaultFaceValue))

	b.Params = b.Params.Normalize()
	sort.Strings(b.FromDB)
	return b, nil
}

// matchAccount 按 account_label 精确匹配账户（去空白、忽略大小写）。
// 匹配不上时把候选列表放进错误里——运维要的是"我该填哪个"，不是"没找到"。
func matchAccount(accounts []argusDTO.AccountDTO, accountLabel string) (*argusDTO.AccountDTO, error) {
	names := make([]string, 0, len(accounts))
	for i := range accounts {
		names = append(names, accounts[i].AccountName)
		if strings.EqualFold(strings.TrimSpace(accounts[i].AccountName), accountLabel) {
			return &accounts[i], nil
		}
	}
	return nil, fmt.Errorf("已发布配置里没有账户 %q（可选：%s）；accountLabel 必须与 argus_account.account_name 完全一致",
		accountLabel, strings.Join(names, " / "))
}

// matchSymbolThreshold 取该合约的 signal_threshold（小数口径）。
// 先按 symbol 列匹配，再退到 trade_instrument / deep_instrument——三个字段在
// 不同实例上填法不完全一致（BTCUSDT vs BTC-USDT-SWAP）。
func matchSymbolThreshold(symbols []argusDTO.MonitorSymbolDTO, symbol string) float64 {
	if symbol == "" {
		return 0
	}
	for _, s := range symbols {
		if s.Enabled == 0 {
			continue
		}
		for _, candidate := range []string{s.Symbol, s.TradeInstrument, s.DeepInstrument} {
			if strings.EqualFold(strings.TrimSpace(candidate), symbol) {
				return s.SignalThreshold
			}
		}
	}
	return 0
}

// applyTrailTiers 解析 argus_account_risk.trailing_stop_tiers_json。
// 键名是 properties 里 position.monitor.trail.* 去掉前缀后的原样 key
// （见 argus_config/importer.go importRisk 的组装），所以这里按同一套 key 读。
func applyTrailTiers(b *SignalBaseline, raw string) {
	if strings.TrimSpace(raw) == "" {
		b.Notes = append(b.Notes, "trailing_stop_tiers_json 为空，移动止盈分档按生产缺省兜底")
		return
	}
	tiers := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		b.Notes = append(b.Notes, "trailing_stop_tiers_json 解析失败，移动止盈分档按生产缺省兜底: "+err.Error())
		return
	}
	set := func(key string, target *float64) {
		if v, ok := tiers[key]; ok && v > 0 {
			*target = v
			b.FromDB = append(b.FromDB, "position.monitor.trail."+key)
		}
	}
	set("small_activate", &b.Params.SmallActivatePct)
	set("small_giveback", &b.Params.SmallGiveback)
	set("medium_activate", &b.Params.MediumActivatePct)
	set("medium_giveback", &b.Params.MediumGiveback)
	set("large_activate", &b.Params.LargeActivatePct)
	set("large_giveback", &b.Params.LargeGiveback)
	set("tier_small_ratio", &b.Params.TierSmallRatio)
	set("tier_large_ratio", &b.Params.TierLargeRatio)
	if len(tiers) == 0 {
		b.Notes = append(b.Notes, "trailing_stop_tiers_json 里没有任何分档键，移动止盈按生产缺省兜底")
	}
}
