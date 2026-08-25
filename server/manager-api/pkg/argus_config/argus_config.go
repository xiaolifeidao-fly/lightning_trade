package argus_config

import (
	commonRouter "common/middleware/routers"
	argusService "service/argus_config"
	argusDTO "service/argus_config/dto"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ArgusConfigHandler struct {
	*commonRouter.BaseHandler
	service *argusService.ArgusConfigService
}

func NewArgusConfigHandler() *ArgusConfigHandler {
	service := argusService.NewArgusConfigService()
	_ = service.EnsureTable()
	return &ArgusConfigHandler{BaseHandler: &commonRouter.BaseHandler{}, service: service}
}

func (h *ArgusConfigHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/argus-config/published", h.getPublished)
	engine.POST("/argus-config/drafts", h.saveDraft)
	engine.POST("/argus-config/versions/:id/publish", h.publish)
}

func (h *ArgusConfigHandler) getPublished(c *gin.Context) {
	result, err := h.service.GetPublished(c.Request.Context())
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) saveDraft(c *gin.Context) {
	var request argusDTO.SaveConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.SaveDraft(&request, actor(c))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) publish(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || versionID == 0 {
		commonRouter.ToError(c, "版本参数错误")
		return
	}
	var request argusDTO.PublishConfigRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			commonRouter.ToError(c, "参数错误")
			return
		}
	}
	result, err := h.service.Publish(c.Request.Context(), versionID, &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

func actor(c *gin.Context) string {
	for _, key := range []string{"X-User-Name", "X-User", "userName"} {
		if value := c.GetHeader(key); value != "" {
			return value
		}
	}
	return "manager-api"
}
