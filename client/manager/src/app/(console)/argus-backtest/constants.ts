"use client";

import { chartPalette } from "@/components/charts/chartTheme";
import type { SignalBacktestParams } from "./api/argus-backtest.api";

/**
 * 盘口信号回测页的口径常量。
 *
 * 这里只放**服务端不返回**的东西：表单的旋钮目录、颜色、格式化。精度等级的判定、
 * 警示文案、对比矩阵的排序与分桶全部来自服务端，前端不另打一份副本。
 */

// ─── 参数表单目录 ────────────────────────────────────────────────────────────

/** 表单旋钮的输入形态。除 `select` 外都是数值输入。 */
export type KnobType = "number" | "select";

export interface ParamKnob {
  /** 回测参数字段名，直接就是 POST 体里的键。 */
  field: keyof SignalBacktestParams;
  label: string;
  unit?: string;
  step?: number;
  precision?: number;
  min?: number;
  type?: KnobType;
  options?: { value: string; label: string }[];
  /** 部署 properties 里的真实参数键。 */
  propertyKey: string;
  /** 配置版本快照里的实际落点；尚无 DB 列的写明「尚无 DB 列」。 */
  storeKey: string;
  /**
   * 对应 argus-config 参数目录里的键。有值才能「把这组参数带去发布」——
   * 没有对应键（如 capOverride 这类只在回测里存在的口径）时按下按钮也带不过去，
   * 页面必须把它标出来而不是静默丢掉。
   */
  configParamKey?: string;
  /** 参数在配置页的作用域，决定预填时要不要带 accountLabel / symbol。 */
  configScope?: "global" | "symbol" | "account";
  hint?: string;
}

export interface ParamKnobGroup {
  key: string;
  name: string;
  desc?: string;
  knobs: ParamKnob[];
}

/**
 * 表单只收录**引擎真的会消费**的旋钮。
 *
 * 原型 backtest.html 上还画了「信号延迟确认」「报价最大陈旧度」「合约面值」
 * 「反向门控开关」四项，但 `signal.Params` 里没有它们（见 {@link NON_KNOBS}）。
 * 摆一个改了不生效的输入框，比不摆更糟——它会让人以为已经验过这个参数。
 */
export const PARAM_KNOB_GROUPS: ParamKnobGroup[] = [
  {
    key: "signal",
    name: "信号",
    desc: "触发条件。改这里会让本组降为频率级精度",
    knobs: [
      {
        field: "signalThresholdBp",
        label: "信号阈值 θ",
        unit: "bp",
        step: 0.5,
        precision: 2,
        min: 0,
        propertyKey: "monitor.symbols.<SYMBOL>.signal_threshold",
        storeKey: "argus_monitor_symbol.signal_threshold",
        configParamKey: "signal_threshold",
        configScope: "symbol",
        hint: "与基线阈值 θ0 不等时，历史事件已被 θ0 结构性截断，本组自动降为频率级精度",
      },
      {
        field: "baselineThresholdBp",
        label: "事件流生产阈值 θ0",
        unit: "bp",
        step: 0.5,
        precision: 2,
        min: 0,
        propertyKey: "monitor.symbols.<SYMBOL>.signal_threshold（事件产生时的值）",
        storeKey: "argus_monitor_symbol.signal_threshold",
        hint: "事件流是在这个阈值下产生的，不是要验的参数。改它等于改「历史是怎么来的」，一般不动",
      },
    ],
  },
  {
    key: "risk",
    name: "仓位与风险",
    desc: "账户级敞口与止损",
    knobs: [
      {
        field: "budgetPct",
        label: "风险预算 f",
        unit: "%",
        step: 0.1,
        precision: 2,
        min: 0,
        propertyKey: "trade.accountN.budget_pct / position.risk.budget_pct",
        storeKey: "argus_account_risk.risk_budget",
        configParamKey: "risk_budget",
        configScope: "account",
      },
      {
        field: "catastropheStopPct",
        label: "兜底止损 S",
        unit: "%",
        step: 25,
        precision: 2,
        min: 0,
        propertyKey: "trade.accountN.catastrophe_stop_pct",
        storeKey: "argus_account_risk.catastrophic_stop_loss",
        configParamKey: "catastrophic_stop_loss",
        configScope: "account",
      },
      {
        field: "ceiling",
        label: "仓位上限（天花板）",
        unit: "张",
        step: 1,
        precision: 0,
        min: 0,
        propertyKey: "trade.accountN.max_contracts_ceiling",
        storeKey: "argus_account_risk.max_contracts",
        configParamKey: "max_contracts",
        configScope: "account",
        hint: "实盘口径：cap 由公式 N_max = f·E·L·100 / (face·P·S) 算出后再被这个天花板截断",
      },
      {
        field: "capOverride",
        label: "固定上限（研究口径）",
        unit: "张",
        step: 1,
        precision: 0,
        min: 0,
        propertyKey: "（无对应生产键）",
        storeKey: "尚无 DB 列 · 仅回测口径",
        hint: "> 0 时直接用这个张数当 cap，绕开公式。金标准脚本用它复现历史结论；线上没有这个开关，带不去发布",
      },
      {
        field: "orderSize",
        label: "单次下单张数",
        unit: "张",
        step: 1,
        precision: 0,
        min: 0,
        propertyKey: "trade.accountN.order_size",
        storeKey: "argus_config.default_order_size（仅全局，账户级尚无 DB 列）",
        configParamKey: "default_order_size",
        configScope: "global",
        hint: "0 = 沿用每条事件自带的 orderSize",
      },
      {
        field: "riskEquity",
        label: "风险本金 E",
        unit: "U",
        step: 10,
        precision: 2,
        min: 0,
        propertyKey: "trade.accountN.risk_equity",
        storeKey: "尚无 DB 列 · 基线回落 argus_account.initial_balance",
        hint: "cap 公式唯一的本金输入。制度性护栏：只能来自配置，禁止「运行时余额 → cap」的自动路径",
      },
      {
        field: "catastropheOvershootRoiPts",
        label: "兜底成交过冲",
        unit: "ROI 点",
        step: 1,
        precision: 2,
        min: 0,
        propertyKey: "（无对应生产键）",
        storeKey: "尚无 DB 列 · 仅回测口径",
        hint: "真实成交总比触发线更差。0 = backtest_dual_side.py 口径；5 = 风险参数研究口径",
      },
      {
        field: "takerFee",
        label: "taker 费率（单边）",
        step: 0.0001,
        precision: 6,
        min: 0,
        propertyKey: "（无对应生产键）",
        storeKey: "尚无 DB 列 · 仅回测口径",
        hint: "寻优页强制 0.00012；单次回测缺省 0.0006。费率口径不同的结果不可直接比大小",
      },
    ],
  },
  {
    key: "trail",
    name: "移动止盈分档",
    desc: "按仓位占上限的比例分小 / 中 / 大三档",
    knobs: [
      {
        field: "smallActivatePct",
        label: "小仓 激活",
        unit: "%",
        step: 10,
        precision: 2,
        min: 0,
        propertyKey: "position.monitor.trail.small_activate",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[small_activate]",
        configParamKey: "trail_small_activate",
        configScope: "account",
      },
      {
        field: "smallGiveback",
        label: "小仓 回吐",
        step: 0.01,
        precision: 4,
        min: 0,
        propertyKey: "position.monitor.trail.small_giveback",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[small_giveback]",
        configParamKey: "trail_small_giveback",
        configScope: "account",
      },
      {
        field: "mediumActivatePct",
        label: "中仓 激活",
        unit: "%",
        step: 10,
        precision: 2,
        min: 0,
        propertyKey: "position.monitor.trail.medium_activate",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[medium_activate]",
        configParamKey: "trail_medium_activate",
        configScope: "account",
      },
      {
        field: "mediumGiveback",
        label: "中仓 回吐",
        step: 0.01,
        precision: 4,
        min: 0,
        propertyKey: "position.monitor.trail.medium_giveback",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[medium_giveback]",
        configParamKey: "trail_medium_giveback",
        configScope: "account",
      },
      {
        field: "largeActivatePct",
        label: "大仓 激活",
        unit: "%",
        step: 5,
        precision: 2,
        min: 0,
        propertyKey: "position.monitor.trail.large_activate",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[large_activate]",
        configParamKey: "trail_large_activate",
        configScope: "account",
      },
      {
        field: "largeGiveback",
        label: "大仓 回吐",
        step: 0.01,
        precision: 4,
        min: 0,
        propertyKey: "position.monitor.trail.large_giveback",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[large_giveback]",
        configParamKey: "trail_large_giveback",
        configScope: "account",
      },
      {
        field: "tierSmallRatio",
        label: "小仓档比例",
        step: 0.05,
        precision: 4,
        min: 0,
        propertyKey: "position.monitor.trail.tier_small_ratio",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[tier_small_ratio]",
        configParamKey: "trail_tier_small_ratio",
        configScope: "account",
      },
      {
        field: "tierLargeRatio",
        label: "大仓档比例",
        step: 0.05,
        precision: 4,
        min: 0,
        propertyKey: "position.monitor.trail.tier_large_ratio",
        storeKey: "argus_account_risk.trailing_stop_tiers_json[tier_large_ratio]",
        configParamKey: "trail_tier_large_ratio",
        configScope: "account",
      },
    ],
  },
  {
    key: "gate",
    name: "门控",
    desc: "反向门控与趋势闸",
    knobs: [
      {
        field: "gateMinProfitPct",
        label: "反向门控最低盈利",
        unit: "%",
        step: 1,
        precision: 2,
        min: 0,
        propertyKey: "trade.accountN.reverse_gate_min_profit_pct",
        storeKey: "argus_account_risk.extra_risk_json[reverse_gate_min_profit_pct]",
        configParamKey: "reverse_gate_min_profit_pct",
        configScope: "account",
      },
      {
        field: "trendGateWindowHours",
        label: "趋势闸 动量窗口",
        unit: "h",
        step: 1,
        precision: 2,
        min: 0,
        propertyKey: "trade.trend_gate.window_hours",
        storeKey: "argus_config.extra_config_json[trade.trend_gate.window_hours]",
        configParamKey: "trend_gate_window_hours",
        configScope: "global",
        hint: "0 = 关闭趋势闸",
      },
      {
        field: "trendGateThresholdPct",
        label: "趋势闸 逆势阈值",
        unit: "%",
        step: 0.5,
        precision: 2,
        min: 0,
        propertyKey: "trade.trend_gate.threshold_pct",
        storeKey: "argus_config.extra_config_json[trade.trend_gate.threshold_pct]",
        configParamKey: "trend_gate_threshold_pct",
        configScope: "global",
        hint: "0 = 关闭趋势闸",
      },
    ],
  },
  {
    key: "replay",
    name: "回放口径",
    desc: "不是策略参数，是回测怎么读这段历史",
    knobs: [
      {
        field: "mode",
        label: "仓位形态",
        type: "select",
        options: [
          { value: "net", label: "net（实盘形态）" },
          { value: "dual", label: "dual（研究对照）" },
        ],
        propertyKey: "（无对应生产键，实盘恒为净仓）",
        storeKey: "尚无 DB 列 · 仅回测口径",
      },
      {
        field: "evalMode",
        label: "bar 内评估",
        type: "select",
        options: [
          { value: "close", label: "close（按收盘判定）" },
          { value: "pessimistic", label: "pessimistic（悲观：先止损）" },
        ],
        propertyKey: "（无对应生产键）",
        storeKey: "尚无 DB 列 · 仅回测口径",
      },
      {
        field: "entryPx",
        label: "成交价口径",
        type: "select",
        options: [
          { value: "bar_close", label: "bar_close（本根收盘）" },
          { value: "sig_last", label: "sig_last（触发瞬间 last）" },
        ],
        propertyKey: "（无对应生产键）",
        storeKey: "尚无 DB 列 · 仅回测口径",
      },
    ],
  },
];

export const ALL_KNOBS: ParamKnob[] = PARAM_KNOB_GROUPS.flatMap((group) => group.knobs);

export function findKnob(field: string): ParamKnob | undefined {
  return ALL_KNOBS.find((knob) => knob.field === field);
}

/**
 * 原型上画了、但引擎里没有对应旋钮的参数。页面必须显式列出来，
 * 否则「表单里没有它」会被读成「这个参数不重要」。
 */
export const NON_KNOBS: { label: string; propertyKey: string; reason: string }[] = [
  {
    label: "信号延迟确认",
    propertyKey: "trade.signal.delay_seconds",
    reason:
      "回放没有秒级下单时序：成交价口径由「bar_close / sig_last」两选一表达，5 秒延迟的影响已经写进精度警示，单独给个旋钮改不了任何行为",
  },
  {
    label: "报价最大陈旧度",
    propertyKey: "monitor.spread.max_price_age_ms",
    reason: "它决定实盘要不要丢弃一条过期报价，只作用在信号产生侧；回放吃的是已经产生的 strategy_event，改它无效",
  },
  {
    label: "合约面值",
    propertyKey: "position.risk.contract_face",
    reason: "属结构性常量，随基线带入（BTC 0.001）。它同时决定 cap 公式与张数换算，作为扫描轴会让每一组的口径都不同",
  },
  {
    label: "反向门控开关",
    propertyKey: "trade.accountN.reverse_gate",
    reason:
      "引擎的净仓形态恒带反向门控，没有 on/off（r16 遗留项 #5）。生产上关掉门控的账户，其反向减仓类差异不可直接外推",
  },
];

// ─── 精度等级 ────────────────────────────────────────────────────────────────

/** 精度等级的语义。**判定由服务端做**，这里只解释「事件级 / 频率级」是什么意思。 */
export const FIDELITY_META: Record<string, { label: string; tone: "success" | "warning"; desc: string }> = {
  event: {
    label: "事件级精度",
    tone: "success",
    desc: "信号序列完全不变，直接回放 strategy_event 里的真实触发时刻。触发点精确到秒，无推导误差。",
  },
  frequency: {
    label: "频率级精度",
    tone: "warning",
    desc:
      "改动了信号阈值，必须重新推导信号序列。历史事件被生产阈值结构性截断（实测 min=5.01bp），" +
      "因此改用 dev_sample 的 DevCross（9 个候选阈值的无条件穿越计数）推算频率 λ(θ)——" +
      "能算出「触发次数会变多少」，但算不出每一次的精确时刻。这类组不产 PnL，也不能与产 PnL 的组比大小。",
  },
};

export function fidelityMeta(fidelity: string) {
  return FIDELITY_META[fidelity] ?? { label: fidelity || "未知精度", tone: "warning" as const, desc: "" };
}

// ─── 颜色 ────────────────────────────────────────────────────────────────────

/**
 * 参数组的固定配色序。按序取，**不循环**——第 7 组起统一用中性灰并靠标签区分，
 * 生成新色只会把两条线画成同一个颜色。
 *
 * 顺序不是随手排的：紫（violet）与蓝（accent）在色觉缺陷下 ΔE 只有 6.6、
 * 正常色觉下 12.5，相邻会认不出来，所以把橙插在它们中间。图例与对比矩阵的
 * 行首色块提供第二重编码，颜色从不是唯一的身份标识。
 */
export const SERIES_COLORS = [
  chartPalette.primary,
  chartPalette.up,
  chartPalette.accent,
  chartPalette.orange,
  chartPalette.violet,
  chartPalette.down,
] as const;

export function seriesColor(index: number): string {
  return index < SERIES_COLORS.length ? SERIES_COLORS[index] : chartPalette.neutral;
}

/** 出场方式的色调，与信号复盘页的出场归因同色。 */
export const EXIT_COLORS: Record<string, string> = {
  trail: chartPalette.up,
  tp: chartPalette.accent,
  sl: chartPalette.down,
  reduceClose: chartPalette.violet,
  timeout: chartPalette.neutral,
  eodOpen: chartPalette.primary,
};

// ─── 状态 ────────────────────────────────────────────────────────────────────

export const BATCH_STATUS_LABEL: Record<string, string> = {
  pending: "排队中",
  running: "回放中",
  done: "已完成",
  partial: "部分完成",
  failed: "失败",
};

export const RUN_STATUS_LABEL: Record<string, string> = {
  pending: "排队中",
  running: "回放中",
  done: "已完成",
  failed: "失败",
};

/** 批次仍在推进时才需要轮询。 */
export function isBatchActive(status: string): boolean {
  return status === "pending" || status === "running";
}

export const BATCH_POLL_INTERVAL = 3000;
export const BATCH_PAGE_SIZE = 20;

// ─── 数值格式 ────────────────────────────────────────────────────────────────

/** 空值一律「—」：0 与「没有这个观测」是两回事。 */
export const EMPTY = "—";

export function fmtNum(value: number | null | undefined, digits = 2, suffix = ""): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${value.toFixed(digits)}${suffix}`;
}

export function fmtSigned(value: number | null | undefined, digits = 2, suffix = ""): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${value > 0 ? "+" : ""}${value.toFixed(digits)}${suffix}`;
}

export function fmtInt(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return String(Math.round(value));
}

export function fmtPct(value: number | null | undefined, digits = 1): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${(value * 100).toFixed(digits)}%`;
}

/** 正绿负红，0 与空值走中性——不要把 0 画成绿的。 */
export function signColor(value: number | null | undefined): string | undefined {
  if (value === null || value === undefined || Number.isNaN(value) || value === 0) return undefined;
  return value > 0 ? chartPalette.up : chartPalette.down;
}

export function fmtDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds <= 0) return EMPTY;
  if (seconds >= 86400) return `${(seconds / 86400).toFixed(1)} 天`;
  if (seconds >= 3600) return `${(seconds / 3600).toFixed(1)} 小时`;
  return `${Math.round(seconds / 60)} 分钟`;
}

/** 本地墙钟串 `YYYY-MM-DD HH:mm:ss`，与 strategy_event.ts 同口径。 */
export function toWallClock(date: Date): string {
  const pad = (v: number) => String(v).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
}

/** 服务端 SignalSource.firstTs/lastTs 是 RFC3339，表单要的是墙钟串。 */
export function rfc3339ToWallClock(value: string): string {
  if (!value) return "";
  const matched = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})/.exec(value);
  return matched ? `${matched[1]} ${matched[2]}` : value;
}

export function shortTs(ts: string): string {
  const wall = rfc3339ToWallClock(ts);
  return wall.length > 5 ? wall.slice(5) : wall || EMPTY;
}
