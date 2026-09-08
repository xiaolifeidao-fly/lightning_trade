---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r3-2de5f8f268"
requirementKey: "req-1788169552180"
resourceName: "触发瞬间 ±1min 秒级切片采集 + dev_sample 采样粒度提升至 10 秒"
conversationTitle: "触发瞬间 ±1min 秒级切片采集 + dev_sample 采样粒度提升至 10 秒 V0.0.1"
threadId: "5ece54d1-4632-4ead-bb67-8dd973c59a21"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-02T01:11:21.515349+00:00"
---

# 任务聊天 · 触发瞬间 ±1min 秒级切片采集 + dev_sample 采样粒度提升至 10 秒 V0.0.1

## 第 1 轮 · completed · mfG5pna9vUCbCLw3M2WL5A

### 用户

执行「动作执行」阶段：触发瞬间 ±1min 秒级切片采集 + dev_sample 采样粒度提升至 10 秒

### 助手

I'll start by loading the delivery execution skill and reading the requirement docs.

### 助手

I'll read the requirement outline and survey the codebase.

### 助手

Now the ring buffer and monitor wiring:

### 助手

Now wire it into `price_monitor.go`:

### 助手

Now fix the wiring test so it's interval-agnostic:

### 助手

Now the eventstore side — model, convert, writer, retention:

### 助手

Now the writer side (`store.go`) and retention:

### 助手

Now register the slice sink in `runtime.go`:

### 助手

Now the read side. First the repository:

### 助手

Now the DTO additions:

### 助手

Now rewrite `slice.go`:

### 助手

Now frontend model sync and tests:

### 助手

Now the tests for the new behaviour:

### 助手

Now eventstore slice tests:

### 助手

Now a service-side test for the read path:

### 助手

Now the design document:

### 助手

Work is complete and verified.

## 改了什么

**读过的文档**：`doc/requirements/req-1788169552180/需求大纲.md`（完整读，验收与容量口径以它为准）。**没有**打开 r1 的需求文档——`instance_key` 的口径已经由 `pkg/eventstore` 的实现固化（`Options.InstanceKey` ← `runtimeManager.InstanceID()`），本任务只是复用同一个字段，没有新的接口面。前置任务 r2 的产出（`trade_kline` 平台维度）只被读侧当上下文参照线用，`slice.go` 里已有的 `NormalizeKlinePlatform` 调用未改。

设计过程文档：[doc/module/r3-2de5f8f268/design/设计过程.md](doc/module/r3-2de5f8f268/design/设计过程.md)

### 新增
- `server/argus_single/pkg/marketslice/marketslice.go` — 新 leaf 包：`Slice` 载荷（±60s / 121 点）+ `Sink` 接口，照 `pkg/eventlog/sink.go` 的形状。避免 monitor↔eventstore 依赖倒挂。
- `server/argus_single/pkg/monitor/slice_buffer.go` — 秒级环形缓冲（240 秒容量）+ `PriceMonitor` 接线：`observeSliceQuote` / `observeSliceBinance` / `armSlice` / `takeDueSlices` / `runSliceFlusher`。`sliceMu` 是**叶子锁**，获取时不持有 `mu`/`signalMu`，不引入新锁序。
- `server/argus_single/pkg/eventstore/retention.go` — 90 天滚动清理，分批 DELETE，只清 `signal_slice`。
- 测试：`pkg/marketslice/marketslice_test.go`、`pkg/monitor/slice_buffer_test.go`、`pkg/eventstore/slice_test.go`、`pkg/eventstore/slice_integration_test.go`、`server/service/argus_event/slice_test.go`。

### 修改
- `pkg/monitor/dev_sampler.go` — `devSampleInterval`: `time.Minute` → `10 * time.Second`。`DevSampleThresholdsBp` 九个阈值与 `DevCross/DevOver` 结构未改一字。
- `pkg/monitor/price_monitor.go` — 三个切片字段 + 构造初始化；`Start()` 起 flusher；币安 handler 在 `mu` 解锁后落槽；信号 handler 在 `signalMu` **之前** 落槽、`fire` 后（锁外）登记切片。
- `pkg/eventstore/model.go` — 新增 `SignalSlice`（表 `signal_slice`，唯一键 `(instance_key, instrument, ts)`），进 `Models()`。
- `pkg/eventstore/convert.go` / `store.go` / `runtime.go` — `Envelope.Slice` 分流 + `ConvertSlice`；`EmitSlice` 复用同一队列与背压，`signal_slice` 用独立 `sliceBatchSize=20`（单行 ~4KB）；`Setup` 注册 sink 并启动清理。
- `server/service/argus_event/{slice.go,dto/dto.go,service.go,repository/repository.go}` — `signal_slice` 优先供数，取不到才降级回 `strategy_event` 稀疏观测点；新增 `binLast` / `sliceAnchorTs` / `anchorLagSec` / `dcPoints` / `binPoints`。
- `client/manager/.../argus-market/api/argus-market.api.ts` — 同步模型字段。
- `pkg/monitor/dev_sample_wiring_test.go` — 时间偏移改成相对 `devSampleInterval`（原来写死 30s/61s，改 10s 后测试会失去意义）。

**两处偏离任务说明字面的判断**（理由在设计文档 §1、§3.7）：① "触发时 flush" 实现为"触发时定格 + 60 秒后落盘"——右半窗在触发那一刻还没发生；② 读侧一并接上，因为 `slice.go` 的注释已经把"r3 落地后换 tickSource"写成了本任务的收尾动作，只建表不接读等于交付一张没人读的表。

另外发现：**事件 ts ≠ 触发瞬间**（`trade.signal.delay_seconds` 默认 5 秒 + 下单往返），所以切片关联用"回看 30 秒取最近锚点"，且 points 以切片锚点为原点。这是本轮最容易做错的一处。

## 怎么验的

```
argus_single: go build ./... + go test ./...                    全部 ok
argus_single: go test -race ./pkg/{monitor,eventstore,marketslice}  全部 ok
service:      go build ./... + go test ./...                    全部 ok
manager-api:  go vet ./...                                      ok
common:       go build + go test                                ok
client/manager: npx tsc --noEmit                                无输出（通过）
gofmt:        新增/改动文件全部干净（仓库既有的 CJK 对齐漂移未动）
```

未通过项：`server/oracle` `go build` 报 `missing go.sum entry for github.com/go-redis/redis`。本轮未改动 `server/oracle` 与 `server/common`（两者在本轮开始前的 `git status` 里已是 modified），是先行任务的遗留依赖状态，需要有人补 `go mod tidy`。

## 对照验收标准（需求大纲 §6.1 中与本任务相关的两条）

- **§6.1.10 容量达标（切片日增 ≤1.5MB 且 90 天外自动清理）**：已满足实现——单行 ~4KB × 每次触发一条（实测 613 条/4 天 ≈ 150 条/天 ≈ 0.6MB/天）；`SliceRetention = 90 天` 有分批清理 goroutine，边界由 `TestIntegrationSlicePurgeRespectsRetention` 覆盖。日增实数需上线后实测。
- **§6.1.1 / 触发瞬间可下钻**：读侧已能返回 121 点逐秒序列（含币安 last 与现算的 121 点 gapBp）。前端抽屉属 r12，本轮只同步了数据模型，所以这条**部分满足**——后端契约已就位。
- **λ(θ) 口径不变（§3.3 频率级回测唯一数据源）**：满足。穿越总数不随窗口大小变化（`lastDerived` 跨窗口保留），读侧 `lambda.go windowStep` 取相邻间隔中位数、已预留混合步长支持。
- **写库失败不阻断交易（§3.6）**：满足。`EmitSlice` 非阻塞、队列满即丢并计数；清理是独立 goroutine。

## 测试交接信息

- **单测**：无需环境，`cd server/argus_single && go test ./...`、`cd server/service && go test ./...`。
- **集成测试**（需要一个**可丢弃**的 MySQL schema，绝不能指生产库，实例键已用 `it-` 前缀隔离）：
  - `EVENTSTORE_TEST_DSN=... go test ./pkg/eventstore/ -run TestIntegrationSignalSlice -v`（去重 + ts 墙钟口径）
  - `EVENTSTORE_TEST_DSN=... go test ./pkg/eventstore/ -run TestIntegrationSlicePurge -v`（90 天边界）
  - `ARGUS_EVENT_TEST_DSN=... go test ./argus_event/ -run TestIntegrationSlice -v`（signal_slice 供数 + 降级路径）
- **端到端**（需真实 WS + MySQL，本轮无法在开发机验证）：起 argus_single → 日志应出现 `秒级切片入 signal_slice，保留 90 天` → 等一次真实触发，60 余秒后 `SELECT ts, dc_points, bin_points, JSON_LENGTH(dc_last_json) FROM signal_slice` 应见 `121 / 接近121 / 121` → 调 `GET /api/argus-event/signals/{id}/slice`，`tickSource=signal_slice`、`tickComplete=true`、121 点、`binLast` 非空 → `SELECT COUNT(*) FROM dev_sample WHERE ts >= CURDATE()` 日增接近 8640。
- **清理**：集成用例自己 `DELETE ... WHERE instance_key IN ('it-...')`；表由 `AutoMigrate` 自动创建，无手工 DDL。

## 遗留与风险

1. **切片与事件的关联是启发式的**：事件行没有锚点字段，只能"回看 30 秒取最近锚点"。同一簇里 20 秒内连着两次触发时，晚到的成交事件可能归到后一个锚点（两条切片窗口高度重叠，图形结论不受影响）。要精确关联需在埋点上给事件带锚点时间戳 —— 建议记进 r13。
2. **切片没有兜底真源**：不进 JSONL（理由见设计文档 §3.5），丢了就是永久缺失，`Stats().Dropped/Failed` 是唯一线索。
3. **`dc_points` 的正常值不是 121**：安静的那几秒 DeepCoin 可能没有 `PO` 推送，覆盖率天然低于满值。前端画图要看覆盖率，别假定 121 点都有值；告警阈值需上线后实测再定。
4. **配置热替换会产出未满窗切片**：`ReloadMonitor` 停旧 monitor 时按 `force` 如实交出（比整批丢掉好），覆盖率字段会暴露缺口。
5. **90 天清理由每个实例各自跑**：DELETE 幂等无害，但实例更多时值得挪到 manager-api 单点执行。
6. **r12 的切片抽屉未建**：本轮只同步了前端数据模型。

批量判定：需人工处理

（原因：`server/oracle` 的 `go build` 因 `go.sum` 缺 `github.com/go-redis/redis` 条目而失败。该模块与 `server/common/middleware/redis/ArgusConfig.go` 在本轮开始前就已是修改态、本轮未触碰，属先行任务遗留的依赖问题，需要有人执行一次 `go mod tidy`。本任务自身的全部改动已在 argus_single / service / manager-api / common 四个模块上编译与测试通过。）

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJtZkc1cG5hOXZVQ2JDTHczTTJXTDVBIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMlQwMDo0Nzo1My4wNzA2OTMrMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDJUMDE6MTE6MjEuMDQ3NTQ4KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8muinpuWPkeeerOmXtCDCsTFtaW4g56eS57qn5YiH54mH6YeH6ZuGICsgZGV2X3NhbXBsZSDph4fmoLfnspLluqbmj5DljYfoh7MgMTAg56eSIn1dfSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiSSdsbCBzdGFydCBieSBsb2FkaW5nIHRoZSBkZWxpdmVyeSBleGVjdXRpb24gc2tpbGwgYW5kIHJlYWRpbmcgdGhlIHJlcXVpcmVtZW50IGRvY3MuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiSSdsbCByZWFkIHRoZSByZXF1aXJlbWVudCBvdXRsaW5lIGFuZCBzdXJ2ZXkgdGhlIGNvZGViYXNlLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgcmluZyBidWZmZXIgYW5kIG1vbml0b3Igd2lyaW5nOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB3aXJlIGl0IGludG8gYHByaWNlX21vbml0b3IuZ29gOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBmaXggdGhlIHdpcmluZyB0ZXN0IHNvIGl0J3MgaW50ZXJ2YWwtYWdub3N0aWM6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBldmVudHN0b3JlIHNpZGUg4oCUIG1vZGVsLCBjb252ZXJ0LCB3cml0ZXIsIHJldGVudGlvbjoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIHdyaXRlciBzaWRlIChgc3RvcmUuZ29gKSBhbmQgcmV0ZW50aW9uOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyByZWdpc3RlciB0aGUgc2xpY2Ugc2luayBpbiBgcnVudGltZS5nb2A6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSByZWFkIHNpZGUuIEZpcnN0IHRoZSByZXBvc2l0b3J5OiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgRFRPIGFkZGl0aW9uczoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgcmV3cml0ZSBgc2xpY2UuZ29gOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBmcm9udGVuZCBtb2RlbCBzeW5jIGFuZCB0ZXN0czoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIHRlc3RzIGZvciB0aGUgbmV3IGJlaGF2aW91cjoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgZXZlbnRzdG9yZSBzbGljZSB0ZXN0czoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgYSBzZXJ2aWNlLXNpZGUgdGVzdCBmb3IgdGhlIHJlYWQgcGF0aDoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIGRlc2lnbiBkb2N1bWVudDoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJXb3JrIGlzIGNvbXBsZXRlIGFuZCB2ZXJpZmllZC5cblxuIyMg5pS55LqG5LuA5LmIXG5cbioq6K+76L+H55qE5paH5qGjKirvvJpgZG9jL3JlcXVpcmVtZW50cy9yZXEtMTc4ODE2OTU1MjE4MC/pnIDmsYLlpKfnurIubWRg77yI5a6M5pW06K+777yM6aqM5pS25LiO5a656YeP5Y+j5b6E5Lul5a6D5Li65YeG77yJ44CCKirmsqHmnIkqKuaJk+W8gCByMSDnmoTpnIDmsYLmlofmoaPigJTigJRgaW5zdGFuY2Vfa2V5YCDnmoTlj6PlvoTlt7Lnu4/nlLEgYHBrZy9ldmVudHN0b3JlYCDnmoTlrp7njrDlm7rljJbvvIhgT3B0aW9ucy5JbnN0YW5jZUtleWAg4oaQIGBydW50aW1lTWFuYWdlci5JbnN0YW5jZUlEKClg77yJ77yM5pys5Lu75Yqh5Y+q5piv5aSN55So5ZCM5LiA5Liq5a2X5q6177yM5rKh5pyJ5paw55qE5o6l5Y+j6Z2i44CC5YmN572u5Lu75YqhIHIyIOeahOS6p+WHuu+8iGB0cmFkZV9rbGluZWAg5bmz5Y+w57u05bqm77yJ5Y+q6KKr6K+75L6n5b2T5LiK5LiL5paH5Y+C54Wn57q/55So77yMYHNsaWNlLmdvYCDph4zlt7LmnInnmoQgYE5vcm1hbGl6ZUtsaW5lUGxhdGZvcm1gIOiwg+eUqOacquaUueOAglxuXG7orr7orqHov4fnqIvmlofmoaPvvJpbZG9jL21vZHVsZS9yMy0yZGU1ZjhmMjY4L2Rlc2lnbi/orr7orqHov4fnqIsubWRdKGRvYy9tb2R1bGUvcjMtMmRlNWY4ZjI2OC9kZXNpZ24v6K6+6K6h6L+H56iLLm1kKVxuXG4jIyMg5paw5aKeXG4tIGBzZXJ2ZXIvYXJndXNfc2luZ2xlL3BrZy9tYXJrZXRzbGljZS9tYXJrZXRzbGljZS5nb2Ag4oCUIOaWsCBsZWFmIOWMhe+8mmBTbGljZWAg6L296I2377yIwrE2MHMgLyAxMjEg54K577yJKyBgU2lua2Ag5o6l5Y+j77yM54WnIGBwa2cvZXZlbnRsb2cvc2luay5nb2Ag55qE5b2i54q244CC6YG/5YWNIG1vbml0b3LihpRldmVudHN0b3JlIOS+nei1luWAkuaMguOAglxuLSBgc2VydmVyL2FyZ3VzX3NpbmdsZS9wa2cvbW9uaXRvci9zbGljZV9idWZmZXIuZ29gIOKAlCDnp5Lnuqfnjq/lvaLnvJPlhrLvvIgyNDAg56eS5a656YeP77yJKyBgUHJpY2VNb25pdG9yYCDmjqXnur/vvJpgb2JzZXJ2ZVNsaWNlUXVvdGVgIC8gYG9ic2VydmVTbGljZUJpbmFuY2VgIC8gYGFybVNsaWNlYCAvIGB0YWtlRHVlU2xpY2VzYCAvIGBydW5TbGljZUZsdXNoZXJg44CCYHNsaWNlTXVgIOaYryoq5Y+25a2Q6ZSBKirvvIzojrflj5bml7bkuI3mjIHmnIkgYG11YC9gc2lnbmFsTXVg77yM5LiN5byV5YWl5paw6ZSB5bqP44CCXG4tIGBzZXJ2ZXIvYXJndXNfc2luZ2xlL3BrZy9ldmVudHN0b3JlL3JldGVudGlvbi5nb2Ag4oCUIDkwIOWkqea7muWKqOa4heeQhu+8jOWIhuaJuSBERUxFVEXvvIzlj6rmuIUgYHNpZ25hbF9zbGljZWDjgIJcbi0g5rWL6K+V77yaYHBrZy9tYXJrZXRzbGljZS9tYXJrZXRzbGljZV90ZXN0LmdvYOOAgWBwa2cvbW9uaXRvci9zbGljZV9idWZmZXJfdGVzdC5nb2DjgIFgcGtnL2V2ZW50c3RvcmUvc2xpY2VfdGVzdC5nb2DjgIFgcGtnL2V2ZW50c3RvcmUvc2xpY2VfaW50ZWdyYXRpb25fdGVzdC5nb2DjgIFgc2VydmVyL3NlcnZpY2UvYXJndXNfZXZlbnQvc2xpY2VfdGVzdC5nb2DjgIJcblxuIyMjIOS/ruaUuVxuLSBgcGtnL21vbml0b3IvZGV2X3NhbXBsZXIuZ29gIOKAlCBgZGV2U2FtcGxlSW50ZXJ2YWxgOiBgdGltZS5NaW51dGVgIOKGkiBgMTAgKiB0aW1lLlNlY29uZGDjgIJgRGV2U2FtcGxlVGhyZXNob2xkc0JwYCDkuZ3kuKrpmIjlgLzkuI4gYERldkNyb3NzL0Rldk92ZXJgIOe7k+aehOacquaUueS4gOWtl+OAglxuLSBgcGtnL21vbml0b3IvcHJpY2VfbW9uaXRvci5nb2Ag4oCUIOS4ieS4quWIh+eJh+Wtl+autSArIOaehOmAoOWIneWni+WMlu+8m2BTdGFydCgpYCDotbcgZmx1c2hlcu+8m+W4geWuiSBoYW5kbGVyIOWcqCBgbXVgIOino+mUgeWQjuiQveanve+8m+S/oeWPtyBoYW5kbGVyIOWcqCBgc2lnbmFsTXVgICoq5LmL5YmNKiog6JC95qe944CBYGZpcmVgIOWQju+8iOmUgeWklu+8ieeZu+iusOWIh+eJh+OAglxuLSBgcGtnL2V2ZW50c3RvcmUvbW9kZWwuZ29gIOKAlCDmlrDlop4gYFNpZ25hbFNsaWNlYO+8iOihqCBgc2lnbmFsX3NsaWNlYO+8jOWUr+S4gOmUriBgKGluc3RhbmNlX2tleSwgaW5zdHJ1bWVudCwgdHMpYO+8ie+8jOi/myBgTW9kZWxzKClg44CCXG4tIGBwa2cvZXZlbnRzdG9yZS9jb252ZXJ0LmdvYCAvIGBzdG9yZS5nb2AgLyBgcnVudGltZS5nb2Ag4oCUIGBFbnZlbG9wZS5TbGljZWAg5YiG5rWBICsgYENvbnZlcnRTbGljZWDvvJtgRW1pdFNsaWNlYCDlpI3nlKjlkIzkuIDpmJ/liJfkuI7og4zljovvvIxgc2lnbmFsX3NsaWNlYCDnlKjni6znq4sgYHNsaWNlQmF0Y2hTaXplPTIwYO+8iOWNleihjCB+NEtC77yJ77ybYFNldHVwYCDms6jlhowgc2luayDlubblkK/liqjmuIXnkIbjgIJcbi0gYHNlcnZlci9zZXJ2aWNlL2FyZ3VzX2V2ZW50L3tzbGljZS5nbyxkdG8vZHRvLmdvLHNlcnZpY2UuZ28scmVwb3NpdG9yeS9yZXBvc2l0b3J5LmdvfWAg4oCUIGBzaWduYWxfc2xpY2VgIOS8mOWFiOS+m+aVsO+8jOWPluS4jeWIsOaJjemZjee6p+WbniBgc3RyYXRlZ3lfZXZlbnRgIOeogOeWj+ingua1i+eCue+8m+aWsOWiniBgYmluTGFzdGAgLyBgc2xpY2VBbmNob3JUc2AgLyBgYW5jaG9yTGFnU2VjYCAvIGBkY1BvaW50c2AgLyBgYmluUG9pbnRzYOOAglxuLSBgY2xpZW50L21hbmFnZXIvLi4uL2FyZ3VzLW1hcmtldC9hcGkvYXJndXMtbWFya2V0LmFwaS50c2Ag4oCUIOWQjOatpeaooeWei+Wtl+auteOAglxuLSBgcGtnL21vbml0b3IvZGV2X3NhbXBsZV93aXJpbmdfdGVzdC5nb2Ag4oCUIOaXtumXtOWBj+enu+aUueaIkOebuOWvuSBgZGV2U2FtcGxlSW50ZXJ2YWxg77yI5Y6f5p2l5YaZ5q27IDMwcy82MXPvvIzmlLkgMTBzIOWQjua1i+ivleS8muWkseWOu+aEj+S5ie+8ieOAglxuXG4qKuS4pOWkhOWBj+emu+S7u+WKoeivtOaYjuWtl+mdoueahOWIpOaWrSoq77yI55CG55Sx5Zyo6K6+6K6h5paH5qGjIMKnMeOAgcKnMy4377yJ77ya4pGgIFwi6Kem5Y+R5pe2IGZsdXNoXCIg5a6e546w5Li6XCLop6blj5Hml7blrprmoLwgKyA2MCDnp5LlkI7okL3nm5hcIuKAlOKAlOWPs+WNiueql+WcqOinpuWPkemCo+S4gOWIu+i/mOayoeWPkeeUn++8m+KRoSDor7vkvqfkuIDlubbmjqXkuIrvvIzlm6DkuLogYHNsaWNlLmdvYCDnmoTms6jph4rlt7Lnu4/miopcInIzIOiQveWcsOWQjuaNoiB0aWNrU291cmNlXCLlhpnmiJDkuobmnKzku7vliqHnmoTmlLblsL7liqjkvZzvvIzlj6rlu7rooajkuI3mjqXor7vnrYnkuo7kuqTku5jkuIDlvKDmsqHkurror7vnmoTooajjgIJcblxu5Y+m5aSW5Y+R546w77yaKirkuovku7YgdHMg4omgIOinpuWPkeeerOmXtCoq77yIYHRyYWRlLnNpZ25hbC5kZWxheV9zZWNvbmRzYCDpu5jorqQgNSDnp5IgKyDkuIvljZXlvoDov5TvvInvvIzmiYDku6XliIfniYflhbPogZTnlKhcIuWbnueciyAzMCDnp5Llj5bmnIDov5HplJrngrlcIu+8jOS4lCBwb2ludHMg5Lul5YiH54mH6ZSa54K55Li65Y6f54K544CC6L+Z5piv5pys6L2u5pyA5a655piT5YGa6ZSZ55qE5LiA5aSE44CCXG5cbiMjIOaAjuS5iOmqjOeahFxuXG5gYGBcbmFyZ3VzX3NpbmdsZTogZ28gYnVpbGQgLi8uLi4gKyBnbyB0ZXN0IC4vLi4uICAgICAgICAgICAgICAgICAgICDlhajpg6ggb2tcbmFyZ3VzX3NpbmdsZTogZ28gdGVzdCAtcmFjZSAuL3BrZy97bW9uaXRvcixldmVudHN0b3JlLG1hcmtldHNsaWNlfSAg5YWo6YOoIG9rXG5zZXJ2aWNlOiAgICAgIGdvIGJ1aWxkIC4vLi4uICsgZ28gdGVzdCAuLy4uLiAgICAgICAgICAgICAgICAgICAg5YWo6YOoIG9rXG5tYW5hZ2VyLWFwaTogIGdvIHZldCAuLy4uLiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgb2tcbmNvbW1vbjogICAgICAgZ28gYnVpbGQgKyBnbyB0ZXN0ICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICBva1xuY2xpZW50L21hbmFnZXI6IG5weCB0c2MgLS1ub0VtaXQgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIOaXoOi+k+WHuu+8iOmAmui/h++8iVxuZ29mbXQ6ICAgICAgICDmlrDlop4v5pS55Yqo5paH5Lu25YWo6YOo5bmy5YeA77yI5LuT5bqT5pei5pyJ55qEIENKSyDlr7npvZDmvILnp7vmnKrliqjvvIlcbmBgYFxuXG7mnKrpgJrov4fpobnvvJpgc2VydmVyL29yYWNsZWAgYGdvIGJ1aWxkYCDmiqUgYG1pc3NpbmcgZ28uc3VtIGVudHJ5IGZvciBnaXRodWIuY29tL2dvLXJlZGlzL3JlZGlzYOOAguacrOi9ruacquaUueWKqCBgc2VydmVyL29yYWNsZWAg5LiOIGBzZXJ2ZXIvY29tbW9uYO+8iOS4pOiAheWcqOacrOi9ruW8gOWni+WJjeeahCBgZ2l0IHN0YXR1c2Ag6YeM5bey5pivIG1vZGlmaWVk77yJ77yM5piv5YWI6KGM5Lu75Yqh55qE6YGX55WZ5L6d6LWW54q25oCB77yM6ZyA6KaB5pyJ5Lq66KGlIGBnbyBtb2QgdGlkeWDjgIJcblxuIyMg5a+554Wn6aqM5pS25qCH5YeG77yI6ZyA5rGC5aSn57qyIMKnNi4xIOS4reS4juacrOS7u+WKoeebuOWFs+eahOS4pOadoe+8iVxuXG4tICoqwqc2LjEuMTAg5a656YeP6L6+5qCH77yI5YiH54mH5pel5aKeIOKJpDEuNU1CIOS4lCA5MCDlpKnlpJboh6rliqjmuIXnkIbvvIkqKu+8muW3sua7oei2s+WunueOsOKAlOKAlOWNleihjCB+NEtCIMOXIOavj+asoeinpuWPkeS4gOadoe+8iOWunua1iyA2MTMg5p2hLzQg5aSpIOKJiCAxNTAg5p2hL+WkqSDiiYggMC42TUIv5aSp77yJ77ybYFNsaWNlUmV0ZW50aW9uID0gOTAg5aSpYCDmnInliIbmibnmuIXnkIYgZ29yb3V0aW5l77yM6L6555WM55SxIGBUZXN0SW50ZWdyYXRpb25TbGljZVB1cmdlUmVzcGVjdHNSZXRlbnRpb25gIOimhuebluOAguaXpeWinuWunuaVsOmcgOS4iue6v+WQjuWunua1i+OAglxuLSAqKsKnNi4xLjEgLyDop6blj5Hnnqzpl7Tlj6/kuIvpkrsqKu+8muivu+S+p+W3suiDvei/lOWbniAxMjEg54K56YCQ56eS5bqP5YiX77yI5ZCr5biB5a6JIGxhc3Qg5LiO546w566X55qEIDEyMSDngrkgZ2FwQnDvvInjgILliY3nq6/mir3lsYnlsZ4gcjEy77yM5pys6L2u5Y+q5ZCM5q2l5LqG5pWw5o2u5qih5Z6L77yM5omA5Lul6L+Z5p2hKirpg6jliIbmu6HotrMqKuKAlOKAlOWQjuerr+Wlkee6puW3suWwseS9jeOAglxuLSAqKs67KM64KSDlj6PlvoTkuI3lj5jvvIjCpzMuMyDpopHnjofnuqflm57mtYvllK/kuIDmlbDmja7mupDvvIkqKu+8mua7oei2s+OAguepv+i2iuaAu+aVsOS4jemaj+eql+WPo+Wkp+Wwj+WPmOWMlu+8iGBsYXN0RGVyaXZlZGAg6Leo56qX5Y+j5L+d55WZ77yJ77yM6K+75L6nIGBsYW1iZGEuZ28gd2luZG93U3RlcGAg5Y+W55u46YK76Ze06ZqU5Lit5L2N5pWw44CB5bey6aKE55WZ5re35ZCI5q2l6ZW/5pSv5oyB44CCXG4tICoq5YaZ5bqT5aSx6LSl5LiN6Zi75pat5Lqk5piT77yIwqczLjbvvIkqKu+8mua7oei2s+OAgmBFbWl0U2xpY2VgIOmdnumYu+WhnuOAgemYn+WIl+a7oeWNs+S4ouW5tuiuoeaVsO+8m+a4heeQhuaYr+eLrOeriyBnb3JvdXRpbmXjgIJcblxuIyMg5rWL6K+V5Lqk5o6l5L+h5oGvXG5cbi0gKirljZXmtYsqKu+8muaXoOmcgOeOr+Wig++8jGBjZCBzZXJ2ZXIvYXJndXNfc2luZ2xlICYmIGdvIHRlc3QgLi8uLi5g44CBYGNkIHNlcnZlci9zZXJ2aWNlICYmIGdvIHRlc3QgLi8uLi5g44CCXG4tICoq6ZuG5oiQ5rWL6K+VKirvvIjpnIDopoHkuIDkuKoqKuWPr+S4ouW8gyoq55qEIE15U1FMIHNjaGVtYe+8jOe7neS4jeiDveaMh+eUn+S6p+W6k++8jOWunuS+i+mUruW3sueUqCBgaXQtYCDliY3nvIDpmpTnprvvvInvvJpcbiAgLSBgRVZFTlRTVE9SRV9URVNUX0RTTj0uLi4gZ28gdGVzdCAuL3BrZy9ldmVudHN0b3JlLyAtcnVuIFRlc3RJbnRlZ3JhdGlvblNpZ25hbFNsaWNlIC12YO+8iOWOu+mHjSArIHRzIOWimemSn+WPo+W+hO+8iVxuICAtIGBFVkVOVFNUT1JFX1RFU1RfRFNOPS4uLiBnbyB0ZXN0IC4vcGtnL2V2ZW50c3RvcmUvIC1ydW4gVGVzdEludGVncmF0aW9uU2xpY2VQdXJnZSAtdmDvvIg5MCDlpKnovrnnlYzvvIlcbiAgLSBgQVJHVVNfRVZFTlRfVEVTVF9EU049Li4uIGdvIHRlc3QgLi9hcmd1c19ldmVudC8gLXJ1biBUZXN0SW50ZWdyYXRpb25TbGljZSAtdmDvvIhzaWduYWxfc2xpY2Ug5L6b5pWwICsg6ZmN57qn6Lev5b6E77yJXG4tICoq56uv5Yiw56uvKirvvIjpnIDnnJ/lrp4gV1MgKyBNeVNRTO+8jOacrOi9ruaXoOazleWcqOW8gOWPkeacuumqjOivge+8ie+8mui1tyBhcmd1c19zaW5nbGUg4oaSIOaXpeW/l+W6lOWHuueOsCBg56eS57qn5YiH54mH5YWlIHNpZ25hbF9zbGljZe+8jOS/neeVmSA5MCDlpKlgIOKGkiDnrYnkuIDmrKHnnJ/lrp7op6blj5HvvIw2MCDkvZnnp5LlkI4gYFNFTEVDVCB0cywgZGNfcG9pbnRzLCBiaW5fcG9pbnRzLCBKU09OX0xFTkdUSChkY19sYXN0X2pzb24pIEZST00gc2lnbmFsX3NsaWNlYCDlupTop4EgYDEyMSAvIOaOpei/kTEyMSAvIDEyMWAg4oaSIOiwgyBgR0VUIC9hcGkvYXJndXMtZXZlbnQvc2lnbmFscy97aWR9L3NsaWNlYO+8jGB0aWNrU291cmNlPXNpZ25hbF9zbGljZWDjgIFgdGlja0NvbXBsZXRlPXRydWVg44CBMTIxIOeCueOAgWBiaW5MYXN0YCDpnZ7nqbog4oaSIGBTRUxFQ1QgQ09VTlQoKikgRlJPTSBkZXZfc2FtcGxlIFdIRVJFIHRzID49IENVUkRBVEUoKWAg5pel5aKe5o6l6L+RIDg2NDDjgIJcbi0gKirmuIXnkIYqKu+8mumbhuaIkOeUqOS+i+iHquW3sSBgREVMRVRFIC4uLiBXSEVSRSBpbnN0YW5jZV9rZXkgSU4gKCdpdC0uLi4nKWDvvJvooajnlLEgYEF1dG9NaWdyYXRlYCDoh6rliqjliJvlu7rvvIzml6DmiYvlt6UgRERM44CCXG5cbiMjIOmBl+eVmeS4jumjjumZqVxuXG4xLiAqKuWIh+eJh+S4juS6i+S7tueahOWFs+iBlOaYr+WQr+WPkeW8j+eahCoq77ya5LqL5Lu26KGM5rKh5pyJ6ZSa54K55a2X5q6177yM5Y+q6IO9XCLlm57nnIsgMzAg56eS5Y+W5pyA6L+R6ZSa54K5XCLjgILlkIzkuIDnsIfph4wgMjAg56eS5YaF6L+e552A5Lik5qyh6Kem5Y+R5pe277yM5pma5Yiw55qE5oiQ5Lqk5LqL5Lu25Y+v6IO95b2S5Yiw5ZCO5LiA5Liq6ZSa54K577yI5Lik5p2h5YiH54mH56qX5Y+j6auY5bqm6YeN5Y+g77yM5Zu+5b2i57uT6K665LiN5Y+X5b2x5ZON77yJ44CC6KaB57K+56Gu5YWz6IGU6ZyA5Zyo5Z+L54K55LiK57uZ5LqL5Lu25bim6ZSa54K55pe26Ze05oizIOKAlOKAlCDlu7rorq7orrDov5sgcjEz44CCXG4yLiAqKuWIh+eJh+ayoeacieWFnOW6leecn+a6kCoq77ya5LiN6L+bIEpTT05M77yI55CG55Sx6KeB6K6+6K6h5paH5qGjIMKnMy4177yJ77yM5Lii5LqG5bCx5piv5rC45LmF57y65aSx77yMYFN0YXRzKCkuRHJvcHBlZC9GYWlsZWRgIOaYr+WUr+S4gOe6v+e0ouOAglxuMy4gKipgZGNfcG9pbnRzYCDnmoTmraPluLjlgLzkuI3mmK8gMTIxKirvvJrlronpnZnnmoTpgqPlh6Dnp5IgRGVlcENvaW4g5Y+v6IO95rKh5pyJIGBQT2Ag5o6o6YCB77yM6KaG55uW546H5aSp54S25L2O5LqO5ruh5YC844CC5YmN56uv55S75Zu+6KaB55yL6KaG55uW546H77yM5Yir5YGH5a6aIDEyMSDngrnpg73mnInlgLzvvJvlkYrorabpmIjlgLzpnIDkuIrnur/lkI7lrp7mtYvlho3lrprjgIJcbjQuICoq6YWN572u54Ot5pu/5o2i5Lya5Lqn5Ye65pyq5ruh56qX5YiH54mHKirvvJpgUmVsb2FkTW9uaXRvcmAg5YGc5penIG1vbml0b3Ig5pe25oyJIGBmb3JjZWAg5aaC5a6e5Lqk5Ye677yI5q+U5pW05om55Lii5o6J5aW977yJ77yM6KaG55uW546H5a2X5q615Lya5pq06Zyy57y65Y+j44CCXG41LiAqKjkwIOWkqea4heeQhueUseavj+S4quWunuS+i+WQhOiHqui3kSoq77yaREVMRVRFIOW5guetieaXoOWus++8jOS9huWunuS+i+abtOWkmuaXtuWAvOW+l+aMquWIsCBtYW5hZ2VyLWFwaSDljZXngrnmiafooYzjgIJcbjYuICoqcjEyIOeahOWIh+eJh+aKveWxieacquW7uioq77ya5pys6L2u5Y+q5ZCM5q2l5LqG5YmN56uv5pWw5o2u5qih5Z6L44CCXG5cbuaJuemHj+WIpOWumu+8mumcgOS6uuW3peWkhOeQhlxuXG7vvIjljp/lm6DvvJpgc2VydmVyL29yYWNsZWAg55qEIGBnbyBidWlsZGAg5ZugIGBnby5zdW1gIOe8uiBgZ2l0aHViLmNvbS9nby1yZWRpcy9yZWRpc2Ag5p2h55uu6ICM5aSx6LSl44CC6K+l5qih5Z2X5LiOIGBzZXJ2ZXIvY29tbW9uL21pZGRsZXdhcmUvcmVkaXMvQXJndXNDb25maWcuZ29gIOWcqOacrOi9ruW8gOWni+WJjeWwseW3suaYr+S/ruaUueaAgeOAgeacrOi9ruacquinpueisO+8jOWxnuWFiOihjOS7u+WKoemBl+eVmeeahOS+nei1lumXrumimO+8jOmcgOimgeacieS6uuaJp+ihjOS4gOasoSBgZ28gbW9kIHRpZHlg44CC5pys5Lu75Yqh6Ieq6Lqr55qE5YWo6YOo5pS55Yqo5bey5ZyoIGFyZ3VzX3NpbmdsZSAvIHNlcnZpY2UgLyBtYW5hZ2VyLWFwaSAvIGNvbW1vbiDlm5vkuKrmqKHlnZfkuIrnvJbor5HkuI7mtYvor5XpgJrov4fjgILvvIkiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6ImZpbmFsX2Fuc3dlciJ9XX1dfQ==
-->
