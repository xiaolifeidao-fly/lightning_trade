"use client";

import { getData, getPage } from "@/utils/axios";

export class NewsSentimentRecord {
  id!: number;
  coinCode = "";
  sentiment = ""; // bullish / bearish / neutral
  score = 0;
  keyEvents: string[] = [];
  riskFlags: string[] = [];
  asOf = "";
  freshness = "";
  summary = "";
  model = "";
  provider = "";
  fetchedTime?: string;
}

export async function fetchLatestNewsSentiments(coinCode?: string, pageSize = 3) {
  return getPage(NewsSentimentRecord, "/news-sentiments", {
    coinCode,
    pageIndex: 1,
    pageSize,
  });
}

// 压力面单个价位：价格 + 强度(0~1) + 原因
export interface PressureLevel {
  price: number;
  strength: number;
  reason: string;
}

export class PressureAnalysisRecord {
  id!: number;
  platformCode = "";
  coinCode = "";
  symbol = "";
  interval = "";
  refPrice = 0;
  bias = ""; // long / short / neutral
  shortPressureLevels: PressureLevel[] = []; // 做空压力位(上方阻力)
  longPressureLevels: PressureLevel[] = []; // 做多压力位(下方支撑)
  keyResistance = 0;
  keySupport = 0;
  summary = "";
  newsSummary = "";
  model = "";
  provider = "";
  analyzedTime?: string;
}

export async function fetchLatestPressureAnalyses(coinCode?: string, pageSize = 3) {
  return getPage(PressureAnalysisRecord, "/pressure-analyses", {
    coinCode,
    pageIndex: 1,
    pageSize,
  });
}

export class TradeOrderRecord {
  id!: number;
  platformCode = "";
  tradeCategory = "";
  tradeType = "";
  orderNo = "";
  userId = 0;
  symbol = "";
  baseCoinCode = "";
  quoteCoinCode = "";
  side = "";
  orderType = "";
  price = 0;
  amount = 0;
  total = 0;
  stopPrice = 0;
  filledAmount = 0;
  filledTotal = 0;
  avgFilledPrice = 0;
  feeCoinCode = "";
  feeAmount = 0;
  status = "";
  timeInForce = "";
  source = "";
  clientOrderId = "";
  submittedTime?: string;
  finishedTime?: string;
  cancelReason = "";
  createdTime?: string;
}

export class TradeDetailRecord {
  id!: number;
  platformCode = "";
  tradeCategory = "";
  tradeType = "";
  userId = 0;
  orderNo = "";
  tradeNo = "";
  symbol = "";
  coinCode = "";
  side = "";
  openDirection = "";
  avgOpenPrice = 0;
  liquidationPrice = 0;
  leverage = 0;
  margin = 0;
  userBalanceOpen = 0;
  price = 0;
  amount = 0;
  total = 0;
  fee = 0;
  pnl = 0;
  pnlRate = 0;
  tradeTime?: string;
  createdTime?: string;
}

export class TradeUserSummaryRecord {
  id!: number;
  userId = 0;
  platformCode = "";
  coinCode = "";
  tradeCategory = "";
  tradeDate = "";
  totalOrders = 0;
  buyOrders = 0;
  sellOrders = 0;
  buyAmount = 0;
  sellAmount = 0;
  buyTotal = 0;
  sellTotal = 0;
  totalFee = 0;
  totalVolume = 0;
  createdTime?: string;
}

export class TradeUserPnlRecord {
  id!: number;
  userId = 0;
  platformCode = "";
  coinCode = "";
  tradeCategory = "";
  tradeDate = "";
  realizedPnl = 0;
  unrealizedPnl = 0;
  totalPnl = 0;
  pnlRate = 0;
  positionAmount = 0;
  positionCost = 0;
  positionValue = 0;
  createdTime?: string;
}

export class TradeAnalysisOption {
  label = "";
  value = "";
}

export class TradeSimulationKlinePoint {
  time = "";
  timestamp = 0;
  openPrice = 0;
  highPrice = 0;
  lowPrice = 0;
  closePrice = 0;
  volume = 0;
}

export class TradeSimulationAIPoint {
  time = ""; // 预测时间：被预测的那根未来 K 线时间
  timestamp = 0;
  createdTime = ""; // 执行时间：本条预测落库的时间
  price = 0;
  predictHigh = 0; // AI 预测的区间最高价(0=未给)
  predictLow = 0; // AI 预测的区间最低价(0=未给)
  invalidation = 0; // 失效价位：方向被证伪的关键价位(0=未给)
  signal = "";
  reason = "";
}

export class TradeSimulationMarker {
  time = ""; // 预测时间：被预测的那根未来 K 线时间
  timestamp = 0;
  createdTime = ""; // 执行时间：本条预测落库的时间
  openTimestamp = 0; // 开盘时间(unix秒)：执行预测那一刻，用作「交易周期」窗口起点
  trend = ""; // AI 预测方向 long/short/neutral
  confidence = 0; // AI 置信度：方向正确的主观概率 0~1
  refPrice = 0; // AI参考开盘价：发起预测时 AI 看盘的收盘价(预测基准价)
  openPrice = 0; // 实际开盘价：AI分析完成后即时采集的真实盘价
  costMs = 0; // AI分析耗时(毫秒)：看盘到检测完成的时间差
  realPrice = 0;
  aiPrice = 0;
  diff = 0;
  diffRate = 0;
  matched = false;
  touched = false; // 区间触达：执行→预测时刻间真实价(高/低)是否曾覆盖预测价
  windowHigh = 0; // 窗口 [执行,预测] 内真实最高价
  windowLow = 0; // 窗口 [执行,预测] 内真实最低价
  predictHigh = 0; // AI 预测的区间最高价(0=未给)
  predictLow = 0; // AI 预测的区间最低价(0=未给)
  invalidation = 0; // 失效价位：方向被证伪的关键价位(0=未给)
  invalidationHit = 0; // 窗口内是否触及失效位 -1未给 0未触发(方向未证伪) 1已触发(方向被证伪)
  bandContain = false; // AI 预测区间是否完整覆盖真实波动
  bandUtil = 0; // 区间利用率=真实波动宽度/预测区间宽度，越接近1越紧致
  label = "";
  reason = "";
}

// 单个预测周期(horizon)的一条预测线及其复核数据。
export class TradeSimulationSeries {
  interval = ""; // 预测周期(horizon)，如 15m/1h/4h/1d
  label = ""; // 显示名，如 "15分钟"
  lastRunTime = "";
  matchCount = 0;
  diffCount = 0;
  touchCount = 0; // 已到期点位中「区间触达」的数量
  avgDiffRate = 0;
  maxDiffRate = 0;
  aiPoints: TradeSimulationAIPoint[] = [];
  markers: TradeSimulationMarker[] = [];
}

export class TradeSimulationAnalysisRecord {
  platformCode = "";
  coinCode = "";
  symbol = "";
  interval = ""; // K线展示周期
  lastRunTime = "";
  matchCount = 0; // 各预测周期已到期点位汇总
  diffCount = 0;
  touchCount = 0; // 各预测周期已到期点位中「区间触达」的汇总
  avgDiffRate = 0;
  maxDiffRate = 0;
  platformOptions: TradeAnalysisOption[] = [];
  coinOptions: TradeAnalysisOption[] = [];
  realKlines: TradeSimulationKlinePoint[] = [];
  series: TradeSimulationSeries[] = []; // 每个预测周期一条预测线，图表叠加展示
}

export interface TradeOrderQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  symbol?: string;
  side?: string;
  status?: string;
  tradeCategory?: string;
  orderNo?: string;
}

export interface TradeDetailQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  symbol?: string;
  orderNo?: string;
  tradeCategory?: string;
}

export interface TradeUserSummaryQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  platformCode?: string;
  coinCode?: string;
  tradeCategory?: string;
  startDate?: string;
  endDate?: string;
}

export interface TradeUserPnlQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  platformCode?: string;
  coinCode?: string;
  tradeCategory?: string;
  startDate?: string;
  endDate?: string;
}

export interface TradeSimulationAnalysisQuery {
  platformCode?: string;
  coinCode?: string;
  interval?: string; // K线展示周期，默认 1m
  limit?: number;
}

export class TradeStrategyBacktestCell {
  takeProfitPct = 0;
  stopLossPct = 0;
  samples = 0;
  tpRate = 0;
  slRate = 0;
  timeoutRate = 0;
  winRate = 0;
  avgWin = 0;
  avgLoss = 0;
  payoff = 0;
  expectancy = 0;
  expectancyRoe = 0;
  profitFactor = 0;
  totalReturn = 0;
  maxDrawdown = 0;
}

export class TradeStrategyBacktestRecord {
  platformCode = "";
  coinCode = "";
  symbol = "";
  interval = "";
  holdBars = 1;
  minConfidence = 0;
  minMovePct = 0;
  takerFeeRate = 0;
  fundingRate = 0;
  leverage = 1;
  costPerTrade = 0;
  rangeStart = "";
  rangeEnd = "";
  totalPredictions = 0;
  qualifiedSignals = 0;
  directionAccuracy = 0;
  avgPredictMovePct = 0;
  tpPercents: number[] = [];
  slPercents: number[] = [];
  cells: TradeStrategyBacktestCell[] = [];
  best: TradeStrategyBacktestCell | null = null;
  platformOptions: TradeAnalysisOption[] = [];
  coinOptions: TradeAnalysisOption[] = [];
}

export interface TradeStrategyBacktestQuery {
  platformCode?: string;
  coinCode?: string;
  interval?: string;
  limit?: number;
  holdBars?: number;
  minConfidence?: number;
  minMovePct?: number;
  takerFeeRate?: number;
  fundingRate?: number;
  leverage?: number;
  tpList?: string;
  slList?: string;
}

export async function fetchTradeOrders(query: TradeOrderQuery) {
  return getPage(TradeOrderRecord, "/trade-orders", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    symbol: query.symbol,
    side: query.side,
    status: query.status,
    tradeCategory: query.tradeCategory,
    orderNo: query.orderNo,
  });
}

export async function fetchTradeDetails(query: TradeDetailQuery) {
  return getPage(TradeDetailRecord, "/trade-details", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    symbol: query.symbol,
    orderNo: query.orderNo,
    tradeCategory: query.tradeCategory,
  });
}

export async function fetchUserSummary(query: TradeUserSummaryQuery) {
  return getPage(TradeUserSummaryRecord, "/trade-user-summary", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    tradeCategory: query.tradeCategory,
    startDate: query.startDate,
    endDate: query.endDate,
  });
}

export async function fetchUserPnl(query: TradeUserPnlQuery) {
  return getPage(TradeUserPnlRecord, "/trade-user-pnl", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    tradeCategory: query.tradeCategory,
    startDate: query.startDate,
    endDate: query.endDate,
  });
}

export async function fetchTradeSimulationAnalysis(query: TradeSimulationAnalysisQuery) {
  return getData(TradeSimulationAnalysisRecord, "/trade-simulation-analysis", {
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    interval: query.interval,
    limit: query.limit,
  });
}

export async function fetchTradeStrategyBacktest(query: TradeStrategyBacktestQuery) {
  return getData(TradeStrategyBacktestRecord, "/trade-strategy-backtest", {
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    interval: query.interval,
    limit: query.limit,
    holdBars: query.holdBars,
    minConfidence: query.minConfidence,
    minMovePct: query.minMovePct,
    takerFeeRate: query.takerFeeRate,
    fundingRate: query.fundingRate,
    leverage: query.leverage,
    tpList: query.tpList,
    slList: query.slList,
  });
}
