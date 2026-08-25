package login

import (
	"context"
	"net/http"

	"argus_single/pkg/trade"
	"common/middleware/routers"

	"github.com/gin-gonic/gin"
)

type LoginHandler struct {
	*routers.BaseHandler
}

func NewLoginHandler() *LoginHandler {
	return &LoginHandler{}
}

func (h *LoginHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.POST("/login/deepcoin", h.login)
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
}

func (h *LoginHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		routers.ToJson(c, nil, err)
		return
	}

	// 从 session.json 中读取账户配置
	store := trade.NewSessionStore("./configs/session.json")
	if err := store.Load(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "读取 session.json 失败: " + err.Error()})
		return
	}

	entry, ok := store.GetByUsername(req.Username)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "session.json 中未找到账户: " + req.Username})
		return
	}

	acc := trade.AccountConfig{
		Name:          entry.AccountName,
		Username:      entry.Username,
		Password:      entry.Password,
		GoogleAuthKey: entry.GoogleAuthKey,
		LoginURL:      entry.LoginURL,
		LoginHeadless: entry.LoginHeadless != nil && *entry.LoginHeadless,
	}

	svc := trade.NewDeepCoinLoginService()
	result, err := svc.Login(context.Background(), acc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	routers.ToJson(c, gin.H{
		"resourceId":      result.ResourceID,
		"loginURL":        result.LoginURL,
		"finalURL":        result.FinalURL,
		"cookie":          result.Cookie,
		"token":           result.Token,
		"oToken":          result.OToken,
		"sentryRelease":   result.SentryRelease,
		"sentryPublicKey": result.SentryPublicKey,
		"baggage":         result.Baggage,
	}, nil)
}

