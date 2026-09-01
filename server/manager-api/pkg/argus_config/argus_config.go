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
	engine.GET("/argus-config/instances", h.listInstances)
	engine.POST("/argus-config/instances", h.registerInstance)
	engine.GET("/argus-config/published", h.getPublished)
	engine.POST("/argus-config/drafts", h.saveDraft)
	engine.POST("/argus-config/versions/:id/publish", h.publish)
}

func (h *ArgusConfigHandler) listInstances(c *gin.Context) {
	onlyEnabled := c.Query("onlyEnabled") == "true"
	result, err := h.service.ListInstances(onlyEnabled)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) registerInstance(c *gin.Context) {
	var request argusDTO.SaveInstanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.RegisterInstance(&request)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) getPublished(c *gin.Context) {
	result, err := h.service.GetPublished(c.Request.Context(), instanceKey(c))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) saveDraft(c *gin.Context) {
	var request argusDTO.SaveConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.SaveDraft(instanceKey(c), &request, actor(c))
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
	result, err := h.service.Publish(c.Request.Context(), instanceKey(c), versionID, &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

// instanceKey 从 query 或请求头读取实例键；为空时交给 Service 解析默认实例，
// 请求体里的 instanceKey 由 Service 兜底回落。
func instanceKey(c *gin.Context) string {
	if value := c.Query("instanceKey"); value != "" {
		return value
	}
	return c.GetHeader("X-Argus-Instance")
}

func actor(c *gin.Context) string {
	for _, key := range []string{"X-User-Name", "X-User", "userName"} {
		if value := c.GetHeader(key); value != "" {
			return value
		}
	}
	return "manager-api"
}
