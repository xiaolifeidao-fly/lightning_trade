"use client";

import { useEffect, useMemo, useState, type PointerEvent } from "react";
import { Button, Segmented, Select, Space, Table, Tag, Tooltip, Typography, message } from "antd";
import { AimOutlined, BarChartOutlined, DownOutlined, QuestionCircleOutlined, ReloadOutlined, SearchOutlined, UpOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import {
  fetchLatestNewsSentiments,
  fetchLatestPressureAnalyses,
  fetchTradeSimulationAnalysis,
  NewsSentimentRecord,
  PressureAnalysisRecord,
  type PressureLevel,
  type TradeSimulationAnalysisRecord,
  type TradeSimulationMarker,
} from "../trade-orders/api/trade.api";
import styles from "./page.module.css";

const { Text } = Typography;

const DEFAULT_PLATFORMS = [
  { label: "Binance", value: "binance" },
];

// AI 预测目前只跑 BTC，其它币种无预测数据，因此只提供 BTC。
const DEFAULT_COINS = [
  { label: "BTC / USDT", value: "BTC" },
];

// K线展示周期（图表上画的真实 K 线）
const INTERVAL_OPTIONS = [
  { label: "1分钟", value: "1m" },
  { label: "5分钟", value: "5m" },
  { label: "15分钟", value: "15m" },
  { label: "1小时", value: "1h" },
  { label: "4小时", value: "4h" },
  { label: "1日", value: "1d" },
];

// 每个预测周期(horizon)一条线，固定调色板，按返回顺序取色。
const SERIES_PALETTE = ["#4D7EFF", "#0ECB81", "#F0B90B", "#C04DFF"];

// 区间命中收紧判定：必须完整覆盖真实波动，且区间利用率(真实宽/预测宽) ≥ 此阈值，否则视为「过宽」(覆盖但区间报得太松)。
const BAND_UTIL_THRESHOLD = 0.5;
type BandHitState = "hit" | "wide" | "miss" | "none";
function bandHitState(record: { predictLow: number; predictHigh: number; bandContain: boolean; bandUtil: number }): BandHitState {
  if (!record.predictLow && !record.predictHigh) return "none";
  if (!record.bandContain) return "miss";
  return record.bandUtil >= BAND_UTIL_THRESHOLD ? "hit" : "wide";
}

// 复核表的一行：在 marker 基础上带上所属预测周期。
type MarkerRow = TradeSimulationMarker & { seriesLabel: string; seriesInterval: string };

type BasePoint = {
  timestamp: number;
  time: string;
  hasReal: boolean;
  openPrice?: number;
  highPrice?: number;
  lowPrice?: number;
  closePrice?: number;
};

// 单条预测线在图表上的落点。future=true 表示未到期、无真实价对比。
type SeriesDot = {
  index: number;
  x: number;
  y: number;
  price: number;
  predictHigh?: number;
  predictLow?: number;
  yHigh?: number; // 预测区间最高价的纵坐标
  yLow?: number; // 预测区间最低价的纵坐标
  invalidation?: number; // 失效价位
  yInvalid?: number; // 失效价位的纵坐标
  diffRate?: number;
  matched?: boolean;
  future: boolean;
  time: string;
  createdTime?: string;
  signal?: string;
  reason?: string;
};

type SeriesRender = {
  interval: string;
  label: string;
  color: string;
  dots: SeriesDot[];
  line: string;
};

type HoverState = {
  index: number;
  x: number;
  y: number;
};

function formatPrice(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  if (Math.abs(value) >= 1000) {
    return value.toFixed(2);
  }
  if (Math.abs(value) >= 1) {
    return value.toFixed(4);
  }
  return value.toFixed(6);
}

function buildPolyline(points: Array<{ x: number; y: number }>) {
  return points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(" ");
}

function pct(value: number) {
  return `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
}

// 消息面方向 → 标签文案与配色
const SENTIMENT_META: Record<string, { text: string; color: string }> = {
  bullish: { text: "利多", color: "green" },
  bearish: { text: "利空", color: "red" },
  neutral: { text: "中性", color: "default" },
};

function sentimentMeta(sentiment: string) {
  return SENTIMENT_META[(sentiment || "").toLowerCase()] ?? { text: sentiment || "未知", color: "default" };
}

// 压力面倾向 → 标签文案与配色
const BIAS_META: Record<string, { text: string; color: string }> = {
  long: { text: "偏多", color: "green" },
  short: { text: "偏空", color: "red" },
  neutral: { text: "中性", color: "default" },
};

function biasMeta(bias: string) {
  return BIAS_META[(bias || "").toLowerCase()] ?? { text: bias || "未知", color: "default" };
}

// 压力面价位分组：阻力(short)/支撑(long)，按强度降序展示前 4 个，带强度条与原因。
function PressureLevelGroup({
  title,
  tone,
  levels,
}: {
  title: string;
  tone: "short" | "long";
  levels?: PressureLevel[];
}) {
  const sorted = [...(levels ?? [])].sort((a, b) => (b.strength ?? 0) - (a.strength ?? 0)).slice(0, 4);
  return (
    <div className={styles.levelGroup}>
      <div className={`${styles.levelGroupTitle} ${tone === "short" ? styles.levelToneShort : styles.levelToneLong}`}>
        {title}
      </div>
      {sorted.length === 0 ? (
        <div className={styles.levelEmpty}>暂无</div>
      ) : (
        sorted.map((lv, index) => (
          <div key={index} className={styles.levelRow} title={lv.reason || ""}>
            <span className={styles.levelPrice}>{formatPrice(lv.price)}</span>
            <span className={styles.levelStrengthBar}>
              <span
                className={tone === "short" ? styles.levelStrengthFillShort : styles.levelStrengthFillLong}
                style={{ width: `${Math.round(Math.min(Math.max(lv.strength ?? 0, 0), 1) * 100)}%` }}
              />
            </span>
            <span className={styles.levelReason}>{lv.reason || "—"}</span>
          </div>
        ))
      )}
    </div>
  );
}

// 把后端返回的时间(ISO 字符串)格式化为「YYYY-MM-DD HH:mm」本地时间
function formatNewsTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatAxisTime(time: string, interval?: string) {
  if (interval === "1d") {
    return time.slice(0, 5);
  }
  // 展示完整的预测时间点（日期 + 时分），便于精确定位每个预测点。
  return time;
}

function getMatchScore(data: TradeSimulationAnalysisRecord | null) {
  const total = (data?.matchCount ?? 0) + (data?.diffCount ?? 0);
  if (!total) {
    return 0;
  }
  return Math.round(((data?.matchCount ?? 0) / total) * 100);
}

function SimulationCompareChart({
  data,
  loading,
  hidden,
  onToggle,
}: {
  data: TradeSimulationAnalysisRecord | null;
  loading: boolean;
  hidden: Set<string>;
  onToggle: (interval: string) => void;
}) {
  const [hover, setHover] = useState<HoverState | null>(null);
  const [pinnedIndex, setPinnedIndex] = useState<number | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const toggleExpanded = (key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const chart = useMemo(() => {
    const realKlines = data?.realKlines ?? [];
    const seriesList = data?.series ?? [];
    if (realKlines.length === 0) {
      return null;
    }

    const realByTimestamp = new Map(realKlines.map((item) => [item.timestamp, item]));
    const lastRealTs = realKlines[realKlines.length - 1].timestamp;

    // 时间轴 = 真实 K 线 ∪ 所有预测周期的预测点时间戳。未到期预测点排在真实 K 线右侧。
    const timeByTs = new Map<number, string>();
    realKlines.forEach((item) => timeByTs.set(item.timestamp, item.time));
    seriesList.forEach((s) => s.aiPoints.forEach((p) => {
      if (!timeByTs.has(p.timestamp)) {
        timeByTs.set(p.timestamp, p.time);
      }
    }));
    const timestamps = Array.from(timeByTs.keys()).sort((a, b) => a - b);
    if (!timestamps.length) {
      return null;
    }
    const indexByTs = new Map(timestamps.map((ts, index) => [ts, index]));

    const basePoints: BasePoint[] = timestamps.map((ts) => {
      const real = realByTimestamp.get(ts);
      return {
        timestamp: ts,
        time: timeByTs.get(ts) ?? "",
        hasReal: Boolean(real),
        openPrice: real?.openPrice,
        highPrice: real?.highPrice,
        lowPrice: real?.lowPrice,
        closePrice: real?.closePrice,
      };
    });

    const visibleSeries = seriesList.filter((s) => !hidden.has(s.interval));

    // Y 轴范围：真实 OHLC + 当前可见预测线的价格。
    const values: number[] = [];
    basePoints.forEach((item) => {
      if (item.hasReal) {
        values.push(item.highPrice as number, item.lowPrice as number, item.closePrice as number);
      }
    });
    visibleSeries.forEach((s) => s.aiPoints.forEach((p) => {
      values.push(p.price);
      if (p.predictHigh > 0) values.push(p.predictHigh);
      if (p.predictLow > 0) values.push(p.predictLow);
      if (p.invalidation > 0) values.push(p.invalidation);
    }));
    if (!values.length) {
      values.push(0, 1);
    }
    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = Math.max(max - min, Math.abs(max) * 0.008, 1);
    const yMin = min - range * 0.12;
    const yMax = max + range * 0.12;

    const width = 1180;
    const height = 520;
    const padding = { left: 76, right: 42, top: 34, bottom: 68 };
    const innerWidth = width - padding.left - padding.right;
    const innerHeight = height - padding.top - padding.bottom;
    const xForIndex = (index: number) => padding.left + (innerWidth * index) / Math.max(basePoints.length - 1, 1);
    const yForValue = (value: number) => padding.top + ((yMax - value) / (yMax - yMin)) * innerHeight;

    const realLine = basePoints
      .map((item, index) => ({ item, index }))
      .filter(({ item }) => item.hasReal)
      .map(({ item, index }) => ({ x: xForIndex(index), y: yForValue(item.closePrice as number) }));

    const seriesRender: SeriesRender[] = seriesList.map((s, seriesIndex) => {
      const color = SERIES_PALETTE[seriesIndex % SERIES_PALETTE.length];
      const markerByTs = new Map(s.markers.map((m) => [m.timestamp, m]));
      const dots: SeriesDot[] = s.aiPoints
        .map((p) => {
          const index = indexByTs.get(p.timestamp) ?? -1;
          const marker = markerByTs.get(p.timestamp);
          return {
            index,
            x: xForIndex(index),
            y: yForValue(p.price),
            price: p.price,
            predictHigh: p.predictHigh > 0 ? p.predictHigh : undefined,
            predictLow: p.predictLow > 0 ? p.predictLow : undefined,
            yHigh: p.predictHigh > 0 ? yForValue(p.predictHigh) : undefined,
            yLow: p.predictLow > 0 ? yForValue(p.predictLow) : undefined,
            invalidation: p.invalidation > 0 ? p.invalidation : undefined,
            yInvalid: p.invalidation > 0 ? yForValue(p.invalidation) : undefined,
            diffRate: marker?.diffRate,
            matched: marker?.matched,
            future: p.timestamp > lastRealTs,
            time: p.time,
            createdTime: p.createdTime,
            signal: p.signal,
            reason: p.reason,
          };
        })
        .filter((dot) => dot.index >= 0)
        .sort((a, b) => a.index - b.index);
      return {
        interval: s.interval,
        label: s.label,
        color,
        dots,
        line: buildPolyline(dots),
      };
    });

    // 悬浮时按时间轴索引聚合各可见预测线的落点，用于 tooltip。
    const dotsByIndex = new Map<number, Array<{ interval: string; label: string; color: string; dot: SeriesDot }>>();
    seriesRender.forEach((s) => {
      if (hidden.has(s.interval)) {
        return;
      }
      s.dots.forEach((dot) => {
        const list = dotsByIndex.get(dot.index) ?? [];
        list.push({ interval: s.interval, label: s.label, color: s.color, dot });
        dotsByIndex.set(dot.index, list);
      });
    });

    return {
      width,
      height,
      padding,
      basePoints,
      yMin,
      yMax,
      xForIndex,
      yForValue,
      realLine,
      seriesRender,
      dotsByIndex,
    };
  }, [data, hidden]);

  if (!chart || !data) {
    const emptyText = loading
      ? "正在加载数据"
      : data
        ? "暂无真实盘K线数据"
        : "请选择平台和币种查看模拟盘分析";
    return (
      <div className={styles.emptyChart}>
        <BarChartOutlined />
        <span>{emptyText}</span>
      </div>
    );
  }

  const ticks = Array.from({ length: 6 }, (_, index) => {
    const value = chart.yMin + ((chart.yMax - chart.yMin) * index) / 5;
    return { value, y: chart.yForValue(value) };
  }).reverse();

  const handlePointerMove = (event: PointerEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const ratioX = chart.width / rect.width;
    const ratioY = chart.height / rect.height;
    const x = (event.clientX - rect.left) * ratioX;
    const rawIndex = Math.round(
      ((x - chart.padding.left) / (chart.width - chart.padding.left - chart.padding.right)) * (chart.basePoints.length - 1),
    );
    const index = Math.max(0, Math.min(chart.basePoints.length - 1, rawIndex));
    setHover({ index, x: chart.xForIndex(index), y: (event.clientY - rect.top) * ratioY });
  };

  const hoverBase = hover ? chart.basePoints[hover.index] : null;
  const hoverDots = hover ? chart.dotsByIndex.get(hover.index) ?? [] : [];
  const hoverHasReason = hoverDots.some((entry) => entry.dot.reason);
  const tooltipWidth = 268;
  const tooltipHeight = 84 + Math.max(hoverDots.length, 1) * 30 + (hoverHasReason ? 96 : 0);
  const tooltipX = hover ? Math.min(Math.max(hover.x + 18, chart.padding.left), chart.width - tooltipWidth - 12) : 0;
  const tooltipY = hover ? Math.min(Math.max(hover.y - 86, chart.padding.top), chart.height - tooltipHeight - 8) : 0;

  const candleWidth = Math.max(3, Math.min(10, 700 / chart.basePoints.length));

  const pinnedBase = pinnedIndex !== null ? chart.basePoints[pinnedIndex] ?? null : null;
  const pinnedDots = pinnedIndex !== null ? chart.dotsByIndex.get(pinnedIndex) ?? [] : [];

  return (
    <div className={styles.chartShell}>
      <div className={styles.chartHeader}>
        <div>
          <Text className="manager-section-label">REAL MARKET VS AI PAPER TRADING</Text>
          <h2>{data.symbol || `${data.coinCode}/USDT`} 模拟盘拟合走势</h2>
        </div>
        <div className={styles.chartLegend}>
          <span><i className={styles.legendReal} />真实收盘</span>
          {chart.seriesRender.map((s) => {
            const off = hidden.has(s.interval);
            return (
              <span
                key={s.interval}
                onClick={() => onToggle(s.interval)}
                style={{ cursor: "pointer", opacity: off ? 0.4 : 1, textDecoration: off ? "line-through" : "none" }}
                title={off ? "点击显示该预测周期" : "点击隐藏该预测周期"}
              >
                <i style={{ background: s.color }} />{s.label}预测
              </span>
            );
          })}
          <span className={styles.legendHint}>空心点 = 未来预测(待结算) · 竖向误差棒 = AI 预测波动区间</span>
        </div>
      </div>

      <svg
        className={styles.chart}
        viewBox={`0 0 ${chart.width} ${chart.height}`}
        role="img"
        aria-label="真实K线与多周期AI预测走势对比"
        onPointerMove={handlePointerMove}
        onPointerLeave={() => setHover(null)}
        onClick={() => {
          if (hover && (chart.dotsByIndex.get(hover.index)?.length ?? 0) > 0) {
            setPinnedIndex(hover.index);
            setExpanded(new Set());
          }
        }}
        style={{ cursor: hover && (chart.dotsByIndex.get(hover.index)?.length ?? 0) > 0 ? "pointer" : "default" }}
      >
        <defs>
          <linearGradient id="simulationChartBg" x1="0" x2="1" y1="0" y2="1">
            <stop offset="0%" stopColor="rgba(240,185,11,0.12)" />
            <stop offset="45%" stopColor="rgba(77,126,255,0.07)" />
            <stop offset="100%" stopColor="rgba(11,14,17,0)" />
          </linearGradient>
          <linearGradient id="realLineGradient" x1="0" x2="1">
            <stop offset="0%" stopColor="#FCD535" />
            <stop offset="100%" stopColor="#F0B90B" />
          </linearGradient>
          <filter id="lineGlow" x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        <rect width={chart.width} height={chart.height} rx="10" fill="url(#simulationChartBg)" />
        {ticks.map((tick) => (
          <g key={tick.value}>
            <line
              x1={chart.padding.left}
              x2={chart.width - chart.padding.right}
              y1={tick.y}
              y2={tick.y}
              stroke="rgba(255,255,255,0.08)"
            />
            <text x="18" y={tick.y + 4} fill="rgba(234,236,239,0.62)" fontSize="12">
              {formatPrice(tick.value)}
            </text>
          </g>
        ))}

        {chart.basePoints.map((item, index) => {
          if (!item.hasReal) {
            return null;
          }
          const x = chart.xForIndex(index);
          const openY = chart.yForValue(item.openPrice as number);
          const closeY = chart.yForValue(item.closePrice as number);
          const highY = chart.yForValue(item.highPrice as number);
          const lowY = chart.yForValue(item.lowPrice as number);
          const up = (item.closePrice as number) >= (item.openPrice as number);
          const color = up ? "#0ECB81" : "#F6465D";

          return (
            <g key={item.timestamp} opacity={hover && hover.index !== index ? 0.45 : 1}>
              <line x1={x} x2={x} y1={highY} y2={lowY} stroke={color} strokeWidth="1.2" opacity="0.76" />
              <rect
                x={x - candleWidth / 2}
                y={Math.min(openY, closeY)}
                width={candleWidth}
                height={Math.max(Math.abs(closeY - openY), 2)}
                fill={up ? "rgba(14,203,129,0.24)" : "rgba(246,70,93,0.22)"}
                stroke={color}
                strokeWidth="1"
                rx="1.4"
              />
            </g>
          );
        })}

        <polyline
          points={buildPolyline(chart.realLine)}
          fill="none"
          stroke="url(#realLineGradient)"
          strokeWidth="3"
          filter="url(#lineGlow)"
        />

        {chart.seriesRender.map((s) => {
          if (hidden.has(s.interval) || s.dots.length === 0) {
            return null;
          }
          return (
            <g key={`line-${s.interval}`}>
              {/* 预测波动区间：每个落点画一根 [预测最低,预测最高] 的竖向误差棒，带顶/底端帽。 */}
              {s.dots.map((dot) =>
                dot.yHigh !== undefined && dot.yLow !== undefined ? (
                  <g key={`band-${s.interval}-${dot.index}`} opacity={dot.future ? 0.5 : 0.7}>
                    <line x1={dot.x} x2={dot.x} y1={dot.yHigh} y2={dot.yLow} stroke={s.color} strokeWidth="1.4" opacity="0.55" />
                    <line x1={dot.x - 3} x2={dot.x + 3} y1={dot.yHigh} y2={dot.yHigh} stroke={s.color} strokeWidth="1.4" />
                    <line x1={dot.x - 3} x2={dot.x + 3} y1={dot.yLow} y2={dot.yLow} stroke={s.color} strokeWidth="1.4" />
                  </g>
                ) : null,
              )}
              {/* 失效价位：方向被证伪的关键价位，画一根红色虚线短横标记。 */}
              {s.dots.map((dot) =>
                dot.yInvalid !== undefined ? (
                  <line
                    key={`invalid-${s.interval}-${dot.index}`}
                    x1={dot.x - 5}
                    x2={dot.x + 5}
                    y1={dot.yInvalid}
                    y2={dot.yInvalid}
                    stroke="#F6465D"
                    strokeWidth="1.6"
                    strokeDasharray="2 2"
                    opacity={dot.future ? 0.55 : 0.85}
                  />
                ) : null,
              )}
              {s.dots.length > 1 && (
                <polyline
                  points={s.line}
                  fill="none"
                  stroke={s.color}
                  strokeWidth="2.4"
                  strokeDasharray="9 7"
                  strokeLinecap="round"
                  opacity="0.92"
                />
              )}
              {s.dots.map((dot) => (
                <circle
                  key={`${s.interval}-${dot.index}`}
                  cx={dot.x}
                  cy={dot.y}
                  r={dot.future ? 4.4 : Math.abs(dot.diffRate ?? 0) >= 0.8 ? 6 : 4.4}
                  fill={dot.future ? "rgba(11,14,17,0.9)" : dot.matched === false ? "#F6465D" : s.color}
                  stroke={s.color}
                  strokeWidth={dot.future ? 2 : 2.2}
                  strokeDasharray={dot.future ? "2.4 2.2" : undefined}
                />
              ))}
            </g>
          );
        })}

        {chart.basePoints
          .filter((_, index) => index % Math.max(1, Math.ceil(chart.basePoints.length / 7)) === 0)
          .map((item) => {
            const index = chart.basePoints.findIndex((row) => row.timestamp === item.timestamp);
            return (
              <text key={item.timestamp} x={chart.xForIndex(index)} y={chart.height - 28} textAnchor="middle" fill="rgba(234,236,239,0.52)" fontSize="12">
                {formatAxisTime(item.time, data.interval)}
              </text>
            );
          })}

        {hover && hoverBase && (
          <g>
            <line
              x1={hover.x}
              x2={hover.x}
              y1={chart.padding.top}
              y2={chart.height - chart.padding.bottom}
              stroke="rgba(255,255,255,0.18)"
              strokeDasharray="4 5"
            />
            {hoverBase.hasReal && (
              <circle cx={hover.x} cy={chart.yForValue(hoverBase.closePrice as number)} r="5" fill="#FCD535" stroke="#0B0E11" strokeWidth="2" />
            )}
            {hoverDots.map((entry) => (
              <circle key={`hover-${entry.label}`} cx={hover.x} cy={entry.dot.y} r="5" fill={entry.color} stroke="#0B0E11" strokeWidth="2" />
            ))}
            <foreignObject x={tooltipX} y={tooltipY} width={tooltipWidth} height={tooltipHeight}>
              <div className={styles.tooltip}>
                <strong>时间 {hoverBase.time}</strong>
                <div><span>真实收盘</span><b>{hoverBase.hasReal ? formatPrice(hoverBase.closePrice as number) : "未到期"}</b></div>
                {hoverDots.length === 0 ? (
                  <em>该时刻无预测点</em>
                ) : (
                  <>
                    {hoverDots.map((entry) => (
                      <div key={`tt-${entry.interval}`} className={styles.tooltipReasonGroup}>
                        <div>
                          <span style={{ color: entry.color }}>{entry.label}预测</span>
                          <b>
                            {formatPrice(entry.dot.price)}
                            {entry.dot.future
                              ? " · 待结算"
                              : entry.dot.diffRate !== undefined
                                ? ` · ${pct(entry.dot.diffRate)}`
                                : ""}
                          </b>
                        </div>
                        {entry.dot.predictHigh !== undefined && entry.dot.predictLow !== undefined ? (
                          <div>
                            <span>预测区间</span>
                            <b>{formatPrice(entry.dot.predictLow)} ~ {formatPrice(entry.dot.predictHigh)}</b>
                          </div>
                        ) : null}
                        {entry.dot.invalidation !== undefined ? (
                          <div>
                            <span>失效价位</span>
                            <b style={{ color: "#F6465D" }}>{formatPrice(entry.dot.invalidation)}</b>
                          </div>
                        ) : null}
                        {entry.dot.reason ? <p className={styles.tooltipReason}>{entry.dot.reason}</p> : null}
                      </div>
                    ))}
                    {hoverHasReason ? <span className={styles.tooltipHint}>点击固定查看完整理由</span> : null}
                  </>
                )}
              </div>
            </foreignObject>
          </g>
        )}
      </svg>

      {pinnedBase && (
        <div className={styles.pinnedCard}>
          <div className={styles.pinnedHeader}>
            <div>
              <Text className="manager-section-label">PREDICTION DETAIL</Text>
              <h4>时间 {pinnedBase.time}</h4>
            </div>
            <button type="button" className={styles.pinnedClose} onClick={() => setPinnedIndex(null)} aria-label="关闭">
              ×
            </button>
          </div>
          <div className={styles.pinnedGrid}>
            <div><span>真实收盘</span><b>{pinnedBase.hasReal ? formatPrice(pinnedBase.closePrice as number) : "未到期"}</b></div>
          </div>
          {pinnedDots.length === 0 ? (
            <p className={styles.pinnedReasonClamp}>该时刻暂无预测点。</p>
          ) : (
            pinnedDots.map((entry) => {
              const reason = entry.dot.reason;
              const isExpanded = expanded.has(entry.interval);
              return (
                <div key={`pin-${entry.interval}`} className={styles.pinnedReasonBlock}>
                  <div className={styles.pinnedGrid}>
                    <div><span>预测周期</span><b style={{ color: entry.color }}>{entry.label}</b></div>
                    <div><span>AI模拟盘</span><b>{formatPrice(entry.dot.price)}</b></div>
                    {entry.dot.predictHigh !== undefined && entry.dot.predictLow !== undefined ? (
                      <div><span>预测区间</span><b>{formatPrice(entry.dot.predictLow)} ~ {formatPrice(entry.dot.predictHigh)}</b></div>
                    ) : null}
                    {entry.dot.invalidation !== undefined ? (
                      <div><span>失效价位</span><b style={{ color: "#F6465D" }}>{formatPrice(entry.dot.invalidation)}</b></div>
                    ) : null}
                    {entry.dot.createdTime ? <div><span>执行时间</span><b>{entry.dot.createdTime}</b></div> : null}
                    {entry.dot.future ? (
                      <div><span>状态</span><b>未到预测时间 · 待结算</b></div>
                    ) : (
                      <>
                        <div><span>差异率</span><b className={(entry.dot.diffRate ?? 0) >= 0 ? styles.positive : styles.negative}>{pct(entry.dot.diffRate ?? 0)}</b></div>
                        <div><span>信号</span><b>{entry.dot.signal || (entry.dot.matched ? "走势贴合" : "需要复核")}</b></div>
                      </>
                    )}
                  </div>
                  {reason ? (
                    <>
                      <span className={styles.pinnedReasonLabel}>AI 理由</span>
                      <p className={isExpanded ? styles.pinnedReasonFull : styles.pinnedReasonClamp}>{reason}</p>
                      {reason.length > 80 ? (
                        <button type="button" className={styles.pinnedReasonToggle} onClick={() => toggleExpanded(entry.interval)}>
                          {isExpanded ? "收起" : "查看更多"}
                        </button>
                      ) : null}
                    </>
                  ) : (
                    <p className={styles.pinnedReasonClamp}>该预测点暂无 AI 文字理由。</p>
                  )}
                </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

export default function TradeSimulationAnalysisPage() {
  const [platformCode, setPlatformCode] = useState("binance");
  const [coinCode, setCoinCode] = useState("BTC");
  const [interval, setInterval] = useState("1m");
  const [data, setData] = useState<TradeSimulationAnalysisRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [hidden, setHidden] = useState<Set<string>>(new Set());
  const [news, setNews] = useState<NewsSentimentRecord[]>([]);
  const [newsLoading, setNewsLoading] = useState(false);
  const [newsExpanded, setNewsExpanded] = useState(false); // 默认收起，仅显示最新一条
  const [pressure, setPressure] = useState<PressureAnalysisRecord[]>([]);
  const [pressureLoading, setPressureLoading] = useState(false);
  const [pressureExpanded, setPressureExpanded] = useState(false); // 默认收起，仅显示最新一条

  const load = async () => {
    setLoading(true);
    try {
      const result = await fetchTradeSimulationAnalysis({
        platformCode,
        coinCode,
        interval,
        limit: 96,
      });
      setData(result);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  // 拉取所选币种最新 3 条消息面，后端已按拉取时间倒序返回。
  // silent=true 用于定时静默刷新，不触发 loading 态闪烁、出错也不弹提示。
  const loadNews = async (silent = false) => {
    if (!silent) {
      setNewsLoading(true);
    }
    try {
      const result = await fetchLatestNewsSentiments(coinCode, 3);
      setNews(result.data ?? []);
    } catch (err) {
      if (!silent) {
        message.error(err instanceof Error ? err.message : "消息面加载失败");
      }
    } finally {
      if (!silent) {
        setNewsLoading(false);
      }
    }
  };

  // 拉取所选币种最新 3 条压力面分析，后端已按分析时间倒序返回。
  const loadPressure = async (silent = false) => {
    if (!silent) {
      setPressureLoading(true);
    }
    try {
      const result = await fetchLatestPressureAnalyses(coinCode, 3);
      setPressure(result.data ?? []);
    } catch (err) {
      if (!silent) {
        message.error(err instanceof Error ? err.message : "压力面加载失败");
      }
    } finally {
      if (!silent) {
        setPressureLoading(false);
      }
    }
  };

  useEffect(() => {
    void load();
    void loadNews();
    void loadPressure();
  }, []);

  // 消息面/压力面快讯每 10s 静默刷新一次；切换币种时重建定时器。
  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadNews(true);
      void loadPressure(true);
    }, 10000);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [coinCode]);

  const toggleSeries = (seriesInterval: string) => {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(seriesInterval)) {
        next.delete(seriesInterval);
      } else {
        next.add(seriesInterval);
      }
      return next;
    });
  };

  const handlePlatformChange = (value: string) => {
    setPlatformCode(value);
  };

  // 复核表的预测周期筛选：all = 全部周期合并。
  const [horizonFilter, setHorizonFilter] = useState<string>("all");

  // 复核表合并各预测周期的已到期点位，加「预测周期」列区分来源；按预测时间倒序，最新在前。
  const markerRows: MarkerRow[] = useMemo(() => {
    return (data?.series ?? [])
      .filter((s) => horizonFilter === "all" || s.interval === horizonFilter)
      .flatMap((s) => s.markers.map((m) => ({ ...m, seriesLabel: s.label, seriesInterval: s.interval })))
      .sort((a, b) => b.timestamp - a.timestamp);
  }, [data, horizonFilter]);

  // 周期筛选选项：全部 + oracle 实际产出的各预测周期。
  const horizonOptions = useMemo(
    () => [
      { label: "全部", value: "all" },
      ...(data?.series ?? []).map((s) => ({ label: s.label, value: s.interval })),
    ],
    [data],
  );

  const markerColumns: ColumnsType<MarkerRow> = [
    {
      title: "预测周期",
      dataIndex: "seriesLabel",
      width: 96,
      render: (value: string) => <Tag color="blue">{value}</Tag>,
    },
    {
      title: "开盘 / 收盘时间",
      key: "klineTime",
      width: 140,
      render: (_: unknown, record: MarkerRow) => (
        <div>
          <div style={{ color: "#8c8c8c", fontSize: 12 }}>开 {record.createdTime || "-"}</div>
          <div style={{ color: "#8c8c8c", fontSize: 12 }}>收 {record.time || "-"}</div>
        </div>
      ),
    },
    {
      title: (
        <Space size={4}>
          实盘
          <Tooltip title="真实方向=预测时刻真实收盘相对开始价的涨跌；开始=AI 执行预测那一刻的真实盘价格(基准价)；区间=窗口[执行,预测]内真实价格波动[最低 ~ 最高]；收盘=预测时刻的真实收盘价。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "realBand",
      width: 190,
      render: (_: unknown, record: MarkerRow) => {
        const ref = Number(record.refPrice);
        const low = Number(record.windowLow);
        const high = Number(record.windowHigh);
        const close = Number(record.realPrice);
        if (!ref && !low && !high && !close) return "-";
        let dirTag = <Tag>平</Tag>;
        if (ref && close) {
          if (close > ref) dirTag = <Tag color="green">涨</Tag>;
          else if (close < ref) dirTag = <Tag color="red">跌</Tag>;
        }
        return (
          <div>
            <div style={{ marginBottom: 2 }}>{dirTag}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>开始 {ref ? formatPrice(ref) : "-"}</div>
            <div>{low || high ? `${formatPrice(low)} ~ ${formatPrice(high)}` : "-"}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>收盘 {close ? formatPrice(close) : "-"}</div>
          </div>
        );
      },
    },
    {
      title: (
        <Space size={4}>
          预测
          <Tooltip title="预测方向=AI 判断的涨跌方向；区间=AI 预测的本期价格波动[最低 ~ 最高]；预测收盘=AI 预测收盘价(区间中枢)。与「实盘」对照衡量预测质量。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "predictBand",
      width: 220,
      render: (_: unknown, record: MarkerRow) => {
        const low = Number(record.predictLow);
        const high = Number(record.predictHigh);
        const price = Number(record.aiPrice);
        const key = (record.trend || "").toLowerCase();
        const dirMap: Record<string, { text: string; color: string }> = {
          long: { text: "看涨", color: "green" },
          short: { text: "看跌", color: "red" },
          neutral: { text: "中性", color: "default" },
        };
        const dir = dirMap[key];
        if (!low && !high && !price && !dir) return "-";
        return (
          <div>
            <div style={{ marginBottom: 2 }}>{dir ? <Tag color={dir.color}>{dir.text}</Tag> : <Tag>{record.trend || "-"}</Tag>}</div>
            <div>{low || high ? `${formatPrice(low)} ~ ${formatPrice(high)}` : "-"}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>预测收盘 {price ? formatPrice(price) : "-"}</div>
          </div>
        );
      },
    },
    {
      title: (
        <Space size={4}>
          方向命中
          <Tooltip title="预测方向与真实涨跌方向是否一致：看涨且真实收盘高于开始价、或看跌且低于开始价即命中。中性不计方向命中。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "directionHit",
      width: 96,
      filters: [
        { text: "命中", value: "hit" },
        { text: "未命中", value: "miss" },
        { text: "中性", value: "neutral" },
      ],
      onFilter: (value, record) => {
        const t = (record.trend || "").toLowerCase();
        if (t !== "long" && t !== "short") return value === "neutral";
        const moved = Number(record.realPrice) - Number(record.refPrice);
        const hit = (t === "long" && moved > 0) || (t === "short" && moved < 0);
        return value === (hit ? "hit" : "miss");
      },
      render: (_: unknown, record: MarkerRow) => {
        const t = (record.trend || "").toLowerCase();
        if (t !== "long" && t !== "short") return <Tag>中性</Tag>;
        const ref = Number(record.refPrice);
        const real = Number(record.realPrice);
        if (!ref || !real) return "-";
        const moved = real - ref;
        const hit = (t === "long" && moved > 0) || (t === "short" && moved < 0);
        return <Tag color={hit ? "green" : "red"}>{hit ? "命中" : "未命中"}</Tag>;
      },
    },
    {
      title: "区间触达",
      dataIndex: "touched",
      width: 96,
      filters: [
        { text: "触达", value: true },
        { text: "未触达", value: false },
      ],
      onFilter: (value, record) => record.touched === value,
      render: (touched: boolean) => <Tag color={touched ? "gold" : "default"}>{touched ? "触达" : "未触达"}</Tag>,
    },
    {
      title: (
        <Space size={4}>
          区间命中
          <Tooltip title={`命中=预测区间完整覆盖真实波动，且区间利用率(真实宽/预测宽)≥${BAND_UTIL_THRESHOLD * 100}%；过宽=虽覆盖但区间报得太松(利用率不足)，覆盖名不副实；未命中=没能包住真实走势。`}>
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "bandContain",
      width: 120,
      filters: [
        { text: "命中", value: "hit" },
        { text: "过宽", value: "wide" },
        { text: "未命中", value: "miss" },
      ],
      onFilter: (value, record) => bandHitState(record) === value,
      render: (_: unknown, record: MarkerRow) => {
        const state = bandHitState(record);
        if (state === "none") return "-";
        const util = `${Math.round(record.bandUtil * 100)}%`;
        if (state === "hit") return <Tag color="green">命中 · 利用 {util}</Tag>;
        if (state === "wide") return <Tag color="orange">过宽 · 利用 {util}</Tag>;
        return <Tag color="red">未命中</Tag>;
      },
    },
    {
      title: (
        <Space size={4}>
          失效位
          <Tooltip title="方向被证伪的关键价位：看涨指下方支撑跌破位、看跌指上方阻力突破位。窗口内真实价触及该位即视为方向判断失效。绿=未触发(方向成立) 红=已触发(方向被证伪)。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "invalidation",
      width: 150,
      filters: [
        { text: "已触发", value: 1 },
        { text: "未触发", value: 0 },
        { text: "未给", value: -1 },
      ],
      onFilter: (value, record) => record.invalidationHit === value,
      render: (_: unknown, record: MarkerRow) => {
        if (!record.invalidation) return "-";
        const price = formatPrice(Number(record.invalidation));
        if (record.invalidationHit === 1) return <Tag color="red">{price} · 已失效</Tag>;
        if (record.invalidationHit === 0) return <Tag color="green">{price} · 成立</Tag>;
        return <span>{price}</span>;
      },
    },
    {
      title: (
        <Space size={4}>
          开仓最大利润
          <Tooltip title="按预测方向、在执行真实价开仓，价格走到区间极值时的最大浮盈。看涨=区间最高-执行真实价；看跌=执行真实价-区间最低。利润率 = 利润 / 执行真实价。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "maxProfit",
      width: 168,
      render: (_: unknown, record: MarkerRow) => {
        const ref = Number(record.refPrice);
        const key = (record.trend || "").toLowerCase();
        let profit: number | null = null;
        if (ref) {
          if (key === "long") profit = Number(record.windowHigh) - ref;
          else if (key === "short") profit = ref - Number(record.windowLow);
        }
        if (profit === null || !Number.isFinite(profit)) return "-";
        const rate = (profit / ref) * 100;
        const color = profit >= 0 ? "#0ECB81" : "#F6465D";
        return (
          <span style={{ color, fontWeight: 700 }}>
            {`${profit >= 0 ? "+" : ""}${formatPrice(profit)} / ${pct(rate)}`}
          </span>
        );
      },
    },
    {
      title: (
        <Space size={4}>
          预测利润 / 利润率
          <Tooltip title="按预测方向、在执行真实价开仓，价格走到 AI 预测价(预测时价格)时的利润。看涨=预测价-执行真实价；看跌=执行真实价-预测价。利润率 = 利润 / 执行真实价。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "predictProfit",
      width: 168,
      render: (_: unknown, record: MarkerRow) => {
        const ref = Number(record.refPrice);
        const ai = Number(record.aiPrice);
        const key = (record.trend || "").toLowerCase();
        let profit: number | null = null;
        if (ref && ai) {
          if (key === "long") profit = ai - ref;
          else if (key === "short") profit = ref - ai;
        }
        if (profit === null || !Number.isFinite(profit)) return "-";
        const rate = (profit / ref) * 100;
        const color = profit >= 0 ? "#0ECB81" : "#F6465D";
        return (
          <span style={{ color, fontWeight: 700 }}>
            {`${profit >= 0 ? "+" : ""}${formatPrice(profit)} / ${pct(rate)}`}
          </span>
        );
      },
    },
    {
      title: "价差",
      dataIndex: "diff",
      width: 120,
      render: (value: number) => {
        const numberValue = Number(value);
        const color = numberValue >= 0 ? "#0ECB81" : "#F6465D";
        return <span style={{ color, fontWeight: 700 }}>{numberValue >= 0 ? "+" : ""}{formatPrice(numberValue)}</span>;
      },
    },
    {
      title: "AI 理由",
      dataIndex: "reason",
      ellipsis: true,
      render: (value: string) => (value ? <span title={value}>{value}</span> : "-"),
    },
  ];

  const platformOptions = data?.platformOptions?.length ? data.platformOptions : DEFAULT_PLATFORMS;
  const coinOptions = data?.coinOptions?.length ? data.coinOptions : DEFAULT_COINS;
  const matchScore = getMatchScore(data);
  const totalMarkers = (data?.matchCount ?? 0) + (data?.diffCount ?? 0);

  // 区间命中率：在「给出了预测区间」的已到期点位中，完整覆盖真实波动「且利用率达标(非过宽)」的占比。
  const bandStats = useMemo(() => {
    const all = (data?.series ?? []).flatMap((s) => s.markers);
    const withBand = all.filter((m) => m.predictLow > 0 || m.predictHigh > 0);
    const hit = withBand.filter((m) => bandHitState(m) === "hit").length;
    const rate = withBand.length ? Math.round((hit / withBand.length) * 100) : 0;
    return { rate, hit, total: withBand.length };
  }, [data]);

  // 方向命中率：仅统计有明确方向(long/short)的已到期点位，真实收盘相对参考价的涨跌方向是否与预测一致。
  const directionStats = useMemo(() => {
    const all = (data?.series ?? []).flatMap((s) => s.markers);
    const directional = all.filter((m) => {
      const t = (m.trend || "").toLowerCase();
      return t === "long" || t === "short";
    });
    const hit = directional.filter((m) => {
      const t = (m.trend || "").toLowerCase();
      const moved = Number(m.realPrice) - Number(m.refPrice);
      return (t === "long" && moved > 0) || (t === "short" && moved < 0);
    }).length;
    const rate = directional.length ? Math.round((hit / directional.length) * 100) : 0;
    return { rate, hit, total: directional.length };
  }, [data]);

  // 方向守住率：在「给出了失效位」的已到期点位中，窗口内未触及失效位(方向未被证伪)的占比。越高越好。
  const invalidationStats = useMemo(() => {
    const all = (data?.series ?? []).flatMap((s) => s.markers);
    const given = all.filter((m) => m.invalidationHit === 0 || m.invalidationHit === 1);
    const held = given.filter((m) => m.invalidationHit === 0).length;
    const rate = given.length ? Math.round((held / given.length) * 100) : 0;
    return { rate, held, total: given.length };
  }, [data]);

  return (
    <div className="manager-page-stack">
      <div className={styles.flashGrid}>
        <section className="manager-data-card">
          <div className={styles.newsHeader}>
            <div>
              <Text className="manager-section-label">MARKET NEWS</Text>
              <h3>消息面快讯</h3>
            </div>
            <Space size={8}>
              <Text type="secondary" style={{ fontSize: 12 }}>{coinCode} · 最新 {news.length || 3} 条</Text>
              {news.length > 1 ? (
                <Button
                  size="small"
                  type="text"
                  icon={newsExpanded ? <UpOutlined /> : <DownOutlined />}
                  onClick={() => setNewsExpanded((prev) => !prev)}
                >
                  {newsExpanded ? "收起" : "展开"}
                </Button>
              ) : null}
              <Button size="small" icon={<ReloadOutlined />} loading={newsLoading} onClick={() => void loadNews()}>
                刷新
              </Button>
            </Space>
          </div>
          {news.length === 0 ? (
            <div className={styles.newsEmpty}>{newsLoading ? "正在加载消息面" : "暂无消息面数据"}</div>
          ) : (
            <div className={styles.newsList}>
              {(newsExpanded ? news : news.slice(0, 1)).map((item) => {
                const meta = sentimentMeta(item.sentiment);
                return (
                  <div key={item.id} className={styles.newsItem}>
                    <div className={styles.newsItemMain}>
                      <div className={styles.newsItemTop}>
                        <Tag color={meta.color}>{meta.text}</Tag>
                        {Number.isFinite(item.score) && item.score !== 0 ? (
                          <Tag color={item.score >= 0 ? "green" : "red"}>评分 {item.score.toFixed(2)}</Tag>
                        ) : null}
                        <span className={styles.newsTime}>{formatNewsTime(item.fetchedTime)}</span>
                      </div>
                      <p className={styles.newsSummary}>{item.summary || "（无综述）"}</p>
                      {item.keyEvents?.length ? (
                        <div className={styles.newsEvents}>
                          {item.keyEvents.slice(0, 4).map((event, index) => (
                            <Tag key={index} color="default">{event}</Tag>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>

        <section className="manager-data-card">
          <div className={styles.newsHeader}>
            <div>
              <Text className="manager-section-label">PRESSURE MAP</Text>
              <h3>压力面快讯</h3>
            </div>
            <Space size={8}>
              <Text type="secondary" style={{ fontSize: 12 }}>{coinCode} · 最新 {pressure.length || 3} 条</Text>
              {pressure.length > 1 ? (
                <Button
                  size="small"
                  type="text"
                  icon={pressureExpanded ? <UpOutlined /> : <DownOutlined />}
                  onClick={() => setPressureExpanded((prev) => !prev)}
                >
                  {pressureExpanded ? "收起" : "展开"}
                </Button>
              ) : null}
              <Button size="small" icon={<ReloadOutlined />} loading={pressureLoading} onClick={() => void loadPressure()}>
                刷新
              </Button>
            </Space>
          </div>
          {pressure.length === 0 ? (
            <div className={styles.newsEmpty}>{pressureLoading ? "正在加载压力面" : "暂无压力面数据"}</div>
          ) : (
            <div className={styles.newsList}>
              {(pressureExpanded ? pressure : pressure.slice(0, 1)).map((item) => {
                const meta = biasMeta(item.bias);
                return (
                  <div key={item.id} className={styles.newsItem}>
                    <div className={styles.newsItemMain}>
                      <div className={styles.newsItemTop}>
                        <Tag color={meta.color}>{meta.text}</Tag>
                        {item.refPrice ? (
                          <span className={styles.newsTime}>现价 {formatPrice(item.refPrice)}</span>
                        ) : null}
                        <span className={styles.newsTime}>{formatNewsTime(item.analyzedTime)}</span>
                      </div>
                      <div className={styles.pressureKeys}>
                        <Tag color="red">
                          关键阻力 {item.keyResistance ? formatPrice(item.keyResistance) : "—"}
                        </Tag>
                        <Tag color="green">
                          关键支撑 {item.keySupport ? formatPrice(item.keySupport) : "—"}
                        </Tag>
                      </div>
                      {item.summary ? <p className={styles.newsSummary}>{item.summary}</p> : null}
                      <div className={styles.pressureCols}>
                        <PressureLevelGroup title="做空压力位 · 阻力" tone="short" levels={item.shortPressureLevels} />
                        <PressureLevelGroup title="做多压力位 · 支撑" tone="long" levels={item.longPressureLevels} />
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>
      </div>

      <section className={styles.heroPanel}>
        <Space wrap size={12} className={styles.toolbar}>
          <Select value={platformCode} options={platformOptions} onChange={handlePlatformChange} className={styles.select} />
          <Select value={coinCode} options={coinOptions} onChange={setCoinCode} className={styles.select} />
          <label className={styles.fieldGroup}>
            <span>K线周期</span>
            <Select value={interval} options={INTERVAL_OPTIONS} onChange={setInterval} className={styles.intervalSelect} />
          </label>
          <Button
            type="primary"
            icon={<SearchOutlined />}
            loading={loading}
            onClick={() => {
              void load();
              void loadNews();
              void loadPressure();
            }}
          >
            分析
          </Button>
          <Button
            icon={<ReloadOutlined />}
            loading={loading}
            onClick={() => {
              void load();
              void loadNews();
              void loadPressure();
            }}
          >
            刷新
          </Button>
        </Space>
      </section>

      <section className={styles.metricGrid}>
        {[
          { label: "交易对", value: data?.symbol ?? "-", meta: data?.lastRunTime ? `最近运行 ${data.lastRunTime}` : "等待数据", icon: <BarChartOutlined /> },
          // 三件套：方向 / 区间 / 失效位——重构后真正衡量预测价值的核心指标。
          { label: "方向命中率", value: `${directionStats.rate}%`, meta: `${directionStats.hit}/${directionStats.total} 有向预测方向正确`, icon: <AimOutlined /> },
          { label: "区间命中率", value: `${bandStats.rate}%`, meta: `${bandStats.hit}/${bandStats.total} 预测区间覆盖真实波动`, icon: <AimOutlined /> },
          { label: "方向守住率", value: `${invalidationStats.rate}%`, meta: `${invalidationStats.held}/${invalidationStats.total} 未触发失效位`, icon: <AimOutlined /> },
          { label: "区间触达率", value: `${totalMarkers ? Math.round(((data?.touchCount ?? 0) / totalMarkers) * 100) : 0}%`, meta: `${data?.touchCount ?? 0}/${totalMarkers || 0} 点位价格曾触达(全周期)`, icon: <AimOutlined /> },
          // 点位精度仅作诊断，不再作为头部 KPI。
          { label: "点位拟合度", value: `${matchScore}%`, meta: `诊断用 · 均差 ${(data?.avgDiffRate ?? 0).toFixed(2)}%`, icon: <BarChartOutlined /> },
        ].map((item) => (
          <div key={item.label} className={styles.metricCard}>
            <div className={styles.metricIcon}>{item.icon}</div>
            <Text>{item.label}</Text>
            <strong>{item.value}</strong>
            <span>{item.meta}</span>
          </div>
        ))}
      </section>

      <section className="manager-data-card">
        <SimulationCompareChart data={data} loading={loading} hidden={hidden} onToggle={toggleSeries} />
      </section>

      <section className="manager-data-card manager-table">
        <div className={styles.tableTitle}>
          <div>
            <Text className="manager-section-label">MARKER REVIEW</Text>
            <h3>点位复核</h3>
          </div>
          <Space size={12} wrap>
            <Segmented
              size="small"
              value={horizonFilter}
              onChange={(value) => setHorizonFilter(String(value))}
              options={horizonOptions}
            />
            <Tag color={directionStats.rate >= 50 ? "green" : "red"}>
              方向命中 {directionStats.rate}% ({directionStats.hit}/{directionStats.total})
            </Tag>
          </Space>
        </div>
        <Table
          rowKey={(record) => `${record.seriesInterval}-${record.timestamp}-${record.label}`}
          loading={loading}
          dataSource={markerRows}
          columns={markerColumns}
          pagination={{ defaultPageSize: 10, showSizeChanger: true, pageSizeOptions: ["10", "20", "50"] }}
          scroll={{ x: "max-content" }}
          size="small"
        />
      </section>
    </div>
  );
}
