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
	// 「K线详情」预测增强：复合方向 + 预测周期 K 线
	engine.GET("/backtest/prediction-detail", h.getPredictionDetail)

	// K 线回填（按币种/周期/最近 N 根增量拉取入库，供回测使用）
	engine.POST("/klines/backfill", h.backfillKlines)
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

func (h *StrategyHandler) backfillKlines(c *gin.Context) {
	var dto tradeDTO.BackfillKlineDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.tradeService.BackfillKlines(c.Request.Context(), dto)
	commonRouter.ToJson(c, result, err)
}

func (h *StrategyHandler) listKlineRange(c *gin.Context) {
	var q tradeDTO.KlineRangeQueryDTO
	if err := c.ShouldBindQuery(&q); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.tradeService.ListKlinesInRange(q.Symbol, q.Interval, q.Start, q.End)
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
