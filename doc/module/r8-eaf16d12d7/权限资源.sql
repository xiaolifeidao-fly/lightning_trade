-- r8-eaf16d12d7 盘口信号回测引擎（事件驱动 + 精度分层）· 权限资源补齐
--
-- 用法：执行前请确认 @role_id 是要授权的管理角色。沿用 r7/r12/r13/r14 的默认值 1；
-- 若实际角色不同，只改下面这一行即可。语句幂等，可重复执行。
--
-- 为什么现在才补：r8 交付时没有出 SQL 文件。审计 manager-api 全部 96 条非公开路由
-- 与仓库已登记的 31 个 api 资源后，`POST /backtest/signal-runs` 是唯一没有任何
-- resource_new 行的新增接口。
--
-- 为什么必须补：manager-api/auth/auth.go 的 Middleware 会把 gin FullPath（去掉
-- request.path 前缀后）交给 service/manager_auth.ValidateToken -> hadResource，
-- 后者按 resource_url **精确字符串**查 resource_new，查不到即 ErrUserNoResource。
-- 也就是说没有这行资源，「发起单组盘口信号回测」在管理端会直接被拒，且报的是
-- 权限错误而不是功能错误。（auth_service.go:141 那句「暂时不校验资源权限」是
-- 过时注释，它下面的代码确实在校验。）
--
-- 读侧不在这里：signal-runs 的结果复用 r14 已登记的 /backtest/runs/:id（145）
-- 与早期需求登记的 /backtest/metrics，故本文件只有一个写接口。
SET @role_id = 1;

-- ---------------------------------------------------------------------------
-- 1) 接口资源
--
-- sort_id 取 139，紧邻 r14 的 backtest 资源簇（140-145），使「发起回测」排在
-- 「信号源列表」之前，与页面上的操作顺序一致。
-- ---------------------------------------------------------------------------
INSERT INTO resource_new (name, code, parent_id, resource_type, resource_url, page_url, component, redirect, menu_name, meta, sort_id, active)
SELECT '提交盘口信号回测', 'argus_backtest:signal_run:create', 0, 'api', '/backtest/signal-runs', '', '', '', '', '', 139, 1
WHERE NOT EXISTS (SELECT 1 FROM resource_new WHERE code = 'argus_backtest:signal_run:create' AND resource_url = '/backtest/signal-runs' AND active = 1);

-- ---------------------------------------------------------------------------
-- 2) 角色绑定
-- ---------------------------------------------------------------------------
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND r.code IN ('argus_backtest:signal_run:create')
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );
