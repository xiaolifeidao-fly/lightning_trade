package login

import (
	commonRouter "common/middleware/routers"
	"common/middleware/vipper"
	webAuth "manager-api/auth"
	authService "service/manager_auth"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type LoginHandler struct {
	*commonRouter.BaseHandler
	authService *authService.AuthService
}

func NewLoginHandler() *LoginHandler {
	return &LoginHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		authService: authService.NewAuthService(),
	}
}

func (h *LoginHandler) RegisterHandler(engine *gin.RouterGroup) {
	webAuth.PublicPOST(engine, "/login", h.login)
}

func (h *LoginHandler) login(context *gin.Context) {
	var req LoginRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}

	maxLoginErrorNum := vipper.GetInt64("user.max.login.error.num")
	if maxLoginErrorNum <= 0 {
		maxLoginErrorNum = 20
	}

	token, _, err := h.authService.Login(req.Username, req.Password, context.ClientIP(), maxLoginErrorNum)
	if err != nil {
		commonRouter.ToError(context, err.Error())
		return
	}
	commonRouter.ToJson(context, &LoginResponse{Token: token}, nil)
}
