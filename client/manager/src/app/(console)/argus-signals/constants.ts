"use client";

import { chartPalette } from "@/components/charts/chartTheme";

/**
 * 信号复盘页的口径常量与格式化。
 *
 * 这里只放**服务端不返回**的东西：颜色、时间范围预设、数值格式。
 * 展示名（事件 / 门控 / 出场方式 / 强度分档）一律用服务端返回的 *Label 字段，
 * 前端不再打一份映射表——写侧改常量时服务端会编译报错，前端副本只会静默漂移。
 */

/** 结果大类的色调，与行情主视图的触发点四态同色。 */
export const RESULT_KIND_COLORS: Record<string, string> = {
  open: chartPalette.up,
  blocked: chartPalette.orange,
  exit: chartPalette.accent,
  alert: chartPalette.primary,
};

/** 四类触发事件在「拦截原因分布」里的条形色，与 argus-market 的标记色一致。 */
export const EVENT_COLORS: Record<string, string> = {
  open: chartPalette.up,
  gate_block: chartPalette.violet,
  cap_skip: chartPalette.orange,
  trend_skip: chartPalette.accent,
  trailing_close: chartPalette.up,
  fixed_close: chartPalette.up,
  catastrophe_stop: chartPalette.down,
  external_close: chartPalette.neutral,
  manual_close: chartPalette.neutral,
  loss_alert: chartPalette.primary,
};

/** 信号强度三档的条形色：弱→中→强用灰、黄、绿，与原型 signals.html 一致。 */
export const STRENGTH_COLORS: Record<string, string> = {
  weak: chartPalette.neutral,
  medium: chartPalette.primary,
  strong: chartPalette.up,
};

/** 出场方式的色调。不计入策略胜率的两类刻意用灰，视觉上就与策略成绩分开。 */
export const EXIT_KIND_COLORS: Record<string, string> = {
  trailing_close: chartPalette.up,
  fixed_close: chartPalette.accent,
  catastrophe_stop: chartPalette.down,
  reduce_to_zero: chartPalette.violet,
  external_close: chartPalette.neutral,
  manual_close: chartPalette.neutral,
  "": chartPalette.primary,
};

/** 事件时间轴轴点色调，取服务端给的 tone。 */
export const TONE_COLORS: Record<string, string> = {
  ok: chartPalette.up,
  warn: chartPalette.primary,
  err: chartPalette.down,
  mute: chartPalette.neutral,
};

export interface RangeOption {
  key: string;
  label: string;
  /** 相对现在往回推的小时数；null = 由服务端按「已入库数据的最新时刻」回推。 */
  hours: number | null;
}

/**
 * 时间范围预设。默认走 latest 而不是「近 24 小时」：本库主力数据是 6–8 月的
 * 回灌历史，按服务器当前时间兜底只会给一张空白页。
 */
export const RANGE_OPTIONS: RangeOption[] = [
  { key: "latest", label: "最近一段（按已入库数据）", hours: null },
  { key: "d1", label: "近 1 天", hours: 24 },
  { key: "d3", label: "近 3 天", hours: 72 },
  { key: "d7", label: "近 7 天", hours: 168 },
  { key: "d30", label: "近 30 天", hours: 720 },
  { key: "d90", label: "近 90 天", hours: 2160 },
];

/** episode 的窗口口径。同一批持仓按三种口径数出来的条数不同，必须显式选。 */
export const TIME_FIELD_OPTIONS = [
  { value: "opened", label: "按建仓时刻（决策归集口径）" },
  { value: "closed", label: "按平仓时刻" },
  { value: "overlap", label: "按窗口内有过持仓" },
];

export const SIGNAL_PAGE_SIZE = 50;
export const EPISODE_PAGE_SIZE = 20;

/** 逐账户判定表的默认列宽基线，抽屉宽度按它定。 */
export const DETAIL_DRAWER_WIDTH = 960;

// ─── 数值格式 ────────────────────────────────────────────────────────────────

/** 空值一律显示「—」：0 与「没有这个观测」是两回事，见 fillPriceAvailable 的口径。 */
export const EMPTY = "—";

export function fmtNum(value: number | null | undefined, digits = 2, suffix = ""): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${value.toFixed(digits)}${suffix}`;
}

/**
 * 比率字段（openRate 等）转百分比显示。
 *
 * 后端的 openRate 是**分数**（0..1，见 service/argus_event/stats.go 的
 * `round4(opened/total)`，aggregate_test.go 也按分数断言），而同一批出参里的
 * winRate 已经是百分数。这里显式乘 100，别再直接把分数丢给 fmtNum 加个 "%"——
 * 那会把 41.8% 显示成 0.4%。
 */
export function fmtRate(value: number | null | undefined, digits = 1): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${(value * 100).toFixed(digits)}%`;
}

/** 带符号的数值，涨跌色由调用方按 signOf 决定。 */
export function fmtSigned(value: number | null | undefined, digits = 2, suffix = ""): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return `${value > 0 ? "+" : ""}${value.toFixed(digits)}${suffix}`;
}

export function fmtPrice(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

export function fmtInt(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
  return String(value);
}

/** 涨跌色：正绿负红，0 与空值走中性，不要把 0 画成绿的。 */
export function signColor(value: number | null | undefined): string | undefined {
  if (value === null || value === undefined || Number.isNaN(value) || value === 0) return undefined;
  return value > 0 ? chartPalette.up : chartPalette.down;
}

/** 秒 → 人读时长，与服务端 formatDuration 同口径。 */
export function fmtDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds < 0) return EMPTY;
  if (seconds >= 86400) return `${(seconds / 86400).toFixed(1)} 天`;
  if (seconds >= 3600) return `${(seconds / 3600).toFixed(1)} 小时`;
  return `${Math.round(seconds / 60)} 分钟`;
}

/** 本地墙钟串 `YYYY-MM-DD HH:mm:ss`，与 argus-event 接口的时间口径一致。 */
export function toWallClock(date: Date): string {
  const pad = (v: number) => String(v).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
}

/** 列表里的时间只显示到「月-日 时:分:秒」，年份在窗口提示里已经写明。 */
export function shortTs(ts: string): string {
  return ts.length > 5 ? ts.slice(5) : ts || EMPTY;
}
