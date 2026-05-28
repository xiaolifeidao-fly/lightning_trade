package routers

import "github.com/gin-gonic/gin"

type Handler interface {
	RegisterHandler(engine *gin.RouterGroup)
}

type BaseHandler struct {
}

func (h *BaseHandler) RegisterHandler(engine *gin.RouterGroup) {

}
