```go
package user

import (
    "pigeon/internal/service/user"
    "common/middleware/routers"

    "github.com/gin-gonic/gin"
)

type UserHandler struct {
    *routers.BaseHandler
    userService *user.UserService
}

func NewUserHandler() *UserHandler {
    return &UserHandler{
        userService: user.NewUserService(),
    }
}

func (h *UserHandler) RegisterHandler(engine *gin.RouterGroup) {
    engine.GET("/user", h.list)
    engine.GET("/user/:id", h.getByID)
    engine.POST("/user", h.create)
    engine.PUT("/user/:id", h.update)
    engine.DELETE("/user/:id", h.delete)
}
```

### 路由方法

- 方法名使用 **小写驼峰**（私有方法），以 CRUD 操作命名：`list`、`getByID`、`create`、`update`、`delete`
- 参数统一命名为 `context *gin.Context`
- 响应统一使用 `routers.ToJson` 或 `routers.ToError`
- 路径参数 `id` 需解析为 `uint` 类型

```go
func (h *UserHandler) list(context *gin.Context) {
    users, err := h.userService.ListUsers()
    routers.ToJson(context, users, err)
}

func (h *UserHandler) getByID(context *gin.Context) {
    id, ok := parseID(context)
    if !ok {
        return
    }
    user, err := h.userService.GetUserByID(id)
    routers.ToJson(context, user, err)
}

func (h *UserHandler) create(context *gin.Context) {
    var req dto.CreateUserDTO
    if err := context.ShouldBindJSON(&req); err != nil {
        routers.ToError(context, "参数错误")
        return
    }
    result, err := h.userService.CreateUser(&req)
    routers.ToJson(context, result, err)
}

func parseID(context *gin.Context) (uint, bool) {
    idValue := context.Param("id")
    id, err := strconv.ParseUint(idValue, 10, 32)
    if err != nil {
        context.JSON(http.StatusOK, gin.H{
            "code":  routers.FailCode,
            "data":  "参数错误",
            "error": "id必须是正整数",
        })
        return 0, false
    }
    return uint(id), true
}
```

---

## 路由注册规范

### 1. 新增 Handler 后，在 `routers/register.go` 中注册

```go
func registerHandler() []routers.Handler {
    handlers := []routers.Handler{
        test.NewTestHandler(),
        user.NewUserHandler(),   // ← 新增
    }
    return handlers
}
```