---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r4-a9ea8f6bc2"
requirementKey: "req-1788169552180"
resourceName: "argus_single 策略事件双写 MySQL（新增 pkg/eventstore，带实例维度）"
conversationTitle: "argus_single 策略事件双写 MySQL（新增 pkg/eventstore，带实例维度）"
threadId: "65191639-60d7-4f8d-95b4-7d004b7a0bcf"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-01T15:33:11.770312+00:00"
---

# 任务聊天 · argus_single 策略事件双写 MySQL（新增 pkg/eventstore，带实例维度）

## 第 1 轮 · completed · EUr3fKeZb-drXEFlAH5z3A

### 用户

执行「动作执行」阶段：argus_single 策略事件双写 MySQL（新增 pkg/eventstore，带实例维度）

### 助手

I'll start by loading the delivery-action-execution skill as instructed.

### 助手

Now let me read the prerequisite task doc and the referenced design doc, plus survey the code.

### 助手

Now I'll implement. Starting with the leaf-safe sink hook in `pkg/eventlog`:

### 助手

Now wire the sink into `Logger.Log` without changing its JSONL behavior:

### 助手

Now the pure functions — hash, normalization, gate structuring:

### 助手

Now the conversion layer (pure) from `eventlog.Event` to table rows:

### 助手

Now the tests:

### 助手

The function is correct — my test's own DSN split was wrong (it grabbed the `?` inside the password). Fixing the test:

### 助手

Now gate + convert + SQL/sink tests:

### 助手

A local MySQL 8.0.28 is reachable — I'll add an env-gated integration test and actually run it against a throwaway schema:

### 助手

The integration test caught a real bug: `equity=0` with `equity_known=1` was being stored as NULL — exactly the case the design doc says must not be lost. Fixing:

### 助手

Cleaning up the throwaway verification schema and running a final full verification:

### 助手

交付完成。

## 1. 改了什么

**读的文档**：需求大纲（需求级，全篇）；`doc/module/r1-ab25892542/文档.md`（前置依赖 — `instance_key`/`config_version` 两个维度全部来自它）；`docs/argus_single/2026-07-28-事件数据持久化与可视化设计.md`（任务说明指定的 §4 分表依据）。r2/r3/r5 及 r6 之后未读：前者与事件表无字段耦合，后者是本任务的下游。

**新增 `server/argus_single/pkg/eventstore/`（7 个实现文件 + 4 个测试文件）**
- `model.go` — 三张事实表模型 + `Models()`；`SourceLive/SourceBackfill`；`JSONLDualWriteUntil = "2026-12-01"`
- `hash.go` — `EventHash = md5(instanceKey + "\n" + eventlog.Marshal(e))`，直写与回灌字节级同源
- `normalize.go` — `NormalizeInstrument`（BTC-USDT-SWAP → BTCUSDT，保留原值）、`ParseTs`（秒精度、本地时区）、`NormalizeDSN`（强制 `loc=Local`/`parseTime`/`utf8mb4`/三个超时）
- `gate.go` — `ParseGate`：门控 reason → `gate_kind` + `threshold` + `actual`，7 个 kind 常量 + 文案漂移兜底
- `convert.go` — `Envelope` → 三表行，纯函数；`equity_known`/`net_size_known` 已知性口径
- `store.go` — 专属 DB 连接、`EnsureTable`、非阻塞 `Emit`、批量 `INSERT ... ON DUPLICATE KEY UPDATE id=id`、`Stats`、`Close`
- `runtime.go` — 包级 `Setup`/`SetConfigVersion`/`SetAccounts`/`Shutdown`

**新增 `pkg/eventlog/sink.go`** — leaf 包里只加 `Sink` 接口 + 注册函数，零新依赖，sink panic 被 recover。

**改 `pkg/eventlog/eventlog.go`** — `Logger.Log` 抽出 `writeLine`，落盘后旁路投递。JSONL 行为一字未变，**12 个埋点一行未改**。

**改 `initialization/initialization.go`** — 在监控启动前 `eventstore.Setup`（失败只告警、退化成只写 JSONL）；reload observer 改为同时通知 heartbeat 与 eventstore；新增 `accountIdentities`。

**改 `routers/routers.go`** — SIGTERM 优雅关闭里加 `eventstore.Shutdown(3s)`。

**`go.mod/go.sum`** — `gorm.io/driver/mysql` 从 indirect 转直接依赖。

设计过程文档：[doc/module/r4-a9ea8f6bc2/design/设计过程.md](doc/module/r4-a9ea8f6bc2/design/设计过程.md)

## 2. 怎么验的

| 验证 | 结果 |
|---|---|
| `go build ./...` × 4 模块（argus_single/common/service/manager-api） | 全绿 |
| `go vet ./...` × 4 模块 | 全绿 |
| `go test ./...` × 4 模块 | 全绿 |
| `gofmt -l` 新增/改动文件 | 无输出（仓库其余历史文件的既有格式偏差未动） |
| `pkg/eventstore` 单测 | 37 PASS + 3 SKIP（集成用例无 DSN 时跳过） |
| 真实 MySQL 8.0.28 集成 | 3 个用例全绿，用一次性 schema `eventstore_check`，验完已 DROP |

集成校验**发现并修掉了两个真问题**：
1. `equity=0` + `equity_known=1` 被"零值 ⇒ NULL"口径吃成 NULL，写出自相矛盾的行 —— 而零/负权益恰是最极端的回撤样本。已改为已知时原值入库，单测断言补强到查值本身。
2. gorm 把 Go `int` 一律映射成 `bigint`（`type:int` 不生效），已用 `size:32` 修正回 `int`。

集成用例实测确认：`ts` 是 `datetime`、`event_hash` 是 `binary(16)`；读回 `ts` 与 JSONL 串**逐字一致**；同一秒两次触发落两行；同批重放被幂等吃掉；`gate_kind/threshold/actual` 正确落库；同 `account_label` 跨两实例各占一行。

## 3. 对照任务说明逐条

| 要求 | 状态 |
|---|---|
| 新增 `pkg/eventstore`，不污染 leaf 包 | ✅ leaf 包只多 `Sink` 接口，零新依赖 |
| 按设计文档 §4 分表 | ✅ 三张表（`strategy_event` / `balance_sample` / `dev_sample`）。dev_sample 单独成表是对 30% 占比的处理，理由见设计文档 §3.3 |
| 每条带 `instance_key` 与 `config_version` | ✅ 三张表都有，Emit 时刻钉住 |
| 账户唯一性用 `(instance_key, account)` 或 uid | ✅ 两列都存，instance_key 进哈希，跨实例同标签已实测各占一行 |
| Go 侧算 `event_hash` | ✅ |
| 保留 `equity_known` | ✅ 含已知零/负权益（集成校验修复点） |
| DATETIME 不用 TIMESTAMP、DSN 显式设 loc | ✅ 已在真库核对列类型与读回值 |
| ts 精度到秒 | ✅ 同秒两次触发落两行 |
| 门控 reason 结构化 | ✅ 4 类形态 + 兜底 kind |
| JSONL 双写保留至 2026-12-01 | ✅ 常量 + 启动日志 |
| 不改策略判定逻辑 | ✅ `pkg/trade`/`pkg/monitor` 一行未改 |
| 不引入 ClickHouse 等新存储 | ✅ |

## 4. 测试交接信息

- **入口**：进程启动即生效，无接口。集成用例：`EVENTSTORE_TEST_DSN='<user>:<pass>@tcp(<host>:3306)/<可丢弃schema>' go test ./pkg/eventstore/ -run TestIntegration -v`（在 `server/argus_single` 下执行）。
- **环境**：可写 MySQL 8.0（表由 `EnsureTable` 自动 AutoMigrate 建出）、Redis；properties 需配 `sqlconn` 与 `argus.instance.id`。凭证来源为部署 properties，请勿指向生产库跑集成用例。
- **执行顺序**：启动进程 → 等一次 balance 心跳（约 1 分钟）→ 查 `balance_sample`；触发一次信号或门控 → 查 `strategy_event` 的 `gate_kind`。
- **可观察结果**：三张表按 `instance_key` 过滤；`ext` 列预留为空；写入侧计数在 `Store.Stats()`（`Dropped`/`Failed` 非零即说明有事件只留在 JSONL）。
- **清理**：`DELETE FROM strategy_event/balance_sample/dev_sample WHERE instance_key = '<实例键>'`；集成 schema 直接 DROP。

## 5. 遗留与风险

- `episode` 表**未建**（r10 范围，设计文档明确它是可重建的派生表）；历史 JSONL **未回灌**（r6 范围，`source=2` 与 `SourceBackfill` 已预留）。
- `fixed_close`/`manual_close` 历史 30 天从未触发过，无金标准数据，只有构造数据的单测覆盖。
- 三实例并发写、配置热更新后 `config_version` 递增，本轮无环境验证，需测试阶段用真实多实例覆盖。
- 表名按需求大纲/设计文档原名（无 `argus_` 前缀）；若后续要统一前缀需同步改 r6/r9/r10 三份文档。
- **工作区未提交**：`chat/requirements/req-1788169552180/task/` 下有两份其他任务的对话记录处于未跟踪状态，该目录历史上出现过真实 DSN/密码。我没有执行任何 `git add`/`commit`/`push`，避免误扫进去；提交前请单独核对这两个文件。

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJFVXIzZktlWmItZHJYRUZsQUg1ejNBIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMVQxNTowNDoxOS43NTY3ODQrMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDFUMTU6MzM6MTEuMjA4NjE3KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8mmFyZ3VzX3NpbmdsZSDnrZbnlaXkuovku7blj4zlhpkgTXlTUUzvvIjmlrDlop4gcGtnL2V2ZW50c3RvcmXvvIzluKblrp7kvovnu7TluqbvvIkifV19LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIHN0YXJ0IGJ5IGxvYWRpbmcgdGhlIGRlbGl2ZXJ5LWFjdGlvbi1leGVjdXRpb24gc2tpbGwgYXMgaW5zdHJ1Y3RlZC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgbGV0IG1lIHJlYWQgdGhlIHByZXJlcXVpc2l0ZSB0YXNrIGRvYyBhbmQgdGhlIHJlZmVyZW5jZWQgZGVzaWduIGRvYywgcGx1cyBzdXJ2ZXkgdGhlIGNvZGUuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IEknbGwgaW1wbGVtZW50LiBTdGFydGluZyB3aXRoIHRoZSBsZWFmLXNhZmUgc2luayBob29rIGluIGBwa2cvZXZlbnRsb2dgOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB3aXJlIHRoZSBzaW5rIGludG8gYExvZ2dlci5Mb2dgIHdpdGhvdXQgY2hhbmdpbmcgaXRzIEpTT05MIGJlaGF2aW9yOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgcHVyZSBmdW5jdGlvbnMg4oCUIGhhc2gsIG5vcm1hbGl6YXRpb24sIGdhdGUgc3RydWN0dXJpbmc6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBjb252ZXJzaW9uIGxheWVyIChwdXJlKSBmcm9tIGBldmVudGxvZy5FdmVudGAgdG8gdGFibGUgcm93czoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIHRlc3RzOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IlRoZSBmdW5jdGlvbiBpcyBjb3JyZWN0IOKAlCBteSB0ZXN0J3Mgb3duIERTTiBzcGxpdCB3YXMgd3JvbmcgKGl0IGdyYWJiZWQgdGhlIGA/YCBpbnNpZGUgdGhlIHBhc3N3b3JkKS4gRml4aW5nIHRoZSB0ZXN0OiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBnYXRlICsgY29udmVydCArIFNRTC9zaW5rIHRlc3RzOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IkEgbG9jYWwgTXlTUUwgOC4wLjI4IGlzIHJlYWNoYWJsZSDigJQgSSdsbCBhZGQgYW4gZW52LWdhdGVkIGludGVncmF0aW9uIHRlc3QgYW5kIGFjdHVhbGx5IHJ1biBpdCBhZ2FpbnN0IGEgdGhyb3dhd2F5IHNjaGVtYToiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJUaGUgaW50ZWdyYXRpb24gdGVzdCBjYXVnaHQgYSByZWFsIGJ1ZzogYGVxdWl0eT0wYCB3aXRoIGBlcXVpdHlfa25vd249MWAgd2FzIGJlaW5nIHN0b3JlZCBhcyBOVUxMIOKAlCBleGFjdGx5IHRoZSBjYXNlIHRoZSBkZXNpZ24gZG9jIHNheXMgbXVzdCBub3QgYmUgbG9zdC4gRml4aW5nOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IkNsZWFuaW5nIHVwIHRoZSB0aHJvd2F3YXkgdmVyaWZpY2F0aW9uIHNjaGVtYSBhbmQgcnVubmluZyBhIGZpbmFsIGZ1bGwgdmVyaWZpY2F0aW9uOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IuS6pOS7mOWujOaIkOOAglxuXG4jIyAxLiDmlLnkuobku4DkuYhcblxuKiror7vnmoTmlofmoaMqKu+8mumcgOaxguWkp+e6su+8iOmcgOaxgue6p++8jOWFqOevh++8ie+8m2Bkb2MvbW9kdWxlL3IxLWFiMjU4OTI1NDIv5paH5qGjLm1kYO+8iOWJjee9ruS+nei1liDigJQgYGluc3RhbmNlX2tleWAvYGNvbmZpZ192ZXJzaW9uYCDkuKTkuKrnu7Tluqblhajpg6jmnaXoh6rlroPvvInvvJtgZG9jcy9hcmd1c19zaW5nbGUvMjAyNi0wNy0yOC3kuovku7bmlbDmja7mjIHkuYXljJbkuI7lj6/op4bljJborr7orqEubWRg77yI5Lu75Yqh6K+05piO5oyH5a6a55qEIMKnNCDliIbooajkvp3mja7vvInjgIJyMi9yMy9yNSDlj4ogcjYg5LmL5ZCO5pyq6K+777ya5YmN6ICF5LiO5LqL5Lu26KGo5peg5a2X5q616ICm5ZCI77yM5ZCO6ICF5piv5pys5Lu75Yqh55qE5LiL5ri444CCXG5cbioq5paw5aKeIGBzZXJ2ZXIvYXJndXNfc2luZ2xlL3BrZy9ldmVudHN0b3JlL2DvvIg3IOS4quWunueOsOaWh+S7tiArIDQg5Liq5rWL6K+V5paH5Lu277yJKipcbi0gYG1vZGVsLmdvYCDigJQg5LiJ5byg5LqL5a6e6KGo5qih5Z6LICsgYE1vZGVscygpYO+8m2BTb3VyY2VMaXZlL1NvdXJjZUJhY2tmaWxsYO+8m2BKU09OTER1YWxXcml0ZVVudGlsID0gXCIyMDI2LTEyLTAxXCJgXG4tIGBoYXNoLmdvYCDigJQgYEV2ZW50SGFzaCA9IG1kNShpbnN0YW5jZUtleSArIFwiXFxuXCIgKyBldmVudGxvZy5NYXJzaGFsKGUpKWDvvIznm7TlhpnkuI7lm57ngYzlrZfoioLnuqflkIzmupBcbi0gYG5vcm1hbGl6ZS5nb2Ag4oCUIGBOb3JtYWxpemVJbnN0cnVtZW50YO+8iEJUQy1VU0RULVNXQVAg4oaSIEJUQ1VTRFTvvIzkv53nlZnljp/lgLzvvInjgIFgUGFyc2VUc2DvvIjnp5Lnsr7luqbjgIHmnKzlnLDml7bljLrvvInjgIFgTm9ybWFsaXplRFNOYO+8iOW8uuWItiBgbG9jPUxvY2FsYC9gcGFyc2VUaW1lYC9gdXRmOG1iNGAv5LiJ5Liq6LaF5pe277yJXG4tIGBnYXRlLmdvYCDigJQgYFBhcnNlR2F0ZWDvvJrpl6jmjqcgcmVhc29uIOKGkiBgZ2F0ZV9raW5kYCArIGB0aHJlc2hvbGRgICsgYGFjdHVhbGDvvIw3IOS4qiBraW5kIOW4uOmHjyArIOaWh+ahiOa8guenu+WFnOW6lVxuLSBgY29udmVydC5nb2Ag4oCUIGBFbnZlbG9wZWAg4oaSIOS4ieihqOihjO+8jOe6r+WHveaVsO+8m2BlcXVpdHlfa25vd25gL2BuZXRfc2l6ZV9rbm93bmAg5bey55+l5oCn5Y+j5b6EXG4tIGBzdG9yZS5nb2Ag4oCUIOS4k+WxniBEQiDov57mjqXjgIFgRW5zdXJlVGFibGVg44CB6Z2e6Zi75aGeIGBFbWl0YOOAgeaJuemHjyBgSU5TRVJUIC4uLiBPTiBEVVBMSUNBVEUgS0VZIFVQREFURSBpZD1pZGDjgIFgU3RhdHNg44CBYENsb3NlYFxuLSBgcnVudGltZS5nb2Ag4oCUIOWMhee6pyBgU2V0dXBgL2BTZXRDb25maWdWZXJzaW9uYC9gU2V0QWNjb3VudHNgL2BTaHV0ZG93bmBcblxuKirmlrDlop4gYHBrZy9ldmVudGxvZy9zaW5rLmdvYCoqIOKAlCBsZWFmIOWMhemHjOWPquWKoCBgU2lua2Ag5o6l5Y+jICsg5rOo5YaM5Ye95pWw77yM6Zu25paw5L6d6LWW77yMc2luayBwYW5pYyDooqsgcmVjb3ZlcuOAglxuXG4qKuaUuSBgcGtnL2V2ZW50bG9nL2V2ZW50bG9nLmdvYCoqIOKAlCBgTG9nZ2VyLkxvZ2Ag5oq95Ye6IGB3cml0ZUxpbmVg77yM6JC955uY5ZCO5peB6Lev5oqV6YCS44CCSlNPTkwg6KGM5Li65LiA5a2X5pyq5Y+Y77yMKioxMiDkuKrln4vngrnkuIDooYzmnKrmlLkqKuOAglxuXG4qKuaUuSBgaW5pdGlhbGl6YXRpb24vaW5pdGlhbGl6YXRpb24uZ29gKiog4oCUIOWcqOebkeaOp+WQr+WKqOWJjSBgZXZlbnRzdG9yZS5TZXR1cGDvvIjlpLHotKXlj6rlkYrorabjgIHpgIDljJbmiJDlj6rlhpkgSlNPTkzvvInvvJtyZWxvYWQgb2JzZXJ2ZXIg5pS55Li65ZCM5pe26YCa55+lIGhlYXJ0YmVhdCDkuI4gZXZlbnRzdG9yZe+8m+aWsOWiniBgYWNjb3VudElkZW50aXRpZXNg44CCXG5cbioq5pS5IGByb3V0ZXJzL3JvdXRlcnMuZ29gKiog4oCUIFNJR1RFUk0g5LyY6ZuF5YWz6Zet6YeM5YqgIGBldmVudHN0b3JlLlNodXRkb3duKDNzKWDjgIJcblxuKipgZ28ubW9kL2dvLnN1bWAqKiDigJQgYGdvcm0uaW8vZHJpdmVyL215c3FsYCDku44gaW5kaXJlY3Qg6L2s55u05o6l5L6d6LWW44CCXG5cbuiuvuiuoei/h+eoi+aWh+aho++8mltkb2MvbW9kdWxlL3I0LWE5ZWE4ZjZiYzIvZGVzaWduL+iuvuiuoei/h+eoiy5tZF0oZG9jL21vZHVsZS9yNC1hOWVhOGY2YmMyL2Rlc2lnbi/orr7orqHov4fnqIsubWQpXG5cbiMjIDIuIOaAjuS5iOmqjOeahFxuXG58IOmqjOivgSB8IOe7k+aenCB8XG58LS0tfC0tLXxcbnwgYGdvIGJ1aWxkIC4vLi4uYCDDlyA0IOaooeWdl++8iGFyZ3VzX3NpbmdsZS9jb21tb24vc2VydmljZS9tYW5hZ2VyLWFwae+8iSB8IOWFqOe7vyB8XG58IGBnbyB2ZXQgLi8uLi5gIMOXIDQg5qih5Z2XIHwg5YWo57u/IHxcbnwgYGdvIHRlc3QgLi8uLi5gIMOXIDQg5qih5Z2XIHwg5YWo57u/IHxcbnwgYGdvZm10IC1sYCDmlrDlop4v5pS55Yqo5paH5Lu2IHwg5peg6L6T5Ye677yI5LuT5bqT5YW25L2Z5Y6G5Y+y5paH5Lu255qE5pei5pyJ5qC85byP5YGP5beu5pyq5Yqo77yJIHxcbnwgYHBrZy9ldmVudHN0b3JlYCDljZXmtYsgfCAzNyBQQVNTICsgMyBTS0lQ77yI6ZuG5oiQ55So5L6L5pegIERTTiDml7bot7Pov4fvvIkgfFxufCDnnJ/lrp4gTXlTUUwgOC4wLjI4IOmbhuaIkCB8IDMg5Liq55So5L6L5YWo57u/77yM55So5LiA5qyh5oCnIHNjaGVtYSBgZXZlbnRzdG9yZV9jaGVja2DvvIzpqozlrozlt7IgRFJPUCB8XG5cbumbhuaIkOagoemqjCoq5Y+R546w5bm25L+u5o6J5LqG5Lik5Liq55yf6Zeu6aKYKirvvJpcbjEuIGBlcXVpdHk9MGAgKyBgZXF1aXR5X2tub3duPTFgIOiiq1wi6Zu25YC8IOKHkiBOVUxMXCLlj6PlvoTlkIPmiJAgTlVMTO+8jOWGmeWHuuiHquebuOefm+ebvueahOihjCDigJTigJQg6ICM6Zu2L+i0n+adg+ebiuaBsOaYr+acgOaegeerr+eahOWbnuaSpOagt+acrOOAguW3suaUueS4uuW3suefpeaXtuWOn+WAvOWFpeW6k++8jOWNlea1i+aWreiogOihpeW8uuWIsOafpeWAvOacrOi6q+OAglxuMi4gZ29ybSDmioogR28gYGludGAg5LiA5b6L5pig5bCE5oiQIGBiaWdpbnRg77yIYHR5cGU6aW50YCDkuI3nlJ/mlYjvvInvvIzlt7LnlKggYHNpemU6MzJgIOS/ruato+WbniBgaW50YOOAglxuXG7pm4bmiJDnlKjkvovlrp7mtYvnoa7orqTvvJpgdHNgIOaYryBgZGF0ZXRpbWVg44CBYGV2ZW50X2hhc2hgIOaYryBgYmluYXJ5KDE2KWDvvJvor7vlm54gYHRzYCDkuI4gSlNPTkwg5LiyKirpgJDlrZfkuIDoh7QqKu+8m+WQjOS4gOenkuS4pOasoeinpuWPkeiQveS4pOihjO+8m+WQjOaJuemHjeaUvuiiq+W5guetieWQg+aOie+8m2BnYXRlX2tpbmQvdGhyZXNob2xkL2FjdHVhbGAg5q2j56Gu6JC95bqT77yb5ZCMIGBhY2NvdW50X2xhYmVsYCDot6jkuKTlrp7kvovlkITljaDkuIDooYzjgIJcblxuIyMgMy4g5a+554Wn5Lu75Yqh6K+05piO6YCQ5p2hXG5cbnwg6KaB5rGCIHwg54q25oCBIHxcbnwtLS18LS0tfFxufCDmlrDlop4gYHBrZy9ldmVudHN0b3JlYO+8jOS4jeaxoeafkyBsZWFmIOWMhSB8IOKchSBsZWFmIOWMheWPquWkmiBgU2lua2Ag5o6l5Y+j77yM6Zu25paw5L6d6LWWIHxcbnwg5oyJ6K6+6K6h5paH5qGjIMKnNCDliIbooaggfCDinIUg5LiJ5byg6KGo77yIYHN0cmF0ZWd5X2V2ZW50YCAvIGBiYWxhbmNlX3NhbXBsZWAgLyBgZGV2X3NhbXBsZWDvvInjgIJkZXZfc2FtcGxlIOWNleeLrOaIkOihqOaYr+WvuSAzMCUg5Y2g5q+U55qE5aSE55CG77yM55CG55Sx6KeB6K6+6K6h5paH5qGjIMKnMy4zIHxcbnwg5q+P5p2h5bimIGBpbnN0YW5jZV9rZXlgIOS4jiBgY29uZmlnX3ZlcnNpb25gIHwg4pyFIOS4ieW8oOihqOmDveacie+8jEVtaXQg5pe25Yi76ZKJ5L2PIHxcbnwg6LSm5oi35ZSv5LiA5oCn55SoIGAoaW5zdGFuY2Vfa2V5LCBhY2NvdW50KWAg5oiWIHVpZCB8IOKchSDkuKTliJfpg73lrZjvvIxpbnN0YW5jZV9rZXkg6L+b5ZOI5biM77yM6Leo5a6e5L6L5ZCM5qCH562+5bey5a6e5rWL5ZCE5Y2g5LiA6KGMIHxcbnwgR28g5L6n566XIGBldmVudF9oYXNoYCB8IOKchSB8XG58IOS/neeVmSBgZXF1aXR5X2tub3duYCB8IOKchSDlkKvlt7Lnn6Xpm7Yv6LSf5p2D55uK77yI6ZuG5oiQ5qCh6aqM5L+u5aSN54K577yJIHxcbnwgREFURVRJTUUg5LiN55SoIFRJTUVTVEFNUOOAgURTTiDmmL7lvI/orr4gbG9jIHwg4pyFIOW3suWcqOecn+W6k+aguOWvueWIl+exu+Wei+S4juivu+WbnuWAvCB8XG58IHRzIOeyvuW6puWIsOenkiB8IOKchSDlkIznp5LkuKTmrKHop6blj5HokL3kuKTooYwgfFxufCDpl6jmjqcgcmVhc29uIOe7k+aehOWMliB8IOKchSA0IOexu+W9ouaAgSArIOWFnOW6lSBraW5kIHxcbnwgSlNPTkwg5Y+M5YaZ5L+d55WZ6IezIDIwMjYtMTItMDEgfCDinIUg5bi46YePICsg5ZCv5Yqo5pel5b+XIHxcbnwg5LiN5pS5562W55Wl5Yik5a6a6YC76L6RIHwg4pyFIGBwa2cvdHJhZGVgL2Bwa2cvbW9uaXRvcmAg5LiA6KGM5pyq5pS5IHxcbnwg5LiN5byV5YWlIENsaWNrSG91c2Ug562J5paw5a2Y5YKoIHwg4pyFIHxcblxuIyMgNC4g5rWL6K+V5Lqk5o6l5L+h5oGvXG5cbi0gKirlhaXlj6MqKu+8mui/m+eoi+WQr+WKqOWNs+eUn+aViO+8jOaXoOaOpeWPo+OAgumbhuaIkOeUqOS+i++8mmBFVkVOVFNUT1JFX1RFU1RfRFNOPSc8dXNlcj46PHBhc3M+QHRjcCg8aG9zdD46MzMwNikvPOWPr+S4ouW8g3NjaGVtYT4nIGdvIHRlc3QgLi9wa2cvZXZlbnRzdG9yZS8gLXJ1biBUZXN0SW50ZWdyYXRpb24gLXZg77yI5ZyoIGBzZXJ2ZXIvYXJndXNfc2luZ2xlYCDkuIvmiafooYzvvInjgIJcbi0gKirnjq/looMqKu+8muWPr+WGmSBNeVNRTCA4LjDvvIjooajnlLEgYEVuc3VyZVRhYmxlYCDoh6rliqggQXV0b01pZ3JhdGUg5bu65Ye677yJ44CBUmVkaXPvvJtwcm9wZXJ0aWVzIOmcgOmFjSBgc3FsY29ubmAg5LiOIGBhcmd1cy5pbnN0YW5jZS5pZGDjgILlh63or4HmnaXmupDkuLrpg6jnvbIgcHJvcGVydGllc++8jOivt+WLv+aMh+WQkeeUn+S6p+W6k+i3kembhuaIkOeUqOS+i+OAglxuLSAqKuaJp+ihjOmhuuW6jyoq77ya5ZCv5Yqo6L+b56iLIOKGkiDnrYnkuIDmrKEgYmFsYW5jZSDlv4Pot7PvvIjnuqYgMSDliIbpkp/vvInihpIg5p+lIGBiYWxhbmNlX3NhbXBsZWDvvJvop6blj5HkuIDmrKHkv6Hlj7fmiJbpl6jmjqcg4oaSIOafpSBgc3RyYXRlZ3lfZXZlbnRgIOeahCBgZ2F0ZV9raW5kYOOAglxuLSAqKuWPr+inguWvn+e7k+aenCoq77ya5LiJ5byg6KGo5oyJIGBpbnN0YW5jZV9rZXlgIOi/h+a7pO+8m2BleHRgIOWIl+mihOeVmeS4uuepuu+8m+WGmeWFpeS+p+iuoeaVsOWcqCBgU3RvcmUuU3RhdHMoKWDvvIhgRHJvcHBlZGAvYEZhaWxlZGAg6Z2e6Zu25Y2z6K+05piO5pyJ5LqL5Lu25Y+q55WZ5ZyoIEpTT05M77yJ44CCXG4tICoq5riF55CGKirvvJpgREVMRVRFIEZST00gc3RyYXRlZ3lfZXZlbnQvYmFsYW5jZV9zYW1wbGUvZGV2X3NhbXBsZSBXSEVSRSBpbnN0YW5jZV9rZXkgPSAnPOWunuS+i+mUrj4nYO+8m+mbhuaIkCBzY2hlbWEg55u05o6lIERST1DjgIJcblxuIyMgNS4g6YGX55WZ5LiO6aOO6ZmpXG5cbi0gYGVwaXNvZGVgIOihqCoq5pyq5bu6KirvvIhyMTAg6IyD5Zu077yM6K6+6K6h5paH5qGj5piO56Gu5a6D5piv5Y+v6YeN5bu655qE5rS+55Sf6KGo77yJ77yb5Y6G5Y+yIEpTT05MICoq5pyq5Zue54GMKirvvIhyNiDojIPlm7TvvIxgc291cmNlPTJgIOS4jiBgU291cmNlQmFja2ZpbGxgIOW3sumihOeVme+8ieOAglxuLSBgZml4ZWRfY2xvc2VgL2BtYW51YWxfY2xvc2VgIOWOhuWPsiAzMCDlpKnku47mnKrop6blj5Hov4fvvIzml6Dph5HmoIflh4bmlbDmja7vvIzlj6rmnInmnoTpgKDmlbDmja7nmoTljZXmtYvopobnm5bjgIJcbi0g5LiJ5a6e5L6L5bm25Y+R5YaZ44CB6YWN572u54Ot5pu05paw5ZCOIGBjb25maWdfdmVyc2lvbmAg6YCS5aKe77yM5pys6L2u5peg546v5aKD6aqM6K+B77yM6ZyA5rWL6K+V6Zi25q6155So55yf5a6e5aSa5a6e5L6L6KaG55uW44CCXG4tIOihqOWQjeaMiemcgOaxguWkp+e6si/orr7orqHmlofmoaPljp/lkI3vvIjml6AgYGFyZ3VzX2Ag5YmN57yA77yJ77yb6Iul5ZCO57ut6KaB57uf5LiA5YmN57yA6ZyA5ZCM5q2l5pS5IHI2L3I5L3IxMCDkuInku73mlofmoaPjgIJcbi0gKirlt6XkvZzljLrmnKrmj5DkuqQqKu+8mmBjaGF0L3JlcXVpcmVtZW50cy9yZXEtMTc4ODE2OTU1MjE4MC90YXNrL2Ag5LiL5pyJ5Lik5Lu95YW25LuW5Lu75Yqh55qE5a+56K+d6K6w5b2V5aSE5LqO5pyq6Lef6Liq54q25oCB77yM6K+l55uu5b2V5Y6G5Y+y5LiK5Ye6546w6L+H55yf5a6eIERTTi/lr4bnoIHjgILmiJHmsqHmnInmiafooYzku7vkvZUgYGdpdCBhZGRgL2Bjb21taXRgL2BwdXNoYO+8jOmBv+WFjeivr+aJq+i/m+WOu++8m+aPkOS6pOWJjeivt+WNleeLrOaguOWvuei/meS4pOS4quaWh+S7tuOAglxuXG7mibnph4/liKTlrprvvJrlrozmiJAiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6ImZpbmFsX2Fuc3dlciJ9XX1dfQ==
-->
