"use client";

import { instance, unwrapApiResponse, type ApiResponse, type PageResult } from "@/utils/axios";

/**
 * 行情主视图（r12）的读接口封装。
 *
 * 数据全部来自 r9 的 argus_event 只读接口与 strategy 的 K 线接口——本页不写任何东西，
 * 唯一的写动作是「回填缺口」（把交易所的历史 K 线补进 trade_kline，不碰实盘）。
 *
 * ⚠️ 时间口径：argus-event 系列接口的入参与出参都是**本地墙钟串** `YYYY-MM-DD HH:mm:ss`，
 * 不带时区，与事件 ts 逐字一致。只有 /klines/backfill-range 例外，它按 RFC3339 解析，
 * 所以那里必须传 toISOString()。两套口径不要互相灌。
 */

// ─── K 线与时间轴聚合 ────────────────────────────────────────────────────────

export class MarketKline {
  declare time: string;
  declare open: number;
  declare high: number;
  declare low: number;
  declare close: number;
  declare volume: number;
}

export class TimelineBucket {
  declare time: string;
  declare total: number;
  declare open: number;
  declare capSkip: number;
  declare gateBlock: number;
  declare trendSkip: number;
  declare exit: number;
  declare maxAbsGapBp: number | null;
  /** 桶末净持仓张数，桶内无事件时沿用上一桶——净持仓阶梯就是这条序列。 */
  declare netSizeEnd: number;
  declare realizedPnl: number | null;
  declare configVersion: number;
}

export class TimelineCoverage {
  declare expected: number;
  declare actual: number;
  declare coveragePct: number;
  declare missing: number;
}

export class TimelineWindow {
  declare start: string;
  declare end: string;
  /** explicit（入参给定）/ latest-data（按已入库最新时刻回推）/ empty。 */
  declare resolved: string;
}

export class MarketTimeline {
  declare instanceKey: string;
  declare instrument: string;
  declare symbol: string;
  declare interval: string;
  declare platformCode: string;
  declare window: TimelineWindow;
  declare klines: MarketKline[];
  declare buckets: TimelineBucket[];
  declare coverage: TimelineCoverage;
  /** 双源对比的第二条序列，只在请求带 comparePlatformCode 时有值。 */
  declare comparePlatformCode: string;
  declare compareKlines: MarketKline[];
  declare compareCoverage: TimelineCoverage;
  declare eventTotal: number;
  declare truncated: boolean;
}

// ─── 触发点 ──────────────────────────────────────────────────────────────────

export class SignalEvent {
  declare eventId: number;
  declare ts: string;
  declare instanceKey: string;
  declare configVersion: number;
  declare uid: string;
  declare accountLabel: string;
  declare variant: string;
  declare event: string;
  declare eventLabel: string;
  declare instrument: string;
  declare instIdRaw: string;
  /** open / blocked / exit / alert。 */
  declare resultKind: string;
  declare side: string;
  declare netSide: string;
  /** 信号方向 UP/DOWN，由 gapBp 符号推出；无报价快照时为空。 */
  declare direction: string;
  declare size: number | null;
  declare orderSize: number | null;
  declare avgPx: number | null;
  declare lastPx: number | null;
  declare roiPct: number | null;
  declare pnl: number | null;
  declare peakPct: number | null;
  declare sigLast: number | null;
  declare sigMark: number | null;
  declare gapBp: number | null;
  declare trendMomPct: number | null;
  declare strengthLevel: string;
  declare gateKind: string;
  declare gateLabel: string;
  declare gateThreshold: number | null;
  declare gateActual: number | null;
  declare reason: string;
  declare source: number;
  declare sourceLabel: string;
}

// ─── 筛选项 ──────────────────────────────────────────────────────────────────

export class EventInstanceOption {
  declare instanceKey: string;
  declare instanceName: string;
  declare enabled: number;
  /** 事件里出现过但 argus_instance 未注册时为 false。 */
  declare registered: boolean;
  declare eventCount: number;
  declare firstTs: string;
  declare lastTs: string;
}

export class MarketFilterOptions {
  declare instances: EventInstanceOption[];
  declare instruments: string[];
  declare dataRange: TimelineWindow;
}

// ─── 秒级切片 ────────────────────────────────────────────────────────────────

export class SlicePoint {
  declare ts: string;
  declare offsetSec: number;
  declare dcLast: number | null;
  declare dcMark: number | null;
  /** 币安 last。只有 signal_slice 逐秒切片才有，降级来源恒为 null。 */
  declare binLast: number | null;
  declare gapBp: number | null;
  declare events: string[];
  declare isTriggerTs: boolean;
}

export class SliceDevSample {
  declare ts: string;
  declare devTicks: number;
  declare devMaxBp: number | null;
  declare devMeanBp: number | null;
}

export class SignalSlice {
  declare signalId: number;
  declare ts: string;
  declare instanceKey: string;
  declare instrument: string;
  declare windowSeconds: number;
  declare window: TimelineWindow;
  /** signal_slice（r3 逐秒切片）/ strategy_event（降级来源，只有触发那一秒有点）。 */
  declare tickSource: string;
  declare tickComplete: boolean;
  /** 供数受限说明（降级来源 / 窗口被收窄），可能与 tickComplete=true 并存。 */
  declare degradedReason: string;
  /** 切片锚点 = 偏离穿越时刻；走 signal_slice 时 points 以它为原点。降级时为空串。 */
  declare sliceAnchorTs: string;
  /** 事件 ts − 切片锚点（信号延迟调度 + 下单往返），秒。 */
  declare anchorLagSec: number;
  /** DC / 币安两条序列各自的非空秒数，用来判断某一侧是否断流。 */
  declare dcPoints: number;
  declare binPoints: number;
  declare points: SlicePoint[];
  declare devSamples: SliceDevSample[];
  declare klines: MarketKline[];
  /** platformCode/symbol/interval，说明下面那条参照线是谁的。 */
  declare klineSource: string;
}

// ─── 缺口回填 ────────────────────────────────────────────────────────────────

export class BackfillRangeItem {
  declare platformCode: string;
  declare symbol: string;
  declare interval: string;
  declare expected: number;
  declare haveBefore: number;
  declare needFetch: number;
  declare fetched: number;
  declare upserted: number;
  declare haveAfter: number;
  declare skipped: boolean;
  /** 窗口左界超出交易所单次「最近 N 根」上限，补不到头。 */
  declare capped: boolean;
  declare note: string;
  declare error?: string;
}

export class BackfillRangeResult {
  declare start: string;
  declare end: string;
  declare total: number;
  declare succeeded: number;
  declare failed: number;
  declare items: BackfillRangeItem[];
}

// ─── 请求 ────────────────────────────────────────────────────────────────────

type QueryParams = Record<string, string | number | undefined>;

async function get<T>(url: string, params?: QueryParams): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

/** 有事件数据的实例、币种与整体数据区间，一次拉齐供筛选器使用。 */
export function fetchMarketFilterOptions(): Promise<MarketFilterOptions> {
  return get<MarketFilterOptions>("/argus-event/filter-options");
}

export interface TimelineParams {
  instanceKey: string;
  instrument: string;
  interval: string;
  platformCode: string;
  /** 双源对比的第二个平台；不需要对比时不传。 */
  comparePlatformCode?: string;
  start: string;
  end: string;
  accountLabel?: string;
}

/** instanceKey 必填：净持仓与开仓率是实例内的量，跨实例相加会串数据。 */
export function fetchMarketTimeline(params: TimelineParams): Promise<MarketTimeline> {
  return get<MarketTimeline>("/argus-event/timeline", { ...params });
}

export interface TriggerPageParams {
  instanceKey: string;
  instrument: string;
  start: string;
  end: string;
  pageIndex: number;
  pageSize: number;
}

/**
 * 取窗口内的触发点。**刻意按事件行返回、不做任何分钟级折叠**：实测 28.7% 的信号
 * 在同一分钟内触发 ≥2 次、单分钟最多 5 次，折叠会直接丢掉约一半触发。
 */
export function fetchTriggerPage(params: TriggerPageParams): Promise<PageResult<SignalEvent>> {
  return get<PageResult<SignalEvent>>("/argus-event/signals", {
    ...params,
    category: "trigger",
    order: "ts_asc",
  });
}

export function fetchSignalSlice(eventId: number, windowSeconds = 60): Promise<SignalSlice> {
  return get<SignalSlice>(`/argus-event/signals/${eventId}/slice`, { windowSeconds });
}

export interface BackfillRangeParams {
  platformCodes: string[];
  symbol: string;
  intervals: string[];
  /** RFC3339（带时区偏移）。这个接口按 RFC3339/UTC 解析，不接受裸墙钟串。 */
  start: string;
  end: string;
}

/**
 * 按窗口覆盖率回填 K 线。
 *
 * 走的是 /klines/backfill-range 而不是 /klines/backfill：后者以「DB 最新一根」为
 * 基准算增量，窗口整段落在过去时会判成「已是最新」而完全不补，正是行情主视图要补的
 * 那种历史空洞。
 */
export function backfillKlineRange(params: BackfillRangeParams): Promise<BackfillRangeResult> {
  return instance
    .post<ApiResponse<BackfillRangeResult>>("/klines/backfill-range", params)
    .then((response) => unwrapApiResponse(response.data));
}
