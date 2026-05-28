---
name: add-permission-resources
description: 当用户要求根据本次上下文新增接口或新增页面生成管理端权限 SQL 时使用。用户只需要提供 role_id，本技能输出插入 resource_new 与 role_resource_new 的幂等 SQL，适用于本仓库 manager-api 权限资源补齐。
---

# 新增权限资源

## 目标

根据当前对话或当前代码改动中新增的管理端接口/页面，输出可直接执行的权限资源 SQL。

用户通常只会给出 `role_id`，例如：

```text
role_id = 1
```

你需要从上下文识别新增接口或页面，生成：

- `resource_new` 插入 SQL
- `role_resource_new` 绑定 SQL

只输出 SQL 和必要的简短说明；不要修改代码，除非用户明确要求落库或写文件。

## 表结构约定

`resource_new` 常用字段：

```sql
name,
code,
parent_id,
resource_type,
resource_url,
page_url,
component,
redirect,
menu_name,
meta,
sort_id,
active
```

`role_resource_new` 常用字段：

```sql
role_id,
resource_id,
active
```

管理端接口权限校验主要按 `resource_url` 匹配。Gin 路由参数使用 `:id` 形式，例如：

```text
/app-users/:id
```

不要把同一路径的不同 HTTP 方法合并成一个 `code`。同一个 `resource_url` 可以有多个资源，例如：

- `GET /app-users/:id` -> `app_user:detail`
- `PUT /app-users/:id` -> `app_user:update`
- `DELETE /app-users/:id` -> `app_user:delete`

## 输出规则

1. 必须幂等：所有 `INSERT INTO resource_new` 使用 `SELECT ... WHERE NOT EXISTS (...)`。
2. `role_resource_new` 绑定也必须幂等：使用 `NOT EXISTS` 检查同一 `role_id + resource_id + active = 1`。
3. `role_id` 使用用户提供的值；如果用户没提供，先简短询问，不要猜。
4. `resource_type` 对接口用 `'api'`，对页面用 `'page'`。
5. 接口资源 `page_url/component/redirect/menu_name/meta` 默认为空字符串。
6. `parent_id` 默认 `0`，除非上下文已有明确父级资源 ID。
7. `sort_id` 可从 `100` 起递增；如果上下文已有同模块排序，延续已有排序。
8. 修改密码、禁用/启用等前端操作如果复用已有接口，例如 `PUT /app-users/:id`，归入该接口对应的更新资源，不额外造不存在的后端接口。

## 命名规则

优先按业务域生成 `code`：

```text
{domain}:{action}
```

示例：

```text
app_user:list
app_user:stats
app_user:detail
app_user:create
app_user:update
app_user:delete
```

常见 action 映射：

```text
GET collection/list -> list
GET /:id -> detail
GET stats/count/summary -> stats
POST -> create
PUT/PATCH -> update
DELETE -> delete
page route -> page
```

## SQL 模板

单个资源插入模板：

```sql
INSERT INTO resource_new (
  name,
  code,
  parent_id,
  resource_type,
  resource_url,
  page_url,
  component,
  redirect,
  menu_name,
  meta,
  sort_id,
  active
)
SELECT
  '{资源名称}',
  '{资源编码}',
  0,
  'api',
  '{资源URL}',
  '',
  '',
  '',
  '',
  '',
  {排序},
  1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = '{资源编码}'
    AND resource_url = '{资源URL}'
    AND active = 1
);
```

角色绑定模板：

```sql
INSERT INTO role_resource_new (
  role_id,
  resource_id,
  active
)
SELECT
  {role_id},
  r.id,
  1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN (
    '{资源编码1}',
    '{资源编码2}'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM role_resource_new rr
    WHERE rr.role_id = {role_id}
      AND rr.resource_id = r.id
      AND rr.active = 1
  );
```

## 当前仓库参考

本仓库管理端权限表模型在：

```text
server/service/manager_permission/repository/model.go
```

管理端后端路由通常在：

```text
server/manager-api/pkg/{domain}/{domain}.go
```

生成 SQL 时优先从当前对话上下文判断新增接口；不确定时可读取对应 handler 的 `RegisterHandler`。
