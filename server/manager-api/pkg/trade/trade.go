package trade

import (
	commonRouter "common/middleware/routers"
	"net/http"
	tradeService "service/trade"
	tradeDTO "service/trade/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TradeHandler struct {
	*commonRouter.BaseHandler
	tradeService *tradeService.TradeService
}

func NewTradeHandler() *TradeHandler {
	service := tradeService.NewTradeService()
	_ = service.EnsureTable()
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
