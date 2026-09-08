-- r14-0691d7c793 盘口信号回测与寻优可视化页 · 权限资源补齐
--
-- 用法：执行前请确认 @role_id 是要授权的管理角色。任务没有给出角色主键，沿用
-- r7/r12/r13 的管理角色默认值 1；若实际角色不同，只改下面这一行即可。
-- 所有新增资源和绑定均幂等。本文件不包含任何进程控制、发布或自动上线权限。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 盘口信号回测接口
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

-- ---------------------------------------------------------------------------
-- 2) 自动参数寻优接口（扫描只创建研究任务；不含任何自动发布能力）
-- ---------------------------------------------------------------------------
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

-- ---------------------------------------------------------------------------
-- 3) 页面资源
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 盘口信号回测', 'argus_backtest:page', 0, 'page', '', '/argus-backtest', '', '', 'Argus 盘口信号回测', '', 150, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:page' AND page_url = '/argus-backtest' AND active = 1);

INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 自动参数寻优', 'argus_optimizer:page', 0, 'page', '', '/argus-optimizer', '', '', 'Argus 自动参数寻优', '', 151, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_optimizer:page' AND page_url = '/argus-optimizer' AND active = 1);

-- ---------------------------------------------------------------------------
-- 4) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN (
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
    'argus_optimizer:page'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
