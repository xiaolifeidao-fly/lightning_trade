# Backend Architecture

本文档基于当前仓库 `server/common`、`server/service`、`server/manager-api` 的真实代码结构，描述后端多模块架构、分层职责和新增业务域时应遵循的设计方式。

## 1. 当前项目总体结构

仓库采用 Go multi-module 组织方式，`common`、`service`、`manager-api` 各自独立维护 `go.mod`，通过模块依赖协同工作。

```text
server/
├── common/
│   ├── base/
│   │   ├── dto/base.go
│   │   └── vo/base.go
│   ├── concurrent/
│   ├── converter/
│   ├── middleware/
│   │   ├── db/
│   │   ├── http/
│   │   ├── redis/
│   │   ├── routers/
│   │   ├── storage/oss/
│   │   └── vipper/
│   └── utils/
├── service/
│   ├── [domain]/
└── manager-api/ 面向后台管理的api
    ├── initialization/
    ├── pkg/
    │   ├── [domain]/
    └── routers/
└── app-api/ 面向electron的api
    ├── initialization/
    ├── pkg/
    │   ├── [domain]/
    └── routers/
```

### 三个模块的职责

- `server/common`：公共基础设施与基础类型
- `server/service`：业务域、DTO、Repository、Service
- `server/manager-api`：Gin Handler、路由注册、启动初始化编排

## 2. 核心调用链

```text
Gin HTTP Request
  -> manager-api/pkg/{domain} Handler
  -> service/{domain} Service
  -> service/{domain}/repository Repository
  -> common/middleware/db.Db (GORM)
```

这条链是当前项目最稳定的主路径。

### 两种常见业务模型

#### 2.1 CRUD 业务域

如 `cases`、`chat`、`chat_history`：

- 一个域一个 `dto/`
- 一个域一个 `repository/`
- 一个域一个 `{domain}_service.go`
- 一个域一个 `manager-api/pkg/{domain}/{domain}.go`

#### 2.2 聚合查询业务域

如 `chatroom`：

- 可能没有独立 repository 目录
- Service 直接组合多个 Repository
- Service 内部负责 Join、子查询、聚合 DTO 拼装
- Handler 只暴露聚合查询接口

这类域不是异常，而是当前仓库已经存在的合法模式。

## 3. common 模块职责

### 3.1 `common/base`

提供当前项目的基础数据结构：

- `BaseDTO`
- `QueryDTO`
- `PageDTO[T]`
- `vo.Base` / `vo.Page[V]` 等可选 VO 能力

当前项目的主流数据流是 `Entity -> DTO`，VO 不是默认层。

### 3.2 `common/middleware/db`

这是后端数据层的核心基础设施，包含：

- 全局 `Db *gorm.DB`
- `Repository[T]` 泛型基础仓储
- `BaseEntity`
- `GetRepository[T]()` 仓储工厂
- `ToDTO` / `ToDTOs` / `ToPO` / `ToPOs` 转换工具

#### `Repository[T]` 提供的基础能力

- `FindById`
- `FindAll`
- `Create`
- `SaveOrUpdate`
- `Delete`
- `GetOne`
- `GetList`
- `Execute`

说明：

- 当前项目的 `Delete` 是物理删除能力
- 业务层多数场景实际上走逻辑删除：把 `Active` 改成 `0` 再 `SaveOrUpdate`

#### `BaseEntity`

所有 Entity 通常嵌入 `db.BaseEntity`，默认包含：

- `Id`
- `Active`
- `CreatedTime`
- `UpdatedTime`
- `CreatedBy`
- `UpdatedBy`

目前仓库中的字段 tag 存在混用现象：

- 新增代码通常补 `gorm` tag
- 旧约定仍保留 `orm` tag 和 `description`

新增字段时建议同时兼容现有风格，而不是只保留单一 tag。

#### `GetRepository[T]()` 的真实行为

工厂内部维护一个全局单例 map：

- 第一次获取时创建仓储实例
- 如果仓储实现了 `SetDb(*gorm.DB)`，则注入全局 `Db`
- 后续通过类型名复用同一实例

这带来两个架构含义：

- 业务代码可以低成本获取仓储
- 测试隔离和并发初始化需要额外留意全局状态

### 3.3 `common/middleware/routers`

这一层承接 Web API 对外能力：

- `Handler` 接口
- `BaseHandler`
- `GinRouter`
- `ToJson` / `ToError`

项目当前没有复杂的 Controller 基类体系，设计相对轻量。

### 3.4 其他中间件

- `redis`：Redis 客户端与分布式锁
- `storage/oss`：OSS 抽象与不同云厂商实现
- `http`：HTTP 客户端封装
- `vipper`：配置加载

## 4. service 模块职责

`server/service` 是后端的业务中心。典型域结构如下：

```text
service/{domain}/
├── dto/
│   └── dto.go
├── repository/
│   ├── model.go
│   └── repository.go
└── {domain}_service.go
```

### 4.1 DTO 层

当前仓库的 DTO 通常分为四类：

- `XxxDTO`：出参 DTO，嵌入 `baseDTO.BaseDTO`
- `CreateXxxDTO`：创建入参
- `UpdateXxxDTO`：更新入参，字段通常使用指针
- `XxxQueryDTO`：查询入参，带 `form` 标签

### 4.2 Repository 层

Repository 目录通常拆成两个文件：

- `model.go`：Entity 定义和 `TableName()`
- `repository.go`：Repository 定义、自定义查询、`EnsureTable()`

当前仓库里，`repository.go` 往往很薄，只包一层：

```go
type CaseRepository struct {
	db.Repository[*Case]
}

func (r *CaseRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&Case{})
}
```

### 4.3 Service 层

Service 是业务规则的主要承载者，负责：

- 仓储组合
- 参数兜底
- 分页计算
- 关系校验
- 状态校验
- DTO 转换
- 聚合查询组装

Service 构造函数当前统一通过 `db.GetRepository[T]()` 获取仓储：

```go
func NewCaseService() *CaseService {
	return &CaseService{
		caseRepository: db.GetRepository[repository.CaseRepository](),
	}
}
```

### 4.4 当前仓库的 Service 风格

通过现有模块可以总结出以下稳定约定：

- 对 DB 未初始化做显式防御
- 查询默认过滤 `active = 1`
- 分页默认值通常为 `pageIndex=1`
- `pageSize` 会设置上限
- `GetByID` / `Update` / `Delete` 都会额外判断 `entity.Active == 0`
- 逻辑删除通过 `entity.Active = 0` 完成

## 5. manager-api 模块职责

`server/manager-api` 负责把业务域暴露成 HTTP 服务。

### 5.1 `pkg/`

按业务域放置 Handler，例如：

- `pkg/cases/cases.go`
- `pkg/chat/chat.go`
- `pkg/chat_history/chat_history.go`
- `pkg/chatroom/chatroom.go`

### 5.2 `routers/`

路由初始化拆为两层：

- `register.go`：收集所有 Handler
- `routers.go`：将 Handler 注入 `GinRouter`

真实注册流程：

```text
initialization.Init()
  -> routers.Init()
  -> registerHandler()
  -> InitAllRouters(router, handlers)
  -> router.Include(handler.RegisterHandler)
  -> router.Run()
```

### 5.3 `initialization/`

系统初始化顺序当前为：

1. `vipper.Init()`
2. `db.InitDB()`
3. `redis.InitRedisClient(...)`
4. `oss.Setup(...)`
5. `routers.Init()`

其中：

- Redis 初始化失败会被跳过
- OSS 初始化失败会被跳过
- DB 初始化失败属于致命错误

这意味着业务代码不能假设所有基础设施都可用，但可以假设配置和路由初始化已经完成。

## 6. 数据对象分层

当前项目的主流分层是：

```text
Entity -> DTO -> HTTP JSON
```

说明：

- Repository 层使用 Entity
- Service 层主要返回 DTO
- Handler 层直接把 DTO 输出到 JSON
- VO 工具存在，但属于按需使用

### 转换工具

位于 `common/middleware/db/base.go`：

- `db.ToDTO[D](po)`
- `db.ToDTOs[D](pos)`
- `db.ToPO[V](dto)`
- `db.ToPOs[V](dtos)`

位于 `common/converter/convert.go`：

- `converter.ToVO[V](dto)`
- `converter.ToVOs[V](dtos)`

实践建议：

- 默认优先使用 `Entity -> DTO`
- 只有在对外字段和内部 DTO 差异明显时，再引入 VO

## 7. 初始化与全局状态的架构影响

当前项目广泛使用全局状态：

- `db.Db`
- `db.GetRepository()` 的全局单例 map
- `viper` 全局配置
- `routers` 全局 router 实例

这让启动和编码都很直接，但也带来一些架构风险：

- 单元测试不易隔离
- 跨用例共享状态可能导致串扰
- 初始化时序依赖较强
- Repository 在 DB 初始化前创建，可能持有 `nil` DB

因此新增代码时要保留足够的防御式判断，不要假设基础设施永远已经就绪。

## 8. 新增业务域的推荐结构

### 标准 CRUD 域

```text
service/foo/
├── dto/dto.go
├── repository/model.go
├── repository/repository.go
└── foo_service.go

manager-api/pkg/foo/foo.go
```

然后在 `manager-api/routers/register.go` 注册 `foo.NewFooHandler()`。

### 聚合查询域

如果新域本质上是“面向页面的聚合读模型”，可以参考 `chatroom`：

- 直接在 Service 中组合多个 Repository
- 针对页面定义聚合 DTO
- 不强求每个聚合域都有独立 Entity 和 Repository

## 9. 设计原则总结

### 优先遵循的不是抽象理论，而是仓库现状

- Web 层保持薄
- 业务逻辑落在 Service
- Repository 承接数据访问和 GORM 操作
- DTO 作为默认出参对象
- 聚合域允许跳过标准 CRUD 套路

### 对当前架构最重要的判断标准

新代码是否：

- 保持 `manager-api / service / common` 的职责边界
- 复用现有 `db.Repository[T]` 和 `GetRepository[T]()` 机制
- 与现有响应结构、分页逻辑、逻辑删除习惯一致
- 对全局状态和初始化时序保留足够防御

如果满足以上几点，通常就算是符合当前仓库架构的实现。
