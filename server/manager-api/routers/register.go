package routers

import (
	"common/middleware/routers"
	"log"
	"manager-api/pkg/login"
	"manager-api/pkg/permission"
	"manager-api/pkg/user"

	"time"
)

func registerHandler() []routers.Handler {
	build := func(name string, fn func() routers.Handler) routers.Handler {
		start := time.Now()
		handler := fn()
		log.Printf("Handler %s initialized in %s", name, time.Since(start))
		return handler
	}

	return []routers.Handler{
		build("login", func() routers.Handler { return login.NewLoginHandler() }),
		build("permission", func() routers.Handler { return permission.NewPermissionHandler() }),
		build("user", func() routers.Handler { return user.NewUserHandler() }),
	}
}
