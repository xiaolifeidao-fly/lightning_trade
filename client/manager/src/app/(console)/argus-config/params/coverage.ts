"use client";

/**
 * 配置面覆盖度：部署 properties 共 63 个键，逐项核对后分成五类。
 *
 * 这是一份人工审计结论（来源 doc/requirements/req-1788169552180/需求大纲.md §3.2，
 * 审计时点 2026-08-31），不是运行时探测结果——运行时无法判断「DB 有列但代码不读」
 * 这种缺口，只能靠读 runtimeFromSnapshot / runtimeAccount 的赋值逐项比对。
 * 收敛工作由 r5「配置面完整收敛到 DB」推进，进度变化时同步改这里。
 */

export const COVERAGE_AUDIT_AT = "2026-08-31";
export const COVERAGE_TOTAL_KEYS = 63;

export interface CoverageBucket {
  key: string;
  name: string;
  count: number;
  color: string;
  detail: string;
  /** true = 这一档是缺口，不是已完成 */
  gap?: boolean;
}

export const COVERAGE_BUCKETS: CoverageBucket[] = [
  {
    key: "properties",
    name: "必须留 properties（连库前置）",
    count: 4,
    color: "#5E6673",
    detail: "sqlconn / redis.addr / redis.password / argus.instance.id",
  },
  {
    key: "consumed",
    name: "DB 有列且运行时已消费",
    count: 34,
    color: "#0ECB81",
    detail: "账户凭证、监控币种、通知、端口与路径等",
  },
  {
    key: "not-read",
    name: "DB 有列但运行时不读",
    count: 13,
    color: "#F6465D",
    gap: true,
    detail: "trail 8 项 / trade_logic / variant / trade_direction / 巡检与告警 3 项",
  },
  {
    key: "no-column",
    name: "DB 完全没有列",
    count: 9,
    color: "#FF9F43",
    gap: true,
    detail: "账户级 order_size / risk_equity / 反向门控与趋势闸阈值 / 合约面值等",
  },
  {
    key: "low-priority",
    name: "可进 DB 但优先级低",
    count: 3,
    color: "#4D7EFF",
    detail: "argus.build.version / argus.heartbeat.interval_seconds / ttl_seconds",
  },
];

/** 覆盖度里那条必须单独喊出来的：它不是「字段没读」，是功能性故障。 */
export const COVERAGE_CRITICAL_NOTE =
  "trade_logic 在 extra_risk_json 里有值，但 runtimeAccount() 不读 → 空字符串被 normalizeTradeLogic 归一化成 spread（老价差逻辑）→ manager.go 的 IsSignalLogic() 分支不会走。切到 DB 配置驱动后整套盘口信号策略会静默失效且不报任何错。同理 variant 为空会让 champion / challenger 归因彻底失灵。";
