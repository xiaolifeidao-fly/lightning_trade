package trade

import (
	commonRouter "common/middleware/routers"
	"net/http"
	tradeService "service/trade"
	tradeDTO "service/trade/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type TradeHandler struct {
	*commonRouter.BaseHandler
	tradeService *tradeService.TradeService
}

func NewTradeHandler() *TradeHandler {
	service := tradeService.NewTradeService()
	// EnsureTable 不只是 AutoMigrate：trade_kline 那一步会回填存量行的
	// platform_code 并 DROP 旧唯一索引 idx_symbol_interval_open。两种失败后果
	// 不对称——建表失败只是接口报错，索引迁移失败则是旧唯一键存活，DeepCoin 与
	// 币安同一 (symbol, interval, open_time) 写入时报重复键，平台维度静默失效。
	// 原来的 `_ =` 会把根因整个吞掉。这里不 panic（会连带拖垮 user/permission
	// 等无关模块），改为 error 级留痕，让根因在启动日志里可检索。
	// 对齐 pkg/argus_config 的同类处理。
	if err := service.EnsureTable(); err != nil {
		logrus.Errorf("交易域表初始化/迁移失败，K 线平台维度可能未生效（旧唯一索引 idx_symbol_interval_open 或仍存活，DeepCoin K 线写入会报重复键）: %v", err)
	}
	return &TradeHandler{
		BaseHandler:  &commonRouter.BaseHandler{},
		tradeService: service,
	}
}

func (h *TradeHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/trade-orders", h.listOrders)
	engine.GET("/trade-orders/:orderNo", h.getOrderByOrderNo)
	engine.POST("/trade-orders", h.placeOrder)
	engine.POST("/trade-orders/cancel", h.cancelOrder)
	engine.PUT("/trade-orders/:orderNo/fill", h.updateOrderFill)

	engine.GET("/trade-matches", h.listRecentMatches)
	engine.POST("/trade-matches", h.recordMatch)

	engine.GET("/trade-klines", h.listKlines)
	engine.GET("/trade-simulation-analysis", h.getSimulationAnalysis)
	engine.GET("/trade-strategy-backtest", h.getStrategyBacktest)

	engine.GET("/trade-details", h.listTradeDetails)
	engine.GET("/trade-details/by-order/:orderNo", h.listDetailsByOrderNo)
	engine.POST("/trade-details", h.createTradeDetail)

	engine.GET("/trade-user-summary", h.listUserSummary)
	engine.GET("/trade-user-pnl", h.listUserPnl)
}

func (h *TradeHandler) listOrders(context *gin.Context) {
	var query tradeDTO.TradeOrderQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.ListOrders(query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) getOrderByOrderNo(context *gin.Context) {
	orderNo := context.Param("orderNo")
	if orderNo == "" {
		commonRouter.ToError(context, "orderNo不能为空")
		return
	}
	result, err := h.tradeService.GetOrderByOrderNo(orderNo)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "trade order not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) placeOrder(context *gin.Context) {
	var req tradeDTO.CreateTradeOrderDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.PlaceOrder(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) cancelOrder(context *gin.Context) {
	var req tradeDTO.CancelTradeOrderDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.CancelOrder(&req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "trade order not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) updateOrderFill(context *gin.Context) {
	orderNo := context.Param("orderNo")
	if orderNo == "" {
		commonRouter.ToError(context, "orderNo不能为空")
		return
	}
	var req tradeDTO.UpdateTradeOrderFillDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.UpdateOrderFill(orderNo, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "trade order not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listRecentMatches(context *gin.Context) {
	var query tradeDTO.TradeMatchQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	if query.UserID > 0 {
		result, err := h.tradeService.ListUserMatches(query.UserID, query.Symbol, query.Limit)
		commonRouter.ToJson(context, result, err)
		return
	}
	result, err := h.tradeService.ListRecentMatches(query.Symbol, query.Limit)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) recordMatch(context *gin.Context) {
	var req tradeDTO.CreateTradeMatchDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.RecordMatch(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listKlines(context *gin.Context) {
	var query tradeDTO.TradeKlineQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.ListKlines(query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) getSimulationAnalysis(context *gin.Context) {
	var query tradeDTO.TradeSimulationAnalysisQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.GetSimulationAnalysis(context.Request.Context(), query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) getStrategyBacktest(context *gin.Context) {
	var query tradeDTO.TradeStrategyBacktestQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.GetStrategyBacktest(context.Request.Context(), query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listTradeDetails(context *gin.Context) {
	var query tradeDTO.TradeDetailQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.ListTradeDetails(query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listDetailsByOrderNo(context *gin.Context) {
	orderNo := context.Param("orderNo")
	if orderNo == "" {
		commonRouter.ToError(context, "orderNo不能为空")
		return
	}
	result, err := h.tradeService.ListDetailsByOrderNo(orderNo)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) createTradeDetail(context *gin.Context) {
	var req tradeDTO.CreateTradeDetailDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.CreateTradeDetail(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listUserSummary(context *gin.Context) {
	var query tradeDTO.TradeUserSummaryQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.ListUserSummary(query)
	commonRouter.ToJson(context, result, err)
}

func (h *TradeHandler) listUserPnl(context *gin.Context) {
	var query tradeDTO.TradeUserPnlQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.tradeService.ListUserPnl(query)
	commonRouter.ToJson(context, result, err)
}

func parseTradeID(context *gin.Context) (uint, bool) {
	idValue := context.Param("id")
	id, err := strconv.ParseUint(idValue, 10, 32)
	if err != nil || id == 0 {
		context.JSON(http.StatusOK, gin.H{
			"code":  commonRouter.FailCode,
			"data":  "参数错误",
			"error": "id必须是正整数",
		})
		return 0, false
	}
	return uint(id), true
}
