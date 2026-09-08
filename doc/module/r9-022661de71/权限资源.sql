-- r9-022661de71 manager-api 新增 argus_event 事件查询接口 · 权限资源补齐
--
-- 用法：把下面的 @role_id 改成实际要授权的角色主键后整段执行。全部语句幂等，可重复跑。
-- 本轮没有指定 role_id，沿用 r7 的写法默认给 1，执行前请确认它就是目标角色。
--
-- 只补接口资源：本任务不做前端页面（行情主视图 r12 / 信号复盘 r13 / 总览 r15
-- 各自补自己的 page 资源），这里凭空造页面资源只会让授权树里出现打不开的菜单。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 接口资源（全部只读 GET）
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
-- 2) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN (
    'argus_event:options',
    'argus_event:signal:list',
    'argus_event:signal:detail',
    'argus_event:signal:slice',
    'argus_event:timeline',
    'argus_event:equity',
    'argus_event:gate:stats',
    'argus_event:instance:summary'
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
