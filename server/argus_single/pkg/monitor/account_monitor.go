package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
	"common/middleware/vipper"
	"common/utils"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

// AccountMonitor 账户监控器
type AccountMonitor struct {
	telegramClient     *utils.TelegramClient
	stopChan           chan struct{}
	wg                 sync.WaitGroup
	queryInterval      time.Duration        // 查询间隔，默认1分钟
	reportInterval     time.Duration        // 报告间隔，默认30分钟
	mu                 sync.RWMutex         // 保护余额状态
	lastBalances       []AccountBalanceInfo // 最新的余额信息
	hasInsufficientBal bool                 // 是否有账户余额不足（<30U，用于交易判断）
	hasLowBalance      bool                 // 是否有账户余额低于报告阈值（<100U，用于发送报告）
	minBalance         decimal.Decimal      // 最小余额阈值，默认30U（用于交易判断）
	reportThreshold    decimal.Decimal      // 报告阈值，默认100U（低于此值才发送报告）

	posQueryInterval   time.Duration        // 持仓查询间隔
	pnlProfitThreshold decimal.Decimal      // 盈利告警阈值（正值，如150表示150%）
	pnlLossThreshold   decimal.Decimal      // 亏损告警阈值（正值，如150表示-150%时告警）
	pnlAlertCooldown   time.Duration        // 同一持仓告警冷却时间
	pnlAlertMu         sync.RWMutex         // 保护告警记录
	lastPnlAlerts      map[string]time.Time // 上次告警时间，key: "账户名:instId:posSide"

	trailStates map[string]TrailState // 移动止盈状态，key 同上（仅 trailing 账户）
	trailMu     sync.RWMutex          // 保护 trailStates

	// P4 trail 持久化：重启不再丢峰值保护。身份只认 posId（探针 v3 结论）。
	trailStatePath string                    // 快照路径（可配置）
	trailDirty     atomic.Bool               // 状态变更标脏，30s 快照据此决定是否落盘
	pendingRestore map[string]persistedTrail // 启动时载入、各账户首轮轮询时消费
	restoreMu      sync.Mutex                // 保护 pendingRestore

	uplByAccount map[string]uplSample // 账户未实现盈亏缓存（持仓轮询每5s刷新，P2-C；带采样时刻防陈旧值冒充实时）
	uplMu        sync.RWMutex         // 保护 uplByAccount

	snapshots map[string]posSnapshot // 上一轮持仓快照，key 同 trailStates（P2-A 对账）
	snapMu    sync.RWMutex           // 保护 snapshots

	closeFails *closeFailTracker // 平仓连败计数（P3，≥3 连败升级告警）

	eventLogDir           string        // 结构化事件日志目录
	compareReportInterval time.Duration // 对比报告周期（实验期 12h）
}

func normalizePositionPnl(pos utils.PositionInfo) (decimal.Decimal, bool) {
	if pos.UnrealizedProfit == "" {
		return decimal.Zero, false
	}

	rawPnl, err := decimal.NewFromString(pos.UnrealizedProfit)
	if err != nil {
		return decimal.Zero, false
	}

	absPnl := rawPnl.Abs()
	avgPx, avgErr := decimal.NewFromString(pos.AvgPx)
	lastPx, lastErr := decimal.NewFromString(pos.LastPx)
	if avgErr != nil || lastErr != nil {
		return rawPnl, true
	}

	switch strings.ToLower(strings.TrimSpace(pos.PosSide)) {
	case "long":
		if lastPx.LessThan(avgPx) {
			return absPnl.Neg(), true
		}
		if lastPx.GreaterThan(avgPx) {
			return absPnl, true
		}
		return decimal.Zero, true
	case "short":
		if lastPx.GreaterThan(avgPx) {
			return absPnl.Neg(), true
		}
		if lastPx.LessThan(avgPx) {
			return absPnl, true
		}
		return decimal.Zero, true
	default:
		return rawPnl, true
	}
}

// NewAccountMonitor 创建账户监控器
func NewAccountMonitor() *AccountMonitor {
	intervalSec := vipper.GetInt("position.monitor.interval_seconds")
	if intervalSec <= 0 {
		intervalSec = 5
	}
	profitThreshold := vipper.GetFloat64("position.monitor.profit_threshold")
	if profitThreshold <= 0 {
		profitThreshold = 150
	}
	lossThreshold := vipper.GetFloat64("position.monitor.loss_threshold")
	if lossThreshold <= 0 {
		lossThreshold = 150
	}

	logrus.Infof("持仓监控配置: 间隔=%ds, 盈利阈值=%.2f%%, 亏损阈值=%.2f%%", intervalSec, profitThreshold, lossThreshold)

	// 事件日志已在 initialization 阶段 Init；这里仅取目录用于对比报告读取
	logDir := vipper.GetString("log.dir")
	if strings.TrimSpace(logDir) == "" {
		logDir = "./logs"
	}

	// P4：trail 快照路径（可配置；默认放部署目录下的 data/，需与部署目录同生命周期）
	trailPath := strings.TrimSpace(vipper.GetString("position.monitor.trail_state_path"))
	if trailPath == "" {
		trailPath = "./data/trail_state.json"
	}

	return &AccountMonitor{
		telegramClient:     utils.NewTelegramClientWithBotTokenAndChatID(vipper.GetString("telegram.bot_token"), vipper.GetString("telegram.chat_id")),
		stopChan:           make(chan struct{}),
		queryInterval:      1 * time.Minute,  // 1分钟查询一次
		reportInterval:     30 * time.Minute, // 30分钟发送一次报告
		lastBalances:       make([]AccountBalanceInfo, 0),
		hasInsufficientBal: false,
		hasLowBalance:      false,
		minBalance:         decimal.NewFromInt(30), // 最小余额30U（用于交易判断）
		reportThreshold:    decimal.NewFromInt(50), // 报告阈值50U（低于此值才发送报告）
		posQueryInterval:   time.Duration(intervalSec) * time.Second,
		pnlProfitThreshold: decimal.NewFromFloat(profitThreshold),
		pnlLossThreshold:   decimal.NewFromFloat(lossThreshold),
		pnlAlertCooldown:   5 * time.Minute, // 同一持仓5分钟内不重复告警
		lastPnlAlerts:      make(map[string]time.Time),
		trailStates:        make(map[string]TrailState),
		trailStatePath:     trailPath,
		uplByAccount:       make(map[string]uplSample),
		snapshots:          make(map[string]posSnapshot),
		closeFails:         newCloseFailTracker(time.Now),

		eventLogDir:           logDir,
		compareReportInterval: 12 * time.Hour,
	}
}

// resolveTrailParamsForAccount 解析某账户的移动止盈参数（账户级覆盖 + 全局回退）。
func resolveTrailParamsForAccount(index int) TrailParams {
	f := func(name, globalKey string, def float64) float64 {
		return trade.AccFloat(index, name, globalKey, def)
	}
	return TrailParams{
		TierSmallRatio:     f("tier_small_ratio", "position.monitor.trail.tier_small_ratio", 0.30),
		TierLargeRatio:     f("tier_large_ratio", "position.monitor.trail.tier_large_ratio", 0.65),
		Small:              Tier{ActivatePct: f("small_activate", "position.monitor.trail.small_activate", 150), GivebackFrac: f("small_giveback", "position.monitor.trail.small_giveback", 0.35)},
		Medium:             Tier{ActivatePct: f("medium_activate", "position.monitor.trail.medium_activate", 90), GivebackFrac: f("medium_giveback", "position.monitor.trail.medium_giveback", 0.28)},
		Large:              Tier{ActivatePct: f("large_activate", "position.monitor.trail.large_activate", 40), GivebackFrac: f("large_giveback", "position.monitor.trail.large_giveback", 0.20)},
		CatastropheStopPct: f("catastrophe_stop_pct", "position.monitor.catastrophe_stop_pct", 300),
	}
}

// Start 启动账户监控
func (am *AccountMonitor) Start() {
	logrus.Info("启动账户监控...")

	// 埋点失败 → TG 告警（底层 web 包不依赖 TG，此处注入通知方式）
	am.installTrackFailureAlert()

	// P4：载入 trail 快照，待各账户首轮持仓轮询时用 posId 对账恢复。
	// 载入失败一律冷启动（=旧行为），不阻断启动。
	if states, err := loadTrailStateFile(am.trailStatePath); err != nil {
		logrus.Warnf("[trail持久化] 载入 %s 失败，按冷启动处理: %v", am.trailStatePath, err)
	} else if len(states) > 0 {
		am.restoreMu.Lock()
		am.pendingRestore = states
		am.restoreMu.Unlock()
		logrus.Infof("[trail持久化] 载入 %d 条待恢复状态，将在各账户首轮持仓轮询时按 posId 对账", len(states))
	}

	// 启动 trail 状态快照（30s，仅脏时落盘）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startTrailSnapshot()
	}()

	// 程序启动后立即查询（如果余额低于100才发送报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.queryBalances(true) // 查询并检查是否需要发送报告
	}()

	// 启动定时查询任务（1分钟查询一次，但不发送报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPeriodicQuery()
	}()

	// 启动定时报告任务（30分钟发送一次报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPeriodicReport()
	}()

	// 启动持仓盈亏监控（5秒一次）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPositionMonitor()
	}()

	// 启动账户对比报告（12h 一份，从结构化事件日志聚合）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startComparisonReport()
	}()

	logrus.Infof("✅ 账户监控已启动，每1分钟查询余额，每%v监控持仓盈亏（盈利>%s%%/亏损<-%s%%）",
		am.posQueryInterval, am.pnlProfitThreshold.String(), am.pnlLossThreshold.String())
}

// startComparisonReport 周期性发送账户对比报告。
func (am *AccountMonitor) startComparisonReport() {
	ticker := time.NewTicker(am.compareReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.sendComparisonReport()
		}
	}
}

// sendComparisonReport 读取 [now-周期, now] 窗口的事件日志（跨午夜读今天+昨天），
// 聚合各账户指标并发送 TG 对比报告。
func (am *AccountMonitor) sendComparisonReport() {
	until := time.Now()
	since := until.Add(-am.compareReportInterval)
	events := eventlog.LoadWindow(am.eventLogDir, since, until)
	msg := formatComparisonReport(eventlog.Aggregate(events), since, until)
	if success, err := am.telegramClient.SendMessage(msg); err != nil {
		logrus.Errorf("[对比报告] 发送失败: %v", err)
	} else if success {
		logrus.Info("[对比报告] 已发送")
	}
}

// formatComparisonReport 把各账户指标格式化为 TG 文本（按账户名排序，口径统一）。
func formatComparisonReport(metrics map[string]*eventlog.AccountMetrics, since, until time.Time) string {
	msg := fmt.Sprintf("📊 账户对比报告\n⏰ %s ~ %s\n",
		since.Format("01-02 15:04"), until.Format("01-02 15:04"))
	names := make([]string, 0, len(metrics))
	for n := range metrics {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return msg + "\n（当日暂无事件）"
	}
	for _, n := range names {
		m := metrics[n]
		// 权益诚实标注：未知不得伪装成已知（codex round-3 #1）。
		// 全覆盖=现有口径；部分覆盖=下界+覆盖率；零覆盖（老日志/持续故障）=降级已实现口径。
		var ddStr string
		switch {
		case m.EquityKnownEvents == 0:
			ddStr = fmt.Sprintf("最大回撤(仅已实现): %.2f%%", m.BalanceDrawdownPct)
		case m.EquityKnownEvents < m.BalanceEvents:
			// 覆盖率向下取整：与"至少"同为下界语义。四舍五入会把 99.86%（实盘常见：
			// 12h 窗口内一次重启后的首查缺 equity）显示成"覆盖100%"，与"至少"自相矛盾。
			ddStr = fmt.Sprintf("最大回撤(含浮亏): 至少%.2f%% (权益覆盖%.0f%%)",
				m.MaxDrawdownPct, math.Floor(m.EquityCoveragePct()))
		default:
			ddStr = fmt.Sprintf("最大回撤(含浮亏): %.2f%%", m.MaxDrawdownPct)
		}
		eqStr := "未知"
		if m.LastEquityKnown {
			eqStr = fmt.Sprintf("%.2f", m.LastEquity)
		}
		msg += fmt.Sprintf(
			"\n━━━━━━━━━━━━━━\n账户: %s  [%s]\n"+
				"收益率: %+.2f%%  %s\n"+
				"余额: %.2f → %.2f  当前权益: %s\n"+
				"最大张数: %d\n"+
				"开仓:%d  上限跳过:%d  门控拦截:%d  亏损告警:%d\n"+
				"平仓: 移动止盈%d / 兜底%d / 固定%d  胜率%.0f%%  (外部%d/手动%d不计)\n"+
				"平仓ROI: 均%.1f%% [%.1f%%~%.1f%%]  锁利合计:%.4f\n",
			m.Account, m.Variant,
			m.PnLPct(), ddStr,
			m.FirstBalance, m.LastBalance, eqStr,
			m.MaxSize,
			m.Opens, m.CapSkips, m.GateBlocks, m.LossAlerts,
			m.TrailingCloses, m.CatastropheStops, m.FixedCloses, m.CloseWinRate()*100,
			m.ExternalCloses, m.ManualCloses,
			m.CloseAvgRoi(), m.CloseRoiMin, m.CloseRoiMax, m.ClosePnlSum,
		)
	}
	return msg
}

// Stop 停止账户监控。停止前同步 flush 一次 trail 状态——
// 30s 异步快照盖不住正常部署重启（设计 R2）。
func (am *AccountMonitor) Stop() {
	close(am.stopChan)
	am.wg.Wait()
	am.flushTrailStates(true) // 同步 flush，必须在 goroutine 全部退出后执行
	logrus.Info("账户监控已停止")
}

// startPeriodicQuery 启动定时查询（1分钟一次，不发送报告）
func (am *AccountMonitor) startPeriodicQuery() {
	ticker := time.NewTicker(am.queryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.queryBalances(false) // 只查询，不发送报告
		}
	}
}

// startPeriodicReport 启动定时报告（30分钟一次）
func (am *AccountMonitor) startPeriodicReport() {
	ticker := time.NewTicker(am.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.sendReport()
		}
	}
}

// queryBalances 查询所有账户余额
// sendReport: 是否发送报告到Telegram
func (am *AccountMonitor) queryBalances(sendReport bool) {
	if !trade.IsInitialized() {
		logrus.Warnf("交易管理器未初始化，跳过账户余额查询")
		return
	}

	tm := trade.GetManager()
	if tm == nil {
		logrus.Warnf("无法获取交易管理器，跳过账户余额查询")
		return
	}

	// 获取账户配置
	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		logrus.Warnf("没有配置账户，跳过余额查询")
		return
	}

	logrus.Info("开始查询账户余额...")

	// 存储所有账户的余额信息
	accountBalances := make([]AccountBalanceInfo, 0, len(config.Accounts))

	// 遍历所有账户，查询余额
	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			logrus.Warnf("账户 %s 的客户端不存在，跳过", acc.Name)
			continue
		}

		// 查询余额
		logrus.Infof("[余额查询] 账户 %s (uid=%s) 开始请求 GetBalancesTyped, instType=SWAP, ccy=USDT", acc.Name, acc.UID)
		balResp, err := client.GetBalancesTyped(&utils.GetBalancesRequest{
			InstType: "SWAP",
			Ccy:      "USDT",
		})

		if err != nil {
			logrus.Errorf("[余额查询] 账户 %s 请求失败: %v", acc.Name, err)
			accountBalances = append(accountBalances, AccountBalanceInfo{
				AccountName:    acc.Name,
				UID:            acc.UID,
				InitialBalance: acc.InitialBalance,
				Error:          err.Error(),
			})
		} else {
			// 打印原始响应，方便排查字段映射问题
			if raw, jerr := json.Marshal(balResp); jerr == nil {
				logrus.Infof("[余额查询] 账户 %s 原始响应: %s", acc.Name, string(raw))
			}
			logrus.Infof("[余额查询] 账户 %s code=%s msg=%s Data条数=%d", acc.Name, balResp.Code, balResp.Msg, len(balResp.Data))

			// 获取USDT余额
			if usdtBal, found := balResp.GetBalance("USDT"); found {
				accountBalances = append(accountBalances, AccountBalanceInfo{
					AccountName:    acc.Name,
					UID:            acc.UID,
					InitialBalance: acc.InitialBalance,
					Ccy:            usdtBal.Ccy,
					Bal:            usdtBal.Bal,
					FrozenBal:      usdtBal.FrozenBal,
					AvailBal:       usdtBal.AvailBal,
				})
				logrus.Infof("[余额查询] 账户 %s 余额: 总=%s 可用=%s 冻结=%s",
					acc.Name, usdtBal.Bal, usdtBal.AvailBal, usdtBal.FrozenBal)
				if bal, perr := strconv.ParseFloat(strings.TrimSpace(usdtBal.Bal), 64); perr == nil {
					// P2-C：补 equity/upl。缓存陈旧（持仓查询持续异常）时诚实省略，
					// 报告回退 balance——旧 UPL 不得冒充实时权益。
					sample, ok := am.getUpl(acc.Name)
					hasUpl := ok && uplFresh(sample, time.Now(), am.posQueryInterval)
					if ok && !hasUpl {
						logrus.Warnf("[余额查询] 账户 %s UPL 缓存已陈旧 %.0fs（持仓查询持续异常?），balance 事件省略 equity/upl",
							acc.Name, time.Since(sample.ObservedAt).Seconds())
					}
					eventlog.Log(buildBalanceEvent(acc, bal, sample.Value, hasUpl, am.accountNetSize(acc.Name)))
				}
			} else {
				logrus.Warnf("[余额查询] 账户 %s Data中未找到 ccy=USDT 的条目（Data共%d条）", acc.Name, len(balResp.Data))
				for idx, b := range balResp.Data {
					logrus.Warnf("[余额查询]   Data[%d]: ccy=%s bal=%s availBal=%s", idx, b.Ccy, b.Bal, b.AvailBal)
				}
				accountBalances = append(accountBalances, AccountBalanceInfo{
					AccountName:    acc.Name,
					UID:            acc.UID,
					InitialBalance: acc.InitialBalance,
					Error:          "未找到USDT余额",
				})
			}
		}

		// 如果不是最后一个账户，sleep 1.2秒
		if i < len(config.Accounts)-1 {
			time.Sleep(1200 * time.Millisecond)
		}
	}

	// 更新余额状态并检查是否有余额不足的账户
	am.updateBalanceStatus(accountBalances)

	// 如果需要发送报告，检查是否有账户余额低于报告阈值
	if sendReport {
		am.mu.RLock()
		hasLowBal := am.hasLowBalance
		am.mu.RUnlock()

		if hasLowBal {
			am.sendReportWithBalances(accountBalances)
		} else {
			logrus.Debugf("所有账户余额正常（>=100U），跳过发送报告")
		}
	}
}

// updateBalanceStatus 更新余额状态并检查是否有余额不足的账户
func (am *AccountMonitor) updateBalanceStatus(balances []AccountBalanceInfo) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.lastBalances = balances
	hasInsufficient := false
	hasLow := false

	// 检查每个账户的可用余额
	for _, bal := range balances {
		if bal.Error != "" {
			continue // 跳过有错误的账户
		}

		if bal.AvailBal != "" {
			if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
				// 检查是否低于交易阈值（30U）
				if availDec.LessThan(am.minBalance) {
					hasInsufficient = true
					logrus.Warnf("账户 %s 可用余额不足（交易阈值）: %s < %s", bal.AccountName, availDec.String(), am.minBalance.String())
				}
				// 检查是否低于报告阈值（100U）
				if availDec.LessThan(am.reportThreshold) {
					hasLow = true
					logrus.Warnf("账户 %s 可用余额低于报告阈值: %s < %s", bal.AccountName, availDec.String(), am.reportThreshold.String())
				}
			}
		}
	}

	am.hasInsufficientBal = hasInsufficient
	am.hasLowBalance = hasLow
}

// sendReport 发送报告（使用最新的余额信息）
// 只有当有账户余额低于报告阈值（100U）时才发送
func (am *AccountMonitor) sendReport() {
	am.mu.RLock()
	balances := make([]AccountBalanceInfo, len(am.lastBalances))
	copy(balances, am.lastBalances)
	hasLowBal := am.hasLowBalance
	am.mu.RUnlock()

	if len(balances) == 0 {
		return
	}

	// 只有当有账户余额低于报告阈值时才发送
	if hasLowBal {
		am.sendReportWithBalances(balances)
	} else {
		logrus.Debugf("所有账户余额正常（>=100U），跳过发送报告")
	}
}

// sendReportWithBalances 使用指定的余额信息发送报告
func (am *AccountMonitor) sendReportWithBalances(balances []AccountBalanceInfo) {
	// 生成消息并发送到Telegram
	message := am.formatBalanceMessage(balances)
	success, err := am.telegramClient.SendMessage(message)
	if err != nil {
		logrus.Errorf("发送Telegram消息失败: %v", err)
	} else if success {
		logrus.Info("账户余额信息已发送到Telegram")
	}
}

// HasSufficientBalance 检查是否有足够的余额（所有账户的可用余额都 >= 30U）
func (am *AccountMonitor) HasSufficientBalance() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return !am.hasInsufficientBal
}

// GetInsufficientAccounts 获取余额不足的账户列表
func (am *AccountMonitor) GetInsufficientAccounts() []string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	insufficientAccounts := make([]string, 0)
	for _, bal := range am.lastBalances {
		if bal.Error != "" {
			continue
		}

		if bal.AvailBal != "" {
			if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
				if availDec.LessThan(am.minBalance) {
					insufficientAccounts = append(insufficientAccounts, bal.AccountName)
				}
			}
		}
	}

	return insufficientAccounts
}

// AccountBalanceInfo 账户余额信息
type AccountBalanceInfo struct {
	AccountName    string
	UID            string  // 用户ID
	InitialBalance float64 // 初始余额
	Ccy            string
	Bal            string // 总余额
	FrozenBal      string // 冻结余额
	AvailBal       string // 可用余额
	Error          string // 错误信息（如果有）
}

// formatBalanceMessage 格式化余额消息
func (am *AccountMonitor) formatBalanceMessage(balances []AccountBalanceInfo) string {
	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("💰 账户余额监控报告\n⏰ 时间: %s\n\n", now)

	if len(balances) == 0 {
		message += "⚠️ 未查询到任何账户余额信息"
		return message
	}

	// 计算总余额汇总和总盈亏
	var totalBal, totalAvailBal, totalFrozenBal, totalInitialBal, totalProfitLoss decimal.Decimal
	successCount := 0

	for i, bal := range balances {
		if bal.Error != "" {
			// 显示账户名称和UID
			accountDisplay := bal.AccountName
			if bal.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", bal.AccountName, bal.UID)
			}
			message += fmt.Sprintf("❌ 账户 %d: %s\n   错误: %s\n\n", i+1, accountDisplay, bal.Error)
		} else {
			// 显示账户名称和UID
			accountDisplay := bal.AccountName
			if bal.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", bal.AccountName, bal.UID)
			}
			message += fmt.Sprintf("✅ 账户 %d: %s\n", i+1, accountDisplay)
			message += fmt.Sprintf("   币种: %s\n", bal.Ccy)

			// 计算余额变动
			var currentBalDec, initialBalDec decimal.Decimal
			var balanceChange decimal.Decimal
			var changeEmoji string

			if bal.Bal != "" {
				if dec, err := decimal.NewFromString(bal.Bal); err == nil {
					currentBalDec = dec
					totalBal = totalBal.Add(dec)
				}
			}

			if bal.InitialBalance > 0 {
				initialBalDec = decimal.NewFromFloat(bal.InitialBalance)
				totalInitialBal = totalInitialBal.Add(initialBalDec)
				balanceChange = currentBalDec.Sub(initialBalDec)

				if balanceChange.IsPositive() {
					changeEmoji = "📈" // 绿色上升箭头
				} else if balanceChange.IsNegative() {
					changeEmoji = "📉" // 红色下降箭头
				} else {
					changeEmoji = "➡️" // 持平
				}
			}

			message += fmt.Sprintf("   初始余额: %s\n", initialBalDec.StringFixed(2))
			message += fmt.Sprintf("   总余额: %s\n", bal.Bal)
			if bal.InitialBalance > 0 {
				changeText := balanceChange.StringFixed(2)
				if balanceChange.IsPositive() {
					changeText = fmt.Sprintf("+%s", changeText)
				}
				message += fmt.Sprintf("   余额变动: %s %s\n", changeEmoji, changeText)
			}
			message += fmt.Sprintf("   可用余额: %s\n", bal.AvailBal)
			message += fmt.Sprintf("   冻结余额: %s\n\n", bal.FrozenBal)

			// 累加余额（只累加成功的账户）
			if bal.AvailBal != "" {
				if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
					totalAvailBal = totalAvailBal.Add(availDec)
				}
			}
			if bal.FrozenBal != "" {
				if frozenDec, err := decimal.NewFromString(bal.FrozenBal); err == nil {
					totalFrozenBal = totalFrozenBal.Add(frozenDec)
				}
			}

			// 累加盈亏
			if bal.InitialBalance > 0 && bal.Bal != "" {
				if currentDec, err := decimal.NewFromString(bal.Bal); err == nil {
					profitLoss := currentDec.Sub(initialBalDec)
					totalProfitLoss = totalProfitLoss.Add(profitLoss)
				}
			}

			successCount++
		}
	}

	// 添加汇总信息
	if successCount > 0 {
		message += "━━━━━━━━━━━━━━━━━━━━\n"
		message += fmt.Sprintf("📊 汇总（%d个账户）:\n", successCount)
		message += fmt.Sprintf("   总余额: %s\n", totalBal.StringFixed(2))
		message += fmt.Sprintf("   可用余额: %s\n", totalAvailBal.StringFixed(2))
		message += fmt.Sprintf("   冻结余额: %s\n", totalFrozenBal.StringFixed(2))

		// 添加总盈亏信息
		if totalInitialBal.GreaterThan(decimal.Zero) {
			message += "\n━━━━━━━━━━━━━━━━━━━━\n"
			message += "💹 账户总盈亏:\n"
			message += fmt.Sprintf("   初始总余额: %s\n", totalInitialBal.StringFixed(2))
			message += fmt.Sprintf("   当前总余额: %s\n", totalBal.StringFixed(2))

			var totalProfitLossEmoji string
			if totalProfitLoss.IsPositive() {
				totalProfitLossEmoji = "📈"
			} else if totalProfitLoss.IsNegative() {
				totalProfitLossEmoji = "📉"
			} else {
				totalProfitLossEmoji = "➡️"
			}

			profitLossText := totalProfitLoss.StringFixed(2)
			if totalProfitLoss.IsPositive() {
				profitLossText = fmt.Sprintf("+%s", profitLossText)
			}
			message += fmt.Sprintf("   总盈亏: %s %s\n", totalProfitLossEmoji, profitLossText)

			// 计算盈亏百分比
			if totalInitialBal.GreaterThan(decimal.Zero) {
				profitLossPercent := totalProfitLoss.Div(totalInitialBal).Mul(decimal.NewFromInt(100))
				percentText := profitLossPercent.StringFixed(2)
				if profitLossPercent.IsPositive() {
					percentText = fmt.Sprintf("+%s%%", percentText)
				} else {
					percentText = fmt.Sprintf("%s%%", percentText)
				}
				message += fmt.Sprintf("   盈亏比例: %s\n", percentText)
			}
		}
	}

	return message
}

// GetBalanceReport 实时查询余额并返回报告（供 Telegram Bot 调用）
func (am *AccountMonitor) GetBalanceReport() string {
	// 实时触发一次查询，更新内部缓存
	am.queryBalances(false)

	am.mu.RLock()
	balances := make([]AccountBalanceInfo, len(am.lastBalances))
	copy(balances, am.lastBalances)
	am.mu.RUnlock()

	if len(balances) == 0 {
		return "⚠️ 暂无账户余额信息，请稍后再试"
	}

	return am.formatBalanceMessage(balances)
}

// GetPositionReport 实时查询所有账户持仓并返回报告（供 Telegram Bot 调用）
func (am *AccountMonitor) GetPositionReport() string {
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法查询持仓"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("📊 持仓查询报告\n⏰ 时间: %s\n\n", now)
	totalAccounts := len(config.Accounts)
	hasAnyPosition := false

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户%d %s: 客户端不存在\n\n", i+1, acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})

		accountDisplay := acc.Name
		if acc.UID != "" {
			accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
		}

		if err != nil {
			logrus.Errorf("[持仓查询] 账户 %s 失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户%d %s\n   错误: %v\n\n", i+1, accountDisplay, err)
			continue
		}

		logrus.Infof("[持仓查询] 账户 %s 持仓条数: %d", acc.Name, len(posResp.Data))

		if len(posResp.Data) == 0 {
			message += fmt.Sprintf("✅ 账户%d %s\n   暂无持仓\n\n", i+1, accountDisplay)
			continue
		}

		hasAnyPosition = true
		message += fmt.Sprintf("✅ 账户%d %s (%d个持仓)\n", i+1, accountDisplay, len(posResp.Data))

		for j, pos := range posResp.Data {
			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}

			pnlDisplay := pos.UnrealizedProfit
			pnlEmoji := "➡️"
			pnlPercentDisplay := ""
			if pnl, ok := normalizePositionPnl(pos); ok {
				pnlDisplay = pnl.String()
				if pnl.IsPositive() {
					pnlEmoji = "📈"
				} else if pnl.IsNegative() {
					pnlEmoji = "📉"
				}
				if pos.UseMargin != "" {
					if margin, merr := decimal.NewFromString(pos.UseMargin); merr == nil && margin.IsPositive() {
						pct := pnl.Div(margin).Mul(decimal.NewFromInt(100))
						pctStr := pct.StringFixed(2)
						if pct.IsPositive() {
							pctStr = "+" + pctStr
						}
						pnlPercentDisplay = fmt.Sprintf("(%s%%)", pctStr)
					}
				}
			}

			message += fmt.Sprintf(
				"  [%d] %s %s  方向:%s\n"+
					"      持仓:%s张  开仓均价:%s\n"+
					"      最新价:%s  强平价:%s\n"+
					"      占用保证金:%s  未实现盈亏:%s %s %s\n\n",
				j+1, directionEmoji, pos.InstId, pos.PosSide,
				pos.Pos, pos.AvgPx,
				pos.LastPx, pos.LiqPx,
				pos.UseMargin, pnlEmoji, pnlDisplay, pnlPercentDisplay,
			)
		}

		// 账户间加间隔
		if i < totalAccounts-1 {
			time.Sleep(1200 * time.Millisecond)
		}
	}

	if !hasAnyPosition {
		message += "📭 所有账户当前无持仓"
	}

	return message
}

// GetROIReport 实时查询所有账户持仓收益率并返回报告（供 Telegram Bot 调用）
func (am *AccountMonitor) GetROIReport() string {
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法查询收益率"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("📈 收益率查询报告\n⏰ 时间: %s\n\n", now)
	hasAnyPosition := false
	totalPnl := decimal.Zero
	totalMargin := decimal.Zero

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户%d %s: 客户端不存在\n\n", i+1, acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			logrus.Errorf("[收益率查询] 账户 %s 失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户%d %s\n   错误: %v\n\n", i+1, acc.Name, err)
			continue
		}

		for _, pos := range posResp.Data {
			if pos.UnrealizedProfit == "" || pos.UseMargin == "" {
				continue
			}
			pnl, ok := normalizePositionPnl(pos)
			if !ok {
				continue
			}
			margin, err := decimal.NewFromString(pos.UseMargin)
			if err != nil || !margin.IsPositive() {
				continue
			}

			hasAnyPosition = true
			totalPnl = totalPnl.Add(pnl)
			totalMargin = totalMargin.Add(margin)

			roi := pnl.Div(margin).Mul(decimal.NewFromInt(100))
			roiStr := roi.StringFixed(2)
			if roi.IsPositive() {
				roiStr = "+" + roiStr
			}
			pnlStr := pnl.StringFixed(4)
			if pnl.IsPositive() {
				pnlStr = "+" + pnlStr
			}

			message += fmt.Sprintf(
				"账户: %s\n"+
					"%s %s %s %s张\n"+
					"收益率: %s%%  浮动盈亏: %s\n\n",
				acc.Name,
				func() string {
					if pos.PosSide == "short" {
						return "🔴"
					}
					return "🔵"
				}(),
				pos.InstId, pos.PosSide, pos.Pos,
				roiStr, pnlStr,
			)
		}
	}

	if !hasAnyPosition {
		return message + "📭 所有账户当前无持仓"
	}

	if totalMargin.IsPositive() {
		totalROI := totalPnl.Div(totalMargin).Mul(decimal.NewFromInt(100))
		totalROIStr := totalROI.StringFixed(2)
		if totalROI.IsPositive() {
			totalROIStr = "+" + totalROIStr
		}
		totalPnlStr := totalPnl.StringFixed(4)
		if totalPnl.IsPositive() {
			totalPnlStr = "+" + totalPnlStr
		}
		message += "━━━━━━━━━━━━━━━━━━━━\n"
		message += fmt.Sprintf("汇总收益率: %s%%\n汇总浮动盈亏: %s\n", totalROIStr, totalPnlStr)
	}

	return message
}

// startPositionMonitor 启动持仓盈亏监控（5秒一次）
func (am *AccountMonitor) startPositionMonitor() {
	ticker := time.NewTicker(am.posQueryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.checkPositionPnl()
		}
	}
}

// checkPositionPnl 检查所有账户持仓的盈亏比例，超过阈值则发送告警
func (am *AccountMonitor) checkPositionPnl() {
	if !trade.IsInitialized() {
		return
	}

	tm := trade.GetManager()
	if tm == nil {
		return
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return
	}

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			// 查询失败：跳过本账户（并跳过 GC，避免误清空已激活仓的状态）
			logrus.Debugf("[持仓监控] 账户 %s 查询失败: %v", acc.Name, err)
			continue
		}

		// P1(R3)：存活判定与"盈亏是否可评估"解耦——UPL/UseMargin 瞬空只跳过本轮
		// 评估，不得让仓位落出 liveKeys 被 GC 误删已激活的 trail 峰值。
		liveKeys := buildLiveKeys(acc.Name, posResp.Data)
		// P4：该账户首轮轮询时用实盘 posId 对账恢复 trail 状态，须在本轮评估之前
		// ——否则首轮会以空状态评估，已激活的峰值保护要等下一轮才回来。
		am.restoreAccountTrails(acc.Name, livePosIds(acc.Name, posResp.Data))
		if upl, complete := sumAccountUpl(posResp.Data); complete { // review fix#1：不完整轮保留旧缓存
			am.setUpl(acc.Name, upl, time.Now())
		}
		am.reconcileExternalCloses(acc, liveKeys) // P2-A：对账须在 GC 之前（保留 peak 供事件）
		for _, pos := range posResp.Data {
			if pos.UnrealizedProfit == "" || pos.UseMargin == "" {
				continue
			}
			pnl, ok := normalizePositionPnl(pos)
			if !ok {
				continue
			}
			margin, mErr := decimal.NewFromString(pos.UseMargin)
			if mErr != nil || !margin.IsPositive() {
				continue
			}
			pct := pnl.Div(margin).Mul(decimal.NewFromInt(100))
			key := fmt.Sprintf("%s:%s:%s", acc.Name, pos.InstId, pos.PosSide)

			if acc.IsTrailingTP() {
				am.handleTrailingPnl(tm, acc, pos, pnl, pct, key)
			} else {
				am.handleFixedPnl(tm, acc, pos, pnl, pct, key)
			}
		}

		// GC：仅对查询成功的账户，清理已不存在的 trail 状态（覆盖手动/外部平仓）
		am.gcTrailStates(acc.Name, liveKeys)
		am.updateSnapshots(acc.Name, posResp.Data)         // P2-A：刷新快照（含 P4 探针日志）
		am.closeFails.clearMissing(acc.Name+":", liveKeys) // P3/R4：仓位消失连败计数随生命周期清零

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// handleFixedPnl 现状逻辑（fixed 模式）：ROI 超过 profit_threshold 整仓平、
// 超过 -loss_threshold 仅告警；同仓告警 5 分钟冷却。行为与改造前一致。
func (am *AccountMonitor) handleFixedPnl(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, pnl, pct decimal.Decimal, key string) {
	isProfit := pct.GreaterThan(am.pnlProfitThreshold)
	isLoss := pct.LessThan(am.pnlLossThreshold.Neg())
	if !isProfit && !isLoss {
		return
	}
	if !am.passAlertCooldown(key) {
		return
	}
	pctStr := signedPctStr(pct)
	logrus.Warnf("[持仓监控] %s 盈亏比例 %s%% 超过阈值(盈利>%s%%/亏损<-%s%%)", key, pctStr, am.pnlProfitThreshold.String(), am.pnlLossThreshold.String())

	closeResultMsg := ""
	if isProfit {
		logrus.Infof("[持仓监控] 盈利超阈值，执行市价全平: %s", key)
		var closed bool
		closeResultMsg, closed = am.marketClosePosition(tm, acc, pos, key)
		if closed {
			// P2-B：fixed 平仓同样补 avgPx/lastPx 审计字段（无 trail 概念，peak 省略）
			eventlog.Log(buildCloseEvent(acc, pos, eventlog.EvFixedClose, pct, pnl, "", TrailState{}))
		}
	} else {
		eventlog.Log(eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: pos.InstId, Event: eventlog.EvLossAlert,
			Side: pos.PosSide, Size: absAtoi(pos.Pos), RoiPct: pct.InexactFloat64(), Pnl: pnl.InexactFloat64()})
	}
	am.sendPnlAlert(acc, pos, pnl, pct, closeResultMsg)
}

// handleTrailingPnl 移动止盈模式：分档移动止盈 + 兜底止损；保留普通亏损告警。
func (am *AccountMonitor) handleTrailingPnl(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, pnl, pct decimal.Decimal, key string) {
	pctF := pct.InexactFloat64()
	size := absAtoi(pos.Pos)
	lastPx, _ := strconv.ParseFloat(strings.TrimSpace(pos.LastPx), 64)
	tp := resolveTrailParamsForAccount(acc.Index) // 账户级移动止盈参数

	guard := tm.CapGuard()
	nmax, capOK := 0, false
	if guard != nil {
		guard.EnsureInit(acc.Name, trade.ResolveRiskEquity(acc), lastPx) // 重启后用 LastPx 兜底初始化；E=risk_equity(P5)
		nmax, capOK = guard.MaxContracts(acc.Name)
	}

	// 降级：cap 不可用 -> 跳过分档移动止盈；兜底止损与亏损告警照常（都不依赖 N_max）
	if !capOK {
		logrus.Warnf("[持仓监控] %s cap 未初始化，降级：跳过分档移动止盈", key)
		if pctF <= -tp.CatastropheStopPct {
			am.closeTrailing(tm, acc, pos, pnl, pct, key, "兜底止损(降级)")
			return
		}
		am.maybeLossAlert(acc, pos, pnl, pct, key)
		return
	}

	cfg := BuildExitConfig(nmax, tp)

	am.trailMu.Lock()
	prev := am.trailStates[key]
	action, newState := EvaluateExit(size, pctF, cfg, prev)
	am.trailStates[key] = newState
	changed := newState != prev
	am.trailMu.Unlock()
	if changed {
		am.markTrailDirty() // P4：峰值/激活态变化才需要重新落盘
	}

	switch action {
	case ActionTrailingClose:
		am.closeTrailing(tm, acc, pos, pnl, pct, key, "移动止盈")
	case ActionCatastropheStop:
		am.closeTrailing(tm, acc, pos, pnl, pct, key, "兜底止损")
	default: // ActionHold
		am.maybeLossAlert(acc, pos, pnl, pct, key)
	}
}

// closeTrailing 执行整仓平并发告警；平仓成功才删除 trail 状态。
func (am *AccountMonitor) closeTrailing(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, pnl, pct decimal.Decimal, key, reason string) {
	logrus.Warnf("[持仓监控] %s 触发%s (ROI %s%%)，执行市价全平", key, reason, signedPctStr(pct))
	msg, closed := am.marketClosePosition(tm, acc, pos, key)
	if closed {
		// P2-B：删除前捕获 trail 状态，平仓事件带 peakPct 审计字段（激活过才有）
		am.trailMu.Lock()
		st := am.trailStates[key]
		delete(am.trailStates, key)
		am.trailMu.Unlock()
		ev := eventlog.EvTrailingClose
		if strings.Contains(reason, "兜底") {
			ev = eventlog.EvCatastropheStop
		}
		eventlog.Log(buildCloseEvent(acc, pos, ev, pct, pnl, reason, st))
		am.sendPnlAlert(acc, pos, pnl, pct, fmt.Sprintf("\n\n🎯 触发: %s%s", reason, msg))
		return
	}
	// 平仓未成功：状态保留，下次扫描自动重试；告警节流避免每 5s 刷屏
	if am.passAlertCooldown(key) {
		am.sendPnlAlert(acc, pos, pnl, pct, fmt.Sprintf("\n\n🎯 触发: %s%s\n⚠️ 下次扫描将重试平仓", reason, msg))
	} else {
		logrus.Warnf("[持仓监控] %s 平仓未成功且在告警冷却内，跳过重复告警（仍会重试平仓）", key)
	}
}

// maybeLossAlert 普通亏损告警：ROI <= -loss_threshold 且过冷却才发（不平仓）。
func (am *AccountMonitor) maybeLossAlert(acc trade.AccountConfig, pos utils.PositionInfo, pnl, pct decimal.Decimal, key string) {
	if !pct.LessThan(am.pnlLossThreshold.Neg()) {
		return
	}
	if !am.passAlertCooldown(key) {
		return
	}
	eventlog.Log(eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: pos.InstId, Event: eventlog.EvLossAlert,
		Side: pos.PosSide, Size: absAtoi(pos.Pos), RoiPct: pct.InexactFloat64(), Pnl: pnl.InexactFloat64()})
	am.sendPnlAlert(acc, pos, pnl, pct, "")
}

// gcTrailStates 清理某账户下已不存在（或 pos=0）的 trail 状态。
func (am *AccountMonitor) gcTrailStates(accName string, liveKeys map[string]bool) {
	prefix := accName + ":"
	am.trailMu.Lock()
	removed := 0
	for k := range am.trailStates {
		if strings.HasPrefix(k, prefix) && !liveKeys[k] {
			delete(am.trailStates, k)
			removed++
		}
	}
	am.trailMu.Unlock()
	if removed > 0 {
		am.markTrailDirty() // P4：仓位消失后快照须同步移除，避免重启复活死仓状态
	}
}

// passAlertCooldown 同一持仓告警冷却：过冷却返回 true 并记录本次时间。
func (am *AccountMonitor) passAlertCooldown(key string) bool {
	return am.passCooldown(key, am.pnlAlertCooldown)
}

// marketClosePosition 市价全平 + 重置信号状态；返回告警文案与是否成功平仓。
func (am *AccountMonitor) marketClosePosition(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, key string) (string, bool) {
	primary, backup := tm.WebCloser(acc.Name), tm.NativeCloser(acc.Name)
	if primary == nil && backup == nil {
		// P3/R4：无任何通道=配置故障，立即升级告警（不进 3 次计数）
		am.escalateCloseFail(acc, pos, key, "无平仓通道配置(web/native 均缺失, 配置故障)", 0, 0)
		return "\n\n⚠️ 自动平仓: 跳过（无平仓通道配置）", false
	}
	if pos.PosId == "" {
		am.escalateCloseFail(acc, pos, key, "无PositionID(数据故障)", 0, 0)
		return "\n\n⚠️ 自动平仓: 跳过（无PositionID）", false
	}

	out := trade.CloseWithFallback(primary, backup, trade.ClosePosArgs{
		InstId: pos.InstId, PosId: pos.PosId,
		LastPx: parsePxOrZero(pos.LastPx), Size: absAtoi(pos.Pos), // 埋点用（快照价，非成交价）
	})
	if !out.OK() {
		logrus.Errorf("[持仓监控] 市价全平失败: %s, err=%v", key, out.Err())
		// 连败只在双通道都失败时累加：P3 告警语义是"安全网停摆"，
		// 备用通道兜住时仓位已平，不算停摆（否则会刷出海量误报）
		if count, since := am.closeFails.recordFail(key); count >= 3 {
			am.escalateCloseFail(acc, pos, key, out.Err().Error(), count, since)
		}
		return closeResultMessage(out), false
	}
	am.closeFails.recordSuccess(key)
	logrus.Infof("[持仓监控] 市价全平成功: %s, 通道=%s 降级=%v", key, out.Channel, out.Degraded)
	trade.MarkBotClose(acc.Name, pos.InstId, pos.PosSide, pos.PosId) // P2-A：自家平仓打标，对账不误报 external
	ResetSignalStateAfterClose(pos.InstId, pos.PosSide)
	if out.Degraded {
		am.alertChannelDegraded(acc, pos, out)
	}
	return closeResultMessage(out), true
}

// alertChannelDegraded 主通道失效但备用通道兜住时的独立告警（节流 30 分钟）。
// 不紧急（仓位已平）但必须可见：开仓仍依赖 Web 会话，cookie 不修就会持续失败。
func (am *AccountMonitor) alertChannelDegraded(acc trade.AccountConfig, pos utils.PositionInfo, out trade.CloseOutcome) {
	if !am.passCooldown(acc.Name+"#channel_degraded", 30*time.Minute) {
		return
	}
	msg := fmt.Sprintf("🟡 平仓通道降级告警\n⏰ %s\n\n账户: %s\n持仓: %s %s\n"+
		"Web通道失败: %v\n已由原生API通道(%s)成功平仓，仓位安全。\n\n"+
		"⚠️ 开仓仍走 Web 会话，请尽快更新 cookie/token",
		time.Now().Format("2006-01-02 15:04:05"), acc.Name, pos.InstId, pos.PosSide, out.PrimaryErr, out.Channel)
	logrus.Warn(msg)
	if _, err := am.telegramClient.SendMessage(msg); err != nil {
		logrus.Errorf("[降级告警] 发送失败: %v", err)
	}
}

// recordManualClose TG 平仓成功后落 manual_close 事件 + 注册表打标（P2-A）。
// 事件字段同 fixed_close（trail 若激活则附 peakPct）；状态清理交给下一轮 GC。
func (am *AccountMonitor) recordManualClose(acc trade.AccountConfig, pos utils.PositionInfo, reason string) {
	pnl, _ := normalizePositionPnl(pos)
	pct := decimal.Zero
	if margin, err := decimal.NewFromString(strings.TrimSpace(pos.UseMargin)); err == nil && margin.IsPositive() {
		pct = pnl.Div(margin).Mul(decimal.NewFromInt(100))
	}
	key := fmt.Sprintf("%s:%s:%s", acc.Name, pos.InstId, pos.PosSide)
	am.trailMu.RLock()
	st := am.trailStates[key]
	am.trailMu.RUnlock()
	eventlog.Log(buildCloseEvent(acc, pos, eventlog.EvManualClose, pct, pnl, reason, st))
	trade.MarkBotClose(acc.Name, pos.InstId, pos.PosSide, pos.PosId)
}

// sendPnlAlert 发送标准持仓盈亏告警（extra 追加自动平仓结果等）。
func (am *AccountMonitor) sendPnlAlert(acc trade.AccountConfig, pos utils.PositionInfo, pnl, pct decimal.Decimal, extra string) {
	accountDisplay := acc.Name
	if acc.UID != "" {
		accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
	}
	directionEmoji := "🔵"
	if pos.PosSide == "short" {
		directionEmoji = "🔴"
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	alertMsg := fmt.Sprintf(
		"🚨 持仓盈亏告警\n"+
			"⏰ %s\n\n"+
			"账户: %s\n"+
			"%s %s  方向:%s\n"+
			"持仓:%s张  开仓均价:%s\n"+
			"最新价:%s  强平价:%s\n"+
			"占用保证金:%s\n"+
			"未实现盈亏: %s (%s%%)%s\n",
		now, accountDisplay,
		directionEmoji, pos.InstId, pos.PosSide,
		pos.Pos, pos.AvgPx,
		pos.LastPx, pos.LiqPx,
		pos.UseMargin,
		pnl.String(), signedPctStr(pct), extra,
	)
	if success, err := am.telegramClient.SendMessage(alertMsg); err != nil {
		logrus.Errorf("[持仓监控] 发送告警消息失败: %v", err)
	} else if success {
		logrus.Infof("[持仓监控] 告警消息已发送")
	}
}

// signedPctStr 把 ROI 百分比格式化为带符号、两位小数的字符串。
func signedPctStr(pct decimal.Decimal) string {
	s := pct.StringFixed(2)
	if pct.IsPositive() {
		s = "+" + s
	}
	return s
}

// absAtoi 解析张数字符串为非负整数（short 为负张数，取绝对值）。
func absAtoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	if n < 0 {
		n = -n
	}
	return n
}

// ClosePositionsBySide 平仓指定方向的所有持仓
func (am *AccountMonitor) ClosePositionsBySide(posSide string) string {
	posSide = strings.ToLower(strings.TrimSpace(posSide))
	if posSide != "long" && posSide != "short" {
		return "⚠️ 平仓方向非法，仅支持 long 或 short"
	}

	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法执行平仓"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("🔄 %s方向平仓执行报告\n⏰ 时间: %s\n\n", posSide, now)
	totalClosed := 0
	totalFailed := 0
	totalSkipped := 0

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户 %s: 客户端不存在\n\n", acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			logrus.Errorf("[一键平仓] 账户 %s 查询持仓失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户 %s: 查询持仓失败: %v\n\n", acc.Name, err)
			continue
		}

		if len(posResp.Data) == 0 {
			message += fmt.Sprintf("✅ 账户 %s: 暂无持仓\n\n", acc.Name)
			continue
		}

		accountDisplay := acc.Name
		if acc.UID != "" {
			accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
		}
		message += fmt.Sprintf("📋 账户 %s\n", accountDisplay)

		matched := 0
		for _, pos := range posResp.Data {
			if !strings.EqualFold(pos.PosSide, posSide) {
				continue
			}
			matched++
			line, res := am.manualClosePosition(tm, acc, pos, "tg方向平仓")
			message += line
			switch res {
			case manualClosed:
				totalClosed++
			case manualFailed:
				totalFailed++
			default:
				totalSkipped++
			}
		}
		if matched == 0 {
			message += fmt.Sprintf("  ✅ 暂无%s持仓\n", posSide)
		}
		message += "\n"

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	message += "━━━━━━━━━━━━━━━━━━━━\n"
	message += fmt.Sprintf("📊 汇总: 成功=%d, 失败=%d, 跳过=%d\n", totalClosed, totalFailed, totalSkipped)

	return message
}

// CloseAllPositions 一键平仓所有账户的所有持仓
func (am *AccountMonitor) CloseAllPositions() string {
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法执行平仓"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("🔄 一键平仓执行报告\n⏰ 时间: %s\n\n", now)
	totalClosed := 0
	totalFailed := 0
	totalSkipped := 0

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户 %s: 客户端不存在\n\n", acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			logrus.Errorf("[一键平仓] 账户 %s 查询持仓失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户 %s: 查询持仓失败: %v\n\n", acc.Name, err)
			continue
		}

		if len(posResp.Data) == 0 {
			message += fmt.Sprintf("✅ 账户 %s: 暂无持仓\n\n", acc.Name)
			continue
		}

		accountDisplay := acc.Name
		if acc.UID != "" {
			accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
		}
		message += fmt.Sprintf("📋 账户 %s (%d个持仓)\n", accountDisplay, len(posResp.Data))

		for _, pos := range posResp.Data {
			line, res := am.manualClosePosition(tm, acc, pos, "tg一键平仓")
			message += line
			switch res {
			case manualClosed:
				totalClosed++
			case manualFailed:
				totalFailed++
			default:
				totalSkipped++
			}
		}
		message += "\n"

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	message += "━━━━━━━━━━━━━━━━━━━━\n"
	message += fmt.Sprintf("📊 汇总: 成功=%d, 失败=%d, 跳过=%d\n", totalClosed, totalFailed, totalSkipped)

	return message
}
