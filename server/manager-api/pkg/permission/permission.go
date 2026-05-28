package permission

import (
	commonRouter "common/middleware/routers"
	"net/http"
	permissionService "service/manager_permission"
	permissionDTO "service/manager_permission/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionHandler struct {
	*commonRouter.BaseHandler
	permissionService *permissionService.PermissionService
}

func NewPermissionHandler() *PermissionHandler {
	service := permissionService.NewPermissionService()
	_ = service.EnsureTable()

	return &PermissionHandler{
		BaseHandler:       &commonRouter.BaseHandler{},
		permissionService: service,
	}
}

func (h *PermissionHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/resources", h.listResources)
	engine.GET("/resources/:id", h.getResourceByID)
	engine.POST("/resources", h.createResource)
	engine.PUT("/resources/:id", h.updateResource)
	engine.DELETE("/resources/:id", h.deleteResource)

	engine.GET("/roles", h.listRoles)
	engine.GET("/roles/:id", h.getRoleByID)
	engine.POST("/roles", h.createRole)
	engine.PUT("/roles/:id", h.updateRole)
	engine.DELETE("/roles/:id", h.deleteRole)

	engine.GET("/role-resources", h.listRoleResources)
	engine.GET("/role-resources/:id", h.getRoleResourceByID)
	engine.POST("/role-resources", h.createRoleResource)
	engine.PUT("/role-resources/:id", h.updateRoleResource)
	engine.DELETE("/role-resources/:id", h.deleteRoleResource)
}

func (h *PermissionHandler) listResources(context *gin.Context) {
	var query permissionDTO.ResourceQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.ListResources(query)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) getResourceByID(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	result, err := h.permissionService.GetResourceByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "resource not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) createResource(context *gin.Context) {
	var req permissionDTO.CreateResourceDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.CreateResource(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) updateResource(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	var req permissionDTO.UpdateResourceDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.UpdateResource(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "resource not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) deleteResource(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	err := h.permissionService.DeleteResource(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "resource not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *PermissionHandler) listRoles(context *gin.Context) {
	var query permissionDTO.RoleQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.ListRoles(query)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) getRoleByID(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	result, err := h.permissionService.GetRoleByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) createRole(context *gin.Context) {
	var req permissionDTO.CreateRoleDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.CreateRole(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) updateRole(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	var req permissionDTO.UpdateRoleDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.UpdateRole(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) deleteRole(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	err := h.permissionService.DeleteRole(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *PermissionHandler) listRoleResources(context *gin.Context) {
	var query permissionDTO.RoleResourceQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.ListRoleResources(query)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) getRoleResourceByID(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	result, err := h.permissionService.GetRoleResourceByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role resource not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) createRoleResource(context *gin.Context) {
	var req permissionDTO.CreateRoleResourceDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.CreateRoleResource(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) updateRoleResource(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	var req permissionDTO.UpdateRoleResourceDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.permissionService.UpdateRoleResource(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role resource not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *PermissionHandler) deleteRoleResource(context *gin.Context) {
	id, ok := parsePermissionID(context)
	if !ok {
		return
	}
	err := h.permissionService.DeleteRoleResource(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "role resource not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func parsePermissionID(context *gin.Context) (uint, bool) {
	idValue := context.Param("id")
	id, err := strconv.ParseUint(idValue, 10, 32)
	if err != nil || id == 0 {
		context.JSON(http.StatusOK, gin.H{
			"code":  commonRouter.FailCode,
			"data":  "参数错误",
			"error": "id必须是正整数",
		})
		return 0, false
	}
	return uint(id), true
}
