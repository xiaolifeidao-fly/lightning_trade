"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export interface LoginPayload {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
}

export async function login(payload: LoginPayload) {
  const response = await instance.post<ApiResponse<LoginResponse>>("/login", payload);
  return unwrapApiResponse(response.data);
}
