---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r6-cf8beef011"
requirementKey: "req-1788169552180"
resourceName: "logs/argus_single 历史 JSONL 回灌与双写一致性校验"
conversationTitle: "logs/argus_single 历史 JSONL 回灌与双写一致性校验"
threadId: "88fcde35-0655-4e10-9169-08addb22769a"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-01T16:01:06.777853+00:00"
---

# 任务聊天 · logs/argus_single 历史 JSONL 回灌与双写一致性校验

## 第 1 轮 · completed · _923_Z0xe457WOoQQNAQtA

### 用户

执行「动作执行」阶段：logs/argus_single 历史 JSONL 回灌与双写一致性校验

### 助手

I'll start by loading the delivery-action-execution skill as instructed.

### 助手

Now I'll implement. First, extract a reusable connection opener in `eventstore`:

### 助手

Now the backfill package — JSONL scanning with honest line-level accounting:

### 助手

Now the daily consistency report renderer:

### 助手

Now the account-identity loader in the config domain:

### 助手

Now the CLI:

### 助手

Let me add a completeness check for days present in the DB but absent from JSONL:

### 助手

Now regenerate the deliverable reports from a clean schema:

### 助手

Now the runbook:

### 助手

Now regenerate the deliverable reports on a clean schema and confirm the integration suite no longer touches real instance keys:

### 助手

## 改了什么

**新增 `server/argus_single/pkg/eventstore/backfill/`**（r4 的 eventstore 子包，实盘二进制不 import 就不链进去）

| 文件 | 意图 |
|---|---|
| `scan.go` | 只认 `events-<date>.jsonl`（目录里躺着 ChatExport / html / txt）；逐行解析并统计坏行、空行、跨日——`eventlog.ParseFile` 静默跳坏行，报告不能把缺口藏起来 |
| `matrix.go` | 设计文档 §6.6 字段适用性矩阵，回灌时兼作校验器，只报数不拒绝导入 |
| `build.go` | 多目录扫描 → uid 映射 → 按天分组 → 复用 `eventstore.Convert` → `event_hash` 去重 → `net_size_known` 按「天×账户」自适应 → 覆盖率统计 |
| `import.go` | `EnsureTables` / `WriteDay`（`ON DUPLICATE KEY UPDATE id=id`） |
| `audit.go` | `AuditDay`（按 `event_hash` 集合差）、`FindOrphanDays`（库里有整天、JSONL 整天没有） |
| `report.go` | 六节 Markdown 巡检报告，行序全排序保证两天可 diff |
| 4 个测试文件 | 15 个单测 + 5 个真库集成用例 |

**改动**
- `pkg/eventstore/store.go`：抽出导出的 `OpenDB(dsn)`，回灌与直写共用同一套 DSN 口径（`loc=Local` 必须由代码强制）。`Open` 行为一字未改。
- `service/argus_config/importer.go`：+53 行，新增 `LoadInstanceAccounts()`，只读实例键与 `trade.accountN.name/uid/variant`，**不碰 session.json、不解析任何凭证**。
- `pkg/eventstore/store_integration_test.go`：r4 的集成用例用真实实例键 `argus-single-roc` 并 `DELETE WHERE instance_key='argus-single-roc'`。本轮实跑被真实触发——回灌完 166,662 行后跑一遍 r4 测试，roc 的 strategy_event 从 7348 掉到 5。改成 `it-eventstore-main/alt` 两个测试专用键，断言逻辑未动。

**新增 CLI** `server/manager-api/cmd/argus-event-import/main.go`（照 `argus-config-import` 先例）：默认 dry run、`--apply` 回灌、`--audit` 只巡检；`--source` 可重复（实例归属按目录，由人显式声明）；缺失行或缺整天 → 退出码 2，cron 可直接告警。

**文档**：[设计过程](doc/module/r6-cf8beef011/design/设计过程.md) · [操作手册](doc/module/r6-cf8beef011/回灌与双写巡检操作手册.md) · [巡检报告 roc](doc/module/r6-cf8beef011/report/一致性巡检-argus-single-roc.md) · [巡检报告 ives](doc/module/r6-cf8beef011/report/一致性巡检-argus-single-ives.md)

## 怎么验的

`go build` / `go vet` / `gofmt` / `go test`：本任务涉及的包全绿（eventstore 37 + backfill 15 个非集成用例）。集成用例 `EVENTSTORE_TEST_DSN` 指向本地一次性 schema，8 个全过。

**全量回灌实跑**（62 个 JSONL，本地 MySQL 8）：

| 实例 | 文件 | 天 | 解析 | 去重后 | 重复 | 入库 |
|---|---:|---:|---:|---:|---:|---:|
| `argus-single-roc` | 58 | 53 | 179,308 | 166,662 | 12,646 | 166,662 |
| `argus-single-ives` | 4 | 4 | 5,829 | 5,829 | 0 | 5,829 |

坏行 0；重叠快照正确收敛（7/10 解析 5177 → 唯一 2965，重复 2212 恰是那个被截断的快照）；原样重跑 `inserted=0 / existed=全部`；53+4 天全部 `missing=0 extra=0 jsonlMissingDays=0`。

真库查询：8/18–8/21 实例1 **16428 条事件**、**613 个去重触发时刻**，`ts` 与 JSONL 逐字一致，`gate_kind` 可聚合（`reverse_gate_profit` 527 / `cap` 221），uid 按实例正确映射，`equity_known` 在 7/19–20 全 0、7/21 转折（1608/2888）、7/22 起 100%——与设计文档记录的 7/21 上线日吻合。`git status logs/` 为空，JSONL mtime 未变。

**注意**：收尾复检时 `service` / `manager-api` / `oracle` 整体 `go build` 失败，来自**并发进行的 r8**（未提交的 `service/trade/signal_backtest.go` 等，引用尚不存在的 DTO 与 repository 方法），与本任务无关；本任务的三处代码单独编译、vet、测试全绿。

## 对照验收标准

| 任务说明 | 状态 |
|---|---|
| 照 `cmd/argus-config-import` 先例新增 `cmd/argus-event-import` | ✅ 同目录、同 flag 风格、同 dry-run 默认 |
| 坑①老日志缺 `equity`/`equityKnown`/`gapBp` | ✅ `EquityKnown` 回退 + `net_size_known` 按天×账户自适应（不硬编码 7/21）+ 报告逐日打覆盖率 |
| 坑② `instId` 系统性分裂 | ✅ 归一化 `BTCUSDT` + `inst_id_raw` 保留原值 |
| 坑③事件无 uid | ✅ 从 properties 构建映射；未知标签**拒绝导入并报错**，绝不从标签猜数字 |
| 按来源目录归属实例 | ✅ `--source` + `--instance`；两个 8.18-8.21 目录各自入库、`event_hash` 含实例键不互相吞 |
| 去重靠 r4 的 `event_hash` | ✅ 12,646 行重复被收敛；直写/回灌同哈希已用集成用例钉住 |
| 每日一致性巡检报告 | ✅ 六节 Markdown、渲染确定性可 diff、退出码可 cron |
| 不修改历史 JSONL | ✅ 只读；mtime 与 git status 双验证 |
| 需求 §6.1.2「613 次触发、实例归属正确」 | ✅ 613 / 16428 逐个对上 |
| 需求 §6.1.2「6 次兜底止损」 | ⚠️ **对不上**，见下 |
| 「20 笔完整持仓」 | 未覆盖——属 r10 的 episode 派生；本任务只保证所需事件完整入库 |

## 遗留与风险

1. **兜底止损次数与需求对不上**：任务说明与需求大纲 §6.1.2 写 6 次，原始 JSONL 实测实例1 **5 次**、实例2 **2 次**（合计 7）。逐条核对了 ts / 账户 / ROI，没有重复行也没有被去重掉的。"6" 在原始数据里找不到对应口径，需要需求方确认——建议以巡检报告的实测数为准。
2. `open` 缺 `size` 30 条被矩阵报成异常，大概率是 reduce-to-zero 的正常语义。建议 r10 派生 episode 时确认，确认正常就把 `open` 的 `size` 从必需集去掉。
3. 回灌行 `config_version=0`：历史事件没有可考的配置版本，r9/r10 按版本分组时历史段会全落进 0。
4. 双写期巡检必须跑**昨天**，跑今天会因两边快照时刻不同假报缺失（已写进操作手册）。
5. 本轮只在本地一次性 schema `argus_event_check` 验证，未接共享库；DSN 不进任何代码与文档。

## 测试交接

- **入口**：`server/manager-api/cmd/argus-event-import`，从 `server/manager-api` 跑；不给 `--dsn` 时读该目录 `configs/application.properties` 的 `sqlconn`。
- **前置**：可访问的 MySQL 8；实例需已由 `argus-config-import` 登记进 `argus_instance`（注册表为空时放行）。
- **顺序**：先不加 flag 跑 dry run 看报告 → `--apply` 回灌 → 再跑一次 `--apply` 验幂等（应 `inserted=0`）→ `--audit` 验一致性。
- **样本**：`logs/argus_single/` 八个目录，映射表见操作手册 §2。
- **可观察**：报告第 4 节 `缺失`/`多出` 全 0；退出码 0。
- **清理**：验证库可 `DROP DATABASE`；本机遗留的一次性 schema 为 `argus_event_check`。
- 报告与文档不含任何凭证、不含账户邮箱。

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJfOTIzX1oweGU0NTdXT29RUU5BUXRBIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMVQxNTozMzozMi44NTI2MTArMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDFUMTY6MDE6MDYuMzEzMzE0KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8mmxvZ3MvYXJndXNfc2luZ2xlIOWOhuWPsiBKU09OTCDlm57ngYzkuI7lj4zlhpnkuIDoh7TmgKfmoKHpqowifV19LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIHN0YXJ0IGJ5IGxvYWRpbmcgdGhlIGRlbGl2ZXJ5LWFjdGlvbi1leGVjdXRpb24gc2tpbGwgYXMgaW5zdHJ1Y3RlZC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgSSdsbCBpbXBsZW1lbnQuIEZpcnN0LCBleHRyYWN0IGEgcmV1c2FibGUgY29ubmVjdGlvbiBvcGVuZXIgaW4gYGV2ZW50c3RvcmVgOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgYmFja2ZpbGwgcGFja2FnZSDigJQgSlNPTkwgc2Nhbm5pbmcgd2l0aCBob25lc3QgbGluZS1sZXZlbCBhY2NvdW50aW5nOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgZGFpbHkgY29uc2lzdGVuY3kgcmVwb3J0IHJlbmRlcmVyOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgYWNjb3VudC1pZGVudGl0eSBsb2FkZXIgaW4gdGhlIGNvbmZpZyBkb21haW46Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBDTEk6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTGV0IG1lIGFkZCBhIGNvbXBsZXRlbmVzcyBjaGVjayBmb3IgZGF5cyBwcmVzZW50IGluIHRoZSBEQiBidXQgYWJzZW50IGZyb20gSlNPTkw6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHJlZ2VuZXJhdGUgdGhlIGRlbGl2ZXJhYmxlIHJlcG9ydHMgZnJvbSBhIGNsZWFuIHNjaGVtYToiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIHJ1bmJvb2s6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHJlZ2VuZXJhdGUgdGhlIGRlbGl2ZXJhYmxlIHJlcG9ydHMgb24gYSBjbGVhbiBzY2hlbWEgYW5kIGNvbmZpcm0gdGhlIGludGVncmF0aW9uIHN1aXRlIG5vIGxvbmdlciB0b3VjaGVzIHJlYWwgaW5zdGFuY2Uga2V5czoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiIjIyDmlLnkuobku4DkuYhcblxuKirmlrDlop4gYHNlcnZlci9hcmd1c19zaW5nbGUvcGtnL2V2ZW50c3RvcmUvYmFja2ZpbGwvYCoq77yIcjQg55qEIGV2ZW50c3RvcmUg5a2Q5YyF77yM5a6e55uY5LqM6L+b5Yi25LiNIGltcG9ydCDlsLHkuI3pk77ov5vljrvvvIlcblxufCDmlofku7YgfCDmhI/lm74gfFxufC0tLXwtLS18XG58IGBzY2FuLmdvYCB8IOWPquiupCBgZXZlbnRzLTxkYXRlPi5qc29ubGDvvIjnm67lvZXph4zourrnnYAgQ2hhdEV4cG9ydCAvIGh0bWwgLyB0eHTvvInvvJvpgJDooYzop6PmnpDlubbnu5/orqHlnY/ooYzjgIHnqbrooYzjgIHot6jml6XigJTigJRgZXZlbnRsb2cuUGFyc2VGaWxlYCDpnZnpu5jot7PlnY/ooYzvvIzmiqXlkYrkuI3og73miornvLrlj6Pol4/otbfmnaUgfFxufCBgbWF0cml4LmdvYCB8IOiuvuiuoeaWh+ahoyDCpzYuNiDlrZfmrrXpgILnlKjmgKfnn6npmLXvvIzlm57ngYzml7blhbzkvZzmoKHpqozlmajvvIzlj6rmiqXmlbDkuI3mi5Lnu53lr7zlhaUgfFxufCBgYnVpbGQuZ29gIHwg5aSa55uu5b2V5omr5o+PIOKGkiB1aWQg5pig5bCEIOKGkiDmjInlpKnliIbnu4Qg4oaSIOWkjeeUqCBgZXZlbnRzdG9yZS5Db252ZXJ0YCDihpIgYGV2ZW50X2hhc2hgIOWOu+mHjSDihpIgYG5ldF9zaXplX2tub3duYCDmjInjgIzlpKnDl+i0puaIt+OAjeiHqumAguW6lCDihpIg6KaG55uW546H57uf6K6hIHxcbnwgYGltcG9ydC5nb2AgfCBgRW5zdXJlVGFibGVzYCAvIGBXcml0ZURheWDvvIhgT04gRFVQTElDQVRFIEtFWSBVUERBVEUgaWQ9aWRg77yJIHxcbnwgYGF1ZGl0LmdvYCB8IGBBdWRpdERheWDvvIjmjIkgYGV2ZW50X2hhc2hgIOmbhuWQiOW3ru+8ieOAgWBGaW5kT3JwaGFuRGF5c2DvvIjlupPph4zmnInmlbTlpKnjgIFKU09OTCDmlbTlpKnmsqHmnInvvIkgfFxufCBgcmVwb3J0LmdvYCB8IOWFreiKgiBNYXJrZG93biDlt6Hmo4DmiqXlkYrvvIzooYzluo/lhajmjpLluo/kv53or4HkuKTlpKnlj68gZGlmZiB8XG58IDQg5Liq5rWL6K+V5paH5Lu2IHwgMTUg5Liq5Y2V5rWLICsgNSDkuKrnnJ/lupPpm4bmiJDnlKjkvosgfFxuXG4qKuaUueWKqCoqXG4tIGBwa2cvZXZlbnRzdG9yZS9zdG9yZS5nb2DvvJrmir3lh7rlr7zlh7rnmoQgYE9wZW5EQihkc24pYO+8jOWbnueBjOS4juebtOWGmeWFseeUqOWQjOS4gOWllyBEU04g5Y+j5b6E77yIYGxvYz1Mb2NhbGAg5b+F6aG755Sx5Luj56CB5by65Yi277yJ44CCYE9wZW5gIOihjOS4uuS4gOWtl+acquaUueOAglxuLSBgc2VydmljZS9hcmd1c19jb25maWcvaW1wb3J0ZXIuZ29g77yaKzUzIOihjO+8jOaWsOWiniBgTG9hZEluc3RhbmNlQWNjb3VudHMoKWDvvIzlj6ror7vlrp7kvovplK7kuI4gYHRyYWRlLmFjY291bnROLm5hbWUvdWlkL3ZhcmlhbnRg77yMKirkuI3norAgc2Vzc2lvbi5qc29u44CB5LiN6Kej5p6Q5Lu75L2V5Yet6K+BKirjgIJcbi0gYHBrZy9ldmVudHN0b3JlL3N0b3JlX2ludGVncmF0aW9uX3Rlc3QuZ29g77yacjQg55qE6ZuG5oiQ55So5L6L55So55yf5a6e5a6e5L6L6ZSuIGBhcmd1cy1zaW5nbGUtcm9jYCDlubYgYERFTEVURSBXSEVSRSBpbnN0YW5jZV9rZXk9J2FyZ3VzLXNpbmdsZS1yb2MnYOOAguacrOi9ruWunui3keiiq+ecn+WunuinpuWPkeKAlOKAlOWbnueBjOWujCAxNjYsNjYyIOihjOWQjui3keS4gOmBjSByNCDmtYvor5XvvIxyb2Mg55qEIHN0cmF0ZWd5X2V2ZW50IOS7jiA3MzQ4IOaOieWIsCA144CC5pS55oiQIGBpdC1ldmVudHN0b3JlLW1haW4vYWx0YCDkuKTkuKrmtYvor5XkuJPnlKjplK7vvIzmlq3oqIDpgLvovpHmnKrliqjjgIJcblxuKirmlrDlop4gQ0xJKiogYHNlcnZlci9tYW5hZ2VyLWFwaS9jbWQvYXJndXMtZXZlbnQtaW1wb3J0L21haW4uZ29g77yI54WnIGBhcmd1cy1jb25maWctaW1wb3J0YCDlhYjkvovvvInvvJrpu5jorqQgZHJ5IHJ1buOAgWAtLWFwcGx5YCDlm57ngYzjgIFgLS1hdWRpdGAg5Y+q5beh5qOA77ybYC0tc291cmNlYCDlj6/ph43lpI3vvIjlrp7kvovlvZLlsZ7mjInnm67lvZXvvIznlLHkurrmmL7lvI/lo7DmmI7vvInvvJvnvLrlpLHooYzmiJbnvLrmlbTlpKkg4oaSIOmAgOWHuueggSAy77yMY3JvbiDlj6/nm7TmjqXlkYrorabjgIJcblxuKirmlofmoaMqKu+8mlvorr7orqHov4fnqItdKGRvYy9tb2R1bGUvcjYtY2Y4YmVlZjAxMS9kZXNpZ24v6K6+6K6h6L+H56iLLm1kKSDCtyBb5pON5L2c5omL5YaMXShkb2MvbW9kdWxlL3I2LWNmOGJlZWYwMTEv5Zue54GM5LiO5Y+M5YaZ5beh5qOA5pON5L2c5omL5YaMLm1kKSDCtyBb5beh5qOA5oql5ZGKIHJvY10oZG9jL21vZHVsZS9yNi1jZjhiZWVmMDExL3JlcG9ydC/kuIDoh7TmgKflt6Hmo4AtYXJndXMtc2luZ2xlLXJvYy5tZCkgwrcgW+W3oeajgOaKpeWRiiBpdmVzXShkb2MvbW9kdWxlL3I2LWNmOGJlZWYwMTEvcmVwb3J0L+S4gOiHtOaAp+W3oeajgC1hcmd1cy1zaW5nbGUtaXZlcy5tZClcblxuIyMg5oCO5LmI6aqM55qEXG5cbmBnbyBidWlsZGAgLyBgZ28gdmV0YCAvIGBnb2ZtdGAgLyBgZ28gdGVzdGDvvJrmnKzku7vliqHmtonlj4rnmoTljIXlhajnu7/vvIhldmVudHN0b3JlIDM3ICsgYmFja2ZpbGwgMTUg5Liq6Z2e6ZuG5oiQ55So5L6L77yJ44CC6ZuG5oiQ55So5L6LIGBFVkVOVFNUT1JFX1RFU1RfRFNOYCDmjIflkJHmnKzlnLDkuIDmrKHmgKcgc2NoZW1h77yMOCDkuKrlhajov4fjgIJcblxuKirlhajph4/lm57ngYzlrp7ot5EqKu+8iDYyIOS4qiBKU09OTO+8jOacrOWcsCBNeVNRTCA477yJ77yaXG5cbnwg5a6e5L6LIHwg5paH5Lu2IHwg5aSpIHwg6Kej5p6QIHwg5Y676YeN5ZCOIHwg6YeN5aSNIHwg5YWl5bqTIHxcbnwtLS18LS0tOnwtLS06fC0tLTp8LS0tOnwtLS06fC0tLTp8XG58IGBhcmd1cy1zaW5nbGUtcm9jYCB8IDU4IHwgNTMgfCAxNzksMzA4IHwgMTY2LDY2MiB8IDEyLDY0NiB8IDE2Niw2NjIgfFxufCBgYXJndXMtc2luZ2xlLWl2ZXNgIHwgNCB8IDQgfCA1LDgyOSB8IDUsODI5IHwgMCB8IDUsODI5IHxcblxu5Z2P6KGMIDDvvJvph43lj6Dlv6vnhafmraPnoa7mlLbmlZvvvIg3LzEwIOino+aekCA1MTc3IOKGkiDllK/kuIAgMjk2Ne+8jOmHjeWkjSAyMjEyIOaBsOaYr+mCo+S4quiiq+aIquaWreeahOW/q+eFp++8ie+8m+WOn+agt+mHjei3kSBgaW5zZXJ0ZWQ9MCAvIGV4aXN0ZWQ95YWo6YOoYO+8mzUzKzQg5aSp5YWo6YOoIGBtaXNzaW5nPTAgZXh0cmE9MCBqc29ubE1pc3NpbmdEYXlzPTBg44CCXG5cbuecn+W6k+afpeivou+8mjgvMTjigJM4LzIxIOWunuS+izEgKioxNjQyOCDmnaHkuovku7YqKuOAgSoqNjEzIOS4quWOu+mHjeinpuWPkeaXtuWIuyoq77yMYHRzYCDkuI4gSlNPTkwg6YCQ5a2X5LiA6Ie077yMYGdhdGVfa2luZGAg5Y+v6IGa5ZCI77yIYHJldmVyc2VfZ2F0ZV9wcm9maXRgIDUyNyAvIGBjYXBgIDIyMe+8ie+8jHVpZCDmjInlrp7kvovmraPnoa7mmKDlsITvvIxgZXF1aXR5X2tub3duYCDlnKggNy8xOeKAkzIwIOWFqCAw44CBNy8yMSDovazmipjvvIgxNjA4LzI4ODjvvInjgIE3LzIyIOi1tyAxMDAl4oCU4oCU5LiO6K6+6K6h5paH5qGj6K6w5b2V55qEIDcvMjEg5LiK57q/5pel5ZC75ZCI44CCYGdpdCBzdGF0dXMgbG9ncy9gIOS4uuepuu+8jEpTT05MIG10aW1lIOacquWPmOOAglxuXG4qKuazqOaEjyoq77ya5pS25bC+5aSN5qOA5pe2IGBzZXJ2aWNlYCAvIGBtYW5hZ2VyLWFwaWAgLyBgb3JhY2xlYCDmlbTkvZMgYGdvIGJ1aWxkYCDlpLHotKXvvIzmnaXoh6oqKuW5tuWPkei/m+ihjOeahCByOCoq77yI5pyq5o+Q5Lqk55qEIGBzZXJ2aWNlL3RyYWRlL3NpZ25hbF9iYWNrdGVzdC5nb2Ag562J77yM5byV55So5bCa5LiN5a2Y5Zyo55qEIERUTyDkuI4gcmVwb3NpdG9yeSDmlrnms5XvvInvvIzkuI7mnKzku7vliqHml6DlhbPvvJvmnKzku7vliqHnmoTkuInlpITku6PnoIHljZXni6znvJbor5HjgIF2ZXTjgIHmtYvor5Xlhajnu7/jgIJcblxuIyMg5a+554Wn6aqM5pS25qCH5YeGXG5cbnwg5Lu75Yqh6K+05piOIHwg54q25oCBIHxcbnwtLS18LS0tfFxufCDnhacgYGNtZC9hcmd1cy1jb25maWctaW1wb3J0YCDlhYjkvovmlrDlop4gYGNtZC9hcmd1cy1ldmVudC1pbXBvcnRgIHwg4pyFIOWQjOebruW9leOAgeWQjCBmbGFnIOmjjuagvOOAgeWQjCBkcnktcnVuIOm7mOiupCB8XG58IOWdkeKRoOiAgeaXpeW/l+e8uiBgZXF1aXR5YC9gZXF1aXR5S25vd25gL2BnYXBCcGAgfCDinIUgYEVxdWl0eUtub3duYCDlm57pgIAgKyBgbmV0X3NpemVfa25vd25gIOaMieWkqcOX6LSm5oi36Ieq6YCC5bqU77yI5LiN56Gs57yW56CBIDcvMjHvvIkrIOaKpeWRiumAkOaXpeaJk+imhueblueOhyB8XG58IOWdkeKRoSBgaW5zdElkYCDns7vnu5/mgKfliIboo4IgfCDinIUg5b2S5LiA5YyWIGBCVENVU0RUYCArIGBpbnN0X2lkX3Jhd2Ag5L+d55WZ5Y6f5YC8IHxcbnwg5Z2R4pGi5LqL5Lu25pegIHVpZCB8IOKchSDku44gcHJvcGVydGllcyDmnoTlu7rmmKDlsITvvJvmnKrnn6XmoIfnrb4qKuaLkue7neWvvOWFpeW5tuaKpemUmSoq77yM57ud5LiN5LuO5qCH562+54yc5pWw5a2XIHxcbnwg5oyJ5p2l5rqQ55uu5b2V5b2S5bGe5a6e5L6LIHwg4pyFIGAtLXNvdXJjZWAgKyBgLS1pbnN0YW5jZWDvvJvkuKTkuKogOC4xOC04LjIxIOebruW9leWQhOiHquWFpeW6k+OAgWBldmVudF9oYXNoYCDlkKvlrp7kvovplK7kuI3kupLnm7jlkJ4gfFxufCDljrvph43pnaAgcjQg55qEIGBldmVudF9oYXNoYCB8IOKchSAxMiw2NDYg6KGM6YeN5aSN6KKr5pS25pWb77yb55u05YaZL+WbnueBjOWQjOWTiOW4jOW3sueUqOmbhuaIkOeUqOS+i+mSieS9jyB8XG58IOavj+aXpeS4gOiHtOaAp+W3oeajgOaKpeWRiiB8IOKchSDlha3oioIgTWFya2Rvd27jgIHmuLLmn5Pnoa7lrprmgKflj68gZGlmZuOAgemAgOWHuueggeWPryBjcm9uIHxcbnwg5LiN5L+u5pS55Y6G5Y+yIEpTT05MIHwg4pyFIOWPquivu++8m210aW1lIOS4jiBnaXQgc3RhdHVzIOWPjOmqjOivgSB8XG58IOmcgOaxgiDCpzYuMS4y44CMNjEzIOasoeinpuWPkeOAgeWunuS+i+W9kuWxnuato+ehruOAjSB8IOKchSA2MTMgLyAxNjQyOCDpgJDkuKrlr7nkuIogfFxufCDpnIDmsYIgwqc2LjEuMuOAjDYg5qyh5YWc5bqV5q2i5o2f44CNIHwg4pqg77iPICoq5a+55LiN5LiKKirvvIzop4HkuIsgfFxufCDjgIwyMCDnrJTlrozmlbTmjIHku5PjgI0gfCDmnKropobnm5bigJTigJTlsZ4gcjEwIOeahCBlcGlzb2RlIOa0vueUn++8m+acrOS7u+WKoeWPquS/neivgeaJgOmcgOS6i+S7tuWujOaVtOWFpeW6kyB8XG5cbiMjIOmBl+eVmeS4jumjjumZqVxuXG4xLiAqKuWFnOW6leatouaNn+asoeaVsOS4jumcgOaxguWvueS4jeS4iioq77ya5Lu75Yqh6K+05piO5LiO6ZyA5rGC5aSn57qyIMKnNi4xLjIg5YaZIDYg5qyh77yM5Y6f5aeLIEpTT05MIOWunua1i+WunuS+izEgKio1IOasoSoq44CB5a6e5L6LMiAqKjIg5qyhKirvvIjlkIjorqEgN++8ieOAgumAkOadoeaguOWvueS6hiB0cyAvIOi0puaItyAvIFJPSe+8jOayoeaciemHjeWkjeihjOS5n+ayoeacieiiq+WOu+mHjeaOieeahOOAglwiNlwiIOWcqOWOn+Wni+aVsOaNrumHjOaJvuS4jeWIsOWvueW6lOWPo+W+hO+8jOmcgOimgemcgOaxguaWueehruiupOKAlOKAlOW7uuiuruS7peW3oeajgOaKpeWRiueahOWunua1i+aVsOS4uuWHhuOAglxuMi4gYG9wZW5gIOe8uiBgc2l6ZWAgMzAg5p2h6KKr55+p6Zi15oql5oiQ5byC5bi477yM5aSn5qaC546H5pivIHJlZHVjZS10by16ZXJvIOeahOato+W4uOivreS5ieOAguW7uuiuriByMTAg5rS+55SfIGVwaXNvZGUg5pe256Gu6K6k77yM56Gu6K6k5q2j5bi45bCx5oqKIGBvcGVuYCDnmoQgYHNpemVgIOS7juW/hemcgOmbhuWOu+aOieOAglxuMy4g5Zue54GM6KGMIGBjb25maWdfdmVyc2lvbj0wYO+8muWOhuWPsuS6i+S7tuayoeacieWPr+iAg+eahOmFjee9rueJiOacrO+8jHI5L3IxMCDmjInniYjmnKzliIbnu4Tml7bljoblj7LmrrXkvJrlhajokL3ov5sgMOOAglxuNC4g5Y+M5YaZ5pyf5beh5qOA5b+F6aG76LeRKirmmKjlpKkqKu+8jOi3keS7iuWkqeS8muWboOS4pOi+ueW/q+eFp+aXtuWIu+S4jeWQjOWBh+aKpee8uuWkse+8iOW3suWGmei/m+aTjeS9nOaJi+WGjO+8ieOAglxuNS4g5pys6L2u5Y+q5Zyo5pys5Zyw5LiA5qyh5oCnIHNjaGVtYSBgYXJndXNfZXZlbnRfY2hlY2tgIOmqjOivge+8jOacquaOpeWFseS6q+W6k++8m0RTTiDkuI3ov5vku7vkvZXku6PnoIHkuI7mlofmoaPjgIJcblxuIyMg5rWL6K+V5Lqk5o6lXG5cbi0gKirlhaXlj6MqKu+8mmBzZXJ2ZXIvbWFuYWdlci1hcGkvY21kL2FyZ3VzLWV2ZW50LWltcG9ydGDvvIzku44gYHNlcnZlci9tYW5hZ2VyLWFwaWAg6LeR77yb5LiN57uZIGAtLWRzbmAg5pe26K+76K+l55uu5b2VIGBjb25maWdzL2FwcGxpY2F0aW9uLnByb3BlcnRpZXNgIOeahCBgc3FsY29ubmDjgIJcbi0gKirliY3nva4qKu+8muWPr+iuv+mXrueahCBNeVNRTCA477yb5a6e5L6L6ZyA5bey55SxIGBhcmd1cy1jb25maWctaW1wb3J0YCDnmbvorrDov5sgYGFyZ3VzX2luc3RhbmNlYO+8iOazqOWGjOihqOS4uuepuuaXtuaUvuihjO+8ieOAglxuLSAqKumhuuW6jyoq77ya5YWI5LiN5YqgIGZsYWcg6LeRIGRyeSBydW4g55yL5oql5ZGKIOKGkiBgLS1hcHBseWAg5Zue54GMIOKGkiDlho3ot5HkuIDmrKEgYC0tYXBwbHlgIOmqjOW5guetie+8iOW6lCBgaW5zZXJ0ZWQ9MGDvvInihpIgYC0tYXVkaXRgIOmqjOS4gOiHtOaAp+OAglxuLSAqKuagt+acrCoq77yaYGxvZ3MvYXJndXNfc2luZ2xlL2Ag5YWr5Liq55uu5b2V77yM5pig5bCE6KGo6KeB5pON5L2c5omL5YaMIMKnMuOAglxuLSAqKuWPr+inguWvnyoq77ya5oql5ZGK56ysIDQg6IqCIGDnvLrlpLFgL2DlpJrlh7pgIOWFqCAw77yb6YCA5Ye656CBIDDjgIJcbi0gKirmuIXnkIYqKu+8mumqjOivgeW6k+WPryBgRFJPUCBEQVRBQkFTRWDvvJvmnKzmnLrpgZfnlZnnmoTkuIDmrKHmgKcgc2NoZW1hIOS4uiBgYXJndXNfZXZlbnRfY2hlY2tg44CCXG4tIOaKpeWRiuS4juaWh+aho+S4jeWQq+S7u+S9leWHreivgeOAgeS4jeWQq+i0puaIt+mCrueuseOAglxuXG7mibnph4/liKTlrprvvJrlrozmiJAiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6ImZpbmFsX2Fuc3dlciJ9XX1dfQ==
-->
