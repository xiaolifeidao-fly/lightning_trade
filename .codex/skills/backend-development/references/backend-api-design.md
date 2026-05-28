# Backend API Design

基于当前仓库 `server/manager-api + server/service + server/common` 的真实实现，总结后端 API 设计约定。目标不是输出通用 REST 教条，而是帮助开发者快速写出和本项目一致的 Handler、DTO、Service 调用链。

## 1. 当前 API 调用链

```text
HTTP Request
  -> manager-api/pkg/{domain} Handler
  -> service/{domain} Service
  -> service/{domain}/repository Repository
  -> common/middleware/db 中的 GORM 基础设施
```

当前仓库有两类接口：

- 标准 CRUD 域：如 `cases`、`chat`、`chat_history`
- 聚合查询域：如 `chatroom`，在 Service 中直接做 Join、子查询、结果组装

## 2. Handler 层设计

Handler 位于 `server/manager-api/pkg/{domain}`，每个域一个文件，通常具备以下结构：

```go
type CaseHandler struct {
	*commonRouter.BaseHandler
	caseService *casesService.CaseService
}

func NewCaseHandler() *CaseHandler {
	service := casesService.NewCaseService()
	_ = service.EnsureTable()

	return &CaseHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		caseService: service,
	}
}

func (h *CaseHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/cases", h.list)
	engine.GET("/cases/:id", h.getByID)
	engine.POST("/cases", h.create)
	engine.PUT("/cases/:id", h.update)
	engine.DELETE("/cases/:id", h.delete)
}
```

### Handler 职责边界

- 负责参数绑定：`ShouldBindQuery`、`ShouldBindJSON`
- 负责路径参数解析：如 `parseID`
- 负责把 Service 结果统一收口成 HTTP 响应
- 可以处理少量表现层错误映射，例如 `gorm.ErrRecordNotFound -> ToError("xxx not found")`
- 不在 Handler 内堆分页、字段清洗、关系校验、状态机等业务逻辑

### 当前仓库中的真实约定

- 构造函数命名统一为 `NewXxxHandler()`
- Handler 结构体实现 `routers.Handler`
- 路由集中在 `RegisterHandler(engine *gin.RouterGroup)` 注册
- 路由最终统一挂在 `request.path` 配置下
- 多数 CRUD Handler 在构造函数里调用 `service.EnsureTable()` 做快速建表

### `EnsureTable()` 的定位

这是当前项目的快速迭代约定，不是正式 migration 方案：

- 适合新模块联调、开发期补表
- 不适合复杂 DDL 变更、字段回填、历史数据迁移
- 如果未来引入独立迁移机制，应把 `EnsureTable()` 从 Handler 构造阶段逐步下线

## 3. 路由设计

### 当前项目推荐风格

- 集合资源：`GET /cases`
- 单资源：`GET /cases/:id`
- 创建：`POST /cases`
- 更新：`PUT /cases/:id`
- 删除：`DELETE /cases/:id`
- 聚合子资源：`GET /chatroom/cases/:caseId/history`

### 命名建议

- 路径使用小写或 kebab-case
- 资源名优先使用复数，和当前新增的 `cases`、`chats`、`chat-histories` 保持一致
- 只在必要时使用动作型路径，例如聚合查询或业务动作明显不属于标准 CRUD

### 避免的写法

```text
GET  /getCase?id=1
POST /createCase
GET  /case_list
```

## 4. 统一响应格式

当前仓库统一使用 `common/middleware/routers/Routers.go` 中的 `ToJson` / `ToError`：

### 成功响应

```json
{
  "success": true,
  "code": 0,
  "data": {},
  "message": "请求成功",
  "error": null
}
```

### 失败响应

```json
{
  "success": false,
  "code": 1,
  "data": null,
  "message": "请求失败",
  "error": "具体错误"
}
```

### 业务失败响应

```json
{
  "success": false,
  "code": 1,
  "data": null,
  "message": "参数错误",
  "error": null
}
```

### 使用规则

- Service 返回 `(data, err)` 时，用 `routers.ToJson(ctx, data, err)`
- 参数错误、资源不存在等可预期业务错误，用 `routers.ToError(ctx, message)`
- 调用 `ToJson` / `ToError` 后，当前分支应立刻 `return`
- 当前仓库错误语义主要靠响应体表达，而不是 HTTP 状态码；大多数响应仍是 `200 OK`

## 5. 参数设计

### 查询参数 DTO

当前项目查询 DTO 普遍放在 `service/{domain}/dto/dto.go` 中，并直接给 Handler 的 `ShouldBindQuery` 使用：

```go
type CaseQueryDTO struct {
	Page                int    `form:"page"`
	PageIndex           int    `form:"pageIndex"`
	PageSize            int    `form:"pageSize"`
	BusinessId          string `form:"businessId"`
	Name                string `form:"name"`
	UserWhatsappAccount string `form:"userWhatsappAccount"`
	WaRemoteJid         string `form:"waRemoteJid"`
	UserID              int    `form:"userId"`
}
```

注意点：

- 当前项目同时兼容 `page` 和 `pageIndex`
- 查询 DTO 不一定嵌入 `baseDTO.QueryDTO`，不少模块是手写字段
- 需要表示“是否传了值”时，用指针字段，例如 `*int8`

### 创建 DTO

```go
type CreateCaseDTO struct {
	BusinessId          string `json:"businessId" binding:"required"`
	Name                string `json:"name" binding:"required"`
	UserWhatsappAccount string `json:"userWhatsappAccount" binding:"required"`
	WaRemoteJid         string `json:"waRemoteJid"`
	UserID              int    `json:"userId"`
}
```

### 更新 DTO

更新 DTO 当前推荐全部使用指针字段区分：

- 未传
- 传了空值
- 传了新值

```go
type UpdateCaseDTO struct {
	BusinessId          *string `json:"businessId,omitempty"`
	Name                *string `json:"name,omitempty"`
	UserWhatsappAccount *string `json:"userWhatsappAccount,omitempty"`
	WaRemoteJid         *string `json:"waRemoteJid,omitempty"`
	UserID              *int    `json:"userId,omitempty"`
}
```

## 6. Service 层返回值设计

当前项目不推荐 Handler 直接操作 Entity，返回值以 DTO 为主：

- 列表查询返回 `*baseDTO.PageDTO[dto.XxxDTO]`
- 详情、创建、更新返回 `*dto.XxxDTO`
- 删除返回 `error`，Handler 常包装成 `gin.H{"deleted": true}`
- 聚合查询返回专用 DTO，如 `ChatroomConversationDTO`

分页统一使用：

```go
return baseDTO.BuildPage(int(total), data), nil
```

## 7. Service 校验与默认值设计

业务校验应放在 Service，而不是 Handler。当前仓库高频模式包括：

- DB 未初始化保护：`repository.Db == nil`
- `req == nil` 防御
- 字符串 `TrimSpace`
- `pageIndex/pageSize` 默认值和上限
- 外键存在性校验，如 `chat` 校验 `case` / `wa account`
- 逻辑删除过滤：统一约定 `active = 1`

典型模式：

```go
if s.caseRepository.Db == nil {
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
```

## 8. Repository 与查询风格

旧文档里“优先原生 SQL”的表述已经不准确。当前仓库更常见的做法是：

- 基础 CRUD 复用 `db.Repository[T]`
- 常规列表筛选使用 GORM 链式查询
- 聚合查询或复杂 Join 时，直接使用 `Db.Table(...).Select(...).Joins(...).Scan(...)`
- 少量场景仍可使用 `GetOne` / `GetList` / `Execute`

因此推荐顺序应为：

1. 优先用 `FindById` / `Create` / `SaveOrUpdate` / `Delete`
2. 常规查询优先 GORM 链式表达
3. 复杂聚合使用 `Table + Select + Joins + Scan`
4. 只有在 GORM 表达明显不顺手时，再退回原生 SQL

## 9. 错误映射建议

当前仓库常见映射方式：

- `gorm.ErrRecordNotFound` -> `routers.ToError(ctx, "xxx not found")`
- 参数绑定失败 -> `routers.ToError(ctx, "参数错误")`
- 业务校验失败 -> 保留 Service 返回错误，由 `ToJson` 输出 `error`

建议新增模块时延续这个风格，避免每个 Handler 自定义不同的错误结构。

## 10. 新增 API 时的最小检查清单

- 是否已在 `manager-api/pkg/{domain}` 创建 Handler
- 是否实现了 `RegisterHandler`
- 是否已在 `manager-api/routers/register.go` 注册
- 是否统一使用 `ToJson` / `ToError`
- 查询 DTO 是否补了 `form` 标签
- 创建 / 更新 DTO 是否补了 `json` 标签
- 更新 DTO 是否使用指针字段
- Service 是否做了 `Db == nil` 防御
- 查询是否默认过滤 `active = 1`
- 分页是否有默认值和上限
- 新表是否真的需要 `EnsureTable()`

## 11. 推荐心智模型

在这个仓库里设计 API 时，把 Handler 当成“薄控制器”，把 Service 当成“业务入口”，把 Repository 当成“数据访问和 GORM 能力承载层”。这样写出来的代码最容易和现有模块保持一致。
