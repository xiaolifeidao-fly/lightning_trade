"use client";

import { instance, unwrapApiResponse, type ApiResponse } from "@/utils/axios";

/**
 * 实例总览：注册信息 + 已发布版本 + 心跳生效状态三合一。
 *
 * 总览页与实例对比页共用它，所以放在共享组件目录而不是某一个页面模块下面。
 * 参数取值本身仍走 /argus-config/published（那是整份快照，只有对比矩阵才需要）。
 */
export class ArgusInstanceRuntime {
  declare instanceKey: string;
  declare instanceName: string;
  declare description?: string;
  declare configSource?: string;
  declare enabled: number;

  /** 0 = 该实例还没有已发布版本。 */
  declare publishedVersion: number;
  declare publishedChecksum: string;
  declare publishedAt?: string;
  declare publishedBy?: string;

  declare online: boolean;
  declare heartbeatAgeSeconds?: number;
  declare heartbeatAt?: string;
  declare runningVersion: number;
  declare runningChecksum?: string;
  declare health?: string;
  declare pid: number;
  declare buildVersion?: string;
  declare startedAt?: string;
  declare lastReloadAt?: string;
  declare lastReloadSuccess?: boolean;
  declare lastReloadError?: string;

  /** effective / awaiting / drift / offline / unknown，与 r7 参数页同一套词表。 */
  declare effectState: string;
  declare versionDrift: boolean;
  declare checksumDrift: boolean;
}

export class ArgusInstanceOverview {
  declare instances: ArgusInstanceRuntime[];
  /** 非空即告警：argus_instance 里实例键重复，配置发布与心跳都可能串实例。 */
  declare duplicateInstanceKeys: string[];
  declare driftInstanceKeys: string[];
  declare offlineInstanceKeys: string[];
  declare notice: string;
}

export function fetchArgusInstanceOverview(onlyEnabled = true): Promise<ArgusInstanceOverview> {
  return instance
    .get<ApiResponse<ArgusInstanceOverview>>("/argus-config/instance-overview", {
      params: { onlyEnabled: onlyEnabled ? "true" : "false" },
    })
    .then((response) => unwrapApiResponse(response.data));
}

/** 生效状态的展示文案与色调，总览页、实例卡片与顶栏选择器共用一份。 */
export const EFFECT_STATE_META: Record<string, { label: string; tone: "ok" | "warn" | "err" | "mute" }> = {
  effective: { label: "参数已生效", tone: "ok" },
  awaiting: { label: "等待生效", tone: "warn" },
  drift: { label: "参数漂移", tone: "err" },
  offline: { label: "心跳超时", tone: "err" },
  unknown: { label: "尚未发布", tone: "mute" },
};

export function effectStateMeta(state: string) {
  return EFFECT_STATE_META[state] ?? EFFECT_STATE_META.unknown;
}
