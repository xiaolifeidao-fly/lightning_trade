---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r9-022661de71"
requirementKey: "req-1788169552180"
resourceName: "manager-api 新增 argus_event 事件查询接口"
conversationTitle: "manager-api 新增 argus_event 事件查询接口"
threadId: "aad83467-6d53-4d40-a1bb-a6ae35340c7b"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-01T16:50:52.229829+00:00"
---

# 任务聊天 · manager-api 新增 argus_event 事件查询接口

## 第 1 轮 · completed · E0MFMgM_iFYtdVuFndTtxg

### 用户

执行「动作执行」阶段：manager-api 新增 argus_event 事件查询接口

### 助手

I'll start by loading the delivery execution skill and reading the requirement docs.

### 助手

I'll start by reading the requirement docs and surveying the codebase.

### 助手

Now I'll write the new domain. Starting with the DTOs.

### 助手

Now the core service file.

### 助手

Now the handler and route registration.

### 助手

Now tests for the aggregation paths.

### 助手

Now let me run the integration suite against a throwaway local schema.

### 助手

Now the design-process document.

### 助手

## 改了什么

**新增域 `service/argus_event`（只读，不写任何数据、不碰实盘链路）**

| 文件 | 意图 |
|---|---|
| `server/service/argus_event/dto/dto.go` | 8 个接口的出入参；时间口径统一为本地墙钟串 |
| `server/service/argus_event/repository/repository.go` | 三张事实表（`strategy_event`/`balance_sample`/`dev_sample`）的只读仓储；复用 `eventstore` 的表定义，不重打 schema |
| `server/service/argus_event/catalog.go` | 口径字典：事件类型/结果大类/门控种类/信号强度分档，枚举全部从写侧常量引出 |
| `server/service/argus_event/service.go` | 入参归一、窗口解析、信号列表、信号详情（逐账户判定 + TG 明细还原） |
| `server/service/argus_event/slice.go` | 触发瞬间秒级切片（含降级自曝，见下） |
| `server/service/argus_event/timeline.go` | K 线对齐的时间轴聚合 + 权益曲线 |
| `server/service/argus_event/stats.go` | 拦截原因聚合、跨实例汇总对比、筛选项枚举 |
| `server/manager-api/pkg/argus_event/argus_event.go` | 8 个只读 GET |
| `server/manager-api/routers/register.go`（改，+2 行） | 按 `build()` 模式注册 |
| `doc/module/r9-022661de71/权限资源.sql` | 8 条 api 资源 + 角色绑定，幂等 |

接口：`/argus-event/filter-options`、`/signals`、`/signals/:id`、`/signals/:id/slice`、`/timeline`、`/equity-curve`、`/gate-stats`、`/instance-summary`。

**三个关键设计决定**（完整推演见设计文档）：

1. **时间全程按串走**。`argus_single` 的 DSN 强制 `loc=Local`，manager-api 的 `sqlconn` 由部署方给、仓库里看不到也不保证带 loc——跨进程 loc 不一致会让 `strategy_event.ts` 整体偏 8 小时。所以查询边界用墙钟串、出参用 `DATE_FORMAT` 读成串，读到的字符与 JSONL/TG 消息逐字一致。集成测试用一条**故意不带 loc** 的读连接把这一点钉死了。
2. **信号归组按报价快照三元组** `(sig_last, sig_mark, gap_bp)`，不按 ts。各账户在独立 goroutine 里判定，成交事件要等下单往返、实测比拦截事件晚数秒；而相邻触发间隔中位只有 61 秒，时间窗没有语义。三元组由 `applySignalQuote` 复制给同一次触发的每个账户，是语义判据。
3. **实例是硬隔离维度**。`/timeline`、`/equity-curve` 强制单实例（缺 `instanceKey` 直接报错）；`/instance-summary` 逐实例独立成行，服务端不做任何跨实例求和。

设计过程文档：[doc/module/r9-022661de71/design/设计过程.md](doc/module/r9-022661de71/design/设计过程.md)
权限 SQL：[doc/module/r9-022661de71/权限资源.sql](doc/module/r9-022661de71/权限资源.sql)

## 怎么验的

- `go build ./...` + `go test ./...`：`service` / `manager-api` / `argus_single` 三个模块全绿；`gofmt -l`、`go vet` 无输出。
- **19 条单元测试**（纯函数）：时间口径不偏移与右界补齐、筛选装配与冲突检测、强度分档边界、信号归组四种情形、详情与 TG 明细还原、时间轴分桶与净仓阶梯、门控聚合、跨实例隔离、权益变动、切片折叠。
- **6 条真实 MySQL 集成测试**（`ARGUS_EVENT_TEST_DSN` 门控，默认跳过）：本轮在本机一次性 schema `argus_event_check` 上全部跑通，跑完已 DROP。覆盖时区不漂移、实例不串数据、同分钟三次触发分别定位到秒、切片/时间轴/权益、聚合与汇总、默认窗口跟随数据。
- **1 条路由测试**：gin engine 实际注册 8 条路由并逐条核对（`/signals/:id` 与 `/signals/:id/slice` 共存不冲突）。

## 对照任务说明

| 项 | 状态 |
|---|---|
| 信号列表（8 个维度筛选） | ✅ 实例/时间/币种/账户+uid/事件类型/结果/gapBp 强度/参数版本，另加变体、方向、数据来源 |
| 信号详情（逐账户判定，TG `[n]`/`[跳过n]`） | ⚠️ 部分满足——门控跳过行的 ROI/阈值/reason 完整还原；**成交行的「均价/成交」还原不了**，这两个价取自交易所下单回执，r4 的埋点未落库。返回体带 `fillPriceAvailable:false` + 说明，前端显示"—"而不是 0 |
| 秒级切片查询 | ⚠️ 降级供数——r3（±1min 逐秒切片采集）未实现，仓库里没有切片表。改用 `strategy_event` 自带的真实秒级报价观测点，用 `tickSource`/`tickComplete`/`degradedReason` 自曝；契约按 r3 形态预留，落地后前端不用改 |
| 与 K 线对齐的时间轴聚合 | ✅ 含桶内结果分布、最大 \|gapBp\|、净持仓阶梯、已实现盈亏、K 线覆盖率（缺口回填空状态） |
| 权益曲线 | ✅ 按 `(实例, 账户)` 归集，SQL 侧降采样，`equity_known` 口径正确 |
| 拦截原因聚合 | ✅ 按 `gate_kind` + 阈值/实测均值 + 信号强度分级 |
| 跨实例汇总对比 | ✅ 逐实例独立，未注册实例标 `registered:false` |
| 权限资源 SQL | ✅ 8 条 api 资源；未做 page 资源（页面属于 r12/r13/r15） |
| 不做前端 | ✅ |

## 测试交接信息

- 入口：manager-api 起来后走 `GET {request.path}/argus-event/*`，需先执行权限 SQL 并确认 `@role_id`（本轮未拿到目标角色，默认写的 1）。
- 环境：一套含 `strategy_event` / `balance_sample` / `dev_sample` 的 MySQL。r6 已回灌两个实例（`argus-single-roc`、`argus-single-ives`，16.6 万条，覆盖 6/28–8/21）。
- 建议验收顺序：`filter-options` 拿到真实实例/账户 → `signals?instanceKey=…&start=2026-08-18&end=2026-08-21`（对 613 次触发）→ 挑一条同分钟多触发的看 `signals/:id` 与 `:id/slice` → `timeline` / `equity-curve` / `gate-stats` / `instance-summary`。
- 集成测试自跑：`ARGUS_EVENT_TEST_DSN='<可丢弃 schema 的 DSN>' go test ./argus_event/ -run TestIntegration -v`。**必须指向可丢弃的库**，用例只操作 `it-` 前缀的测试实例键。
- 清理：接口只读，无副作用；`EnsureTable` 的 AutoMigrate 幂等。

## 遗留与风险

1. **秒级切片依赖 r3**，当前是降级形态（R1）。
2. **成交价埋点缺失**（R2）：建议在 r4 侧补 `fill_px`/`order_avg_px`，否则需求大纲 §6.1 验收 1 的"完整还原一条 TG 消息"只能做到"除这两个价之外完整"。
3. **净持仓阶梯是派生量**（R3），部分平仓与重启期间的变化追不到；精确生命周期等 r10。
4. **`service/trade` 的信号回测读侧仍用 `time.Time`**（R4）——在 `sqlconn` 缺 `loc=Local` 的部署上有同样的 8 小时隐患。本轮没有夹带改它，建议单开小任务。
5. 聚合有 20 万行上限，命中时返回 `truncated:true`；当前全库量不会触发，多币种接入后需复核。

读了哪些文档：需求大纲（全文，验收口径以它为准）、`prototype/` 的 signals/market/index 三页（本接口的直接消费方，字段按它们对齐）、r6 的一致性巡检报告（确认库内真实数据形态）；r8 设计文档只查了它对 r3 现状的判断。**没读 r1 的需求文档**——它讲实例注册与配置分域，我直接读了落地后的 `service/argus_config/instance.go` 与 `repository/model.go`（代码比文档更接近事实），其余分域内容与只读查询无关。

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJFME1GTWdNX2lGWXRkVnVGbmRUdHhnIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMVQxNjoxNTo1MC4yNjk1MjYrMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDFUMTY6NTA6NTEuNjg1NjM0KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8mm1hbmFnZXItYXBpIOaWsOWiniBhcmd1c19ldmVudCDkuovku7bmn6Xor6LmjqXlj6MifV19LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIHN0YXJ0IGJ5IGxvYWRpbmcgdGhlIGRlbGl2ZXJ5IGV4ZWN1dGlvbiBza2lsbCBhbmQgcmVhZGluZyB0aGUgcmVxdWlyZW1lbnQgZG9jcy4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIHN0YXJ0IGJ5IHJlYWRpbmcgdGhlIHJlcXVpcmVtZW50IGRvY3MgYW5kIHN1cnZleWluZyB0aGUgY29kZWJhc2UuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IEknbGwgd3JpdGUgdGhlIG5ldyBkb21haW4uIFN0YXJ0aW5nIHdpdGggdGhlIERUT3MuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBjb3JlIHNlcnZpY2UgZmlsZS4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIGhhbmRsZXIgYW5kIHJvdXRlIHJlZ2lzdHJhdGlvbi4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGVzdHMgZm9yIHRoZSBhZ2dyZWdhdGlvbiBwYXRocy4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgbGV0IG1lIHJ1biB0aGUgaW50ZWdyYXRpb24gc3VpdGUgYWdhaW5zdCBhIHRocm93YXdheSBsb2NhbCBzY2hlbWEuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBkZXNpZ24tcHJvY2VzcyBkb2N1bWVudC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiIjIyDmlLnkuobku4DkuYhcblxuKirmlrDlop7ln58gYHNlcnZpY2UvYXJndXNfZXZlbnRg77yI5Y+q6K+777yM5LiN5YaZ5Lu75L2V5pWw5o2u44CB5LiN56Kw5a6e55uY6ZO+6Lev77yJKipcblxufCDmlofku7YgfCDmhI/lm74gfFxufC0tLXwtLS18XG58IGBzZXJ2ZXIvc2VydmljZS9hcmd1c19ldmVudC9kdG8vZHRvLmdvYCB8IDgg5Liq5o6l5Y+j55qE5Ye65YWl5Y+C77yb5pe26Ze05Y+j5b6E57uf5LiA5Li65pys5Zyw5aKZ6ZKf5LiyIHxcbnwgYHNlcnZlci9zZXJ2aWNlL2FyZ3VzX2V2ZW50L3JlcG9zaXRvcnkvcmVwb3NpdG9yeS5nb2AgfCDkuInlvKDkuovlrp7ooajvvIhgc3RyYXRlZ3lfZXZlbnRgL2BiYWxhbmNlX3NhbXBsZWAvYGRldl9zYW1wbGVg77yJ55qE5Y+q6K+75LuT5YKo77yb5aSN55SoIGBldmVudHN0b3JlYCDnmoTooajlrprkuYnvvIzkuI3ph43miZMgc2NoZW1hIHxcbnwgYHNlcnZlci9zZXJ2aWNlL2FyZ3VzX2V2ZW50L2NhdGFsb2cuZ29gIHwg5Y+j5b6E5a2X5YW477ya5LqL5Lu257G75Z6LL+e7k+aenOWkp+exuy/pl6jmjqfnp43nsbsv5L+h5Y+35by65bqm5YiG5qGj77yM5p6a5Li+5YWo6YOo5LuO5YaZ5L6n5bi46YeP5byV5Ye6IHxcbnwgYHNlcnZlci9zZXJ2aWNlL2FyZ3VzX2V2ZW50L3NlcnZpY2UuZ29gIHwg5YWl5Y+C5b2S5LiA44CB56qX5Y+j6Kej5p6Q44CB5L+h5Y+35YiX6KGo44CB5L+h5Y+36K+m5oOF77yI6YCQ6LSm5oi35Yik5a6aICsgVEcg5piO57uG6L+Y5Y6f77yJIHxcbnwgYHNlcnZlci9zZXJ2aWNlL2FyZ3VzX2V2ZW50L3NsaWNlLmdvYCB8IOinpuWPkeeerOmXtOenkue6p+WIh+eJh++8iOWQq+mZjee6p+iHquabne+8jOingeS4i++8iSB8XG58IGBzZXJ2ZXIvc2VydmljZS9hcmd1c19ldmVudC90aW1lbGluZS5nb2AgfCBLIOe6v+Wvuem9kOeahOaXtumXtOi9tOiBmuWQiCArIOadg+ebiuabsue6vyB8XG58IGBzZXJ2ZXIvc2VydmljZS9hcmd1c19ldmVudC9zdGF0cy5nb2AgfCDmi6bmiKrljp/lm6DogZrlkIjjgIHot6jlrp7kvovmsYfmgLvlr7nmr5TjgIHnrZvpgInpobnmnprkuL4gfFxufCBgc2VydmVyL21hbmFnZXItYXBpL3BrZy9hcmd1c19ldmVudC9hcmd1c19ldmVudC5nb2AgfCA4IOS4quWPquivuyBHRVQgfFxufCBgc2VydmVyL21hbmFnZXItYXBpL3JvdXRlcnMvcmVnaXN0ZXIuZ29g77yI5pS577yMKzIg6KGM77yJIHwg5oyJIGBidWlsZCgpYCDmqKHlvI/ms6jlhowgfFxufCBgZG9jL21vZHVsZS9yOS0wMjI2NjFkZTcxL+adg+mZkOi1hOa6kC5zcWxgIHwgOCDmnaEgYXBpIOi1hOa6kCArIOinkuiJsue7keWumu+8jOW5guetiSB8XG5cbuaOpeWPo++8mmAvYXJndXMtZXZlbnQvZmlsdGVyLW9wdGlvbnNg44CBYC9zaWduYWxzYOOAgWAvc2lnbmFscy86aWRg44CBYC9zaWduYWxzLzppZC9zbGljZWDjgIFgL3RpbWVsaW5lYOOAgWAvZXF1aXR5LWN1cnZlYOOAgWAvZ2F0ZS1zdGF0c2DjgIFgL2luc3RhbmNlLXN1bW1hcnlg44CCXG5cbioq5LiJ5Liq5YWz6ZSu6K6+6K6h5Yaz5a6aKirvvIjlrozmlbTmjqjmvJTop4Horr7orqHmlofmoaPvvInvvJpcblxuMS4gKirml7bpl7TlhajnqIvmjInkuLLotbAqKuOAgmBhcmd1c19zaW5nbGVgIOeahCBEU04g5by65Yi2IGBsb2M9TG9jYWxg77yMbWFuYWdlci1hcGkg55qEIGBzcWxjb25uYCDnlLHpg6jnvbLmlrnnu5njgIHku5PlupPph4znnIvkuI3liLDkuZ/kuI3kv53or4HluKYgbG9j4oCU4oCU6Leo6L+b56iLIGxvYyDkuI3kuIDoh7TkvJrorqkgYHN0cmF0ZWd5X2V2ZW50LnRzYCDmlbTkvZPlgY8gOCDlsI/ml7bjgILmiYDku6Xmn6Xor6LovrnnlYznlKjlopnpkp/kuLLjgIHlh7rlj4LnlKggYERBVEVfRk9STUFUYCDor7vmiJDkuLLvvIzor7vliLDnmoTlrZfnrKbkuI4gSlNPTkwvVEcg5raI5oGv6YCQ5a2X5LiA6Ie044CC6ZuG5oiQ5rWL6K+V55So5LiA5p2hKirmlYXmhI/kuI3luKYgbG9jKiog55qE6K+76L+e5o6l5oqK6L+Z5LiA54K56ZKJ5q275LqG44CCXG4yLiAqKuS/oeWPt+W9kue7hOaMieaKpeS7t+W/q+eFp+S4ieWFg+e7hCoqIGAoc2lnX2xhc3QsIHNpZ19tYXJrLCBnYXBfYnApYO+8jOS4jeaMiSB0c+OAguWQhOi0puaIt+WcqOeLrOeriyBnb3JvdXRpbmUg6YeM5Yik5a6a77yM5oiQ5Lqk5LqL5Lu26KaB562J5LiL5Y2V5b6A6L+U44CB5a6e5rWL5q+U5oum5oiq5LqL5Lu25pma5pWw56eS77yb6ICM55u46YK76Kem5Y+R6Ze06ZqU5Lit5L2N5Y+q5pyJIDYxIOenku+8jOaXtumXtOeql+ayoeacieivreS5ieOAguS4ieWFg+e7hOeUsSBgYXBwbHlTaWduYWxRdW90ZWAg5aSN5Yi257uZ5ZCM5LiA5qyh6Kem5Y+R55qE5q+P5Liq6LSm5oi377yM5piv6K+t5LmJ5Yik5o2u44CCXG4zLiAqKuWunuS+i+aYr+ehrOmalOemu+e7tOW6pioq44CCYC90aW1lbGluZWDjgIFgL2VxdWl0eS1jdXJ2ZWAg5by65Yi25Y2V5a6e5L6L77yI57y6IGBpbnN0YW5jZUtleWAg55u05o6l5oql6ZSZ77yJ77ybYC9pbnN0YW5jZS1zdW1tYXJ5YCDpgJDlrp7kvovni6znq4vmiJDooYzvvIzmnI3liqHnq6/kuI3lgZrku7vkvZXot6jlrp7kvovmsYLlkozjgIJcblxu6K6+6K6h6L+H56iL5paH5qGj77yaW2RvYy9tb2R1bGUvcjktMDIyNjYxZGU3MS9kZXNpZ24v6K6+6K6h6L+H56iLLm1kXShkb2MvbW9kdWxlL3I5LTAyMjY2MWRlNzEvZGVzaWduL+iuvuiuoei/h+eoiy5tZClcbuadg+mZkCBTUUzvvJpbZG9jL21vZHVsZS9yOS0wMjI2NjFkZTcxL+adg+mZkOi1hOa6kC5zcWxdKGRvYy9tb2R1bGUvcjktMDIyNjYxZGU3MS/mnYPpmZDotYTmupAuc3FsKVxuXG4jIyDmgI7kuYjpqoznmoRcblxuLSBgZ28gYnVpbGQgLi8uLi5gICsgYGdvIHRlc3QgLi8uLi5g77yaYHNlcnZpY2VgIC8gYG1hbmFnZXItYXBpYCAvIGBhcmd1c19zaW5nbGVgIOS4ieS4quaooeWdl+WFqOe7v++8m2Bnb2ZtdCAtbGDjgIFgZ28gdmV0YCDml6DovpPlh7rjgIJcbi0gKioxOSDmnaHljZXlhYPmtYvor5UqKu+8iOe6r+WHveaVsO+8ie+8muaXtumXtOWPo+W+hOS4jeWBj+enu+S4juWPs+eVjOihpem9kOOAgeetm+mAieijhemFjeS4juWGsueqgeajgOa1i+OAgeW8uuW6puWIhuaho+i+ueeVjOOAgeS/oeWPt+W9kue7hOWbm+enjeaDheW9ouOAgeivpuaDheS4jiBURyDmmI7nu4bov5jljp/jgIHml7bpl7TovbTliIbmobbkuI7lh4Dku5PpmLbmoq/jgIHpl6jmjqfogZrlkIjjgIHot6jlrp7kvovpmpTnprvjgIHmnYPnm4rlj5jliqjjgIHliIfniYfmipjlj6DjgIJcbi0gKio2IOadoeecn+WuniBNeVNRTCDpm4bmiJDmtYvor5UqKu+8iGBBUkdVU19FVkVOVF9URVNUX0RTTmAg6Zeo5o6n77yM6buY6K6k6Lez6L+H77yJ77ya5pys6L2u5Zyo5pys5py65LiA5qyh5oCnIHNjaGVtYSBgYXJndXNfZXZlbnRfY2hlY2tgIOS4iuWFqOmDqOi3kemAmu+8jOi3keWujOW3siBEUk9Q44CC6KaG55uW5pe25Yy65LiN5ryC56e744CB5a6e5L6L5LiN5Liy5pWw5o2u44CB5ZCM5YiG6ZKf5LiJ5qyh6Kem5Y+R5YiG5Yir5a6a5L2N5Yiw56eS44CB5YiH54mHL+aXtumXtOi9tC/mnYPnm4rjgIHogZrlkIjkuI7msYfmgLvjgIHpu5jorqTnqpflj6Pot5/pmo/mlbDmja7jgIJcbi0gKioxIOadoei3r+eUsea1i+ivlSoq77yaZ2luIGVuZ2luZSDlrp7pmYXms6jlhowgOCDmnaHot6/nlLHlubbpgJDmnaHmoLjlr7nvvIhgL3NpZ25hbHMvOmlkYCDkuI4gYC9zaWduYWxzLzppZC9zbGljZWAg5YWx5a2Y5LiN5Yay56qB77yJ44CCXG5cbiMjIOWvueeFp+S7u+WKoeivtOaYjlxuXG58IOmhuSB8IOeKtuaAgSB8XG58LS0tfC0tLXxcbnwg5L+h5Y+35YiX6KGo77yIOCDkuKrnu7TluqbnrZvpgInvvIkgfCDinIUg5a6e5L6LL+aXtumXtC/luIHnp40v6LSm5oi3K3VpZC/kuovku7bnsbvlnosv57uT5p6cL2dhcEJwIOW8uuW6pi/lj4LmlbDniYjmnKzvvIzlj6bliqDlj5jkvZPjgIHmlrnlkJHjgIHmlbDmja7mnaXmupAgfFxufCDkv6Hlj7for6bmg4XvvIjpgJDotKbmiLfliKTlrprvvIxURyBgW25dYC9gW+i3s+i/h25dYO+8iSB8IOKaoO+4jyDpg6jliIbmu6HotrPigJTigJTpl6jmjqfot7Pov4fooYznmoQgUk9JL+mYiOWAvC9yZWFzb24g5a6M5pW06L+Y5Y6f77ybKirmiJDkuqTooYznmoTjgIzlnYfku7cv5oiQ5Lqk44CN6L+Y5Y6f5LiN5LqGKirvvIzov5nkuKTkuKrku7flj5boh6rkuqTmmJPmiYDkuIvljZXlm57miafvvIxyNCDnmoTln4vngrnmnKrokL3lupPjgILov5Tlm57kvZPluKYgYGZpbGxQcmljZUF2YWlsYWJsZTpmYWxzZWAgKyDor7TmmI7vvIzliY3nq6/mmL7npLpcIuKAlFwi6ICM5LiN5pivIDAgfFxufCDnp5LnuqfliIfniYfmn6Xor6IgfCDimqDvuI8g6ZmN57qn5L6b5pWw4oCU4oCUcjPvvIjCsTFtaW4g6YCQ56eS5YiH54mH6YeH6ZuG77yJ5pyq5a6e546w77yM5LuT5bqT6YeM5rKh5pyJ5YiH54mH6KGo44CC5pS555SoIGBzdHJhdGVneV9ldmVudGAg6Ieq5bim55qE55yf5a6e56eS57qn5oql5Lu36KeC5rWL54K577yM55SoIGB0aWNrU291cmNlYC9gdGlja0NvbXBsZXRlYC9gZGVncmFkZWRSZWFzb25gIOiHquabne+8m+Wlkee6puaMiSByMyDlvaLmgIHpooTnlZnvvIzokL3lnLDlkI7liY3nq6/kuI3nlKjmlLkgfFxufCDkuI4gSyDnur/lr7npvZDnmoTml7bpl7TovbTogZrlkIggfCDinIUg5ZCr5qG25YaF57uT5p6c5YiG5biD44CB5pyA5aSnIFxcfGdhcEJwXFx844CB5YeA5oyB5LuT6Zi25qKv44CB5bey5a6e546w55uI5LqP44CBSyDnur/opobnm5bnjofvvIjnvLrlj6Plm57loavnqbrnirbmgIHvvIkgfFxufCDmnYPnm4rmm7Lnur8gfCDinIUg5oyJIGAo5a6e5L6LLCDotKbmiLcpYCDlvZLpm4bvvIxTUUwg5L6n6ZmN6YeH5qC377yMYGVxdWl0eV9rbm93bmAg5Y+j5b6E5q2j56GuIHxcbnwg5oum5oiq5Y6f5Zug6IGa5ZCIIHwg4pyFIOaMiSBgZ2F0ZV9raW5kYCArIOmYiOWAvC/lrp7mtYvlnYflgLwgKyDkv6Hlj7flvLrluqbliIbnuqcgfFxufCDot6jlrp7kvovmsYfmgLvlr7nmr5QgfCDinIUg6YCQ5a6e5L6L54us56uL77yM5pyq5rOo5YaM5a6e5L6L5qCHIGByZWdpc3RlcmVkOmZhbHNlYCB8XG58IOadg+mZkOi1hOa6kCBTUUwgfCDinIUgOCDmnaEgYXBpIOi1hOa6kO+8m+acquWBmiBwYWdlIOi1hOa6kO+8iOmhtemdouWxnuS6jiByMTIvcjEzL3IxNe+8iSB8XG58IOS4jeWBmuWJjeerryB8IOKchSB8XG5cbiMjIOa1i+ivleS6pOaOpeS/oeaBr1xuXG4tIOWFpeWPo++8mm1hbmFnZXItYXBpIOi1t+adpeWQjui1sCBgR0VUIHtyZXF1ZXN0LnBhdGh9L2FyZ3VzLWV2ZW50Lypg77yM6ZyA5YWI5omn6KGM5p2D6ZmQIFNRTCDlubbnoa7orqQgYEByb2xlX2lkYO+8iOacrOi9ruacquaLv+WIsOebruagh+inkuiJsu+8jOm7mOiupOWGmeeahCAx77yJ44CCXG4tIOeOr+Wig++8muS4gOWll+WQqyBgc3RyYXRlZ3lfZXZlbnRgIC8gYGJhbGFuY2Vfc2FtcGxlYCAvIGBkZXZfc2FtcGxlYCDnmoQgTXlTUUzjgIJyNiDlt7Llm57ngYzkuKTkuKrlrp7kvovvvIhgYXJndXMtc2luZ2xlLXJvY2DjgIFgYXJndXMtc2luZ2xlLWl2ZXNg77yMMTYuNiDkuIfmnaHvvIzopobnm5YgNi8yOOKAkzgvMjHvvInjgIJcbi0g5bu66K6u6aqM5pS26aG65bqP77yaYGZpbHRlci1vcHRpb25zYCDmi7/liLDnnJ/lrp7lrp7kvosv6LSm5oi3IOKGkiBgc2lnbmFscz9pbnN0YW5jZUtleT3igKYmc3RhcnQ9MjAyNi0wOC0xOCZlbmQ9MjAyNi0wOC0yMWDvvIjlr7kgNjEzIOasoeinpuWPke+8ieKGkiDmjJHkuIDmnaHlkIzliIbpkp/lpJrop6blj5HnmoTnnIsgYHNpZ25hbHMvOmlkYCDkuI4gYDppZC9zbGljZWAg4oaSIGB0aW1lbGluZWAgLyBgZXF1aXR5LWN1cnZlYCAvIGBnYXRlLXN0YXRzYCAvIGBpbnN0YW5jZS1zdW1tYXJ5YOOAglxuLSDpm4bmiJDmtYvor5Xoh6rot5HvvJpgQVJHVVNfRVZFTlRfVEVTVF9EU049Jzzlj6/kuKLlvIMgc2NoZW1hIOeahCBEU04+JyBnbyB0ZXN0IC4vYXJndXNfZXZlbnQvIC1ydW4gVGVzdEludGVncmF0aW9uIC12YOOAgioq5b+F6aG75oyH5ZCR5Y+v5Lii5byD55qE5bqTKirvvIznlKjkvovlj6rmk43kvZwgYGl0LWAg5YmN57yA55qE5rWL6K+V5a6e5L6L6ZSu44CCXG4tIOa4heeQhu+8muaOpeWPo+WPquivu++8jOaXoOWJr+S9nOeUqO+8m2BFbnN1cmVUYWJsZWAg55qEIEF1dG9NaWdyYXRlIOW5guetieOAglxuXG4jIyDpgZfnlZnkuI7po47pmalcblxuMS4gKirnp5LnuqfliIfniYfkvp3otZYgcjMqKu+8jOW9k+WJjeaYr+mZjee6p+W9ouaAge+8iFIx77yJ44CCXG4yLiAqKuaIkOS6pOS7t+Wfi+eCuee8uuWksSoq77yIUjLvvInvvJrlu7rorq7lnKggcjQg5L6n6KGlIGBmaWxsX3B4YC9gb3JkZXJfYXZnX3B4YO+8jOWQpuWImemcgOaxguWkp+e6siDCpzYuMSDpqozmlLYgMSDnmoRcIuWujOaVtOi/mOWOn+S4gOadoSBURyDmtojmga9cIuWPquiDveWBmuWIsFwi6Zmk6L+Z5Lik5Liq5Lu35LmL5aSW5a6M5pW0XCLjgIJcbjMuICoq5YeA5oyB5LuT6Zi25qKv5piv5rS+55Sf6YePKirvvIhSM++8ie+8jOmDqOWIhuW5s+S7k+S4jumHjeWQr+acn+mXtOeahOWPmOWMlui/veS4jeWIsO+8m+eyvuehrueUn+WRveWRqOacn+etiSByMTDjgIJcbjQuICoqYHNlcnZpY2UvdHJhZGVgIOeahOS/oeWPt+Wbnua1i+ivu+S+p+S7jeeUqCBgdGltZS5UaW1lYCoq77yIUjTvvInigJTigJTlnKggYHNxbGNvbm5gIOe8uiBgbG9jPUxvY2FsYCDnmoTpg6jnvbLkuIrmnInlkIzmoLfnmoQgOCDlsI/ml7bpmpDmgqPjgILmnKzova7msqHmnInlpLnluKbmlLnlroPvvIzlu7rorq7ljZXlvIDlsI/ku7vliqHjgIJcbjUuIOiBmuWQiOaciSAyMCDkuIfooYzkuIrpmZDvvIzlkb3kuK3ml7bov5Tlm54gYHRydW5jYXRlZDp0cnVlYO+8m+W9k+WJjeWFqOW6k+mHj+S4jeS8muinpuWPke+8jOWkmuW4geenjeaOpeWFpeWQjumcgOWkjeaguOOAglxuXG7or7vkuoblk6rkupvmlofmoaPvvJrpnIDmsYLlpKfnurLvvIjlhajmlofvvIzpqozmlLblj6PlvoTku6XlroPkuLrlh4bvvInjgIFgcHJvdG90eXBlL2Ag55qEIHNpZ25hbHMvbWFya2V0L2luZGV4IOS4iemhte+8iOacrOaOpeWPo+eahOebtOaOpea2iOi0ueaWue+8jOWtl+auteaMieWug+S7rOWvuem9kO+8ieOAgXI2IOeahOS4gOiHtOaAp+W3oeajgOaKpeWRiu+8iOehruiupOW6k+WGheecn+WunuaVsOaNruW9ouaAge+8ie+8m3I4IOiuvuiuoeaWh+aho+WPquafpeS6huWug+WvuSByMyDnjrDnirbnmoTliKTmlq3jgIIqKuayoeivuyByMSDnmoTpnIDmsYLmlofmoaMqKuKAlOKAlOWug+iusuWunuS+i+azqOWGjOS4jumFjee9ruWIhuWfn++8jOaIkeebtOaOpeivu+S6huiQveWcsOWQjueahCBgc2VydmljZS9hcmd1c19jb25maWcvaW5zdGFuY2UuZ29gIOS4jiBgcmVwb3NpdG9yeS9tb2RlbC5nb2DvvIjku6PnoIHmr5TmlofmoaPmm7TmjqXov5Hkuovlrp7vvInvvIzlhbbkvZnliIbln5/lhoXlrrnkuI7lj6ror7vmn6Xor6Lml6DlhbPjgIJcblxu5om56YeP5Yik5a6a77ya5a6M5oiQIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiJmaW5hbF9hbnN3ZXIifV19XX0=
-->
