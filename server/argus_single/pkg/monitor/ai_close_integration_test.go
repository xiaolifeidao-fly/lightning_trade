package monitor

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"common/utils"

	"github.com/shopspring/decimal"
)

// TestAICloseFullFlow 是 AI 平仓逻辑的端到端集成测试入口。
//
// 运行命令：
//
//	cd argus_single
//	go test ./pkg/monitor/... -v -run TestAICloseFullFlow -timeout 60s
func TestAICloseFullFlow(t *testing.T) {
	sep := strings.Repeat("─", 60)

	// ── Step 1: 读取 application.properties 中的 AI 配置 ─────────────────
	t.Log(sep)
	t.Log("STEP 1  读取 AI 配置 (application.properties)")
	t.Log(sep)

	cfg, err := loadAICloseConfig()
	if err != nil {
		t.Fatalf("读取 AI 配置失败: %v", err)
	}
	t.Logf("  api_url : %s", cfg.apiURL)
	t.Logf("  model   : %s", cfg.model)
	t.Logf("  api_key : %s...（已隐藏）", cfg.apiKey[:min(12, len(cfg.apiKey))])

	// ── Step 2: 从 Binance 拉取真实 BTC 行情 ─────────────────────────────
	t.Log(sep)
	t.Log("STEP 2  拉取 BTC 行情数据 (Binance REST)")
	t.Log(sep)

	feed := &BTCMarketDataFeed{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		klines: map[string][]BTCKline{
			"1h": make([]BTCKline, 0, btcMarketDefaultKlineSize),
			"4h": make([]BTCKline, 0, btcMarketDefaultKlineSize),
			"1d": make([]BTCKline, 0, btcMarketDefaultKlineSize),
		},
		rollingTickers: make(map[string]*binanceRollingTicker),
	}
	if err := feed.bootstrap(); err != nil {
		t.Fatalf("Binance bootstrap 失败（网络不通？）: %v", err)
	}
	t.Logf("  1H K线: %d 条  4H K线: %d 条  日线: %d 条",
		len(feed.klines["1h"]), len(feed.klines["4h"]), len(feed.klines["1d"]))

	// ── Step 3: 计算技术指标，组装 BTCAnalysisSnapshot ───────────────────
	t.Log(sep)
	t.Log("STEP 3  计算技术指标")
	t.Log(sep)

	snapshot, err := buildSnapshotFromFeed(feed)
	if err != nil {
		t.Fatalf("组装 BTCAnalysisSnapshot 失败: %v", err)
	}
	logSnapshot(t, snapshot)

	// ── Step 4: 拉取真实持仓，构建仓位快照 ──────────────────────────────
	t.Log(sep)
	t.Log("STEP 4  从 DeepCoin API 拉取真实持仓")
	t.Log(sep)

	accCfg, err := loadAccountConfig()
	if err != nil {
		t.Fatalf("读取账户配置失败: %v", err)
	}
	t.Logf("  账户: %s  uid: %s", accCfg.name, accCfg.uid)

	dcClient := utils.NewDeepCoinClient(accCfg.apiKey, accCfg.secretKey, accCfg.passphrase)
	posResp, err := dcClient.GetPositionsTyped(&utils.GetPositionsRequest{InstType: "SWAP"})
	if err != nil {
		t.Fatalf("获取真实持仓失败: %v", err)
	}
	t.Logf("  持仓数量: %d", len(posResp.Data))
	for i, p := range posResp.Data {
		t.Logf("  [%d] instId=%s side=%s pos=%s avgPx=%s lastPx=%s upl=%s margin=%s lever=%s liqPx=%s",
			i, p.InstId, p.PosSide, p.Pos, p.AvgPx, p.LastPx, p.UnrealizedProfit, p.UseMargin, p.Lever, p.LiqPx)
	}

	var posSnap PositionSnapshot
	if len(posResp.Data) == 0 {
		t.Log("  当前无持仓，继续执行 AI 空仓分析")
		posSnap = buildNoPositionSnapshot(accCfg, snapshot)
	} else {
		// 取第一个持仓（优先 BTC-USDT-SWAP）
		realPos := posResp.Data[0]
		for _, p := range posResp.Data {
			if p.InstId == "BTC-USDT-SWAP" {
				realPos = p
				break
			}
		}
		t.Logf("  使用持仓: instId=%s side=%s pos=%s", realPos.InstId, realPos.PosSide, realPos.Pos)
		posSnap = buildPositionSnapshotFromReal(realPos, accCfg, snapshot)
	}

	// ── Step 5: 生成 AI Prompt ────────────────────────────────────────────
	t.Log(sep)
	t.Log("STEP 5  生成 AI Prompt")
	t.Log(sep)

	prompt, err := BuildAIClosePrompt("", posSnap)
	if err != nil {
		t.Fatalf("BuildAIClosePrompt 失败: %v", err)
	}
	t.Logf("  Prompt 长度: %d 字符\n", len(prompt))
	t.Log(prompt)

	// ── Step 6: 调用真实 AI 决策 ─────────────────────────────────────────
	t.Log(sep)
	t.Log("STEP 6  调用 AI 决策")
	t.Log(sep)
	actualEndpoint := strings.TrimRight(cfg.apiURL, "/")
	if !strings.HasSuffix(actualEndpoint, "/chat/completions") {
		actualEndpoint += "/chat/completions"
	}
	t.Logf("  Model      : %s", cfg.model)
	t.Logf("  API URL    : %s", cfg.apiURL)
	t.Logf("  实际请求地址: %s", actualEndpoint)
	t.Logf("  API Key    : %s...", cfg.apiKey[:min(12, len(cfg.apiKey))])

	decider := &Tu2doCloseDecider{
		client:      &http.Client{Timeout: time.Duration(cfg.timeoutSeconds) * time.Second},
		apiURL:      cfg.apiURL,
		apiKey:      cfg.apiKey,
		model:       cfg.model,
		temperature: cfg.temperature,
		maxTokens:   cfg.maxTokens,
		promptTpl:   defaultAIClosePromptTemplate,
	}

	decision, err := decider.Decide(posSnap)
	if err != nil {
		t.Fatalf("AI Decide 调用失败: %v", err)
	}

	t.Log("\n── AI 决策结果 ──")
	t.Log(formatAICloseDecision(decision))
	t.Logf("\n── 持仓建议 ──")
	if decision.LongSuggestedHold != "" {
		t.Logf("  做多建议持仓: %s", decision.LongSuggestedHold)
	} else {
		t.Log("  做多建议持仓: (AI未返回)")
	}
	if decision.ShortSuggestedHold != "" {
		t.Logf("  做空建议持仓: %s", decision.ShortSuggestedHold)
	} else {
		t.Log("  做空建议持仓: (AI未返回)")
	}
	if decision.NextCheckIn != "" {
		t.Logf("  下次风险复检: %s后", decision.NextCheckIn)
	} else {
		t.Log("  下次风险复检: (AI未返回)")
	}
	t.Logf("\nRaw JSON:\n%s", decision.RawResponse)

	if decision.RiskLevel == "" {
		t.Error("RiskLevel 不应为空")
	}
	if decision.Confidence.IsNegative() {
		t.Error("Confidence 不应为负数")
	}
	if decision.ContinueSide == "" {
		t.Error("ContinueSide 不应为空")
	}

	// ── Step 7: 发送 AI 决策结果到 Telegram ──────────────────────────────
	t.Log(sep)
	t.Log("STEP 7  发送 AI 决策结果到 Telegram")
	t.Log(sep)

	tgCfg, err := loadTelegramConfig()
	if err != nil {
		t.Logf("  ⚠️ 未配置 Telegram，跳过发送: %v", err)
	} else {
		msg := buildAIDecisionTelegramMessage(posSnap, decision)
		tgClient := utils.NewTelegramClientWithBotTokenAndChatID(tgCfg.botToken, tgCfg.chatID)
		ok, tgErr := tgClient.SendMessage(msg)
		if tgErr != nil {
			t.Logf("  ⚠️ Telegram 发送失败: %v", tgErr)
		} else if ok {
			t.Logf("  ✅ Telegram 消息发送成功 (chat_id=%s)", tgCfg.chatID)
		}
	}

	t.Log(sep)
	t.Log("✅ 集成测试完成（BTC数据 + Prompt + AI 调用均正常；有仓位走平仓分析，无仓位走空仓分析）")
}

// ─── AI 配置读取 ──────────────────────────────────────────────────────────────

type aiCloseTestConfig struct {
	apiURL         string
	apiKey         string
	model          string
	timeoutSeconds int
	maxTokens      int
	temperature    float64
}

func loadAICloseConfig() (*aiCloseTestConfig, error) {
	propsPath := resolvePropsPath()
	f, err := os.Open(propsPath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败 (%s): %w", propsPath, err)
	}
	defer f.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		props[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	apiURL := props["position.ai_close.api_url"]
	apiKey := props["position.ai_close.api_key"]
	model := props["position.ai_close.model"]
	if apiURL == "" {
		return nil, fmt.Errorf("position.ai_close.api_url 未配置")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("position.ai_close.api_key 未配置")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	timeoutSeconds := 20
	if v, err := strconv.Atoi(props["position.ai_close.timeout_seconds"]); err == nil && v > 0 {
		timeoutSeconds = v
	}
	maxTokens := 900
	if v, err := strconv.Atoi(props["position.ai_close.max_tokens"]); err == nil && v > 0 {
		maxTokens = v
	}
	temperature := 0.2
	if v, err := strconv.ParseFloat(props["position.ai_close.temperature"], 64); err == nil && v > 0 {
		temperature = v
	}

	return &aiCloseTestConfig{
		apiURL:         apiURL,
		apiKey:         apiKey,
		model:          model,
		timeoutSeconds: timeoutSeconds,
		maxTokens:      maxTokens,
		temperature:    temperature,
	}, nil
}

// resolvePropsPath 从测试文件位置向上找 configs/application.properties
func resolvePropsPath() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	// pkg/monitor/ → argus_single/configs/
	return filepath.Join(dir, "..", "..", "configs", "application.properties")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── 私有辅助：从 feed 组装 snapshot ─────────────────────────────────────────

func buildSnapshotFromFeed(feed *BTCMarketDataFeed) (*BTCAnalysisSnapshot, error) {
	feed.mu.RLock()
	klines1H := append([]BTCKline(nil), feed.klines["1h"]...)
	klines4H := append([]BTCKline(nil), feed.klines["4h"]...)
	klines1D := append([]BTCKline(nil), feed.klines["1d"]...)
	feed.mu.RUnlock()

	if len(klines1H) == 0 || len(klines4H) == 0 || len(klines1D) == 0 {
		return nil, fmt.Errorf("K线数据为空")
	}

	todayInfo, _ := feed.getTodayInfo()

	closes1H, err := klineCloses(klines1H)
	if err != nil {
		return nil, err
	}
	closes4H, err := klineCloses(klines4H)
	if err != nil {
		return nil, err
	}
	closes1D, err := klineCloses(klines1D)
	if err != nil {
		return nil, err
	}

	snap := &BTCAnalysisSnapshot{
		Symbol:    BTCSymbol,
		Klines1H:  klines1H,
		Klines4H:  klines4H,
		Klines1D:  klines1D,
		TodayInfo: todayInfo,
	}

	snap.EMA1H20 = buildMovingAverage(BTCSymbol, "1h", 20, "close", closes1H, calculateEMA)
	snap.EMA1H50 = buildMovingAverage(BTCSymbol, "1h", 50, "close", closes1H, calculateEMA)
	snap.EMA4H20 = buildMovingAverage(BTCSymbol, "4h", 20, "close", closes4H, calculateEMA)
	snap.EMA4H50 = buildMovingAverage(BTCSymbol, "4h", 50, "close", closes4H, calculateEMA)
	snap.EMA1D20 = buildMovingAverage(BTCSymbol, "1d", 20, "close", closes1D, calculateEMA)
	snap.EMA1D50 = buildMovingAverage(BTCSymbol, "1d", 50, "close", closes1D, calculateEMA)
	snap.MA1D200 = buildMovingAverage(BTCSymbol, "1d", 200, "close", closes1D, calculateSMA)

	snap.MACD1H = buildMACD(BTCSymbol, "1h", closes1H, 12, 26, 9)
	snap.MACD4H = buildMACD(BTCSymbol, "4h", closes4H, 12, 26, 9)
	snap.MACD1D = buildMACD(BTCSymbol, "1d", closes1D, 12, 26, 9)

	snap.RSI1H14 = buildRSI(BTCSymbol, "1h", closes1H, 14)
	snap.RSI4H14 = buildRSI(BTCSymbol, "4h", closes4H, 14)
	snap.RSI1D14 = buildRSI(BTCSymbol, "1d", closes1D, 14)

	snap.ATR1H14 = buildATR(BTCSymbol, "1h", klines1H, 14)
	snap.ATR4H14 = buildATR(BTCSymbol, "4h", klines4H, 14)
	snap.ATR1D14 = buildATR(BTCSymbol, "1d", klines1D, 14)

	snap.Bollinger1H = buildBollingerBands(BTCSymbol, "1h", closes1H, 20, "2")
	snap.Bollinger4H = buildBollingerBands(BTCSymbol, "4h", closes4H, 20, "2")
	snap.Bollinger1D = buildBollingerBands(BTCSymbol, "1d", closes1D, 20, "2")

	snap.Volume1H = buildVolumeProfile(BTCSymbol, "1h", klines1H, 20)
	snap.Volume4H = buildVolumeProfile(BTCSymbol, "4h", klines4H, 20)
	snap.Volume1D = buildVolumeProfile(BTCSymbol, "1d", klines1D, 20)

	return snap, nil
}

func logSnapshot(t *testing.T, s *BTCAnalysisSnapshot) {
	t.Helper()
	if s.TodayInfo != nil {
		t.Logf("  当前价: %s  涨跌幅: %s%%  高: %s  低: %s",
			s.TodayInfo.CurrentPrice, s.TodayInfo.TodayChangePercent,
			s.TodayInfo.TodayHighPrice, s.TodayInfo.TodayLowPrice)
	}
	logMA := func(name string, ma *BTCMovingAverage) {
		if ma != nil {
			t.Logf("  %-12s = %s", name, ma.Value)
		}
	}
	logMA("EMA_1H_20", s.EMA1H20)
	logMA("EMA_1H_50", s.EMA1H50)
	logMA("EMA_4H_20", s.EMA4H20)
	logMA("EMA_4H_50", s.EMA4H50)
	logMA("EMA_1D_20", s.EMA1D20)
	logMA("EMA_1D_50", s.EMA1D50)
	logMA("MA_1D_200", s.MA1D200)
	if s.MACD1H != nil {
		t.Logf("  MACD_1H      macd=%s  signal=%s  hist=%s", s.MACD1H.MACD, s.MACD1H.Signal, s.MACD1H.Histogram)
	}
	if s.MACD4H != nil {
		t.Logf("  MACD_4H      macd=%s  signal=%s  hist=%s", s.MACD4H.MACD, s.MACD4H.Signal, s.MACD4H.Histogram)
	}
	if s.RSI1H14 != nil {
		t.Logf("  RSI_1H_14    = %s", s.RSI1H14.Value)
	}
	if s.RSI4H14 != nil {
		t.Logf("  RSI_4H_14    = %s", s.RSI4H14.Value)
	}
	if s.RSI1D14 != nil {
		t.Logf("  RSI_1D_14    = %s", s.RSI1D14.Value)
	}
	if s.ATR1H14 != nil {
		t.Logf("  ATR_1H_14    = %s", s.ATR1H14.Value)
	}
	if s.Bollinger1H != nil {
		t.Logf("  BOLL_1H      mid=%s  up=%s  lo=%s", s.Bollinger1H.MiddleBand, s.Bollinger1H.UpperBand, s.Bollinger1H.LowerBand)
	}
	if s.Volume1H != nil {
		t.Logf("  VOL_1H       cur=%s  avg=%s  ratio=%s", s.Volume1H.CurrentVolume, s.Volume1H.AverageVolume, s.Volume1H.VolumeRatio)
	}
}

// ─── 账户配置读取 ─────────────────────────────────────────────────────────────

type accountTestConfig struct {
	name       string
	uid        string
	apiKey     string
	secretKey  string
	passphrase string
}

func loadAccountConfig() (*accountTestConfig, error) {
	propsPath := resolvePropsPath()
	f, err := os.Open(propsPath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败 (%s): %w", propsPath, err)
	}
	defer f.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		props[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	apiKey := props["trade.account1.api_key"]
	secretKey := props["trade.account1.secret_key"]
	passphrase := props["trade.account1.passphrase"]
	if apiKey == "" || secretKey == "" {
		return nil, fmt.Errorf("trade.account1.api_key / secret_key 未配置")
	}
	return &accountTestConfig{
		name:       props["trade.account1.name"],
		uid:        props["trade.account1.uid"],
		apiKey:     apiKey,
		secretKey:  secretKey,
		passphrase: passphrase,
	}, nil
}

// ─── 真实持仓 → PositionSnapshot ─────────────────────────────────────────────

func buildPositionSnapshotFromReal(pos utils.PositionInfo, acc *accountTestConfig, market *BTCAnalysisSnapshot) PositionSnapshot {
	uplF, _ := strconv.ParseFloat(pos.UnrealizedProfit, 64)
	marginF, _ := strconv.ParseFloat(pos.UseMargin, 64)
	var pnlPct decimal.Decimal
	if marginF != 0 {
		pnlPct = decimal.NewFromFloat(uplF / marginF * 100)
	}

	triggerType := "scheduled"
	if pnlPct.GreaterThan(decimal.NewFromFloat(100)) {
		triggerType = "profit"
	} else if pnlPct.LessThan(decimal.NewFromFloat(-50)) {
		triggerType = "loss"
	}

	return PositionSnapshot{
		AccountName:      acc.name,
		AccountUID:       acc.uid,
		HasPosition:      true,
		InstID:           pos.InstId,
		PositionID:       pos.PosId,
		PositionSide:     pos.PosSide,
		PositionSize:     pos.Pos,
		AvgPrice:         pos.AvgPx,
		LastPrice:        pos.LastPx,
		LiqPrice:         pos.LiqPx,
		UseMargin:        pos.UseMargin,
		UnrealizedProfit: pos.UnrealizedProfit,
		PnLPercent:       pnlPct,
		TriggerType:      triggerType,
		BTCMarket:        market,
		CurrentPosition: CurrentPositionDetails{
			InstType:         pos.InstType,
			InstID:           pos.InstId,
			PositionID:       pos.PosId,
			PositionSide:     pos.PosSide,
			PositionSize:     pos.Pos,
			AvgPrice:         pos.AvgPx,
			LastPrice:        pos.LastPx,
			LiqPrice:         pos.LiqPx,
			UseMargin:        pos.UseMargin,
			UnrealizedProfit: pos.UnrealizedProfit,
			PnLPercent:       pnlPct.StringFixed(2),
			Leverage:         pos.Lever,
			MarginMode:       pos.MgnMode,
			MarginPosition:   pos.MrgPosition,
			Currency:         pos.Ccy,
			CreateTime:       pos.CTime,
			UpdateTime:       pos.UTime,
		},
	}
}

func buildNoPositionSnapshot(acc *accountTestConfig, market *BTCAnalysisSnapshot) PositionSnapshot {
	return PositionSnapshot{
		AccountName:     acc.name,
		AccountUID:      acc.uid,
		HasPosition:     false,
		BTCMarket:       market,
		PositionSummary: "no_open_position=true",
		TriggerType:     "no_position",
		PnLPercent:      decimal.Zero,
	}
}

// ─── Telegram 配置 ────────────────────────────────────────────────────────────

type telegramTestConfig struct {
	botToken string
	chatID   string
}

func loadTelegramConfig() (*telegramTestConfig, error) {
	propsPath := resolvePropsPath()
	f, err := os.Open(propsPath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer f.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		props[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	botToken := props["telegram.bot_token"]
	chatID := props["telegram.chat_id"]
	if botToken == "" || chatID == "" {
		return nil, fmt.Errorf("telegram.bot_token / chat_id 未配置")
	}
	return &telegramTestConfig{botToken: botToken, chatID: chatID}, nil
}

func buildAIDecisionTelegramMessage(snap PositionSnapshot, decision *AICloseDecision) string {
	riskIcons := map[string]string{"low": "🟢", "medium": "🟡", "high": "🔴"}
	holdLine := func(side, hold string) string {
		if hold == "" {
			return fmt.Sprintf("%s胜率持仓建议: -", side)
		}
		if hold == "不建议" {
			return fmt.Sprintf("%s胜率持仓建议: 不建议", side)
		}
		return fmt.Sprintf("%s胜率持仓建议: %s", side, hold)
	}
	nextCheckLine := func() string {
		if decision.NextCheckIn == "" {
			return "下次风险复检: -"
		}
		return fmt.Sprintf("下次风险复检: %s后", decision.NextCheckIn)
	}

	if !snapshotHasPosition(snap) {
		riskIcon := riskIcons[decision.RiskLevel]
		if riskIcon == "" {
			riskIcon = "⚪"
		}
		return fmt.Sprintf(
			"🤖 AI 空仓分析报告\n\n"+
				"账户: %s\n"+
				"最终动作: %s\n"+
				"建议方向: %s\n"+
				"做多胜率: %s%%  做空胜率: %s%%\n"+
				"%s\n"+
				"%s\n"+
				"%s\n"+
				"置信度: %s%%  %s 风险: %s\n"+
				"模型: %s\n\n"+
				"📋 原因:\n%s",
			defaultString(snap.AccountName, "unknown"),
			decision.FinalAction,
			decision.ContinueSide,
			decision.LongWinRate.StringFixed(1), decision.ShortWinRate.StringFixed(1),
			holdLine("做多", decision.LongSuggestedHold),
			holdLine("做空", decision.ShortSuggestedHold),
			nextCheckLine(),
			decision.Confidence.StringFixed(1), riskIcon, decision.RiskLevel,
			decision.Model,
			decision.Reason,
		)
	}

	closeIcon := "🟢"
	if decision.ShouldClose {
		closeIcon = "🔴"
	}
	riskIcon := riskIcons[decision.RiskLevel]
	if riskIcon == "" {
		riskIcon = "⚪"
	}

	return fmt.Sprintf(
		"🤖 AI 平仓决策报告\n\n"+
			"仓位: %s %s  盈亏: %s%%\n"+
			"均价: %s  最新: %s  强平: %s\n\n"+
			"%s 建议平仓: %v\n"+
			"继续方向: %s\n"+
			"做多胜率: %s%%  做空胜率: %s%%\n"+
			"%s\n"+
			"%s\n"+
			"%s\n"+
			"置信度: %s%%  %s 风险: %s\n"+
			"模型: %s\n\n"+
			"📋 原因:\n%s",
		snap.InstID, snap.PositionSide, snap.PnLPercent.StringFixed(2),
		snap.AvgPrice, snap.LastPrice, snap.LiqPrice,
		closeIcon, decision.ShouldClose,
		decision.ContinueSide,
		decision.LongWinRate.StringFixed(1), decision.ShortWinRate.StringFixed(1),
		holdLine("做多", decision.LongSuggestedHold),
		holdLine("做空", decision.ShortSuggestedHold),
		nextCheckLine(),
		decision.Confidence.StringFixed(1), riskIcon, decision.RiskLevel,
		decision.Model,
		decision.Reason,
	)
}
