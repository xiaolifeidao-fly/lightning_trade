"use client";

import { chartPalette } from "@/components/charts/chartTheme";
import { RANGE_OPTIONS, toWallClock } from "../argus-signals/constants";

/**
 * 总览页的口径常量。
 *
 * 展示名（事件 / 门控 / 出场方式）一律用服务端返回的 *Label 字段；这里只放
 * 服务端不返回的东西：颜色、窗口换算、健康清单的判定阈值。
 */

/** 账户权益曲线的配色，按序取；同一实例最多两个账户（实例2 的 A/B）。 */
export const EQUITY_SERIES_COLORS = [
  chartPalette.primary,
  chartPalette.accent,
  chartPalette.violet,
  chartPalette.orange,
];

/** 健康项的三档色调，与实例卡片、生效状态共用同一套词。 */
export type HealthTone = "ok" | "warn" | "err" | "mute";

export const HEALTH_TONE_COLORS: Record<HealthTone, string> = {
  ok: chartPalette.up,
  warn: chartPalette.primary,
  err: chartPalette.down,
  mute: chartPalette.neutral,
};

/** K 线覆盖率低于这个值就报警：缺口段的触发点在图上挂不到 K 线。 */
export const COVERAGE_WARN_PCT = 99;
export const COVERAGE_ERROR_PCT = 95;

/** 权益曲线的降采样粒度，与原型一致（心跳每分钟一条，10 分钟一个点）。 */
export const EQUITY_BUCKET_SECONDS = 600;

/** 最近信号流只取一屏，看全量去信号复盘页。 */
export const RECENT_SIGNAL_LIMIT = 12;

/**
 * 实例状态与心跳的轮询间隔。
 *
 * 原来是 10 秒，理由写的是「心跳 5s 上报 / 15s TTL」——那个口径早就不在了：
 * 线上 argus.heartbeat.interval_seconds=30 / ttl_seconds=90。也就是说 10 秒一轮
 * 会把同一个心跳值重复取三遍，而每一轮是 8 个请求（其中行情时间线是最重的那个）。
 *
 * 60 秒的取舍：心跳 30 秒写一次、TTL 90 秒，所以掉线最迟 90+60 秒会显示出来，
 * 对巡检页足够；信号本身今天也才 ~9 分钟一次。要更快就点「刷新状态」。
 */
export const OVERVIEW_REFRESH_INTERVAL = 60_000;

function wallClockToDate(wallClock: string): Date | null {
  const matched = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/.exec(wallClock.trim());
  if (!matched) return null;
  const [, y, mo, d, h, mi, s] = matched;
  return new Date(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s));
}

export interface ResolvedRange {
  start?: string;
  end?: string;
}

/**
 * 相对窗口锚在**已入库数据的最新时刻**上，不是服务器当前时间——本库主力数据是
 * 6–8 月的回灌历史，按 now 回推「近 1 天」永远是空页。与信号复盘页同一口径。
 */
export function resolveRange(rangeKey: string, dataEnd: string): ResolvedRange {
  const option = RANGE_OPTIONS.find((item) => item.key === rangeKey);
  if (!option || option.hours === null) return {};
  const anchor = wallClockToDate(dataEnd) ?? new Date();
  const start = new Date(anchor.getTime() - option.hours * 3600 * 1000);
  return { start: toWallClock(start), end: toWallClock(anchor) };
}

/** 墙钟串距今多久（秒）；解析不出来返回 null。 */
export function wallClockAgeSeconds(wallClock: string): number | null {
  const date = wallClockToDate(wallClock);
  if (!date) return null;
  return Math.max(0, Math.round((Date.now() - date.getTime()) / 1000));
}

/** 秒 → 「3 秒前 / 12 分钟前 / 2.4 小时前 / 5.1 天前」。 */
export function fmtAge(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || Number.isNaN(seconds)) return "—";
  if (seconds < 60) return `${seconds} 秒前`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} 分钟前`;
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)} 小时前`;
  return `${(seconds / 86400).toFixed(1)} 天前`;
}
