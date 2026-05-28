---
name: backend-development
description: 面向本仓库 server/common + server/service + server/manager-api 三模块 Go 后端的开发技能。适用于新增业务域、扩展 Gin 接口、编写 Service/Repository、接入 GORM/MySQL/Redis/OSS、排查初始化与数据流问题，并在需要时参考通用高可用、性能优化与测试实践。
license: MIT
version: 1.1.0
---

# Backend Development Skill

本技能优先遵循当前仓库 `server/` 的真实实现，而不是套用通用 Go 后端模板。

## 何时使用

- 在 `server/manager-api/pkg/*` 新增或修改 HTTP Handler
- 在 `server/service/*` 新增业务域、DTO、Repository、Service
- 在 `server/service/{domain}/consumer` 新增或修改异步消费者、队列消息结构、消费循环
- 在 `server/common/*` 调整公共基础设施，如 DB、Redis、Router、Viper、OSS
- 排查接口响应、分页查询、数据库初始化、Repository 注入、配置读取问题
- 需要在当前项目约定下做性能优化、测试补充或高可用增强

## 先看真实项目结构

当前仓库采用 Go multi-module 结构，核心目录如下：

## Kakrolot 是原项目的Java服务端
   当前是正在用go来重构原来的Java端的代码

## 当前项目的核心调用链

```text
HTTP Request
  -> manager-api/pkg/{domain} Handler
  -> service/{domain} Service
  -> service/{domain}/repository Repository
  -> common/middleware/db 中的 GORM 基础设施

Async Message
  -> service/{domain}/consumer Consumer
  -> service/{domain} Service
  -> service/{domain}/repository Repository
```

这是当前项目最重要的分层约定：

- `manager-api/pkg/*` 负责入参绑定、HTTP 状态收口、统一输出，面相管理端界面的api
- `app-api/pkg/*` 负责入参绑定、HTTP 状态收口、统一输出，面相electron的api
- `service/*` 负责业务逻辑、参数兜底、分页、状态机、幂等处理
- `service/*/consumer` 负责异步消息入队、出队、消费循环和消息反序列化，不承载领域业务细节
- `service/*/repository` 负责实体定义和数据访问
- `common/middleware/db` 提供泛型 CRUD 基础能力与 Repository 工厂

## 必须遵守的项目约定

### 1. Handler 放在 `server/manager-api/pkg/{domain}`

- 每个业务域一个 handler 文件，例如 `pkg/account/account.go`
- Handler 实现 `common/middleware/routers.Handler`
- 通常嵌入 `*routers.BaseHandler`
- 路由在 `RegisterHandler(engine *gin.RouterGroup)` 中集中注册
- 请求成功或失败优先使用 `routers.ToJson` / `routers.ToError`

典型模式：

```go
type AccountHandler struct {
    *commonRouter.BaseHandler
    accountService *accountService.AccountService
}

func (h *AccountHandler) RegisterHandler(engine *gin.RouterGroup) {
    engine.GET("/accounts", h.list)
    engine.POST("/accounts", h.create)
}
```

### 2. Service 放在 `server/service/{domain}`

- 构造函数里通过 `db.GetRepository[repository.XxxRepository]()` 获取仓储单例
- Service 负责参数校验、默认值处理、分页、业务状态判断
- 返回值以 DTO 为主，不直接向 Handler 暴露 Entity
- 当前项目里部分 Service 会提供 `EnsureTable()`，供 Handler 初始化时触发表结构创建

### 3. Repository 放在 `server/service/{domain}/repository`

- Repository 通过组合嵌入 `db.Repository[*Entity]`
- 自定义查询优先直接使用 `r.Db` 的 GORM 能力；基础 CRUD 可复用泛型基类
- 当 DB 未初始化时，要主动返回明确错误，如 `database is not initialized`

示例：

```go
type AccountRepository struct {
    db.Repository[*Account]
}

func (r *AccountRepository) FindByRemoteJid(remoteJid string) (*Account, error) {
    var entity Account
    err := r.Db.Where("remote_jid = ? AND active = ?", remoteJid, 1).
        Order("id DESC").
        First(&entity).Error
    if err != nil {
        return nil, err
    }
    return &entity, nil
}
```

### 4. Consumer 放在 `server/service/{domain}/consumer`

- 只要提到后端 consumer、消费者、异步队列消费，都优先放到 `server/service/{domain}/consumer`
- Consumer 包负责队列 key、消息结构、入队函数、消费循环、日志和上下文退出
- 具体业务处理继续放在 `service/{domain}`，通过接口注入给 consumer，避免 consumer 和 service 互相 import
- 初始化入口在 `manager-api/initialization` 或 `app-api/initialization` 中显式启动 consumer
- 具体写法参考 `references/backend-consumer.md`

典型模式：

```text
service/{domain}/consumer
  -> 定义 XxxMessage / EnqueueXxx / StartXxxConsumer
service/{domain}
  -> 实现 ProcessXxx(...) 等业务处理方法
initialization
  -> consumer.StartXxxConsumer(ctx, domainService.NewXxxService())
```

### 5. Entity / DTO 是当前项目主流数据对象

当前项目实际数据流以 `Entity <-> DTO` 为主，VO 是可选能力，不是默认层。

```text
Repository 使用 Entity
Service / Handler 主要使用 DTO
需要额外出参视图时才考虑 converter 中的 VO 转换
```

转换工具：

- `db.ToDTO[D](entity)`
- `db.ToDTOs[D](entities)`
- `db.ToPO[V](dto)`
- `db.ToPOs[V](dtos)`
- `converter.ToVO[V](dto)` 仅在确实需要 VO 分层时使用

### 6. DTO 设计贴近当前仓库

在 `service/{domain}/dto/dto.go` 中通常定义：

- `XxxDTO`：响应结构，嵌入 `baseDTO.BaseDTO`
- `CreateXxxDTO`：创建入参
- `UpdateXxxDTO`：更新入参，字段通常用指针区分“未传”和“传空值”
- `XxxQueryDTO`：查询参数，承接 `ShouldBindQuery`

分页统一使用 `common/base/dto.PageDTO[T]`，构造可复用 `baseDTO.BuildPage(total, data)`。

### 7. 初始化顺序遵循 `manager-api/initialization`

真实初始化顺序如下：

1. `vipper.Init()`
2. `db.InitDB()`
3. `redis.InitRedisClient(...)`
4. `oss.Setup(...)`
5. `routers.Init()`

处理时要注意：

- Redis、OSS 当前被视为“可选初始化”，失败会跳过
- DB 初始化失败后 `db.Db` 可能为 `nil`，Service / Repository 里必须做好空指针防御
- `vipper` 当前固定读取 `./configs/application.properties`

## 在本项目中新增一个业务域时怎么做

以新增 `foo` 域为例，优先按下面路径落地：

1. 新建 `server/service/foo/dto/dto.go`
2. 新建 `server/service/foo/repository/model.go`
3. 新建 `server/service/foo/repository/repository.go`
4. 新建 `server/service/foo/foo_service.go`
5. 如需异步消费，新建 `server/service/foo/consumer/{xxx}_consumer.go`
6. 新建 `server/manager-api/pkg/foo/foo.go`
7. 在 `server/manager-api/routers/register.go` 注册 `foo.NewFooHandler()`
8. 如有 consumer，在 `server/manager-api/initialization` 中按依赖顺序启动

落地原则：

- 先定义 Entity 和表名，再定义 DTO
- Repository 先复用泛型 CRUD，不够再写领域查询
- Service 负责聚合分页、业务校验和 DTO 转换
- Handler 只做绑定、调用、响应，不堆业务判断

## 编码时的高频检查点

- 新增路由后，是否已在 `register.go` 注册
- DTO 的 `json` / `form` / `binding` 标签是否与前端请求一致
- 更新 DTO 是否使用指针字段，避免误覆盖
- 查询是否默认带 `active = 1`，与当前逻辑删除习惯保持一致
- Service 中是否检查 `repository.Db == nil`
- Consumer 是否放在 `service/{domain}/consumer`，业务处理是否通过 Service 接口注入
- Consumer 启动顺序是否晚于 Redis/DB 初始化，并支持 `context.Context` 退出
- 分页是否统一兜底：`pageIndex <= 0 => 1`、`pageSize <= 0 => 20`
- 新表是否需要 `EnsureTable()` 或迁移逻辑
- 错误是否通过 `ToJson` / `ToError` 统一输出

## 当前项目值得延续的实践

- 用 `db.Repository[T]` 承接通用 CRUD，减少重复代码
- 用 `db.GetRepository[T]()` 统一注入仓储实例
- 用 DTO 指针字段实现 PATCH/PUT 风格的部分更新
- 在 Service 层做状态归一化，例如 `NormalizeStatus`
- 在 `initialization` 中集中管理基础设施启动顺序

## 当前项目的真实风险点

这些点在开发时需要额外留意：

- `vipper.Init()` 固定读 `application.properties`，环境切换能力较弱
- `db.GetRepository[T]()` 使用全局单例 map，测试隔离和并发初始化要小心
- `EnsureTable()` 放在 Handler 构造阶段，适合快速迭代，但不等同于正式迁移方案
- `routers.ToJson` 当前总是返回 HTTP 200，错误语义依赖 body 中的 `success/code/error`
- 部分基础层仍混用 `orm` 与 `gorm` tag，新增字段时要和现有风格保持兼容

## 使用参考文档的方式

优先级建议如下：

1. 先看 `server/` 真实代码
2. 再看 `references/backend-architecture.md` 理解整体分层
3. 涉及 consumer / 异步队列消费时看 `references/backend-consumer.md`
4. 需要性能优化时看 `references/backend-performance.md`
5. 需要高可用治理时看 `references/backend-go-ha.md`
6. 需要补测试时看 `references/backend-testing.md`

如果参考文档和仓库现状不一致，以仓库现状为准，再在改动中逐步收敛。

## Reference Navigation

- `references/backend-architecture.md`：多模块架构、分层职责、Repository/DTO/VO 设计
- `references/backend-consumer.md`：Consumer 包位置、接口注入、Redis 队列消费写法
- `references/backend-performance.md`：索引、连接池、缓存、异步处理、性能监控
- `references/backend-go-ha.md`：优雅关闭、超时控制、重试、熔断、限流、分布式锁
- `references/backend-testing.md`：单元测试、集成测试、负载测试、安全扫描

## 一句话工作流

在这个仓库里做后端开发时，先从 `server/manager-api/pkg`、`server/service`、`server/common/middleware` 的真实实现建立上下文，再按现有三模块分层补齐 Handler、Service、Repository、DTO 和 `service/{domain}/consumer`，而不是先套一个外部脚手架式架构。
