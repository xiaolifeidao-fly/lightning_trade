-- r7-f74ca0f149 argus-config 参数编辑页重构 · 权限资源补齐
--
-- 用法：把下面的 @role_id 改成实际要授权的角色主键后整段执行。全部语句幂等，可重复跑。
-- 未指定 role_id 时不要盲猜——本文件默认给出 1，执行前请确认它就是目标角色。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 下线 start / stop / restart：接口已从 manager-api 移除，残留的资源会继续
--    出现在角色授权树里，让人以为页面上还有停进程的入口。
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
-- 2) 接口资源
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

-- ---------------------------------------------------------------------------
-- 3) 页面资源
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 参数与运行控制', 'argus_config:page', 0, 'page', '', '/argus-config', '', '', 'Argus 参数与运行控制', '', 109, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_config:page' AND page_url = '/argus-config' AND active = 1);

-- ---------------------------------------------------------------------------
-- 4) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
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
    'argus_config:page'
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
