"use client";

import { getPage, instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export class CoinRecord {
  id!: number;
  code = "";
  name = "";
  fullName = "";
  icon = "";
  chainName = "";
  contractAddress = "";
  decimals = 8;
  pricePrecision = 8;
  amountPrecision = 8;
  minWithdrawal = 0;
  maxWithdrawal = 0;
  withdrawalFee = 0;
  depositEnable = 1;
  withdrawEnable = 1;
  tradeEnable = 1;
  status = "online";
  sortOrder = 0;
  description = "";
  createdTime?: string;
  updatedTime?: string;
}

export class CoinPairRecord {
  id!: number;
  symbol = "";
  baseCoinCode = "";
  quoteCoinCode = "";
  pricePrecision = 8;
  amountPrecision = 8;
  minAmount = 0;
  maxAmount = 0;
  minTotal = 0;
  takerFeeRate = 0;
  makerFeeRate = 0;
  status = "online";
  sortOrder = 0;
  createdTime?: string;
  updatedTime?: string;
}

export interface CoinListQuery {
  pageIndex?: number;
  pageSize?: number;
  code?: string;
  name?: string;
  chainName?: string;
  status?: string;
}

export interface CoinPayload {
  code?: string;
  name?: string;
  fullName?: string;
  icon?: string;
  chainName?: string;
  contractAddress?: string;
  decimals?: number;
  pricePrecision?: number;
  amountPrecision?: number;
  minWithdrawal?: number;
  maxWithdrawal?: number;
  withdrawalFee?: number;
  depositEnable?: number;
  withdrawEnable?: number;
  tradeEnable?: number;
  status?: string;
  sortOrder?: number;
  description?: string;
}

export interface CoinPairListQuery {
  pageIndex?: number;
  pageSize?: number;
  symbol?: string;
  baseCoinCode?: string;
  quoteCoinCode?: string;
  status?: string;
}

export interface CoinPairPayload {
  symbol?: string;
  baseCoinCode?: string;
  quoteCoinCode?: string;
  pricePrecision?: number;
  amountPrecision?: number;
  minAmount?: number;
  maxAmount?: number;
  minTotal?: number;
  takerFeeRate?: number;
  makerFeeRate?: number;
  status?: string;
  sortOrder?: number;
}

export async function fetchCoins(query: CoinListQuery) {
  return getPage(CoinRecord, "/coins", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    code: query.code,
    name: query.name,
    chainName: query.chainName,
    status: query.status,
  });
}

export async function createCoin(payload: CoinPayload) {
  const response = await instance.post<ApiResponse<CoinRecord>>("/coins", payload);
  return unwrapApiResponse(response.data);
}

export async function updateCoin(id: number, payload: Partial<CoinPayload>) {
  const response = await instance.put<ApiResponse<CoinRecord>>(`/coins/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deleteCoin(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coins/${id}`);
  return unwrapApiResponse(response.data);
}

export async function fetchCoinPairs(query: CoinPairListQuery) {
  return getPage(CoinPairRecord, "/coin-pairs", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    symbol: query.symbol,
    baseCoinCode: query.baseCoinCode,
    quoteCoinCode: query.quoteCoinCode,
    status: query.status,
  });
}

export async function createCoinPair(payload: CoinPairPayload) {
  const response = await instance.post<ApiResponse<CoinPairRecord>>("/coin-pairs", payload);
  return unwrapApiResponse(response.data);
}

export async function updateCoinPair(id: number, payload: Partial<CoinPairPayload>) {
  const response = await instance.put<ApiResponse<CoinPairRecord>>(`/coin-pairs/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deleteCoinPair(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coin-pairs/${id}`);
  return unwrapApiResponse(response.data);
}
