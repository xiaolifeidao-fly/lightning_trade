"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

type QueryParams = Record<string, string | number | undefined>;

async function get<T>(url: string, params?: QueryParams): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

async function post<T>(url: string, body: unknown): Promise<T> {
  const response = await instance.post<ApiResponse<T>>(url, body);
  return unwrapApiResponse(response.data);
}

export class OptimizeSource {
  declare instanceKey: string;
  declare accountLabel: string;
  declare variant: string;
  declare instrument: string;
  declare eventCount: number;
  declare firstTs: string;
  declare lastTs: string;
}

export class OptimizeSourceList {
  declare sources: OptimizeSource[];
}

export function fetchOptimizeSources(): Promise<OptimizeSourceList> {
  return get<OptimizeSourceList>("/backtest/signal-sources");
}

export interface CreateOptimizeStudyPayload {
  name?: string;
  platformCode: string;
  symbol: string;
  startTime: string;
  endTime: string;
  instanceKey: string;
  accountLabel: string;
  concurrency?: number;
}

export class OptimizeStudy {
  declare id: number;
  declare name: string;
  declare instanceKey: string;
  declare accountLabel: string;
  declare platformCode: string;
  declare coinCode: string;
  declare symbol: string;
  declare startTime: string;
  declare endTime: string;
  declare sampleKind: string;
  declare sampleNote: string;
  declare oosBaseId: number;
  declare stage: string;
  declare status: string;
  declare errorMsg: string;
  declare concurrency: number;
  declare coarseCellCount: number;
  declare fineCellCount: number;
  declare doneCellCount: number;
  declare failedCellCount: number;
  declare skipCellCount: number;
  declare replayCount: number;
  declare convergeNote: string;
  declare signalCount: number;
  declare klineCount: number;
  declare trendDayCount: number;
  declare volDayCount: number;
  declare gateLockedAt: string;
  declare gateNote: string;
  declare verdict: string;
  declare passedCellCount: number;
  declare baselineSource: string;
  declare createdTime: string;
}

export class OptimizeStudyList {
  declare total: number;
  declare list: OptimizeStudy[];
}

export class OptimizeCell {
  declare id: number;
  declare stage: string;
  declare key: string;
  declare mode: string;
  declare cap: number;
  declare stopPct: number;
  declare gatePct: number;
  declare fidelity: string;
  declare status: string;
  declare errorMsg: string;
  declare pathCount: number;
  declare medPnl28: number;
  declare p25Pnl28: number;
  declare p75Pnl28: number;
  declare iqrPnl28: number;
  declare minPnl28: number;
  declare maxPnl28: number;
  declare signRatio: number;
  declare lambdaBear: number;
  declare meanStopLoss: number;
  declare stopBudget: number;
  declare stopCount: number;
  declare p90MaxDrawdown: number;
  declare maxStack: number;
  declare medFee: number;
  declare medDays: number;
  declare medSignalRun: number;
  declare bearPoolSize: number;
  declare bearP10: number;
  declare bearP50: number;
  declare bearP90: number;
  declare chopP10: number;
  declare chopP50: number;
  declare mixedP10: number;
  declare mixedP50: number;
  declare okSign: boolean;
  declare okBear: boolean;
  declare okDd: boolean;
  declare okBudget: boolean;
  declare passed: boolean;
  declare passCount: number;
  declare verdictNote: string;
  declare dominatedBy: string[];
  declare isIncumbent: boolean;
  declare onDdFrontier: boolean;
  declare onBearFrontier: boolean;
  declare notes: string[];
  declare params: Record<string, unknown>;
}

export class OptimizeDetail {
  declare study: OptimizeStudy;
  declare space: Record<string, unknown>;
  declare protocol: Record<string, unknown>;
  declare converge: Record<string, unknown>;
  declare gates: Record<string, unknown>;
  declare baseline: { source: string; params: Record<string, unknown>; notes: string[] };
  declare incumbent: Record<string, unknown>;
  declare coarse: OptimizeCell[];
  declare fine: OptimizeCell[];
  declare conclusion: Record<string, unknown>;
  declare frontier: Record<string, unknown>[];
  declare scaleChecks: Record<string, unknown>[];
  declare warnings: string[];
}

export class OptimizeDefaults {
  declare space: Record<string, unknown>;
  declare coarseCellCount: number;
  declare coarseCells: string[];
  declare protocol: Record<string, unknown>;
  declare finePathCount: number;
  declare finePaths: string[];
  declare coarsePathCount: number;
  declare coarsePaths: string[];
  declare converge: Record<string, unknown>;
  declare gates: Record<string, unknown>;
  declare gateNote: string;
  declare estimatedReplays: number;
  declare notes: string[];
}

export function fetchOptimizeDefaults(): Promise<OptimizeDefaults> {
  return get<OptimizeDefaults>("/backtest/optimize-defaults");
}

export function fetchOptimizeStudies(params: { page: number; pageSize: number; instanceKey?: string; accountLabel?: string }): Promise<OptimizeStudyList> {
  return get<OptimizeStudyList>("/backtest/optimize-studies", params);
}

export function fetchOptimizeStudy(id: number): Promise<OptimizeDetail> {
  return get<OptimizeDetail>(`/backtest/optimize-studies/${id}`);
}

export function createOptimizeStudy(payload: CreateOptimizeStudyPayload): Promise<{ studyId: number }> {
  return post<{ studyId: number }>("/backtest/optimize-studies", payload);
}
