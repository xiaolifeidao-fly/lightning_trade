---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r2-a0179c42fe"
requirementKey: "req-1788169552180"
resourceName: "trade_kline 增加平台与多币种维度，打通 DC/币安双源多周期回填"
conversationTitle: "trade_kline 增加平台与多币种维度，打通 DC/币安双源多周期回填"
threadId: "aaac06f3-94e8-42b0-b7d8-5d5a08ab5bfd"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-01T09:39:59.117209+00:00"
---

# 任务聊天 · trade_kline 增加平台与多币种维度，打通 DC/币安双源多周期回填

## 第 1 轮 · completed · Q8jP6U3ZSWStTKZxjYJ6_g

### 用户

执行「动作执行」阶段：trade_kline 增加平台与多币种维度，打通 DC/币安双源多周期回填

### 助手

I'll start by loading the delivery execution skill and reading the requirement doc.

### 助手

I'll read the requirement doc and project skills first.

### 助手

Requirement doc `doc/module/r2-a0179c42fe/文档.md` does not exist. I'll proceed from the task description (which is detailed and cites real file paths) and note the gap. Loading the backend skill:

### 助手

Now implementing. First the model:

### 助手

Now the repository layer:

### 助手

Now the remaining service callers and handlers:

### 助手

Updating the base DDL to match, then writing the design/migration doc:

### 助手

Now the design doc:

### 助手

Final verification pass across modules:

### 助手

交付完成。

## 1. 改了什么

**前提说明**：`doc/module/r2-a0179c42fe/文档.md` 不存在（梳理需求阶段未产出该文件）。实现以任务说明列出的范围 + `server/` 现状代码为准，并把实际落地的设计写进了 [doc/module/r2-a0179c42fe/design/trade-kline-平台维度设计.md](doc/module/r2-a0179c42fe/design/trade-kline-平台维度设计.md)。

**数据模型**
- `server/service/trade/repository/model.go:66` — `TradeKline` 新增 `PlatformCode`（`varchar(32) NOT NULL DEFAULT 'binance'`），唯一索引由 `idx_symbol_interval_open(symbol,interval,open_time)` 改为 `idx_kline_platform_dim(platform_code,symbol,interval,open_time)`，另加 `idx_platform_code` / `idx_symbol`。
- `docs/coin_domain_tables.sql:376` — 基线建表语句同步更新。

**Repository（四个方法加平台过滤 + upsert 冲突列）**
- `server/service/trade/repository/repository.go:186` — `EnsureTable()` 追加 `migrateKlinePlatformDim()`：先把 `platform_code` 为空/NULL 的历史行统一回填 `binance`，再删旧唯一索引 `idx_symbol_interval_open`（不删的话第二个平台会被旧唯一键挡住）。
- 新增 `KlinePlatformBinance` / `NormalizeKlinePlatform`（trim+lower，空值兜底 `binance`）/ `scopeKlinePlatform`。
- `ListBySymbolInterval` / `ListBySymbolIntervalTimeRange` / `LatestKline` / `CountBySymbolIntervalRange` 全部首参加 `platform`。
- `UpsertKlines` 冲突列改为 `(platform_code, symbol, interval, open_time)`，入库前归一 `platform_code`。

**回填链路四个入口透传（`server/service/trade/kline_backfill.go`）**
- `BackfillKlines`（请求体 `platformCode`）、`fetchAndStoreRecentKlines`（同一 platform 同时用于 `GetKlinesByPlatform` 与入库）、`ensureBacktestKlines`（`run.PlatformCode`）、`ListKlinesInRange`（新增 platform 入参）；`klinesToModels` 落 `PlatformCode`。
- 新增 `BackfillKlinesBatch`：**平台 × 币种 × 周期** 笛卡尔积批量回填，周期缺省 `["1m","5m","1h","1d"]`、平台缺省 `["binance"]`，组合数上限 200，单组合失败只记 `error` 继续跑完。

**行情源（补的一处必要缺口）**
- `server/argus_single/pkg/trade/market_data.go:301` — DeepCoin 的 `bar` 是 OKX 口径（分钟小写、小时/天/周大写）。原来 `1h`/`1d` 原样透传会被服务端判为非法周期，**DC 侧 1h/1d 根本回不了**。新增 `deepcoinMarketBar()` 做映射，未知周期仍原样透传。币安分支不变。

**接口（manager-api）**
- `POST /klines/backfill` — 改绑 `BatchBackfillKlineDTO`，新增 `platformCodes/symbols/intervals`，兼容老的单值 `platformCode/symbol/interval`；响应改为 `BackfillKlineBatchResultDTO{total,succeeded,failed,items[]}`。
- `GET /klines/range`、`GET /trade-klines` — 新增 `platformCode` 查询参数；`TradeKlineDTO`、`BackfillKlineResultDTO` 响应补 `platformCode`。

**其余调用点**：`backtest.go`（2 处）、`trade_service.go`（`ListKlines` + `listSimulationKlinesFromDB` 两个调用点）全部按已有的 `run.PlatformCode` / `platformCode` 透传。

**前端**：`client/manager/.../trade-backtest-runs` 的「K线详情」弹窗改为透传 `run.platformCode`（`BacktestRun` 上已有该字段）。

**新增测试**：`server/service/trade/kline_backfill_test.go`、`market_data_test.go` 加 `TestMarketDataClientDeepcoinBarInterval`。

## 2. 怎么验的

| 命令 | 结果 |
| --- | --- |
| `go build ./...`（common / service / oracle） | OK |
| `go build ./pkg/trade/... ./pkg/strategy/...`（manager-api） | OK |
| `go vet ./pkg/trade/... ./pkg/strategy/...` + `go vet ./trade/...` | OK |
| `go test ./trade/...`（service） | ok，新增 8 个子用例全过 |
| `go test ./pkg/trade/`（argus_single） | ok，含新增 bar 映射用例 |
| `gofmt -l` 我改的所有文件 | 无输出 |

**未通过但与本任务无关**：`manager-api` 与 `argus_single` 整模块构建失败，全部落在 `argus_config` / `runtimeconfig` / `common/middleware/redis/ArgusConfig.go` —— 这些是本会话期间**另一个并发任务**在工作区里改的未提交代码（给 Redis 配置快照加 `instanceKey` 参数，改到一半）。我一行都没碰这些文件，`git status` 可核对。

**未跑**：前端 `tsc --noEmit` —— `client/manager/node_modules` 未安装，环境里跑不了。改动只是给已有 `BacktestRun.platformCode` 字段做透传 + 给 `fetchKlineRange` 参数加可选 `platformCode?: string`，类型面很小。

## 3. 对照验收标准

需求文档缺失，按任务说明的「范围」逐条核对：

| 条目 | 状态 |
| --- | --- |
| TradeKline 加 platform_code 并入唯一键 | 已满足 |
| repository 四个方法加平台过滤 | 已满足（另含 `UpsertKlines` 冲突列） |
| kline_backfill.go 四个入口透传 | 已满足 |
| manager-api 三个接口补参数 | 已满足 |
| 复用 `market_data.go` 的 `GetKlinesByPlatform` | 已满足（未新增适配层，仅修 DC bar 口径） |
| 历史行回填 `platform_code=binance` | 已满足（`EnsureTable` 自动执行，SQL 也写进设计文档） |
| 1m/5m/1h/1d 批量回填、多币种 | 已满足（`BackfillKlinesBatch`） |
| 不引入新时序数据库、不加实例维度 | 已遵守 |

## 4. 测试交接信息

- **前置**：服务启动会走 `TradeService.EnsureTables()` 自动迁移。先确认 `trade_kline` 已有 `platform_code` 列、`idx_kline_platform_dim` 存在且 `idx_symbol_interval_open` 已消失；手工 SQL 见设计文档第 3 节。
- **无需密钥**：`GetKlinesByPlatform` 走公开行情接口，不加载账号会话。需要外网可达 `fapi.binance.com` 与 `api.deepcoin.com`。
- **执行顺序**：
  1. `POST /klines/backfill` body `{"platformCodes":["binance","deepcoin"],"symbols":["BTCUSDT","ETHUSDT"],"intervals":["1m","5m","1h","1d"],"limit":500}` → 期望 `total=16`，`failed=0`。
  2. 查库 `SELECT platform_code, interval, count(*) FROM trade_kline WHERE symbol='BTCUSDT' GROUP BY 1,2` → 期望 8 组，两个平台的同一 `open_time` 各存一行且不互相覆盖。
  3. 重复执行步骤 1 → 幂等，行数不增（`upserted` 只刷行情值）。
  4. `GET /trade-klines?platformCode=deepcoin&symbol=BTCUSDT&interval=1h`、`GET /klines/range?platformCode=deepcoin&...` → 只返回 DC 那份；不传 `platformCode` 时返回币安那份。
  5. 老请求体 `{"platformCode":"binance","symbol":"BTCUSDT","interval":"1m","limit":100}` 仍可用（`total=1`）。
- **清理**：`DELETE FROM trade_kline WHERE platform_code='deepcoin'` 即可回到单源状态，不影响历史币安数据。

## 5. 遗留与风险

- **需求文档缺失**：`doc/module/r2-a0179c42fe/文档.md` 不存在，本轮未代写（该文件归梳理需求阶段所有）。设计说明已单独落在 `design/` 目录。
- **并发改动干扰**：工作区里有另一个任务未完成的 `argus_config` instanceKey 改造，导致 `manager-api` / `argus_single` 整模块暂时构建失败。**这会挡住测试阶段启服务**，需等那条任务收尾。
- **DeepCoin 单次返回上限 1000 根**（币安 1500），批量回填 `limit>1000` 时 DC 侧实际只会拿到 1000 根，`fetched` 数会小于 `needFetch`，属预期而非错误。
- **`/klines/backfill` 响应结构变了**（单对象 → 批量汇总）。当前前端无调用方，如有外部脚本对接需同步。
- **前端类型检查未跑**（无 node_modules），建议在有依赖的环境里补一次 `tsc --noEmit`。
- **DC 与币安的 K 线 open_time 对齐**未做校验：两所同周期 K 线的时间戳理论上一致，若 DC 返回带偏移的时间戳，同一 `open_time` 的跨源对比会错位——建议测试阶段抽查一组 1h 数据比对时间戳。

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJROGpQNlUzWlNXU3RUS1p4allKNl9nIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMVQwOTozMDo0OC45MDQzOTUrMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDFUMDk6Mzk6NTguNjI0MzkxKzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8mnRyYWRlX2tsaW5lIOWinuWKoOW5s+WPsOS4juWkmuW4geenjee7tOW6pu+8jOaJk+mAmiBEQy/luIHlronlj4zmupDlpJrlkajmnJ/lm57loasifV19LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIHN0YXJ0IGJ5IGxvYWRpbmcgdGhlIGRlbGl2ZXJ5IGV4ZWN1dGlvbiBza2lsbCBhbmQgcmVhZGluZyB0aGUgcmVxdWlyZW1lbnQgZG9jLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IkknbGwgcmVhZCB0aGUgcmVxdWlyZW1lbnQgZG9jIGFuZCBwcm9qZWN0IHNraWxscyBmaXJzdC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJSZXF1aXJlbWVudCBkb2MgYGRvYy9tb2R1bGUvcjItYTAxNzljNDJmZS/mlofmoaMubWRgIGRvZXMgbm90IGV4aXN0LiBJJ2xsIHByb2NlZWQgZnJvbSB0aGUgdGFzayBkZXNjcmlwdGlvbiAod2hpY2ggaXMgZGV0YWlsZWQgYW5kIGNpdGVzIHJlYWwgZmlsZSBwYXRocykgYW5kIG5vdGUgdGhlIGdhcC4gTG9hZGluZyB0aGUgYmFja2VuZCBza2lsbDoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgaW1wbGVtZW50aW5nLiBGaXJzdCB0aGUgbW9kZWw6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSByZXBvc2l0b3J5IGxheWVyOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgcmVtYWluaW5nIHNlcnZpY2UgY2FsbGVycyBhbmQgaGFuZGxlcnM6Iiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiVXBkYXRpbmcgdGhlIGJhc2UgRERMIHRvIG1hdGNoLCB0aGVuIHdyaXRpbmcgdGhlIGRlc2lnbi9taWdyYXRpb24gZG9jOiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgZGVzaWduIGRvYzoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJGaW5hbCB2ZXJpZmljYXRpb24gcGFzcyBhY3Jvc3MgbW9kdWxlczoiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiLkuqTku5jlrozmiJDjgIJcblxuIyMgMS4g5pS55LqG5LuA5LmIXG5cbioq5YmN5o+Q6K+05piOKirvvJpgZG9jL21vZHVsZS9yMi1hMDE3OWM0MmZlL+aWh+ahoy5tZGAg5LiN5a2Y5Zyo77yI5qKz55CG6ZyA5rGC6Zi25q615pyq5Lqn5Ye66K+l5paH5Lu277yJ44CC5a6e546w5Lul5Lu75Yqh6K+05piO5YiX5Ye655qE6IyD5Zu0ICsgYHNlcnZlci9gIOeOsOeKtuS7o+eggeS4uuWHhu+8jOW5tuaKiuWunumZheiQveWcsOeahOiuvuiuoeWGmei/m+S6hiBbZG9jL21vZHVsZS9yMi1hMDE3OWM0MmZlL2Rlc2lnbi90cmFkZS1rbGluZS3lubPlj7Dnu7Tluqborr7orqEubWRdKGRvYy9tb2R1bGUvcjItYTAxNzljNDJmZS9kZXNpZ24vdHJhZGUta2xpbmUt5bmz5Y+w57u05bqm6K6+6K6hLm1kKeOAglxuXG4qKuaVsOaNruaooeWeiyoqXG4tIGBzZXJ2ZXIvc2VydmljZS90cmFkZS9yZXBvc2l0b3J5L21vZGVsLmdvOjY2YCDigJQgYFRyYWRlS2xpbmVgIOaWsOWiniBgUGxhdGZvcm1Db2RlYO+8iGB2YXJjaGFyKDMyKSBOT1QgTlVMTCBERUZBVUxUICdiaW5hbmNlJ2DvvInvvIzllK/kuIDntKLlvJXnlLEgYGlkeF9zeW1ib2xfaW50ZXJ2YWxfb3BlbihzeW1ib2wsaW50ZXJ2YWwsb3Blbl90aW1lKWAg5pS55Li6IGBpZHhfa2xpbmVfcGxhdGZvcm1fZGltKHBsYXRmb3JtX2NvZGUsc3ltYm9sLGludGVydmFsLG9wZW5fdGltZSlg77yM5Y+m5YqgIGBpZHhfcGxhdGZvcm1fY29kZWAgLyBgaWR4X3N5bWJvbGDjgIJcbi0gYGRvY3MvY29pbl9kb21haW5fdGFibGVzLnNxbDozNzZgIOKAlCDln7rnur/lu7rooajor63lj6XlkIzmraXmm7TmlrDjgIJcblxuKipSZXBvc2l0b3J577yI5Zub5Liq5pa55rOV5Yqg5bmz5Y+w6L+H5rukICsgdXBzZXJ0IOWGsueqgeWIl++8iSoqXG4tIGBzZXJ2ZXIvc2VydmljZS90cmFkZS9yZXBvc2l0b3J5L3JlcG9zaXRvcnkuZ286MTg2YCDigJQgYEVuc3VyZVRhYmxlKClgIOi/veWKoCBgbWlncmF0ZUtsaW5lUGxhdGZvcm1EaW0oKWDvvJrlhYjmioogYHBsYXRmb3JtX2NvZGVgIOS4uuepui9OVUxMIOeahOWOhuWPsuihjOe7n+S4gOWbnuWhqyBgYmluYW5jZWDvvIzlho3liKDml6fllK/kuIDntKLlvJUgYGlkeF9zeW1ib2xfaW50ZXJ2YWxfb3BlbmDvvIjkuI3liKDnmoTor53nrKzkuozkuKrlubPlj7DkvJrooqvml6fllK/kuIDplK7mjKHkvY/vvInjgIJcbi0g5paw5aKeIGBLbGluZVBsYXRmb3JtQmluYW5jZWAgLyBgTm9ybWFsaXplS2xpbmVQbGF0Zm9ybWDvvIh0cmltK2xvd2Vy77yM56m65YC85YWc5bqVIGBiaW5hbmNlYO+8iS8gYHNjb3BlS2xpbmVQbGF0Zm9ybWDjgIJcbi0gYExpc3RCeVN5bWJvbEludGVydmFsYCAvIGBMaXN0QnlTeW1ib2xJbnRlcnZhbFRpbWVSYW5nZWAgLyBgTGF0ZXN0S2xpbmVgIC8gYENvdW50QnlTeW1ib2xJbnRlcnZhbFJhbmdlYCDlhajpg6jpppblj4LliqAgYHBsYXRmb3JtYOOAglxuLSBgVXBzZXJ0S2xpbmVzYCDlhrLnqoHliJfmlLnkuLogYChwbGF0Zm9ybV9jb2RlLCBzeW1ib2wsIGludGVydmFsLCBvcGVuX3RpbWUpYO+8jOWFpeW6k+WJjeW9kuS4gCBgcGxhdGZvcm1fY29kZWDjgIJcblxuKirlm57loavpk77ot6/lm5vkuKrlhaXlj6PpgI/kvKDvvIhgc2VydmVyL3NlcnZpY2UvdHJhZGUva2xpbmVfYmFja2ZpbGwuZ29g77yJKipcbi0gYEJhY2tmaWxsS2xpbmVzYO+8iOivt+axguS9kyBgcGxhdGZvcm1Db2RlYO+8ieOAgWBmZXRjaEFuZFN0b3JlUmVjZW50S2xpbmVzYO+8iOWQjOS4gCBwbGF0Zm9ybSDlkIzml7bnlKjkuo4gYEdldEtsaW5lc0J5UGxhdGZvcm1gIOS4juWFpeW6k++8ieOAgWBlbnN1cmVCYWNrdGVzdEtsaW5lc2DvvIhgcnVuLlBsYXRmb3JtQ29kZWDvvInjgIFgTGlzdEtsaW5lc0luUmFuZ2Vg77yI5paw5aKeIHBsYXRmb3JtIOWFpeWPgu+8ie+8m2BrbGluZXNUb01vZGVsc2Ag6JC9IGBQbGF0Zm9ybUNvZGVg44CCXG4tIOaWsOWiniBgQmFja2ZpbGxLbGluZXNCYXRjaGDvvJoqKuW5s+WPsCDDlyDluIHnp40gw5cg5ZGo5pyfKiog56yb5Y2h5bCU56ev5om56YeP5Zue5aGr77yM5ZGo5pyf57y655yBIGBbXCIxbVwiLFwiNW1cIixcIjFoXCIsXCIxZFwiXWDjgIHlubPlj7DnvLrnnIEgYFtcImJpbmFuY2VcIl1g77yM57uE5ZCI5pWw5LiK6ZmQIDIwMO+8jOWNlee7hOWQiOWksei0peWPquiusCBgZXJyb3JgIOe7p+e7rei3keWujOOAglxuXG4qKuihjOaDhea6kO+8iOihpeeahOS4gOWkhOW/heimgee8uuWPo++8iSoqXG4tIGBzZXJ2ZXIvYXJndXNfc2luZ2xlL3BrZy90cmFkZS9tYXJrZXRfZGF0YS5nbzozMDFgIOKAlCBEZWVwQ29pbiDnmoQgYGJhcmAg5pivIE9LWCDlj6PlvoTvvIjliIbpkp/lsI/lhpnjgIHlsI/ml7Yv5aSpL+WRqOWkp+WGme+8ieOAguWOn+adpSBgMWhgL2AxZGAg5Y6f5qC36YCP5Lyg5Lya6KKr5pyN5Yqh56uv5Yik5Li66Z2e5rOV5ZGo5pyf77yMKipEQyDkvqcgMWgvMWQg5qC55pys5Zue5LiN5LqGKirjgILmlrDlop4gYGRlZXBjb2luTWFya2V0QmFyKClgIOWBmuaYoOWwhO+8jOacquefpeWRqOacn+S7jeWOn+agt+mAj+S8oOOAguW4geWuieWIhuaUr+S4jeWPmOOAglxuXG4qKuaOpeWPo++8iG1hbmFnZXItYXBp77yJKipcbi0gYFBPU1QgL2tsaW5lcy9iYWNrZmlsbGAg4oCUIOaUuee7kSBgQmF0Y2hCYWNrZmlsbEtsaW5lRFRPYO+8jOaWsOWiniBgcGxhdGZvcm1Db2Rlcy9zeW1ib2xzL2ludGVydmFsc2DvvIzlhbzlrrnogIHnmoTljZXlgLwgYHBsYXRmb3JtQ29kZS9zeW1ib2wvaW50ZXJ2YWxg77yb5ZON5bqU5pS55Li6IGBCYWNrZmlsbEtsaW5lQmF0Y2hSZXN1bHREVE97dG90YWwsc3VjY2VlZGVkLGZhaWxlZCxpdGVtc1tdfWDjgIJcbi0gYEdFVCAva2xpbmVzL3JhbmdlYOOAgWBHRVQgL3RyYWRlLWtsaW5lc2Ag4oCUIOaWsOWiniBgcGxhdGZvcm1Db2RlYCDmn6Xor6Llj4LmlbDvvJtgVHJhZGVLbGluZURUT2DjgIFgQmFja2ZpbGxLbGluZVJlc3VsdERUT2Ag5ZON5bqU6KGlIGBwbGF0Zm9ybUNvZGVg44CCXG5cbioq5YW25L2Z6LCD55So54K5KirvvJpgYmFja3Rlc3QuZ29g77yIMiDlpITvvInjgIFgdHJhZGVfc2VydmljZS5nb2DvvIhgTGlzdEtsaW5lc2AgKyBgbGlzdFNpbXVsYXRpb25LbGluZXNGcm9tREJgIOS4pOS4quiwg+eUqOeCue+8ieWFqOmDqOaMieW3suacieeahCBgcnVuLlBsYXRmb3JtQ29kZWAgLyBgcGxhdGZvcm1Db2RlYCDpgI/kvKDjgIJcblxuKirliY3nq68qKu+8mmBjbGllbnQvbWFuYWdlci8uLi4vdHJhZGUtYmFja3Rlc3QtcnVuc2Ag55qE44CMS+e6v+ivpuaDheOAjeW8ueeql+aUueS4uumAj+S8oCBgcnVuLnBsYXRmb3JtQ29kZWDvvIhgQmFja3Rlc3RSdW5gIOS4iuW3suacieivpeWtl+aute+8ieOAglxuXG4qKuaWsOWinua1i+ivlSoq77yaYHNlcnZlci9zZXJ2aWNlL3RyYWRlL2tsaW5lX2JhY2tmaWxsX3Rlc3QuZ29g44CBYG1hcmtldF9kYXRhX3Rlc3QuZ29gIOWKoCBgVGVzdE1hcmtldERhdGFDbGllbnREZWVwY29pbkJhckludGVydmFsYOOAglxuXG4jIyAyLiDmgI7kuYjpqoznmoRcblxufCDlkb3ku6QgfCDnu5PmnpwgfFxufCAtLS0gfCAtLS0gfFxufCBgZ28gYnVpbGQgLi8uLi5g77yIY29tbW9uIC8gc2VydmljZSAvIG9yYWNsZe+8iSB8IE9LIHxcbnwgYGdvIGJ1aWxkIC4vcGtnL3RyYWRlLy4uLiAuL3BrZy9zdHJhdGVneS8uLi5g77yIbWFuYWdlci1hcGnvvIkgfCBPSyB8XG58IGBnbyB2ZXQgLi9wa2cvdHJhZGUvLi4uIC4vcGtnL3N0cmF0ZWd5Ly4uLmAgKyBgZ28gdmV0IC4vdHJhZGUvLi4uYCB8IE9LIHxcbnwgYGdvIHRlc3QgLi90cmFkZS8uLi5g77yIc2VydmljZe+8iSB8IG9r77yM5paw5aKeIDgg5Liq5a2Q55So5L6L5YWo6L+HIHxcbnwgYGdvIHRlc3QgLi9wa2cvdHJhZGUvYO+8iGFyZ3VzX3NpbmdsZe+8iSB8IG9r77yM5ZCr5paw5aKeIGJhciDmmKDlsITnlKjkvosgfFxufCBgZ29mbXQgLWxgIOaIkeaUueeahOaJgOacieaWh+S7tiB8IOaXoOi+k+WHuiB8XG5cbioq5pyq6YCa6L+H5L2G5LiO5pys5Lu75Yqh5peg5YWzKirvvJpgbWFuYWdlci1hcGlgIOS4jiBgYXJndXNfc2luZ2xlYCDmlbTmqKHlnZfmnoTlu7rlpLHotKXvvIzlhajpg6jokL3lnKggYGFyZ3VzX2NvbmZpZ2AgLyBgcnVudGltZWNvbmZpZ2AgLyBgY29tbW9uL21pZGRsZXdhcmUvcmVkaXMvQXJndXNDb25maWcuZ29gIOKAlOKAlCDov5nkupvmmK/mnKzkvJror53mnJ/pl7QqKuWPpuS4gOS4quW5tuWPkeS7u+WKoSoq5Zyo5bel5L2c5Yy66YeM5pS555qE5pyq5o+Q5Lqk5Luj56CB77yI57uZIFJlZGlzIOmFjee9ruW/q+eFp+WKoCBgaW5zdGFuY2VLZXlgIOWPguaVsO+8jOaUueWIsOS4gOWNiu+8ieOAguaIkeS4gOihjOmDveayoeeisOi/meS6m+aWh+S7tu+8jGBnaXQgc3RhdHVzYCDlj6/moLjlr7njgIJcblxuKirmnKrot5EqKu+8muWJjeerryBgdHNjIC0tbm9FbWl0YCDigJTigJQgYGNsaWVudC9tYW5hZ2VyL25vZGVfbW9kdWxlc2Ag5pyq5a6J6KOF77yM546v5aKD6YeM6LeR5LiN5LqG44CC5pS55Yqo5Y+q5piv57uZ5bey5pyJIGBCYWNrdGVzdFJ1bi5wbGF0Zm9ybUNvZGVgIOWtl+auteWBmumAj+S8oCArIOe7mSBgZmV0Y2hLbGluZVJhbmdlYCDlj4LmlbDliqDlj6/pgIkgYHBsYXRmb3JtQ29kZT86IHN0cmluZ2DvvIznsbvlnovpnaLlvojlsI/jgIJcblxuIyMgMy4g5a+554Wn6aqM5pS25qCH5YeGXG5cbumcgOaxguaWh+aho+e8uuWkse+8jOaMieS7u+WKoeivtOaYjueahOOAjOiMg+WbtOOAjemAkOadoeaguOWvue+8mlxuXG58IOadoeebriB8IOeKtuaAgSB8XG58IC0tLSB8IC0tLSB8XG58IFRyYWRlS2xpbmUg5YqgIHBsYXRmb3JtX2NvZGUg5bm25YWl5ZSv5LiA6ZSuIHwg5bey5ruh6LazIHxcbnwgcmVwb3NpdG9yeSDlm5vkuKrmlrnms5XliqDlubPlj7Dov4fmu6QgfCDlt7Lmu6HotrPvvIjlj6blkKsgYFVwc2VydEtsaW5lc2Ag5Yay56qB5YiX77yJIHxcbnwga2xpbmVfYmFja2ZpbGwuZ28g5Zub5Liq5YWl5Y+j6YCP5LygIHwg5bey5ruh6LazIHxcbnwgbWFuYWdlci1hcGkg5LiJ5Liq5o6l5Y+j6KGl5Y+C5pWwIHwg5bey5ruh6LazIHxcbnwg5aSN55SoIGBtYXJrZXRfZGF0YS5nb2Ag55qEIGBHZXRLbGluZXNCeVBsYXRmb3JtYCB8IOW3sua7oei2s++8iOacquaWsOWinumAgumFjeWxgu+8jOS7heS/riBEQyBiYXIg5Y+j5b6E77yJIHxcbnwg5Y6G5Y+y6KGM5Zue5aGrIGBwbGF0Zm9ybV9jb2RlPWJpbmFuY2VgIHwg5bey5ruh6Laz77yIYEVuc3VyZVRhYmxlYCDoh6rliqjmiafooYzvvIxTUUwg5Lmf5YaZ6L+b6K6+6K6h5paH5qGj77yJIHxcbnwgMW0vNW0vMWgvMWQg5om56YeP5Zue5aGr44CB5aSa5biB56eNIHwg5bey5ruh6Laz77yIYEJhY2tmaWxsS2xpbmVzQmF0Y2hg77yJIHxcbnwg5LiN5byV5YWl5paw5pe25bqP5pWw5o2u5bqT44CB5LiN5Yqg5a6e5L6L57u05bqmIHwg5bey6YG15a6IIHxcblxuIyMgNC4g5rWL6K+V5Lqk5o6l5L+h5oGvXG5cbi0gKirliY3nva4qKu+8muacjeWKoeWQr+WKqOS8mui1sCBgVHJhZGVTZXJ2aWNlLkVuc3VyZVRhYmxlcygpYCDoh6rliqjov4Hnp7vjgILlhYjnoa7orqQgYHRyYWRlX2tsaW5lYCDlt7LmnIkgYHBsYXRmb3JtX2NvZGVgIOWIl+OAgWBpZHhfa2xpbmVfcGxhdGZvcm1fZGltYCDlrZjlnKjkuJQgYGlkeF9zeW1ib2xfaW50ZXJ2YWxfb3BlbmAg5bey5raI5aSx77yb5omL5belIFNRTCDop4Horr7orqHmlofmoaPnrKwgMyDoioLjgIJcbi0gKirml6DpnIDlr4bpkqUqKu+8mmBHZXRLbGluZXNCeVBsYXRmb3JtYCDotbDlhazlvIDooYzmg4XmjqXlj6PvvIzkuI3liqDovb3otKblj7fkvJror53jgILpnIDopoHlpJbnvZHlj6/ovr4gYGZhcGkuYmluYW5jZS5jb21gIOS4jiBgYXBpLmRlZXBjb2luLmNvbWDjgIJcbi0gKirmiafooYzpobrluo8qKu+8mlxuICAxLiBgUE9TVCAva2xpbmVzL2JhY2tmaWxsYCBib2R5IGB7XCJwbGF0Zm9ybUNvZGVzXCI6W1wiYmluYW5jZVwiLFwiZGVlcGNvaW5cIl0sXCJzeW1ib2xzXCI6W1wiQlRDVVNEVFwiLFwiRVRIVVNEVFwiXSxcImludGVydmFsc1wiOltcIjFtXCIsXCI1bVwiLFwiMWhcIixcIjFkXCJdLFwibGltaXRcIjo1MDB9YCDihpIg5pyf5pybIGB0b3RhbD0xNmDvvIxgZmFpbGVkPTBg44CCXG4gIDIuIOafpeW6kyBgU0VMRUNUIHBsYXRmb3JtX2NvZGUsIGludGVydmFsLCBjb3VudCgqKSBGUk9NIHRyYWRlX2tsaW5lIFdIRVJFIHN5bWJvbD0nQlRDVVNEVCcgR1JPVVAgQlkgMSwyYCDihpIg5pyf5pybIDgg57uE77yM5Lik5Liq5bmz5Y+w55qE5ZCM5LiAIGBvcGVuX3RpbWVgIOWQhOWtmOS4gOihjOS4lOS4jeS6kuebuOimhuebluOAglxuICAzLiDph43lpI3miafooYzmraXpqqQgMSDihpIg5bmC562J77yM6KGM5pWw5LiN5aKe77yIYHVwc2VydGVkYCDlj6rliLfooYzmg4XlgLzvvInjgIJcbiAgNC4gYEdFVCAvdHJhZGUta2xpbmVzP3BsYXRmb3JtQ29kZT1kZWVwY29pbiZzeW1ib2w9QlRDVVNEVCZpbnRlcnZhbD0xaGDjgIFgR0VUIC9rbGluZXMvcmFuZ2U/cGxhdGZvcm1Db2RlPWRlZXBjb2luJi4uLmAg4oaSIOWPqui/lOWbniBEQyDpgqPku73vvJvkuI3kvKAgYHBsYXRmb3JtQ29kZWAg5pe26L+U5Zue5biB5a6J6YKj5Lu944CCXG4gIDUuIOiAgeivt+axguS9kyBge1wicGxhdGZvcm1Db2RlXCI6XCJiaW5hbmNlXCIsXCJzeW1ib2xcIjpcIkJUQ1VTRFRcIixcImludGVydmFsXCI6XCIxbVwiLFwibGltaXRcIjoxMDB9YCDku43lj6/nlKjvvIhgdG90YWw9MWDvvInjgIJcbi0gKirmuIXnkIYqKu+8mmBERUxFVEUgRlJPTSB0cmFkZV9rbGluZSBXSEVSRSBwbGF0Zm9ybV9jb2RlPSdkZWVwY29pbidgIOWNs+WPr+WbnuWIsOWNlea6kOeKtuaAge+8jOS4jeW9seWTjeWOhuWPsuW4geWuieaVsOaNruOAglxuXG4jIyA1LiDpgZfnlZnkuI7po47pmalcblxuLSAqKumcgOaxguaWh+aho+e8uuWksSoq77yaYGRvYy9tb2R1bGUvcjItYTAxNzljNDJmZS/mlofmoaMubWRgIOS4jeWtmOWcqO+8jOacrOi9ruacquS7o+WGme+8iOivpeaWh+S7tuW9kuais+eQhumcgOaxgumYtuauteaJgOacie+8ieOAguiuvuiuoeivtOaYjuW3suWNleeLrOiQveWcqCBgZGVzaWduL2Ag55uu5b2V44CCXG4tICoq5bm25Y+R5pS55Yqo5bmy5omwKirvvJrlt6XkvZzljLrph4zmnInlj6bkuIDkuKrku7vliqHmnKrlrozmiJDnmoQgYGFyZ3VzX2NvbmZpZ2AgaW5zdGFuY2VLZXkg5pS56YCg77yM5a+86Ie0IGBtYW5hZ2VyLWFwaWAgLyBgYXJndXNfc2luZ2xlYCDmlbTmqKHlnZfmmoLml7bmnoTlu7rlpLHotKXjgIIqKui/meS8muaMoeS9j+a1i+ivlemYtuauteWQr+acjeWKoSoq77yM6ZyA562J6YKj5p2h5Lu75Yqh5pS25bC+44CCXG4tICoqRGVlcENvaW4g5Y2V5qyh6L+U5Zue5LiK6ZmQIDEwMDAg5qC5KirvvIjluIHlrokgMTUwMO+8ie+8jOaJuemHj+WbnuWhqyBgbGltaXQ+MTAwMGAg5pe2IERDIOS+p+WunumZheWPquS8muaLv+WIsCAxMDAwIOague+8jGBmZXRjaGVkYCDmlbDkvJrlsI/kuo4gYG5lZWRGZXRjaGDvvIzlsZ7pooTmnJ/ogIzpnZ7plJnor6/jgIJcbi0gKipgL2tsaW5lcy9iYWNrZmlsbGAg5ZON5bqU57uT5p6E5Y+Y5LqGKirvvIjljZXlr7nosaEg4oaSIOaJuemHj+axh+aAu++8ieOAguW9k+WJjeWJjeerr+aXoOiwg+eUqOaWue+8jOWmguacieWklumDqOiEmuacrOWvueaOpemcgOWQjOatpeOAglxuLSAqKuWJjeerr+exu+Wei+ajgOafpeacqui3kSoq77yI5pegIG5vZGVfbW9kdWxlc++8ie+8jOW7uuiuruWcqOacieS+nei1lueahOeOr+Wig+mHjOihpeS4gOasoSBgdHNjIC0tbm9FbWl0YOOAglxuLSAqKkRDIOS4juW4geWuieeahCBLIOe6vyBvcGVuX3RpbWUg5a+56b2QKirmnKrlgZrmoKHpqozvvJrkuKTmiYDlkIzlkajmnJ8gSyDnur/nmoTml7bpl7TmiLPnkIborrrkuIrkuIDoh7TvvIzoi6UgREMg6L+U5Zue5bim5YGP56e755qE5pe26Ze05oiz77yM5ZCM5LiAIGBvcGVuX3RpbWVgIOeahOi3qOa6kOWvueavlOS8mumUmeS9jeKAlOKAlOW7uuiurua1i+ivlemYtuauteaKveafpeS4gOe7hCAxaCDmlbDmja7mr5Tlr7nml7bpl7TmiLPjgIJcblxu5om56YeP5Yik5a6a77ya5a6M5oiQIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiJmaW5hbF9hbnN3ZXIifV19XX0=
-->
