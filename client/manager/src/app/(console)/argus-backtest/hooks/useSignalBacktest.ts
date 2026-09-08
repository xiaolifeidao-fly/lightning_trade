"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useArgusInstanceScope } from "@/components/argus/instanceScope";
import {
  createSignalBatch,
  fetchBacktestRunDetail,
  fetchSignalBaseline,
  fetchSignalBatchDetail,
  fetchSignalBatches,
  fetchSignalSources,
  type BacktestRunDetail,
  type SignalBacktestGroup,
  type SignalBacktestParams,
  type SignalBaseline,
  type SignalBatch,
  type SignalBatchDetail,
  type SignalSource,
} from "../api/argus-backtest.api";
import { ALL_KNOBS, BATCH_PAGE_SIZE, BATCH_POLL_INTERVAL, isBatchActive, rfc3339ToWallClock } from "../constants";

const SOURCE_STORAGE_KEY = "argus-backtest:source";

/** 待提交的一组参数：只存**与基线的增量**，没改的旋钮不进 payload。 */
export interface DraftGroup {
  /** 本地临时 id，提交后由服务端的 runId 接管。 */
  localId: string;
  label: string;
  overrides: SignalBacktestParams;
}

function sourceKey(source: SignalSource): string {
  return `${source.instanceKey}|${source.accountLabel}|${source.instrument}`;
}

function readStoredSource(): string {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(SOURCE_STORAGE_KEY) ?? "";
}

/** 基线里取一个旋钮的当前值。基线是 `signal.Params` 的原样 JSON，键名即字段名。 */
function baselineValue(baseline: SignalBaseline | null, field: string): number | string | undefined {
  if (!baseline) return undefined;
  return baseline.params?.[field];
}

/**
 * 盘口信号回测页的数据编排。
 *
 * 三条约束决定了它的形状：
 * 1. **信号源是批次级的**：组间要能比大小的前提是吃同一份触发流，所以实例、账户、
 *    窗口选在顶部，组只带参数增量；
 * 2. **基线 = 所选实例当前生产参数**，由服务端 `/backtest/signal-baseline` 解析，
 *    前端不自己去拼配置快照——兜底口径（哪些字段还没有 DB 列）只有服务端知道；
 * 3. **回放是异步的**，提交后只拿到 batchId，进度靠轮询批次详情。
 */
export function useSignalBacktest() {
  const [instanceScope, setInstanceScope] = useArgusInstanceScope();
  const [sources, setSources] = useState<SignalSource[]>([]);
  const [thresholdCandidates, setThresholdCandidates] = useState<number[]>([]);
  const [selectedSourceKey, setSelectedSourceKey] = useState("");
  const [bootstrapping, setBootstrapping] = useState(true);
  const [bootError, setBootError] = useState("");

  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [platformCode, setPlatformCode] = useState("deepcoin");

  const [baseline, setBaseline] = useState<SignalBaseline | null>(null);
  const [baselineLoading, setBaselineLoading] = useState(false);
  const [baselineError, setBaselineError] = useState("");

  const [drafts, setDrafts] = useState<DraftGroup[]>([]);
  const [submitting, setSubmitting] = useState(false);

  const [batches, setBatches] = useState<SignalBatch[]>([]);
  const [batchId, setBatchId] = useState<number | null>(null);
  const [detail, setDetail] = useState<SignalBatchDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");

  const [runDetails, setRunDetails] = useState<Record<number, BacktestRunDetail>>({});
  const runFetchingRef = useRef<Set<number>>(new Set());

  const source = useMemo(
    () => sources.find((item) => sourceKey(item) === selectedSourceKey) ?? null,
    [selectedSourceKey, sources],
  );

  // ─── 信号源 ────────────────────────────────────────────────────────────────

  useEffect(() => {
    let cancelled = false;
    void fetchSignalSources()
      .then((result) => {
        if (cancelled) return;
        const list = result.sources ?? [];
        setSources(list);
        setThresholdCandidates(result.thresholdCandidates ?? []);
        const stored = readStoredSource();
        // 顶栏的实例作用域优先于旧的页内缓存：从行情/复盘页切来时，回测基线也必须
        // 立即切到同一实例，不能拿上次浏览的 challenger 生产参数当参照。
        const preferred = list.find((item) => item.instanceKey === instanceScope) ?? list.find((item) => sourceKey(item) === stored) ?? list[0];
        if (preferred) setSelectedSourceKey(sourceKey(preferred));
        setBootError("");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setBootError(error instanceof Error ? error.message : "读取信号源失败");
      })
      .finally(() => {
        if (!cancelled) setBootstrapping(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const selectSource = useCallback((key: string) => {
    const next = sources.find((item) => sourceKey(item) === key);
    setSelectedSourceKey(key);
    setDrafts([]);
    setBatchId(null);
    setDetail(null);
    if (typeof window !== "undefined") window.localStorage.setItem(SOURCE_STORAGE_KEY, key);
    if (next && next.instanceKey !== instanceScope) setInstanceScope(next.instanceKey);
  }, [instanceScope, setInstanceScope, sources]);

  // 顶栏手动切实例时，保留“信号源”这个明确的账户选择语义，但切到目标实例的第一条
  // 源，避免跨实例误把旧账户的生产参数当作当前基线。
  useEffect(() => {
    if (!instanceScope || !sources.length) return;
    const current = sources.find((item) => sourceKey(item) === selectedSourceKey);
    if (current?.instanceKey === instanceScope) return;
    const next = sources.find((item) => item.instanceKey === instanceScope);
    if (next && sourceKey(next) !== selectedSourceKey) setSelectedSourceKey(sourceKey(next));
  }, [instanceScope, selectedSourceKey, sources]);

  // 窗口默认铺满该信号源已入库的覆盖区间：本库主力数据是 6–8 月的回灌历史，
  // 按当前时间兜底只会给一张空白页。
  useEffect(() => {
    if (!source) return;
    setStart(rfc3339ToWallClock(source.firstTs));
    setEnd(rfc3339ToWallClock(source.lastTs));
  }, [source]);

  // ─── 基线 ──────────────────────────────────────────────────────────────────

  const loadBaseline = useCallback(async () => {
    if (!source) return;
    setBaselineLoading(true);
    try {
      const result = await fetchSignalBaseline({
        instanceKey: source.instanceKey,
        accountLabel: source.accountLabel,
        symbol,
      });
      setBaseline(result);
      setBaselineError("");
    } catch (error: unknown) {
      setBaseline(null);
      setBaselineError(error instanceof Error ? error.message : "读取生产参数基线失败");
    } finally {
      setBaselineLoading(false);
    }
  }, [source, symbol]);

  useEffect(() => {
    void loadBaseline();
  }, [loadBaseline]);

  /** 表单初值：基线里有就用基线的，没有就留空（不编一个缺省顶上去）。 */
  const baselineForm = useMemo(() => {
    const values: Record<string, number | string> = {};
    for (const knob of ALL_KNOBS) {
      const value = baselineValue(baseline, knob.field);
      if (value !== undefined && value !== null) values[knob.field] = value;
    }
    return values;
  }, [baseline]);

  // ─── 扫描队列 ──────────────────────────────────────────────────────────────

  const addDraft = useCallback((label: string, overrides: SignalBacktestParams) => {
    setDrafts((previous) => [
      ...previous,
      { localId: `draft-${previous.length + 1}-${label}`, label, overrides },
    ]);
  }, []);

  const removeDraft = useCallback((localId: string) => {
    setDrafts((previous) => previous.filter((item) => item.localId !== localId));
  }, []);

  const clearDrafts = useCallback(() => setDrafts([]), []);

  // ─── 批次 ──────────────────────────────────────────────────────────────────

  const loadBatches = useCallback(async () => {
    if (!source) return;
    try {
      const result = await fetchSignalBatches({
        page: 1,
        pageSize: BATCH_PAGE_SIZE,
        instanceKey: source.instanceKey,
        accountLabel: source.accountLabel,
      });
      setBatches(result.list ?? []);
    } catch {
      // 批次列表读不到不该挡住主流程：提交与查看当前批次都不依赖它。
      setBatches([]);
    }
  }, [source]);

  useEffect(() => {
    void loadBatches();
  }, [loadBatches]);

  const loadDetail = useCallback(async (id: number, silent = false) => {
    if (!silent) setDetailLoading(true);
    try {
      const result = await fetchSignalBatchDetail(id);
      setDetail(result);
      setDetailError("");
      return result;
    } catch (error: unknown) {
      setDetailError(error instanceof Error ? error.message : "读取批次详情失败");
      return null;
    } finally {
      if (!silent) setDetailLoading(false);
    }
  }, []);

  const selectBatch = useCallback(
    (id: number) => {
      setBatchId(id);
      setDetail(null);
      setRunDetails({});
      void loadDetail(id);
    },
    [loadDetail],
  );

  // 回放中的批次每 3 秒刷一次；跑完就停，不做无意义的常驻轮询。
  useEffect(() => {
    if (!batchId || !detail || !isBatchActive(detail.batch.status)) return;
    const timer = window.setInterval(() => {
      void loadDetail(batchId, true).then((next) => {
        if (next && !isBatchActive(next.batch.status)) void loadBatches();
      });
    }, BATCH_POLL_INTERVAL);
    return () => window.clearInterval(timer);
  }, [batchId, detail, loadBatches, loadDetail]);

  const submit = useCallback(
    async (name: string, includeBaselineRun: boolean, concurrency: number) => {
      if (!source) throw new Error("请先选择信号源");
      if (!start || !end) throw new Error("请填写回测窗口");
      if (drafts.length === 0) throw new Error("扫描队列是空的，请先加入至少一组参数");
      setSubmitting(true);
      try {
        const groups: SignalBacktestGroup[] = drafts.map((draft) => ({
          label: draft.label,
          ...draft.overrides,
        }));
        const { batchId: created } = await createSignalBatch({
          name,
          platformCode,
          symbol,
          startTime: start,
          endTime: end,
          instanceKey: source.instanceKey,
          accountLabel: source.accountLabel,
          concurrency,
          includeBaselineRun,
          groups,
        });
        setDrafts([]);
        selectBatch(created);
        void loadBatches();
        return created;
      } finally {
        setSubmitting(false);
      }
    },
    [drafts, end, loadBatches, platformCode, selectBatch, source, start, symbol],
  );

  // ─── 单组逐笔（按需拉取，供净值曲线与明细抽屉共用）─────────────────────────

  const loadRunDetail = useCallback(async (runId: number) => {
    if (!runId || runFetchingRef.current.has(runId)) return;
    runFetchingRef.current.add(runId);
    try {
      const result = await fetchBacktestRunDetail(runId);
      setRunDetails((previous) => ({ ...previous, [runId]: result }));
    } catch {
      // 单组逐笔取不到不影响对比矩阵：曲线上少一条线，表格数据仍然完整。
    } finally {
      runFetchingRef.current.delete(runId);
    }
  }, []);

  /** 批次跑完后一次性把已完成组的逐笔取回来，净值曲线才画得出。 */
  useEffect(() => {
    if (!detail) return;
    for (const group of detail.groups ?? []) {
      for (const row of group.rows ?? []) {
        if (row.status === "done" && row.pnlAvailable && !runDetails[row.runId]) void loadRunDetail(row.runId);
      }
    }
  }, [detail, loadRunDetail, runDetails]);

  return {
    sources,
    source,
    selectedSourceKey,
    selectSource,
    thresholdCandidates,
    bootstrapping,
    bootError,

    start,
    setStart,
    end,
    setEnd,
    symbol,
    setSymbol,
    platformCode,
    setPlatformCode,

    baseline,
    baselineForm,
    baselineLoading,
    baselineError,
    reloadBaseline: loadBaseline,

    drafts,
    addDraft,
    removeDraft,
    clearDrafts,

    batches,
    batchId,
    detail,
    detailLoading,
    detailError,
    selectBatch,
    refreshDetail: () => (batchId ? loadDetail(batchId) : Promise.resolve(null)),

    runDetails,
    loadRunDetail,

    submitting,
    submit,
  };
}
