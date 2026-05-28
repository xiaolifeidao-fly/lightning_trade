"use client";

import { getPage, instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export class CoinUserRecord {
  id!: number;
  platformId = 0;
  platformCode = "";
  username = "";
  nickname = "";
  email = "";
  phone = "";
  balance = 0;
  country = "";
  kycLevel = 0;
  kycStatus = "pending";
  status = "active";
  inviteCode = "";
  inviterId = 0;
  lastLoginIp = "";
  lastLoginTime = "";
  twoFaEnabled = 0;
  remark = "";
  createdTime?: string;
  updatedTime?: string;
}

export class CoinUserAssetRecord {
  id!: number;
  userId = 0;
  coinId = 0;
  coinCode = "";
  available = 0;
  frozen = 0;
  total = 0;
  address = "";
  withdrawEnable = 1;
}

export class CoinUserPositionRecord {
  id!: number;
  userId = 0;
  symbol = "";
  baseCoinCode = "";
  quoteCoinCode = "";
  amount = 0;
  avgCostPrice = 0;
  totalCost = 0;
  status = "open";
  createdTime?: string;
  updatedTime?: string;
}

export class CoinUserPositionAnalysisRecord {
  id!: number;
  userId = 0;
  positionId = 0;
  symbol = "";
  side = "";
  avgPrice = 0;
  liquidationPrice = 0;
  leverage = 0;
  contracts = 0;
  margin = 0;
  balanceAtOpen = 0;
  aiAdvice = "";
  createdTime?: string;
}

export class CoinUserLoginRecord {
  id!: number;
  userId = 0;
  ip = "";
  device = "";
  location = "";
  success = 1;
  createdTime?: string;
}

export interface CoinUserListQuery {
  pageIndex?: number;
  pageSize?: number;
  search?: string;
  platformCode?: string;
  kycStatus?: string;
  status?: string;
}

export interface CoinUserCreatePayload {
  platformId?: number;
  platformCode?: string;
  account?: string;
  username?: string;
  nickname?: string;
  email?: string;
  phone?: string;
  password?: string;
  balance?: number;
  country?: string;
  inviteCode?: string;
  inviterId?: number;
  remark?: string;
}

export interface CoinUserUpdatePayload {
  platformCode?: string;
  balance?: number;
  nickname?: string;
  email?: string;
  phone?: string;
  country?: string;
  kycLevel?: number;
  kycStatus?: string;
  status?: string;
  twoFaEnabled?: number;
  remark?: string;
}

export interface CoinUserAssetPayload {
  userId?: number;
  coinId?: number;
  coinCode?: string;
  address?: string;
  available?: number;
  frozen?: number;
  total?: number;
  withdrawEnable?: number;
}

export interface CoinUserPositionPayload {
  userId?: number;
  symbol?: string;
  baseCoinCode?: string;
  quoteCoinCode?: string;
  amount?: number;
  avgCostPrice?: number;
  totalCost?: number;
  status?: string;
}

export interface CoinUserPositionAnalysisPayload {
  userId?: number;
  positionId?: number;
  symbol?: string;
  side?: string;
  avgPrice?: number;
  liquidationPrice?: number;
  leverage?: number;
  contracts?: number;
  margin?: number;
  balanceAtOpen?: number;
  aiAdvice?: string;
}

export async function fetchCoinUsers(query: CoinUserListQuery) {
  return getPage(CoinUserRecord, "/coin-users", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    search: query.search,
    platformCode: query.platformCode,
    kycStatus: query.kycStatus,
    status: query.status,
  });
}

export async function createCoinUser(payload: CoinUserCreatePayload) {
  const response = await instance.post<ApiResponse<CoinUserRecord>>("/coin-users", payload);
  return unwrapApiResponse(response.data);
}

export async function updateCoinUser(id: number, payload: Partial<CoinUserUpdatePayload>) {
  const response = await instance.put<ApiResponse<CoinUserRecord>>(`/coin-users/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deleteCoinUser(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/coin-users/${id}`);
  return unwrapApiResponse(response.data);
}

export async function fetchUserAssets(userId: number) {
  const response = await instance.get<ApiResponse<CoinUserAssetRecord[]>>(`/coin-users/${userId}/assets`);
  return unwrapApiResponse(response.data);
}

export async function upsertUserAsset(payload: CoinUserAssetPayload) {
  const response = await instance.post<ApiResponse<CoinUserAssetRecord>>("/coin-users/assets", payload);
  return unwrapApiResponse(response.data);
}

export async function updateUserAsset(id: number, payload: Partial<CoinUserAssetPayload>) {
  const response = await instance.put<ApiResponse<CoinUserAssetRecord>>(`/coin-users/assets/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function fetchUserPositions(userId: number) {
  const response = await instance.get<ApiResponse<CoinUserPositionRecord[]>>(`/coin-users/${userId}/positions`);
  return unwrapApiResponse(response.data);
}

export async function upsertUserPosition(payload: CoinUserPositionPayload) {
  const response = await instance.post<ApiResponse<CoinUserPositionRecord>>("/coin-users/positions", payload);
  return unwrapApiResponse(response.data);
}

export async function updateUserPosition(id: number, payload: Partial<CoinUserPositionPayload>) {
  const response = await instance.put<ApiResponse<CoinUserPositionRecord>>(`/coin-users/positions/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function fetchUserPositionAnalysis(userId: number) {
  const response = await instance.get<ApiResponse<CoinUserPositionAnalysisRecord[]>>(`/coin-users/${userId}/position-analysis`);
  return unwrapApiResponse(response.data);
}

export async function createUserPositionAnalysis(payload: CoinUserPositionAnalysisPayload) {
  const response = await instance.post<ApiResponse<CoinUserPositionAnalysisRecord>>("/coin-users/position-analysis", payload);
  return unwrapApiResponse(response.data);
}

export async function updateUserPositionAnalysis(id: number, payload: Partial<CoinUserPositionAnalysisPayload>) {
  const response = await instance.put<ApiResponse<CoinUserPositionAnalysisRecord>>(`/coin-users/position-analysis/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function fetchLoginRecords(userId: number, limit = 20) {
  const response = await instance.get<ApiResponse<CoinUserLoginRecord[]>>(
    `/coin-users/${userId}/login-records`,
    { params: { limit } },
  );
  return unwrapApiResponse(response.data);
}
