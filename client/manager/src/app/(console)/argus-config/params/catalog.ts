"use client";

import type { ArgusConfigDraft, ArgusConfigSnapshot } from "../api/argus-config.api";

/**
 * 参数目录：把「策略参数」这个业务视角，映射回 argus_config 快照里真实的落点。
 *
 * 三条约束决定了这张表的形状：
 * 1. 参数按实例分域，同一个键在三个实例上取值不同，所以取值必须从当前实例的
 *    已发布快照里读，不能有任何写死的默认值；
 * 2. 每个参数都要标出真实参数键（properties 键）与 DB 落点，改错地方的代价是实盘；
 * 3. 有的参数 DB 有列但 argus_single 还没消费（runtimeFromSnapshot / runtimeAccount
 *    里没有对应赋值），页面必须显式告诉运维「改了也不会立刻生效」，否则就是骗人。
 */

export type ParamScope = "global" | "symbol" | "account";

export type ParamCarrier =
  /** argus_config 的结构化列 */
  | { kind: "config"; field: ConfigNumberField }
  /** argus_monitor_symbol 的结构化列 */
  | { kind: "symbol"; field: "signalThreshold" | "spreadThreshold" }
  /** argus_account_risk 的结构化列 */
  | { kind: "risk"; field: "riskBudget" | "catastrophicStopLoss" | "maxContracts" | "reverseGateEnabled" | "reverseGateMinProfitPct" }
  /** argus_config.extra_config_json 里的键。r5 之后只剩没有独立列的项才该走这里 */
  | { kind: "configExtra"; key: string }
  /** argus_account_risk.extra_risk_json 里的键 */
  | { kind: "riskExtra"; key: string }
  /** argus_account_risk.trailing_stop_tiers_json 里的键，importer 已在写 */
  | { kind: "trailTier"; key: string };

type ConfigNumberField =
  | "defaultOrderSize"
  | "monitorIntervalSecond"
  | "profitThreshold"
  | "lossThreshold"
  | "serverPort"
  | "contractFace"
  | "signalDelaySecond"
  | "spreadMaxPriceAgeMs"
  | "trendGateWindowHour"
  | "trendGateThresholdPct";

export interface ParamDef {
  /** 表单内的唯一键，同时用于 diff */
  key: string;
  label: string;
  unit?: string;
  step: number;
  precision: number;
  min?: number;
  max?: number;
  scope: ParamScope;
  carrier: ParamCarrier;
  /** 真实参数键：部署 properties 里的键名 */
  propertyKey: string;
  /** DB 落点：表.列（或 JSON 列里的路径） */
  storeKey: string;
  /** false = 结构性参数，发布后不热生效，需重启 argus_single */
  hot: boolean;
  /** 关键参数：直接影响实盘下单与风控，发布前必须二次确认 */
  critical?: boolean;
  /**
   * true = DB 侧可写，但 argus_single 运行时还没消费这个值。页面必须把它和
   * 「改完就生效」的参数区分开。
   *
   * r5 收敛完成后，本目录里**已没有参数**处于这个状态：每一项都能从
   * argus_config/argus_account_risk 的列经 runtimeconfig/tuning.go 推进 viper，
   * 再由 price_monitor / account_monitor / trend_gate / account_params 实际读取。
   * 新增参数时若确实还没接运行时，再把这个标记打上——判据是「viper 键在
   * argus_single 里没有任何消费点」，不是「看起来没接」。
   */
  pendingRuntime?: boolean;
  /** 开关型参数用下拉，其余用数字输入 */
  type?: "switch";
  /** DB 存的是比例，页面按 bp / % 显示时的换算 */
  scale?: number;
  hint?: string;
}

export interface ParamGroup {
  key: string;
  name: string;
  desc?: string;
  note?: string;
  params: ParamDef[];
}

export const PARAM_GROUPS: ParamGroup[] = [
  {
    key: "signal",
    name: "信号",
    desc: "盘口信号的触发条件",
    params: [
      {
        key: "signal_threshold",
        label: "信号阈值",
        unit: "bp",
        step: 0.5,
        precision: 2,
        min: 0,
        scope: "symbol",
        carrier: { kind: "symbol", field: "signalThreshold" },
        propertyKey: "monitor.symbols.<SYMBOL>.signal_threshold",
        storeKey: "argus_monitor_symbol.signal_threshold",
        hot: true,
        critical: true,
        scale: 10000,
        hint: "DB 存的是 last 相对 mark 的偏离比例，页面按 bp 显示（1bp = 0.0001）",
      },
      {
        key: "spread_threshold",
        label: "价差阈值",
        step: 0.0005,
        precision: 6,
        min: 0,
        scope: "symbol",
        carrier: { kind: "symbol", field: "spreadThreshold" },
        propertyKey: "monitor.symbols.<SYMBOL>.threshold",
        storeKey: "argus_monitor_symbol.spread_threshold",
        hot: true,
      },
      {
        key: "signal_delay_seconds",
        label: "信号延迟确认",
        unit: "s",
        step: 1,
        precision: 0,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "signalDelaySecond" },
        propertyKey: "trade.signal.delay_seconds",
        storeKey: "argus_config.signal_delay_second",
        hot: true,
      },
      {
        key: "max_price_age_ms",
        label: "报价最大陈旧度",
        unit: "ms",
        step: 50,
        precision: 0,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "spreadMaxPriceAgeMs" },
        propertyKey: "monitor.spread.max_price_age_ms",
        storeKey: "argus_config.spread_max_price_age_ms",
        hot: true,
      },
    ],
  },
  {
    key: "risk",
    name: "仓位与风险",
    desc: "账户级敞口与止损",
    params: [
      {
        key: "risk_budget",
        label: "风险预算 f",
        unit: "%",
        step: 0.1,
        precision: 2,
        min: 0,
        scope: "account",
        carrier: { kind: "risk", field: "riskBudget" },
        propertyKey: "trade.accountN.budget_pct / position.risk.budget_pct",
        storeKey: "argus_account_risk.risk_budget",
        hot: true,
        critical: true,
      },
      {
        key: "catastrophic_stop_loss",
        label: "兜底止损 S",
        unit: "%",
        step: 25,
        precision: 2,
        min: 0,
        scope: "account",
        carrier: { kind: "risk", field: "catastrophicStopLoss" },
        propertyKey: "trade.accountN.catastrophe_stop_pct",
        storeKey: "argus_account_risk.catastrophic_stop_loss",
        hot: true,
        critical: true,
      },
      {
        key: "max_contracts",
        label: "仓位上限",
        unit: "张",
        step: 1,
        precision: 0,
        min: 0,
        scope: "account",
        carrier: { kind: "risk", field: "maxContracts" },
        propertyKey: "trade.accountN.max_contracts_ceiling",
        storeKey: "argus_account_risk.max_contracts",
        hot: true,
        critical: true,
      },
      {
        key: "default_order_size",
        label: "单次下单张数",
        unit: "张",
        step: 1,
        precision: 0,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "defaultOrderSize" },
        propertyKey: "trade.order_size",
        storeKey: "argus_config.default_order_size",
        hot: true,
        critical: true,
        hint: "置 0 = 暂停新开仓：停止开新仓，但已有持仓的移动止盈与兜底止损继续看管",
      },
      {
        key: "contract_face",
        label: "合约面值",
        unit: "BTC",
        step: 0.001,
        precision: 6,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "contractFace" },
        propertyKey: "position.risk.contract_face",
        storeKey: "argus_config.contract_face",
        hot: false,
      },
    ],
  },
  {
    key: "trail",
    name: "移动止盈分档",
    desc: "按仓位占上限的比例分小/中/大三档",
    note: "这 8 个参数 importer 已写入 trailing_stop_tiers_json，但 runtimeAccount() 目前不读；消费侧由 r5「配置面完整收敛到 DB」补齐。",
    params: [
      trailTier("small_activate", "小仓 激活", "%", 10, 2),
      trailTier("small_giveback", "小仓 回吐", "", 0.01, 4),
      trailTier("medium_activate", "中仓 激活", "%", 10, 2),
      trailTier("medium_giveback", "中仓 回吐", "", 0.01, 4),
      trailTier("large_activate", "大仓 激活", "%", 5, 2),
      trailTier("large_giveback", "大仓 回吐", "", 0.01, 4),
      trailTier("tier_small_ratio", "小仓档比例", "", 0.05, 4),
      trailTier("tier_large_ratio", "大仓档比例", "", 0.05, 4),
    ],
  },
  {
    key: "gate",
    name: "反向门控",
    desc: "反向信号减仓的准入条件",
    params: [
      {
        key: "reverse_gate_enabled",
        label: "反向门控",
        step: 1,
        precision: 0,
        scope: "account",
        carrier: { kind: "risk", field: "reverseGateEnabled" },
        propertyKey: "trade.accountN.reverse_gate",
        storeKey: "argus_account_risk.reverse_gate_enabled",
        hot: true,
        critical: true,
        type: "switch",
      },
      {
        key: "reverse_gate_min_profit_pct",
        label: "最低盈利要求",
        unit: "%",
        step: 1,
        precision: 2,
        min: 0,
        scope: "account",
        carrier: { kind: "risk", field: "reverseGateMinProfitPct" },
        propertyKey: "trade.accountN.reverse_gate_min_profit_pct",
        storeKey: "argus_account_risk.reverse_gate_min_profit_pct",
        hot: true,
      },
    ],
  },
  {
    key: "trend",
    name: "趋势闸",
    desc: "方向性门控：只拦逆势加仓",
    params: [
      {
        key: "trend_gate_window_hours",
        label: "动量窗口",
        unit: "h",
        step: 1,
        precision: 0,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "trendGateWindowHour" },
        propertyKey: "trade.trend_gate.window_hours",
        storeKey: "argus_config.trend_gate_window_hour",
        hot: true,
      },
      {
        key: "trend_gate_threshold_pct",
        label: "逆势阈值",
        unit: "%",
        step: 0.5,
        precision: 2,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "trendGateThresholdPct" },
        propertyKey: "trade.trend_gate.threshold_pct",
        storeKey: "argus_config.trend_gate_threshold_pct",
        hot: true,
      },
    ],
  },
  {
    key: "monitor",
    name: "巡检与告警",
    desc: "持仓巡检频率与告警阈值",
    params: [
      {
        key: "monitor_interval_second",
        label: "仓位巡检间隔",
        unit: "s",
        step: 1,
        precision: 0,
        min: 1,
        scope: "global",
        carrier: { kind: "config", field: "monitorIntervalSecond" },
        propertyKey: "position.monitor.interval_seconds",
        storeKey: "argus_config.monitor_interval_second",
        hot: true,
      },
      {
        key: "profit_threshold",
        label: "盈利告警阈值",
        unit: "%",
        step: 10,
        precision: 2,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "profitThreshold" },
        propertyKey: "position.monitor.profit_threshold",
        storeKey: "argus_config.profit_threshold",
        hot: true,
      },
      {
        key: "loss_threshold",
        label: "亏损告警阈值",
        unit: "%",
        step: 10,
        precision: 2,
        min: 0,
        scope: "global",
        carrier: { kind: "config", field: "lossThreshold" },
        propertyKey: "position.monitor.loss_threshold",
        storeKey: "argus_config.loss_threshold",
        hot: true,
      },
      {
        key: "server_port",
        label: "服务端口",
        step: 1,
        precision: 0,
        min: 1,
        max: 65535,
        scope: "global",
        carrier: { kind: "config", field: "serverPort" },
        propertyKey: "server.port",
        storeKey: "argus_config.server_port",
        hot: false,
      },
    ],
  },
];

function trailTier(key: string, label: string, unit: string, step: number, precision: number): ParamDef {
  return {
    key: `trail_${key}`,
    label,
    unit: unit || undefined,
    step,
    precision,
    min: 0,
    scope: "account",
    carrier: { kind: "trailTier", key },
    propertyKey: `position.monitor.trail.${key}`,
    storeKey: `argus_account_risk.trailing_stop_tiers_json[${key}]`,
    hot: true,
  };
}

export const ALL_PARAMS: ParamDef[] = PARAM_GROUPS.flatMap((group) => group.params);

export function findParam(key: string): ParamDef | undefined {
  return ALL_PARAMS.find((param) => param.key === key);
}

/** 参数取值上下文：账户级与币种级参数需要知道读写的是哪一行。 */
export interface ParamContext {
  /** monitorSymbols 下标 */
  symbolIndex: number;
  /** accountRisks 下标 */
  riskIndex: number;
}

export type ParamValues = Record<string, number>;

function parseJsonMap(raw?: string): Record<string, unknown> {
  if (!raw || !raw.trim()) return {};
  try {
    const parsed: unknown = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : {};
  } catch {
    // 手工写坏的 JSON 不应该让整页崩掉：当作空对象，编辑后会被重新序列化覆盖。
    return {};
  }
}

function toNumber(value: unknown): number {
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

/** 从快照读出某个参数的当前值，已按 scale 换算成页面显示单位。 */
export function readParam(param: ParamDef, snapshot: ArgusConfigSnapshot, ctx: ParamContext): number {
  const raw = readRaw(param, snapshot, ctx);
  return param.scale ? raw * param.scale : raw;
}

function readRaw(param: ParamDef, snapshot: ArgusConfigSnapshot, ctx: ParamContext): number {
  const carrier = param.carrier;
  switch (carrier.kind) {
    case "config":
      return toNumber(snapshot.config?.[carrier.field]);
    case "symbol":
      return toNumber(snapshot.monitorSymbols?.[ctx.symbolIndex]?.[carrier.field]);
    case "risk":
      return toNumber(snapshot.accountRisks?.[ctx.riskIndex]?.[carrier.field]);
    case "configExtra":
      return toNumber(parseJsonMap(snapshot.config?.extraConfigJson)[carrier.key]);
    case "riskExtra":
      return toNumber(parseJsonMap(snapshot.accountRisks?.[ctx.riskIndex]?.extraRiskJson)[carrier.key]);
    case "trailTier":
      return toNumber(parseJsonMap(snapshot.accountRisks?.[ctx.riskIndex]?.trailingStopTiersJson)[carrier.key]);
    default:
      return 0;
  }
}

/** 读出一个实例在给定上下文下的全部参数当前值，作为 diff 的基线。 */
export function readAllParams(snapshot: ArgusConfigSnapshot, ctx: ParamContext): ParamValues {
  const values: ParamValues = {};
  for (const param of ALL_PARAMS) {
    values[param.key] = readParam(param, snapshot, ctx);
  }
  return values;
}

/**
 * 把改动写回草稿。draft 必须是快照的深拷贝：直接改 snapshot 会让「放弃改动」失效，
 * 也会让 diff 基线跟着漂。
 */
export function applyParam(draft: ArgusConfigDraft, param: ParamDef, displayValue: number, ctx: ParamContext) {
  const value = param.scale ? displayValue / param.scale : displayValue;
  const carrier = param.carrier;
  switch (carrier.kind) {
    case "config":
      draft.config[carrier.field] = value;
      break;
    case "symbol": {
      const symbol = draft.monitorSymbols?.[ctx.symbolIndex];
      if (symbol) symbol[carrier.field] = value;
      break;
    }
    case "risk": {
      const risk = draft.accountRisks?.[ctx.riskIndex];
      if (risk) risk[carrier.field] = value;
      break;
    }
    case "configExtra": {
      const map = parseJsonMap(draft.config.extraConfigJson);
      map[carrier.key] = value;
      draft.config.extraConfigJson = JSON.stringify(map);
      break;
    }
    case "riskExtra": {
      const risk = draft.accountRisks?.[ctx.riskIndex];
      if (!risk) break;
      const map = parseJsonMap(risk.extraRiskJson);
      map[carrier.key] = value;
      risk.extraRiskJson = JSON.stringify(map);
      break;
    }
    case "trailTier": {
      const risk = draft.accountRisks?.[ctx.riskIndex];
      if (!risk) break;
      const map = parseJsonMap(risk.trailingStopTiersJson);
      map[carrier.key] = value;
      risk.trailingStopTiersJson = JSON.stringify(map);
      break;
    }
    default:
      break;
  }
}

export function formatParamValue(param: ParamDef, value: number): string {
  if (param.type === "switch") return value === 1 ? "开" : "关";
  const text = param.precision > 0 ? Number(value.toFixed(param.precision)).toString() : String(Math.round(value));
  return param.unit ? `${text} ${param.unit}` : text;
}

/** 极端行情快速下发的预设。全部指向真实参数键，不做「自动调参」。 */
export interface QuickAction {
  key: string;
  label: string;
  paramKey: string;
  value: number;
  danger?: boolean;
  note: string;
}

export const QUICK_ACTIONS: QuickAction[] = [
  { key: "raise-threshold", label: "阈值提到 8bp", paramKey: "signal_threshold", value: 8, note: "抬高信号阈值，降低触发频率" },
  { key: "cap-10", label: "上限降到 10 张", paramKey: "max_contracts", value: 10, note: "压低单账户敞口上限" },
  { key: "gate-off", label: "关闭反向门控", paramKey: "reverse_gate_enabled", value: 0, note: "允许反向信号无条件减仓" },
  {
    key: "pause-open",
    label: "暂停新开仓",
    paramKey: "default_order_size",
    value: 0,
    danger: true,
    note: "order_size = 0：停止开新仓，已有持仓继续由移动止盈与兜底止损看管",
  },
];

/** 一条待发布的参数改动。scopeLabel 说明它落在哪个账户 / 币种上。 */
export interface ParamChange {
  param: ParamDef;
  ctx: ParamContext;
  scopeLabel: string;
  from: number;
  to: number;
}

/** 草稿改动的存放形式：同一个参数在不同账户上的改动互不覆盖。 */
export interface ParamOverride {
  paramKey: string;
  ctx: ParamContext;
  value: number;
}

export function overrideKey(param: ParamDef, ctx: ParamContext): string {
  if (param.scope === "symbol") return `${param.key}@s${ctx.symbolIndex}`;
  if (param.scope === "account") return `${param.key}@a${ctx.riskIndex}`;
  return `${param.key}@g`;
}

/**
 * 把改动套用到已发布快照的深拷贝上，生成可提交的草稿。
 * 必须深拷贝：直接改 snapshot 会让「放弃改动」失效，diff 基线也会跟着漂。
 */
export function buildDraft(
  snapshot: ArgusConfigSnapshot,
  overrides: Record<string, ParamOverride>,
  releaseNote: string,
  instanceKey: string,
): ArgusConfigDraft {
  const cloned = JSON.parse(JSON.stringify(snapshot)) as ArgusConfigSnapshot;
  const draft: ArgusConfigDraft = {
    instanceKey,
    config: cloned.config,
    accounts: cloned.accounts ?? [],
    accountRisks: cloned.accountRisks ?? [],
    monitorSymbols: cloned.monitorSymbols ?? [],
    notification: cloned.notification,
    sessions: cloned.sessions ?? [],
    releaseNote,
  };
  for (const override of Object.values(overrides)) {
    const param = findParam(override.paramKey);
    if (param) applyParam(draft, param, override.value, override.ctx);
  }
  return draft;
}

/**
 * 跨实例同步前的适配检查：目标实例的账户数 / 币种数可能与源实例不同
 * （实例1 有 2 个账户，另外两个各 1 个），落不下的改动必须显式丢弃并告知，
 * 不能靠 applyParam 静默忽略——那会让人以为同步全量成功了。
 */
export function splitApplicableOverrides(
  target: ArgusConfigSnapshot,
  overrides: Record<string, ParamOverride>,
): { applicable: Record<string, ParamOverride>; skipped: ParamDef[] } {
  const applicable: Record<string, ParamOverride> = {};
  const skipped: ParamDef[] = [];
  for (const [key, override] of Object.entries(overrides)) {
    const param = findParam(override.paramKey);
    if (!param) continue;
    const fits =
      param.scope === "account"
        ? override.ctx.riskIndex < (target.accountRisks?.length ?? 0)
        : param.scope === "symbol"
          ? override.ctx.symbolIndex < (target.monitorSymbols?.length ?? 0)
          : true;
    if (fits) applicable[key] = override;
    else skipped.push(param);
  }
  return { applicable, skipped };
}

// ─── 从回测 / 寻优页带过来的参数预填 ─────────────────────────────────────────

/**
 * 一次「把这组参数带去发布」的预填请求。
 *
 * 它**只把值填进草稿**，不发布、不改任何线上参数。回测与寻优给出的是一份统计
 * 结论（而且寻优刻意不输出"最优参数"），发不发布是人的决定——所以这条链路的
 * 终点必须停在编辑器里，由人看过 diff、写完发布说明、过完关键参数二次确认之后
 * 才可能发出去。
 */
export interface ParamPrefill {
  /** catalog 的 param.key → 目标值。单位与 {@link readParam} 一致（已按 scale 换算成页面显示单位）。 */
  values: Record<string, number>;
  /**
   * 账户级参数落到哪个账户。取回测的 `accountLabel`（= `strategy_event.account_label`，
   * 与 `argus_account.account_name` 同源）。匹配不上时账户级参数全部丢弃并报出来——
   * 按下标兜底会把 challenger 的参数填到 champion 上。
   */
  accountLabel?: string;
  /** 币种级参数落到哪个 monitor symbol。匹配不上时同样丢弃。 */
  symbol?: string;
  /** 发布说明的预填文本。 */
  note?: string;
  /** 来源描述，显示在提示条上（如「批量扫描 #12 · 组 capOverride=26」）。 */
  source?: string;
}

/** 一项没能落下的预填，附上原因——静默丢弃会让人以为参数已经填全了。 */
export interface PrefillSkip {
  /** 参数目录里不存在这个键时为 null，此时只有 key 有意义。 */
  param: ParamDef | null;
  key: string;
  reason: string;
}

export interface PrefillResolution {
  overrides: Record<string, ParamOverride>;
  applied: ParamDef[];
  skipped: PrefillSkip[];
  /** 解析出的编辑上下文，用于把编辑器切到对应的账户 / 币种。 */
  ctx: ParamContext;
  accountResolved: boolean;
  symbolResolved: boolean;
}

/**
 * 把预填请求解析成可以直接塞进编辑器的改动集合。
 *
 * 三条判定，缺一不可：
 * 1. 账户级参数按 `accountLabel` 找 `argus_account.account_name` 定位到 accountRisks 下标，
 *    找不到就整类丢弃（实例1 有 champion / challenger 两个账户，填错等于改错实验体）；
 * 2. 币种级参数按 `symbol` 定位 monitorSymbols 下标，同样不兜底；
 * 3. 与该实例当前生产值相同的项直接丢掉，避免 diff 里出现一堆"改了个寂寞"的行。
 */
export function resolvePrefill(snapshot: ArgusConfigSnapshot, prefill: ParamPrefill): PrefillResolution {
  const symbolIndex = prefill.symbol
    ? (snapshot.monitorSymbols ?? []).findIndex((item) => item.symbol === prefill.symbol)
    : -1;
  const riskIndex = prefill.accountLabel
    ? (snapshot.accountRisks ?? []).findIndex((risk) => {
        const account = (snapshot.accounts ?? []).find((item) => item.id === risk.accountId);
        return account?.accountName === prefill.accountLabel;
      })
    : -1;
  const ctx: ParamContext = { symbolIndex: symbolIndex < 0 ? 0 : symbolIndex, riskIndex: riskIndex < 0 ? 0 : riskIndex };

  const overrides: Record<string, ParamOverride> = {};
  const applied: ParamDef[] = [];
  const skipped: PrefillSkip[] = [];

  for (const [key, value] of Object.entries(prefill.values)) {
    if (!Number.isFinite(value)) continue;
    const param = findParam(key);
    if (!param) {
      skipped.push({ param: null, key, reason: "参数目录里没有这个键，无法定位到配置快照的落点" });
      continue;
    }
    if (param.scope === "account" && riskIndex < 0) {
      skipped.push({
        param,
        key,
        reason: prefill.accountLabel
          ? `该实例没有名为「${prefill.accountLabel}」的账户，账户级参数不按下标兜底`
          : "来源没有给出账户，账户级参数无法定位",
      });
      continue;
    }
    if (param.scope === "symbol" && symbolIndex < 0) {
      skipped.push({
        param,
        key,
        reason: prefill.symbol ? `该实例没有 ${prefill.symbol} 的监控行` : "来源没有给出币种，币种级参数无法定位",
      });
      continue;
    }
    const current = readParam(param, snapshot, ctx);
    // 浮点比较留一点容差：回测参数经过 JSON 往返，1e-12 级的尾差不该显示成改动。
    if (Math.abs(current - value) < 1e-9) {
      skipped.push({ param, key, reason: "与该实例当前生产值一致，无需改动" });
      continue;
    }
    overrides[overrideKey(param, ctx)] = { paramKey: param.key, ctx, value };
    applied.push(param);
  }

  return { overrides, applied, skipped, ctx, accountResolved: riskIndex >= 0, symbolResolved: symbolIndex >= 0 };
}
