package coin

import (
	commonRouter "common/middleware/routers"
	"net/http"
	coinService "service/coin"
	coinDTO "service/coin/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CoinHandler struct {
	*commonRouter.BaseHandler
	coinService *coinService.CoinService
}

func NewCoinHandler() *CoinHandler {
	service := coinService.NewCoinService()
	_ = service.EnsureTable()
	return &CoinHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		coinService: service,
	}
}

func (h *CoinHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/coins", h.listCoins)
	engine.GET("/coins/:id", h.getCoinByID)
	engine.POST("/coins", h.createCoin)
	engine.PUT("/coins/:id", h.updateCoin)
	engine.DELETE("/coins/:id", h.deleteCoin)

	engine.GET("/coin-pairs", h.listCoinPairs)
	engine.POST("/coin-pairs", h.createCoinPair)
	engine.PUT("/coin-pairs/:id", h.updateCoinPair)
	engine.DELETE("/coin-pairs/:id", h.deleteCoinPair)

	engine.GET("/coin-prices/latest", h.getLatestPrice)
	engine.POST("/coin-prices", h.upsertCoinPrice)
}

func (h *CoinHandler) listCoins(context *gin.Context) {
	var query coinDTO.CoinQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.ListCoins(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) getCoinByID(context *gin.Context) {
	id, ok := parseCoinID(context)
	if !ok {
		return
	}
	result, err := h.coinService.GetCoinByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) createCoin(context *gin.Context) {
	var req coinDTO.CreateCoinDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.CreateCoin(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) updateCoin(context *gin.Context) {
	id, ok := parseCoinID(context)
	if !ok {
		return
	}
	var req coinDTO.UpdateCoinDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.UpdateCoin(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) deleteCoin(context *gin.Context) {
	id, ok := parseCoinID(context)
	if !ok {
		return
	}
	err := h.coinService.DeleteCoin(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *CoinHandler) listCoinPairs(context *gin.Context) {
	var query coinDTO.CoinPairQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.ListCoinPairs(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) createCoinPair(context *gin.Context) {
	var req coinDTO.CreateCoinPairDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.CreateCoinPair(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) updateCoinPair(context *gin.Context) {
	id, ok := parseCoinID(context)
	if !ok {
		return
	}
	var req coinDTO.UpdateCoinPairDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.UpdateCoinPair(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin pair not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) deleteCoinPair(context *gin.Context) {
	id, ok := parseCoinID(context)
	if !ok {
		return
	}
	err := h.coinService.DeleteCoinPair(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin pair not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *CoinHandler) getLatestPrice(context *gin.Context) {
	coinCode := context.Query("coinCode")
	quoteCode := context.DefaultQuery("quoteCode", "USDT")
	if coinCode == "" {
		commonRouter.ToError(context, "coinCode不能为空")
		return
	}
	result, err := h.coinService.GetLatestPrice(coinCode, quoteCode)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "price not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinHandler) upsertCoinPrice(context *gin.Context) {
	var req coinDTO.CreateCoinPriceDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinService.UpsertPrice(&req)
	commonRouter.ToJson(context, result, err)
}

func parseCoinID(context *gin.Context) (uint, bool) {
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
