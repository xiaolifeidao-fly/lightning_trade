package trade

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"common/middleware/vipper"
	"common/utils"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type TradeManager struct {
	config          *TradingSystemConfig
	clients         map[string]*utils.DeepCoinClient
	webClients      map[string]*DirectWebClient
	exchangeClients map[string]ExchangeClient // 按平台工厂构建，key=账户Name
	mu              sync.RWMutex
	lastTrade       time.Time
	tradeCooldown   time.Duration
	stopShuffle     chan struct{} // 用于停止shuffle goroutine
	telegramClient  *utils.TelegramClient
	loginScheduler  *LoginScheduler
}

func NewTradeManager(config *TradingSystemConfig) *TradeManager {
	tm := &TradeManager{
		config:          config,
		clients:         make(map[string]*utils.DeepCoinClient),
		webClients:      make(map[string]*DirectWebClient),
		exchangeClients: make(map[string]ExchangeClient),
		tradeCooldown:   5 * time.Second,
		stopShuffle:     make(chan struct{}),
		telegramClient:  utils.NewTelegramClientWithBotTokenAndChatID(vipper.GetString("telegram.bot_token"), vipper.GetString("telegram.chat_id")),
	}

	// 为每个账户创建客户端
	for _, acc := range config.Accounts {
		// 创建直连 DeepCoin API 客户端
		client := utils.NewDeepCoinClient(acc.APIKey, acc.SecretKey, acc.Passphrase)
		tm.clients[acc.Name] = client
		logrus.Infof("✅ 账户 %s 直连API客户端已创建", acc.Name)

		var webClient *DirectWebClient
		// 如果有 Web 配置种子，创建直连 Web 客户端。
		if acc.HasWebCredentialSeed() {
			userProvider, err := BuildUserProvider(acc)
			if err != nil {
				logrus.Warnf("⚠️  账户 %s Web 凭证提供器创建失败: %v", acc.Name, err)
			} else {
				webClient = NewDirectWebClient(userProvider)
				tm.webClients[acc.Name] = webClient
				logrus.Infof("✅ 账户 %s 直连Web客户端已创建(mode=%s)", acc.Name, acc.LoginType)
			}
		}

		// 通过工厂构建统一交易所客户端（platform 字段决定使用哪套 API）
		tm.exchangeClients[acc.Name] = NewExchangeClient(acc, webClient)
		logrus.Infof("✅ 账户 %s 交易所客户端已创建(platform=%s)", acc.Name, acc.Platform)
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

// EnsureSessionsReady 启动时主动检测所有密码型账户的 session 是否有效。
// 检测方式与 TestSessionAccountValid 一致（net-wapi 接口）。
// 失效则立即触发无头模式重新登录。定时调度器（凌晨1点）复用同一套流程。
func (tm *TradeManager) EnsureSessionsReady() {
	tm.mu.RLock()
	snapshot := make(map[string]*DirectWebClient, len(tm.webClients))
	for k, v := range tm.webClients {
		snapshot[k] = v
	}
	tm.mu.RUnlock()

	if len(snapshot) == 0 {
		logrus.Info("[session] 无密码型账户，跳过启动检测")
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

// getAccountOrderSize 获取指定账户的开仓张数（优先账户级 order_size，回退到全局 trade.order_size）
func (tm *TradeManager) getAccountOrderSize(acc AccountConfig) int {
	return acc.GetOrderSize(tm.config.Trade.OrderSize)
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
	accounts := tm.config.Accounts
	if len(accounts) == 0 {
		logrus.Warnf("没有配置任何账户，跳过开仓")
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
		if acc.IsLongAccount() {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

// getShortAccounts 获取所有做空账户
func (tm *TradeManager) getShortAccounts() []AccountConfig {
	accounts := make([]AccountConfig, 0)
	for _, acc := range tm.config.Accounts {
		if acc.IsShortAccount() {
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
// GetExchangeClient 返回账户对应的交易所统一客户端（由工厂模式构建）。
func (tm *TradeManager) GetExchangeClient(accountName string) ExchangeClient {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.exchangeClients[accountName]
}

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

// OpenPositionByAI 按 AI 决策对指定账户市价开/加仓。
// side: "long"=做多/买入, "short"=做空/卖出。size 为合约张数。tradePrice 仅用于风控埋点。
// 内部通过 ExchangeClient 工厂分发，支持 deepcoin / binance 等多平台。
func (tm *TradeManager) OpenPositionByAI(accountName, instId, side string, size int, tradePrice float64) (*utils.WebOrderResponse, error) {
	if size <= 0 {
		return nil, fmt.Errorf("开仓张数无效: %d", size)
	}

	ec := tm.GetExchangeClient(accountName)
	if ec == nil {
		return nil, fmt.Errorf("账户 %s 未找到交易所客户端", accountName)
	}

	return ec.OpenPosition(instId, side, size, tradePrice)
}

// Stop 停止TradeManager
func (tm *TradeManager) Stop() {
	close(tm.stopShuffle)
	if tm.loginScheduler != nil {
		tm.loginScheduler.Stop()
	}
	logrus.Info("🛑 TradeManager已停止")
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
