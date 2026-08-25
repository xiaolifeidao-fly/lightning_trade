"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export interface Strategy {
  id: number;
  platformCode: string;
  coinCode: string;
  symbol: string;
  interval: string;
  enabled: number;
  minConfidence: number;
  minMovePct: number;
  trendFilter: string;
  maxOpenPositions: number;
  requireCompositeDir: number; // 复合方向门槛 1启用 0不启用
  holdDuration: number;
  maxHoldDuration: number;
  tradingPeriod: string; // 交易周期 1h/4h/12h/1d/1w，空=不启用
  takeProfitPct: number;
  stopLossPct: number;
  // 止盈止损来源(三选一) percent(离入场价%)/predict(跟AI预测)/pressure(跟AI压力面)
  takeProfitSource: string;
  stopLossSource: string;
  predictSlBufferPct: number; // predict止损：失效价缓冲%
  pressureBufferPct: number; // pressure止盈/止损：结构位缓冲%
  // 移动止盈(峰值回撤+时间收敛)：0=不启用，退回静态止盈
  trailActivatePct: number; // 激活阈值：浮盈ROI%(含杠杆)
  trailGiveback: number; // 峰值回撤比例r0(0~1)
  trailGivebackMin: number; // 周期末回撤比例(时间收敛)，<=0或≥r0=不收敛
  // 早段疲软离场：前X%时间内峰值浮盈<Y%(含杠杆)则市价平仓。0=不启用
  earlyCutTimePct: number; // 触发时间点%(0~100)
  earlyCutMinProfitPct: number; // 利润门槛ROI%(含杠杆)
  // 早段逆行离场(MAE 软止损)：从未走出浮盈且逆行浮亏达阈值时先于硬止损离场。0=不启用
  earlyCutMaxAdversePct: number; // 逆行止损阈值ROI%(含杠杆)
  earlyCutArmProfitPct: number; // 解除阈值ROI%(含杠杆)，<=0=始终武装
  leverage: number;
  contracts: number;
  makerFeeRate: number;
  takerFeeRate: number;
  // 入场策略(状态机)
  entryMode: string;
  entryAlpha: number;
  exitGamma: number;
  entryTtl: number;
  efficiencyRoute: number;
  predictionVariant: string;
  remark: string;
  createdTime: string;
  updatedTime: string;
}

export interface StrategyList {
  total: number;
  list: Strategy[];
}

export interface StrategyPayload {
  platformCode: string;
  coinCode: string;
  symbol: string;
  interval: string;
  enabled?: number;
  minConfidence?: number;
  minMovePct?: number;
  trendFilter?: string;
  maxOpenPositions?: number;
  requireCompositeDir?: number; // 1启用 0不启用
  holdDuration?: string; // 秒或 "4h"
  maxHoldDuration?: string;
  tradingPeriod?: string; // 1h/4h/12h/1d/1w，空=不启用
  takeProfitPct?: number;
  stopLossPct?: number;
  takeProfitSource?: string;
  stopLossSource?: string;
  predictSlBufferPct?: number;
  pressureBufferPct?: number;
  trailActivatePct?: number;
  trailGiveback?: number;
  trailGivebackMin?: number;
  earlyCutTimePct?: number;
  earlyCutMinProfitPct?: number;
  earlyCutMaxAdversePct?: number;
  earlyCutArmProfitPct?: number;
  leverage?: number;
  contracts?: number;
  makerFeeRate?: number;
  takerFeeRate?: number;
  entryMode?: string;
  entryAlpha?: number;
  exitGamma?: number;
  entryTtl?: number;
  efficiencyRoute?: number;
  predictionVariant?: string;
  remark?: string;
}

export async function fetchStrategies(params: {
  page?: number;
  pageSize?: number;
  platformCode?: string;
  coinCode?: string;
  symbol?: string;
  interval?: string;
}): Promise<StrategyList> {
  const res = await instance.get<ApiResponse<StrategyList>>("/strategies", { params });
  return unwrapApiResponse(res.data);
}

export async function createStrategy(payload: StrategyPayload): Promise<Strategy> {
  const res = await instance.post<ApiResponse<Strategy>>("/strategies", payload);
  return unwrapApiResponse(res.data);
}

export async function updateStrategy(id: number, payload: Partial<StrategyPayload>): Promise<{ id: number }> {
  const res = await instance.put<ApiResponse<{ id: number }>>(`/strategies/${id}`, payload);
  return unwrapApiResponse(res.data);
}

export async function deleteStrategy(id: number): Promise<{ id: number }> {
  const res = await instance.delete<ApiResponse<{ id: number }>>(`/strategies/${id}`);
  return unwrapApiResponse(res.data);
}
