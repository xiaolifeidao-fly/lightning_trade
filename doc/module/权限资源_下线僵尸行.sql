-- =============================================================================
-- 下线没有对应路由的僵尸接口资源
--
-- 生成时间：2026-09-09
-- 影响：resource_new 9 行 + role_resource_new 8 行，全部只置 active=0，不 DELETE
--
-- 对账方法（不是凭印象挑的）：
--   后端真实注册路由  grep 'engine.(GET|POST|PUT|DELETE|PATCH)("…")' server/manager-api
--                     → 139 个「方法+路径」，去重后 96 个路径
--   前端真实页面      find client/manager/src/app -name page.tsx
--                     → 20 个页面
--   库里生效资源      resource_new WHERE active=1 → 153 个 api + 15 个 page
--   逐条比对结果      api 有对应路由 144 / 僵尸 9；page 全部有对应页面，僵尸 0
--
-- 注意：153 个 api 资源对 96 个路径并不矛盾——同一路径的 GET/POST/PUT/DELETE
-- 各占一行资源，所以行数天然多于路径数。（先前粗算「96 vs 168」得出的
-- 「70+ 条僵尸」是错的，实际只有 9 条。）
--
-- 实测确认这两族路由确实不存在（不是 grep 漏了）：
--   GET /api/accounts        → 404 page not found
--   GET /api/account-details → 404 page not found
--   GET /api/users           → 200（对照组，证明网关与鉴权都正常）
--
-- 幂等：可重复执行。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 第 0 步：执行前自检（只读）。先单独跑这段，确认待下线的就是这 9 行。
-- ---------------------------------------------------------------------------
SELECT r.id, r.code, r.resource_url,
       (SELECT COUNT(*) FROM role_resource_new rr WHERE rr.resource_id = r.id AND rr.active = 1) AS bindings
FROM resource_new r
WHERE r.active = 1
  AND r.resource_type = 'api'
  AND (r.resource_url IN ('/accounts', '/accounts/:id', '/account-details', '/account-details/:id'))
ORDER BY r.resource_url, r.code;


-- ---------------------------------------------------------------------------
-- 第 1 步：下线
-- ---------------------------------------------------------------------------
START TRANSACTION;

-- 先断角色绑定，再下线资源本身。顺序无所谓（都按 resource_url 定位），
-- 但分两条写是为了让影响行数分别可见。
UPDATE role_resource_new rr
JOIN resource_new r ON r.id = rr.resource_id
SET rr.active = 0
WHERE rr.active = 1
  AND r.resource_type = 'api'
  AND r.resource_url IN ('/accounts', '/accounts/:id', '/account-details', '/account-details/:id');

UPDATE resource_new
SET active = 0
WHERE active = 1
  AND resource_type = 'api'
  AND resource_url IN ('/accounts', '/accounts/:id', '/account-details', '/account-details/:id');

COMMIT;


-- ---------------------------------------------------------------------------
-- 第 2 步：执行后校验
-- ---------------------------------------------------------------------------
-- 期望 0 行
SELECT id, code, resource_url
FROM resource_new
WHERE active = 1
  AND resource_type = 'api'
  AND resource_url IN ('/accounts', '/accounts/:id', '/account-details', '/account-details/:id');

-- 期望：api 资源 153 → 144，且「资源点数」与「当前角色授权」不再差 1
-- （差的那 1 个原本就是 post_accounts，它没有任何绑定）
SELECT
  (SELECT COUNT(*) FROM resource_new WHERE active = 1) AS 资源点数,
  (SELECT COUNT(*) FROM resource_new r
     JOIN role_resource_new rr ON rr.resource_id = r.id AND rr.active = 1 AND rr.role_id = 1
   WHERE r.active = 1) AS 角色1已授权;


-- =============================================================================
-- 遗留问题：这 9 行里的 /accounts 那 5 行，对应的是**前端还在调的功能**
--
--   client/manager/src/app/(console)/user/components/UserManagementDemo.tsx
--     :201 / :203  调整用户余额      → createAccount / updateAccount
--     :222 / :224  切换账户状态      → createAccount / updateAccount
--   client/manager/src/app/(console)/user/api/user.api.ts
--     :133  POST /accounts
--     :138  PUT  /accounts/{id}
--
-- 后端从来没有这两个路由，所以用户管理页上的「余额调整」与「账户状态切换」
-- 点下去必然 404。下线资源行不会让它更坏（路由本来就 404），但也修不了它。
-- 要么补后端，要么把这两个入口从界面摘掉——这是产品取舍，没在本脚本里替你定。
-- =============================================================================
