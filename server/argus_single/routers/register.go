package routers

import (
	"argus_single/pkg/health"
	"argus_single/pkg/hello"
	"argus_single/pkg/login"
	"common/middleware/routers"
)

func registerHandler() []routers.Handler {
	handlers := []routers.Handler{
		health.NewHealthHandler(),
		hello.NewHelloHandler(),
		login.NewLoginHandler(),
	}
	return handlers
}

