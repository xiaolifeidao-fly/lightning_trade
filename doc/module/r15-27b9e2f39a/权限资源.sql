-- r15-27b9e2f39a Argus 总览页、实例对比页与菜单重组 · 权限资源补齐
--
-- 用法：执行前把 @role_id 确认为目标角色的真实主键。任务未给 role_id，沿用 r7/r9/r12/r13
-- 的可审阅模板默认值 1；未经权限管理员确认不要直接执行。所有 INSERT 都是幂等的。
--
-- 前置 SQL：
--   r7  已登记 /argus-config/instances、/published 等配置资源；
--   r9  已登记 /argus-event/filter-options、signals、gate-stats、equity-curve、instance-summary 等只读资源。
-- 本任务只新增实例概览接口与两个页面资源，不重复造同路径资源。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 新增只读接口资源
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 实例运行概览', 'argus_config:instance:overview', 0, 'api', '/argus-config/instance-overview', '', '', '', '', '', 133, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_config:instance:overview'
    AND resource_url = '/argus-config/instance-overview'
    AND active = 1
);

-- ---------------------------------------------------------------------------
-- 2) 页面资源（ManagerShell 中归属同一个「Argus 管理」分组）
-- ---------------------------------------------------------------------------
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
-- 3) 角色绑定（同一 role_id + resource_id + active=1 不重复插入）
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN (
    'argus_config:instance:overview',
    'argus_dashboard:page',
    'argus_instances:page'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM role_resource_new rr
    WHERE rr.role_id = @role_id
      AND rr.resource_id = r.id
      AND rr.active = 1
  );
