"use client";

import axios from "axios";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  controlArgus,
  fetchArgusRuntimeStatus,
  fetchPublishedArgusConfig,
  publishArgusConfig,
  reloadArgus,
  saveArgusConfigDraft,
  type ArgusConfigDraft,
  type ArgusConfigSnapshot,
  type ArgusControlResult,
  type ArgusRuntimeStatus,
} from "../api/argus-config.api";

const AUTO_REFRESH_INTERVAL = 10000;
const STARTUP_RETRY_DELAYS = [1000, 2000, 4000, 8000, 12000, 15000];

function isRetryableStartupError(error: unknown) {
  if (!axios.isAxiosError(error)) return false;
  const status = error.response?.status;
  return status === 502 || status === 503 || status === 504 || error.code === "ECONNREFUSED";
}

function wait(delay: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, delay));
}

export function useArgusConfig() {
  const [snapshot, setSnapshot] = useState<ArgusConfigSnapshot | null>(null);
  const [runtime, setRuntime] = useState<ArgusRuntimeStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [controlAction, setControlAction] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<ArgusControlResult | null>(null);
  const [lastSyncAt, setLastSyncAt] = useState<number | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [loadError, setLoadError] = useState<Error | null>(null);
  const initializedRef = useRef(false);

  const refresh = useCallback(async (retryForStartup = false) => {
    if (!initializedRef.current) setLoading(true);
    setRefreshing(true);
    try {
      const delays = retryForStartup ? [0, ...STARTUP_RETRY_DELAYS] : [0];
      let latestError: unknown;

      for (const delay of delays) {
        if (delay > 0) await wait(delay);
        try {
          const [nextSnapshot, nextRuntime] = await Promise.all([
            fetchPublishedArgusConfig(),
            fetchArgusRuntimeStatus(),
          ]);
          setSnapshot(nextSnapshot);
          setRuntime(nextRuntime);
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
  }, []);

  const refreshRuntime = useCallback(async () => {
    const nextRuntime = await fetchArgusRuntimeStatus();
    setRuntime(nextRuntime);
    setLastSyncAt(Date.now());
  }, []);

  const publishAndReload = useCallback(
    async (draft: ArgusConfigDraft) => {
      setSaving(true);
      try {
        const saved = await saveArgusConfigDraft(draft);
        const published = await publishArgusConfig(saved.id, draft.releaseNote);
        const result = await reloadArgus();
        setLastResult(result);
        await refresh();
        return published;
      } finally {
        setSaving(false);
      }
    },
    [refresh],
  );

  const runControl = useCallback(
    async (action: "start" | "stop" | "restart" | "reload") => {
      setControlAction(action);
      try {
        const result = action === "reload" ? await reloadArgus() : await controlArgus(action);
        setLastResult(result);
        await refreshRuntime();
        return result;
      } finally {
        setControlAction(null);
      }
    },
    [refreshRuntime],
  );

  useEffect(() => {
    void refresh(true).catch(() => undefined);
  }, [refresh]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void refreshRuntime().catch(() => undefined);
    }, AUTO_REFRESH_INTERVAL);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refreshRuntime]);

  return {
    snapshot,
    runtime,
    loading,
    refreshing,
    saving,
    controlAction,
    lastResult,
    lastSyncAt,
    loadError,
    autoRefresh,
    setAutoRefresh,
    refresh,
    publishAndReload,
    runControl,
  };
}
