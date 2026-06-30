"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

// 回测任务
export interface BacktestRun {
  id: number;
  name: string;
  platformCode: string;
  coinCode: string;
  symbol: string;
  predictionInterval: string;
  predictionVariant: string;
  priceInterval: string;
  priceSource: string;
  tradingPeriod: string; // 1h/4h/8h/12h/1d，空=仅按预测周期
  startTime: string;
  endTime: string;
  strategyId: number;
  paramsSnapshot: string;
  status: string; // pending / running / done / failed
  errorMsg: string;
  createdTime: string;
  klineCount: number; // 回放使用的K线根数
  klineStart: string; // 实际K线起始时间
  klineEnd: string; // 实际K线结束时间
}

// 回测汇总指标
export interface BacktestMetric {
  runId: number;
  calcMode: string; // prediction / trading
  tradeCount: number;
  fillCount: number;
  expiredCount: number;
  fillRate: number;
  winCount: number;
  winRate: number;
  grossPnl: number;
  feeTotal: number;
  netPnl: number;
  expectancy: number;
  profitFactor: number;
  maxDrawdown: number;
  sharpe: number;
  avgHoldSecs: number;
  tpCount: number;
  slCount: number;
  timeoutCount: number;
}

// 回测逐笔
export interface BacktestTrade {
  id: number;
  predictionId: number;
  calcMode: string; // prediction / trading
  predictTime: string; // 预测目标时刻(关联预测 predict_time)，与 requestedAt 框定预测周期
  direction: string;
  entryMode: string;
  plannedEntryPrice: number;
  takeProfitPrice: number;
  stopLossPrice: number;
  status: string; // open / closed / expired
  openPrice: number;
  closePrice: number;
  closeReason: string;
  requestedAt: string;
  openedAt: string;
  closedAt: string;
  pnl: number;
  pnlRate: number; // 盈亏率%(含杠杆)
  netPnl: number;
  fee: number;
  confidence: number;
  efficiency: number;
  predHigh: number; // 预测区间上沿
  predLow: number; // 预测区间下沿
  predClose: number; // 预测收盘价
  windowOpen: number; // 信号后窗口实际开盘价
  windowClose: number; // 信号后窗口实际收盘价
  windowLow: number; // 信号后窗口最低价
  windowHigh: number; // 信号后窗口最高价
  pressureHigh: number; // 压力面最高价(关键阻力)
  pressureLow: number; // 压力面最低价(关键支撑)
  maxPriceDuringHold: number; // 持仓期间最高价
  minPriceDuringHold: number; // 持仓期间最低价
  leverage: number; // 杠杆倍数(算含杠杆浮盈用)
  markPrice: number; // 当前最新价(标记价)，仅持仓中(open)有值
  unrealizedPnl: number; // 浮动毛盈亏 USDT
  unrealizedPnlRate: number; // 浮动盈亏率%(含杠杆)
  unrealizedNetPnl: number; // 浮动净盈亏 = 浮动盈亏 - 预估手续费
}

export interface BacktestRunDetail {
  run: BacktestRun;
  metrics: BacktestMetric[]; // 按 calcMode 区分：prediction / trading
  trades: BacktestTrade[];
}

export interface BacktestRunList {
  total: number;
  list: BacktestRun[];
}

// 单根 K 线（K线详情弹窗）
export interface KlinePoint {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

// 一根「预测 K 线」：由一条 AI 预测构造(开=参考价 收=预测价 高/低=预测极值)。
export interface PredictionCandle {
  openTime: string;
  closeTime: string;
  open: number;
  high: number;
  low: number;
  close: number;
  trend: string;
  confidence: number;
}

export interface PredictionSeries {
  interval: string;
  candles: PredictionCandle[];
}

// 复合方向某高周期一行。
export interface CompositeRow {
  interval: string;
  direction: string;
  confidence: number;
  predLow: number; // 预测区间最低
  predHigh: number; // 预测区间最高
  favorableExtreme: number;
  profitPct: number;
  score: number;
  dominant: boolean;
  hasData: boolean;
  predictTime: string;
}

export interface CompositeDirection {
  entry: number;
  ownInterval: string;
  ownDirection: string;
  recommendedDirection: string;
  dominantInterval: string;
  agree: boolean;
  rows: CompositeRow[];
}

// 「K线详情」预测增强：复合方向 + 自身周期预测K线 + 高周期预测K线。
export interface PredictionDetail {
  composite: CompositeDirection;
  ownSeries: PredictionSeries;
  higherSeries: PredictionSeries[];
}

// 策略下拉项（复用策略列表接口）
export interface StrategyOption {
  id: number;
  platformCode: string;
  coinCode: string;
  symbol: string;
  interval: string;
  remark: string;
}

export interface StrategyList {
  total: number;
  list: StrategyOption[];
}

export interface CreateBacktestRunPayload {
  name?: string;
  platformCode: string;
  coinCode: string;
  symbol: string;
  predictionInterval: string;
  predictionVariant?: string;
  priceInterval?: string;
  priceSource?: string;
  tradingPeriod?: string;
  startTime: string;
  endTime: string;
  strategyId: number;
}

// 新建回测任务，返回 runId（后台异步执行，前端轮询 status）。
export async function createBacktestRun(payload: CreateBacktestRunPayload): Promise<{ runId: number }> {
  const res = await instance.post<ApiResponse<{ runId: number }>>("/backtest/runs", payload);
  return unwrapApiResponse(res.data);
}

// 回测任务列表（分页）。
export async function fetchBacktestRuns(params: {
  page?: number;
  pageSize?: number;
  symbol?: string;
  strategyId?: number;
}): Promise<BacktestRunList> {
  const res = await instance.get<ApiResponse<BacktestRunList>>("/backtest/runs", { params });
  return unwrapApiResponse(res.data);
}

// 单次回测详情：任务 + 汇总指标 + 逐笔。
export async function fetchBacktestRunDetail(id: number): Promise<BacktestRunDetail> {
  const res = await instance.get<ApiResponse<BacktestRunDetail>>(`/backtest/runs/${id}`);
  return unwrapApiResponse(res.data);
}

// 拉取某 symbol+interval 在时间区间内的 K 线（K线详情弹窗）。
export async function fetchKlineRange(params: {
  symbol: string;
  interval: string;
  start: string;
  end: string;
}): Promise<KlinePoint[]> {
  const res = await instance.get<ApiResponse<KlinePoint[]>>("/klines/range", { params });
  return unwrapApiResponse(res.data);
}

// 拉取某笔「K线详情」的预测增强：复合方向 + 预测周期 K 线。
export async function fetchPredictionDetail(params: {
  platform: string;
  coin: string;
  interval: string;
  signal: string;
  start: string;
  end: string;
  entry: number;
}): Promise<PredictionDetail> {
  const res = await instance.get<ApiResponse<PredictionDetail>>("/backtest/prediction-detail", {
    params: { ...params, entry: String(params.entry) },
  });
  return unwrapApiResponse(res.data);
}

// 多 run 指标横向对比。
export async function fetchBacktestMetrics(runIds: number[]): Promise<BacktestMetric[]> {
  const res = await instance.get<ApiResponse<BacktestMetric[]>>("/backtest/metrics", {
    params: { runIds: runIds.join(",") },
  });
  return unwrapApiResponse(res.data);
}

// 策略下拉数据（取启用的策略）。
export async function fetchStrategyOptions(params: {
  platformCode?: string;
  coinCode?: string;
  symbol?: string;
}): Promise<StrategyList> {
  const res = await instance.get<ApiResponse<StrategyList>>("/strategies", {
    params: { ...params, page: 1, pageSize: 200 },
  });
  return unwrapApiResponse(res.data);
}
