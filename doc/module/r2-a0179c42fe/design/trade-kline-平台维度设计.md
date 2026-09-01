# trade_kline 平台维度改造设计（r2-a0179c42fe）

> 说明：本任务在动作执行阶段没有找到 `doc/module/r2-a0179c42fe/文档.md`（梳理需求阶段未产出），
> 实现以任务说明中列出的范围和 `server/` 现状代码为准，本文记录实际落地的设计。

## 1. 目标

同一 `symbol` 下并存 DeepCoin 与币安两条行情，`1m/5m/1h/1d` 可批量回填，按多币种设计。
改造前 `TradeKline` 唯一键只有 `(symbol, interval, open_time)`，两个平台的同一根 K 线会互相覆盖。

## 2. 数据模型

`server/service/trade/repository/model.go` 的 `TradeKline` 新增 `platform_code` 并入唯一键：

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `platform_code` | `varchar(32) NOT NULL DEFAULT 'binance'` | 行情平台 `binance` / `deepcoin` |

- 新唯一索引 `idx_kline_platform_dim(platform_code, symbol, interval, open_time)`
- 旧唯一索引 `idx_symbol_interval_open` 在迁移中删除
- 附加普通索引 `idx_platform_code`、`idx_symbol`

## 3. 迁移方式

`TradeKlineRepository.EnsureTable()` 在 `AutoMigrate` 之后执行 `migrateKlinePlatformDim()`：

1. `UPDATE trade_kline SET platform_code='binance' WHERE platform_code IS NULL OR platform_code=''`
   —— 历史行全部来自币安；
2. 若 `idx_symbol_interval_open` 仍存在则 `DropIndex`，否则同一 symbol 的第二个平台会被旧唯一键挡住。

服务启动时 `TradeService.EnsureTables()` 会调到，无需人工执行。手工执行等价 SQL：

```sql
ALTER TABLE `trade_kline`
  ADD COLUMN `platform_code` varchar(32) NOT NULL DEFAULT 'binance' COMMENT '行情平台代码 binance/deepcoin' AFTER `updated_by`;

UPDATE `trade_kline` SET `platform_code` = 'binance'
 WHERE `platform_code` IS NULL OR `platform_code` = '';

ALTER TABLE `trade_kline`
  ADD UNIQUE KEY `idx_kline_platform_dim` (`platform_code`, `symbol`, `interval`, `open_time`),
  ADD KEY `idx_platform_code` (`platform_code`),
  ADD KEY `idx_symbol` (`symbol`),
  DROP INDEX `idx_symbol_interval_open`;
```

全新库直接用 `docs/coin_domain_tables.sql` 里已更新的建表语句。

## 4. Repository 平台过滤

`server/service/trade/repository/repository.go`：

- `ListBySymbolInterval(platform, symbol, interval, limit)`
- `ListBySymbolIntervalTimeRange(platform, symbol, interval, start, end)`
- `LatestKline(platform, symbol, interval)`
- `CountBySymbolIntervalRange(platform, symbol, interval, start, end)`
- `UpsertKlines` 冲突列改为 `(platform_code, symbol, interval, open_time)`，入库前归一 `platform_code`

平台归一统一走 `NormalizeKlinePlatform`：`trim + lower`，**空值兜底为 `binance`**，
保证没传平台的老调用方读到的仍是原来那份数据，不会把两个平台的行情混在一条曲线里。

## 5. 回填与查询链路

`server/service/trade/kline_backfill.go` 四个入口全部透传平台：

| 入口 | 平台来源 |
| --- | --- |
| `BackfillKlines` | 请求体 `platformCode`，空→`binance` |
| `fetchAndStoreRecentKlines` | 调用方传入，同时用于 `GetKlinesByPlatform` 与入库 `platform_code` |
| `ensureBacktestKlines` | `run.PlatformCode` |
| `ListKlinesInRange` | 入参 `platform`，空→`binance` |

行情拉取复用 `server/argus_single/pkg/trade/market_data.go` 的 `GetKlinesByPlatform`，
已支持 `binance` 与 `deepcoin` 两个分支，无需新增适配。

## 6. 批量回填

新增 `BackfillKlinesBatch`：按 **平台 × 币种 × 周期** 笛卡尔积逐组合调用 `BackfillKlines`。

- 复数字段 `platformCodes` / `symbols` / `intervals` 为空时回落到单数字段，兼容老请求体
- 周期缺省为 `["1m","5m","1h","1d"]`，平台缺省为 `["binance"]`
- 组合数上限 200（`klineBatchMaxCombos`），超限直接报错，避免打爆交易所限频
- 单组合失败只记 `error` 并继续，返回 `total/succeeded/failed + items`

## 7. 接口变更

| 接口 | 变更 |
| --- | --- |
| `POST /klines/backfill` | 入参改绑 `BatchBackfillKlineDTO`，新增 `platformCodes/symbols/intervals`；响应改为批量汇总 `BackfillKlineBatchResultDTO` |
| `GET /klines/range` | 新增 `platformCode` 查询参数 |
| `GET /trade-klines` | 新增 `platformCode` 查询参数；`TradeKlineDTO` 响应补 `platformCode` |

前端 `client/manager/.../trade-backtest-runs` 的「K线详情」弹窗改为透传 `run.platformCode`。

## 8. 容量结论

`1m + 5m + 1h + 1d` 四档双源约 **0.4GB/年·币种**。全时段 1s 粒度约 18GB/年·币种，已否决。
不引入新时序数据库；行情属市场数据，不加实例维度。
