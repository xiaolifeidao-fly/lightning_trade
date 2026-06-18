"use client";

import { getPage, instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export class PlatformRecord {
  id!: number;
  code = "";
  name = "";
  fullName = "";
  icon = "";
  website = "";
  country = "";
  apiBaseUrl = "";
  wsBaseUrl = "";
  docsUrl = "";
  supportedTypes = "";
  defaultFeeRate = 0;
  rateLimitPerSec = 0;
  status = "online";
  sortOrder = 0;
  description = "";
  createdTime?: string;
  updatedTime?: string;
}

export class PlatformCoinRecord {
  id!: number;
  platformId = 0;
  platformCode = "";
  coinId = 0;
  coinCode = "";
  platformSymbol = "";
  chainName = "";
  contractAddress = "";
  depositEnable = 1;
  withdrawEnable = 1;
  tradeEnable = 1;
  minWithdrawal = 0;
  withdrawalFee = 0;
  confirmations = 0;
  createdTime?: string;
  updatedTime?: string;
}

export class PlatformAccountRecord {
  id!: number;
  platformId = 0;
  platformCode = "";
  accountName = "";
  accountType = "";
  apiKey = "";
  ipWhitelist = "";
  permissions = "";
  status = "active";
  lastUsedTime?: string;
  expireTime?: string;
  remark = "";
  createdTime?: string;
  updatedTime?: string;
}

export interface PlatformListQuery {
  pageIndex?: number;
  pageSize?: number;
  code?: string;
  name?: string;
  country?: string;
  status?: string;
}

export interface PlatformPayload {
  code?: string;
  name?: string;
  fullName?: string;
  icon?: string;
  website?: string;
  country?: string;
  apiBaseUrl?: string;
  wsBaseUrl?: string;
  docsUrl?: string;
  supportedTypes?: string;
  defaultFeeRate?: number;
  rateLimitPerSec?: number;
  status?: string;
  sortOrder?: number;
  description?: string;
}

export interface PlatformCoinPayload {
  platformId?: number;
  platformCode?: string;
  coinId?: number;
  coinCode?: string;
  platformSymbol?: string;
  chainName?: string;
  contractAddress?: string;
  depositEnable?: number;
  withdrawEnable?: number;
  tradeEnable?: number;
  minWithdrawal?: number;
  withdrawalFee?: number;
  confirmations?: number;
}

export interface PlatformCoinListQuery {
  pageIndex?: number;
  pageSize?: number;
  platformId?: number;
  coinId?: number;
  coinCode?: string;
  chainName?: string;
}

export interface PlatformAccountPayload {
  platformId?: number;
  platformCode?: string;
  accountName?: string;
  accountType?: string;
  apiKey?: string;
  apiSecret?: string;
  passphrase?: string;
  ipWhitelist?: string;
  permissions?: string;
  status?: string;
  expireTime?: string;
  remark?: string;
}

export interface PlatformAccountListQuery {
  pageIndex?: number;
  pageSize?: number;
  platformId?: number;
  accountName?: string;
  status?: string;
}

export async function fetchPlatforms(query: PlatformListQuery) {
  return getPage(PlatformRecord, "/coin-platforms", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    code: query.code,
    name: query.name,
    country: query.country,
    status: query.status,
  });
}

export async function createPlatform(payload: PlatformPayload) {
  const response = await instance.post<ApiResponse<PlatformRecord>>("/coin-platforms", payload);
  return unwrapApiResponse(response.data);
}

export async function updatePlatform(id: number, payload: Partial<PlatformPayload>) {
  const response = await instance.put<ApiResponse<PlatformRecord>>(`/coin-platforms/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deletePlatform(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coin-platforms/${id}`);
  return unwrapApiResponse(response.data);
}

export async function fetchPlatformCoins(query: PlatformCoinListQuery = {}) {
  return getPage(PlatformCoinRecord, "/coin-platform-coins", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    platformId: query.platformId,
    coinId: query.coinId,
    coinCode: query.coinCode,
    chainName: query.chainName,
  });
}

export async function upsertPlatformCoin(payload: PlatformCoinPayload) {
  const response = await instance.post<ApiResponse<PlatformCoinRecord>>("/coin-platform-coins", payload);
  return unwrapApiResponse(response.data);
}

export async function updatePlatformCoin(id: number, payload: Partial<PlatformCoinPayload>) {
  const response = await instance.put<ApiResponse<PlatformCoinRecord>>(`/coin-platform-coins/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deletePlatformCoin(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coin-platform-coins/${id}`);
  return unwrapApiResponse(response.data);
}

export async function fetchPlatformAccounts(query: PlatformAccountListQuery = {}) {
  return getPage(PlatformAccountRecord, "/coin-platform-accounts", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    platformId: query.platformId,
    accountName: query.accountName,
    status: query.status,
  });
}

export async function createPlatformAccount(payload: PlatformAccountPayload) {
  const response = await instance.post<ApiResponse<PlatformAccountRecord>>("/coin-platform-accounts", payload);
  return unwrapApiResponse(response.data);
}

export async function updatePlatformAccount(id: number, payload: Partial<PlatformAccountPayload>) {
  const response = await instance.put<ApiResponse<PlatformAccountRecord>>(`/coin-platform-accounts/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deletePlatformAccount(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coin-platform-accounts/${id}`);
  return unwrapApiResponse(response.data);
}
