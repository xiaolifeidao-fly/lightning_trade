package argus_runtime

import (
	"fmt"
	"path/filepath"

	commonRouter "common/middleware/routers"
	"common/middleware/vipper"
	runtimeService "service/argus_runtime"

	"github.com/gin-gonic/gin"
)

type ArgusRuntimeHandler struct {
	*commonRouter.BaseHandler
	service    *runtimeService.ArgusRuntimeService
	instanceID string
}

func NewArgusRuntimeHandler() *ArgusRuntimeHandler {
	controlScript := vipper.GetString("argus.control.script")
	if controlScript == "" {
		controlScript = "../argus_single/script/control.sh"
	}
	service, err := runtimeService.NewArgusRuntimeService(filepath.Clean(controlScript))
	if err != nil {
		panic(fmt.Errorf("initialize argus runtime control: %w", err))
	}
	instanceID := vipper.GetString("argus.instance.id")
	if instanceID == "" {
		instanceID = "default"
	}
	return &ArgusRuntimeHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		service:     service,
		instanceID:  instanceID,
	}
}

func (h *ArgusRuntimeHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/argus/runtime/status", h.status)
	engine.POST("/argus/runtime/start", h.start)
	engine.POST("/argus/runtime/stop", h.stop)
	engine.POST("/argus/runtime/restart", h.restart)
	engine.POST("/argus/runtime/reload", h.reload)
}

func (h *ArgusRuntimeHandler) status(c *gin.Context) {
	result, err := h.service.Status(c.Request.Context(), h.instanceID)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusRuntimeHandler) start(c *gin.Context) {
	h.control(c, runtimeService.ActionStart)
}

func (h *ArgusRuntimeHandler) stop(c *gin.Context) {
	h.control(c, runtimeService.ActionStop)
}

func (h *ArgusRuntimeHandler) restart(c *gin.Context) {
	h.control(c, runtimeService.ActionRestart)
}

func (h *ArgusRuntimeHandler) reload(c *gin.Context) {
	result, err := h.service.Reload(c.Request.Context(), h.instanceID)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusRuntimeHandler) control(c *gin.Context, action string) {
	result, err := h.service.Control(c.Request.Context(), action)
	commonRouter.ToJson(c, result, err)
}
