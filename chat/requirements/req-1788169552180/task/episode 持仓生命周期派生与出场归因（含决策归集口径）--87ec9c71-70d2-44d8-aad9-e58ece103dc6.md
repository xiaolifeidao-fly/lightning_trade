---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r10-7289d6a1b3"
requirementKey: "req-1788169552180"
resourceName: "episode 持仓生命周期派生与出场归因（含决策归集口径）"
conversationTitle: "episode 持仓生命周期派生与出场归因（含决策归集口径）"
threadId: "87ec9c71-70d2-44d8-aad9-e58ece103dc6"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-01T16:54:57.912849+00:00"
---

# 任务聊天 · episode 持仓生命周期派生与出场归因（含决策归集口径）

## 第 1 轮 · completed · KHlCtqD2beycQGRDJ6Q8TQ

### 用户

执行「动作执行」阶段：episode 持仓生命周期派生与出场归因（含决策归集口径）

### 助手

I'll start by loading the delivery execution skill and reading the requirement docs.

### 助手

I'll start by reading the requirement outline document.

### 助手

Now let me load the project's backend development skill before implementing.

### 助手

Data model is clear. Now I'll implement the episode package.

### 助手

Now let me run it against the local event database that r6 populated.

### 助手

Now let me apply the rebuild to the local event database and produce the reports.

### 助手

完成。以下是本轮交付说明。

## 读了哪几份文档

| 文档 | 为什么读 |
|---|---|
| `doc/requirements/req-1788169552180/需求大纲.md` | 需求级最高优先上下文，全文读完 |
| `docs/argus_single/2026-07-28-事件数据持久化与可视化设计.md` §4.3 / §6 全节 | 任务说明点名的「已定稿状态机与六种出场方式」原文 |
| `docs/argus_single/2026-07-27-波动率状态自适应研究设计.md` §10.3 | 任务说明点名的决策归集反例 |
| `docs/argus_single/vol_state_phase0.py` L100–200 | §10.3 的复现脚本，决策归集口径的唯一可执行真值 |
| `doc/module/r6-cf8beef011/`（设计过程 + 操作手册 + 巡检报告） | **前置任务**，事件表的实际内容、字段可用性断代、目录→实例映射都在这 |
| `doc/module/r1-ab25892542/文档.md`（略读） | 确认 `instance_key` 与 `argus_instance` 注册表口径 |

未打开 r2/r7/r8 的文档——与本任务无接口或字段口径关系。

## 改了什么

**全部是新增，没有修改任何已有文件。**

新包 `server/argus_single/pkg/eventstore/episode/`（放这里而不是 `service/` 域：r9 的 `service/argus_event/repository` 已经直接 `import "argus_single/pkg/eventstore"` 并 `AutoMigrate(eventstore.Models()...)`，读侧复用写侧模型是本仓库既定先例）：

- `model.go` — `episode` / `episode_entry` 两张派生表。归属键改成 `(instance_key, account_label, instrument)`（设计文档原文的 `uid` 在配置没填 uid 时是空串，会把所有账户挤成一个账本）；唯一键锚点用 `first_event_at` 而非 `opened_at`（后者会是 NULL，NULL 进唯一键等于没去重）
- `derive.go` — 纯函数状态机。加仓/减仓只能靠 `size` 快照的增减判定：实测 2026-07-24 之前的减仓事件 `net_side` 和 `pnl` 都是空的
- `rebuild.go` — `LoadEvents` / `ApplyDepthFidelity` / `Replace`（整实例先删后插、包事务）
- `report.go` — Markdown 报告，账户邮箱本地部分整段打码
- `derive_test.go`（20 个单测）、`rebuild_integration_test.go`（3 个集成用例）

新命令 `server/manager-api/cmd/argus-episode-rebuild/main.go` — 照 `argus-event-import` 的先例：可多实例、默认 dry run、`--apply` 才写库。

## 怎么验的

```
go test ./pkg/eventstore/episode/  →  23/23 PASS
go build ./... + go vet ./...      →  common / service / manager-api / argus_single 全绿
gofmt -l                            →  新文件均已格式化
```

**最关键的一条**：`TestIntegrationMatchesDesignDocCensus` 拿 r6 灌进本地库的真实事件，截到设计文档那个 30 天窗口，逐格比对 §6.2 的实测普查——**10/10 吻合**（A: trailing 37 / reduce_to_zero 6 / catastrophe 1 / external 1 / 仍持仓 1；B: 45 / 11 / 1 / 1 / 1）。`reduce_to_zero` 与 `external_close` 是口径改错时最先崩的两类。

全量跑（两实例，2026-06-28~08-21）：

```
argus-single-roc : 7348 事件 → 164 episode / 1501 决策行 / 跳变 13 / 截断头 2
argus-single-ives:  949 事件 →   9 episode /  120 决策行 / 跳变  0 / 截断头 1
```

- 守恒：`Σattributed_pnl 152.2332 + Σunattributed_pnl 1.6119 = Σepisode.pnl 153.8451`
- 快照滞后实测 **5 次**（2026-07-07 05:10–05:11），与设计文档 §6.5「A 3 处 + B 2 处 Δ=0」逐格吻合
- 「反向单反而做大仓位」= 0，与 reverse_gate 拦掉翻转单的预期一致
- 库被并行会话 drop 后我重建了一个独立库 `argus_r10_check` 重跑，数字逐位一致（纯函数性的额外证据）

**决策归集的实证**（报告 §4，同一批钱两种口径按日汇总）：

| 日期 | 决策归集 | 平仓归集 | 差 |
|---|---:|---:|---:|
| 2026-08-19 | 20.84 | 34.52 | −13.69 |
| 2026-08-20 | −81.44 | −66.22 | −15.21 |
| 2026-08-21 | −52.57 | −82.82 | **+30.25** |

8/21 那 30U 说明得很清楚：亏损是 8/20 那波急涨里做的决策造成的，只是拖到 8/21 才平掉。

## 对照任务说明

| 要求 | 状态 |
|---|---|
| §6 状态机 + 六种出场方式 | 已满足，含 §6.2 普查逐格复现 |
| episode 归属 `(instance_key, account)` | 已满足，有跨实例隔离单测 |
| variant 取开仓时刻 | 已满足（§6.3） |
| 深度两档保真度 | 已满足，判据改成「带 upl 心跳 / 全部心跳」的比值——心跳实测是 30 秒一条不是文档写的 1 分钟，按绝对条数判会随采样周期漂移 |
| 仓位跳变记标记但不断开 | 已满足（D6） |
| 派生表可纯函数重建 | 已满足，`reflect.DeepEqual` 单测 + 真实库集成用例 |
| 决策归集口径 | 已满足。整仓平掉时退化成 `pnl/张数`，与 `vol_state_phase0.py:164` 逐字一致 |
| external/manual 不计入策略胜率 | 已满足：`strategy_attributable=0`，盈亏照记但 `pnl_strategy` / `attributed_pnl_strategy` 剔除 |

**两处与需求大纲的出入，如实报数未改需求**：

1. §6.2 的「20 笔完整持仓（A 6 / B 14）」是**只认平仓事件**的口径。同窗口实测：只认平仓事件 A 7 / B 14 = 21；加上 `reduce_to_zero` 后 A 13 / B 16 = 29。B 逐格吻合，A 差的 1 笔是 `2026-08-18 00:11:23` 那笔建仓在窗口之前的 trailing_close。设计文档 §6.2 本就点名这个口径会漏掉 `reduce_to_zero`。
2. 我把 `max_roi_pct` 改名成 `max_roi_pct_observed`——原名看上去像精确值，实际和 min 一样是观测边界。

## 测试交接信息

- **入口**：`cd server/manager-api && go run ./cmd/argus-episode-rebuild --instance <键> [--apply]`。不带 `--apply` 是 dry run，一行都不写库
- **前置数据**：该实例的事件必须已由 r6 的 `argus-event-import --apply` 灌进 `strategy_event` / `balance_sample`
- **本地已备好的库**：`argus_r10_check`（本机 MySQL），已含两实例全部历史 + 派生结果，可直接查
- **集成用例**：`EVENTSTORE_TEST_DSN='<dsn>' go test ./pkg/eventstore/episode/ -run TestIntegration -v`；写库部分只碰 `it-episode-rebuild` 测试实例键
- **可观察结果**：`episode` / `episode_entry` 两表 + `doc/module/r10-7289d6a1b3/report/` 下的 Markdown
- **清理**：两张表可直接 `DROP`，重跑即恢复
- 报告与文档中不含任何凭证；账户邮箱已整段打码

## 遗留与风险

- **等比例退场是模型选择不是观测事实**。交易所净额均价记账下不存在「减掉的是哪几张」，但这仍是假设。FIFO/LIFO 已在设计文档里写明否决理由
- **老数据段 24% 的决策行归因不完整**（2026-07-24 前的减仓不写 pnl，波及 335 行；再加 24 个仍在场的，共 364/1501）。分层统计必须用 `pnl_known=1` 过滤，当成 0 会造假样本
- **`reduce_to_zero` 的 `exit_roi_pct` 是最后那笔减仓的 ROI**，不是整段持仓收益率，前端要标注
- **重建目前只能手工跑**，episode 会滞后于最新持仓。挂 cron 还是读服务增量触发，建议 r13 落页面时一起定
- **episode 的读 API 不在本任务范围**（r9 的 `timeline.go:154` 自己写着 episode 是 r10 的事）。r13 加读路径时落在 `service/argus_event` 域内、复用 `episode.Models()`
- `server/oracle` 模块有 `go.sum` 缺项导致的编译失败，**改动前就存在**（在 HEAD 上复现过），与本任务无关，未触碰

## 产物

- [设计过程](doc/module/r10-7289d6a1b3/design/设计过程.md)
- [episode 派生与重建操作手册](doc/module/r10-7289d6a1b3/episode%20派生与重建操作手册.md)
- [派生报告 · argus-single-roc](doc/module/r10-7289d6a1b3/report/episode-argus-single-roc.md)
- [派生报告 · argus-single-ives](doc/module/r10-7289d6a1b3/report/episode-argus-single-ives.md)

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJLSGxDdHFEMmJleWNRR1JESjZROFRRIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMVQxNjoxNToyOS44MDIyNjArMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDFUMTY6NTQ6NTYuMjY3MDE1KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8mmVwaXNvZGUg5oyB5LuT55Sf5ZG95ZGo5pyf5rS+55Sf5LiO5Ye65Zy65b2S5Zug77yI5ZCr5Yaz562W5b2S6ZuG5Y+j5b6E77yJIn1dfSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiSSdsbCBzdGFydCBieSBsb2FkaW5nIHRoZSBkZWxpdmVyeSBleGVjdXRpb24gc2tpbGwgYW5kIHJlYWRpbmcgdGhlIHJlcXVpcmVtZW50IGRvY3MuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiSSdsbCBzdGFydCBieSByZWFkaW5nIHRoZSByZXF1aXJlbWVudCBvdXRsaW5lIGRvY3VtZW50LiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBsZXQgbWUgbG9hZCB0aGUgcHJvamVjdCdzIGJhY2tlbmQgZGV2ZWxvcG1lbnQgc2tpbGwgYmVmb3JlIGltcGxlbWVudGluZy4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJEYXRhIG1vZGVsIGlzIGNsZWFyLiBOb3cgSSdsbCBpbXBsZW1lbnQgdGhlIGVwaXNvZGUgcGFja2FnZS4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgbGV0IG1lIHJ1biBpdCBhZ2FpbnN0IHRoZSBsb2NhbCBldmVudCBkYXRhYmFzZSB0aGF0IHI2IHBvcHVsYXRlZC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgbGV0IG1lIGFwcGx5IHRoZSByZWJ1aWxkIHRvIHRoZSBsb2NhbCBldmVudCBkYXRhYmFzZSBhbmQgcHJvZHVjZSB0aGUgcmVwb3J0cy4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiLlrozmiJDjgILku6XkuIvmmK/mnKzova7kuqTku5jor7TmmI7jgIJcblxuIyMg6K+75LqG5ZOq5Yeg5Lu95paH5qGjXG5cbnwg5paH5qGjIHwg5Li65LuA5LmI6K+7IHxcbnwtLS18LS0tfFxufCBgZG9jL3JlcXVpcmVtZW50cy9yZXEtMTc4ODE2OTU1MjE4MC/pnIDmsYLlpKfnurIubWRgIHwg6ZyA5rGC57qn5pyA6auY5LyY5YWI5LiK5LiL5paH77yM5YWo5paH6K+75a6MIHxcbnwgYGRvY3MvYXJndXNfc2luZ2xlLzIwMjYtMDctMjgt5LqL5Lu25pWw5o2u5oyB5LmF5YyW5LiO5Y+v6KeG5YyW6K6+6K6hLm1kYCDCpzQuMyAvIMKnNiDlhajoioIgfCDku7vliqHor7TmmI7ngrnlkI3nmoTjgIzlt7LlrprnqL/nirbmgIHmnLrkuI7lha3np43lh7rlnLrmlrnlvI/jgI3ljp/mlocgfFxufCBgZG9jcy9hcmd1c19zaW5nbGUvMjAyNi0wNy0yNy3ms6LliqjnjofnirbmgIHoh6rpgILlupTnoJTnqbborr7orqEubWRgIMKnMTAuMyB8IOS7u+WKoeivtOaYjueCueWQjeeahOWGs+etluW9kumbhuWPjeS+iyB8XG58IGBkb2NzL2FyZ3VzX3NpbmdsZS92b2xfc3RhdGVfcGhhc2UwLnB5YCBMMTAw4oCTMjAwIHwgwqcxMC4zIOeahOWkjeeOsOiEmuacrO+8jOWGs+etluW9kumbhuWPo+W+hOeahOWUr+S4gOWPr+aJp+ihjOecn+WAvCB8XG58IGBkb2MvbW9kdWxlL3I2LWNmOGJlZWYwMTEvYO+8iOiuvuiuoei/h+eoiyArIOaTjeS9nOaJi+WGjCArIOW3oeajgOaKpeWRiu+8iSB8ICoq5YmN572u5Lu75YqhKirvvIzkuovku7booajnmoTlrp7pmYXlhoXlrrnjgIHlrZfmrrXlj6/nlKjmgKfmlq3ku6PjgIHnm67lvZXihpLlrp7kvovmmKDlsITpg73lnKjov5kgfFxufCBgZG9jL21vZHVsZS9yMS1hYjI1ODkyNTQyL+aWh+ahoy5tZGDvvIjnlaXor7vvvIkgfCDnoa7orqQgYGluc3RhbmNlX2tleWAg5LiOIGBhcmd1c19pbnN0YW5jZWAg5rOo5YaM6KGo5Y+j5b6EIHxcblxu5pyq5omT5byAIHIyL3I3L3I4IOeahOaWh+aho+KAlOKAlOS4juacrOS7u+WKoeaXoOaOpeWPo+aIluWtl+auteWPo+W+hOWFs+ezu+OAglxuXG4jIyDmlLnkuobku4DkuYhcblxuKirlhajpg6jmmK/mlrDlop7vvIzmsqHmnInkv67mlLnku7vkvZXlt7LmnInmlofku7bjgIIqKlxuXG7mlrDljIUgYHNlcnZlci9hcmd1c19zaW5nbGUvcGtnL2V2ZW50c3RvcmUvZXBpc29kZS9g77yI5pS+6L+Z6YeM6ICM5LiN5pivIGBzZXJ2aWNlL2Ag5Z+f77yacjkg55qEIGBzZXJ2aWNlL2FyZ3VzX2V2ZW50L3JlcG9zaXRvcnlgIOW3sue7j+ebtOaOpSBgaW1wb3J0IFwiYXJndXNfc2luZ2xlL3BrZy9ldmVudHN0b3JlXCJgIOW5tiBgQXV0b01pZ3JhdGUoZXZlbnRzdG9yZS5Nb2RlbHMoKS4uLilg77yM6K+75L6n5aSN55So5YaZ5L6n5qih5Z6L5piv5pys5LuT5bqT5pei5a6a5YWI5L6L77yJ77yaXG5cbi0gYG1vZGVsLmdvYCDigJQgYGVwaXNvZGVgIC8gYGVwaXNvZGVfZW50cnlgIOS4pOW8oOa0vueUn+ihqOOAguW9kuWxnumUruaUueaIkCBgKGluc3RhbmNlX2tleSwgYWNjb3VudF9sYWJlbCwgaW5zdHJ1bWVudClg77yI6K6+6K6h5paH5qGj5Y6f5paH55qEIGB1aWRgIOWcqOmFjee9ruayoeWhqyB1aWQg5pe25piv56m65Liy77yM5Lya5oqK5omA5pyJ6LSm5oi35oyk5oiQ5LiA5Liq6LSm5pys77yJ77yb5ZSv5LiA6ZSu6ZSa54K555SoIGBmaXJzdF9ldmVudF9hdGAg6ICM6Z2eIGBvcGVuZWRfYXRg77yI5ZCO6ICF5Lya5pivIE5VTEzvvIxOVUxMIOi/m+WUr+S4gOmUruetieS6juayoeWOu+mHje+8iVxuLSBgZGVyaXZlLmdvYCDigJQg57qv5Ye95pWw54q25oCB5py644CC5Yqg5LuTL+WHj+S7k+WPquiDvemdoCBgc2l6ZWAg5b+r54Wn55qE5aKe5YeP5Yik5a6a77ya5a6e5rWLIDIwMjYtMDctMjQg5LmL5YmN55qE5YeP5LuT5LqL5Lu2IGBuZXRfc2lkZWAg5ZKMIGBwbmxgIOmDveaYr+epuueahFxuLSBgcmVidWlsZC5nb2Ag4oCUIGBMb2FkRXZlbnRzYCAvIGBBcHBseURlcHRoRmlkZWxpdHlgIC8gYFJlcGxhY2Vg77yI5pW05a6e5L6L5YWI5Yig5ZCO5o+S44CB5YyF5LqL5Yqh77yJXG4tIGByZXBvcnQuZ29gIOKAlCBNYXJrZG93biDmiqXlkYrvvIzotKbmiLfpgq7nrrHmnKzlnLDpg6jliIbmlbTmrrXmiZPnoIFcbi0gYGRlcml2ZV90ZXN0LmdvYO+8iDIwIOS4quWNlea1i++8ieOAgWByZWJ1aWxkX2ludGVncmF0aW9uX3Rlc3QuZ29g77yIMyDkuKrpm4bmiJDnlKjkvovvvIlcblxu5paw5ZG95LukIGBzZXJ2ZXIvbWFuYWdlci1hcGkvY21kL2FyZ3VzLWVwaXNvZGUtcmVidWlsZC9tYWluLmdvYCDigJQg54WnIGBhcmd1cy1ldmVudC1pbXBvcnRgIOeahOWFiOS+i++8muWPr+WkmuWunuS+i+OAgem7mOiupCBkcnkgcnVu44CBYC0tYXBwbHlgIOaJjeWGmeW6k+OAglxuXG4jIyDmgI7kuYjpqoznmoRcblxuYGBgXG5nbyB0ZXN0IC4vcGtnL2V2ZW50c3RvcmUvZXBpc29kZS8gIOKGkiAgMjMvMjMgUEFTU1xuZ28gYnVpbGQgLi8uLi4gKyBnbyB2ZXQgLi8uLi4gICAgICDihpIgIGNvbW1vbiAvIHNlcnZpY2UgLyBtYW5hZ2VyLWFwaSAvIGFyZ3VzX3NpbmdsZSDlhajnu79cbmdvZm10IC1sICAgICAgICAgICAgICAgICAgICAgICAgICAgIOKGkiAg5paw5paH5Lu25Z2H5bey5qC85byP5YyWXG5gYGBcblxuKirmnIDlhbPplK7nmoTkuIDmnaEqKu+8mmBUZXN0SW50ZWdyYXRpb25NYXRjaGVzRGVzaWduRG9jQ2Vuc3VzYCDmi78gcjYg54GM6L+b5pys5Zyw5bqT55qE55yf5a6e5LqL5Lu277yM5oiq5Yiw6K6+6K6h5paH5qGj6YKj5LiqIDMwIOWkqeeql+WPo++8jOmAkOagvOavlOWvuSDCpzYuMiDnmoTlrp7mtYvmma7mn6XigJTigJQqKjEwLzEwIOWQu+WQiCoq77yIQTogdHJhaWxpbmcgMzcgLyByZWR1Y2VfdG9femVybyA2IC8gY2F0YXN0cm9waGUgMSAvIGV4dGVybmFsIDEgLyDku43mjIHku5MgMe+8m0I6IDQ1IC8gMTEgLyAxIC8gMSAvIDHvvInjgIJgcmVkdWNlX3RvX3plcm9gIOS4jiBgZXh0ZXJuYWxfY2xvc2VgIOaYr+WPo+W+hOaUuemUmeaXtuacgOWFiOW0qeeahOS4pOexu+OAglxuXG7lhajph4/ot5HvvIjkuKTlrp7kvovvvIwyMDI2LTA2LTI4fjA4LTIx77yJ77yaXG5cbmBgYFxuYXJndXMtc2luZ2xlLXJvYyA6IDczNDgg5LqL5Lu2IOKGkiAxNjQgZXBpc29kZSAvIDE1MDEg5Yaz562W6KGMIC8g6Lez5Y+YIDEzIC8g5oiq5pat5aS0IDJcbmFyZ3VzLXNpbmdsZS1pdmVzOiAgOTQ5IOS6i+S7tiDihpIgICA5IGVwaXNvZGUgLyAgMTIwIOWGs+etluihjCAvIOi3s+WPmCAgMCAvIOaIquaWreWktCAxXG5gYGBcblxuLSDlrojmgZLvvJpgzqNhdHRyaWJ1dGVkX3BubCAxNTIuMjMzMiArIM6jdW5hdHRyaWJ1dGVkX3BubCAxLjYxMTkgPSDOo2VwaXNvZGUucG5sIDE1My44NDUxYFxuLSDlv6vnhafmu57lkI7lrp7mtYsgKio1IOasoSoq77yIMjAyNi0wNy0wNyAwNToxMOKAkzA1OjEx77yJ77yM5LiO6K6+6K6h5paH5qGjIMKnNi4144CMQSAzIOWkhCArIEIgMiDlpIQgzpQ9MOOAjemAkOagvOWQu+WQiFxuLSDjgIzlj43lkJHljZXlj43ogIzlgZrlpKfku5PkvY3jgI09IDDvvIzkuI4gcmV2ZXJzZV9nYXRlIOaLpuaOiee/u+i9rOWNleeahOmihOacn+S4gOiHtFxuLSDlupPooqvlubbooYzkvJror50gZHJvcCDlkI7miJHph43lu7rkuobkuIDkuKrni6znq4vlupMgYGFyZ3VzX3IxMF9jaGVja2Ag6YeN6LeR77yM5pWw5a2X6YCQ5L2N5LiA6Ie077yI57qv5Ye95pWw5oCn55qE6aKd5aSW6K+B5o2u77yJXG5cbioq5Yaz562W5b2S6ZuG55qE5a6e6K+BKirvvIjmiqXlkYogwqc077yM5ZCM5LiA5om56ZKx5Lik56eN5Y+j5b6E5oyJ5pel5rGH5oC777yJ77yaXG5cbnwg5pel5pyfIHwg5Yaz562W5b2S6ZuGIHwg5bmz5LuT5b2S6ZuGIHwg5beuIHxcbnwtLS18LS0tOnwtLS06fC0tLTp8XG58IDIwMjYtMDgtMTkgfCAyMC44NCB8IDM0LjUyIHwg4oiSMTMuNjkgfFxufCAyMDI2LTA4LTIwIHwg4oiSODEuNDQgfCDiiJI2Ni4yMiB8IOKIkjE1LjIxIHxcbnwgMjAyNi0wOC0yMSB8IOKIkjUyLjU3IHwg4oiSODIuODIgfCAqKiszMC4yNSoqIHxcblxuOC8yMSDpgqMgMzBVIOivtOaYjuW+l+W+iOa4healmu+8muS6j+aNn+aYryA4LzIwIOmCo+azouaApea2qOmHjOWBmueahOWGs+etlumAoOaIkOeahO+8jOWPquaYr+aLluWIsCA4LzIxIOaJjeW5s+aOieOAglxuXG4jIyDlr7nnhafku7vliqHor7TmmI5cblxufCDopoHmsYIgfCDnirbmgIEgfFxufC0tLXwtLS18XG58IMKnNiDnirbmgIHmnLogKyDlha3np43lh7rlnLrmlrnlvI8gfCDlt7Lmu6HotrPvvIzlkKsgwqc2LjIg5pmu5p+l6YCQ5qC85aSN546wIHxcbnwgZXBpc29kZSDlvZLlsZ4gYChpbnN0YW5jZV9rZXksIGFjY291bnQpYCB8IOW3sua7oei2s++8jOaciei3qOWunuS+i+malOemu+WNlea1iyB8XG58IHZhcmlhbnQg5Y+W5byA5LuT5pe25Yi7IHwg5bey5ruh6Laz77yIwqc2LjPvvIkgfFxufCDmt7HluqbkuKTmoaPkv53nnJ/luqYgfCDlt7Lmu6HotrPvvIzliKTmja7mlLnmiJDjgIzluKYgdXBsIOW/g+i3syAvIOWFqOmDqOW/g+i3s+OAjeeahOavlOWAvOKAlOKAlOW/g+i3s+Wunua1i+aYryAzMCDnp5LkuIDmnaHkuI3mmK/mlofmoaPlhpnnmoQgMSDliIbpkp/vvIzmjInnu53lr7nmnaHmlbDliKTkvJrpmo/ph4fmoLflkajmnJ/mvILnp7sgfFxufCDku5PkvY3ot7Plj5jorrDmoIforrDkvYbkuI3mlq3lvIAgfCDlt7Lmu6HotrPvvIhENu+8iSB8XG58IOa0vueUn+ihqOWPr+e6r+WHveaVsOmHjeW7uiB8IOW3sua7oei2s++8jGByZWZsZWN0LkRlZXBFcXVhbGAg5Y2V5rWLICsg55yf5a6e5bqT6ZuG5oiQ55So5L6LIHxcbnwg5Yaz562W5b2S6ZuG5Y+j5b6EIHwg5bey5ruh6Laz44CC5pW05LuT5bmz5o6J5pe26YCA5YyW5oiQIGBwbmwv5byg5pWwYO+8jOS4jiBgdm9sX3N0YXRlX3BoYXNlMC5weToxNjRgIOmAkOWtl+S4gOiHtCB8XG58IGV4dGVybmFsL21hbnVhbCDkuI3orqHlhaXnrZbnlaXog5znjocgfCDlt7Lmu6HotrPvvJpgc3RyYXRlZ3lfYXR0cmlidXRhYmxlPTBg77yM55uI5LqP54Wn6K6w5L2GIGBwbmxfc3RyYXRlZ3lgIC8gYGF0dHJpYnV0ZWRfcG5sX3N0cmF0ZWd5YCDliZTpmaQgfFxuXG4qKuS4pOWkhOS4jumcgOaxguWkp+e6sueahOWHuuWFpe+8jOWmguWunuaKpeaVsOacquaUuemcgOaxgioq77yaXG5cbjEuIMKnNi4yIOeahOOAjDIwIOeslOWujOaVtOaMgeS7k++8iEEgNiAvIEIgMTTvvInjgI3mmK8qKuWPquiupOW5s+S7k+S6i+S7tioq55qE5Y+j5b6E44CC5ZCM56qX5Y+j5a6e5rWL77ya5Y+q6K6k5bmz5LuT5LqL5Lu2IEEgNyAvIEIgMTQgPSAyMe+8m+WKoOS4iiBgcmVkdWNlX3RvX3plcm9gIOWQjiBBIDEzIC8gQiAxNiA9IDI544CCQiDpgJDmoLzlkLvlkIjvvIxBIOW3rueahCAxIOeslOaYryBgMjAyNi0wOC0xOCAwMDoxMToyM2Ag6YKj56yU5bu65LuT5Zyo56qX5Y+j5LmL5YmN55qEIHRyYWlsaW5nX2Nsb3Nl44CC6K6+6K6h5paH5qGjIMKnNi4yIOacrOWwseeCueWQjei/meS4quWPo+W+hOS8mua8j+aOiSBgcmVkdWNlX3RvX3plcm9g44CCXG4yLiDmiJHmioogYG1heF9yb2lfcGN0YCDmlLnlkI3miJAgYG1heF9yb2lfcGN0X29ic2VydmVkYOKAlOKAlOWOn+WQjeeci+S4iuWOu+WDj+eyvuehruWAvO+8jOWunumZheWSjCBtaW4g5LiA5qC35piv6KeC5rWL6L6555WM44CCXG5cbiMjIOa1i+ivleS6pOaOpeS/oeaBr1xuXG4tICoq5YWl5Y+jKirvvJpgY2Qgc2VydmVyL21hbmFnZXItYXBpICYmIGdvIHJ1biAuL2NtZC9hcmd1cy1lcGlzb2RlLXJlYnVpbGQgLS1pbnN0YW5jZSA86ZSuPiBbLS1hcHBseV1g44CC5LiN5bimIGAtLWFwcGx5YCDmmK8gZHJ5IHJ1bu+8jOS4gOihjOmDveS4jeWGmeW6k1xuLSAqKuWJjee9ruaVsOaNrioq77ya6K+l5a6e5L6L55qE5LqL5Lu25b+F6aG75bey55SxIHI2IOeahCBgYXJndXMtZXZlbnQtaW1wb3J0IC0tYXBwbHlgIOeBjOi/myBgc3RyYXRlZ3lfZXZlbnRgIC8gYGJhbGFuY2Vfc2FtcGxlYFxuLSAqKuacrOWcsOW3suWkh+WlveeahOW6kyoq77yaYGFyZ3VzX3IxMF9jaGVja2DvvIjmnKzmnLogTXlTUUzvvInvvIzlt7LlkKvkuKTlrp7kvovlhajpg6jljoblj7IgKyDmtL7nlJ/nu5PmnpzvvIzlj6/nm7TmjqXmn6Vcbi0gKirpm4bmiJDnlKjkvosqKu+8mmBFVkVOVFNUT1JFX1RFU1RfRFNOPSc8ZHNuPicgZ28gdGVzdCAuL3BrZy9ldmVudHN0b3JlL2VwaXNvZGUvIC1ydW4gVGVzdEludGVncmF0aW9uIC12YO+8m+WGmeW6k+mDqOWIhuWPqueisCBgaXQtZXBpc29kZS1yZWJ1aWxkYCDmtYvor5Xlrp7kvovplK5cbi0gKirlj6/op4Llr5/nu5PmnpwqKu+8mmBlcGlzb2RlYCAvIGBlcGlzb2RlX2VudHJ5YCDkuKTooaggKyBgZG9jL21vZHVsZS9yMTAtNzI4OWQ2YTFiMy9yZXBvcnQvYCDkuIvnmoQgTWFya2Rvd25cbi0gKirmuIXnkIYqKu+8muS4pOW8oOihqOWPr+ebtOaOpSBgRFJPUGDvvIzph43ot5HljbPmgaLlpI1cbi0g5oql5ZGK5LiO5paH5qGj5Lit5LiN5ZCr5Lu75L2V5Yet6K+B77yb6LSm5oi36YKu566x5bey5pW05q615omT56CBXG5cbiMjIOmBl+eVmeS4jumjjumZqVxuXG4tICoq562J5q+U5L6L6YCA5Zy65piv5qih5Z6L6YCJ5oup5LiN5piv6KeC5rWL5LqL5a6eKirjgILkuqTmmJPmiYDlh4Dpop3lnYfku7forrDotKbkuIvkuI3lrZjlnKjjgIzlh4/mjonnmoTmmK/lk6rlh6DlvKDjgI3vvIzkvYbov5nku43mmK/lgYforr7jgIJGSUZPL0xJRk8g5bey5Zyo6K6+6K6h5paH5qGj6YeM5YaZ5piO5ZCm5Yaz55CG55SxXG4tICoq6ICB5pWw5o2u5q61IDI0JSDnmoTlhrPnrZbooYzlvZLlm6DkuI3lrozmlbQqKu+8iDIwMjYtMDctMjQg5YmN55qE5YeP5LuT5LiN5YaZIHBubO+8jOazouWPiiAzMzUg6KGM77yb5YaN5YqgIDI0IOS4quS7jeWcqOWcuueahO+8jOWFsSAzNjQvMTUwMe+8ieOAguWIhuWxgue7n+iuoeW/hemhu+eUqCBgcG5sX2tub3duPTFgIOi/h+a7pO+8jOW9k+aIkCAwIOS8mumAoOWBh+agt+acrFxuLSAqKmByZWR1Y2VfdG9femVyb2Ag55qEIGBleGl0X3JvaV9wY3RgIOaYr+acgOWQjumCo+eslOWHj+S7k+eahCBST0kqKu+8jOS4jeaYr+aVtOauteaMgeS7k+aUtuebiueOh++8jOWJjeerr+imgeagh+azqFxuLSAqKumHjeW7uuebruWJjeWPquiDveaJi+W3pei3kSoq77yMZXBpc29kZSDkvJrmu57lkI7kuo7mnIDmlrDmjIHku5PjgILmjIIgY3JvbiDov5jmmK/or7vmnI3liqHlop7ph4/op6blj5HvvIzlu7rorq4gcjEzIOiQvemhtemdouaXtuS4gOi1t+WumlxuLSAqKmVwaXNvZGUg55qE6K+7IEFQSSDkuI3lnKjmnKzku7vliqHojIPlm7QqKu+8iHI5IOeahCBgdGltZWxpbmUuZ286MTU0YCDoh6rlt7HlhpnnnYAgZXBpc29kZSDmmK8gcjEwIOeahOS6i++8ieOAgnIxMyDliqDor7vot6/lvoTml7bokL3lnKggYHNlcnZpY2UvYXJndXNfZXZlbnRgIOWfn+WGheOAgeWkjeeUqCBgZXBpc29kZS5Nb2RlbHMoKWBcbi0gYHNlcnZlci9vcmFjbGVgIOaooeWdl+aciSBgZ28uc3VtYCDnvLrpobnlr7zoh7TnmoTnvJbor5HlpLHotKXvvIwqKuaUueWKqOWJjeWwseWtmOWcqCoq77yI5ZyoIEhFQUQg5LiK5aSN546w6L+H77yJ77yM5LiO5pys5Lu75Yqh5peg5YWz77yM5pyq6Kem56KwXG5cbiMjIOS6p+eJqVxuXG4tIFvorr7orqHov4fnqItdKGRvYy9tb2R1bGUvcjEwLTcyODlkNmExYjMvZGVzaWduL+iuvuiuoei/h+eoiy5tZClcbi0gW2VwaXNvZGUg5rS+55Sf5LiO6YeN5bu65pON5L2c5omL5YaMXShkb2MvbW9kdWxlL3IxMC03Mjg5ZDZhMWIzL2VwaXNvZGUlMjDmtL7nlJ/kuI7ph43lu7rmk43kvZzmiYvlhowubWQpXG4tIFvmtL7nlJ/miqXlkYogwrcgYXJndXMtc2luZ2xlLXJvY10oZG9jL21vZHVsZS9yMTAtNzI4OWQ2YTFiMy9yZXBvcnQvZXBpc29kZS1hcmd1cy1zaW5nbGUtcm9jLm1kKVxuLSBb5rS+55Sf5oql5ZGKIMK3IGFyZ3VzLXNpbmdsZS1pdmVzXShkb2MvbW9kdWxlL3IxMC03Mjg5ZDZhMWIzL3JlcG9ydC9lcGlzb2RlLWFyZ3VzLXNpbmdsZS1pdmVzLm1kKVxuXG7mibnph4/liKTlrprvvJrlrozmiJAiLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6ImZpbmFsX2Fuc3dlciJ9XX1dfQ==
-->
