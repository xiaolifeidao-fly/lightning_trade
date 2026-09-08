"use client";

import axios from "axios";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useArgusInstanceScope } from "@/components/argus/instanceScope";
import {
  fetchArgusConfigVersions,
  fetchArgusInstances,
  fetchArgusRuntimeStatus,
  fetchPublishedArgusConfig,
  publishArgusConfig,
  reloadArgus,
  rollbackArgusConfig,
  saveArgusConfigDraft,
  type ArgusConfigDraft,
  type ArgusConfigSnapshot,
  type ArgusConfigVersion,
  type ArgusControlResult,
  type ArgusInstance,
  type ArgusRuntimeStatus,
} from "../api/argus-config.api";

const IDLE_REFRESH_INTERVAL = 10000;
/** 等待生效期间加密轮询，保证「发布后 15 秒内看到已生效」这条验收成立。 */
const AWAITING_REFRESH_INTERVAL = 2500;
/** 超过这个时长仍未收到匹配心跳，就停止加密轮询并提示人工排查。 */
const AWAITING_TIMEOUT = 60000;
const STARTUP_RETRY_DELAYS = [1000, 2000, 4000, 8000, 12000, 15000];

export type EffectState = "effective" | "awaiting" | "drift" | "offline" | "unknown";

function isRetryableStartupError(error: unknown) {
  if (!axios.isAxiosError(error)) return false;
  const status = error.response?.status;
  return status === 502 || status === 503 || status === 504 || error.code === "ECONNREFUSED";
}

function wait(delay: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, delay));
}

export function useArgusConfig() {
  // 编辑实例走全局作用域：顶栏选择器、总览页与本页共用一份选择。本页不接受
  // 「全部实例」——一次发布波及多个实例就把 champion/challenger 对照组毁掉了，
  // 所以下面 bootstrap 时会兜到具体实例。
  const [instanceKey, setScope] = useArgusInstanceScope();
  const [instances, setInstances] = useState<ArgusInstance[]>([]);
  const [snapshot, setSnapshot] = useState<ArgusConfigSnapshot | null>(null);
  const [versions, setVersions] = useState<ArgusConfigVersion[]>([]);
  const [runtime, setRuntime] = useState<ArgusRuntimeStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [lastResult, setLastResult] = useState<ArgusControlResult | null>(null);
  const [lastSyncAt, setLastSyncAt] = useState<number | null>(null);
  const [loadError, setLoadError] = useState<Error | null>(null);
  const [instanceError, setInstanceError] = useState<Error | null>(null);
  /** 最近一次发布的时刻，用于判断心跳里的热加载记录是不是这次发布带来的 */
  const [publishedAt, setPublishedAt] = useState<number | null>(null);
  const initializedRef = useRef(false);
  // 实例列表只拉一次，用 ref 读当前作用域，避免把它加进依赖后反复 bootstrap。
  const instanceKeyRef = useRef(instanceKey);
  instanceKeyRef.current = instanceKey;

  const setInstanceKey = useCallback(
    (next: string) => {
      setScope(next);
    },
    [setScope],
  );

  // 换实例就把「刚发布过」的标记清掉：publishedAt 是用来判断心跳不匹配算等待还是
  // 漂移的，留着上一个实例的时刻会把新实例的旧版本误判成正在生效。
  useEffect(() => {
    setPublishedAt(null);
  }, [instanceKey]);

  // 实例列表先于任何配置读写加载：没有选定实例就不允许发任何请求，
  // 否则服务端会按默认实例兜底，那就是「改 A 误伤 B」。
  useEffect(() => {
    let cancelled = false;
    void fetchArgusInstances()
      .then((list) => {
        if (cancelled) return;
        setInstances(list);
        setInstanceError(null);
        const preferred = list.find((item) => item.instanceKey === instanceKeyRef.current) ?? list[0];
        if (preferred) setScope(preferred.instanceKey);
        else setLoading(false);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setInstanceError(error instanceof Error ? error : new Error("读取实例列表失败"));
        setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const refresh = useCallback(
    async (retryForStartup = false) => {
      if (!instanceKey) return;
      if (!initializedRef.current) setLoading(true);
      setRefreshing(true);
      try {
        const delays = retryForStartup ? [0, ...STARTUP_RETRY_DELAYS] : [0];
        let latestError: unknown;

        for (const delay of delays) {
          if (delay > 0) await wait(delay);
          try {
            const [nextSnapshot, nextRuntime, nextVersions] = await Promise.all([
              fetchPublishedArgusConfig(instanceKey),
              fetchArgusRuntimeStatus(instanceKey),
              fetchArgusConfigVersions(instanceKey),
            ]);
            setSnapshot(nextSnapshot);
            setRuntime(nextRuntime);
            setVersions(nextVersions ?? []);
            setLastSyncAt(Date.now());
            setLoadError(null);
            return;
          } catch (error) {
            latestError = error;
            if (!retryForStartup || !isRetryableStartupError(error)) break;
          }
        }

        const error = latestError instanceof Error ? latestError : new Error("读取 Argus 配置失败");
        setLoadError(error);
        throw error;
      } finally {
        initializedRef.current = true;
        setLoading(false);
        setRefreshing(false);
      }
    },
    [instanceKey],
  );

  const refreshRuntime = useCallback(async () => {
    if (!instanceKey) return;
    const nextRuntime = await fetchArgusRuntimeStatus(instanceKey);
    setRuntime(nextRuntime);
    setLastSyncAt(Date.now());
  }, [instanceKey]);

  /** 发布：保存草稿 → 发布版本（服务端顺带写 Redis 快照并广播）。 */
  const publish = useCallback(
    async (draft: ArgusConfigDraft) => {
      setSaving(true);
      try {
        const saved = await saveArgusConfigDraft(draft);
        const published = await publishArgusConfig(draft.instanceKey, saved.id, draft.releaseNote);
        setPublishedAt(Date.now());
        await refresh();
        return published;
      } finally {
        setSaving(false);
      }
    },
    [refresh],
  );

  /** 跨实例同步只生成草稿，不发布：目标实例的账户与凭证跟本实例不同，必须人去确认。 */
  const saveDraftOnly = useCallback(async (draft: ArgusConfigDraft) => {
    setSaving(true);
    try {
      return await saveArgusConfigDraft(draft);
    } finally {
      setSaving(false);
    }
  }, []);

  const rollback = useCallback(
    async (versionId: number, releaseNote: string) => {
      setSaving(true);
      try {
        const result = await rollbackArgusConfig(instanceKey, versionId, releaseNote);
        setPublishedAt(Date.now());
        await refresh();
        return result;
      } finally {
        setSaving(false);
      }
    },
    [instanceKey, refresh],
  );

  const reload = useCallback(async () => {
    setReloading(true);
    try {
      const result = await reloadArgus(instanceKey);
      setLastResult(result);
      await refreshRuntime();
      return result;
    } finally {
      setReloading(false);
    }
  }, [instanceKey, refreshRuntime]);

  useEffect(() => {
    if (!instanceKey) return;
    void refresh(true).catch(() => undefined);
  }, [instanceKey, refresh]);

  const heartbeat = runtime?.heartbeat;
  const publishedVersion = snapshot?.version;

  /**
   * 生效判定只认心跳：published 只说明管理端写成功了，程序有没有真读到要看
   * argus:heartbeat:<instance> 回报的 version + checksum。校验和缺失（老构建）
   * 时退化成只比版本号，并在界面上标注。
   */
  const effectState: EffectState = useMemo(() => {
    if (!publishedVersion) return "unknown";
    if (!runtime) return "unknown";
    if (!runtime.online || !heartbeat) return "offline";
    const versionMatched = heartbeat.version === publishedVersion.version;
    const checksumMatched = !heartbeat.configChecksum || heartbeat.configChecksum === publishedVersion.snapshotChecksum;
    if (versionMatched && checksumMatched) return "effective";
    return publishedAt ? "awaiting" : "drift";
  }, [heartbeat, publishedAt, publishedVersion, runtime]);

  const awaiting = effectState === "awaiting" && publishedAt !== null && Date.now() - publishedAt < AWAITING_TIMEOUT;

  useEffect(() => {
    if (!instanceKey) return;
    const interval = awaiting ? AWAITING_REFRESH_INTERVAL : IDLE_REFRESH_INTERVAL;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void refreshRuntime().catch(() => undefined);
    }, interval);
    return () => window.clearInterval(timer);
  }, [awaiting, instanceKey, refreshRuntime]);

  // 生效之后把「刚发布」状态清掉，避免后续的漂移被一直显示成「等待生效」。
  useEffect(() => {
    if (effectState === "effective" && publishedAt !== null) setPublishedAt(null);
  }, [effectState, publishedAt]);

  const instance = useMemo(
    () => instances.find((item) => item.instanceKey === instanceKey) ?? null,
    [instanceKey, instances],
  );

  return {
    instances,
    instance,
    instanceKey,
    setInstanceKey,
    instanceError,
    snapshot,
    versions,
    runtime,
    heartbeat,
    effectState,
    awaiting,
    publishedAt,
    loading,
    refreshing,
    saving,
    reloading,
    lastResult,
    lastSyncAt,
    loadError,
    refresh,
    publish,
    saveDraftOnly,
    rollback,
    reload,
  };
}
