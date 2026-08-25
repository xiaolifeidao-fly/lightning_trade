package health

import (
	"common/middleware/routers"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	*routers.BaseHandler
}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/health", h.health)
}

func (h *HealthHandler) health(context *gin.Context) {
	routers.ToJson(context, gin.H{
		"status":  "ok",
		"message": "service is healthy",
	}, nil)
}

