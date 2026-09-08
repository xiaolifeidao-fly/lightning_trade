"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

/**
 * 盘口信号回测页（r14）的接口封装。
 *
 * 读写两侧都落在 manager-api 的 `/backtest/signal-*` 上：单跑走 r8、批量扫描与
 * 对比矩阵走 r11。本页**不新增任何后端接口**——r8/r11 已经把「精度分层」「跨精度
 * 禁止混排」「基线从哪来」这三件事定死在 DTO 里了，前端只负责如实展示。
 *
 * ⚠️ 时间口径：`startTime` / `endTime` 是**本地墙钟串** `YYYY-MM-DD HH:mm:ss`，
 * 与 `strategy_event.ts`、logs/ 下的 JSONL 和 Telegram 消息逐字一致，不做时区换算
 * （服务端 `parseSignalWindowTime` 明确按 `time.Local` 解析）。
 *
 * ⚠️ 精度等级（`fidelity`）与警示文案（`fidelityNote`）一律取服务端返回值，前端
 * 不自己判定：判定规则是「本组阈值 ≠ 基线阈值就降为频率级」，写在引擎里，前端再
 * 抄一份只会在规则变化时静默漂移。
 */

type QueryParams = Record<string, string | number | undefined>;

async function get<T>(url: string, params?: QueryParams): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

async function post<T>(url: string, body: unknown): Promise<T> {
  const response = await instance.post<ApiResponse<T>>(url, body);
  return unwrapApiResponse(response.data);
}

// ─── 信号源 ──────────────────────────────────────────────────────────────────

/** 一个可回测的信号源 = 实例 × 账户 × 合约，附已入库的覆盖区间。 */
export class SignalSource {
  declare instanceKey: string;
  declare accountLabel: string;
  declare variant: string;
  declare instrument: string;
  declare eventCount: number;
  /** RFC3339，服务端是 time.Time 直出。 */
  declare firstTs: string;
  declare lastTs: string;
}

export class SignalSourceList {
  declare sources: SignalSource[];
  /** 频率级回测的候选阈值（bp），与生产 dev_sampler 的 DevSampleThresholdsBp 同源。 */
  declare thresholdCandidates: number[];
}

export function fetchSignalSources(): Promise<SignalSourceList> {
  return get<SignalSourceList>("/backtest/signal-sources");
}

// ─── 基线 ────────────────────────────────────────────────────────────────────

export class SignalBaseline {
  /** instance_published（该实例已发布配置版本）/ request（请求显式给定）。 */
  declare source: string;
  /** 键名即回测参数键（signal.Params 的 JSON 形态）。 */
  declare params: Record<string, number | string>;
  /**
   * 必须与参数一起显示：配置面收敛（r5）没做完之前，基线里有一部分字段是按实盘
   * 缺省兜底的，不说清楚就会被当成生产事实读。
   */
  declare notes: string[];
  /** 确实从 DB 取到值的配置键。 */
  declare fromDb: string[];
}

export function fetchSignalBaseline(params: {
  instanceKey: string;
  accountLabel: string;
  symbol?: string;
}): Promise<SignalBaseline> {
  return get<SignalBaseline>("/backtest/signal-baseline", { ...params });
}

// ─── 参数旋钮 ────────────────────────────────────────────────────────────────

/**
 * 一组参数旋钮。全部可选：**没给的键沿用基线**（= 所选实例当前生产参数），
 * 而不是回落代码缺省——这样「cap 15/26/40 三组」的 diff 里就只有 cap 一行。
 */
export interface SignalBacktestParams {
  mode?: string;
  evalMode?: string;
  entryPx?: string;
  orderSize?: number;
  riskEquity?: number;
  capOverride?: number;
  budgetPct?: number;
  catastropheStopPct?: number;
  ceiling?: number;
  catastropheOvershootRoiPts?: number;
  gateMinProfitPct?: number;
  trendGateWindowHours?: number;
  trendGateThresholdPct?: number;
  tierSmallRatio?: number;
  tierLargeRatio?: number;
  smallActivatePct?: number;
  smallGiveback?: number;
  mediumActivatePct?: number;
  mediumGiveback?: number;
  largeActivatePct?: number;
  largeGiveback?: number;
  takerFee?: number;
  /** 与 baselineThresholdBp 不等时，本组精度自动降为频率级。 */
  signalThresholdBp?: number;
  baselineThresholdBp?: number;
}

export interface SignalBacktestGroup extends SignalBacktestParams {
  /** 留空时服务端按 diff 自动生成（如 `capOverride=26`）。 */
  label?: string;
}

export interface CreateSignalBatchPayload {
  name?: string;
  platformCode?: string;
  coinCode?: string;
  symbol?: string;
  startTime: string;
  endTime: string;
  instanceKey: string;
  accountLabel: string;
  concurrency?: number;
  includeBaselineRun?: boolean;
  groups: SignalBacktestGroup[];
}

export function createSignalBatch(payload: CreateSignalBatchPayload): Promise<{ batchId: number }> {
  return post<{ batchId: number }>("/backtest/signal-batches", payload);
}

// ─── 批次与对比矩阵 ──────────────────────────────────────────────────────────

export class SignalBatch {
  declare id: number;
  declare name: string;
  declare instanceKey: string;
  declare accountLabel: string;
  declare platformCode: string;
  declare coinCode: string;
  declare symbol: string;
  declare startTime: string;
  declare endTime: string;
  /** pending / running / done / partial / failed。 */
  declare status: string;
  declare errorMsg: string;
  declare concurrency: number;
  declare groupCount: number;
  declare doneCount: number;
  declare failedCount: number;
  declare baselineRunId: number;
  declare baselineSource: string;
  declare createdTime: string;
}

export class SignalBatchList {
  declare total: number;
  declare list: SignalBatch[];
}

export class BacktestMetric {
  declare runId: number;
  declare calcMode: string;
  declare tradeCount: number;
  declare fillCount: number;
  declare expiredCount: number;
  declare fillRate: number;
  declare winCount: number;
  declare winRate: number;
  declare grossPnl: number;
  declare feeTotal: number;
  declare netPnl: number;
  declare expectancy: number;
  declare profitFactor: number;
  declare maxDrawdown: number;
  declare sharpe: number;
  declare avgHoldSecs: number;
  declare tpCount: number;
  declare slCount: number;
  declare trailCount: number;
  declare timeoutCount: number;
  declare fidelity: string;
  declare fidelityNote: string;
  declare signalCount: number;
  declare signalDropped: number;
  declare signalFiltered: number;
  declare capSkipCount: number;
  declare gateSkipCount: number;
  declare trendSkipCount: number;
  declare reduceCount: number;
  declare reduceCloseCount: number;
  declare eodOpenCount: number;
  declare maxStack: number;
  declare capEffective: number;
  declare realizedPnl: number;
  declare floatingPnl: number;
  declare maxDrawdownPct: number;
  /** 频率级组的主产出：λ(θ)。这类组不产 PnL，也不能按净利与别的组比大小。 */
  declare lambdaPerDay: number;
  declare lambdaRatio: number;
  declare lambdaSelfTest: number;
}

export class ParamDiffRow {
  /** 生产配置键口径，如 position.risk.max_contracts_ceiling。 */
  declare key: string;
  /** 回测参数字段名，如 ceiling。 */
  declare field: string;
  declare baseline: string;
  declare value: string;
  declare delta: number;
  declare numeric: boolean;
}

export class MetricDiff {
  declare netPnl: number;
  declare netPnlPct: number;
  declare winRate: number;
  declare profitFactor: number;
  declare maxDrawdown: number;
  declare sharpe: number;
  declare tradeCount: number;
  declare trailCount: number;
  declare slCount: number;
  declare reduceClose: number;
  declare eodOpen: number;
  declare maxStack: number;
}

export class ComparisonRow {
  declare runId: number;
  declare groupLabel: string;
  declare isBaseline: boolean;
  declare status: string;
  declare errorMsg: string;
  declare fidelity: string;
  declare paramDiff: ParamDiffRow[];
  declare metric: BacktestMetric | null;
  /** 仅在与基线**同精度等级**且两侧都有指标时非空。 */
  declare metricDiff: MetricDiff | null;
  /** 没有 metricDiff 时说明原因，空串表示有 diff。 */
  declare diffBlockedReason: string;
  /** false = 本组只产 λ 不产 PnL，不能按净利排序。 */
  declare pnlAvailable: boolean;
}

export class FidelityGroup {
  declare fidelity: string;
  declare fidelityLabel: string;
  declare comparableToBaseline: boolean;
  /** netPnl（有 PnL）或 lambdaPerDay（只有 λ 的频率级组）。 */
  declare sortedBy: string;
  declare notes: string[];
  declare rows: ComparisonRow[];
}

export class SignalBatchDetail {
  declare batch: SignalBatch;
  declare baseline: SignalBaseline;
  /** 按精度等级分组。接口层从不返回跨精度的全局榜，前端也就没法误排。 */
  declare groups: FidelityGroup[];
  declare warnings: string[];
}

export function fetchSignalBatches(params: {
  page: number;
  pageSize: number;
  instanceKey?: string;
  accountLabel?: string;
}): Promise<SignalBatchList> {
  return get<SignalBatchList>("/backtest/signal-batches", { ...params });
}

export function fetchSignalBatchDetail(id: number): Promise<SignalBatchDetail> {
  return get<SignalBatchDetail>(`/backtest/signal-batches/${id}`);
}

// ─── 单组逐笔 ────────────────────────────────────────────────────────────────

export class BacktestRun {
  declare id: number;
  declare name: string;
  declare platformCode: string;
  declare symbol: string;
  declare startTime: string;
  declare endTime: string;
  declare paramsSnapshot: string;
  declare status: string;
  declare errorMsg: string;
  declare klineCount: number;
  declare klineStart: string;
  declare klineEnd: string;
  declare engineKind: string;
  declare instanceKey: string;
  declare accountLabel: string;
  declare signalSource: string;
  declare fidelity: string;
  declare fidelityNote: string;
  declare signalCount: number;
}

/** 盘口信号回测里，一行 = 一个持仓生命周期（不是一次触发）。 */
export class BacktestTrade {
  declare id: number;
  declare calcMode: string;
  declare direction: string;
  declare status: string;
  declare openPrice: number;
  declare closePrice: number;
  declare closeReason: string;
  declare openedAt: string;
  declare closedAt: string;
  declare pnl: number;
  declare netPnl: number;
  declare netPnlRate: number;
  declare pnlRate: number;
  declare fee: number;
  declare contracts: number;
  declare maxContracts: number;
  declare addCount: number;
  /** 移动止盈峰值 ROI%。回测用 1m high 算，**偏高**（实盘判定频率是 5 秒）。 */
  declare peakPct: number;
  declare reducedPnl: number;
  declare unrealizedNetPnl: number;
}

export class BacktestRunDetail {
  declare run: BacktestRun;
  declare metrics: BacktestMetric[];
  declare trades: BacktestTrade[];
}

export function fetchBacktestRunDetail(id: number): Promise<BacktestRunDetail> {
  return get<BacktestRunDetail>(`/backtest/runs/${id}`);
}
