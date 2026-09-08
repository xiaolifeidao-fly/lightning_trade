package strategy

import (
	commonRouter "common/middleware/routers"
	tradeService "service/trade"
	tradeDTO "service/trade/dto"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StrategyHandler struct {
	*commonRouter.BaseHandler
	tradeService *tradeService.TradeService
}

func NewStrategyHandler() *StrategyHandler {
	svc := tradeService.NewTradeService()
	_ = svc.EnsureTable()
	return &StrategyHandler{
		BaseHandler:  &commonRouter.BaseHandler{},
		tradeService: svc,
	}
}

func (h *StrategyHandler) RegisterHandler(engine *gin.RouterGroup) {
	// Strategy CRUD
	engine.GET("/strategies", h.listStrategies)
	engine.POST("/strategies", h.createStrategy)
	engine.GET("/strategies/:id", h.getStrategy)
	engine.PUT("/strategies/:id", h.updateStrategy)
	engine.DELETE("/strategies/:id", h.deleteStrategy)

	// Position queries — summary must be registered before :id to avoid route conflict
	engine.GET("/strategy-positions/summary", h.getPositionSummary)
	engine.GET("/strategy-positions", h.listPositions)
	engine.GET("/strategy-positions/:id", h.getPosition)
	engine.POST("/strategy-positions/:id/close", h.manualClosePosition)

	// Market price (reads directly from Binance REST, hub is oracle-only)
	engine.GET("/market/price", h.getMarketPrice)

	// Backtest（策略回测层）
	engine.POST("/backtest/runs", h.createBacktestRun)
	engine.GET("/backtest/runs", h.listBacktestRuns)
	engine.GET("/backtest/runs/:id", h.getBacktestRun)
	engine.GET("/backtest/metrics", h.getBacktestMetrics)
	// 盘口信号回测（事件驱动 + 精度分层）。读侧复用上面的 runs/:id 与 metrics
	// 两个接口——同一张表、同一套 DTO，只是 engineKind=signal。
	engine.POST("/backtest/signal-runs", h.createSignalBacktestRun)
	engine.GET("/backtest/signal-sources", h.listSignalSources)
	// 参数组批量扫描与横向对比（r11）。一批 = 一条 batch + N 条 run，
	// 单组详情仍走上面的 runs/:id，不另造读接口。
	engine.POST("/backtest/signal-batches", h.createSignalBacktestBatch)
	engine.GET("/backtest/signal-batches", h.listSignalBacktestBatches)
	engine.GET("/backtest/signal-batches/:id", h.getSignalBacktestBatch)
	// 基线预览：提交批次前先看清"与什么比"，含哪些字段是兜底来的。
	engine.GET("/backtest/signal-baseline", h.getSignalBaseline)
	// 后台自动参数寻优（r16）：粗网格 → 自动收敛精算格 → 16 路径降噪协议 →
	// 预注册三关判定 → 无解分支出权衡前沿。不输出"最优参数"、不改线上参数。
	engine.POST("/backtest/optimize-studies", h.createSignalOptimizeStudy)
	engine.GET("/backtest/optimize-studies", h.listSignalOptimizeStudies)
	engine.GET("/backtest/optimize-defaults", h.getSignalOptimizeDefaults)
	engine.GET("/backtest/optimize-studies/:id", h.getSignalOptimizeStudy)
	// 「K线详情」预测增强：复合方向 + 预测周期 K 线
	engine.GET("/backtest/prediction-detail", h.getPredictionDetail)

	// K 线回填（按币种/周期/最近 N 根增量拉取入库，供回测使用）
	engine.POST("/klines/backfill", h.backfillKlines)
	// K 线窗口回填（按时间窗口覆盖率补历史空洞，供行情主视图的「回填缺口」使用）
	engine.POST("/klines/backfill-range", h.backfillKlineRange)
	// K 线区间查询（供回测逐笔的“K线详情”弹窗）
	engine.GET("/klines/range", h.listKlineRange)
}

// ─── Strategy handlers ────────────────────────────────────────────────────────

func (h *StrategyHandler) listStrategies(c *gin.Context) {
	var query tradeDTO.TradeStrategyQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListStrategies(query)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getStrategy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.tradeService.GetStrategyByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(c, "策略不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) createStrategy(c *gin.Context) {
	var dto tradeDTO.CreateTradeStrategyDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.tradeService.CreateStrategy(dto)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	commonRouter.ToJson(c, result, nil)
}

func (h *StrategyHandler) updateStrategy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	var dto tradeDTO.UpdateTradeStrategyDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	if err := h.tradeService.UpdateStrategy(id, dto); err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	commonRouter.ToJson(c, gin.H{"id": id}, nil)
}

func (h *StrategyHandler) deleteStrategy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	if err := h.tradeService.DeleteStrategy(id); err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	commonRouter.ToJson(c, gin.H{"id": id}, nil)
}

// ─── Position handlers ────────────────────────────────────────────────────────

func (h *StrategyHandler) listPositions(c *gin.Context) {
	var query tradeDTO.TradeStrategyPositionQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListPositions(query)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getPosition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.tradeService.GetPositionByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(c, "持仓不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getPositionSummary(c *gin.Context) {
	var query tradeDTO.TradeStrategyPositionSummaryQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.GetPositionSummary(query)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) manualClosePosition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	if err := h.tradeService.ManualClosePosition(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			commonRouter.ToError(c, "持仓不存在或已平仓")
			return
		}
		commonRouter.ToJson(c, nil, err)
		return
	}
	commonRouter.ToJson(c, gin.H{"id": id}, nil)
}

// ─── Market handler ───────────────────────────────────────────────────────────

func (h *StrategyHandler) getMarketPrice(c *gin.Context) {
	symbol := c.Query("symbol")
	if symbol == "" {
		commonRouter.ToError(c, "symbol 不能为空")
		return
	}
	price, err := h.tradeService.GetMarketPrice(symbol)
	commonRouter.ToJson(c, gin.H{"symbol": symbol, "price": price}, err)
}

// ─── Backtest handlers ────────────────────────────────────────────────────────

func (h *StrategyHandler) createBacktestRun(c *gin.Context) {
	var dto tradeDTO.CreateBacktestRunDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	runID, err := h.tradeService.CreateBacktestRun(dto)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	// 异步执行：立即返回 runId，前端轮询 run.status 看进度。
	commonRouter.ToJson(c, gin.H{"runId": runID}, nil)
}

// createSignalBacktestRun 用 strategy_event 的真实触发流跑一次参数回测。
// 与预测驱动回测走独立执行路径，落同一组表（engine_kind/calc_mode=signal）。
func (h *StrategyHandler) createSignalBacktestRun(c *gin.Context) {
	var dto tradeDTO.CreateSignalBacktestRunDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	runID, err := h.tradeService.CreateSignalBacktestRun(dto)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	// 异步回放：立即返回 runId，前端轮询 run.status 看进度。
	commonRouter.ToJson(c, gin.H{"runId": runID}, nil)
}

// listSignalSources 枚举已入库的信号源（实例 × 账户 × 合约 + 覆盖区间），
// 让表单只能选真实存在的实例与账户，而不是让人手打 instanceKey。
// 同时返回频率级回测的候选阈值（与生产采样器同源）。
func (h *StrategyHandler) listSignalSources(c *gin.Context) {
	sources, err := h.tradeService.ListSignalSources()
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	commonRouter.ToJson(c, gin.H{
		"sources":             sources,
		"thresholdCandidates": tradeService.SignalThresholdCandidatesBp(),
	}, nil)
}

// createSignalBacktestBatch 一次提交多组参数并发回放，立即返回 batchId。
// 组参数是相对基线的增量，基线缺省取所选实例当前生产参数。
func (h *StrategyHandler) createSignalBacktestBatch(c *gin.Context) {
	var dto tradeDTO.CreateSignalBacktestBatchDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	batchID, err := h.tradeService.CreateSignalBacktestBatch(c.Request.Context(), dto)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	// 异步并发回放：立即返回 batchId，前端轮询 batch.status 与 doneCount 看进度。
	commonRouter.ToJson(c, gin.H{"batchId": batchID}, nil)
}

func (h *StrategyHandler) listSignalBacktestBatches(c *gin.Context) {
	var query tradeDTO.SignalBacktestBatchQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListSignalBacktestBatches(query)
	commonRouter.ToJson(c, result, err)
}

// getSignalBacktestBatch 批次详情：基线 + 按精度等级分组的对比矩阵。
// 接口层不返回跨精度的全局榜，事件级与频率级各自成组（需求大纲 §3.3）。
func (h *StrategyHandler) getSignalBacktestBatch(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.tradeService.GetSignalBacktestBatchDetail(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(c, "批量扫描不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

// getSignalBaseline 预览所选实例×账户的生产参数基线。
func (h *StrategyHandler) getSignalBaseline(c *gin.Context) {
	var query tradeDTO.SignalBaselineQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: instanceKey 与 accountLabel 必填")
		return
	}
	result, err := h.tradeService.GetSignalBaselinePreview(c.Request.Context(), query)
	commonRouter.ToJson(c, result, err)
}

// createSignalOptimizeStudy 发起一次后台自动参数寻优。
//
// 三关阈值在这一刻锁定并落库，跑完不接受修改——跑完再定标准等于用同一份数据
// 既定标准又选参数。要换阈值只能建新任务。
func (h *StrategyHandler) createSignalOptimizeStudy(c *gin.Context) {
	var dto tradeDTO.CreateSignalOptimizeStudyDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	studyID, err := h.tradeService.CreateSignalOptimizeStudy(c.Request.Context(), dto)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	// 异步执行：立即返回 studyId，前端轮询 study.status / doneCellCount 看进度。
	commonRouter.ToJson(c, gin.H{"studyId": studyID}, nil)
}

func (h *StrategyHandler) listSignalOptimizeStudies(c *gin.Context) {
	var query tradeDTO.SignalOptimizeStudyQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListSignalOptimizeStudies(query)
	commonRouter.ToJson(c, result, err)
}

// getSignalOptimizeStudy 寻优任务详情：冻结的搜索空间/降噪协议/三关阈值 +
// 粗网格与精算结果（分两段返回，禁止混排）+ 结论与权衡前沿。
// withPaths=1 时附带逐路径产出（16 条/格，数据量大，缺省不返回）。
func (h *StrategyHandler) getSignalOptimizeStudy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	withPaths := c.Query("withPaths") == "1" || strings.EqualFold(c.Query("withPaths"), "true")
	result, err := h.tradeService.GetSignalOptimizeStudyDetail(id, withPaths)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(c, "寻优任务不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

// getSignalOptimizeDefaults 发起表单的缺省预览：搜索空间展开成多少格、降噪协议
// 是哪几条路径、三关阈值多少、预估要跑多少次回放。
func (h *StrategyHandler) getSignalOptimizeDefaults(c *gin.Context) {
	commonRouter.ToJson(c, h.tradeService.GetSignalOptimizeDefaults(), nil)
}

func (h *StrategyHandler) listBacktestRuns(c *gin.Context) {
	var query tradeDTO.BacktestRunQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListBacktestRuns(query)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getBacktestRun(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.tradeService.GetBacktestRunDetail(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(c, "回测任务不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getBacktestMetrics(c *gin.Context) {
	ids := parseIDList(c.Query("runIds"))
	if len(ids) == 0 {
		commonRouter.ToError(c, "runIds 不能为空")
		return
	}
	result, err := h.tradeService.GetBacktestMetrics(ids)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) getPredictionDetail(c *gin.Context) {
	var q tradeDTO.PredictionDetailQueryDTO
	if err := c.ShouldBindQuery(&q); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.GetPredictionDetail(q)
	commonRouter.ToJson(c, result, err)
}

// ─── Kline backfill handler ────────────────────────────────────────────────────

// backfillKlines 支持单组合与批量：请求体可用 platformCodes/symbols/intervals 传多值，
// 也兼容老的 platformCode/symbol/interval 单值写法。
func (h *StrategyHandler) backfillKlines(c *gin.Context) {
	var dto tradeDTO.BatchBackfillKlineDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.tradeService.BackfillKlinesBatch(c.Request.Context(), dto)
	commonRouter.ToJson(c, result, err)
}

// backfillKlineRange 按时间窗口覆盖率回填：窗口整段落在过去时，「最近 N 根增量」
// 会误判成已是最新而漏补，行情主视图的缺口只能靠这条路补。
func (h *StrategyHandler) backfillKlineRange(c *gin.Context) {
	var dto tradeDTO.BackfillKlineRangeDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.tradeService.BackfillKlineRange(c.Request.Context(), dto)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) listKlineRange(c *gin.Context) {
	var q tradeDTO.KlineRangeQueryDTO
	if err := c.ShouldBindQuery(&q); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListKlinesInRange(q.PlatformCode, q.Symbol, q.Interval, q.Start, q.End)
	commonRouter.ToJson(c, result, err)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// parseIDList 解析逗号分隔的 id 列表(如 "1,2,3")，忽略非法项。
func parseIDList(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}
