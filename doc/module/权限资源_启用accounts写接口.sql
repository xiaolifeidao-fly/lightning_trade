-- =============================================================================
-- 重新启用 /accounts 的两条写接口权限（POST /accounts、PUT /accounts/:id）
--
-- 生成时间：2026-09-10
--
-- 背景：前一步「下线僵尸行」把 9 条没有路由的资源全部 active=0，其中包括
-- /accounts 那 5 条。现在后端补上了这两条路由，权限行必须跟着回来，否则
-- 用户管理页的充值与冻结按钮会从 404 变成 {"success":false,"message":
-- "user not resource"} —— 从"没实现"变成"没权限"，更难查。
--
-- 只启用**已经实现**的两个方法：
--   POST /accounts       → 新建资金账户（首次充值 / 首次冻结时走这条）
--   PUT  /accounts/:id   → 改余额或状态
-- 仍然保持下线的（路由确实不存在，别再放回去）：
--   GET    /accounts          get_accounts
--   GET    /accounts/:id      get_accounts_by_id
--   DELETE /accounts/:id      delete_accounts_by_id
--   /account-details 那 4 条
--
-- 关于「一个 URL 一行就够」：鉴权是**方法盲**的——resolveRequestURL 只取路径、
-- 不带 HTTP 方法（manager-api/auth/auth.go:94），findResourceByURL 取第一条
-- 匹配行。所以启用 post_accounts 就让 /accounts 这个路径整体通过，
-- 无需再启用 get_accounts。
--
-- 幂等：可重复执行。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 第 0 步：执行前自检（只读）。期望这两行 active=0。
-- ---------------------------------------------------------------------------
SELECT id, code, resource_url, active,
       (SELECT COUNT(*) FROM role_resource_new rr WHERE rr.resource_id = r.id AND rr.active = 1) AS active_bindings
FROM resource_new r
WHERE r.code IN ('post_accounts', 'put_accounts_by_id')
ORDER BY r.code;


-- ---------------------------------------------------------------------------
-- 第 1 步：启用
-- ---------------------------------------------------------------------------
START TRANSACTION;

SET @role_id = 1;

-- 角色存在性开关：为 0 时下面的绑定一条都不会插。
SET @role_ok = (SELECT COUNT(*) FROM role WHERE id = @role_id AND active = 1);
SELECT @role_id AS target_role, IF(@role_ok = 1, 'ok', 'role missing — 请 ROLLBACK') AS precheck;

UPDATE resource_new
SET active = 1
WHERE active = 0
  AND code IN ('post_accounts', 'put_accounts_by_id');

-- 之前若有被一并下线的绑定，恢复它
UPDATE role_resource_new rr
JOIN resource_new r ON r.id = rr.resource_id
SET rr.active = 1
WHERE rr.active = 0
  AND rr.role_id = @role_id
  AND r.code IN ('post_accounts', 'put_accounts_by_id');

-- post_accounts 历史上就没有任何绑定（正是「资源点 168 / 授权 167」差的那 1 个），
-- 所以这里还要补插缺失的绑定。
INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND @role_ok = 1
  AND r.code IN ('post_accounts', 'put_accounts_by_id')
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );

COMMIT;


-- ---------------------------------------------------------------------------
-- 第 2 步：执行后校验
-- ---------------------------------------------------------------------------
-- 期望两行都是 active=1 且 active_bindings=1
SELECT id, code, resource_url, active,
       (SELECT COUNT(*) FROM role_resource_new rr WHERE rr.resource_id = r.id AND rr.active = 1) AS active_bindings
FROM resource_new r
WHERE r.code IN ('post_accounts', 'put_accounts_by_id')
ORDER BY r.code;

-- 期望仍为 0 行：没实现的那些不能被顺手放回来
SELECT id, code, resource_url
FROM resource_new
WHERE active = 1
  AND code IN ('get_accounts', 'get_accounts_by_id', 'delete_accounts_by_id',
               'get_account_details', 'post_account_details',
               'get_account_details_by_id', 'put_account_details_by_id');

-- 资源点数与角色授权数应当仍然相等（两者都 +2）
SELECT
  (SELECT COUNT(*) FROM resource_new WHERE active = 1) AS resource_count,
  (SELECT COUNT(*) FROM resource_new r
     JOIN role_resource_new rr ON rr.resource_id = r.id AND rr.active = 1 AND rr.role_id = 1
   WHERE r.active = 1) AS granted_count;
