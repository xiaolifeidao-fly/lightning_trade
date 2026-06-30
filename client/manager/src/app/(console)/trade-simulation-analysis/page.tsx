"use client";

import { useEffect, useMemo, useState, type PointerEvent, type ReactNode } from "react";
import { Button, InputNumber, Segmented, Select, Space, Switch, Table, Tag, Tooltip, Typography, message } from "antd";
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
  type TradeSimulationKlinePoint,
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

// U本位合约标准费率：Maker 0.02% / Taker 0.05%。
// 模拟交易手续费口径：开仓按市价吃单(Taker)；平仓走止盈限价记 Maker、按收盘市价记 Taker。
const MAKER_FEE_RATE = 0.0002;
const TAKER_FEE_RATE = 0.0005;

// 合约面值：1 张 = 0.001 BTC。开仓数量(BTC) = 张数 × 此值。
const CONTRACT_SIZE_BTC = 0.001;
type BandHitState = "hit" | "wide" | "miss" | "none";
function bandHitState(record: { predictLow: number; predictHigh: number; bandContain: boolean; bandUtil: number }): BandHitState {
  if (!record.predictLow && !record.predictHigh) return "none";
  if (!record.bandContain) return "miss";
  return record.bandUtil >= BAND_UTIL_THRESHOLD ? "hit" : "wide";
}

// 预测方向：不看预测收盘价，而是按预测区间相对开盘价(基准价)的两侧空间来判定——
// 比较 |开盘价-区间最高| 与 |开盘价-区间最低|，哪侧绝对值更大就往哪侧做(高侧大=看涨, 低侧大=看跌)。
// 无开盘价或无区间数据、或两侧相等时记为中性。
function predictTrend(record: { refPrice: number; predictHigh: number; predictLow: number }): "long" | "short" | "neutral" {
  const ref = Number(record.refPrice);
  const high = Number(record.predictHigh);
  const low = Number(record.predictLow);
  if (!ref || (!high && !low)) return "neutral";
  const up = high > 0 ? Math.abs(high - ref) : 0;
  const down = low > 0 ? Math.abs(ref - low) : 0;
  if (up === down) return "neutral";
  return up > down ? "long" : "short";
}

// 单条点位的开仓/平仓盈亏测算结果。列渲染与汇总统计共用，保证口径一致。
type OrderProfit = {
  ref: number; // 开仓基准价：实际开盘价(open_price)，缺失时回退 ref_price
  key: "long" | "short" | "neutral"; // 预测方向(按区间相对开盘价推导)
  opened: boolean; // 是否达到开仓阈值
  actualProfit: number | null; // 实际最大利润(窗口极值)
  predictProfit: number | null; // 预测利润(预测区间极值)
  close: { profit: number; price: number; via: "止盈" | "收盘" } | null; // 平仓结算
  fee: number | null; // 开平仓往返手续费(每单位)，U本位标准费率；未开仓为 null
};

// settle 为可选的结算窗口口径：缺省时按「预测周期」窗口(record.windowHigh/Low + realPrice)结算；
// 传入时按「交易周期」窗口结算(整周期真实最高/最低价 + 周期收盘价/最新价)。开仓判定与方向始终按 AI 预测推导，不随口径变化。
function computeOrderProfit(
  record: { refPrice: number; openPrice: number; predictHigh: number; predictLow: number; windowHigh: number; windowLow: number; realPrice: number },
  profitThreshold: number,
  takeProfitPct: number,
  settle?: { high: number; low: number; close: number },
): OrderProfit {
  // 开仓基准价：用 AI 检测完成后即时采集的实际开盘价(open_price)；旧数据无该值时回退到参考价(ref_price)。
  // 方向仍按 AI 当时看盘的参考价(ref_price)推导，不受开仓价口径影响。
  const ref = Number(record.openPrice) || Number(record.refPrice);
  const key = predictTrend(record);
  const calc = (target: number): number | null => {
    if (!ref || !Number.isFinite(target)) return null;
    if (key === "long") return target - ref;
    if (key === "short") return ref - target;
    return null;
  };
  // 结算窗口的真实极值与收盘价：默认取预测周期窗口，settle 传入时改用交易周期窗口。
  const winHigh = settle ? settle.high : Number(record.windowHigh);
  const winLow = settle ? settle.low : Number(record.windowLow);
  const settleClose = settle ? settle.close : Number(record.realPrice);
  const actualProfit = key === "long" ? calc(winHigh) : key === "short" ? calc(winLow) : null;
  const predictProfit = key === "long" ? calc(Number(record.predictHigh)) : key === "short" ? calc(Number(record.predictLow)) : null;
  // 开仓判定：仅当预测利润率超过阈值才会开仓。
  const predictRate = predictProfit !== null && ref ? (predictProfit / ref) * 100 : null;
  const opened = predictRate !== null && predictRate > profitThreshold;
  if (!opened) {
    return { ref, key, opened: false, actualProfit, predictProfit, close: null, fee: null };
  }
  // 平仓结算：达到「预测最大利润 × 平仓比例」即止盈(走预测价位)；窗口内未触及则按真实收盘价平仓(走收盘价位)。
  const close = ((): OrderProfit["close"] => {
    if (predictProfit === null || predictProfit <= 0) return null;
    const tpProfit = predictProfit * (takeProfitPct / 100);
    const tpPrice = key === "long" ? ref + tpProfit : ref - tpProfit;
    const reached = key === "long" ? winHigh >= tpPrice : winLow <= tpPrice;
    if (reached) return { profit: tpProfit, price: tpPrice, via: "止盈" };
    const real = settleClose;
    const profit = real ? calc(real) : null;
    return profit !== null ? { profit, price: real, via: "收盘" } : null;
  })();
  // 往返手续费(每单位)：开仓市价吃单(Taker)，平仓止盈记 Maker / 收盘市价记 Taker。
  // 费 = 开仓名义×费率 + 平仓名义×费率，名义以各自成交价计。
  const fee = close
    ? ref * TAKER_FEE_RATE + close.price * (close.via === "止盈" ? MAKER_FEE_RATE : TAKER_FEE_RATE)
    : null;
  return { ref, key, opened: true, actualProfit, predictProfit, close, fee };
}

// 把 unix 秒格式化为「MM-DD HH:mm」(本地时区)，与后端 createdTime 口径一致。
function formatShortTime(unixSeconds: number) {
  if (!unixSeconds) return "-";
  const date = new Date(unixSeconds * 1000);
  if (Number.isNaN(date.getTime())) return "-";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

// 交易周期窗口：以开盘时间(openTimestamp)为起点、按用户设定的交易周期时长截取的真实行情窗口。
type TradeCycleInfo = {
  openTs: number; // 开盘时间(unix秒)
  closeTs: number; // 按交易周期算的收盘时间(unix秒)
  high: number; // 整个交易周期的真实最高价
  low: number; // 整个交易周期的真实最低价
  close: number | null; // 收盘价格(周期已到=收盘价；未到=null)
  complete: boolean; // 交易周期是否已走完
};

// buildTradeCycle 从展示用真实 K 线里截取 [开盘时间, 开盘时间+交易周期] 窗口，算出整周期真实最高/最低价与收盘价。
// 周期未到(closeTs 超过最后一根真实 K 线)时，以最新价并入极值、收盘价留空。
function buildTradeCycle(
  record: { openTimestamp: number; openPrice: number; refPrice: number },
  klines: TradeSimulationKlinePoint[],
  cycleMinutes: number,
  latestPrice: number | null,
  lastRealTs: number,
): TradeCycleInfo | null {
  const openTs = Number(record.openTimestamp);
  if (!openTs || !cycleMinutes) return null;
  const closeTs = openTs + Math.round(cycleMinutes * 60);
  const complete = lastRealTs > 0 && lastRealTs >= closeTs;

  const highs: number[] = [];
  const lows: number[] = [];
  let closeBar: TradeSimulationKlinePoint | null = null;
  let closeBarDist = Infinity;
  klines.forEach((k) => {
    const ts = Number(k.timestamp);
    if (ts >= openTs && ts <= closeTs) {
      if (Number.isFinite(k.highPrice) && k.highPrice > 0) highs.push(k.highPrice);
      if (Number.isFinite(k.lowPrice) && k.lowPrice > 0) lows.push(k.lowPrice);
    }
    // 收盘价取最接近收盘时间的那根 K 线收盘。
    if (ts <= closeTs) {
      const dist = closeTs - ts;
      if (dist < closeBarDist) {
        closeBarDist = dist;
        closeBar = k;
      }
    }
  });
  // 以实际开盘价兜底，保证极值至少有一个有效值。
  const openP = Number(record.openPrice) || Number(record.refPrice);
  if (openP > 0) {
    highs.push(openP);
    lows.push(openP);
  }
  // 周期未到：把最新价并入极值，反映当前已走到的波动范围。
  if (!complete && latestPrice && latestPrice > 0) {
    highs.push(latestPrice);
    lows.push(latestPrice);
  }
  const high = highs.length ? Math.max(...highs) : 0;
  const low = lows.length ? Math.min(...lows) : 0;
  const close = complete && closeBar ? Number((closeBar as TradeSimulationKlinePoint).closePrice) : null;
  return { openTs, closeTs, high, low, close, complete };
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

// formatCost 把 AI 分析耗时(毫秒)格式化为可读时间差：≥1s 显示秒，否则显示毫秒。
function formatCost(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) {
    return "-";
  }
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`;
  }
  return `${Math.round(ms)}ms`;
}

// renderStartLine 渲染「开始」行：预测看盘价(ref) · 实际开盘价(open) · 时间差(AI耗时)，同一行展示。
// 看盘价是发起预测时 AI 参考的盘价，实际开盘价是 AI 检测完成后即时采集的真实盘价，时间差即两者之间的 AI 耗时。
function renderStartLine(record: MarkerRow) {
  const ref = Number(record.refPrice);
  const open = Number(record.openPrice);
  const cost = Number(record.costMs);
  return (
    <div style={{ color: "#8c8c8c", fontSize: 12, whiteSpace: "nowrap" }}>
      开始 {ref ? formatPrice(ref) : "-"}
      {open ? <> · 实际 {formatPrice(open)}</> : null}
      {cost > 0 ? <> · {formatCost(cost)}</> : null}
    </div>
  );
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
  const [interval, setInterval] = useState("15m");
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

  // 复核表的预测周期筛选：默认 15 分钟，不提供「全部」合并选项。
  const [horizonFilter, setHorizonFilter] = useState<string>("15m");

  // 开仓阈值(%)：仅当 AI 预测利润率超过该值才视为「会开仓」，未开仓的点位不展示利润数据。默认 0.5%。
  const [profitThreshold, setProfitThreshold] = useState<number>(0.5);

  // 平仓比例(%)：开仓后达到「预测最大利润」的该百分比即止盈平仓。默认 80%。
  // 若窗口内真实价未触及该止盈价，视为未平仓，则按实际收盘价结算开仓利润。
  const [takeProfitPct, setTakeProfitPct] = useState<number>(80);

  // 利润结算口径：predict=按预测周期窗口；trade=按交易周期窗口。默认按预测周期。
  const [profitCycleMode, setProfitCycleMode] = useState<"predict" | "trade">("predict");

  // 交易周期时长(小时)：从开盘时间起算的结算窗口长度，可输入更改，默认 1 小时。
  const [tradeCycleHours, setTradeCycleHours] = useState<number>(1);
  const tradeCycleMinutes = (Number(tradeCycleHours) || 0) * 60;

  // 杠杆倍数：影响保证金占用，进而放大利润率(利润率=价格变动幅度×杠杆)。默认 100x。
  const [leverage, setLeverage] = useState<number>(100);

  // 单次开仓张数：1 张 = 0.001 BTC。开仓数量(BTC) = 张数 × CONTRACT_SIZE_BTC，用于把每单位价差换算成实际盈亏与手续费。默认 1 张。
  const [contracts, setContracts] = useState<number>(1);

  // 复核表合并各预测周期的已到期点位，加「预测周期」列区分来源；按预测时间倒序，最新在前。
  const markerRows: MarkerRow[] = useMemo(() => {
    return (data?.series ?? [])
      .filter((s) => s.interval === horizonFilter)
      .flatMap((s) => s.markers.map((m) => ({ ...m, seriesLabel: s.label, seriesInterval: s.interval })))
      .sort((a, b) => b.timestamp - a.timestamp);
  }, [data, horizonFilter]);

  // 只看可开仓：开启后仅展示「预测利润率超过开仓阈值」的点位(computeOrderProfit.opened)。
  const [onlyOpenable, setOnlyOpenable] = useState<boolean>(true);
  const displayRows: MarkerRow[] = useMemo(() => {
    if (!onlyOpenable) return markerRows;
    return markerRows.filter((row) => computeOrderProfit(row, profitThreshold, takeProfitPct).opened);
  }, [markerRows, onlyOpenable, profitThreshold, takeProfitPct]);

  // 最新价回落值：真实 K 线最后一根的收盘价(分析数据自带，加载后才有)。
  const fallbackPrice = useMemo(() => {
    const klines = data?.realKlines ?? [];
    if (!klines.length) return null;
    const last = klines[klines.length - 1];
    const price = Number(last.closePrice);
    return Number.isFinite(price) && price > 0 ? price : null;
  }, [data]);

  // 实时最新价：直接轮询 Binance 公共行情(无需鉴权)，每 5s 刷新一次；失败静默并回落到分析数据收盘价。
  const [livePrice, setLivePrice] = useState<number | null>(null);
  useEffect(() => {
    setLivePrice(null); // 切币种先清空，避免显示旧币种价格
    const symbol = `${coinCode}USDT`.toUpperCase();
    let stopped = false;
    const fetchPrice = async () => {
      try {
        const res = await fetch(`https://api.binance.com/api/v3/ticker/price?symbol=${symbol}`);
        if (!res.ok) return;
        const json = (await res.json()) as { price?: string };
        const price = Number(json.price);
        if (!stopped && Number.isFinite(price) && price > 0) {
          setLivePrice(price);
        }
      } catch {
        // 静默：拉取失败保持回落到分析数据收盘价
      }
    };
    void fetchPrice();
    const timer = window.setInterval(() => void fetchPrice(), 5000);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [coinCode]);

  const latestPrice = livePrice ?? fallbackPrice;

  // 真实 K 线与最后一根的时间戳，供「交易周期」窗口截取使用。
  const klines = useMemo(() => data?.realKlines ?? [], [data]);
  const lastRealTs = klines.length ? Number(klines[klines.length - 1].timestamp) : 0;

  // 某行的交易周期窗口(开盘→收盘的真实最高/最低/收盘价)。
  const tradeCycleFor = (record: MarkerRow): TradeCycleInfo | null =>
    buildTradeCycle(record, klines, tradeCycleMinutes, latestPrice, lastRealTs);

  // 利润结算口径：按交易周期时返回交易周期窗口极值与收盘(未到则用最新价)，按预测周期时返回 undefined(沿用预测窗口)。
  const settleFor = (record: MarkerRow): { high: number; low: number; close: number } | undefined => {
    if (profitCycleMode !== "trade") return undefined;
    const tc = tradeCycleFor(record);
    if (!tc) return undefined;
    const close = tc.complete ? tc.close ?? latestPrice ?? 0 : latestPrice ?? 0;
    return { high: tc.high, low: tc.low, close: close ?? 0 };
  };

  // 结算选择：按预测周期口径直接结算；按交易周期口径时——先看「到预测周期收盘」是否已盈利，
  // 盈利则锁定预测周期结算(提前止盈/收盘获利)，亏损则继续持有按交易周期窗口结算(博取回本)。
  const resolveOrderProfit = (record: MarkerRow): OrderProfit & { settleSource: "predict" | "trade" } => {
    const predictResult = computeOrderProfit(record, profitThreshold, takeProfitPct);
    if (profitCycleMode !== "trade") {
      return { ...predictResult, settleSource: "predict" };
    }
    const predictNet = predictResult.close ? predictResult.close.profit - (predictResult.fee ?? 0) : null;
    if (predictNet !== null && predictNet > 0) {
      return { ...predictResult, settleSource: "predict" };
    }
    const tradeResult = computeOrderProfit(record, profitThreshold, takeProfitPct, settleFor(record));
    return { ...tradeResult, settleSource: "trade" };
  };

  // 表格当前筛选后的数据(含列筛选)，用于汇总开仓利润。Segmented 切周期/开关会重建数据，此处同步回落。
  const [tableFiltered, setTableFiltered] = useState<MarkerRow[]>([]);
  useEffect(() => {
    setTableFiltered(displayRows);
  }, [displayRows]);

  // 汇总：当前筛选出来的已开仓点位的平仓净利(按张数换算成 USDT、扣往返手续费)总和与累计利润率(含杠杆)。
  const profitSummary = useMemo(() => {
    const qty = (Number(contracts) || 0) * CONTRACT_SIZE_BTC; // 持仓(BTC)，1张=0.001BTC(已含100x口径)
    let total = 0;
    let totalRate = 0;
    let count = 0;
    let win = 0;
    tableFiltered.forEach((row) => {
      const { ref, close, fee } = resolveOrderProfit(row);
      if (!close) return;
      const net = close.profit - (fee ?? 0); // 每单位净利(价差口径)
      total += net * qty; // 实际净利(USDT)
      if (ref) totalRate += (net / ref) * leverage * 100; // 含杠杆的净利率
      count += 1;
      if (net > 0) win += 1;
    });
    return { total, totalRate, count, win };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tableFiltered, profitThreshold, takeProfitPct, contracts, leverage, profitCycleMode, tradeCycleMinutes, latestPrice, klines, lastRealTs]);

  // 周期筛选选项：oracle 实际产出的各预测周期(不含「全部」合并选项)。
  const horizonOptions = useMemo(
    () => (data?.series ?? []).map((s) => ({ label: s.label, value: s.interval })),
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
        <div>
          <Space size={4}>
            交易周期
            <Tooltip title="按「交易周期时间」(默认1小时)从开盘时间起算的真实行情窗口：开盘时间、按交易周期算的收盘时间，以及整个交易周期内的真实最低/最高价与收盘价。周期未到时以最新价计入波动、收盘价留空。">
              <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
            </Tooltip>
          </Space>
          <div style={{ marginTop: 4 }}>
            <Tooltip title="交易周期时间(小时)：从开盘时间起算的窗口长度，可输入更改，默认 1 小时">
              <InputNumber
                size="small"
                min={0}
                step={0.5}
                value={tradeCycleHours}
                onChange={(value) => setTradeCycleHours(Number(value) || 0)}
                formatter={(value) => `${value}小时`}
                parser={(value) => (value ? value.replace(/[^\d.]/g, "") : "") as unknown as number}
                style={{ width: 88 }}
              />
            </Tooltip>
          </div>
        </div>
      ),
      key: "tradeCycle",
      width: 200,
      render: (_: unknown, record: MarkerRow) => {
        const tc = tradeCycleFor(record);
        if (!tc) return <span style={{ color: "#8c8c8c" }}>-</span>;
        return (
          <div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>开 {formatShortTime(tc.openTs)}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>
              收 {formatShortTime(tc.closeTs)}
              {!tc.complete ? <Tag color="gold" style={{ marginLeft: 4 }}>进行中</Tag> : null}
            </div>
            <div>{tc.low || tc.high ? `${formatPrice(tc.low)} ~ ${formatPrice(tc.high)}` : "-"}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>
              收盘 {tc.complete && tc.close ? formatPrice(tc.close) : "-"}
            </div>
          </div>
        );
      },
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
      width: 250,
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
            {renderStartLine(record)}
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
          <Tooltip title="AI=AI 直接输出的预测方向，置信=AI 标定的方向正确主观概率(0~100%)；计算=按预测区间相对开盘价两侧空间推导的方向(上沿空间大=看涨，下沿空间大=看跌)，开仓利润与方向命中均按此口径，可能与 AI 方向不一致；开始=AI 执行预测那一刻的真实盘价格(基准价)；区间=AI 预测的本期价格波动[最低 ~ 最高]；预测收盘=AI 预测收盘价(区间中枢)。与「实盘」对照衡量预测质量。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "predictBand",
      width: 280,
      render: (_: unknown, record: MarkerRow) => {
        const ref = Number(record.refPrice);
        const low = Number(record.predictLow);
        const high = Number(record.predictHigh);
        const price = Number(record.aiPrice);
        const dirMap: Record<string, { text: string; color: string }> = {
          long: { text: "看涨", color: "green" },
          short: { text: "看跌", color: "red" },
          neutral: { text: "中性", color: "default" },
        };
        // AI 方向：AI 直接输出的 trend；计算方向：按预测区间相对开盘价两侧空间推导。两者可能不一致。
        const aiDir = dirMap[(record.trend || "").toLowerCase()] ?? dirMap.neutral;
        const calcDir = dirMap[predictTrend(record)];
        const conf = Number(record.confidence);
        if (!ref && !low && !high && !price) return "-";
        return (
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 4, marginBottom: 2 }}>
              <span style={{ color: "#8c8c8c", fontSize: 12, width: 28, flexShrink: 0 }}>AI</span>
              <Tag color={aiDir.color}>{aiDir.text}</Tag>
              {conf > 0 ? <span style={{ color: "#8c8c8c", fontSize: 12 }}>置信 {Math.round(conf * 100)}%</span> : null}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 4, marginBottom: 2 }}>
              <span style={{ color: "#8c8c8c", fontSize: 12, width: 28, flexShrink: 0 }}>计算</span>
              <Tag color={calcDir.color}>{calcDir.text}</Tag>
            </div>
            {renderStartLine(record)}
            <div>{low || high ? `${formatPrice(low)} ~ ${formatPrice(high)}` : "-"}</div>
            <div style={{ color: "#8c8c8c", fontSize: 12 }}>预测收盘 {price ? formatPrice(price) : "-"}</div>
          </div>
        );
      },
    },
    {
      title: (
        <div>
          <Space size={4}>
            开仓利润 / 利润率
            <Tooltip title="按预测方向、在实际开盘价(open_price，AI检测完成后即时采集的真实盘价；旧数据缺失时回退参考价)开仓的盈亏。仅当预测利润率超过开仓阈值才视为会开仓，未开仓的点位不展示数据。实际最大=窗口内真实价走到有利极值时(看涨=区间最高-开盘价；看跌=开盘价-区间最低)；预测=价格走到预测区间极值时(看涨=预测最高-开盘价；看跌=开盘价-预测最低)；平仓=开仓后达到「预测最大利润×平仓比例」即止盈结算，窗口内未触及该止盈价则按实际收盘价结算。利润率 = 利润 / 实际开盘价。结算窗口可在下方切换「按预测周期 / 按交易周期」。">
              <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
            </Tooltip>
          </Space>
          <div style={{ marginTop: 4 }}>
            <Tooltip title="结算口径：按预测周期=用预测周期窗口的真实极值与收盘；按交易周期=用上方交易周期窗口的真实极值与收盘(未到则用最新价)。按交易周期时附加条件：若「到预测周期收盘」已盈利则锁定该笔提前获利(标预测周期)，亏损则继续持有按交易周期结算(标交易周期)。开仓判定与方向始终按 AI 预测，不随口径变化。">
              <Segmented
                size="small"
                value={profitCycleMode}
                onChange={(value) => setProfitCycleMode(value as "predict" | "trade")}
                options={[
                  { label: "按预测周期", value: "predict" },
                  { label: "按交易周期", value: "trade" },
                ]}
              />
            </Tooltip>
          </div>
          <Space size={4} style={{ marginTop: 4 }}>
            <Tooltip title="开仓阈值：预测利润率超过此值才会开仓">
              <InputNumber
                size="small"
                min={0}
                step={0.1}
                value={profitThreshold}
                onChange={(value) => setProfitThreshold(Number(value) || 0)}
                formatter={(value) => `≥${value}%`}
                parser={(value) => (value ? value.replace(/[^\d.]/g, "") : "") as unknown as number}
                style={{ width: 84 }}
              />
            </Tooltip>
            <Tooltip title="平仓比例：达到预测最大利润的此百分比即止盈平仓">
              <InputNumber
                size="small"
                min={0}
                max={100}
                step={5}
                value={takeProfitPct}
                onChange={(value) => setTakeProfitPct(Number(value) || 0)}
                formatter={(value) => `平${value}%`}
                parser={(value) => (value ? value.replace(/[^\d.]/g, "") : "") as unknown as number}
                style={{ width: 84 }}
              />
            </Tooltip>
            <Tooltip title="杠杆倍数：放大利润率(利润率=价格变动幅度×杠杆)，不改变名义盈亏金额">
              <InputNumber
                size="small"
                min={1}
                max={125}
                step={1}
                value={leverage}
                onChange={(value) => setLeverage(Number(value) || 1)}
                formatter={(value) => `${value}x`}
                parser={(value) => (value ? value.replace(/[^\d.]/g, "") : "") as unknown as number}
                style={{ width: 72 }}
              />
            </Tooltip>
          </Space>
        </div>
      ),
      key: "profit",
      width: 380,
      render: (_: unknown, record: MarkerRow) => {
        const { ref, opened, actualProfit, predictProfit, close, fee, settleSource } = resolveOrderProfit(record);
        // 持仓数量(BTC) = 张数 × 0.001(1张=0.001BTC，该面值本身已是100x口径，杠杆不再二次放大金额)。
        const qty = (Number(contracts) || 0) * CONTRACT_SIZE_BTC; // 持仓(BTC)
        // 入参 profit 为「每单位(1 BTC)价差」；按持仓换算成实际盈亏金额(USDT)；利润率=价差幅度×杠杆(保证金回报)。
        const renderProfit = (label: string, profit: number | null, suffix?: string) => {
          if (profit === null || !Number.isFinite(profit)) {
            return (
              <div style={{ color: "#8c8c8c", fontSize: 12 }}>{label} -</div>
            );
          }
          const amount = profit * qty;
          const rate = ref ? (profit / ref) * leverage * 100 : 0;
          const color = profit >= 0 ? "#0ECB81" : "#F6465D";
          return (
            <div style={{ fontSize: 12 }}>
              <span style={{ color: "#8c8c8c" }}>{label} </span>
              <span style={{ color, fontWeight: 700 }}>
                {`${amount >= 0 ? "+" : ""}${formatPrice(amount)} USDT / ${pct(rate)}`}
              </span>
              {suffix ? <span style={{ color: "#8c8c8c" }}> {suffix}</span> : null}
            </div>
          );
        };
        if (!opened) {
          return <span style={{ color: "#8c8c8c" }}>未开仓</span>;
        }
        const closeProfit = close?.profit ?? null;
        if (actualProfit === null && predictProfit === null) return "-";
        // 净利 = 平仓利润 - 往返手续费；盈亏标签按扣费后的净利判断。
        const netProfit =
          closeProfit !== null && Number.isFinite(closeProfit) ? closeProfit - (fee ?? 0) : null;
        const profitable = netProfit !== null ? netProfit > 0 : null;
        // 交易周期口径相对「纯预测周期」结算的净利差额：用以标记切换交易周期后多盈利/多亏损多少(USDT)。
        const predictBase = computeOrderProfit(record, profitThreshold, takeProfitPct);
        const predictBaseNet = predictBase.close ? predictBase.close.profit - (predictBase.fee ?? 0) : null;
        const deltaAmount =
          profitCycleMode === "trade" && netProfit !== null && predictBaseNet !== null
            ? (netProfit - predictBaseNet) * qty
            : null;
        return (
          <div>
            {renderProfit("实际最大", actualProfit)}
            {renderProfit("预测", predictProfit)}
            {renderProfit("平仓", closeProfit)}
            <div style={{ fontSize: 12 }}>
              <span style={{ color: "#8c8c8c" }}>开盘均价 </span>
              <span>{ref ? formatPrice(ref) : "-"}</span>
              {close ? (
                <span style={{ color: "#8c8c8c" }}> {close.via}({formatPrice(close.price)})</span>
              ) : null}
            </div>
            {fee !== null && Number.isFinite(fee) ? (
              <div style={{ fontSize: 12 }}>
                <span style={{ color: "#8c8c8c" }}>手续费 </span>
                <span style={{ color: "#F6465D" }}>-{formatPrice(fee * qty)} USDT</span>
                <span style={{ color: "#8c8c8c" }}> {close?.via === "止盈" ? "T+M" : "T+T"}</span>
              </div>
            ) : null}
            {renderProfit("净利", netProfit)}
            <Space size={4} style={{ marginTop: 2 }}>
              {profitable !== null ? (
                <Tag color={profitable ? "green" : "red"} style={{ margin: 0 }}>
                  {profitable ? "盈利" : "亏损"}
                </Tag>
              ) : null}
              {profitCycleMode === "trade" ? (
                <Tooltip title={settleSource === "predict" ? "到预测周期收盘已盈利，锁定该笔提前获利" : "到预测周期收盘亏损，继续持有按交易周期结算"}>
                  <Tag color={settleSource === "predict" ? "blue" : "purple"} style={{ margin: 0 }}>
                    {settleSource === "predict" ? "预测周期" : "交易周期"}
                  </Tag>
                </Tooltip>
              ) : null}
            </Space>
            {deltaAmount !== null ? (
              <div style={{ fontSize: 12, marginTop: 2 }}>
                <span style={{ color: "#8c8c8c" }}>较预测周期 </span>
                {Math.abs(deltaAmount) < 1e-9 ? (
                  <span style={{ color: "#8c8c8c" }}>持平</span>
                ) : (
                  <span style={{ color: deltaAmount > 0 ? "#0ECB81" : "#F6465D", fontWeight: 700 }}>
                    {deltaAmount > 0
                      ? `多盈利 +${formatPrice(deltaAmount)} USDT`
                      : `多亏损 -${formatPrice(Math.abs(deltaAmount))} USDT`}
                  </span>
                )}
              </div>
            ) : null}
          </div>
        );
      },
    },
    {
      title: (
        <Space size={4}>
          复核指标
          <Tooltip title="方向命中=预测方向与真实涨跌一致(看涨且真实收盘高于开始价、或看跌且低于即命中，中性不计)；区间触达=执行→预测窗口内真实价是否曾覆盖预测价；区间命中=预测区间完整覆盖真实波动且利用率(真实宽/预测宽)达标，过宽=覆盖但报得太松；失效位=方向被证伪的关键价位，窗口内触及即失效(绿=成立/红=已失效)。">
            <QuestionCircleOutlined style={{ color: "#8c8c8c", cursor: "help" }} />
          </Tooltip>
        </Space>
      ),
      key: "review",
      width: 220,
      filters: [
        { text: "方向命中", value: "dir_hit" },
        { text: "方向未命中", value: "dir_miss" },
        { text: "区间触达", value: "touched" },
        { text: "区间命中", value: "band_hit" },
        { text: "区间过宽", value: "band_wide" },
        { text: "已失效", value: "inval_hit" },
      ],
      onFilter: (value, record) => {
        switch (value) {
          case "dir_hit":
          case "dir_miss": {
            const t = predictTrend(record);
            if (t !== "long" && t !== "short") return false;
            const moved = Number(record.realPrice) - Number(record.refPrice);
            const hit = (t === "long" && moved > 0) || (t === "short" && moved < 0);
            return value === (hit ? "dir_hit" : "dir_miss");
          }
          case "touched":
            return record.touched === true;
          case "band_hit":
            return bandHitState(record) === "hit";
          case "band_wide":
            return bandHitState(record) === "wide";
          case "inval_hit":
            return record.invalidationHit === 1;
          default:
            return false;
        }
      },
      render: (_: unknown, record: MarkerRow) => {
        // 行内复用：固定宽度灰色标签 + 取值，四项纵向排列。
        const line = (label: string, value: ReactNode) => (
          <div style={{ display: "flex", alignItems: "center", gap: 4, lineHeight: "20px" }}>
            <span style={{ color: "#8c8c8c", fontSize: 12, width: 40, flexShrink: 0 }}>{label}</span>
            {value}
          </div>
        );

        // 方向命中
        const t = predictTrend(record);
        let dirTag: ReactNode = <Tag>中性</Tag>;
        if (t === "long" || t === "short") {
          const ref = Number(record.refPrice);
          const real = Number(record.realPrice);
          if (!ref || !real) {
            dirTag = <span style={{ color: "#8c8c8c" }}>-</span>;
          } else {
            const hit = (t === "long" && real - ref > 0) || (t === "short" && real - ref < 0);
            dirTag = <Tag color={hit ? "green" : "red"}>{hit ? "命中" : "未命中"}</Tag>;
          }
        }

        // 区间触达
        const touchTag = <Tag color={record.touched ? "gold" : "default"}>{record.touched ? "触达" : "未触达"}</Tag>;

        // 区间命中
        const bandState = bandHitState(record);
        let bandTag: ReactNode = <span style={{ color: "#8c8c8c" }}>-</span>;
        if (bandState !== "none") {
          const util = `${Math.round(record.bandUtil * 100)}%`;
          if (bandState === "hit") bandTag = <Tag color="green">命中 · 利用 {util}</Tag>;
          else if (bandState === "wide") bandTag = <Tag color="orange">过宽 · 利用 {util}</Tag>;
          else bandTag = <Tag color="red">未命中</Tag>;
        }

        // 失效位
        let invalTag: ReactNode = <span style={{ color: "#8c8c8c" }}>-</span>;
        if (record.invalidation) {
          const price = formatPrice(Number(record.invalidation));
          if (record.invalidationHit === 1) invalTag = <Tag color="red">{price} · 已失效</Tag>;
          else if (record.invalidationHit === 0) invalTag = <Tag color="green">{price} · 成立</Tag>;
          else invalTag = <span>{price}</span>;
        }

        return (
          <div>
            {line("方向", dirTag)}
            {line("触达", touchTag)}
            {line("区间", bandTag)}
            {line("失效", invalTag)}
          </div>
        );
      },
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
      const t = predictTrend(m);
      return t === "long" || t === "short";
    });
    const hit = directional.filter((m) => {
      const t = predictTrend(m);
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
            <Space size={8} align="baseline">
              <h3 style={{ margin: 0 }}>点位复核</h3>
              {latestPrice !== null ? (
                <Text type="secondary" style={{ fontSize: 13 }}>
                  最新价 <b style={{ color: "#FCD535" }}>{formatPrice(latestPrice)}</b>
                </Text>
              ) : null}
              <Tooltip title={`单次开仓张数：1 张 = ${CONTRACT_SIZE_BTC} BTC，盈亏/手续费按此数量换算`}>
                <InputNumber
                  size="small"
                  min={1}
                  step={1}
                  value={contracts}
                  onChange={(value) => setContracts(Number(value) || 0)}
                  formatter={(value) => `${value}张`}
                  parser={(value) => (value ? value.replace(/[^\d.]/g, "") : "") as unknown as number}
                  style={{ width: 84 }}
                />
              </Tooltip>
              {contracts > 0 ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  ≈{formatPrice(contracts * CONTRACT_SIZE_BTC)} BTC
                </Text>
              ) : null}
            </Space>
          </div>
          <Space size={12} wrap>
            <Segmented
              size="small"
              value={horizonFilter}
              onChange={(value) => setHorizonFilter(String(value))}
              options={horizonOptions}
            />
            <Tooltip title="只显示预测利润率超过开仓阈值、会实际开仓的点位">
              <Space size={6}>
                <Switch size="small" checked={onlyOpenable} onChange={setOnlyOpenable} />
                <Text style={{ fontSize: 13 }}>只看可开仓</Text>
              </Space>
            </Tooltip>
            <Tooltip title="U本位合约标准费率：开仓市价吃单(Taker)；平仓走止盈限价记 Maker、按收盘市价记 Taker。利润已扣往返手续费。">
              <Tag color="blue">
                费率 Maker {(MAKER_FEE_RATE * 100).toFixed(2)}% / Taker {(TAKER_FEE_RATE * 100).toFixed(2)}%
              </Tag>
            </Tooltip>
            <Tag color={directionStats.rate >= 50 ? "green" : "red"}>
              方向命中 {directionStats.rate}% ({directionStats.hit}/{directionStats.total})
            </Tag>
            <Tag color={profitSummary.total >= 0 ? "green" : "red"}>
              开仓净利 {profitSummary.total >= 0 ? "+" : ""}{formatPrice(profitSummary.total)} USDT / {pct(profitSummary.totalRate)}（{profitSummary.win}/{profitSummary.count} 盈利·{contracts}张·{leverage}x·已扣手续费）
            </Tag>
            <Tag color="geekblue">
              累计开仓 {profitSummary.count * contracts} 张（≈{formatPrice(profitSummary.count * contracts * CONTRACT_SIZE_BTC)} BTC）
            </Tag>
          </Space>
        </div>
        <Table
          rowKey={(record) => `${record.seriesInterval}-${record.timestamp}-${record.label}`}
          loading={loading}
          dataSource={displayRows}
          columns={markerColumns}
          onChange={(_pagination, _filters, _sorter, extra) => setTableFiltered(extra.currentDataSource)}
          pagination={{ defaultPageSize: 10, showSizeChanger: true, pageSizeOptions: ["10", "20", "50"] }}
          scroll={{ x: "max-content" }}
          size="small"
        />
      </section>
    </div>
  );
}
