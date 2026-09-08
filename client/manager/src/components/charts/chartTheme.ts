"use client";

import { ColorType, CrosshairMode, LineStyle, type DeepPartial, type ChartOptions, type UTCTimestamp } from "lightweight-charts";

/**
 * 管理端图表的统一视觉令牌与时间口径。
 *
 * 取值直接对齐 src/app/globals.css 里的 --manager-* 变量：lightweight-charts 只接受
 * 具体色值，读不了 CSS 变量，所以这里必须留一份常量副本。改主题时两边一起改。
 */
export const chartPalette = {
  bg: "transparent",
  text: "#B7BDC6",
  textFaint: "#848E9C",
  grid: "rgba(43, 49, 57, 0.55)",
  border: "#2B3139",
  up: "#0ECB81",
  down: "#F6465D",
  primary: "#F0B90B",
  accent: "#4D7EFF",
  violet: "#A78BFA",
  orange: "#FF9F43",
  neutral: "#848E9C",
} as const;

/**
 * 时间口径：后端（argus-event / trade）返回的是**本地墙钟串** `YYYY-MM-DD HH:mm:ss`，
 * 与 logs/ 下的 JSONL、Telegram 消息逐字一致，不带时区。
 *
 * lightweight-charts 的 UTCTimestamp 一律按 UTC 渲染坐标轴。所以这里刻意把墙钟串
 * **当作 UTC** 折算成时间戳——图上显示的刻度就等于后端给的那串字，和表格、抽屉里的
 * 时间完全对得上。全页所有序列与标记都走这一个函数，彼此对齐；真实纪元时刻在页面上
 * 不需要，需要回链时用原始字符串，不做反向折算。
 */
export function wallClockToChartTime(wallClock: string): UTCTimestamp | null {
  const matched = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/.exec(wallClock.trim());
  if (!matched) return null;
  const [, y, mo, d, h, mi, s] = matched;
  const ms = Date.UTC(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s));
  return Math.floor(ms / 1000) as UTCTimestamp;
}

/** chartTime → 墙钟串，用于把折算回来的刻度显示成和后端一致的文本。 */
export function chartTimeToWallClock(time: UTCTimestamp): string {
  const date = new Date(time * 1000);
  const pad = (v: number) => String(v).padStart(2, "0");
  return (
    `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())} ` +
    `${pad(date.getUTCHours())}:${pad(date.getUTCMinutes())}:${pad(date.getUTCSeconds())}`
  );
}

/** 把一个墙钟串对齐到所属 K 线的开盘时刻，标记才能挂到蜡烛上。 */
export function alignToBar(time: UTCTimestamp, intervalSeconds: number): UTCTimestamp {
  if (intervalSeconds <= 0) return time;
  return (Math.floor(time / intervalSeconds) * intervalSeconds) as UTCTimestamp;
}

/** 各周期的秒数，与后端 intervalDuration 同口径。 */
export const intervalSeconds: Record<string, number> = {
  "1m": 60,
  "5m": 300,
  "15m": 900,
  "30m": 1800,
  "1h": 3600,
  "4h": 14400,
  "1d": 86400,
};

/** 管理端深色主题的图表基础配置。 */
export function baseChartOptions(): DeepPartial<ChartOptions> {
  return {
    layout: {
      background: { type: ColorType.Solid, color: chartPalette.bg },
      textColor: chartPalette.text,
      fontSize: 11,
      fontFamily:
        "'SF Mono', 'JetBrains Mono', 'Roboto Mono', Menlo, Consolas, monospace",
      attributionLogo: false,
      panes: { separatorColor: chartPalette.border, separatorHoverColor: chartPalette.primary },
    },
    grid: {
      vertLines: { color: chartPalette.grid, style: LineStyle.Dotted },
      horzLines: { color: chartPalette.grid, style: LineStyle.Dotted },
    },
    rightPriceScale: { borderColor: chartPalette.border, scaleMargins: { top: 0.12, bottom: 0.12 } },
    timeScale: {
      borderColor: chartPalette.border,
      timeVisible: true,
      secondsVisible: false,
      rightOffset: 2,
    },
    crosshair: {
      mode: CrosshairMode.Normal,
      vertLine: { color: chartPalette.primary, width: 1, style: LineStyle.Dashed, labelBackgroundColor: "#1E2329" },
      horzLine: { color: chartPalette.primary, width: 1, style: LineStyle.Dashed, labelBackgroundColor: "#1E2329" },
    },
    localization: {
      locale: "zh-CN",
      priceFormatter: (price: number) => price.toLocaleString("zh-CN", { maximumFractionDigits: 2 }),
    },
  };
}
