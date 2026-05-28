package coin_user

import (
	commonRouter "common/middleware/routers"
	"net/http"
	coinUserService "service/coin_user"
	coinUserDTO "service/coin_user/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CoinUserHandler struct {
	*commonRouter.BaseHandler
	coinUserService *coinUserService.CoinUserService
}

func NewCoinUserHandler() *CoinUserHandler {
	service := coinUserService.NewCoinUserService()
	_ = service.EnsureTable()
	return &CoinUserHandler{
		BaseHandler:     &commonRouter.BaseHandler{},
		coinUserService: service,
	}
}

func (h *CoinUserHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/coin-users", h.listCoinUsers)
	engine.GET("/coin-users/:id", h.getCoinUserByID)
	engine.POST("/coin-users", h.createCoinUser)
	engine.PUT("/coin-users/:id", h.updateCoinUser)
	engine.DELETE("/coin-users/:id", h.deleteCoinUser)

	engine.GET("/coin-users/:id/assets", h.listUserAssets)
	engine.POST("/coin-users/assets", h.upsertUserAsset)
	engine.PUT("/coin-users/assets/:id", h.updateUserAsset)

	engine.GET("/coin-users/:id/login-records", h.listLoginRecords)
	engine.POST("/coin-users/login-records", h.recordLogin)

	engine.GET("/coin-users/:id/positions", h.listUserPositions)
	engine.POST("/coin-users/positions", h.upsertUserPosition)
	engine.PUT("/coin-users/positions/:id", h.updateUserPosition)

	engine.GET("/coin-users/:id/position-analysis", h.listPositionAnalysis)
	engine.POST("/coin-users/position-analysis", h.createPositionAnalysis)
	engine.PUT("/coin-users/position-analysis/:id", h.updatePositionAnalysis)
}

func (h *CoinUserHandler) listCoinUsers(context *gin.Context) {
	var query coinUserDTO.CoinUserQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.ListCoinUsers(query)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) getCoinUserByID(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	result, err := h.coinUserService.GetCoinUserByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin user not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) createCoinUser(context *gin.Context) {
	var req coinUserDTO.CreateCoinUserDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.CreateCoinUser(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) updateCoinUser(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	var req coinUserDTO.UpdateCoinUserDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpdateCoinUser(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin user not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) deleteCoinUser(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	err := h.coinUserService.DeleteCoinUser(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "coin user not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *CoinUserHandler) listUserAssets(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	result, err := h.coinUserService.ListUserAssets(uint64(id))
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) upsertUserAsset(context *gin.Context) {
	var req coinUserDTO.CreateCoinUserAssetDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpsertUserAsset(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) updateUserAsset(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	var req coinUserDTO.UpdateCoinUserAssetDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpdateUserAsset(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "asset not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) listLoginRecords(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	limitStr := context.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)
	result, err := h.coinUserService.ListLoginRecords(uint64(id), limit)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) recordLogin(context *gin.Context) {
	var req coinUserDTO.CreateCoinUserLoginRecordDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.RecordLogin(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) listUserPositions(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	result, err := h.coinUserService.ListUserPositions(uint64(id))
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) upsertUserPosition(context *gin.Context) {
	var req coinUserDTO.CreateCoinUserPositionDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpsertUserPosition(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) updateUserPosition(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	var req coinUserDTO.UpdateCoinUserPositionDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpdateUserPosition(id, &req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) listPositionAnalysis(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	result, err := h.coinUserService.ListPositionAnalysisByUser(uint64(id))
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) createPositionAnalysis(context *gin.Context) {
	var req coinUserDTO.CreateCoinUserPositionAnalysisDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.CreatePositionAnalysis(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *CoinUserHandler) updatePositionAnalysis(context *gin.Context) {
	id, ok := parseCoinUserID(context)
	if !ok {
		return
	}
	var req coinUserDTO.UpdateCoinUserPositionAnalysisDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.coinUserService.UpdatePositionAnalysis(id, &req)
	commonRouter.ToJson(context, result, err)
}

func parseCoinUserID(context *gin.Context) (uint, bool) {
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
