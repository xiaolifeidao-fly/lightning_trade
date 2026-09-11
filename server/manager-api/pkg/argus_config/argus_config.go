package argus_config

import (
	commonRouter "common/middleware/routers"
	argusService "service/argus_config"
	argusDTO "service/argus_config/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ArgusConfigHandler struct {
	*commonRouter.BaseHandler
	service *argusService.ArgusConfigService
}

func NewArgusConfigHandler() *ArgusConfigHandler {
	service := argusService.NewArgusConfigService()
	// EnsureTable 不只是 AutoMigrate：它还会把 published/version 唯一索引重建成
	// 实例内唯一，并把存量版本行归属到默认实例。这两步失败后果不对称——建表失败
	// 只是接口报错，迁移失败则是「按实例查不到已发布版本」，三个实例全都加载不到
	// 配置，而原来的 `_ =` 会把原因整个吞掉。这里不 panic（会连带拖垮 user/
	// permission 等无关模块），改为 error 级留痕，让根因在启动日志里可检索。
	if err := service.EnsureTable(); err != nil {
		logrus.Errorf("Argus 配置表初始化/迁移失败，配置面可能不可用（按实例查不到已发布版本）: %v", err)
	}
	return &ArgusConfigHandler{BaseHandler: &commonRouter.BaseHandler{}, service: service}
}

func (h *ArgusConfigHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/argus-config/instances", h.listInstances)
	// instance-overview 是总览页与实例对比页的一次性拉取：注册信息 + 已发布版本 +
	// 心跳生效状态。只读，不带任何子表与凭证。
	engine.GET("/argus-config/instance-overview", h.instanceOverview)
	engine.POST("/argus-config/instances", h.registerInstance)
	engine.GET("/argus-config/published", h.getPublished)
	engine.GET("/argus-config/versions", h.listVersions)
	engine.POST("/argus-config/drafts", h.saveDraft)
	engine.POST("/argus-config/versions/:id/publish", h.publish)
	engine.POST("/argus-config/versions/:id/rollback", h.rollback)

	// 会话凭证轮换。**写入式**：服务端从不回显明文，所以没有对应的 GET。
	// 与 argus-session-rotate CLI 共用服务层同一条路径。
	engine.POST("/argus-config/sessions/rotate", h.rotateSession)
}

func (h *ArgusConfigHandler) listVersions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	result, err := h.service.ListVersions(instanceKey(c), limit)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) listInstances(c *gin.Context) {
	onlyEnabled := c.Query("onlyEnabled") == "true"
	result, err := h.service.ListInstances(onlyEnabled)
	commonRouter.ToJson(c, result, err)
}

// instanceOverview 默认只回启用中的实例；实例对比页要看停用的实例时传 onlyEnabled=false。
func (h *ArgusConfigHandler) instanceOverview(c *gin.Context) {
	onlyEnabled := c.Query("onlyEnabled") != "false"
	result, err := h.service.InstanceOverview(c.Request.Context(), onlyEnabled)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) registerInstance(c *gin.Context) {
	var request argusDTO.SaveInstanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.RegisterInstance(&request)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) getPublished(c *gin.Context) {
	result, err := h.service.GetPublished(c.Request.Context(), instanceKey(c))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) saveDraft(c *gin.Context) {
	var request argusDTO.SaveConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.SaveDraft(instanceKey(c), &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusConfigHandler) publish(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || versionID == 0 {
		commonRouter.ToError(c, "版本参数错误")
		return
	}
	var request argusDTO.PublishConfigRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			commonRouter.ToError(c, "参数错误")
			return
		}
	}
	result, err := h.service.Publish(c.Request.Context(), instanceKey(c), versionID, &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

// rollback 把某个已归档版本重新推上 published 槽位，不新建版本号。
func (h *ArgusConfigHandler) rollback(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || versionID == 0 {
		commonRouter.ToError(c, "版本参数错误")
		return
	}
	var request argusDTO.RollbackConfigRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			commonRouter.ToError(c, "参数错误")
			return
		}
	}
	result, err := h.service.Rollback(c.Request.Context(), instanceKey(c), versionID, &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

// instanceKey 从 query 或请求头读取实例键；为空时交给 Service 解析默认实例，
// 请求体里的 instanceKey 由 Service 兜底回落。
// rotateSession 更新单个账户的 cookie / token。
//
// 管理端唯一能改凭证的入口。参数编辑那套走的是配置版本（草稿→发布），凭证不能
// 混进去：它不是参数，重发一版只为换 cookie 会让版本历史失去意义。
func (h *ArgusConfigHandler) rotateSession(c *gin.Context) {
	var request argusDTO.RotateSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		commonRouter.ToError(c, "参数错误")
		return
	}
	result, err := h.service.RotateAccountSession(c.Request.Context(), instanceKey(c), &request, actor(c))
	commonRouter.ToJson(c, result, err)
}

func instanceKey(c *gin.Context) string {
	if value := c.Query("instanceKey"); value != "" {
		return value
	}
	return c.GetHeader("X-Argus-Instance")
}

func actor(c *gin.Context) string {
	for _, key := range []string{"X-User-Name", "X-User", "userName"} {
		if value := c.GetHeader(key); value != "" {
			return value
		}
	}
	return "manager-api"
}
