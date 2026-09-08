"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useArgusInstanceScope } from "@/components/argus/instanceScope";
import {
  createOptimizeStudy,
  fetchOptimizeDefaults,
  fetchOptimizeStudies,
  fetchOptimizeStudy,
  fetchOptimizeSources,
  type OptimizeDefaults,
  type OptimizeDetail,
  type OptimizeSource,
  type OptimizeStudy,
} from "../api/argus-optimizer.api";

const SOURCE_STORAGE_KEY = "argus-optimizer:source";
const STUDY_POLL_INTERVAL = 3000;

function keyOf(source: OptimizeSource): string {
  return `${source.instanceKey}|${source.accountLabel}|${source.instrument}`;
}

function wallClock(value: string): string {
  const matched = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})/.exec(value);
  return matched ? `${matched[1]} ${matched[2]}` : value;
}

function storedSource(): string {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(SOURCE_STORAGE_KEY) ?? "";
}

function active(status: string): boolean {
  return status === "pending" || status === "running";
}

/**
 * 寻优状态编排：服务端冻结空间/协议/三关阈值；前端只负责发起、轮询并如实显示，
 * 从不在浏览器重新计算 gate、挑选“最优格”或改写扫描中的标准。
 */
export function useOptimizer() {
  const [instanceScope, setInstanceScope] = useArgusInstanceScope();
  const [sources, setSources] = useState<OptimizeSource[]>([]);
  const [selectedSourceKey, setSelectedSourceKey] = useState("");
  const [bootstrapping, setBootstrapping] = useState(true);
  const [bootError, setBootError] = useState("");
  const [defaults, setDefaults] = useState<OptimizeDefaults | null>(null);
  const [defaultsError, setDefaultsError] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [platformCode, setPlatformCode] = useState("deepcoin");
  const [studies, setStudies] = useState<OptimizeStudy[]>([]);
  const [studyId, setStudyId] = useState<number | null>(null);
  const [detail, setDetail] = useState<OptimizeDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const source = useMemo(
    () => sources.find((item) => keyOf(item) === selectedSourceKey) ?? null,
    [selectedSourceKey, sources],
  );

  useEffect(() => {
    let cancelled = false;
    void Promise.all([fetchOptimizeSources(), fetchOptimizeDefaults()])
      .then(([sourceResult, defaultsResult]) => {
        if (cancelled) return;
        const list = sourceResult.sources ?? [];
        setSources(list);
        const scoped = list.find((item) => item.instanceKey === instanceScope);
        const saved = list.find((item) => keyOf(item) === storedSource());
        const preferred = scoped ?? saved ?? list[0];
        if (preferred) setSelectedSourceKey(keyOf(preferred));
        setDefaults(defaultsResult);
        setBootError("");
        setDefaultsError("");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        const reason = error instanceof Error ? error.message : "读取寻优初始化数据失败";
        setBootError(reason);
        setDefaultsError(reason);
      })
      .finally(() => {
        if (!cancelled) setBootstrapping(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const selectSource = useCallback(
    (nextKey: string) => {
      const next = sources.find((item) => keyOf(item) === nextKey);
      setSelectedSourceKey(nextKey);
      setStudyId(null);
      setDetail(null);
      if (typeof window !== "undefined") window.localStorage.setItem(SOURCE_STORAGE_KEY, nextKey);
      if (next && next.instanceKey !== instanceScope) setInstanceScope(next.instanceKey);
    },
    [instanceScope, setInstanceScope, sources],
  );

  useEffect(() => {
    if (!instanceScope || !sources.length) return;
    const current = sources.find((item) => keyOf(item) === selectedSourceKey);
    if (current?.instanceKey === instanceScope) return;
    const matched = sources.find((item) => item.instanceKey === instanceScope);
    if (matched && keyOf(matched) !== selectedSourceKey) setSelectedSourceKey(keyOf(matched));
  }, [instanceScope, selectedSourceKey, sources]);

  useEffect(() => {
    if (!source) return;
    setStart(wallClock(source.firstTs));
    setEnd(wallClock(source.lastTs));
  }, [source]);

  const loadStudies = useCallback(async () => {
    if (!source) return;
    try {
      const result = await fetchOptimizeStudies({ page: 1, pageSize: 20, instanceKey: source.instanceKey, accountLabel: source.accountLabel });
      setStudies(result.list ?? []);
    } catch {
      setStudies([]);
    }
  }, [source]);

  useEffect(() => {
    void loadStudies();
  }, [loadStudies]);

  const loadDetail = useCallback(async (id: number, silent = false) => {
    if (!silent) setDetailLoading(true);
    try {
      const result = await fetchOptimizeStudy(id);
      setDetail(result);
      setDetailError("");
      return result;
    } catch (error: unknown) {
      setDetailError(error instanceof Error ? error.message : "读取寻优任务详情失败");
      return null;
    } finally {
      if (!silent) setDetailLoading(false);
    }
  }, []);

  const selectStudy = useCallback(
    (id: number) => {
      setStudyId(id);
      setDetail(null);
      void loadDetail(id);
    },
    [loadDetail],
  );

  useEffect(() => {
    if (!studyId || !detail || !active(detail.study.status)) return;
    const timer = window.setInterval(() => {
      void loadDetail(studyId, true).then((result) => {
        if (result && !active(result.study.status)) void loadStudies();
      });
    }, STUDY_POLL_INTERVAL);
    return () => window.clearInterval(timer);
  }, [detail, loadDetail, loadStudies, studyId]);

  const submit = useCallback(
    async (name: string) => {
      if (!source) throw new Error("请先选择实例、账户和信号源");
      if (!start || !end) throw new Error("请填写扫描窗口");
      setSubmitting(true);
      try {
        const created = await createOptimizeStudy({
          name,
          platformCode,
          symbol,
          startTime: start,
          endTime: end,
          instanceKey: source.instanceKey,
          accountLabel: source.accountLabel,
          concurrency: 3,
        });
        selectStudy(created.studyId);
        void loadStudies();
        return created.studyId;
      } finally {
        setSubmitting(false);
      }
    },
    [end, loadStudies, platformCode, selectStudy, source, start, symbol],
  );

  return {
    sources,
    source,
    selectedSourceKey,
    selectSource,
    bootstrapping,
    bootError,
    defaults,
    defaultsError,
    start,
    setStart,
    end,
    setEnd,
    symbol,
    setSymbol,
    platformCode,
    setPlatformCode,
    studies,
    studyId,
    detail,
    detailLoading,
    detailError,
    selectStudy,
    refreshDetail: () => (studyId ? loadDetail(studyId) : Promise.resolve(null)),
    submitting,
    submit,
  };
}
