-- =============================================================================
-- Argus 管理端权限资源补齐 · 合并脚本
--
-- 生成时间：2026-09-09
-- 来源：doc/module/{r7,r8,r9,r12,r13,r14,r15}-*/权限资源.sql 七个文件机械合并
-- 覆盖：42 条资源 + 42 条角色绑定（已交叉核对，无重复 code、无孤立绑定）
--
-- 背景：这些接口代码早已上线，但资源行没写进生产库，manager-api 的
--       hadResource() 走 resource_new + role_resource_new 双表精确匹配，
--       匹配不上就返回 {"success":false,"message":"user not resource"}。
--       实测生产上 18 个只读接口被拦。
--
-- 幂等：所有 INSERT 都带 NOT EXISTS 守卫，可重复执行。
-- 事务：整段包在一个事务里，中途报错会全部回滚。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 第 0 步：执行前自检（只读，先单独跑这段，确认输出符合预期再往下）
-- ---------------------------------------------------------------------------
-- 期望只有一行：id=1 / code=admin。若你的目标角色不是 1，改下面的 @role_id。
SELECT id, name, code, active FROM role WHERE active = 1;

-- 当前已生效的 argus/backtest 接口资源（补齐前应远少于 42 条）
SELECT COUNT(*) AS 现有资源数
FROM resource_new
WHERE active = 1
  AND resource_type = 'api'
  AND (resource_url LIKE '/argus%' OR resource_url LIKE '/backtest%' OR resource_url LIKE '/klines%');


-- ---------------------------------------------------------------------------
-- 第 1 步：正式补齐
-- ---------------------------------------------------------------------------
START TRANSACTION;

SET @role_id = 1;

-- 角色存在性开关：为 0 时下面的绑定 INSERT 一条都不会插，避免把资源绑到空角色上。
-- 纯 SQL 没法在这里直接中断，所以做成开关 + 打印，请看一眼输出再继续。
SET @role_ok = (SELECT COUNT(*) FROM role WHERE id = @role_id AND active = 1);
SELECT @role_id AS 目标角色, IF(@role_ok = 1, '存在，可继续', '不存在或已停用 —— 请 ROLLBACK 并改 @role_id') AS 自检结果;

-- ---------------------------------------------------------------------------
-- 1.1 下线已从 manager-api 移除的接口（来自 r7）
--     代码里 /argus/runtime/{start,stop,restart} 三个路由已删除（已核实 grep 为空），
--     残留资源会让角色授权树里出现打不开的"停进程"入口。
-- ---------------------------------------------------------------------------
UPDATE resource_new
SET active = 0
WHERE active = 1
  AND resource_url IN ('/argus/runtime/start', '/argus/runtime/stop', '/argus/runtime/restart');

UPDATE role_resource_new rr
JOIN resource_new r ON r.id = rr.resource_id
SET rr.active = 0
WHERE rr.active = 1
  AND r.resource_url IN ('/argus/runtime/start', '/argus/runtime/stop', '/argus/runtime/restart');


-- ---------------------------------------------------------------------------
-- 1.2 r7-f74ca0f149 — argus-config 参数与运行控制页（10 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 实例列表', 'argus_config:instance:list', 0, 'api', '/argus-config/instances', '', '', '', '', '', 100, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:instance:list' AND resource_url = '/argus-config/instances' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 实例注册', 'argus_config:instance:create', 0, 'api', '/argus-config/instances', '', '', '', '', '', 101, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:instance:create' AND resource_url = '/argus-config/instances' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 已发布配置', 'argus_config:published', 0, 'api', '/argus-config/published', '', '', '', '', '', 102, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:published' AND resource_url = '/argus-config/published' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 版本历史', 'argus_config:version:list', 0, 'api', '/argus-config/versions', '', '', '', '', '', 103, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:version:list' AND resource_url = '/argus-config/versions' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 保存配置草稿', 'argus_config:draft:create', 0, 'api', '/argus-config/drafts', '', '', '', '', '', 104, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:draft:create' AND resource_url = '/argus-config/drafts' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 发布配置版本', 'argus_config:version:publish', 0, 'api', '/argus-config/versions/:id/publish', '', '', '', '', '', 105, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:version:publish' AND resource_url = '/argus-config/versions/:id/publish' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 回滚配置版本', 'argus_config:version:rollback', 0, 'api', '/argus-config/versions/:id/rollback', '', '', '', '', '', 106, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:version:rollback' AND resource_url = '/argus-config/versions/:id/rollback' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 运行状态', 'argus_runtime:status', 0, 'api', '/argus/runtime/status', '', '', '', '', '', 107, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_runtime:status' AND resource_url = '/argus/runtime/status' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 立即热加载', 'argus_runtime:reload', 0, 'api', '/argus/runtime/reload', '', '', '', '', '', 108, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_runtime:reload' AND resource_url = '/argus/runtime/reload' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 参数与运行控制', 'argus_config:page', 0, 'page', '', '/argus-config', '', '', 'Argus 参数与运行控制', '', 109, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:page' AND page_url = '/argus-config' AND active = 1);


-- ---------------------------------------------------------------------------
-- 1.3 r8-eaf16d12d7 — 信号回测提交接口（1 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '提交盘口信号回测', 'argus_backtest:signal_run:create', 0, 'api', '/backtest/signal-runs', '', '', '', '', '', 139, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_run:create' AND resource_url = '/backtest/signal-runs' AND active = 1);


-- ---------------------------------------------------------------------------
-- 1.4 r9-022661de71 — argus_event 事件查询接口（8 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 事件筛选项', 'argus_event:options', 0, 'api', '/argus-event/filter-options', '', '', '', '', '', 120, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:options' AND resource_url = '/argus-event/filter-options' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 信号列表', 'argus_event:signal:list', 0, 'api', '/argus-event/signals', '', '', '', '', '', 121, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:signal:list' AND resource_url = '/argus-event/signals' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 信号详情', 'argus_event:signal:detail', 0, 'api', '/argus-event/signals/:id', '', '', '', '', '', 122, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:signal:detail' AND resource_url = '/argus-event/signals/:id' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 触发瞬间秒级切片', 'argus_event:signal:slice', 0, 'api', '/argus-event/signals/:id/slice', '', '', '', '', '', 123, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:signal:slice' AND resource_url = '/argus-event/signals/:id/slice' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 时间轴聚合', 'argus_event:timeline', 0, 'api', '/argus-event/timeline', '', '', '', '', '', 124, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:timeline' AND resource_url = '/argus-event/timeline' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 权益曲线', 'argus_event:equity', 0, 'api', '/argus-event/equity-curve', '', '', '', '', '', 125, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:equity' AND resource_url = '/argus-event/equity-curve' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 拦截原因聚合', 'argus_event:gate:stats', 0, 'api', '/argus-event/gate-stats', '', '', '', '', '', 126, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:gate:stats' AND resource_url = '/argus-event/gate-stats' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 跨实例汇总对比', 'argus_event:instance:summary', 0, 'api', '/argus-event/instance-summary', '', '', '', '', '', 127, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:instance:summary' AND resource_url = '/argus-event/instance-summary' AND active = 1);


-- ---------------------------------------------------------------------------
-- 1.5 r12-f9de15b4a5 — 行情主视图与 K 线回填（2 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'K 线窗口回填', 'trade_kline:backfill:range', 0, 'api', '/klines/backfill-range', '', '', '', '', '', 130, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'trade_kline:backfill:range' AND resource_url = '/klines/backfill-range' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 历史行情与触发点', 'argus_market:page', 0, 'page', '', '/argus-market', '', '', '历史行情与触发点', '', 136, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_market:page'
    AND page_url = '/argus-market'
    AND active = 1
);


-- ---------------------------------------------------------------------------
-- 1.6 r13-0585d5f39b — 信号复盘 / episode（6 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 持仓列表', 'argus_event:episode:list', 0, 'api', '/argus-event/episodes', '', '', '', '', '', 128, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:episode:list' AND resource_url = '/argus-event/episodes' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 持仓生命周期详情', 'argus_event:episode:detail', 0, 'api', '/argus-event/episodes/:id', '', '', '', '', '', 129, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:episode:detail' AND resource_url = '/argus-event/episodes/:id' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 出场归因分布', 'argus_event:episode:stats', 0, 'api', '/argus-event/episode-stats', '', '', '', '', '', 130, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:episode:stats' AND resource_url = '/argus-event/episode-stats' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 出场方式筛选项', 'argus_event:episode:exitkinds', 0, 'api', '/argus-event/episode-exit-kinds', '', '', '', '', '', 131, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:episode:exitkinds' AND resource_url = '/argus-event/episode-exit-kinds' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 参数版本切片对比', 'argus_event:slice:compare', 0, 'api', '/argus-event/slice-compare', '', '', '', '', '', 132, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_event:slice:compare' AND resource_url = '/argus-event/slice-compare' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 信号复盘', 'argus_signals:page', 0, 'page', '', '/argus-signals', '', '', 'Argus 信号复盘', '', 111, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_signals:page' AND page_url = '/argus-signals' AND active = 1);


-- ---------------------------------------------------------------------------
-- 1.7 r14-0691d7c793 — 回测批次与参数寻优（12 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '盘口信号源列表', 'argus_backtest:signal_source:list', 0, 'api', '/backtest/signal-sources', '', '', '', '', '', 140, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_source:list' AND resource_url = '/backtest/signal-sources' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '盘口信号生产基线预览', 'argus_backtest:signal_baseline:preview', 0, 'api', '/backtest/signal-baseline', '', '', '', '', '', 141, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_baseline:preview' AND resource_url = '/backtest/signal-baseline' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '提交盘口信号批量回测', 'argus_backtest:signal_batch:create', 0, 'api', '/backtest/signal-batches', '', '', '', '', '', 142, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_batch:create' AND resource_url = '/backtest/signal-batches' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '盘口信号批量回测列表', 'argus_backtest:signal_batch:list', 0, 'api', '/backtest/signal-batches', '', '', '', '', '', 143, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_batch:list' AND resource_url = '/backtest/signal-batches' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '盘口信号批量回测详情', 'argus_backtest:signal_batch:detail', 0, 'api', '/backtest/signal-batches/:id', '', '', '', '', '', 144, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_batch:detail' AND resource_url = '/backtest/signal-batches/:id' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '盘口信号回测逐笔详情', 'argus_backtest:run:detail', 0, 'api', '/backtest/runs/:id', '', '', '', '', '', 145, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:run:detail' AND resource_url = '/backtest/runs/:id' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '自动寻优默认协议预览', 'argus_optimizer:defaults', 0, 'api', '/backtest/optimize-defaults', '', '', '', '', '', 146, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:defaults' AND resource_url = '/backtest/optimize-defaults' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '发起自动参数寻优', 'argus_optimizer:study:create', 0, 'api', '/backtest/optimize-studies', '', '', '', '', '', 147, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:study:create' AND resource_url = '/backtest/optimize-studies' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '自动参数寻优任务列表', 'argus_optimizer:study:list', 0, 'api', '/backtest/optimize-studies', '', '', '', '', '', 148, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:study:list' AND resource_url = '/backtest/optimize-studies' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '自动参数寻优任务详情', 'argus_optimizer:study:detail', 0, 'api', '/backtest/optimize-studies/:id', '', '', '', '', '', 149, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:study:detail' AND resource_url = '/backtest/optimize-studies/:id' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 盘口信号回测', 'argus_backtest:page', 0, 'page', '', '/argus-backtest', '', '', 'Argus 盘口信号回测', '', 150, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:page' AND page_url = '/argus-backtest' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 自动参数寻优', 'argus_optimizer:page', 0, 'page', '', '/argus-optimizer', '', '', 'Argus 自动参数寻优', '', 151, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:page' AND page_url = '/argus-optimizer' AND active = 1);


-- ---------------------------------------------------------------------------
-- 1.8 r15-27b9e2f39a — 多实例总览（3 条）
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 实例运行概览', 'argus_config:instance:overview', 0, 'api', '/argus-config/instance-overview', '', '', '', '', '', 133, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_config:instance:overview'
    AND resource_url = '/argus-config/instance-overview'
    AND active = 1
);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 总览', 'argus_dashboard:page', 0, 'page', '', '/argus-dashboard', '', '', 'Argus 总览', '', 134, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_dashboard:page'
    AND page_url = '/argus-dashboard'
    AND active = 1
);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 实例与参数对比', 'argus_instances:page', 0, 'page', '', '/argus-instances', '', '', '实例与参数对比', '', 135, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_instances:page'
    AND page_url = '/argus-instances'
    AND active = 1
);


-- ---------------------------------------------------------------------------
-- 1.9 角色绑定（42 条，合并七个文件的绑定清单）
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND @role_ok = 1
  AND r.code IN (
    'argus_config:instance:list',
    'argus_config:instance:create',
    'argus_config:published',
    'argus_config:version:list',
    'argus_config:draft:create',
    'argus_config:version:publish',
    'argus_config:version:rollback',
    'argus_runtime:status',
    'argus_runtime:reload',
    'argus_config:page',
    'argus_backtest:signal_run:create',
    'argus_event:options',
    'argus_event:signal:list',
    'argus_event:signal:detail',
    'argus_event:signal:slice',
    'argus_event:timeline',
    'argus_event:equity',
    'argus_event:gate:stats',
    'argus_event:instance:summary',
    'trade_kline:backfill:range',
    'argus_market:page',
    'argus_event:episode:list',
    'argus_event:episode:detail',
    'argus_event:episode:stats',
    'argus_event:episode:exitkinds',
    'argus_event:slice:compare',
    'argus_signals:page',
    'argus_backtest:signal_source:list',
    'argus_backtest:signal_baseline:preview',
    'argus_backtest:signal_batch:create',
    'argus_backtest:signal_batch:list',
    'argus_backtest:signal_batch:detail',
    'argus_backtest:run:detail',
    'argus_optimizer:defaults',
    'argus_optimizer:study:create',
    'argus_optimizer:study:list',
    'argus_optimizer:study:detail',
    'argus_backtest:page',
    'argus_optimizer:page',
    'argus_config:instance:overview',
    'argus_dashboard:page',
    'argus_instances:page'
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );

COMMIT;


-- ---------------------------------------------------------------------------
-- 第 2 步：执行后校验
-- ---------------------------------------------------------------------------
-- 期望 35 条 api 资源全部绑定（另有 7 条 page 资源不在此计数内，42 = 35 + 7）
SELECT COUNT(*) AS 已绑定资源数
FROM resource_new r
JOIN role_resource_new rr ON rr.resource_id = r.id AND rr.active = 1 AND rr.role_id = 1
WHERE r.active = 1
  AND r.resource_type = 'api'
  AND (r.resource_url LIKE '/argus%' OR r.resource_url LIKE '/backtest%' OR r.resource_url LIKE '/klines%');

-- 期望 0 行：列出仍然没有绑定的接口资源
SELECT r.resource_url, r.code
FROM resource_new r
WHERE r.active = 1
  AND r.resource_type = 'api'
  AND (r.resource_url LIKE '/argus%' OR r.resource_url LIKE '/backtest%' OR r.resource_url LIKE '/klines%')
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.resource_id = r.id AND rr.role_id = 1 AND rr.active = 1
  )
ORDER BY r.resource_url;

-- 期望 0 行：同一个 URL 出现多条生效资源（findResourceByURL 取 First，重复会让判定不确定）
-- 注意 /argus-config/instances 天然有两条（GET list + POST create 共用 URL），
-- 出现在这里属预期，其它 URL 若上榜说明重复插入了。
SELECT resource_url, COUNT(*) AS 条数, GROUP_CONCAT(code) AS codes
FROM resource_new
WHERE active = 1 AND resource_type = 'api'
  AND (resource_url LIKE '/argus%' OR resource_url LIKE '/backtest%' OR resource_url LIKE '/klines%')
GROUP BY resource_url HAVING COUNT(*) > 1;
