"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

export class ArgusConfigVersion {
  declare id: number;
  declare version: number;
  declare status: string;
  declare releaseNote: string;
  declare publishedBy: string;
  declare publishedAt?: string;
  declare snapshotChecksum: string;
}

export class ArgusConfig {
  declare id: number;
  declare serverPort: number;
  declare requestPath: string;
  declare logDir: string;
  declare enabled: number;
  declare tradeEnabled: number;
  declare defaultOrderSize: number;
  declare monitorIntervalSecond: number;
  declare profitThreshold: number;
  declare lossThreshold: number;
  declare loginScheduledEnabled: number;
  declare loginScheduledHour: number;
  declare loginScheduledMinute: number;
  declare sessionMaxAgeDay: number;
  declare extraConfigJson?: string;
  declare aiCloseEnabled: number;
  declare aiCloseProvider: string;
  declare aiCloseApiUrl: string;
  declare aiCloseApiKey?: string;
  declare aiCloseModel: string;
  declare aiCloseTimeoutSecond: number;
  declare aiCloseMaxTokens: number;
  declare aiCloseTemperature: number;
  declare aiCloseIntervalMinute: number;
  declare aiCloseMinInterval: number;
  declare aiCloseMaxInterval: number;
  declare aiOpenEnabled: number;
  declare aiOpenAutoTrade: number;
  declare aiOpenApiUrl: string;
  declare aiOpenApiKey?: string;
  declare aiOpenModel: string;
  declare aiOpenTimeoutSecond: number;
  declare aiOpenMaxTokens: number;
  declare aiOpenTemperature: number;
  declare aiOpenIntervalMinute: number;
  declare aiOpenMinInterval: number;
  declare aiOpenMaxInterval: number;
  declare aiOpenMinLiqDistancePercent: number;
  declare aiOpenMinLiqDistanceUsd: number;
  declare aiOpenMaxBalancePercent: number;
  declare aiOpenMinOrderContracts: number;
  declare aiOpenMaxOrderContracts: number;
  declare aiOpenMaxTotalContracts: number;
  declare aiOpenCooldownMinute: number;
  declare aiOpenLiqSafetyFactor: number;
}

export class ArgusAccount {
  declare id: number;
  declare accountName: string;
  declare platform?: string;
  declare url: string;
  declare uid: string;
  declare loginType: string;
  declare loginHeadless: number;
  declare username: string;
  declare password?: string;
  declare googleAuthKey?: string;
  declare apiKey?: string;
  declare secretKey?: string;
  declare passphrase?: string;
  declare resourceId: string;
  declare positionMode: string;
  declare positionSide: string;
  declare closeStrategy: string;
  declare initialBalance: number;
  declare enabled: number;
}

export class ArgusAccountRisk {
  declare id: number;
  declare accountId: number;
  declare takeProfitMode: string;
  declare stopLossMode: string;
  declare trailingStopTiersJson: string;
  declare riskBudget: number;
  declare catastrophicStopLoss: number;
  declare reverseGateEnabled: number;
  declare maxContracts: number;
  declare extraRiskJson?: string;
}

export class ArgusMonitorSymbol {
  declare id: number;
  declare symbol: string;
  declare deepInstrument: string;
  declare tradeInstrument: string;
  declare spreadThreshold: number;
  declare signalThreshold: number;
  declare enabled: number;
}

export class ArgusNotification {
  declare id: number;
  declare telegramEnabled: number;
  declare telegramBotToken?: string;
  declare telegramChatId?: string;
}

export class ArgusRuntimeSession {
  declare id: number;
  declare accountId: number;
  declare cookie?: string;
  declare token?: string;
  declare otoken?: string;
  declare sentryRelease?: string;
  declare sentryPublicKey?: string;
  declare baggage?: string;
  declare loginUrl: string;
  declare finalUrl: string;
  declare valid: number;
  declare sessionUpdatedAt: string;
  declare expiresAt?: string;
  declare lastError?: string;
}

export class ArgusConfigSnapshot {
  declare version: ArgusConfigVersion;
  declare config: ArgusConfig;
  declare accounts: ArgusAccount[];
  declare accountRisks: ArgusAccountRisk[];
  declare monitorSymbols: ArgusMonitorSymbol[];
  declare notification: ArgusNotification;
  declare sessions: ArgusRuntimeSession[];
}

export class ArgusHeartbeat {
  declare instanceId: string;
  declare version: number;
  declare startedAt: string;
  declare reloadedAt?: string;
  declare lastReloadError?: string;
}

export class ArgusRuntimeStatus {
  declare online: boolean;
  declare heartbeat?: ArgusHeartbeat;
}

export class ArgusControlResult {
  declare action: string;
  declare output?: string;
}

export type ArgusConfigDraft = Omit<ArgusConfigSnapshot, "version"> & { releaseNote: string };

async function post<T>(url: string, body?: unknown): Promise<T> {
  const response = await instance.post<ApiResponse<T>>(url, body);
  return unwrapApiResponse(response.data);
}

export async function fetchPublishedArgusConfig(): Promise<ArgusConfigSnapshot | null> {
	const response = await instance.get<ApiResponse<ArgusConfigSnapshot | null>>("/argus-config/published");
	return unwrapApiResponse(response.data);
}

export function saveArgusConfigDraft(payload: ArgusConfigDraft): Promise<ArgusConfigVersion> {
  return post<ArgusConfigVersion>("/argus-config/drafts", payload);
}

export function publishArgusConfig(versionId: number, releaseNote: string): Promise<ArgusConfigVersion> {
  return post<ArgusConfigVersion>(`/argus-config/versions/${versionId}/publish`, { releaseNote });
}

export async function fetchArgusRuntimeStatus(): Promise<ArgusRuntimeStatus> {
  const response = await instance.get<ApiResponse<ArgusRuntimeStatus>>("/argus/runtime/status");
  return unwrapApiResponse(response.data);
}

export function controlArgus(action: "start" | "stop" | "restart"): Promise<ArgusControlResult> {
  return post<ArgusControlResult>(`/argus/runtime/${action}`);
}

export function reloadArgus(): Promise<ArgusControlResult> {
  return post<ArgusControlResult>("/argus/runtime/reload");
}
