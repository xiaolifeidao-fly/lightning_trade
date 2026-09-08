-- r13-0585d5f39b 信号复盘与 episode 详情页 · 权限资源补齐
--
-- 用法：把下面的 @role_id 改成实际要授权的角色主键后整段执行。全部语句幂等，可重复跑。
-- 本轮没有指定 role_id，沿用 r7 / r9 的写法默认给 1，执行前请确认它就是目标角色。
--
-- 本文件只补 r13 新增的东西：五个 episode / 切片对比接口 + 复盘页的页面资源。
-- r9 已经补过的八个信号侧接口（filter-options / signals / gate-stats 等）不在这里重复，
-- 复盘页同样依赖它们——先执行 doc/module/r9-022661de71/权限资源.sql，再执行本文件。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 接口资源（全部只读 GET）
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

-- ---------------------------------------------------------------------------
-- 2) 页面资源
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 信号复盘', 'argus_signals:page', 0, 'page', '', '/argus-signals', '', '', 'Argus 信号复盘', '', 111, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_signals:page' AND page_url = '/argus-signals' AND active = 1);

-- ---------------------------------------------------------------------------
-- 3) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN (
    'argus_event:episode:list',
    'argus_event:episode:detail',
    'argus_event:episode:stats',
    'argus_event:episode:exitkinds',
    'argus_event:slice:compare',
    'argus_signals:page'
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
