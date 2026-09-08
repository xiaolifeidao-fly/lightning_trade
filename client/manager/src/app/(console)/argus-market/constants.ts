"use client";

import { chartPalette } from "@/components/charts/chartTheme";

/**
 * 行情主视图的口径常量：四类触发点、周期、数据源、时间范围。
 *
 * 触发点的 key 与写侧 `argus_single/pkg/eventlog` 的事件常量逐字一致
 * （open / cap_skip / gate_block / trend_skip），不要在这里另起名字。
 */

export type TriggerKind = "open" | "gate_block" | "cap_skip" | "trend_skip";

export interface TriggerKindMeta {
  key: TriggerKind;
  label: string;
  color: string;
  /** 成交画在蜡烛下方，三类拦截画在上方——一眼分开「做成了」和「被挡住」。 */
  position: "belowBar" | "aboveBar";
  shape: "arrowUp" | "arrowDown" | "circle" | "square";
}

export const TRIGGER_KINDS: TriggerKindMeta[] = [
  { key: "open", label: "成功开仓", color: chartPalette.up, position: "belowBar", shape: "arrowUp" },
  { key: "gate_block", label: "反向门控", color: chartPalette.violet, position: "aboveBar", shape: "arrowDown" },
  { key: "cap_skip", label: "上限跳过", color: chartPalette.orange, position: "aboveBar", shape: "arrowDown" },
  { key: "trend_skip", label: "趋势闸", color: chartPalette.accent, position: "aboveBar", shape: "arrowDown" },
];

export const TRIGGER_KIND_MAP: Record<string, TriggerKindMeta> = Object.fromEntries(
  TRIGGER_KINDS.map((item) => [item.key, item]),
) as Record<string, TriggerKindMeta>;

export const MARKET_INTERVALS = ["1m", "5m", "1h", "1d"] as const;
export type MarketInterval = (typeof MARKET_INTERVALS)[number];

export const PLATFORM_LABELS: Record<string, string> = {
  binance: "币安",
  deepcoin: "DeepCoin",
};

/** 数据源：单源看蜡烛，双源在币安蜡烛上叠一条 DeepCoin 收盘线做对比。 */
export type MarketSource = "binance" | "deepcoin" | "both";

export interface RangeOption {
  key: string;
  label: string;
  /** 相对现在往回推的小时数；null = 由后端按「已入库数据的最新时刻」回推默认窗口。 */
  hours: number | null;
}

export const RANGE_OPTIONS: RangeOption[] = [
  { key: "latest", label: "最近一段（按已入库数据）", hours: null },
  { key: "h6", label: "近 6 小时", hours: 6 },
  { key: "d1", label: "近 1 天", hours: 24 },
  { key: "d3", label: "近 3 天", hours: 72 },
  { key: "d7", label: "近 7 天", hours: 168 },
  { key: "d30", label: "近 30 天", hours: 720 },
];

/**
 * 同一根 K 线上最多叠几个标记。
 *
 * 5 不是拍脑袋：实测（8.18–8.21）单分钟最多 5 次触发，1m 视图按这个上限叠满就够。
 * 超出的不静默丢弃——由页面统计出溢出条数显式提示。
 */
export const MAX_MARKERS_PER_BAR = 5;

/** 触发点最多拉几页（服务端单页上限 200）。超出时页面显式提示已截断。 */
export const MAX_TRIGGER_PAGES = 5;
export const TRIGGER_PAGE_SIZE = 200;
