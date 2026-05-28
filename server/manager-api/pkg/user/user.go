package user

import (
	commonRouter "common/middleware/routers"
	"net/http"
	userService "service/manager_user"
	userDTO "service/manager_user/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserHandler struct {
	*commonRouter.BaseHandler
	userService *userService.UserService
}

func NewUserHandler() *UserHandler {
	service := userService.NewUserService()
	_ = service.EnsureTable()

	return &UserHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		userService: service,
	}
}

func (h *UserHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/users", h.listUsers)
	engine.GET("/users/stats", h.getUserStats)
	engine.GET("/users/:id", h.getUserByID)
	engine.POST("/users", h.createUser)
	engine.PUT("/users/:id", h.updateUser)
	engine.DELETE("/users/:id", h.deleteUser)

	engine.GET("/user-login-records", h.listUserLoginRecords)
	engine.GET("/user-login-records/:id", h.getUserLoginRecordByID)
	engine.POST("/user-login-records", h.createUserLoginRecord)
	engine.PUT("/user-login-records/:id", h.updateUserLoginRecord)
	engine.DELETE("/user-login-records/:id", h.deleteUserLoginRecord)

	engine.GET("/user-roles", h.listUserRoles)
	engine.GET("/user-roles/:id", h.getUserRoleByID)
	engine.POST("/user-roles", h.createUserRole)
	engine.PUT("/user-roles/:id", h.updateUserRole)
	engine.DELETE("/user-roles/:id", h.deleteUserRole)
}

func (h *UserHandler) listUsers(context *gin.Context) {
	var query userDTO.UserQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.ListUsers(query)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) getUserStats(context *gin.Context) {
	result, err := h.userService.GetUserStats()
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) getUserByID(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	result, err := h.userService.GetUserByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) createUser(context *gin.Context) {
	var req userDTO.CreateUserDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.CreateUser(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) updateUser(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	var req userDTO.UpdateUserDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.UpdateUser(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) deleteUser(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	err := h.userService.DeleteUser(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *UserHandler) listUserLoginRecords(context *gin.Context) {
	var query userDTO.UserLoginRecordQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.ListUserLoginRecords(query)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) getUserLoginRecordByID(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	result, err := h.userService.GetUserLoginRecordByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user login record not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) createUserLoginRecord(context *gin.Context) {
	var req userDTO.CreateUserLoginRecordDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.CreateUserLoginRecord(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) updateUserLoginRecord(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	var req userDTO.UpdateUserLoginRecordDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.UpdateUserLoginRecord(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user login record not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) deleteUserLoginRecord(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	err := h.userService.DeleteUserLoginRecord(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user login record not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func (h *UserHandler) listUserRoles(context *gin.Context) {
	var query userDTO.UserRoleQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.ListUserRoles(query)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) getUserRoleByID(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	result, err := h.userService.GetUserRoleByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user role not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) createUserRole(context *gin.Context) {
	var req userDTO.CreateUserRoleDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.CreateUserRole(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) updateUserRole(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	var req userDTO.UpdateUserRoleDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.userService.UpdateUserRole(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user role not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *UserHandler) deleteUserRole(context *gin.Context) {
	id, ok := parseUserID(context)
	if !ok {
		return
	}
	err := h.userService.DeleteUserRole(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "user role not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func parseUserID(context *gin.Context) (uint, bool) {
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
