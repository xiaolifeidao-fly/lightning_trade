-- =============================================================================
-- 趋势条件止损：argus_config / argus_account_risk 各加两列
--
--   trend_stop_trigger_pct  X：逆向 24h 动量达到多少个百分点才收紧兜底线
--   trend_stop_pct          Y：收紧后的兜底线
--   两者 0/0 = 未配置（=关闭）。列定义对齐同表的 trend_gate_threshold_pct。
--
-- 为什么必须**先加列再发二进制**：SaveDraft 走 tx.Create(&config)，GORM 会带上
-- 结构体全部字段，列不存在时 insert 直接失败——那会让"发布配置"整条路径断掉。
--
-- 幂等：按 information_schema 判定，可重复执行。
-- 生成时间：2026-09-15
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 第 0 步：执行前自检（只读）。期望 0 行。
-- ---------------------------------------------------------------------------
SELECT table_name, column_name, column_type
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('argus_config', 'argus_account_risk')
  AND column_name IN ('trend_stop_trigger_pct', 'trend_stop_pct');


-- ---------------------------------------------------------------------------
-- 第 1 步：加列（幂等）
-- ---------------------------------------------------------------------------
SET @ddl := (SELECT IF(COUNT(*) > 0, 'SELECT ''argus_config.trend_stop_trigger_pct 已存在'' AS skipped',
  'ALTER TABLE argus_config ADD COLUMN trend_stop_trigger_pct DECIMAL(10,4) NULL DEFAULT 0
     COMMENT ''趋势条件止损触发阈值% position.monitor.trend_stop.trigger_pct；0=未配置''')
  FROM information_schema.columns WHERE table_schema = DATABASE()
    AND table_name = 'argus_config' AND column_name = 'trend_stop_trigger_pct');
PREPARE st FROM @ddl; EXECUTE st; DEALLOCATE PREPARE st;

SET @ddl := (SELECT IF(COUNT(*) > 0, 'SELECT ''argus_config.trend_stop_pct 已存在'' AS skipped',
  'ALTER TABLE argus_config ADD COLUMN trend_stop_pct DECIMAL(10,4) NULL DEFAULT 0
     COMMENT ''趋势条件止损收紧后的兜底线% position.monitor.trend_stop.stop_pct；0=未配置''')
  FROM information_schema.columns WHERE table_schema = DATABASE()
    AND table_name = 'argus_config' AND column_name = 'trend_stop_pct');
PREPARE st FROM @ddl; EXECUTE st; DEALLOCATE PREPARE st;

SET @ddl := (SELECT IF(COUNT(*) > 0, 'SELECT ''argus_account_risk.trend_stop_trigger_pct 已存在'' AS skipped',
  'ALTER TABLE argus_account_risk ADD COLUMN trend_stop_trigger_pct DECIMAL(10,4) NULL DEFAULT 0
     COMMENT ''账户级趋势条件止损触发阈值%；0=未配置''')
  FROM information_schema.columns WHERE table_schema = DATABASE()
    AND table_name = 'argus_account_risk' AND column_name = 'trend_stop_trigger_pct');
PREPARE st FROM @ddl; EXECUTE st; DEALLOCATE PREPARE st;

SET @ddl := (SELECT IF(COUNT(*) > 0, 'SELECT ''argus_account_risk.trend_stop_pct 已存在'' AS skipped',
  'ALTER TABLE argus_account_risk ADD COLUMN trend_stop_pct DECIMAL(10,4) NULL DEFAULT 0
     COMMENT ''账户级趋势条件止损收紧后的兜底线%；0=未配置''')
  FROM information_schema.columns WHERE table_schema = DATABASE()
    AND table_name = 'argus_account_risk' AND column_name = 'trend_stop_pct');
PREPARE st FROM @ddl; EXECUTE st; DEALLOCATE PREPARE st;


-- ---------------------------------------------------------------------------
-- 第 2 步：执行后校验。期望 4 行，且全部为 0（未配置=关闭）
-- ---------------------------------------------------------------------------
SELECT table_name, column_name, column_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('argus_config', 'argus_account_risk')
  AND column_name IN ('trend_stop_trigger_pct', 'trend_stop_pct')
ORDER BY table_name, column_name;

SELECT 'argus_config' AS t, config_version_id, trend_stop_trigger_pct, trend_stop_pct FROM argus_config
UNION ALL
SELECT 'argus_account_risk', account_id, trend_stop_trigger_pct, trend_stop_pct FROM argus_account_risk;
