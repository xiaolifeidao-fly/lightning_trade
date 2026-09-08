# episode 派生与重建 · 操作手册

> 任务：`r10-7289d6a1b3` · 需求：`req-1788169552180`
> 工具：`server/manager-api/cmd/argus-episode-rebuild`
> 依赖：`r6-cf8beef011` 已把事件灌进 `strategy_event` / `balance_sample`

---

## 1. 一句话

把事件库里的离散事件归集成 `episode`（一次完整持仓）与 `episode_entry`
（一次建仓决策 + 归给它的已实现盈亏）。**派生是纯函数**：两张表可随时整表
DROP 再跑一次，结果一致。

---

## 2. 跑一次

从 `server/manager-api` 目录跑；不带 `--dsn` 时读该目录 `configs/application.properties`
的 `sqlconn`。

```bash
cd server/manager-api

# 先 dry run：只派生并出报告，一行都不写库
go run ./cmd/argus-episode-rebuild \
  --instance argus-single-roc \
  --instance argus-single-ives \
  --report-dir ../../doc/module/r10-7289d6a1b3/report

# 确认后写库
go run ./cmd/argus-episode-rebuild \
  --instance argus-single-roc \
  --instance argus-single-ives \
  --report-dir ../../doc/module/r10-7289d6a1b3/report \
  --apply
```

| 参数 | 说明 |
|---|---|
| `--instance` | 可重复。**必填**：每个实例整表替换，不给就没有默认值 |
| `--dsn` | 事件库 DSN；缺省读 `configs/application.properties` 的 `sqlconn` |
| `--apply` | 真正写库。缺省只派生并出报告 |
| `--report-dir` | 每个实例一份 `episode-<实例键>.md` |
| `--report` | 单实例报告路径，与 `--report-dir` 二选一 |
| `--batch-size` | 单条 INSERT 行数，默认 500 |

**没有增量模式，也不会有。** episode 会跨天（实测最长 68.8 小时），按日期切片
会把跨界持仓拦腰砍成两个假 episode。一个实例三个月的持仓相关事件不到 8 千条，
全量派生 < 1 秒。

**`--apply` 是整实例先删后插、包在一个事务里。** 同一个实例键下的旧派生行会被
全部清掉。DSN 指错库不会破坏事实表（本工具只读 `strategy_event` / `balance_sample`），
但会清掉那个库里同名实例的 episode。

---

## 3. 报告怎么读

| 章节 | 看什么 |
|---|---|
| 1 出场方式分布 | `reduce_to_zero` 应该占一到两成。它是 0 说明减仓判定失效了 |
| 2 逐账户 | 峰值张数应与该账户的 cap 对得上（roc A=26 / B=13，ives=246） |
| 3 决策归集 | 「落不到任何决策上的盈亏」= 截断头造成的缺口；「归因不完整的决策行」按 `pnl_known=0` 过滤 |
| 4 决策 vs 平仓归集 | **两列逐日不同是正常且必须的**。相同才是出了问题（说明归集退化成了平仓口径） |
| 5 深度保真度 | 2026-07-21 之前应全是 `alert_sampled`，之后应全是 `minute` |
| 6 异常与边界 | 「反向单反而做大仓位」必须是 0；不是 0 说明 reverse_gate 出了问题或事件被漏读 |
| 7 episode 明细 | 建仓列是「—」即截断头（建仓在数据窗口之前），跳变列的 ⚠ 见 §6.5 |

报告里的账户邮箱本地部分整段打码（`账户A-***@qq.com`），不含任何凭证。

---

## 4. 下游怎么查

**按状态/条件分层的收益归因一律走 `episode_entry`，用 `decided_at` 分桶。**
用 `episode.closed_at` 分桶会把「更容易平仓的状态」误读成「更赚钱的状态」——
波动率研究 §10.3 记录过这个伪影：按平仓归集得出「高波动期收益是低波动期 8–11 倍、
16/16 路径同号」的极强信号，改决策归集后差异塌缩到 0.001–0.065。

```sql
-- 对：按建仓决策时刻分层
SELECT variant, SUM(attributed_pnl_strategy)
FROM episode_entry
WHERE instance_key = ? AND pnl_known = 1 AND decided_at BETWEEN ? AND ?
GROUP BY variant;

-- 策略胜率（剔除交易所侧与人工平仓）
SELECT SUM(pnl > 0) / COUNT(*)
FROM episode
WHERE instance_key = ? AND strategy_attributable = 1 AND closed_at IS NOT NULL;
```

| 字段 | 陷阱 |
|---|---|
| `min_roi_pct_observed` | 只是真实最深的**下界**（loss_alert 5 分钟采样 + −150% 门槛）。UI 要写成「观测最深 −362%（5 分钟采样，真实更深）」 |
| `exit_roi_pct`（`reduce_to_zero` 时） | 是最后那笔减仓的 ROI，不是整段持仓的收益率 |
| `open_size` vs `sum(entry.open_size)` | 前者含 `hidden_size`（截断头里建仓决策不可见的张数），两者会差这么多 |
| `pnl` vs `sum(entry.attributed_pnl)` | 差额 = `unattributed_pnl` |
| `opened_at IS NULL` | 截断头，`variant` / `config_version` 不是决策时刻的值，不要进 variant 归因 |
| `depth_fidelity` | `mixed` 段两侧精度不同，**不得混排比较** |

---

## 5. 什么时候要重跑

- 事件表新灌了历史（跑完 `argus-event-import --apply` 之后）；
- 双写期巡检补录了缺失行；
- 派生口径本身改了（改了 `pkg/eventstore/episode` 就要全量重跑）。

事件是持续写入的，episode 会滞后于最新持仓。是否挂 cron 每日重建由 r13 落页面时一起定。

---

## 6. 自查

```bash
cd server/argus_single
go test ./pkg/eventstore/episode/                      # 20 个单测，不需要库

# 有可丢弃的事件库时，再跑 3 个集成用例（含设计文档 §6.2 普查的逐格复现）
EVENTSTORE_TEST_DSN='<dsn>' go test ./pkg/eventstore/episode/ -run TestIntegration -v
```

集成用例的写库部分只碰 `it-episode-rebuild` 这个测试实例键，不会动真实实例的行。
`TestIntegrationMatchesDesignDocCensus` 需要库里有 `argus-single-roc` 的历史事件，
没有就自动跳过。
