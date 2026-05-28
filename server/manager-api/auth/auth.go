package auth

import (
	commonRouter "common/middleware/routers"
	"common/middleware/vipper"
	authService "service/manager_auth"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserKey      = "auth.user"
	ContextUserIDKey    = "auth.userId"
	ContextRoleIDsKey   = "auth.roleIds"
	ContextTokenKey     = "auth.token"
)

var publicRouteRegistry sync.Map

func Middleware() gin.HandlerFunc {
	service := authService.NewAuthService()
	return func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		fullPath := c.FullPath()
		if isPublicRoute(c.Request.Method, fullPath) || isPublicRoute(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		token := extractToken(c)
		user, err := service.ValidateToken(token, resolveRequestURL(c))
		if err != nil {
			commonRouter.ToError(c, err.Error())
			c.Abort()
			return
		}

		c.Set(ContextUserKey, user)
		c.Set(ContextUserIDKey, user.ID)
		c.Set(ContextRoleIDsKey, user.RoleIDs)
		c.Set(ContextTokenKey, token)
		c.Next()
	}
}

func RegisterPublicRoute(method, path string) {
	key := buildRouteKey(method, path)
	if key == "" {
		return
	}
	publicRouteRegistry.Store(key, struct{}{})
}

func PublicGET(group *gin.RouterGroup, relativePath string, handlers ...gin.HandlerFunc) {
	RegisterPublicRoute("GET", joinRoute(group.BasePath(), relativePath))
	group.GET(relativePath, handlers...)
}

func PublicPOST(group *gin.RouterGroup, relativePath string, handlers ...gin.HandlerFunc) {
	RegisterPublicRoute("POST", joinRoute(group.BasePath(), relativePath))
	group.POST(relativePath, handlers...)
}

func PublicPUT(group *gin.RouterGroup, relativePath string, handlers ...gin.HandlerFunc) {
	RegisterPublicRoute("PUT", joinRoute(group.BasePath(), relativePath))
	group.PUT(relativePath, handlers...)
}

func PublicDELETE(group *gin.RouterGroup, relativePath string, handlers ...gin.HandlerFunc) {
	RegisterPublicRoute("DELETE", joinRoute(group.BasePath(), relativePath))
	group.DELETE(relativePath, handlers...)
}

func isPublicRoute(method, path string) bool {
	_, ok := publicRouteRegistry.Load(buildRouteKey(method, path))
	return ok
}

func buildRouteKey(method, path string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizePath(path)
	if method == "" || path == "" {
		return ""
	}
	return method + " " + path
}

func resolveRequestURL(c *gin.Context) string {
	if fullPath := normalizePath(c.FullPath()); fullPath != "" {
		return trimRequestPathPrefix(fullPath)
	}
	return trimRequestPathPrefix(c.Request.URL.Path)
}

func extractToken(c *gin.Context) string {
	if token := strings.TrimSpace(c.GetHeader("token")); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.GetHeader("X-Token")); token != "" {
		return token
	}
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization == "" {
		return strings.TrimSpace(c.Query("token"))
	}
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	return authorization
}

func joinRoute(basePath, relativePath string) string {
	basePath = normalizePath(basePath)
	relativePath = normalizePath(relativePath)
	switch {
	case basePath == "" && relativePath == "":
		return ""
	case basePath == "":
		return relativePath
	case relativePath == "":
		return basePath
	case basePath == "/":
		return relativePath
	case relativePath == "/":
		return basePath
	default:
		return normalizePath(basePath + "/" + strings.TrimPrefix(relativePath, "/"))
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func trimRequestPathPrefix(path string) string {
	path = normalizePath(path)
	requestPath := normalizePath(vipper.GetString("request.path"))
	if path == "" || requestPath == "" || requestPath == "/" {
		return path
	}
	if path == requestPath {
		return "/"
	}
	if strings.HasPrefix(path, requestPath+"/") {
		return normalizePath(strings.TrimPrefix(path, requestPath))
	}
	return path
}
