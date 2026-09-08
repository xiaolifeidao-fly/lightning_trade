"use client";

import {
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
  LineStyle,
  LineType,
  createSeriesMarkers,
  type AutoscaleInfo,
  type IChartApi,
  type ISeriesApi,
  type ISeriesMarkersPluginApi,
  type SeriesMarker,
  type Time,
  type UTCTimestamp,
} from "lightweight-charts";
import { useEffect, useMemo, useRef } from "react";
import { LightweightChart } from "@/components/charts/LightweightChart";
import { alignToBar, chartPalette, intervalSeconds, wallClockToChartTime } from "@/components/charts/chartTheme";
import type { MarketKline, SignalEvent, TimelineBucket } from "../api/argus-market.api";
import { MAX_MARKERS_PER_BAR, PLATFORM_LABELS, TRIGGER_KIND_MAP, type MarketInterval } from "../constants";

/** 一次触发在图上的标记 id；点选时由 hoveredObjectId 原样回传。 */
const markerId = (eventId: number) => `sig-${eventId}`;
const eventIdOfMarker = (id: string) => Number(id.replace("sig-", ""));

interface MarketChartProps {
  interval: MarketInterval;
  platformCode: string;
  klines: MarketKline[];
  comparePlatformCode: string;
  compareKlines: MarketKline[];
  buckets: TimelineBucket[];
  /** 窗口内的全部触发点（不受「触发点类型」筛选影响），偏离副图取自这一份。 */
  triggers: SignalEvent[];
  /** 已按「触发点类型」筛过的触发点，图上标记取自这一份。 */
  visibleTriggers: SignalEvent[];
  /** 偏离副图的阈值线（bp），跟随实例的已发布配置；读不到时不画线。 */
  thresholdBp: number | null;
  /** 净持仓阶梯的上限线（张），实例内各账户 maxContracts 之和。 */
  positionCap: number | null;
  selectedEventId: number | null;
  onSelectTrigger: (eventId: number) => void;
  /** 同一根 K 线上超过上限被折叠掉的触发点条数，交给页面显式提示。 */
  onMarkerOverflow: (count: number) => void;
}

/**
 * 三个 pane 的高度占比：主图 / 偏离 / 净持仓。
 *
 * 用 setStretchFactor 而不是 setHeight——setHeight 是按当前总高换算成占比再重分配，
 * 三个 pane 顺序调用会互相稀释，最后谁也不是设定的高度（实测偏离副图被压到 ~35px）。
 */
const PANE_STRETCH = [5.5, 2, 1.6];
const TOTAL_HEIGHT = 584;

/**
 * 行情与触发点叠加主视图。
 *
 * 三个 pane 共用一条时间轴：主图 K 线（可叠第二个数据源的收盘对比线）、
 * last-vs-mark 偏离、净持仓阶梯。四类触发点作为标记挂在 K 线序列上——挂在序列上
 * 而不是自绘图层，是为了拿到 lightweight-charts 自己的同 bar 堆叠与命中测试：
 * 实测同一分钟内最多 5 次触发，堆叠与逐个点选是这一页的硬要求。
 */
export function MarketChart(props: MarketChartProps) {
  return (
    <LightweightChart
      height={TOTAL_HEIGHT}
      options={{
        timeScale: {
          timeVisible: props.interval !== "1d",
          secondsVisible: false,
          borderColor: chartPalette.border,
        },
      }}
    >
      {(chart) => <MarketSeries chart={chart} {...props} />}
    </LightweightChart>
  );
}

/**
 * 序列层：只跑副作用、不渲染 DOM。
 *
 * 拆成两个 effect 是刻意的——数据 effect 会重建序列并 fitContent，标记 effect 只换
 * 标记。勾掉一类触发点或点选一行时走的是后者，图的缩放与十字光标位置不会被刷掉。
 */
function MarketSeries({
  chart,
  interval,
  platformCode,
  klines,
  comparePlatformCode,
  compareKlines,
  buckets,
  triggers,
  visibleTriggers,
  thresholdBp,
  positionCap,
  selectedEventId,
  onSelectTrigger,
  onMarkerOverflow,
}: MarketChartProps & { chart: IChartApi }) {
  const candleRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  // 插件泛型跟随图表的横轴类型（Time），不是我们喂进去的 UTCTimestamp 子类型。
  const markersRef = useRef<ISeriesMarkersPluginApi<Time> | null>(null);
  // 点选回调每次渲染都是新函数，用 ref 兜住，避免把订阅 effect 拖成每渲染重订阅。
  const selectRef = useRef(onSelectTrigger);
  selectRef.current = onSelectTrigger;
  const overflowRef = useRef(onMarkerOverflow);
  overflowRef.current = onMarkerOverflow;

  const barSeconds = intervalSeconds[interval] ?? 60;

  // K 线时间网格：标记只能挂在序列里真实存在的 bar 上（lightweight-charts 对找不到
  // 数据的 marker 直接跳过），所以标记与副图都要先对齐到这张网格。
  const candleData = useMemo(() => toCandleData(klines), [klines]);
  const barTimes = useMemo(() => new Set(candleData.map((item) => item.time)), [candleData]);

  useEffect(() => {
    if (candleData.length === 0) return;

    const candle = chart.addSeries(
      CandlestickSeries,
      {
        upColor: chartPalette.up,
        downColor: chartPalette.down,
        borderUpColor: chartPalette.up,
        borderDownColor: chartPalette.down,
        wickUpColor: chartPalette.up,
        wickDownColor: chartPalette.down,
        priceLineVisible: false,
        lastValueVisible: false,
        title: PLATFORM_LABELS[platformCode] ?? platformCode,
      },
      0,
    );
    candle.setData(candleData);
    candleRef.current = candle;

    // 双源对比：第二个数据源只画收盘线。两条蜡烛叠在一起谁也看不清，而这一页要看的
    // 是「同一时刻两家报价差多少」，一条线足够。
    let compare: ISeriesApi<"Line"> | null = null;
    const compareData = toLineData(compareKlines, (item) => item.close);
    if (comparePlatformCode && compareData.length > 0) {
      compare = chart.addSeries(
        LineSeries,
        {
          color: chartPalette.accent,
          lineWidth: 1,
          priceLineVisible: false,
          lastValueVisible: false,
          crosshairMarkerVisible: false,
          title: PLATFORM_LABELS[comparePlatformCode] ?? comparePlatformCode,
        },
        0,
      );
      compare.setData(compareData);
    }

    // 偏离副图用直方图而不是折线：偏离只在触发那一刻被观测到（生产阈值以下的样本
    // 根本不进 strategy_event），把离散观测连成曲线会让人以为中间那段也被测过。
    const deviation = chart.addSeries(
      HistogramSeries,
      {
        base: 0,
        priceLineVisible: false,
        lastValueVisible: false,
        priceFormat: { type: "price", precision: 2, minMove: 0.01 },
        // 阈值线不参与 lightweight-charts 的自动缩放，窗口内偏离都没越阈时它会被挤出
        // 可视区。这条副图的意义就是「越没越阈」，所以把阈值强行纳入取值范围。
        autoscaleInfoProvider: (base: () => AutoscaleInfo | null) =>
          widenRange(base(), thresholdBp == null ? [] : [thresholdBp, -thresholdBp]),
      },
      1,
    );
    deviation.setData(toDeviationData(triggers, barSeconds, barTimes));

    const position = chart.addSeries(
      LineSeries,
      {
        color: chartPalette.primary,
        lineWidth: 2,
        lineType: LineType.WithSteps,
        priceLineVisible: false,
        lastValueVisible: false,
        priceFormat: { type: "price", precision: 0, minMove: 1 },
        // 同理：离上限还有多少空间是这条阶梯要回答的问题，净仓远低于上限时
        // 上限线不能被裁掉，否则看不出「还能加多少」。
        autoscaleInfoProvider: (base: () => AutoscaleInfo | null) =>
          widenRange(base(), positionCap == null ? [0] : [0, positionCap]),
      },
      2,
    );
    position.setData(toPositionData(buckets));

    // 阈值线跟随实例：三个实例的 signal_threshold 本来就不同（5bp / 3bp），
    // 前端写死等于把对照组画错，读不到已发布配置时宁可不画。
    if (thresholdBp != null && thresholdBp > 0) {
      for (const value of [thresholdBp, -thresholdBp]) {
        deviation.createPriceLine({
          price: value,
          color: chartPalette.orange,
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          axisLabelVisible: true,
          title: `阈值 ${value > 0 ? "+" : ""}${value.toFixed(1)}bp`,
        });
      }
    }
    if (positionCap != null && positionCap > 0) {
      position.createPriceLine({
        price: positionCap,
        color: chartPalette.down,
        lineWidth: 1,
        lineStyle: LineStyle.Dashed,
        axisLabelVisible: true,
        title: `上限 ${positionCap} 张`,
      });
    }

    const panes = chart.panes();
    PANE_STRETCH.forEach((factor, index) => panes[index]?.setStretchFactor(factor));
    chart.timeScale().fitContent();

    return () => {
      markersRef.current = null;
      candleRef.current = null;
      chart.removeSeries(candle);
      if (compare) chart.removeSeries(compare);
      chart.removeSeries(deviation);
      chart.removeSeries(position);
    };
    // 依赖里只有窗口数据，没有 visibleTriggers：勾掉一类触发点只该换标记，
    // 不该重建序列——重建会把用户的缩放与十字光标位置一起刷掉。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    chart,
    candleData,
    barTimes,
    platformCode,
    compareKlines,
    comparePlatformCode,
    buckets,
    triggers,
    thresholdBp,
    positionCap,
    barSeconds,
  ]);

  // 标记：同一根 K 线上最多叠 MAX_MARKERS_PER_BAR 个，超出的显式报数不静默丢弃。
  useEffect(() => {
    const candle = candleRef.current;
    if (!candle) return;
    const { markers, overflow } = buildMarkers(visibleTriggers, barSeconds, barTimes, selectedEventId);
    if (!markersRef.current) {
      markersRef.current = createSeriesMarkers(candle, markers);
    } else {
      markersRef.current.setMarkers(markers);
    }
    overflowRef.current(overflow);
  }, [visibleTriggers, barSeconds, barTimes, selectedEventId, candleData]);

  // 点标记下钻。lightweight-charts 把 marker.id 原样透出成 hoveredObjectId，
  // 所以同一根上叠的 5 个标记能各自被点到——这正是任务要求的「分别点选」。
  useEffect(() => {
    const handler = (param: { hoveredObjectId?: unknown }) => {
      const id = typeof param.hoveredObjectId === "string" ? param.hoveredObjectId : "";
      if (!id.startsWith("sig-")) return;
      const eventId = eventIdOfMarker(id);
      if (Number.isFinite(eventId)) selectRef.current(eventId);
    };
    chart.subscribeClick(handler);
    return () => chart.unsubscribeClick(handler);
  }, [chart]);

  // 选中表格某一行时把图定位过去：只在该 bar 已经不在可视区时才移动，
  // 否则每点一行图都跳一下反而看不清上下文。
  useEffect(() => {
    if (selectedEventId == null || candleData.length === 0) return;
    const target = triggers.find((item) => item.eventId === selectedEventId);
    if (!target) return;
    const time = alignedTime(target.ts, barSeconds);
    if (time == null) return;
    const index = candleData.findIndex((item) => item.time === time);
    if (index < 0) return;
    const range = chart.timeScale().getVisibleLogicalRange();
    if (range && index >= range.from && index <= range.to) return;
    const span = range ? Math.max(10, (range.to - range.from) / 2) : 60;
    chart.timeScale().setVisibleLogicalRange({ from: index - span, to: index + span });
  }, [chart, selectedEventId, triggers, candleData, barSeconds]);

  return null;
}

/**
 * 把参考线的取值并进序列自身的自动缩放范围。
 *
 * lightweight-charts 的 createPriceLine 不影响价格轴范围——线画在范围外就直接看不见。
 * 阈值线与上限线恰恰是"数据没碰到它"的时候最需要被看见，所以这里显式撑开。
 */
function widenRange(base: AutoscaleInfo | null, extras: number[]): AutoscaleInfo | null {
  const values = extras.filter((value) => Number.isFinite(value));
  if (values.length === 0) return base;
  const range = base?.priceRange;
  const minValue = Math.min(...values, range?.minValue ?? Number.POSITIVE_INFINITY);
  const maxValue = Math.max(...values, range?.maxValue ?? Number.NEGATIVE_INFINITY);
  return { ...base, priceRange: { minValue, maxValue } };
}

// ─── 数据转换 ────────────────────────────────────────────────────────────────

interface CandlePoint {
  time: UTCTimestamp;
  open: number;
  high: number;
  low: number;
  close: number;
}

/**
 * lightweight-charts 要求序列按时间升序且时间唯一，否则整条序列直接抛错。
 * 后端已按 open_time 升序返回，这里仍然排序去重——K 线双源回填是幂等 upsert，
 * 表里出现同一根的重复行不是不可能，一根脏数据不该让整页白屏。
 */
function dedupeAscending<T extends { time: UTCTimestamp }>(points: T[]): T[] {
  const byTime = new Map<number, T>();
  for (const point of points) byTime.set(point.time, point);
  return Array.from(byTime.values()).sort((a, b) => a.time - b.time);
}

function toCandleData(klines: MarketKline[]): CandlePoint[] {
  const points: CandlePoint[] = [];
  for (const item of klines) {
    const time = wallClockToChartTime(item.time);
    if (time == null) continue;
    points.push({ time, open: item.open, high: item.high, low: item.low, close: item.close });
  }
  return dedupeAscending(points);
}

function toLineData(klines: MarketKline[], pick: (item: MarketKline) => number) {
  const points: { time: UTCTimestamp; value: number }[] = [];
  for (const item of klines) {
    const time = wallClockToChartTime(item.time);
    if (time == null) continue;
    points.push({ time, value: pick(item) });
  }
  return dedupeAscending(points);
}

function alignedTime(wallClock: string, barSeconds: number): UTCTimestamp | null {
  const time = wallClockToChartTime(wallClock);
  return time == null ? null : alignToBar(time, barSeconds);
}

/**
 * 偏离副图：一根 K 线取窗口内**绝对值最大**的那次触发的带符号 gapBp。
 *
 * 取绝对值最大而不是均值，是因为这条副图要回答的是「这根里最极端的一次偏离有多大、
 * 够不够阈值」；均值会把同一根里的一正一负抵消掉，看上去像没触发过。带符号是必须的
 * ——正 = UP、负 = DOWN，方向丢了就没法和主图上的开仓方向对上。
 */
function toDeviationData(triggers: SignalEvent[], barSeconds: number, barTimes: Set<UTCTimestamp>) {
  const byBar = new Map<number, number>();
  for (const item of triggers) {
    if (item.gapBp == null) continue;
    const time = alignedTime(item.ts, barSeconds);
    if (time == null || !barTimes.has(time)) continue;
    const current = byBar.get(time);
    if (current == null || Math.abs(item.gapBp) > Math.abs(current)) byBar.set(time, item.gapBp);
  }
  return Array.from(byBar.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([time, value]) => ({
      time: time as UTCTimestamp,
      value,
      color: value >= 0 ? chartPalette.up : chartPalette.down,
    }));
}

/** 净持仓阶梯：桶末净仓，桶内无事件时后端已沿用上一桶，这里直接铺开成阶梯线。 */
function toPositionData(buckets: TimelineBucket[]) {
  const points: { time: UTCTimestamp; value: number }[] = [];
  for (const item of buckets) {
    const time = wallClockToChartTime(item.time);
    if (time == null) continue;
    points.push({ time, value: item.netSizeEnd });
  }
  return dedupeAscending(points);
}

/**
 * 触发点标记。
 *
 * 成交画在蜡烛下方、三类拦截画在上方，一眼分开「做成了」和「被挡住」。同一根上的多个
 * 标记由 lightweight-charts 自己沿垂直方向堆叠，所以这里只需要保证：① 时间对齐到 bar
 * 且该 bar 真实存在；② 按时间升序；③ 每根不超过上限。
 */
function buildMarkers(
  triggers: SignalEvent[],
  barSeconds: number,
  barTimes: Set<UTCTimestamp>,
  selectedEventId: number | null,
): { markers: SeriesMarker<UTCTimestamp>[]; overflow: number } {
  const perBar = new Map<number, number>();
  const markers: SeriesMarker<UTCTimestamp>[] = [];
  let overflow = 0;

  const sorted = [...triggers].sort((a, b) => (a.ts === b.ts ? a.eventId - b.eventId : a.ts.localeCompare(b.ts)));
  for (const item of sorted) {
    const meta = TRIGGER_KIND_MAP[item.event];
    if (!meta) continue;
    const time = alignedTime(item.ts, barSeconds);
    if (time == null || !barTimes.has(time)) continue;
    const used = perBar.get(time) ?? 0;
    if (used >= MAX_MARKERS_PER_BAR) {
      overflow += 1;
      continue;
    }
    perBar.set(time, used + 1);
    const selected = selectedEventId === item.eventId;
    markers.push({
      time,
      position: meta.position,
      shape: meta.shape,
      color: selected ? chartPalette.primary : meta.color,
      id: markerId(item.eventId),
      size: selected ? 1.6 : 1,
      // 只给选中的那个挂文字：每个标记都带文字时同一根上叠 5 个会糊成一片。
      text: selected ? `${meta.label} ${item.ts.slice(11)}` : undefined,
    });
  }
  return { markers, overflow };
}
