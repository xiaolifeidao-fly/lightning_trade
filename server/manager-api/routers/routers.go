package routers

import (
	"common/middleware/routers"

	"github.com/gin-gonic/gin"
)

var router *routers.GinRouter

func Init() {
	router = routers.NewGinRouter()
	registerHandlers := registerHandler()
	InitAllRouters(router, registerHandlers)
}

func Run(middleware ...gin.HandlerFunc) error {
	router.Use(middleware...)
	return router.Run()
}

// InitAllRouters 初始化所有router

func InitAllRouters(router *routers.GinRouter, handlers []routers.Handler) {
	for _, handler := range handlers {
		router.Include(handler.RegisterHandler)
	}
}
