"use client";

import { getData, getPage, instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export class UserRecord {
  id!: number;

  name = "";

  username = "";

  email = "";

  phone = "";

  department = "";

  role = "member";

  status = "active";

  remark = "";

  tineUserId = "";

  tineNickname = "";

  tineBalance?: number;

  tineCurrency = "CNY";

  password = "";

  originPassword = "";

  secretKey = "";

  pubToken = "";

  banCount = 0;

  accountId?: number;

  accountStatus = "";

  balanceAmount = "0";

  lastLoginTime?: string;

  updatedTime?: string;

  createdTime?: string;
}

export class UserStats {
  visibleUsers = 0;

  accountCount = 0;

  privilegedUsers = 0;

  recentLoginUsers = 0;

  activeUsers = 0;
}

export interface UserListQuery {
  pageIndex?: number;
  pageSize?: number;
  search?: string;
  role?: string;
  status?: string;
}

export interface UserPayload {
  name: string;
  username: string;
  email?: string;
  phone?: string;
  department?: string;
  role: string;
  status: string;
  remark?: string;
  tineUserId?: string;
  tineNickname?: string;
  tineBalance?: number;
  tineCurrency?: string;
  password?: string;
  originPassword?: string;
  secretKey?: string;
  pubToken?: string;
  banCount?: number;
}

export async function fetchUsers(query: UserListQuery) {
  return getPage(UserRecord, "/users", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    search: query.search,
    role: query.role,
    status: query.status,
  });
}

export async function fetchUserStats() {
  return getData(UserStats, "/users/stats");
}

export async function fetchUserDetail(id: number) {
  return getData(UserRecord, `/users/${id}`);
}

export async function createUser(payload: UserPayload) {
  const response = await instance.post<ApiResponse<UserRecord>>("/users", payload);
  return unwrapApiResponse(response.data);
}

export async function updateUser(id: number, payload: Partial<UserPayload>) {
  const response = await instance.put<ApiResponse<UserRecord>>(`/users/${id}`, payload);
  return unwrapApiResponse(response.data);
}

export async function deleteUser(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/users/${id}`);
  return unwrapApiResponse(response.data);
}

export interface AccountPayload {
  userId: number;
  accountStatus?: string;
  balanceAmount?: string;
}

export async function createAccount(payload: AccountPayload) {
  const response = await instance.post<ApiResponse<{ id: number }>>("/accounts", payload);
  return unwrapApiResponse(response.data);
}

export async function updateAccount(id: number, payload: Partial<AccountPayload>) {
  const response = await instance.put<ApiResponse<{ id: number }>>(`/accounts/${id}`, payload);
  return unwrapApiResponse(response.data);
}
