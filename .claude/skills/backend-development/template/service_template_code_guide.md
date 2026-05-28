# Service Template Code Guide

本文档给出一个贴合当前仓库的新增业务域模板，适用于在 `server/service` 新增标准 CRUD 模块。示例使用 `foo` 作为占位业务域。

## 1. 适用范围

适合以下场景：

- 新增一个标准表驱动业务域
- 需要提供列表、详情、创建、更新、删除接口
- 需要在 `manager-api/pkg` 暴露 HTTP API

如果你的需求更接近 `chatroom` 这种聚合查询域，可以只复用 DTO 和 Service 的写法，不必强行创建完整 CRUD 模板。

## 2. 推荐目录结构

```text
server/service/foo/
├── dto/
│   └── dto.go
├── repository/
│   ├── model.go
│   └── repository.go
└── foo_service.go
```

对应的 API Handler 放在：

```text
server/manager-api/pkg/foo/foo.go
```

## 3. DTO 模板

文件：`server/service/foo/dto/dto.go`

```go
package dto

import baseDTO "common/base/dto"

type FooDTO struct {
	baseDTO.BaseDTO
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status int8   `json:"status"`
}

type CreateFooDTO struct {
	Name   string `json:"name" binding:"required"`
	Code   string `json:"code" binding:"required"`
	Status int8   `json:"status"`
}

type UpdateFooDTO struct {
	Name   *string `json:"name,omitempty"`
	Code   *string `json:"code,omitempty"`
	Status *int8   `json:"status,omitempty"`
}

type FooQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Name      string `form:"name"`
	Code      string `form:"code"`
	Status    *int8  `form:"status"`
}
```

### DTO 设计要点

- 返回 DTO 嵌入 `baseDTO.BaseDTO`
- 创建 DTO 用值类型并补 `binding:"required"`
- 更新 DTO 优先用指针字段
- 查询 DTO 补 `form` 标签
- 若前端仍兼容旧分页字段，保留 `page`

## 4. Entity 模板

文件：`server/service/foo/repository/model.go`

```go
package repository

import "common/middleware/db"

type Foo struct {
	db.BaseEntity
	Name   string `gorm:"column:name;type:varchar(128);not null;default:'';index:idx_name" orm:"column(name);size(128);null" description:"名称"`
	Code   string `gorm:"column:code;type:varchar(64);not null;default:'';uniqueIndex:uk_code" orm:"column(code);size(64);null" description:"编码"`
	Status int8   `gorm:"column:status;type:tinyint;not null;default:1;index:idx_status" orm:"column(status);null" description:"状态"`
}

func (f *Foo) TableName() string {
	return "foo"
}
```

### Entity 设计要点

- 默认嵌入 `db.BaseEntity`
- 表名由 `TableName()` 显式声明
- 新字段建议同时保留 `gorm`、`orm`、`description` 风格，和仓库现状兼容
- 查询高频字段可提前补索引 tag

## 5. Repository 模板

文件：`server/service/foo/repository/repository.go`

```go
package repository

import (
	"common/middleware/db"
	"fmt"
)

type FooRepository struct {
	db.Repository[*Foo]
}

func (r *FooRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&Foo{})
}
```

### Repository 设计要点

- 默认组合嵌入 `db.Repository[*Foo]`
- 常规 CRUD 先复用基类
- 复杂查询优先直接用 `r.Db`
- 只有必要时再补 `GetOne` / `GetList` 风格的原生 SQL 方法

## 6. Service 模板

文件：`server/service/foo/foo_service.go`

```go
package foo

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	fooDTO "service/foo/dto"
	fooRepository "service/foo/repository"
	"strings"

	"gorm.io/gorm"
)

type FooService struct {
	fooRepository *fooRepository.FooRepository
}

func NewFooService() *FooService {
	return &FooService{
		fooRepository: db.GetRepository[fooRepository.FooRepository](),
	}
}

func (s *FooService) EnsureTable() error {
	return s.fooRepository.EnsureTable()
}

func (s *FooService) List(query fooDTO.FooQueryDTO) (*baseDTO.PageDTO[fooDTO.FooDTO], error) {
	if s.fooRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}

	pageIndex := query.PageIndex
	if pageIndex <= 0 {
		pageIndex = query.Page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}

	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	dbQuery := s.fooRepository.Db.Model(&fooRepository.Foo{}).Where("active = ?", 1)

	if name := strings.TrimSpace(query.Name); name != "" {
		dbQuery = dbQuery.Where("name LIKE ?", "%"+name+"%")
	}
	if code := strings.TrimSpace(query.Code); code != "" {
		dbQuery = dbQuery.Where("code LIKE ?", "%"+code+"%")
	}
	if query.Status != nil {
		dbQuery = dbQuery.Where("status = ?", *query.Status)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	var entities []*fooRepository.Foo
	if err := dbQuery.
		Order("id DESC").
		Offset((pageIndex - 1) * pageSize).
		Limit(pageSize).
		Find(&entities).Error; err != nil {
		return nil, err
	}

	data := db.ToDTOs[fooDTO.FooDTO](entities)
	return baseDTO.BuildPage(int(total), data), nil
}

func (s *FooService) GetByID(id uint) (*fooDTO.FooDTO, error) {
	entity, err := s.fooRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[fooDTO.FooDTO](entity), nil
}

func (s *FooService) Create(req *fooDTO.CreateFooDTO) (*fooDTO.FooDTO, error) {
	if s.fooRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	name := strings.TrimSpace(req.Name)
	code := strings.TrimSpace(req.Code)
	if name == "" || code == "" {
		return nil, fmt.Errorf("name and code are required")
	}

	created, err := s.fooRepository.Create(&fooRepository.Foo{
		Name:   name,
		Code:   code,
		Status: req.Status,
	})
	if err != nil {
		return nil, err
	}

	return db.ToDTO[fooDTO.FooDTO](created), nil
}

func (s *FooService) Update(id uint, req *fooDTO.UpdateFooDTO) (*fooDTO.FooDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	entity, err := s.fooRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		entity.Name = name
	}
	if req.Code != nil {
		code := strings.TrimSpace(*req.Code)
		if code == "" {
			return nil, fmt.Errorf("code cannot be empty")
		}
		entity.Code = code
	}
	if req.Status != nil {
		entity.Status = *req.Status
	}

	saved, err := s.fooRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}

	return db.ToDTO[fooDTO.FooDTO](saved), nil
}

func (s *FooService) Delete(id uint) error {
	entity, err := s.fooRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}

	entity.Active = 0
	_, err = s.fooRepository.SaveOrUpdate(entity)
	return err
}
```

### Service 编写要点

- 构造函数统一走 `db.GetRepository[T]()`
- `List` 里统一处理分页默认值和上限
- 所有查询默认过滤 `active = 1`
- `GetByID/Update/Delete` 补 `Active == 0` 判断
- 创建和更新时做 `TrimSpace`、空值校验、范围校验
- 逻辑删除优先，不直接调用基类 `Delete`

## 7. 对应 Handler 模板

文件：`server/manager-api/pkg/foo/foo.go`

```go
package foo

import (
	commonRouter "common/middleware/routers"
	"net/http"
	fooService "service/foo"
	fooDTO "service/foo/dto"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FooHandler struct {
	*commonRouter.BaseHandler
	fooService *fooService.FooService
}

func NewFooHandler() *FooHandler {
	service := fooService.NewFooService()
	_ = service.EnsureTable()

	return &FooHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		fooService:  service,
	}
}

func (h *FooHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/foos", h.list)
	engine.GET("/foos/:id", h.getByID)
	engine.POST("/foos", h.create)
	engine.PUT("/foos/:id", h.update)
	engine.DELETE("/foos/:id", h.delete)
}

func (h *FooHandler) list(context *gin.Context) {
	var query fooDTO.FooQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}

	result, err := h.fooService.List(query)
	commonRouter.ToJson(context, result, err)
}

func (h *FooHandler) getByID(context *gin.Context) {
	id, ok := parseID(context)
	if !ok {
		return
	}

	result, err := h.fooService.GetByID(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "foo not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *FooHandler) create(context *gin.Context) {
	var req fooDTO.CreateFooDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}

	result, err := h.fooService.Create(&req)
	commonRouter.ToJson(context, result, err)
}

func (h *FooHandler) update(context *gin.Context) {
	id, ok := parseID(context)
	if !ok {
		return
	}

	var req fooDTO.UpdateFooDTO
	if err := context.ShouldBindJSON(&req); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}

	result, err := h.fooService.Update(id, &req)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "foo not found")
		return
	}
	commonRouter.ToJson(context, result, err)
}

func (h *FooHandler) delete(context *gin.Context) {
	id, ok := parseID(context)
	if !ok {
		return
	}

	err := h.fooService.Delete(id)
	if err == gorm.ErrRecordNotFound {
		commonRouter.ToError(context, "foo not found")
		return
	}
	commonRouter.ToJson(context, gin.H{"deleted": true}, err)
}

func parseID(context *gin.Context) (uint, bool) {
	idValue := context.Param("id")
	id, err := strconv.ParseUint(idValue, 10, 32)
	if err != nil || id == 0 {
		context.JSON(http.StatusOK, gin.H{
			"code":  commonRouter.FailCode,
			"data":  nil,
			"error": "id必须是正整数",
		})
		return 0, false
	}
	return uint(id), true
}
```

别忘了在 `server/manager-api/routers/register.go` 注册：

```go
func registerHandler() []routers.Handler {
	return []routers.Handler{
		foo.NewFooHandler(),
	}
}
```

## 8. 什么时候不要照抄这个模板

以下场景需要调整：

- 这是一个聚合读模型，不对应单表 CRUD
- 需要跨多个仓储做联查或组装
- 创建 / 更新有复杂状态流转
- 需要事务、幂等、锁、外部服务调用

这时建议保留目录结构和基础约定，但把重点放在 Service 设计上，而不是强行把逻辑塞进模板式 CRUD。

## 9. 开发完成后的自检清单

- 是否已创建 `dto.go`、`model.go`、`repository.go`、`foo_service.go`
- 是否已创建 `manager-api/pkg/foo/foo.go`
- 是否已在 `manager-api/routers/register.go` 注册
- 是否默认过滤了 `active = 1`
- 是否对 `Db == nil` 做了防御
- 是否给分页补了默认值和上限
- 更新 DTO 是否使用了指针字段
- 创建 / 更新是否做了字符串清洗和必填校验
- 删除是否使用逻辑删除
- 新表是否真的需要 `EnsureTable()`

## 10. 推荐使用方式

新增一个新模块时，先把这份模板替换成你的领域名，再对照现有 `cases`、`chat`、`chat_history` 做细节收敛。模板的作用是帮你快速起步，最终仍以仓库现有实现风格为准。
