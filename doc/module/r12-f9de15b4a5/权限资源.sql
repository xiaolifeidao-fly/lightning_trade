-- r12-f9de15b4a5 引入 lightweight-charts，建行情与触发点叠加主视图 · 权限资源补齐
--
-- 用法：把下面的 @role_id 改成实际要授权的角色主键后整段执行。全部语句幂等，可重复跑。
-- 本轮没有拿到目标角色，沿用 r7 / r9 的写法默认给 1，执行前请确认它就是目标角色。
--
-- 本页不新增任何接口：
--   · /argus-event/* 八个只读接口由 r9 的 doc/module/r9-022661de71/权限资源.sql 补齐；
--   · /argus-config/instances 与 /argus-config/published 由 r7 的 SQL 补齐。
-- 唯一没被任何前置任务登记过的是 K 线窗口回填（r2 落地时没出 SQL），这里补上——
-- 它是本页「一键回填缺口」按钮唯一的写动作，只写 trade_kline，不碰实盘链路。
--
-- 补记（交付后复核）：上面这段只登记了接口，漏了页面级资源本身。ManagerShell.tsx
-- 的七个 Argus 页面里，/argus-config(r7)、/argus-signals(r13)、/argus-dashboard 与
-- /argus-instances(r15)、/argus-backtest 与 /argus-optimizer(r14) 都有 page 资源，
-- 只有本页 /argus-market 没有。见下面第 3、4 节。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 接口资源
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'K 线窗口回填', 'trade_kline:backfill:range', 0, 'api', '/klines/backfill-range', '', '', '', '', '', 130, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'trade_kline:backfill:range' AND resource_url = '/klines/backfill-range' AND active = 1);

-- ---------------------------------------------------------------------------
-- 2) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN ('trade_kline:backfill:range')
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );

-- ---------------------------------------------------------------------------
-- 3) 页面资源（补齐：/argus-market 本身）
--
-- sort_id 取 136，紧随 r15 的 /argus-dashboard(134)、/argus-instances(135)，
-- 与 ManagerShell.tsx「Argus 管理」分组里 总览 → 实例对比 → 历史行情 的菜单顺序一致。
-- name / menu_name 的拆分沿用 r15：name 带 Argus 前缀，menu_name 用菜单原文案。
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT 'Argus 历史行情与触发点', 'argus_market:page', 0, 'page', '', '/argus-market', '', '', '历史行情与触发点', '', 136, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_market:page'
    AND page_url = '/argus-market'
    AND active = 1
);

-- ---------------------------------------------------------------------------
-- 4) 页面资源的角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN ('argus_market:page')
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
