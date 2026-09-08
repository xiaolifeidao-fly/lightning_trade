"use client";

import { instance, unwrapApiResponse, type ApiResponse, type PageResult } from "@/utils/axios";

/**
 * 信号复盘与 episode 详情页（r13）的读接口封装。
 *
 * 数据源全部是 r9 的 argus_event 只读接口与 r10 派生出来的 episode 两张表。
 * 本页**没有任何写入动作**——它要取代的是「导出 TG 历史消息交给 AI 读」这个
 * 只读流程，页面上不存在能影响实盘进程或改数据的入口。
 *
 * ⚠️ 时间口径：入参与出参都是**本地墙钟串** `YYYY-MM-DD HH:mm:ss`，与事件 ts、
 * logs/ 下的 JSONL 和 Telegram 消息逐字一致，不做任何时区换算。
 *
 * ⚠️ 展示名一律取服务端返回的 *Label 字段（eventLabel / gateLabel / exitLabel /
 * statusLabel），不要在前端另打一份映射表：写侧改了事件常量，服务端会编译报错，
 * 而前端的副本只会静默显示成旧名字。
 */

// ─── 通用片段 ────────────────────────────────────────────────────────────────

export class EventWindow {
  declare start: string;
  declare end: string;
  /** explicit（入参给定）/ latest-data（按已入库最新时刻回推）/ empty。 */
  declare resolved: string;
}

export class EventOption {
  declare value: string;
  declare label: string;
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

export class EventAccountOption {
  /** 账户唯一性是 (instanceKey, accountLabel)：账户名会跨实例重号。 */
  declare instanceKey: string;
  declare accountLabel: string;
  declare uid: string;
  declare variant: string;
  declare instrument: string;
  declare eventCount: number;
  declare firstTs: string;
  declare lastTs: string;
}

export class EventConfigVersionOption {
  declare instanceKey: string;
  declare configVersion: number;
  declare eventCount: number;
  declare firstTs: string;
  declare lastTs: string;
}

export class EventStrengthOption {
  declare value: string;
  declare label: string;
  declare minAbsBp: number | null;
  declare maxAbsBp: number | null;
}

export class SignalFilterOptions {
  declare instances: EventInstanceOption[];
  declare accounts: EventAccountOption[];
  declare instruments: string[];
  declare variants: string[];
  declare configVersions: EventConfigVersionOption[];
  declare eventKinds: EventOption[];
  declare gateKinds: EventOption[];
  declare strengths: EventStrengthOption[];
  declare sources: EventOption[];
  declare dataRange: EventWindow;
}

export class ExitKindOption {
  /** 'open' = 仍持仓（派生表里 exit_kind IS NULL）。 */
  declare value: string;
  declare label: string;
  declare count: number;
  declare countedInWinRate: boolean;
}

// ─── 信号列表 ────────────────────────────────────────────────────────────────

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

// ─── 信号详情 ────────────────────────────────────────────────────────────────

export class AccountDecision {
  declare eventId: number;
  declare ts: string;
  /** 相对锚点事件的秒偏移：下单往返会让成交类事件比拦截类晚几秒。 */
  declare tsOffsetSec: number;
  declare accountLabel: string;
  declare uid: string;
  declare variant: string;
  declare result: string;
  declare resultLabel: string;
  declare resultKind: string;
  declare side: string;
  declare netSide: string;
  declare orderSize: number | null;
  declare netSize: number | null;
  declare avgPx: number | null;
  declare lastPx: number | null;
  declare roiPct: number | null;
  declare pnl: number | null;
  declare gateKind: string;
  declare gateLabel: string;
  declare gateThreshold: number | null;
  declare gateActual: number | null;
  declare reason: string;
  declare configVersion: number;
}

export class SignalDetail {
  declare signalId: number;
  declare ts: string;
  declare instanceKey: string;
  declare instrument: string;
  declare instIdRaw: string;
  declare direction: string;
  declare side: string;
  declare sigLast: number | null;
  declare sigMark: number | null;
  declare gapBp: number | null;
  declare strengthLevel: string;
  declare trendMomPct: number | null;
  declare configVersions: number[];
  declare variants: string[];
  declare accountCount: number;
  declare openedCount: number;
  declare blockedCount: number;
  declare totalOrderQty: number;
  declare accounts: AccountDecision[];
  /** 按 TG 消息原格式还原的 [n] / [跳过n] 明细行。 */
  declare telegramLines: string[];
  /** false 时成交价/委托均价必须显示「—」而不是 0：埋点没落这两个字段。 */
  declare fillPriceAvailable: boolean;
  declare notice: string;
  declare groupWindowSec: number;
}

// ─── episode ─────────────────────────────────────────────────────────────────

export class EpisodeRebuildState {
  declare episodeCount: number;
  declare lastRebuiltAt: string;
  /** 派生结果覆盖到的最末事件时刻。 */
  declare derivedThroughTs: string;
  /** 事件表里的最末事件时刻；明显晚于 derivedThroughTs 就是滞后。 */
  declare latestEventTs: string;
  declare lagSeconds: number | null;
  declare stale: boolean;
  declare notice: string;
}

export class Episode {
  declare episodeId: number;
  declare instanceKey: string;
  declare uid: string;
  declare accountLabel: string;
  declare instrument: string;
  declare side: string;
  declare firstEventAt: string;
  /** 空串 = 建仓在数据窗口之前（截断头）。 */
  declare openedAt: string;
  declare lastEventAt: string;
  /** 空串 = 仍持仓。 */
  declare closedAt: string;
  declare durationSec: number | null;
  declare exitKind: string;
  declare exitLabel: string;
  declare status: string;
  declare statusLabel: string;
  /** false = 这笔不计入策略胜率（external_close / manual_close / 仍持仓）。 */
  declare strategyAttributable: boolean;
  declare truncatedHead: boolean;
  declare addCount: number;
  declare reduceCount: number;
  declare entrySizeTotal: number;
  declare maxSize: number;
  declare openSize: number;
  declare hiddenSize: number;
  declare minRoiPctObserved: number | null;
  declare maxRoiPctObserved: number | null;
  declare peakPct: number | null;
  declare exitRoiPct: number | null;
  /** 回吐比例；peakPct 缺失或 ≤ 0 时为 null，不补 0。 */
  declare givebackPct: number | null;
  declare pnl: number;
  declare pnlStrategy: number;
  declare unattributedPnl: number;
  /** false 时 pnl 偏小且偏差方向未知（历史反向减仓只写 size 不写 pnl）。 */
  declare pnlKnown: boolean;
  declare realizedEvents: number;
  declare missingPnlEvents: number;
  declare capSkipCount: number;
  declare gateBlockCount: number;
  declare trendSkipCount: number;
  declare lossAlertCount: number;
  declare balanceSampleCount: number;
  declare uplSampleCount: number;
  declare positionGapCount: number;
  declare hasPositionGap: boolean;
  declare variant: string;
  declare configVersion: number;
  /** minute / mixed / alert_sampled，决定浮盈轨迹能画多细。 */
  declare depthFidelity: string;
  declare depthFidelityLabel: string;
  declare rebuiltAt: string;
}

export class EpisodeList {
  declare total: number;
  declare data: Episode[];
  declare window: EventWindow;
  declare timeField: string;
  declare timeFieldNotice: string;
  declare crossInstance: boolean;
  declare notice: string;
  declare rebuild: EpisodeRebuildState;
}

export class EpisodeEntry {
  declare entryId: number;
  declare decidedAt: string;
  declare side: string;
  declare addedSize: number;
  declare orderSize: number | null;
  declare closedSize: number;
  declare openSize: number;
  declare attributedPnl: number;
  declare attributedPnlStrategy: number;
  declare missingPnlEvents: number;
  declare pnlKnown: boolean;
  declare variant: string;
  declare configVersion: number;
  declare gapBp: number | null;
  declare strengthLevel: string;
  declare avgPx: number | null;
  declare lastPx: number | null;
}

export class EpisodeRoiPoint {
  declare ts: string;
  declare roiPct: number;
  declare event: string;
  declare label: string;
  /** observed（真实观测）/ peak（trail 峰值，时刻未知，挂在出场时刻上）。 */
  declare kind: string;
}

export class EpisodeUplPoint {
  declare ts: string;
  /** 账户级未实现盈亏，单位 U。**不换算成 ROI%**，保证金没有落库。 */
  declare upl: number | null;
  declare equity: number | null;
  declare netSize: number | null;
}

export class EpisodeTimelineItem {
  declare ts: string;
  declare eventId: number;
  declare event: string;
  declare label: string;
  /** ok / warn / err / mute，服务端给的轴点色调，前端不要另写一套。 */
  declare tone: string;
  declare side: string;
  declare netSize: number | null;
  declare orderSize: number | null;
  declare avgPx: number | null;
  declare lastPx: number | null;
  declare roiPct: number | null;
  declare pnl: number | null;
  declare peakPct: number | null;
  declare gapBp: number | null;
  declare gateKind: string;
  declare gateLabel: string;
  declare reason: string;
  /** 这条事件产生了一笔建仓决策（对应一行 episode_entry）。 */
  declare isDecision: boolean;
}

export class EpisodeExitAttribution {
  declare exitKind: string;
  declare exitLabel: string;
  declare attributable: boolean;
  declare countedInWinRate: boolean;
  declare reason: string;
  declare exitRoiPct: number | null;
  declare peakPct: number | null;
  declare givebackPct: number | null;
  declare givebackNote: string;
}

export class EpisodeDetail {
  declare episode: Episode;
  declare exit: EpisodeExitAttribution;
  declare entries: EpisodeEntry[];
  declare timeline: EpisodeTimelineItem[];
  declare roiTrack: EpisodeRoiPoint[];
  declare uplTrack: EpisodeUplPoint[];
  /** depth_fidelity = alert_sampled 时恒为 false：那段时间的浮盈轨迹不存在。 */
  declare uplTrackAvailable: boolean;
  declare uplTrackNotice: string;
  declare roiTrackNotice: string;
  declare eventTruncated: boolean;
  declare uplTruncated: boolean;
}

// ─── 聚合统计 ────────────────────────────────────────────────────────────────

export class ResultBucket {
  declare event: string;
  declare label: string;
  declare count: number;
  declare share: number;
}

export class GateBucket {
  declare gateKind: string;
  declare label: string;
  declare count: number;
  declare share: number;
  declare avgThreshold: number | null;
  declare avgActual: number | null;
  declare lastTs: string;
  declare sampleReason: string;
}

export class StrengthBucket {
  declare level: string;
  declare label: string;
  declare minAbsBp: number | null;
  declare maxAbsBp: number | null;
  declare count: number;
  declare opened: number;
  declare openRate: number;
}

export class GateStats {
  declare window: EventWindow;
  declare instanceKeys: string[];
  declare crossInstance: boolean;
  declare notice: string;
  declare totalTriggers: number;
  declare opened: number;
  declare blocked: number;
  declare openRate: number;
  declare byResult: ResultBucket[];
  declare byGate: GateBucket[];
  declare byStrength: StrengthBucket[];
  /** 命中单次扫描行数上限：聚合只覆盖窗口内的一部分事件，必须提示缩小窗口。 */
  declare truncated: boolean;
}

export class ExitKindBucket {
  /** 空串 = 仍持仓。 */
  declare exitKind: string;
  declare label: string;
  declare count: number;
  declare share: number;
  declare pnl: number;
  declare wins: number;
  /** 不计入策略胜率的那两类恒为 null。 */
  declare winRate: number | null;
  declare countedInWinRate: boolean;
}

export class EpisodeStats {
  declare window: EventWindow;
  declare timeField: string;
  declare instanceKeys: string[];
  declare crossInstance: boolean;
  declare notice: string;
  declare total: number;
  declare closed: number;
  declare stillOpen: number;
  /** 计入策略胜率的条数；excluded 是被剔除的条数，其盈亏仍计入 pnl。 */
  declare attributable: number;
  declare excluded: number;
  declare wins: number;
  /** 分母是 attributable，不是 total。 */
  declare winRate: number | null;
  declare pnl: number;
  declare pnlStrategy: number;
  declare unattributedPnl: number;
  declare incompletePnl: number;
  declare truncatedHead: number;
  declare byExitKind: ExitKindBucket[];
  declare rebuild: EpisodeRebuildState;
}

// ─── 切片对比 ────────────────────────────────────────────────────────────────

export class SliceCompareRow {
  declare instanceKey: string;
  declare instanceName: string;
  declare registered: boolean;
  declare configVersion: number;
  declare signals: number;
  declare opened: number;
  declare capSkip: number;
  declare gateBlock: number;
  declare trendSkip: number;
  declare openRate: number;
  declare avgGapBp: number | null;
  declare weak: number;
  declare medium: number;
  declare strong: number;
  declare firstTs: string;
  declare lastTs: string;
  declare episodes: number;
  declare closedEpisodes: number;
  declare attributableEpisodes: number;
  declare wins: number;
  declare winRate: number | null;
  declare pnl: number;
  declare pnlStrategy: number;
  declare avgPeakPct: number | null;
  declare avgExitRoiPct: number | null;
  /** false 既可能是「这段没有持仓」，也可能是「还没跑重建」，靠 rebuild 区分。 */
  declare episodeAvailable: boolean;
  declare variants: string[];
}

export class SliceCompare {
  declare window: EventWindow;
  declare timeField: string;
  declare rows: SliceCompareRow[];
  declare crossInstance: boolean;
  declare notice: string;
  declare signalTruncated: boolean;
  declare rebuild: EpisodeRebuildState;
}

// ─── 请求 ────────────────────────────────────────────────────────────────────

type QueryParams = Record<string, string | number | undefined>;

async function get<T>(url: string, params?: QueryParams): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

export function fetchSignalFilterOptions(instanceKeys?: string): Promise<SignalFilterOptions> {
  return get<SignalFilterOptions>("/argus-event/filter-options", { instanceKeys });
}

export interface SignalListParams {
  pageIndex: number;
  pageSize: number;
  /** 空 = 全部实例；此时每行仍带 instanceKey，页面必须逐行标注归属。 */
  instanceKey?: string;
  start?: string;
  end?: string;
  instrument?: string;
  accountLabels?: string;
  /** trigger（默认，四类触发）/ exit（六类出场）/ all。 */
  category?: string;
  /** 显式事件类型，给出时覆盖 category。 */
  events?: string;
  /** open / blocked / 具体 gate_kind。 */
  result?: string;
  strength?: string;
  configVersions?: string;
  variants?: string;
  order?: string;
}

export function fetchSignals(params: SignalListParams): Promise<PageResult<SignalEvent>> {
  return get<PageResult<SignalEvent>>("/argus-event/signals", { ...params });
}

export function fetchSignalDetail(eventId: number): Promise<SignalDetail> {
  return get<SignalDetail>(`/argus-event/signals/${eventId}`);
}

export interface GateStatsParams {
  instanceKey?: string;
  instrument?: string;
  accountLabel?: string;
  start?: string;
  end?: string;
}

export function fetchGateStats(params: GateStatsParams): Promise<GateStats> {
  return get<GateStats>("/argus-event/gate-stats", { ...params });
}

export interface EpisodeListParams {
  pageIndex: number;
  pageSize: number;
  instanceKey?: string;
  start?: string;
  end?: string;
  /** opened（默认，决策时刻口径）/ closed / overlap。同一批持仓三种口径条数不同。 */
  timeField?: string;
  instrument?: string;
  accountLabels?: string;
  exitKinds?: string;
  status?: string;
  /** 1 = 只看计入策略胜率的持仓。 */
  strategyOnly?: number;
  order?: string;
}

export function fetchEpisodes(params: EpisodeListParams): Promise<EpisodeList> {
  return get<EpisodeList>("/argus-event/episodes", { ...params });
}

export function fetchEpisodeDetail(episodeId: number): Promise<EpisodeDetail> {
  return get<EpisodeDetail>(`/argus-event/episodes/${episodeId}`);
}

export interface EpisodeStatsParams {
  instanceKey?: string;
  instrument?: string;
  accountLabel?: string;
  start?: string;
  end?: string;
  timeField?: string;
}

export function fetchEpisodeStats(params: EpisodeStatsParams): Promise<EpisodeStats> {
  return get<EpisodeStats>("/argus-event/episode-stats", { ...params });
}

export function fetchExitKindOptions(instanceKey?: string): Promise<ExitKindOption[]> {
  return get<ExitKindOption[]>("/argus-event/episode-exit-kinds", { instanceKey });
}

export function fetchSliceCompare(params: EpisodeStatsParams): Promise<SliceCompare> {
  return get<SliceCompare>("/argus-event/slice-compare", { ...params });
}
