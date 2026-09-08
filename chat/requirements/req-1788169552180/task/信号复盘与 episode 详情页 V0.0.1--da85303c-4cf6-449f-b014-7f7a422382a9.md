---
format: "delivery-task-planner-chat/v1"
resourceType: "task"
resourceKey: "r13-0585d5f39b"
requirementKey: "req-1788169552180"
resourceName: "信号复盘与 episode 详情页"
conversationTitle: "信号复盘与 episode 详情页 V0.0.1"
threadId: "da85303c-4cf6-449f-b014-7f7a422382a9"
provider: "claude"
phase: "development"
lastTurnStatus: "completed"
archivedAt: "2026-09-02T01:14:56.680128+00:00"
---

# 任务聊天 · 信号复盘与 episode 详情页 V0.0.1

## 第 1 轮 · completed · JaegdbFaH2Z2rcOyDB2YPQ

### 用户

执行「动作执行」阶段：信号复盘与 episode 详情页

### 助手

I'll start by loading the delivery execution skill and the project's development skills.

### 助手

I'll begin by reading the requirement doc and surveying the current state.

### 助手

Now let me survey the prototype and the prerequisite task outputs in parallel.

### 助手

Now I have the full picture. Let me start with the backend wiring that r10 left incomplete.

### 助手

Now the HTTP routes for episodes and slice-compare.

### 助手

Now update the route check test to cover the new endpoints.

### 助手

Now the constants and formatting helpers.

### 助手

Now the filter bar.

### 助手

Now the stats panels (KPI tiles + 拦截原因分布 + 强度分级 + 出场归因).

### 助手

Now the signal table.

### 助手

Now the two drawers — the signal detail (TG message reconstruction) and the episode lifecycle.

### 助手

Now the slice-compare table and the page itself.

### 助手

Now the CSS additions and menu registration.

### 助手

Now type-check the frontend.

### 助手

Now the permission SQL for the new endpoints plus the page resource.

### 助手

Now the design process document.

### 助手

All work is complete and verified.

## 改了什么

**后端 — 补完 r10 遗留的 episode 读路径接线**（r10 设计文档 §遗留项明确写了「episode 的读 API 不在本任务范围，应该在 r13 那一刀里加」。开工时 `service/argus_event` 有 9 处编译错误）

- `server/service/argus_event/service.go` — 加 `ErrEpisodeNotFound` 哨兵；`ArgusEventService` 加 `episodeRepository` / `episodeEntryRepository` 两个只读仓储并在构造函数里接上；`EnsureTable()` 追加建两张派生表（没跑过重建时页面看到空表+提示，而不是未知表 500）。
- `server/manager-api/pkg/argus_event/argus_event.go` — 追加 5 条只读 GET：`/episodes`、`/episodes/:id`、`/episode-stats`、`/episode-exit-kinds`、`/slice-compare`。出场方式选项走独立路径而非 `/episodes/exit-kinds`，避免跟 `:id` 抢段。
- `server/manager-api/pkg/argus_event/route_check_test.go` — 期望路由表补齐 5 条。

**前端 — 新增 `client/manager/src/app/(console)/argus-signals/`**（12 个文件）

`api/argus-signals.api.ts`（21 个 DTO + 9 个请求）、`constants.ts`、`hooks/useArgusSignals.ts`，components 下 `SignalFilterBar` / `ReviewStats` / `SignalTable` / `EpisodeTable` / `SignalDetailDrawer` / `EpisodeDrawer` / `SliceCompareTable` / `TrackChart`，以及 `page.tsx`（信号 / episode / 切片对比 三视角）。

**共享文件只做追加**：`globals.css` 末尾追加 r13 样式段（全新类名）；`ManagerShell.tsx` 加一个菜单项 + 标题 + 展开分支（菜单重组是 r15 的事，本轮不动）。

**文档与权限**：[doc/module/r13-0585d5f39b/design/设计过程.md](doc/module/r13-0585d5f39b/design/设计过程.md)、[doc/module/r13-0585d5f39b/权限资源.sql](doc/module/r13-0585d5f39b/权限资源.sql)

## 怎么验的

| 验证 | 结果 |
|---|---|
| `cd server/service && go build ./...` | 通过（修复前 9 处编译错误） |
| `go test ./argus_event/...` | `ok service/argus_event` |
| `cd server/manager-api && go build ./...` | 通过 |
| `go test ./pkg/argus_event/...` | `ok`（13 条 GET 全注册、无 gin 冲突） |
| `npx tsc --noEmit` | 0 错误 |
| `APP_ENV=local npx next build` | 通过，`/argus-signals` 21.1 kB / First Load 447 kB |
| `next dev` + `curl /argus-signals` | HTTP 200，编译无错，日志无运行时异常 |

**没能验的**：带真实数据的可视核对。本机没有跑起来的 MySQL/Redis/manager-api，也没有可用无头浏览器（无 playwright/puppeteer/chromium），且 `(console)/layout.tsx` 客户端校验登录态，curl 拿不到正文。覆盖到「编译/类型/路由/构建/页面能起来」，**未覆盖「数据渲染正确」**。

## 对照任务说明

| 要求 | 状态 |
|---|---|
| 多维筛选（实例/账户/事件类型/结果/强度/参数版本） | 已满足，结果筛选含全部具体 `gate_kind` |
| 拦截原因分布 + 强度分级统计 | 已满足，带阈值 vs 实测均值、开仓率 |
| 逐账户判定表还原 `[n]`/`[跳过n]`（账户/结果/张数/均价/成交/门控说明） | 已满足，另附 `telegramLines` 原文块 |
| episode 抽屉：ROI 轨迹 / 事件时间轴 / trail 峰值与回吐比例 / 出场归因 | 已满足。ROI 画虚线+点（离散观测），upl 另起一张（单位 U，不换算 ROI%） |
| 按 config_version 与实例切片对比 | 已满足（独立 Tab，按需加载） |
| 全部实例视图逐行标实例 + 账户重号提示 | 已满足，顶部 Alert 点名实例1/实例3 的 `account1`，两表插入实例列，账户下拉带实例前缀 |
| external_close/manual_close 明确标注不计入策略胜率 | 已满足，三处标注 + KPI 写明胜率分母是 `attributable` 而非 `total` |

## 测试交接

- 入口：`/argus-signals`（Argus 管理 → 信号复盘）。先执行 `doc/module/r9-022661de71/权限资源.sql` 再执行 r13 那份。
- 前置数据：需 MySQL + Redis；`strategy_event` 有数据（r4 双写或 r6 回灌）；episode 视角需先跑 `go run ./cmd/argus-episode-rebuild --instance <key> --apply`，否则列表空并提示「还没派生」。
- 主验收数据集：8/18–8/21（613 次触发 / 20 笔持仓 / 6 次兜底）。
- 建议顺序：先「全部实例 + latest」看跨实例提示与逐行实例列 → 锁定单实例看分布图 → 点开一条 `gate_block` 核对抽屉与当时 TG 消息逐字一致 → 切 episode 视角点开一笔 `trailing_close` 看回吐比例 → 切「切片对比」核对同实例两个版本的行。

## 遗留与风险

1. episode 仍需手工跑 CLI 重建（r10 遗留）。页面已把滞后暴露成顶部 Alert，但不解决滞后本身。
2. `fillPriceAvailable` 恒为 false — 成交价/均价埋点未落库，开仓行两列恒显示「—」，这是埋点缺口不是页面缺陷。
3. `gate-stats` 单次扫 200k 行上限，命中时页面提示缩小窗口但不自动缩窗。测试若选「近 90 天 + 全部实例」看到的分布可能是截断的。
4. 相对时间档锚在**全库** `dataRange.end`，某实例停机很久时锁定它选「近 1 天」会空页；`latest` 档不受影响（服务端按筛选条件回推，更准）。
5. **并行分支冲突面**：本轮施工期间 r12（argus-market）与 r16（signal_optimize）在同一分支并行改动，一度导致 `service/trade` 短暂编译失败（非本任务代码，对方随后补全）。本轮结束时两个 Go 模块与前端构建均为绿；合并前建议再跑一次全量 build + test。
6. 未做：CSV 导出（原型有按钮，任务说明未列入范围）；跳转行情主视图定位触发点的深链（等 r12 页面落地后再接）。

批量判定：完成

<!-- delivery-task-planner-chat-data
eyJ0dXJucyI6W3siaWQiOiJKYWVnZGJGYUgyWjJyY095REIyWVBRIiwic3RhdHVzIjoiY29tcGxldGVkIiwiY3JlYXRlZEF0IjoiMjAyNi0wOS0wMlQwMDo0Nzo0MS45NTQ5NDErMDA6MDAiLCJjb21wbGV0ZWRBdCI6IjIwMjYtMDktMDJUMDE6MTQ6NTMuMDgyMzI4KzAwOjAwIiwiaXRlbXMiOlt7InR5cGUiOiJ1c2VyTWVzc2FnZSIsImNvbnRlbnQiOlt7InR5cGUiOiJ0ZXh0IiwidGV4dCI6IuaJp+ihjOOAjOWKqOS9nOaJp+ihjOOAjemYtuaute+8muS/oeWPt+WkjeebmOS4jiBlcGlzb2RlIOivpuaDhemhtSJ9XX0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6IkknbGwgc3RhcnQgYnkgbG9hZGluZyB0aGUgZGVsaXZlcnkgZXhlY3V0aW9uIHNraWxsIGFuZCB0aGUgcHJvamVjdCdzIGRldmVsb3BtZW50IHNraWxscy4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJJJ2xsIGJlZ2luIGJ5IHJlYWRpbmcgdGhlIHJlcXVpcmVtZW50IGRvYyBhbmQgc3VydmV5aW5nIHRoZSBjdXJyZW50IHN0YXRlLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBsZXQgbWUgc3VydmV5IHRoZSBwcm90b3R5cGUgYW5kIHRoZSBwcmVyZXF1aXNpdGUgdGFzayBvdXRwdXRzIGluIHBhcmFsbGVsLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyBJIGhhdmUgdGhlIGZ1bGwgcGljdHVyZS4gTGV0IG1lIHN0YXJ0IHdpdGggdGhlIGJhY2tlbmQgd2lyaW5nIHRoYXQgcjEwIGxlZnQgaW5jb21wbGV0ZS4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdGhlIEhUVFAgcm91dGVzIGZvciBlcGlzb2RlcyBhbmQgc2xpY2UtY29tcGFyZS4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJOb3cgdXBkYXRlIHRoZSByb3V0ZSBjaGVjayB0ZXN0IHRvIGNvdmVyIHRoZSBuZXcgZW5kcG9pbnRzLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgY29uc3RhbnRzIGFuZCBmb3JtYXR0aW5nIGhlbHBlcnMuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBmaWx0ZXIgYmFyLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgc3RhdHMgcGFuZWxzIChLUEkgdGlsZXMgKyDmi6bmiKrljp/lm6DliIbluIMgKyDlvLrluqbliIbnuqcgKyDlh7rlnLrlvZLlm6ApLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgc2lnbmFsIHRhYmxlLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgdHdvIGRyYXdlcnMg4oCUIHRoZSBzaWduYWwgZGV0YWlsIChURyBtZXNzYWdlIHJlY29uc3RydWN0aW9uKSBhbmQgdGhlIGVwaXNvZGUgbGlmZWN5Y2xlLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgc2xpY2UtY29tcGFyZSB0YWJsZSBhbmQgdGhlIHBhZ2UgaXRzZWxmLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgQ1NTIGFkZGl0aW9ucyBhbmQgbWVudSByZWdpc3RyYXRpb24uIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHR5cGUtY2hlY2sgdGhlIGZyb250ZW5kLiIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiIn0seyJ0eXBlIjoiYWdlbnRNZXNzYWdlIiwidGV4dCI6Ik5vdyB0aGUgcGVybWlzc2lvbiBTUUwgZm9yIHRoZSBuZXcgZW5kcG9pbnRzIHBsdXMgdGhlIHBhZ2UgcmVzb3VyY2UuIiwic3RhdHVzIjoiY29tcGxldGVkIiwicGhhc2UiOiIifSx7InR5cGUiOiJhZ2VudE1lc3NhZ2UiLCJ0ZXh0IjoiTm93IHRoZSBkZXNpZ24gcHJvY2VzcyBkb2N1bWVudC4iLCJzdGF0dXMiOiJjb21wbGV0ZWQiLCJwaGFzZSI6IiJ9LHsidHlwZSI6ImFnZW50TWVzc2FnZSIsInRleHQiOiJBbGwgd29yayBpcyBjb21wbGV0ZSBhbmQgdmVyaWZpZWQuXG5cbiMjIOaUueS6huS7gOS5iFxuXG4qKuWQjuerryDigJQg6KGl5a6MIHIxMCDpgZfnlZnnmoQgZXBpc29kZSDor7vot6/lvoTmjqXnur8qKu+8iHIxMCDorr7orqHmlofmoaMgwqfpgZfnlZnpobnmmI7noa7lhpnkuobjgIxlcGlzb2RlIOeahOivuyBBUEkg5LiN5Zyo5pys5Lu75Yqh6IyD5Zu077yM5bqU6K+l5ZyoIHIxMyDpgqPkuIDliIDph4zliqDjgI3jgILlvIDlt6Xml7YgYHNlcnZpY2UvYXJndXNfZXZlbnRgIOaciSA5IOWkhOe8luivkemUmeivr++8iVxuXG4tIGBzZXJ2ZXIvc2VydmljZS9hcmd1c19ldmVudC9zZXJ2aWNlLmdvYCDigJQg5YqgIGBFcnJFcGlzb2RlTm90Rm91bmRgIOWTqOWFte+8m2BBcmd1c0V2ZW50U2VydmljZWAg5YqgIGBlcGlzb2RlUmVwb3NpdG9yeWAgLyBgZXBpc29kZUVudHJ5UmVwb3NpdG9yeWAg5Lik5Liq5Y+q6K+75LuT5YKo5bm25Zyo5p6E6YCg5Ye95pWw6YeM5o6l5LiK77ybYEVuc3VyZVRhYmxlKClgIOi/veWKoOW7uuS4pOW8oOa0vueUn+ihqO+8iOayoei3kei/h+mHjeW7uuaXtumhtemdoueci+WIsOepuuihqCvmj5DnpLrvvIzogIzkuI3mmK/mnKrnn6XooaggNTAw77yJ44CCXG4tIGBzZXJ2ZXIvbWFuYWdlci1hcGkvcGtnL2FyZ3VzX2V2ZW50L2FyZ3VzX2V2ZW50LmdvYCDigJQg6L+95YqgIDUg5p2h5Y+q6K+7IEdFVO+8mmAvZXBpc29kZXNg44CBYC9lcGlzb2Rlcy86aWRg44CBYC9lcGlzb2RlLXN0YXRzYOOAgWAvZXBpc29kZS1leGl0LWtpbmRzYOOAgWAvc2xpY2UtY29tcGFyZWDjgILlh7rlnLrmlrnlvI/pgInpobnotbDni6znq4vot6/lvoTogIzpnZ4gYC9lcGlzb2Rlcy9leGl0LWtpbmRzYO+8jOmBv+WFjei3nyBgOmlkYCDmiqLmrrXjgIJcbi0gYHNlcnZlci9tYW5hZ2VyLWFwaS9wa2cvYXJndXNfZXZlbnQvcm91dGVfY2hlY2tfdGVzdC5nb2Ag4oCUIOacn+acm+i3r+eUseihqOihpem9kCA1IOadoeOAglxuXG4qKuWJjeerryDigJQg5paw5aKeIGBjbGllbnQvbWFuYWdlci9zcmMvYXBwLyhjb25zb2xlKS9hcmd1cy1zaWduYWxzL2AqKu+8iDEyIOS4quaWh+S7tu+8iVxuXG5gYXBpL2FyZ3VzLXNpZ25hbHMuYXBpLnRzYO+8iDIxIOS4qiBEVE8gKyA5IOS4quivt+axgu+8ieOAgWBjb25zdGFudHMudHNg44CBYGhvb2tzL3VzZUFyZ3VzU2lnbmFscy50c2DvvIxjb21wb25lbnRzIOS4iyBgU2lnbmFsRmlsdGVyQmFyYCAvIGBSZXZpZXdTdGF0c2AgLyBgU2lnbmFsVGFibGVgIC8gYEVwaXNvZGVUYWJsZWAgLyBgU2lnbmFsRGV0YWlsRHJhd2VyYCAvIGBFcGlzb2RlRHJhd2VyYCAvIGBTbGljZUNvbXBhcmVUYWJsZWAgLyBgVHJhY2tDaGFydGDvvIzku6Xlj4ogYHBhZ2UudHN4YO+8iOS/oeWPtyAvIGVwaXNvZGUgLyDliIfniYflr7nmr5Qg5LiJ6KeG6KeS77yJ44CCXG5cbioq5YWx5Lqr5paH5Lu25Y+q5YGa6L+95YqgKirvvJpgZ2xvYmFscy5jc3NgIOacq+Wwvui/veWKoCByMTMg5qC35byP5q6177yI5YWo5paw57G75ZCN77yJ77ybYE1hbmFnZXJTaGVsbC50c3hgIOWKoOS4gOS4quiPnOWNlemhuSArIOagh+mimCArIOWxleW8gOWIhuaUr++8iOiPnOWNlemHjee7hOaYryByMTUg55qE5LqL77yM5pys6L2u5LiN5Yqo77yJ44CCXG5cbioq5paH5qGj5LiO5p2D6ZmQKirvvJpbZG9jL21vZHVsZS9yMTMtMDU4NWQ1ZjM5Yi9kZXNpZ24v6K6+6K6h6L+H56iLLm1kXShkb2MvbW9kdWxlL3IxMy0wNTg1ZDVmMzliL2Rlc2lnbi/orr7orqHov4fnqIsubWQp44CBW2RvYy9tb2R1bGUvcjEzLTA1ODVkNWYzOWIv5p2D6ZmQ6LWE5rqQLnNxbF0oZG9jL21vZHVsZS9yMTMtMDU4NWQ1ZjM5Yi/mnYPpmZDotYTmupAuc3FsKVxuXG4jIyDmgI7kuYjpqoznmoRcblxufCDpqozor4EgfCDnu5PmnpwgfFxufC0tLXwtLS18XG58IGBjZCBzZXJ2ZXIvc2VydmljZSAmJiBnbyBidWlsZCAuLy4uLmAgfCDpgJrov4fvvIjkv67lpI3liY0gOSDlpITnvJbor5HplJnor6/vvIkgfFxufCBgZ28gdGVzdCAuL2FyZ3VzX2V2ZW50Ly4uLmAgfCBgb2sgc2VydmljZS9hcmd1c19ldmVudGAgfFxufCBgY2Qgc2VydmVyL21hbmFnZXItYXBpICYmIGdvIGJ1aWxkIC4vLi4uYCB8IOmAmui/hyB8XG58IGBnbyB0ZXN0IC4vcGtnL2FyZ3VzX2V2ZW50Ly4uLmAgfCBgb2tg77yIMTMg5p2hIEdFVCDlhajms6jlhozjgIHml6AgZ2luIOWGsueqge+8iSB8XG58IGBucHggdHNjIC0tbm9FbWl0YCB8IDAg6ZSZ6K+vIHxcbnwgYEFQUF9FTlY9bG9jYWwgbnB4IG5leHQgYnVpbGRgIHwg6YCa6L+H77yMYC9hcmd1cy1zaWduYWxzYCAyMS4xIGtCIC8gRmlyc3QgTG9hZCA0NDcga0IgfFxufCBgbmV4dCBkZXZgICsgYGN1cmwgL2FyZ3VzLXNpZ25hbHNgIHwgSFRUUCAyMDDvvIznvJbor5Hml6DplJnvvIzml6Xlv5fml6Dov5DooYzml7blvILluLggfFxuXG4qKuayoeiDvemqjOeahCoq77ya5bim55yf5a6e5pWw5o2u55qE5Y+v6KeG5qC45a+544CC5pys5py65rKh5pyJ6LeR6LW35p2l55qEIE15U1FML1JlZGlzL21hbmFnZXItYXBp77yM5Lmf5rKh5pyJ5Y+v55So5peg5aS05rWP6KeI5Zmo77yI5pegIHBsYXl3cmlnaHQvcHVwcGV0ZWVyL2Nocm9taXVt77yJ77yM5LiUIGAoY29uc29sZSkvbGF5b3V0LnRzeGAg5a6i5oi356uv5qCh6aqM55m75b2V5oCB77yMY3VybCDmi7/kuI3liLDmraPmlofjgILopobnm5bliLDjgIznvJbor5Ev57G75Z6LL+i3r+eUsS/mnoTlu7ov6aG16Z2i6IO96LW35p2l44CN77yMKirmnKropobnm5bjgIzmlbDmja7muLLmn5PmraPnoa7jgI0qKuOAglxuXG4jIyDlr7nnhafku7vliqHor7TmmI5cblxufCDopoHmsYIgfCDnirbmgIEgfFxufC0tLXwtLS18XG58IOWkmue7tOetm+mAie+8iOWunuS+iy/otKbmiLcv5LqL5Lu257G75Z6LL+e7k+aenC/lvLrluqYv5Y+C5pWw54mI5pys77yJIHwg5bey5ruh6Laz77yM57uT5p6c562b6YCJ5ZCr5YWo6YOo5YW35L2TIGBnYXRlX2tpbmRgIHxcbnwg5oum5oiq5Y6f5Zug5YiG5biDICsg5by65bqm5YiG57qn57uf6K6hIHwg5bey5ruh6Laz77yM5bim6ZiI5YC8IHZzIOWunua1i+Wdh+WAvOOAgeW8gOS7k+eOhyB8XG58IOmAkOi0puaIt+WIpOWumuihqOi/mOWOnyBgW25dYC9gW+i3s+i/h25dYO+8iOi0puaIty/nu5Pmnpwv5byg5pWwL+Wdh+S7ty/miJDkuqQv6Zeo5o6n6K+05piO77yJIHwg5bey5ruh6Laz77yM5Y+m6ZmEIGB0ZWxlZ3JhbUxpbmVzYCDljp/mloflnZcgfFxufCBlcGlzb2RlIOaKveWxie+8mlJPSSDovajov7kgLyDkuovku7bml7bpl7TovbQgLyB0cmFpbCDls7DlgLzkuI7lm57lkJDmr5TkvosgLyDlh7rlnLrlvZLlm6AgfCDlt7Lmu6HotrPjgIJST0kg55S76Jma57q/K+eCue+8iOemu+aVo+ingua1i++8ie+8jHVwbCDlj6botbfkuIDlvKDvvIjljZXkvY0gVe+8jOS4jeaNoueulyBST0kl77yJIHxcbnwg5oyJIGNvbmZpZ192ZXJzaW9uIOS4juWunuS+i+WIh+eJh+WvueavlCB8IOW3sua7oei2s++8iOeLrOeriyBUYWLvvIzmjInpnIDliqDovb3vvIkgfFxufCDlhajpg6jlrp7kvovop4blm77pgJDooYzmoIflrp7kvosgKyDotKbmiLfph43lj7fmj5DnpLogfCDlt7Lmu6HotrPvvIzpobbpg6ggQWxlcnQg54K55ZCN5a6e5L6LMS/lrp7kvoszIOeahCBgYWNjb3VudDFg77yM5Lik6KGo5o+S5YWl5a6e5L6L5YiX77yM6LSm5oi35LiL5ouJ5bim5a6e5L6L5YmN57yAIHxcbnwgZXh0ZXJuYWxfY2xvc2UvbWFudWFsX2Nsb3NlIOaYjuehruagh+azqOS4jeiuoeWFpeetlueVpeiDnOeOhyB8IOW3sua7oei2s++8jOS4ieWkhOagh+azqCArIEtQSSDlhpnmmI7og5znjofliIbmr43mmK8gYGF0dHJpYnV0YWJsZWAg6ICM6Z2eIGB0b3RhbGAgfFxuXG4jIyDmtYvor5XkuqTmjqVcblxuLSDlhaXlj6PvvJpgL2FyZ3VzLXNpZ25hbHNg77yIQXJndXMg566h55CGIOKGkiDkv6Hlj7flpI3nm5jvvInjgILlhYjmiafooYwgYGRvYy9tb2R1bGUvcjktMDIyNjYxZGU3MS/mnYPpmZDotYTmupAuc3FsYCDlho3miafooYwgcjEzIOmCo+S7veOAglxuLSDliY3nva7mlbDmja7vvJrpnIAgTXlTUUwgKyBSZWRpc++8m2BzdHJhdGVneV9ldmVudGAg5pyJ5pWw5o2u77yIcjQg5Y+M5YaZ5oiWIHI2IOWbnueBjO+8ie+8m2VwaXNvZGUg6KeG6KeS6ZyA5YWI6LeRIGBnbyBydW4gLi9jbWQvYXJndXMtZXBpc29kZS1yZWJ1aWxkIC0taW5zdGFuY2UgPGtleT4gLS1hcHBseWDvvIzlkKbliJnliJfooajnqbrlubbmj5DnpLrjgIzov5jmsqHmtL7nlJ/jgI3jgIJcbi0g5Li76aqM5pS25pWw5o2u6ZuG77yaOC8xOOKAkzgvMjHvvIg2MTMg5qyh6Kem5Y+RIC8gMjAg56yU5oyB5LuTIC8gNiDmrKHlhZzlupXvvInjgIJcbi0g5bu66K6u6aG65bqP77ya5YWI44CM5YWo6YOo5a6e5L6LICsgbGF0ZXN044CN55yL6Leo5a6e5L6L5o+Q56S65LiO6YCQ6KGM5a6e5L6L5YiXIOKGkiDplIHlrprljZXlrp7kvovnnIvliIbluIPlm74g4oaSIOeCueW8gOS4gOadoSBgZ2F0ZV9ibG9ja2Ag5qC45a+55oq95bGJ5LiO5b2T5pe2IFRHIOa2iOaBr+mAkOWtl+S4gOiHtCDihpIg5YiHIGVwaXNvZGUg6KeG6KeS54K55byA5LiA56yUIGB0cmFpbGluZ19jbG9zZWAg55yL5Zue5ZCQ5q+U5L6LIOKGkiDliIfjgIzliIfniYflr7nmr5TjgI3moLjlr7nlkIzlrp7kvovkuKTkuKrniYjmnKznmoTooYzjgIJcblxuIyMg6YGX55WZ5LiO6aOO6ZmpXG5cbjEuIGVwaXNvZGUg5LuN6ZyA5omL5bel6LeRIENMSSDph43lu7rvvIhyMTAg6YGX55WZ77yJ44CC6aG16Z2i5bey5oqK5rue5ZCO5pq06Zyy5oiQ6aG26YOoIEFsZXJ077yM5L2G5LiN6Kej5Yaz5rue5ZCO5pys6Lqr44CCXG4yLiBgZmlsbFByaWNlQXZhaWxhYmxlYCDmgZLkuLogZmFsc2Ug4oCUIOaIkOS6pOS7ty/lnYfku7fln4vngrnmnKrokL3lupPvvIzlvIDku5PooYzkuKTliJfmgZLmmL7npLrjgIzigJTjgI3vvIzov5nmmK/ln4vngrnnvLrlj6PkuI3mmK/pobXpnaLnvLrpmbfjgIJcbjMuIGBnYXRlLXN0YXRzYCDljZXmrKHmiasgMjAwayDooYzkuIrpmZDvvIzlkb3kuK3ml7bpobXpnaLmj5DnpLrnvKnlsI/nqpflj6PkvYbkuI3oh6rliqjnvKnnqpfjgILmtYvor5Xoi6XpgInjgIzov5EgOTAg5aSpICsg5YWo6YOo5a6e5L6L44CN55yL5Yiw55qE5YiG5biD5Y+v6IO95piv5oiq5pat55qE44CCXG40LiDnm7jlr7nml7bpl7TmoaPplJrlnKgqKuWFqOW6kyoqIGBkYXRhUmFuZ2UuZW5kYO+8jOafkOWunuS+i+WBnOacuuW+iOS5heaXtumUgeWumuWug+mAieOAjOi/kSAxIOWkqeOAjeS8muepuumhte+8m2BsYXRlc3RgIOaho+S4jeWPl+W9seWTje+8iOacjeWKoeerr+aMieetm+mAieadoeS7tuWbnuaOqO+8jOabtOWHhu+8ieOAglxuNS4gKirlubbooYzliIbmlK/lhrLnqoHpnaIqKu+8muacrOi9ruaWveW3peacn+mXtCByMTLvvIhhcmd1cy1tYXJrZXTvvInkuI4gcjE277yIc2lnbmFsX29wdGltaXpl77yJ5Zyo5ZCM5LiA5YiG5pSv5bm26KGM5pS55Yqo77yM5LiA5bqm5a+86Ie0IGBzZXJ2aWNlL3RyYWRlYCDnn63mmoLnvJbor5HlpLHotKXvvIjpnZ7mnKzku7vliqHku6PnoIHvvIzlr7nmlrnpmo/lkI7ooaXlhajvvInjgILmnKzova7nu5PmnZ/ml7bkuKTkuKogR28g5qih5Z2X5LiO5YmN56uv5p6E5bu65Z2H5Li657u/77yb5ZCI5bm25YmN5bu66K6u5YaN6LeR5LiA5qyh5YWo6YePIGJ1aWxkICsgdGVzdOOAglxuNi4g5pyq5YGa77yaQ1NWIOWvvOWHuu+8iOWOn+Wei+acieaMiemSru+8jOS7u+WKoeivtOaYjuacquWIl+WFpeiMg+WbtO+8ie+8m+i3s+i9rOihjOaDheS4u+inhuWbvuWumuS9jeinpuWPkeeCueeahOa3semTvu+8iOetiSByMTIg6aG16Z2i6JC95Zyw5ZCO5YaN5o6l77yJ44CCXG5cbuaJuemHj+WIpOWumu+8muWujOaIkCIsInN0YXR1cyI6ImNvbXBsZXRlZCIsInBoYXNlIjoiZmluYWxfYW5zd2VyIn1dfV19
-->
