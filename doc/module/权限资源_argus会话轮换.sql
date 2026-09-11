-- =============================================================================
-- 新增接口权限：Argus 会话凭证轮换
--
--   POST /argus-config/sessions/rotate   →  argus_config:session:rotate
--
-- 生成时间：2026-09-11
--
-- 背景：管理端此前没有更新 cookie / token 的入口——账户是 login_type=config
-- （静态凭证），进程永远不会自己重登，而会话面板是只读的，于是凭证过期只能
-- 直接改库（argus_runtime_session 里还留着 rotate-drill 这类手工痕迹）。
-- 现在「配置面巡检」里加了「更新凭证」，后端就是这条新路由。
--
-- 不加这两行的后果：按钮点下去返回
--   {"success":false,"message":"user not resource"}
-- ——从"没做"变成"没权限"，比 404 更难查。
--
-- 关于「一个 URL 一行就够」：鉴权是**方法盲**的——resolveRequestURL 只取路径、
-- 不带 HTTP 方法（manager-api/auth/auth.go:94），findResourceByURL 取第一条
-- 匹配行。这个路径只有 POST 一个方法，所以一行即可。
--
-- 幂等：可重复执行。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 第 0 步：执行前自检（只读）。期望 0 行。
-- ---------------------------------------------------------------------------
SELECT id, code, resource_url, active
FROM resource_new
WHERE code = 'argus_config:session:rotate';


-- ---------------------------------------------------------------------------
-- 第 1 步：写入
-- ---------------------------------------------------------------------------
START TRANSACTION;

SET @role_id = 1;

-- 角色存在性开关：为 0 时下面的绑定一条都不会插。
-- （不要用 SELECT IF(...) 当断言——它只打印，不会中断执行。）
SET @role_ok = (SELECT COUNT(*) FROM role WHERE id = @role_id AND active = 1);
SELECT @role_id AS target_role, IF(@role_ok = 1, 'ok', 'role missing — 请 ROLLBACK') AS precheck;

INSERT INTO resource_new (
  name, code, parent_id, resource_type, resource_url,
  page_url, component, redirect, menu_name, meta, sort_id, active
)
SELECT
  'Argus 会话凭证轮换', 'argus_config:session:rotate', 0, 'api', '/argus-config/sessions/rotate',
  '', '', '', '', '', 134, 1
WHERE NOT EXISTS (
  SELECT 1 FROM resource_new
  WHERE code = 'argus_config:session:rotate'
    AND resource_url = '/argus-config/sessions/rotate'
    AND active = 1
);

INSERT INTO role_resource_new (role_id, resource_id, active)
SELECT @role_id, r.id, 1
FROM resource_new r
WHERE r.active = 1
  AND @role_ok = 1
  AND r.code = 'argus_config:session:rotate'
  AND NOT EXISTS (
    SELECT 1 FROM role_resource_new rr
    WHERE rr.role_id = @role_id AND rr.resource_id = r.id AND rr.active = 1
  );

COMMIT;


-- ---------------------------------------------------------------------------
-- 第 2 步：执行后校验。期望 1 行，且 active_bindings = 1
-- ---------------------------------------------------------------------------
SELECT r.id, r.code, r.resource_url, r.active,
       (SELECT COUNT(*) FROM role_resource_new rr
         WHERE rr.resource_id = r.id AND rr.active = 1 AND rr.role_id = 1) AS active_bindings
FROM resource_new r
WHERE r.code = 'argus_config:session:rotate';

-- 资源点数与角色授权数应当仍然相等（两者都 +1）
SELECT
  (SELECT COUNT(*) FROM resource_new WHERE active = 1) AS resource_count,
  (SELECT COUNT(*) FROM resource_new r
     JOIN role_resource_new rr ON rr.resource_id = r.id AND rr.active = 1 AND rr.role_id = 1
   WHERE r.active = 1) AS granted_count;
