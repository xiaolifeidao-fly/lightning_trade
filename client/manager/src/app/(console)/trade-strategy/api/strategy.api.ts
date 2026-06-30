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
  holdDuration: number;
  maxHoldDuration: number;
  takeProfitPct: number;
  stopLossPct: number;
  // 止盈止损来源(三选一) percent(离入场价%)/predict(跟AI预测)/pressure(跟AI压力面)
  takeProfitSource: string;
  stopLossSource: string;
  predictSlBufferPct: number; // predict止损：失效价缓冲%
  pressureBufferPct: number; // pressure止盈/止损：结构位缓冲%
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
  holdDuration?: string; // 秒或 "4h"
  maxHoldDuration?: string;
  takeProfitPct?: number;
  stopLossPct?: number;
  takeProfitSource?: string;
  stopLossSource?: string;
  predictSlBufferPct?: number;
  pressureBufferPct?: number;
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
