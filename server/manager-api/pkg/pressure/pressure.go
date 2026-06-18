package pressure

import (
	commonRouter "common/middleware/routers"
	pressureService "service/pressure"
	pressureDTO "service/pressure/dto"

	"github.com/gin-gonic/gin"
)

type PressureHandler struct {
	*commonRouter.BaseHandler
	pressureService *pressureService.PressureService
}

func NewPressureHandler() *PressureHandler {
	service := pressureService.NewPressureService()
	_ = service.EnsureTable()
	return &PressureHandler{
		BaseHandler:     &commonRouter.BaseHandler{},
		pressureService: service,
	}
}

func (h *PressureHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/pressure-analyses", h.listPressures)
}

func (h *PressureHandler) listPressures(context *gin.Context) {
	var query pressureDTO.PressureAnalysisQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.pressureService.ListPressures(query)
	commonRouter.ToJson(context, result, err)
}
