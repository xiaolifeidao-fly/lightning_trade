package coin_platform

import (
	commonRouter "common/middleware/routers"
	"net/http"
	coinPlatformService "service/coin_platform"
	coinPlatformDTO "service/coin_platform/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CoinPlatformHandler struct {
	*commonRouter.BaseHandler
	coinPlatformService *coinPlatformService.CoinPlatformService
}

func NewCoinPlatformHandler() *CoinPlatformHandler {
	service := coinPlatformService.NewCoinPlatformService()
	_ = service.EnsureTable()
	return &CoinPlatformHandler{
		BaseHandler:         &commonRouter.BaseHandler{},
		coinPlatformService: service,
	}
}

func (h *CoinPlatformHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/coin-platforms", h.listPlatforms)
	engine.GET("/coin-platforms/:id", h.getPlatformByID)
	engine.POST("/coin-platforms", h.createPlatform)
	engine.PUT("/coin-platforms/:id", h.updatePlatform)
	engine.DELETE("/coin-platforms/:id", h.deletePlatform)

	engine.GET("/coin-platform-coins", h.listPlatformCoins)
	engine.POST("/coin-platform-coins", h.upsertPlatformCoin)
	engine.PUT("/coin-platform-coins/:id", h.updatePlatformCoin)
	engine.DELETE("/coin-platform-coins/:id", h.deletePlatformCoin)

	engine.GET("/coin-platform-accounts", h.listPlatformAccounts)
	engine.POST("/coin-platform-accounts", h.createPlatformAccount)
	engine.PUT("/coin-platform-accounts/:id", h.updatePlatformAccount)
	engine.DELETE("/coin-platform-accounts/:id", h.deletePlatformAccount)
}

func (h *CoinPlatformHandler) listPlatforms(context *gin.Context) {
	var query coinPlatformDTO.CoinPlatformQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.ListPlatforms(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) getPlatformByID(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	result, err := h.coinPlatformService.GetPlatformByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) createPlatform(context *gin.Context) {
	var req coinPlatformDTO.CreateCoinPlatformDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.CreatePlatform(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) updatePlatform(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	var req coinPlatformDTO.UpdateCoinPlatformDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.UpdatePlatform(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) deletePlatform(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	err := h.coinPlatformService.DeletePlatform(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *CoinPlatformHandler) listPlatformCoins(context *gin.Context) {
	var query coinPlatformDTO.CoinPlatformCoinQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.ListPlatformCoins(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) upsertPlatformCoin(context *gin.Context) {
	var req coinPlatformDTO.CreateCoinPlatformCoinDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.UpsertPlatformCoin(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) updatePlatformCoin(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	var req coinPlatformDTO.UpdateCoinPlatformCoinDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.UpdatePlatformCoin(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform coin not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) deletePlatformCoin(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	err := h.coinPlatformService.DeletePlatformCoin(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform coin not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *CoinPlatformHandler) listPlatformAccounts(context *gin.Context) {
	var query coinPlatformDTO.CoinPlatformAccountQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.ListPlatformAccounts(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) createPlatformAccount(context *gin.Context) {
	var req coinPlatformDTO.CreateCoinPlatformAccountDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.CreatePlatformAccount(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) updatePlatformAccount(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	var req coinPlatformDTO.UpdateCoinPlatformAccountDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinPlatformService.UpdatePlatformAccount(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform account not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinPlatformHandler) deletePlatformAccount(context *gin.Context) {
	id, ok := parsePlatformID(context)
	if !ok {
		return
	}
	err := h.coinPlatformService.DeletePlatformAccount(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "platform account not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func parsePlatformID(context *gin.Context) (uint, bool) {
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
