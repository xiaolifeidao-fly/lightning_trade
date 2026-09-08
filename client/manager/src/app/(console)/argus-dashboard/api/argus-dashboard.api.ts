"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";
import type { EventWindow } from "../../argus-signals/api/argus-signals.api";

/**
 * 总览页独有的两个读接口。
 *
 * 其余数据一律复用已有模块的 API 层，不在这里再抄一份：
 *   · 拦截原因分布 / 信号流 / 持仓派生状态 → ../../argus-signals/api
 *   · K 线覆盖率                          → ../../argus-market/api
 *   · 已发布参数快照                      → ../../argus-config/api
 *   · 实例注册与心跳                      → @/components/argus/argus-instance.api
 */

async function get<T>(url: string, params?: Record<string, string | number | undefined>): Promise<T> {
  const response = await instance.get<ApiResponse<T>>(url, { params });
  return unwrapApiResponse(response.data);
}

// ─── 跨实例汇总 ──────────────────────────────────────────────────────────────

export class InstanceAccount {
  declare accountLabel: string;
  declare uid: string;
  declare variant: string;
  declare lastEquity: number | null;
  declare lastBalance: number | null;
  declare lastNetSize: number | null;
  declare lastSampleTs: string;
}

export class InstanceSummary {
  declare instanceKey: string;
  declare instanceName: string;
  declare enabled: number;
  /** false = 事件里出现过这个实例键，但 argus_instance 没注册它。 */
  declare registered: boolean;

  declare signals: number;
  declare opened: number;
  declare capSkip: number;
  declare gateBlock: number;
  declare trendSkip: number;
  declare exits: number;

  declare openRate: number;
  declare realizedPnl: number | null;
  declare variants: string[];
  /** 窗口内出现过的最大参数版本号，是实例内序列，跨实例比大小没有意义。 */
  declare configVersion: number;
  declare firstTs: string;
  declare lastTs: string;

  declare accounts: InstanceAccount[];
  declare equityTotal: number | null;
  declare netSizeTotal: number | null;
}

export class InstanceSummaryResult {
  declare window: EventWindow;
  declare instances: InstanceSummary[];
  /** 服务端给的固定提示：三实例参数不同，指标不能跨实例相加。 */
  declare notice: string;
  declare truncated: boolean;
}

export interface InstanceSummaryParams {
  instrument?: string;
  start?: string;
  end?: string;
}

export function fetchInstanceSummary(params: InstanceSummaryParams): Promise<InstanceSummaryResult> {
  return get<InstanceSummaryResult>("/argus-event/instance-summary", { ...params });
}

// ─── 权益曲线 ────────────────────────────────────────────────────────────────

export class EquityPoint {
  /** 本地墙钟串；与 strategy_event 的时间口径一致。 */
  declare time: string;
  declare equity: number | null;
  declare balance: number | null;
  declare upl: number | null;
  declare minEquity: number | null;
  declare maxEquity: number | null;
  declare samples: number;
  /** 相对本序列首个已知权益的变动百分比。125x 下两个账户只有按百分比才同屏可比。 */
  declare changePct: number | null;
}

export class EquitySeries {
  declare instanceKey: string;
  declare accountLabel: string;
  declare uid: string;
  declare variant: string;
  declare firstEquity: number | null;
  declare lastEquity: number | null;
  declare changePct: number | null;
  declare points: EquityPoint[];
}

export class EquityCurve {
  declare instanceKey: string;
  declare window: EventWindow;
  declare bucketSeconds: number;
  declare series: EquitySeries[];
  declare notice: string;
}

export interface EquityCurveParams {
  instanceKey: string;
  accountLabels?: string;
  start?: string;
  end?: string;
  bucketSeconds?: number;
}

/** instanceKey 必填：账户唯一性是 (实例, 账户)，跨实例合并会把两个账户串成一条线。 */
export function fetchEquityCurve(params: EquityCurveParams): Promise<EquityCurve> {
  return get<EquityCurve>("/argus-event/equity-curve", { ...params });
}
