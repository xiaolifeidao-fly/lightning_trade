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
  // r5 把下面这几项从 extra_config_json 收敛成了独立列，运行时读的就是这些列
  // （argus_single/pkg/runtimeconfig/tuning.go 的 config.XXX）。
  declare contractFace: number;
  declare signalDelaySecond: number;
  declare spreadMaxPriceAgeMs: number;
  declare trendGateWindowHour: number;
  declare trendGateThresholdPct: number;
  declare reverseGateMinProfitPct: number;
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
  // 账户级覆盖也已收敛成独立列，运行时优先取 >0 的账户级值。
  declare reverseGateMinProfitPct: number;
  declare trendGateThresholdPct: number;
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
  /** 服务端只回 "******" 占位，明文永不出网；巡检看长度即可。 */
  declare cookie?: string;
  declare token?: string;
  declare otoken?: string;
  declare cookieLength: number;
  declare tokenLength: number;
  declare otokenLength: number;
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
  declare instanceKey: string;
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
  declare pid: number;
  declare buildVersion: string;
  declare version: number;
  /** 程序实际加载的快照校验和，与已发布版本的 snapshotChecksum 比对才算真生效。 */
  declare configChecksum?: string;
  declare startedAt: string;
  declare lastReloadAt?: string;
  declare lastReloadSuccess?: boolean;
  declare lastReloadError?: string;
  declare health: string;
  declare updatedAt: string;
}

export class ArgusRuntimeStatus {
  declare online: boolean;
  declare heartbeat?: ArgusHeartbeat;
}

export class ArgusControlResult {
  declare action: string;
  declare output?: string;
}

export class ArgusInstance {
  declare id: number;
  declare instanceKey: string;
  declare instanceName: string;
  declare description?: string;
  declare configSource?: string;
  declare enabled: number;
}

export type ArgusConfigDraft = Omit<ArgusConfigSnapshot, "version" | "instanceKey"> & {
  instanceKey: string;
  releaseNote: string;
};

async function get<T>(url: string, params?: Record<string, string | number>): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

async function post<T>(url: string, body?: unknown, params?: Record<string, string | number>): Promise<T> {
  const response = await instance.post<ApiResponse<T>>(url, body, { params });
  return unwrapApiResponse(response.data);
}

/**
 * 全部配置读写都必须带 instanceKey：参数按实例分域，缺实例键时服务端会按默认实例
 * 兜底解析，那正是「改 A 误伤 B」的入口，所以页面这一层强制传。
 */
export function fetchArgusInstances(): Promise<ArgusInstance[]> {
  return get<ArgusInstance[]>("/argus-config/instances", { onlyEnabled: "true" });
}

export function fetchPublishedArgusConfig(instanceKey: string): Promise<ArgusConfigSnapshot | null> {
  return get<ArgusConfigSnapshot | null>("/argus-config/published", { instanceKey });
}

export function fetchArgusConfigVersions(instanceKey: string, limit = 30): Promise<ArgusConfigVersion[]> {
  return get<ArgusConfigVersion[]>("/argus-config/versions", { instanceKey, limit });
}

export function saveArgusConfigDraft(payload: ArgusConfigDraft): Promise<ArgusConfigVersion> {
  return post<ArgusConfigVersion>("/argus-config/drafts", payload, { instanceKey: payload.instanceKey });
}

export function publishArgusConfig(
  instanceKey: string,
  versionId: number,
  releaseNote: string,
): Promise<ArgusConfigVersion> {
  return post<ArgusConfigVersion>(`/argus-config/versions/${versionId}/publish`, { instanceKey, releaseNote }, { instanceKey });
}

/** 回滚不新建版本号：把该历史版本的不可变快照重新推上 published 槽位并广播。 */
export function rollbackArgusConfig(
  instanceKey: string,
  versionId: number,
  releaseNote: string,
): Promise<ArgusConfigVersion> {
  return post<ArgusConfigVersion>(`/argus-config/versions/${versionId}/rollback`, { instanceKey, releaseNote }, { instanceKey });
}

export function fetchArgusRuntimeStatus(instanceKey: string): Promise<ArgusRuntimeStatus> {
  return get<ArgusRuntimeStatus>("/argus/runtime/status", { instanceKey });
}

export function reloadArgus(instanceKey: string): Promise<ArgusControlResult> {
  return post<ArgusControlResult>("/argus/runtime/reload", undefined, { instanceKey });
}
