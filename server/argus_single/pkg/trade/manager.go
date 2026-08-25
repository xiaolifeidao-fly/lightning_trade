package trade

import (
	"argus_single/pkg/eventlog"
	"common/middleware/vipper"
	"common/utils"
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

// SignalLeverage 盘口信号开仓使用的杠杆（与仓位上限公式中的 L 保持一致）。
const SignalLeverage = 125

type TradeManager struct {
	config                        *TradingSystemConfig
	clients                       map[string]*utils.DeepCoinClient
	webClients                    map[string]*DirectWebClient
	mu                            sync.RWMutex
	lastTrade                     time.Time
	tradeCooldown                 time.Duration
	stopShuffle                   chan struct{} // 用于停止shuffle goroutine
	stopOnce                      sync.Once
	telegramClient                *utils.TelegramClient
	loginScheduler                *LoginScheduler
	capGuard                      *PositionCapGuard  // 仓位上限守卫（仅 trailing 账户使用）
	reverseGateMinProfitByAccount map[string]float64 // 账户级反向减仓最小盈利 ROI%
	riskEquityByAccount           map[string]float64 // 风险基数（cap 公式唯一输入, P5 三拆）
}

func (tm *TradeManager) Config() *TradingSystemConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.config == nil {
		return nil
	}
	copy := *tm.config
	copy.Accounts = append([]AccountConfig(nil), tm.config.Accounts...)
	return &copy
}

func NewTradeManager(config *TradingSystemConfig) *TradeManager {
	// 账户级参数解析（champion/challenger）
	capByAccount := make(map[string]CapParams)
	gateMinByAccount := make(map[string]float64)
	riskEquityByAccount := make(map[string]float64)
	for _, acc := range config.Accounts {
		capByAccount[acc.Name] = resolveCapParams(acc)
		gateMinByAccount[acc.Name] = resolveReverseGateMinProfit(acc)
		riskEquityByAccount[acc.Name] = ResolveRiskEquity(acc)
		// P5：启动校验（fail-fast）+ 静态参数打印（动态行在 cap guard 首次初始化时打）
		if acc.IsTrailingTP() {
			view := resolveRiskParamsView(acc, config.Trade.OrderSize)
			if err := ValidateRiskParams(view); err != nil {
				logrus.Fatalf("[风险参数] 账户 %s 配置非法, 拒绝启动: %v", acc.Name, err)
			}
			logStaticRiskParams(acc, view)
		}
	}

	tm := &TradeManager{
		config:                        config,
		clients:                       make(map[string]*utils.DeepCoinClient),
		webClients:                    make(map[string]*DirectWebClient),
		tradeCooldown:                 5 * time.Second,
		stopShuffle:                   make(chan struct{}),
		telegramClient:                utils.NewTelegramClientWithBotTokenAndChatID(vipper.GetString("telegram.bot_token"), vipper.GetString("telegram.chat_id")),
		capGuard:                      NewPositionCapGuard(loadCapParams(), capByAccount),
		reverseGateMinProfitByAccount: gateMinByAccount,
		riskEquityByAccount:           riskEquityByAccount,
	}

	// 为每个账户创建客户端
	for _, acc := range config.Accounts {
		// 创建直连 DeepCoin API 客户端
		client := utils.NewDeepCoinClient(acc.APIKey, acc.SecretKey, acc.Passphrase)
		tm.clients[acc.Name] = client
		logrus.Infof("✅ 账户 %s 直连API客户端已创建", acc.Name)

		// 如果有 Web 配置种子，创建直连 Web 客户端。
		// userProvider 负责在交易时解析 cookie/token：config 模式直读静态凭证，
		// password 模式走 session 检测 + pl-instance 无头登录。
		if acc.HasWebCredentialSeed() {
			userProvider, err := BuildUserProvider(acc)
			if err != nil {
				logrus.Warnf("⚠️  账户 %s Web 凭证提供器创建失败: %v", acc.Name, err)
				continue
			}

			webClient := NewDirectWebClient(userProvider)
			tm.webClients[acc.Name] = webClient
			logrus.Infof("✅ 账户 %s 直连Web客户端已创建(mode=%s)", acc.Name, acc.LoginType)
		}
	}

	// 启动定时打乱账户position_side的goroutine
	go tm.startPositionSideShuffle()

	// 启动定时登录调度器（仅当配置启用且存在密码型账户时生效）
	if vipper.GetBool("login.scheduled.enabled") {
		hour := vipper.GetInt("login.scheduled.hour")
		minute := vipper.GetInt("login.scheduled.minute")
		tm.loginScheduler = newLoginScheduler(tm, hour, minute)
		go tm.loginScheduler.Start()
		logrus.Infof("⏰ 定时登录调度器已启动，每天 %02d:%02d 触发", hour, minute)
	}

	return tm
}

// loadCapParams 从配置读取仓位上限公式参数（带默认值）。
func loadCapParams() CapParams {
	budgetPct := vipper.GetFloat64("position.risk.budget_pct")
	if budgetPct <= 0 {
		budgetPct = 20
	}
	stopPct := vipper.GetFloat64("position.monitor.catastrophe_stop_pct")
	if stopPct <= 0 {
		stopPct = 300
	}
	ceiling := vipper.GetInt("position.risk.max_contracts_ceiling")
	if ceiling <= 0 {
		ceiling = 20
	}
	face := vipper.GetFloat64("position.risk.contract_face")
	if face <= 0 {
		face = 0.001 // BTC 1张=0.001
	}
	return CapParams{
		Leverage:           SignalLeverage,
		FaceValue:          face,
		RiskBudgetFraction: budgetPct / 100,
		CatastropheStopPct: stopPct,
		Ceiling:            ceiling,
	}
}

// CapGuard 返回仓位上限守卫（供 monitor 按 N_max 定档使用）。
func (tm *TradeManager) CapGuard() *PositionCapGuard {
	return tm.capGuard
}

// capParamsFor 该账户的 cap 参数；guard 缺失时回退默认合约面值（防御，P2-D 用）。
func (tm *TradeManager) capParamsFor(account string) CapParams {
	if tm.capGuard != nil {
		return tm.capGuard.paramsFor(account)
	}
	return CapParams{FaceValue: 0.001}
}

// instrumentBase 提取合约的基础币，用于归一化匹配 trade_inst 与 swap inst。
// 例：BTCUSDT -> BTC，BTC-USDT-SWAP -> BTC。
func instrumentBase(instId string) string {
	s := strings.ToUpper(strings.TrimSpace(instId))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimSuffix(s, "SWAP")
	s = strings.TrimSuffix(s, "USDT")
	return s
}

// parseNetPosition 从持仓列表中解析 instId 对应合约的净持仓（纯函数，可测）。
// 按基础币归一化匹配（BTCUSDT ↔ BTC-USDT-SWAP）。
// 张数/价格解析失败或价格 <=0 → ok=false（A2 fail-closed）；无匹配 → Size=0, ok=true（flat）。
func parseNetPosition(data []utils.PositionInfo, instId string) (NetPosition, bool) {
	base := instrumentBase(instId)
	for _, p := range data {
		if instrumentBase(p.InstId) != base {
			continue
		}
		n, perr := strconv.Atoi(strings.TrimSpace(p.Pos))
		if perr != nil {
			return NetPosition{}, false
		}
		if n < 0 {
			n = -n
		}
		if n == 0 {
			continue
		}
		avg, aerr := strconv.ParseFloat(strings.TrimSpace(p.AvgPx), 64)
		last, lerr := strconv.ParseFloat(strings.TrimSpace(p.LastPx), 64)
		if aerr != nil || lerr != nil || avg <= 0 || last <= 0 {
			return NetPosition{}, false // A2: 价格解析失败/异常 → fail-closed
		}
		return NetPosition{
			Side:   strings.ToLower(strings.TrimSpace(p.PosSide)),
			Size:   n,
			AvgPx:  avg,
			LastPx: last,
			PosId:  p.PosId,
		}, true
	}
	return NetPosition{Size: 0}, true // 无持仓（flat）
}

// EnsureSessionsReady 启动时主动检测所有密码型账户的 session 是否有效（net-wapi 接口）。
// 失效则立即触发无头模式重新登录。定时调度器复用同一套流程。
func (tm *TradeManager) EnsureSessionsReady() {
	tm.mu.RLock()
	snapshot := make(map[string]*DirectWebClient, len(tm.webClients))
	for k, v := range tm.webClients {
		snapshot[k] = v
	}
	tm.mu.RUnlock()

	if len(snapshot) == 0 {
		logrus.Info("[session] 无 Web 客户端账户，跳过启动检测")
		return
	}

	logrus.Infof("[session] 启动检测：共 %d 个账户，使用 net-wapi 接口校验 session", len(snapshot))
	for name, client := range snapshot {
		lp, ok := client.userProvider.(*LoginUserProvider)
		if !ok {
			logrus.Infof("[session] 账户 %s 为静态凭证模式，跳过检测", name)
			continue
		}

		// Invalidate 清除内存缓存，强制 GetUser 走完整检测+登录流程
		lp.Invalidate()

		ctx, cancel := context.WithTimeout(context.Background(), defaultLoginTimeout+30*time.Second)
		_, err := lp.GetUser(ctx)
		cancel()

		if err != nil {
			logrus.Errorf("[session] ❌ 账户 %s 启动检测/登录失败: %v", name, err)
		} else {
			logrus.Infof("[session] ✅ 账户 %s session 就绪", name)
		}
	}
}

// currentNetPosition 查询账户在 instId 对应合约上的当前净持仓（净仓模式）。
// 返回 ok=false 表示查询/解析失败/价格异常（调用方 fail-closed）；Size=0 表示无持仓。
func (tm *TradeManager) currentNetPosition(accName, instId string) (NetPosition, bool) {
	client := tm.clients[accName]
	if client == nil {
		return NetPosition{}, false
	}
	resp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{InstType: "SWAP"})
	if err != nil {
		return NetPosition{}, false
	}
	np, ok := parseNetPosition(resp.Data, instId)
	if !ok {
		logrus.Warnf("[持仓门控] 账户 %s %s 持仓解析失败/价格异常，fail-closed 跳过本单", accName, instId)
	}
	return np, ok
}

// getAccountOrderSize 获取指定账户的开仓张数（优先账户级 order_size，回退到全局 trade.order_size）
func (tm *TradeManager) getAccountOrderSize(acc AccountConfig) int {
	return acc.GetOrderSize(tm.config.Trade.OrderSize)
}

func (tm *TradeManager) getSpreadLogicAccounts() []AccountConfig {
	accounts := make([]AccountConfig, 0)
	for _, acc := range tm.config.Accounts {
		if acc.IsSpreadLogic() {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

func (tm *TradeManager) getSignalLogicAccounts() []AccountConfig {
	accounts := make([]AccountConfig, 0)
	for _, acc := range tm.config.Accounts {
		if acc.IsSignalLogic() {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

func (tm *TradeManager) HasSpreadLogicAccounts() bool {
	return len(tm.getSpreadLogicAccounts()) > 0
}

func (tm *TradeManager) HasSignalLogicAccounts() bool {
	return len(tm.getSignalLogicAccounts()) > 0
}

// OpenedAccount 记录开过仓的账户信息
type OpenedAccount struct {
	Account AccountConfig
	Size    int
	PosSide string                  // "long" or "short"
	WebResp *utils.WebOrderResponse // Web开仓响应（包含开仓均价和成交价）
}

// AccountInfo 账户详细信息（用于发送TG消息）
type AccountInfo struct {
	Name      string
	PosSide   string
	Size      int
	AvgPx     string
	LiqPx     string
	UseMargin string
	TpPrice   string
	SlPrice   string
	IsMain    bool
}

// OpenedAccountWithTPSL 开仓账户信息加止盈止损价格
type OpenedAccountWithTPSL struct {
	OpenedAccount
	TpPrice        string // 止盈价（用于计算的基准价）
	SlPrice        string // 止损价（用于计算的基准价）
	OpenPrice      string // 开仓均价
	TradePrice     string // 本次成交价
	TpTriggerPrice string // 止盈触发价
	TpOrderPrice   string // 止盈委托价
	SlTriggerPrice string // 止损触发价
	SlOrderPrice   string // 止损委托价
}

// executeArbitrageTrades_From_WEB 执行套利交易（使用Web接口）
// needBuyDeep: true=行情显示应开多（币安价>DeepCoin价），false=行情显示应开空
// 每个账户根据自身 trade_direction 决定跟随行情（forward）或反向对冲（reverse），
// 并使用自身 order_size 决定下单张数。
// 返回: 所有开仓成功的账号
func (tm *TradeManager) executeArbitrageTrades_From_WEB(instId string, price float64, needBuyDeep bool) []OpenedAccount {
	accounts := tm.getSpreadLogicAccounts()
	if len(accounts) == 0 {
		logrus.Debugf("没有配置价差老逻辑账户，跳过价差开仓")
		return nil
	}

	marketSide := "long"
	marketEmoji := "🔵"
	if !needBuyDeep {
		marketSide = "short"
		marketEmoji = "🔴"
	}

	logrus.Infof("%s [Web] 并发按账户配置开仓: %s, 价格=%.2f, 行情方向=%s, 账户数=%d",
		marketEmoji, instId, price, marketSide, len(accounts))

	type result struct {
		acc OpenedAccount
		err error
	}

	resultCh := make(chan result, len(accounts))

	lever := 125
	isCrossMargin := 1

	for _, acc := range accounts {
		acc := acc // capture
		accPosSide := acc.GetPosSide(needBuyDeep)
		accSize := tm.getAccountOrderSize(acc)
		accEmoji := "🔵"
		if accPosSide == "short" {
			accEmoji = "🔴"
		}
		dirTag := "正向"
		if acc.IsReverseDirection() {
			dirTag = "反向"
		}

		go func() {
			webClient := tm.webClients[acc.Name]
			if webClient == nil {
				logrus.Errorf("  ⚠️  %s 未配置Web客户端，跳过", acc.Name)
				resultCh <- result{err: fmt.Errorf("%s 未配置Web客户端", acc.Name)}
				return
			}

			var resp *utils.WebOrderResponse
			var err error
			if accPosSide == "long" {
				resp, err = webClient.MarketBuyLongWithRisk(instId, accSize, lever, isCrossMargin, acc.UID, price)
			} else {
				resp, err = webClient.MarketSellShortWithRisk(instId, accSize, lever, isCrossMargin, acc.UID, price)
			}

			if err != nil {
				logrus.Errorf("  ⚠️  %s [%s] 开%s失败: %v", acc.Name, dirTag, accPosSide, err)
				resultCh <- result{err: err}
				return
			}

			if resp.Code != 0 && resp.Code != 200 {
				logrus.Errorf("  ⚠️  %s [%s] 开%s失败: code=%d, msg=%s", acc.Name, dirTag, accPosSide, resp.Code, resp.Msg)
				resultCh <- result{err: fmt.Errorf("code=%d msg=%s", resp.Code, resp.Msg)}
				return
			}

			logrus.Infof("  ✅ %s %s [Web-%s] 开%s成功: 张数=%d, code=%d", accEmoji, acc.Name, dirTag, accPosSide, accSize, resp.Code)
			resultCh <- result{acc: OpenedAccount{
				Account: acc,
				Size:    accSize,
				PosSide: accPosSide,
				WebResp: resp,
			}}
		}()
	}

	opened := make([]OpenedAccount, 0, len(accounts))
	for range accounts {
		r := <-resultCh
		if r.err == nil {
			opened = append(opened, r.acc)
		}
	}
	return opened
}

// ExecuteArbitrage_From_WEB 执行套利交易（使用Web接口）
// 遍历所有账户，按账户自身 trade_direction 决定跟随或反向行情，按 order_size 决定张数
func (tm *TradeManager) ExecuteArbitrage_From_WEB(instId string, binPrice, deepPrice float64) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查冷却时间
	if time.Since(tm.lastTrade) < tm.tradeCooldown {
		logrus.Debugf("交易冷却中，距上次: %.1fs", time.Since(tm.lastTrade).Seconds())
		return nil
	}

	needBuyDeep := binPrice > deepPrice // 币安价格高，行情方向为开多

	openedAccounts := tm.executeArbitrageTrades_From_WEB(instId, deepPrice, needBuyDeep)

	// 记录交易时间
	tm.lastTrade = time.Now()

	if len(openedAccounts) > 0 {
		accountsCopy := make([]OpenedAccount, len(openedAccounts))
		copy(accountsCopy, openedAccounts)
		go tm.sendOpenSummaryToTelegram(instId, binPrice, deepPrice, accountsCopy)
	}

	return nil
}

func normalizeOrderBookSignal(direction string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case OrderBookSignalUp:
		return OrderBookSignalUp, nil
	case OrderBookSignalDown:
		return OrderBookSignalDown, nil
	default:
		return "", fmt.Errorf("未知盘口信号方向: %s", direction)
	}
}

func signalPosSide(direction string) string {
	if direction == OrderBookSignalUp {
		return "long"
	}
	return "short"
}

func signalSideEmoji(direction string) string {
	if direction == OrderBookSignalUp {
		return "🔵"
	}
	return "🔴"
}

// ExecuteSignalTrade_From_WEB 执行盘口信号交易（使用Web接口）。
// UP 固定开多，DOWN 固定开空；只作用于 trade_logic=signal 的账户。
// ExecuteSignalTrade_From_WEB 执行盘口信号开仓。
// 返回 opened=true 仅当至少一个账户真正开仓（全跳过/全失败均为 false），供调度器据此判定是否进入 POSITION 状态。
func (tm *TradeManager) ExecuteSignalTrade_From_WEB(instId string, price float64, direction string, q SignalQuote) (opened bool, err error) {
	direction, err = normalizeOrderBookSignal(direction)
	if err != nil {
		return false, err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	accounts := tm.getSignalLogicAccounts()
	if len(accounts) == 0 {
		logrus.Debugf("没有配置盘口信号逻辑账户，跳过信号交易")
		return false, nil
	}

	posSide := signalPosSide(direction)
	logrus.Infof("%s [盘口信号] 执行%s: %s, 价格=%.2f, signal账户数=%d",
		signalSideEmoji(direction), posSide, instId, price, len(accounts))

	openedAccounts, failures, skipped := tm.executeSignalTrades_From_WEB(accounts, instId, price, direction, q)
	tm.lastTrade = time.Now()

	if len(openedAccounts) > 0 || len(failures) > 0 || len(skipped) > 0 {
		accountsCopy := make([]OpenedAccount, len(openedAccounts))
		copy(accountsCopy, openedAccounts)
		failuresCopy := append([]string(nil), failures...)
		skippedCopy := append([]string(nil), skipped...)
		go tm.sendSignalSummaryToTelegram(instId, price, direction, accountsCopy, failuresCopy, skippedCopy)
	}

	// opened 仅当确有账户开仓为真；全跳过/全失败为 false（调度器据此不进入 POSITION）
	opened = len(openedAccounts) > 0
	// 全部失败（无成功、无跳过）才算错误；全部被 cap/gate 跳过不算错误（正常生效）
	if !opened && len(failures) > 0 {
		return false, fmt.Errorf("盘口信号 %s 下单全部失败: %v", direction, failures)
	}
	return opened, nil
}

func (tm *TradeManager) executeSignalTrades_From_WEB(accounts []AccountConfig, instId string, price float64, direction string, q SignalQuote) ([]OpenedAccount, []string, []string) {
	type result struct {
		acc     OpenedAccount
		err     error
		skipped bool
		skipMsg string
	}

	resultCh := make(chan result, len(accounts))
	posSide := signalPosSide(direction)
	lever := SignalLeverage
	isCrossMargin := 1

	for _, acc := range accounts {
		acc := acc
		accSize := tm.getAccountOrderSize(acc)

		go func() {
			webClient := tm.webClients[acc.Name]
			if webClient == nil {
				err := fmt.Errorf("%s 未配置Web客户端", acc.Name)
				logrus.Errorf("  ⚠️  %v，跳过盘口信号下单", err)
				resultCh <- result{err: err}
				return
			}

			// 持仓门控：cap(加仓) / reverse_gate(反向减仓)，共用一次净仓查询；无法核验时 fail-closed
			var net NetPosition
			netOK := false
			if acc.IsTrailingTP() || acc.IsReverseGate() {
				net, netOK = tm.currentNetPosition(acc.Name, instId)
				if !netOK {
					logrus.Warnf("  ⚠️  %s [持仓门控] 查询净仓失败，fail-closed 跳过开%s", acc.Name, posSide)
					resultCh <- result{skipped: true, skipMsg: fmt.Sprintf("%s 净仓查询失败(fail-closed)", acc.Name)}
					return
				}
				if isReduction(posSide, net.Side, net.Size) {
					// 反向减仓 → reverse_gate（盈利且不翻转才放行）
					if acc.IsReverseGate() {
						dec := EvaluateReverseGate(net.Side, net.AvgPx, net.LastPx, SignalLeverage, tm.reverseGateMinProfitByAccount[acc.Name], accSize, net.Size)
						if !dec.Allow {
							logrus.Warnf("  🚦 %s [门控] 反向减仓被拦截: %s", acc.Name, dec.Reason)
							eventlog.Log(applySignalQuote(eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: instId, Event: eventlog.EvGateBlock,
								Side: posSide, NetSide: net.Side, Size: net.Size, AvgPx: net.AvgPx, LastPx: net.LastPx, RoiPct: dec.RoiPct, OrderSize: accSize, Reason: dec.Reason}, q))
							resultCh <- result{skipped: true, skipMsg: fmt.Sprintf("🚦门控 %s %s", acc.Name, dec.Reason)}
							return
						}
					}
				} else if acc.IsTrailingTP() && tm.capGuard != nil {
					// 全新开仓 / 加仓 → 仓位上限
					if tm.capGuard.WouldExceedCap(acc.Name, tm.riskEquityByAccount[acc.Name], net.Size, accSize, price) {
						nmax, _ := tm.capGuard.MaxContracts(acc.Name)
						logrus.Warnf("  ⛔ %s [仓位上限] 当前%d张 + %d张 > 上限%d，跳过开%s", acc.Name, net.Size, accSize, nmax, posSide)
						eventlog.Log(applySignalQuote(eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: instId, Event: eventlog.EvCapSkip,
							Side: posSide, Size: net.Size, OrderSize: accSize, Reason: fmt.Sprintf("当前%d+%d>上限%d", net.Size, accSize, nmax)}, q))
						resultCh <- result{skipped: true, skipMsg: fmt.Sprintf("⛔上限 %s 当前%d张+%d>上限%d", acc.Name, net.Size, accSize, nmax)}
						return
					}
				}
			}

			resp, err := tm.placeSignalWebOrderWithRetry(webClient, acc, instId, accSize, lever, isCrossMargin, price, posSide)
			if err != nil {
				logrus.Errorf("  ⚠️  %s [盘口信号] 开%s失败: %v", acc.Name, posSide, err)
				resultCh <- result{err: err}
				return
			}

			logrus.Infof("  ✅ %s %s [盘口信号] 开%s成功: 张数=%d, code=%d",
				signalSideEmoji(direction), acc.Name, posSide, accSize, resp.Code)
			// Size 记开仓后预计净仓张数（让"最大堆积"口径正确）；OrderSize 记本次下单张数
			postNet := accSize
			if netOK {
				if net.Size == 0 || strings.EqualFold(posSide, net.Side) {
					postNet = net.Size + accSize // 全新/加仓
				} else {
					postNet = net.Size - accSize // 反向减仓
					if postNet < 0 {
						postNet = 0
					}
				}
			}
			openEv := eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: instId, Event: eventlog.EvOpen, Side: posSide, Size: postNet, OrderSize: accSize}
			if netOK && isReduction(posSide, net.Side, net.Size) && postNet == 0 {
				// P2-A：减仓清零=自家平仓，打标防对账误报 external_close
				MarkBotClose(acc.Name, instId, net.Side, net.PosId)
			}
			if netOK && isReduction(posSide, net.Side, net.Size) {
				// P2-D：减仓锁利的已实现盈亏估算（lastPx=门控时点价，非成交价）
				face := tm.capParamsFor(acc.Name).FaceValue
				openEv.Pnl = estimateReducePnl(net.Side, net.AvgPx, net.LastPx, face, accSize)
				openEv.RoiPct = netRoiPct(net.Side, net.AvgPx, net.LastPx, SignalLeverage)
				openEv.AvgPx, openEv.LastPx, openEv.NetSide = net.AvgPx, net.LastPx, net.Side
				openEv.Reason = "减仓锁利(pnl为估算)"
			}
			eventlog.Log(applySignalQuote(openEv, q))
			resultCh <- result{acc: OpenedAccount{
				Account: acc,
				Size:    accSize,
				PosSide: posSide,
				WebResp: resp,
			}}
		}()
	}

	opened := make([]OpenedAccount, 0, len(accounts))
	failures := make([]string, 0)
	skipped := make([]string, 0)
	for range accounts {
		r := <-resultCh
		switch {
		case r.skipped:
			// 被仓位上限拦下，不计成功也不计失败，但记录用于可见性
			if r.skipMsg != "" {
				skipped = append(skipped, r.skipMsg)
			}
		case r.err == nil:
			opened = append(opened, r.acc)
		default:
			failures = append(failures, r.err.Error())
		}
	}
	return opened, failures, skipped
}

func (tm *TradeManager) placeSignalWebOrderWithRetry(
	webClient *DirectWebClient,
	acc AccountConfig,
	instId string,
	size, lever, isCrossMargin int,
	price float64,
	posSide string,
) (*utils.WebOrderResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		var resp *utils.WebOrderResponse
		var err error
		if posSide == "long" {
			resp, err = webClient.MarketBuyLongWithRisk(instId, size, lever, isCrossMargin, acc.UID, price)
		} else {
			resp, err = webClient.MarketSellShortWithRisk(instId, size, lever, isCrossMargin, acc.UID, price)
		}

		if err == nil && resp != nil && (resp.Code == 0 || resp.Code == 200) {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else if resp == nil {
			lastErr = fmt.Errorf("空响应")
		} else {
			lastErr = fmt.Errorf("code=%d msg=%s", resp.Code, resp.Msg)
		}

		if attempt < 3 {
			logrus.Warnf("  ⚠️  %s [盘口信号] 第%d次开%s失败，准备重试: %v", acc.Name, attempt, posSide, lastErr)
			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func (tm *TradeManager) sendSignalSummaryToTelegram(instId string, price float64, direction string, accounts []OpenedAccount, failures []string, skipped []string) {
	if tm.telegramClient == nil {
		return
	}

	posSide := signalPosSide(direction)
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	totalSize := 0
	for _, acc := range accounts {
		totalSize += acc.Size
	}

	capSkip, gateSkip := 0, 0
	for _, s := range skipped {
		if strings.Contains(s, "门控") {
			gateSkip++
		} else {
			capSkip++
		}
	}

	msg := fmt.Sprintf(
		"🔔 盘口信号开仓完成\n\n"+
			"📊 交易对: %s\n"+
			"%s 信号: %s -> %s\n"+
			"💰 参考价格: %.2f\n"+
			"📦 总张数: %d\n"+
			"✅ 成功账户数: %d\n"+
			"⚠️ 失败账户数: %d\n"+
			"⛔ 上限跳过: %d  🚦 门控跳过: %d\n"+
			"⏰ 时间: %s\n\n",
		instId,
		signalSideEmoji(direction), direction, posSide,
		price,
		totalSize,
		len(accounts),
		len(failures),
		capSkip, gateSkip,
		currentTime,
	)

	for i, acc := range accounts {
		var openPrice, tradePrice string
		if acc.WebResp != nil {
			if od, err := acc.WebResp.GetOrderData(); err == nil {
				openPrice = fmt.Sprintf("%.2f", od.OpenPrice)
				tradePrice = fmt.Sprintf("%.2f", od.Price)
			}
		}
		msg += fmt.Sprintf("[%d] %s %s [%s %d张]  均价:%s  成交:%s\n",
			i+1, signalSideEmoji(direction), acc.Account.Name, acc.PosSide, acc.Size, openPrice, tradePrice)
	}
	for i, failure := range failures {
		msg += fmt.Sprintf("[失败%d] %s\n", i+1, failure)
	}
	for i, s := range skipped {
		msg += fmt.Sprintf("[跳过%d] %s\n", i+1, s)
	}

	success, err := tm.telegramClient.SendMessage(msg)
	if err != nil {
		logrus.Errorf("❌ 发送Telegram盘口信号开仓消息失败: %v", err)
	} else if success {
		logrus.Info("✅ Telegram盘口信号开仓消息发送成功")
	}
}

// sendOpenSummaryToTelegram 发送开仓汇总到Telegram
// 由于各账户可独立配置 trade_direction/order_size，这里按账户维度展示方向与张数
func (tm *TradeManager) sendOpenSummaryToTelegram(instId string, binPrice, deepPrice float64, accounts []OpenedAccount) {
	if tm.telegramClient == nil {
		return
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")

	longCount, shortCount, totalSize := 0, 0, 0
	for _, acc := range accounts {
		if acc.PosSide == "long" {
			longCount++
		} else {
			shortCount++
		}
		totalSize += acc.Size
	}

	marketEmoji := "🔵"
	marketDesc := "多方（币安>DeepCoin）"
	if binPrice < deepPrice {
		marketEmoji = "🔴"
		marketDesc = "空方（币安<DeepCoin）"
	}

	msg := fmt.Sprintf(
		"🔔 套利开仓完成\n\n"+
			"📊 交易对: %s\n"+
			"%s 行情: %s\n"+
			"💰 币安价格: %.2f\n"+
			"💰 DeepCoin价格: %.2f\n"+
			"📈 价差: %.2f (%.4f%%)\n"+
			"🔵 多头账户: %d  🔴 空头账户: %d\n"+
			"📦 总张数: %d\n"+
			"✅ 成功账户数: %d\n"+
			"⏰ 时间: %s\n\n",
		instId,
		marketEmoji, marketDesc,
		binPrice,
		deepPrice,
		binPrice-deepPrice, (binPrice-deepPrice)/deepPrice*100,
		longCount, shortCount,
		totalSize,
		len(accounts),
		currentTime,
	)

	for i, acc := range accounts {
		sideEmoji := "🔵"
		if acc.PosSide == "short" {
			sideEmoji = "🔴"
		}
		dirTag := "正向"
		if acc.Account.IsReverseDirection() {
			dirTag = "反向"
		}
		var openPrice, tradePrice string
		if acc.WebResp != nil {
			if od, err := acc.WebResp.GetOrderData(); err == nil {
				openPrice = fmt.Sprintf("%.2f", od.OpenPrice)
				tradePrice = fmt.Sprintf("%.2f", od.Price)
			}
		}
		line := fmt.Sprintf("[%d] %s %s [%s-%s %d张]  均价:%s  成交:%s\n",
			i+1, sideEmoji, acc.Account.Name, dirTag, acc.PosSide, acc.Size, openPrice, tradePrice)
		msg += line
	}

	success, err := tm.telegramClient.SendMessage(msg)
	if err != nil {
		logrus.Errorf("❌ 发送Telegram开仓消息失败: %v", err)
	} else if success {
		logrus.Info("✅ Telegram开仓消息发送成功")
	}
}

// executeArbitrageTrades 执行套利交易：开主方向仓位和反向仓位
// needBuyDeep: true=开多, false=开空
// 主账号张数由选中账户自身 order_size 决定；反向账户共享该总张数池按随机分配
// 返回: 所有开仓账号, 主账号(主方向的那个账号), error
func (tm *TradeManager) executeArbitrageTrades_From_API(instId string, price float64, needBuyDeep bool) ([]OpenedAccount, *OpenedAccount, error) {
	openedAccounts := make([]OpenedAccount, 0)

	var mainPosSide, reversePosSide string
	var mainAccounts, reverseAccounts []AccountConfig
	var mainEmoji, reverseEmoji string

	if needBuyDeep {
		// 币安价格 > DeepCoin，开多
		mainPosSide = "long"
		reversePosSide = "short"
		mainEmoji = "🔵"
		reverseEmoji = "🔴"
		mainAccounts = tm.getLongAccounts()
		reverseAccounts = tm.getShortAccounts()
	} else {
		// 币安价格 < DeepCoin，开空
		mainPosSide = "short"
		reversePosSide = "long"
		mainEmoji = "🔴"
		reverseEmoji = "🔵"
		mainAccounts = tm.getShortAccounts()
		reverseAccounts = tm.getLongAccounts()
	}

	// 1. 开主方向仓位（只用一个账号）
	if len(mainAccounts) == 0 {
		return nil, nil, fmt.Errorf("没有配置%s账户，跳过开%s", mainPosSide, mainPosSide)
	}

	selectedAcc := mainAccounts[rand.Intn(len(mainAccounts))]
	// 使用主账号自身配置的张数
	totalSize := tm.getAccountOrderSize(selectedAcc)
	sizeStr := fmt.Sprintf("%d", totalSize)
	logrus.Infof("%s 开%s策略: %s, 价格=%.2f, 张数=%d (账户级), 选中账户=%s",
		mainEmoji, mainPosSide, instId, price, totalSize, selectedAcc.Name)

	client := tm.clients[selectedAcc.Name]

	var resp *utils.PlaceOrderResponse
	var err error
	if needBuyDeep {
		resp, err = client.MarketBuyLong(&utils.QuickOrderRequest{
			InstId: instId,
			Size:   sizeStr,
		})
	} else {
		resp, err = client.MarketSellShort(&utils.QuickOrderRequest{
			InstId: instId,
			Size:   sizeStr,
		})
	}

	if err != nil {
		return nil, nil, fmt.Errorf("%s开%s失败: %w", selectedAcc.Name, mainPosSide, err)
	}

	if !resp.Data.IsSuccess() {
		return nil, nil, fmt.Errorf("%s开%s失败: %s", selectedAcc.Name, mainPosSide, resp.Data.GetError())
	}

	logrus.Infof("  ✅ %s 开%s成功: ordId=%s", selectedAcc.Name, mainPosSide, resp.Data.OrdId)
	mainAccount := OpenedAccount{
		Account: selectedAcc,
		Size:    totalSize,
		PosSide: mainPosSide,
	}
	openedAccounts = append(openedAccounts, mainAccount)

	// 主账号开仓完成后，等待5ms
	time.Sleep(5 * time.Millisecond)

	// 2. 反向开仓
	if len(reverseAccounts) == 0 {
		logrus.Warnf("没有配置%s账户，跳过反向开%s", reversePosSide, reversePosSide)
	} else {
		if len(reverseAccounts) == 1 {
			// 只有一个反向账户，开相同张数
			selectedReverseAcc := reverseAccounts[0]
			logrus.Infof("%s 反向开%s: %s, 价格=%.2f, 张数=%d (相同), 选中账户=%s",
				reverseEmoji, reversePosSide, instId, price, totalSize, selectedReverseAcc.Name)

			reverseClient := tm.clients[selectedReverseAcc.Name]
			var reverseResp *utils.PlaceOrderResponse
			if needBuyDeep {
				reverseResp, err = reverseClient.MarketSellShort(&utils.QuickOrderRequest{
					InstId: instId,
					Size:   sizeStr,
				})
			} else {
				reverseResp, err = reverseClient.MarketBuyLong(&utils.QuickOrderRequest{
					InstId: instId,
					Size:   sizeStr,
				})
			}

			if err != nil {
				logrus.Errorf("  ⚠️  %s 反向开%s失败: %v", selectedReverseAcc.Name, reversePosSide, err)
			} else if !reverseResp.Data.IsSuccess() {
				logrus.Errorf("  ⚠️  %s 反向开%s失败: %s", selectedReverseAcc.Name, reversePosSide, reverseResp.Data.GetError())
			} else {
				logrus.Infof("  ✅ %s 反向开%s成功: ordId=%s", selectedReverseAcc.Name, reversePosSide, reverseResp.Data.OrdId)
				openedAccounts = append(openedAccounts, OpenedAccount{
					Account: selectedReverseAcc,
					Size:    totalSize,
					PosSide: reversePosSide,
				})
			}
		} else {
			// 多个反向账户，随机分配
			allocations := tm.randomAllocate(totalSize, len(reverseAccounts))
			logrus.Infof("%s 反向开%s: %s, 价格=%.2f, 总张数=%d, 分配方案=%v",
				reverseEmoji, reversePosSide, instId, price, totalSize, allocations)

			for i, reverseAcc := range reverseAccounts {
				if allocations[i] == 0 {
					continue
				}

				allocSizeStr := fmt.Sprintf("%d", allocations[i])
				reverseClient := tm.clients[reverseAcc.Name]
				var reverseResp *utils.PlaceOrderResponse
				if needBuyDeep {
					reverseResp, err = reverseClient.MarketSellShort(&utils.QuickOrderRequest{
						InstId: instId,
						Size:   allocSizeStr,
					})
				} else {
					reverseResp, err = reverseClient.MarketBuyLong(&utils.QuickOrderRequest{
						InstId: instId,
						Size:   allocSizeStr,
					})
				}

				if err != nil {
					logrus.Errorf("  ⚠️  %s 反向开%s失败: %v", reverseAcc.Name, reversePosSide, err)
				} else if !reverseResp.Data.IsSuccess() {
					logrus.Errorf("  ⚠️  %s 反向开%s失败: %s", reverseAcc.Name, reversePosSide, reverseResp.Data.GetError())
				} else {
					logrus.Infof("  ✅ %s 反向开%s成功: ordId=%s, 张数=%d", reverseAcc.Name, reversePosSide, reverseResp.Data.OrdId, allocations[i])
					openedAccounts = append(openedAccounts, OpenedAccount{
						Account: reverseAcc,
						Size:    allocations[i],
						PosSide: reversePosSide,
					})
				}
			}
		}
	}

	return openedAccounts, &mainAccount, nil
}

// ExecuteArbitrage 执行套利交易
func (tm *TradeManager) ExecuteArbitrage(instId string, binPrice, deepPrice float64) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查冷却时间
	if time.Since(tm.lastTrade) < tm.tradeCooldown {
		logrus.Debugf("交易冷却中，距上次: %.1fs", time.Since(tm.lastTrade).Seconds())
		return nil
	}

	diff := binPrice - deepPrice
	needBuyDeep := diff > 0 // 币安价格高，需要买入DeepCoin（开多）

	// 1. 执行套利交易：开主方向仓位和反向仓位（使用API接口）
	//    张数由各账户自身 order_size 决定
	openedAccounts, mainAccount, err := tm.executeArbitrageTrades_From_API(instId, deepPrice, needBuyDeep)
	if err != nil {
		return err
	}

	// 2. 当仓位全部开完后，先获取主账号的平均价格，根据主账号计算止盈止损价
	if len(openedAccounts) > 0 && mainAccount != nil {
		time.Sleep(5 * time.Millisecond) // 等待仓位建立

		// 获取主账号的仓位信息
		mainClient := tm.clients[mainAccount.Account.Name]
		mainPosResp, err := mainClient.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
			InstId:   instId,
		})

		if err != nil {
			logrus.Errorf("  ⚠️  主账号 %s 获取仓位失败: %v", mainAccount.Account.Name, err)
			return nil
		}

		// 找到主账号的仓位
		var mainPosition *utils.PositionInfo
		for i := range mainPosResp.Data {
			if mainPosResp.Data[i].PosSide == mainAccount.PosSide {
				mainPosition = &mainPosResp.Data[i]
				break
			}
		}

		if mainPosition == nil {
			logrus.Warnf("  ⚠️  主账号 %s 未找到%s仓位，跳过设置止盈止损", mainAccount.Account.Name, mainAccount.PosSide)
			return nil
		}

		// 解析主账号的平均价格
		mainAvgPx, err := decimal.NewFromString(mainPosition.AvgPx)
		if err != nil {
			logrus.Errorf("  ⚠️  主账号 %s 解析平均价格失败: %v, avgPx=%s", mainAccount.Account.Name, err, mainPosition.AvgPx)
			return nil
		}

		// 将配置的百分比转换为 decimal（使用默认止盈止损比例）
		takeProfitPercent := decimal.NewFromFloat(0.004)
		stopLossPercent := decimal.NewFromFloat(0.004)

		// 根据主账号的方向计算止盈止损价
		var mainTpPrice, mainSlPrice decimal.Decimal
		if mainAccount.PosSide == "long" {
			mainTpPrice = mainAvgPx.Mul(decimal.NewFromInt(1).Add(takeProfitPercent))
			mainSlPrice = mainAvgPx.Mul(decimal.NewFromInt(1).Sub(stopLossPercent))
		} else {
			mainTpPrice = mainAvgPx.Mul(decimal.NewFromInt(1).Sub(takeProfitPercent))
			mainSlPrice = mainAvgPx.Mul(decimal.NewFromInt(1).Add(stopLossPercent))
		}

		// 准备TG消息所需的基础信息（不包含仓位详情）
		accountsWithTPSL := make([]OpenedAccountWithTPSL, 0)

		// 遍历所有开过仓的账号，设置止盈止损
		for _, openedAcc := range openedAccounts {
			client := tm.clients[openedAcc.Account.Name]

			var tpPrice, slPrice decimal.Decimal
			isMainAcc := openedAcc.Account.Name == mainAccount.Account.Name && openedAcc.PosSide == mainAccount.PosSide

			// 如果是主账号，使用正常的止盈止损
			if isMainAcc {
				tpPrice = mainTpPrice
				slPrice = mainSlPrice
			} else {
				// 反向仓位：主账号的止盈价变成止损价，主账号的止损价变成止盈价
				tpPrice = mainSlPrice
				slPrice = mainTpPrice
			}

			// 设置止盈止损
			sltpReq := &utils.SetPositionSLTPRequest{
				InstType:    "SWAP",
				InstId:      instId,
				PosSide:     openedAcc.PosSide,
				MrgPosition: "merge",
				TdMode:      "cross",
				TpTriggerPx: tpPrice.StringFixed(1),
				TpOrdPx:     tpPrice.StringFixed(1),
				SlTriggerPx: slPrice.StringFixed(1),
				SlOrdPx:     slPrice.StringFixed(1),
				Sz:          fmt.Sprintf("%d", openedAcc.Size),
			}

			sltpResp, err := client.SetPositionSLTPTyped(sltpReq)
			if err != nil {
				logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %v", openedAcc.Account.Name, err)
				continue
			}

			if !sltpResp.Data.IsSuccess() {
				logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %s", openedAcc.Account.Name, sltpResp.Data.GetError())
				continue
			}

			if isMainAcc {
				logrus.Infof("  📊 主账号 %s 止盈止损设置成功: 平均价=%s, TP=%s(%+.2f%%), SL=%s(%+.2f%%), ordId=%s",
					openedAcc.Account.Name, mainAvgPx.StringFixed(2), tpPrice.StringFixed(2),
					0.004*100, slPrice.StringFixed(2),
					0.004*100, sltpResp.Data.OrdId)
			} else {
				logrus.Infof("  📊 反向账号 %s 止盈止损设置成功(反向): TP=%s, SL=%s, ordId=%s",
					openedAcc.Account.Name, tpPrice.StringFixed(2), slPrice.StringFixed(2), sltpResp.Data.OrdId)
			}

			// 记录账户和止盈止损价格，仓位信息稍后在协程中获取
			accountsWithTPSL = append(accountsWithTPSL, OpenedAccountWithTPSL{
				OpenedAccount: openedAcc,
				TpPrice:       tpPrice.StringFixed(2),
				SlPrice:       slPrice.StringFixed(2),
			})
		}

		// 发送TG消息（异步，不阻塞主流程）
		if len(accountsWithTPSL) > 0 {
			accountsCopy := make([]OpenedAccountWithTPSL, len(accountsWithTPSL))
			copy(accountsCopy, accountsWithTPSL)
			go func(accounts []OpenedAccountWithTPSL) {
				for _, acc := range accounts {
					logrus.Infof("📊 [API] 开仓完成 %s (%s): TP=%s, SL=%s", acc.Account.Name, acc.PosSide, acc.TpPrice, acc.SlPrice)
				}
			}(accountsCopy)
		}
	}

	// 记录交易时间
	tm.lastTrade = time.Now()

	return nil
}

// executeOpenLong 执行开多
func (tm *TradeManager) executeOpenLong(instId string, price float64) error {
	// 找出所有做多账户
	longAccounts := tm.getLongAccounts()
	if len(longAccounts) == 0 {
		logrus.Warnf("没有配置做多账户，跳过开多")
		return nil
	}

	// 随机选一个账户
	selectedAcc := longAccounts[rand.Intn(len(longAccounts))]
	totalSize := tm.getAccountOrderSize(selectedAcc)
	priceStr := fmt.Sprintf("%.2f", price)
	sizeStr := fmt.Sprintf("%d", totalSize)

	logrus.Infof("🔵 开多策略: %s, 价格=%s, 总张数=%d (账户级), 选中账户=%s",
		instId, priceStr, totalSize, selectedAcc.Name)

	client := tm.clients[selectedAcc.Name]

	// IOC下单
	resp, err := client.IOCBuyLong(&utils.QuickOrderRequest{
		InstId: instId,
		Size:   sizeStr,
		Price:  priceStr,
	})

	if err != nil {
		return fmt.Errorf("%s开多失败: %w", selectedAcc.Name, err)
	}

	if !resp.Data.IsSuccess() {
		return fmt.Errorf("%s开多失败: %s", selectedAcc.Name, resp.Data.GetError())
	}

	logrus.Infof("  ✅ %s 开多成功: ordId=%s", selectedAcc.Name, resp.Data.OrdId)

	// 如果是止盈止损策略，设置止盈止损
	if selectedAcc.IsSLTPStrategy() {
		time.Sleep(200 * time.Millisecond)
		tm.setSLTPForLong(selectedAcc, instId, price)
	}

	// 记录交易时间
	tm.lastTrade = time.Now()

	return nil
}

// executeOpenShort 执行开空
func (tm *TradeManager) executeOpenShort(instId string, price float64) error {
	// 找出所有做空账户
	shortAccounts := tm.getShortAccounts()
	if len(shortAccounts) == 0 {
		logrus.Warnf("没有配置做空账户，跳过开空")
		return nil
	}

	// 随机选一个账户
	selectedAcc := shortAccounts[rand.Intn(len(shortAccounts))]
	totalSize := tm.getAccountOrderSize(selectedAcc)
	priceStr := fmt.Sprintf("%.2f", price)
	sizeStr := fmt.Sprintf("%d", totalSize)

	logrus.Infof("🔴 开空策略: %s, 价格=%s, 总张数=%d (账户级), 选中账户=%s",
		instId, priceStr, totalSize, selectedAcc.Name)

	client := tm.clients[selectedAcc.Name]

	// IOC下单
	resp, err := client.IOCSellShort(&utils.QuickOrderRequest{
		InstId: instId,
		Size:   sizeStr,
		Price:  priceStr,
	})

	if err != nil {
		return fmt.Errorf("%s开空失败: %w", selectedAcc.Name, err)
	}

	if !resp.Data.IsSuccess() {
		return fmt.Errorf("%s开空失败: %s", selectedAcc.Name, resp.Data.GetError())
	}

	logrus.Infof("  ✅ %s 开空成功: ordId=%s", selectedAcc.Name, resp.Data.OrdId)

	// 如果是止盈止损策略，设置止盈止损
	if selectedAcc.IsSLTPStrategy() {
		time.Sleep(200 * time.Millisecond)
		tm.setSLTPForShort(selectedAcc, instId, price)
	}

	// 记录交易时间
	tm.lastTrade = time.Now()

	return nil
}

// setSLTPForLong 为多头仓位设置止盈止损
func (tm *TradeManager) setSLTPForLong(account AccountConfig, instId string, entryPrice float64) {
	tpPrice := entryPrice * (1 + 0.004)
	slPrice := entryPrice * (1 - 0.004)

	sltpReq := &utils.SetPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      instId,
		PosSide:     "long",
		MrgPosition: "merge",
		TdMode:      "cross",
		TpTriggerPx: fmt.Sprintf("%.2f", tpPrice),
		TpOrdPx:     fmt.Sprintf("%.2f", tpPrice),
		SlTriggerPx: fmt.Sprintf("%.2f", slPrice),
		SlOrdPx:     fmt.Sprintf("%.2f", slPrice),
	}

	client := tm.clients[account.Name]
	resp, err := client.SetPositionSLTPTyped(sltpReq)

	if err != nil {
		logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %v", account.Name, err)
		return
	}

	if !resp.Data.IsSuccess() {
		logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %s", account.Name, resp.Data.GetError())
		return
	}

	logrus.Infof("  📊 %s 止盈止损设置成功: TP=%.2f(+%.2f%%), SL=%.2f(-%.2f%%), ordId=%s",
		account.Name, tpPrice, 0.004*100,
		slPrice, 0.004*100, resp.Data.OrdId)
}

// setSLTPForShort 为空头仓位设置止盈止损
func (tm *TradeManager) setSLTPForShort(account AccountConfig, instId string, entryPrice float64) {
	tpPrice := entryPrice * (1 - 0.004)
	slPrice := entryPrice * (1 + 0.004)

	sltpReq := &utils.SetPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      instId,
		PosSide:     "short",
		MrgPosition: "merge",
		TdMode:      "cross",
		TpTriggerPx: fmt.Sprintf("%.2f", tpPrice),
		TpOrdPx:     fmt.Sprintf("%.2f", tpPrice),
		SlTriggerPx: fmt.Sprintf("%.2f", slPrice),
		SlOrdPx:     fmt.Sprintf("%.2f", slPrice),
	}

	client := tm.clients[account.Name]
	resp, err := client.SetPositionSLTPTyped(sltpReq)

	if err != nil {
		logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %v", account.Name, err)
		return
	}

	if !resp.Data.IsSuccess() {
		logrus.Errorf("  ⚠️  %s 设置止盈止损失败: %s", account.Name, resp.Data.GetError())
		return
	}

	logrus.Infof("  📊 %s 止盈止损设置成功: TP=%.2f(-%.2f%%), SL=%.2f(+%.2f%%), ordId=%s",
		account.Name, tpPrice, 0.004*100,
		slPrice, 0.004*100, resp.Data.OrdId)
}

// getLongAccounts 获取所有做多账户
func (tm *TradeManager) getLongAccounts() []AccountConfig {
	accounts := make([]AccountConfig, 0)
	for _, acc := range tm.config.Accounts {
		if acc.IsSpreadLogic() && acc.IsLongAccount() {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

// getShortAccounts 获取所有做空账户
func (tm *TradeManager) getShortAccounts() []AccountConfig {
	accounts := make([]AccountConfig, 0)
	for _, acc := range tm.config.Accounts {
		if acc.IsSpreadLogic() && acc.IsShortAccount() {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

// randomAllocate 随机分配张数到多个账户
// totalSize: 总张数
// accountCount: 账户数量
// 返回: 每个账户分配的张数
func (tm *TradeManager) randomAllocate(totalSize, accountCount int) []int {
	if accountCount == 0 {
		return []int{}
	}

	if accountCount == 1 {
		return []int{totalSize}
	}

	// 随机分配策略：确保每个账户至少分配1张（如果总数足够）
	allocations := make([]int, accountCount)
	remaining := totalSize

	// 先给每个账户分配至少1张
	for i := 0; i < accountCount && remaining > 0; i++ {
		allocations[i] = 1
		remaining--
	}

	// 剩余的随机分配
	for remaining > 0 {
		idx := rand.Intn(accountCount)
		allocations[idx]++
		remaining--
	}

	logrus.Debugf("分配方案: 总%d张 -> %v", totalSize, allocations)
	return allocations
}

// SetCooldown 设置交易冷却时间
func (tm *TradeManager) SetCooldown(duration time.Duration) {
	tm.tradeCooldown = duration
}

// GetAccountStatus 获取账户状态
func (tm *TradeManager) GetAccountStatus() map[string]interface{} {
	status := make(map[string]interface{})

	for _, acc := range tm.config.Accounts {
		client := tm.clients[acc.Name]

		// 获取余额
		balResp, err := client.GetBalancesTyped(&utils.GetBalancesRequest{
			InstType: "SWAP",
			Ccy:      "USDT",
		})

		accStatus := map[string]interface{}{
			"name":          acc.Name,
			"positionSide":  acc.PositionSide,
			"closeStrategy": acc.CloseStrategy,
		}

		if err == nil {
			if usdtBal, found := balResp.GetBalance("USDT"); found {
				accStatus["balance"] = usdtBal.AvailBal
			}
		}

		status[acc.Name] = accStatus
	}

	return status
}

// GetClient 获取指定账户的客户端
func (tm *TradeManager) GetClient(accountName string) *utils.DeepCoinClient {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.clients[accountName]
}

// GetWebClient 获取指定账户的直连Web客户端
func (tm *TradeManager) GetWebClient(accountName string) *DirectWebClient {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.webClients[accountName]
}

// Stop 停止TradeManager
func (tm *TradeManager) Stop() {
	tm.stopOnce.Do(func() {
		close(tm.stopShuffle)
		if tm.loginScheduler != nil {
			tm.loginScheduler.Stop()
		}
		logrus.Info("🛑 TradeManager已停止")
	})
}

// startPositionSideShuffle 启动定时打乱账户position_side
func (tm *TradeManager) startPositionSideShuffle() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	logrus.Info("🔀 账户position_side定时打乱已启动 (每30分钟)")

	for {
		select {
		case <-ticker.C:
			tm.shufflePositionSides()
		case <-tm.stopShuffle:
			logrus.Info("🔀 账户position_side定时打乱已停止")
			return
		}
	}
}

// checkAllPositionsClosed 检查所有账户是否都没有仓位
func (tm *TradeManager) checkAllPositionsClosed() bool {
	for _, acc := range tm.config.Accounts {
		client := tm.clients[acc.Name]

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})

		if err != nil {
			logrus.Warnf("⚠️  检查账户 %s 仓位失败: %v", acc.Name, err)
			return false
		}

		// 检查是否有持仓
		for _, pos := range posResp.Data {
			if pos.Pos != "" && pos.Pos != "0" {
				logrus.Debugf("账户 %s 存在仓位: %s %s", acc.Name, pos.InstId, pos.Pos)
				return false
			}
		}
	}

	return true
}

// shufflePositionSides 打乱所有账户的position_side
func (tm *TradeManager) shufflePositionSides() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 检查所有账户是否都没有仓位
	if !tm.checkAllPositionsClosed() {
		logrus.Info("🔀 跳过打乱position_side: 部分账户存在仓位")
		return
	}

	accountCount := len(tm.config.Accounts)
	if accountCount < 2 {
		logrus.Warn("🔀 账户数量少于2个，无需打乱")
		return
	}

	logrus.Info("🔀 开始打乱账户position_side...")

	// 随机分配long和short
	// 至少保证一个long和一个short
	newSides := make([]string, accountCount)

	// 先随机分配第一个账户为long或short
	if rand.Intn(2) == 0 {
		newSides[0] = "long"
		newSides[1] = "short"
	} else {
		newSides[0] = "short"
		newSides[1] = "long"
	}

	// 剩余的随机分配
	for i := 2; i < accountCount; i++ {
		if rand.Intn(2) == 0 {
			newSides[i] = "long"
		} else {
			newSides[i] = "short"
		}
	}

	// 应用新的position_side
	longCount := 0
	shortCount := 0
	for i := range tm.config.Accounts {
		oldSide := tm.config.Accounts[i].PositionSide
		newSide := newSides[i]
		tm.config.Accounts[i].PositionSide = newSide

		if newSide == "long" {
			longCount++
		} else {
			shortCount++
		}

		if oldSide != newSide {
			logrus.Infof("  📝 账户 %s: %s -> %s", tm.config.Accounts[i].Name, oldSide, newSide)
		} else {
			logrus.Debugf("  ⏸️  账户 %s: %s (未变)", tm.config.Accounts[i].Name, newSide)
		}
	}

	logrus.Infof("✅ position_side打乱完成: %d个long, %d个short", longCount, shortCount)
}
