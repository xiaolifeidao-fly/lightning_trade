package argus_runtime

import (
	commonRouter "common/middleware/routers"
	argusService "service/argus_config"
	runtimeService "service/argus_runtime"

	"github.com/gin-gonic/gin"
)

// ArgusRuntimeHandler 只暴露「查状态」与「立即热加载」两个动作。start / stop /
// restart 已随本次参数页重构下线：页面上不再存在任何能停掉实盘进程的入口。
type ArgusRuntimeHandler struct {
	*commonRouter.BaseHandler
	service       *runtimeService.ArgusRuntimeService
	configService *argusService.ArgusConfigService
}

func NewArgusRuntimeHandler() *ArgusRuntimeHandler {
	return &ArgusRuntimeHandler{
		BaseHandler:   &commonRouter.BaseHandler{},
		service:       runtimeService.NewArgusRuntimeService(),
		configService: argusService.NewArgusConfigService(),
	}
}

func (h *ArgusRuntimeHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/argus/runtime/status", h.status)
	engine.POST("/argus/runtime/reload", h.reload)
}

func (h *ArgusRuntimeHandler) status(c *gin.Context) {
	instanceKey, err := h.resolveInstanceKey(c)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	result, err := h.service.Status(c.Request.Context(), instanceKey)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusRuntimeHandler) reload(c *gin.Context) {
	instanceKey, err := h.resolveInstanceKey(c)
	if err != nil {
		commonRouter.ToJson(c, nil, err)
		return
	}
	result, err := h.service.Reload(c.Request.Context(), instanceKey)
	commonRouter.ToJson(c, result, err)
}

// resolveInstanceKey 与配置接口共用同一套实例解析规则，避免「配置发给 A、心跳
// 读的是 B」这类跨实例错配；缺实例键时由 Service 报错，不做跨实例兜底。
func (h *ArgusRuntimeHandler) resolveInstanceKey(c *gin.Context) (string, error) {
	value := c.Query("instanceKey")
	if value == "" {
		value = c.GetHeader("X-Argus-Instance")
	}
	return h.configService.ResolveInstanceKey(value)
}
