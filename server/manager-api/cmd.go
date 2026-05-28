package main

import (
	webAuth "manager-api/auth"
	"manager-api/initialization"
	"manager-api/routers"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	initialization.Init()
	routers.Run(webAuth.Middleware())
}
