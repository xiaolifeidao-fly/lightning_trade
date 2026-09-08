"use client";

import type { PageResult } from "@/utils/axios";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useArgusInstanceScope } from "@/components/argus/instanceScope";
import {
  fetchEpisodeStats,
  fetchEpisodes,
  fetchExitKindOptions,
  fetchGateStats,
  fetchSignalFilterOptions,
  fetchSignals,
  fetchSliceCompare,
  type Episode,
  type EpisodeList,
  type EpisodeStats,
  type ExitKindOption,
  type GateStats,
  type SignalEvent,
  type SignalFilterOptions,
  type SliceCompare,
} from "../api/argus-signals.api";
import { EPISODE_PAGE_SIZE, RANGE_OPTIONS, SIGNAL_PAGE_SIZE, toWallClock } from "../constants";

/** 三个视角共用一套筛选器，各自只用得上其中一部分。 */
export type SignalView = "signal" | "episode" | "compare";

export interface SignalFilters {
  /** 空串 = 全部实例。全部实例视图下每行必须标出实例归属。 */
  instanceKey: string;
  instrument: string;
  accountLabels: string[];
  /** 事件大类：trigger（四类触发）/ exit（六类出场）/ all。 */
  category: string;
  /** 具体事件类型，给出时覆盖 category。 */
  events: string[];
  /** 结果：''（全部）/ open / blocked / 具体 gate_kind。 */
  result: string;
  strength: string;
  configVersions: string[];
  rangeKey: string;
  /** episode 与切片对比的窗口口径：opened / closed / overlap。 */
  timeField: string;
  exitKinds: string[];
  /** true = 只看计入策略胜率的持仓。 */
  strategyOnly: boolean;
}

const INITIAL_FILTERS: SignalFilters = {
  instanceKey: "",
  instrument: "",
  accountLabels: [],
  category: "trigger",
  events: [],
  result: "",
  strength: "",
  configVersions: [],
  rangeKey: "latest",
  timeField: "opened",
  exitKinds: [],
  strategyOnly: false,
};

/** 墙钟串 → 本地 Date，用于按数据末刻回推相对窗口。 */
function wallClockToDate(wallClock: string): Date | null {
  const matched = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/.exec(wallClock.trim());
  if (!matched) return null;
  const [, y, mo, d, h, mi, s] = matched;
  return new Date(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s));
}

export interface ResolvedRange {
  start?: string;
  end?: string;
}

/**
 * 相对窗口锚在**已入库数据的最新时刻**上，不是服务器当前时间。
 *
 * 主力数据是 6–8 月的回灌历史，按 now 回推「近 7 天」永远是空页；锚在数据末刻
 * 上，「近 7 天」才是「有数据的那批里最近的 7 天」。rangeKey=latest 时干脆不传
 * 时间，交给服务端按同样的规则定窗口，并把结果回显在 window.resolved 上。
 */
function resolveRange(rangeKey: string, dataEnd: string): ResolvedRange {
  const option = RANGE_OPTIONS.find((item) => item.key === rangeKey);
  if (!option || option.hours === null) return {};
  const anchor = wallClockToDate(dataEnd) ?? new Date();
  const start = new Date(anchor.getTime() - option.hours * 3600 * 1000);
  return { start: toWallClock(start), end: toWallClock(anchor) };
}

const csv = (values: string[]): string | undefined => (values.length ? values.join(",") : undefined);

export function useArgusSignals() {
  // 实例选择是全局的（顶栏选择器与其余 Argus 页面共用一份），本页只把它同步进筛选器。
  const [scope, setScope] = useArgusInstanceScope();
  const [options, setOptions] = useState<SignalFilterOptions | null>(null);
  const [exitKinds, setExitKinds] = useState<ExitKindOption[]>([]);
  const [filters, setFilters] = useState<SignalFilters>(() => ({ ...INITIAL_FILTERS, instanceKey: scope }));
  const [view, setView] = useState<SignalView>("signal");

  const [signalPage, setSignalPage] = useState(1);
  const [signals, setSignals] = useState<PageResult<SignalEvent>>({ total: 0, data: [] });
  const [gateStats, setGateStats] = useState<GateStats | null>(null);

  const [episodePage, setEpisodePage] = useState(1);
  const [episodes, setEpisodes] = useState<EpisodeList | null>(null);
  const [episodeStats, setEpisodeStats] = useState<EpisodeStats | null>(null);

  const [sliceCompare, setSliceCompare] = useState<SliceCompare | null>(null);

  const [bootstrapping, setBootstrapping] = useState(true);
  const [bootError, setBootError] = useState("");
  const [listLoading, setListLoading] = useState(false);
  const [statsLoading, setStatsLoading] = useState(false);
  const [compareLoading, setCompareLoading] = useState(false);
  const [loadError, setLoadError] = useState("");

  // 并发保护：切实例 / 切视角时旧请求可能后到，用序号丢弃过期结果。
  const listSeq = useRef(0);
  const statsSeq = useRef(0);
  const compareSeq = useRef(0);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const result = await fetchSignalFilterOptions();
        if (!alive) return;
        setOptions(result);
        setFilters((prev) => ({ ...prev, instrument: prev.instrument || result.instruments[0] || "" }));
        if ((result.instances ?? []).length === 0) {
          setBootError("事件表里还没有任何数据：请先跑 r4 的双写或 r6 的历史回灌，再回到本页复盘。");
        }
      } catch (error) {
        if (alive) setBootError(error instanceof Error ? error.message : "读取筛选项失败");
      } finally {
        if (alive) setBootstrapping(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  // 顶栏切了实例（或别的 Argus 页面切了）就同步下来。账户名跨实例重号、版本号各自独立，
  // 所以一并清掉这两项选中值，否则会筛出一张空表还看不出是筛错了。
  useEffect(() => {
    setFilters((prev) =>
      prev.instanceKey === scope ? prev : { ...prev, instanceKey: scope, accountLabels: [], configVersions: [] },
    );
  }, [scope]);

  // 出场方式的条数是按实例统计的，切实例要重拉；失败不阻断页面，只是筛选框里没有条数。
  useEffect(() => {
    let alive = true;
    void fetchExitKindOptions(filters.instanceKey || undefined)
      .then((result) => {
        if (alive) setExitKinds(result);
      })
      .catch(() => {
        if (alive) setExitKinds([]);
      });
    return () => {
      alive = false;
    };
  }, [filters.instanceKey]);

  const dataEnd = options?.dataRange?.end ?? "";
  const range = useMemo(() => resolveRange(filters.rangeKey, dataEnd), [filters.rangeKey, dataEnd]);

  /** 账户名跨实例重号，所以只有锁定单实例时账户筛选才有确定含义。 */
  const accountOptions = useMemo(() => {
    const all = options?.accounts ?? [];
    if (!filters.instanceKey) return all;
    return all.filter((item) => item.instanceKey === filters.instanceKey);
  }, [options, filters.instanceKey]);

  const versionOptions = useMemo(() => {
    const all = options?.configVersions ?? [];
    if (!filters.instanceKey) return all;
    return all.filter((item) => item.instanceKey === filters.instanceKey);
  }, [options, filters.instanceKey]);

  // ── 列表 ───────────────────────────────────────────────────────────────────

  const loadSignals = useCallback(async () => {
    const seq = ++listSeq.current;
    setListLoading(true);
    try {
      const result = await fetchSignals({
        pageIndex: signalPage,
        pageSize: SIGNAL_PAGE_SIZE,
        instanceKey: filters.instanceKey || undefined,
        instrument: filters.instrument || undefined,
        accountLabels: csv(filters.accountLabels),
        category: filters.events.length ? undefined : filters.category,
        events: csv(filters.events),
        result: filters.result || undefined,
        strength: filters.strength || undefined,
        configVersions: csv(filters.configVersions),
        order: "ts_desc",
        ...range,
      });
      if (seq !== listSeq.current) return;
      setSignals(result);
      setLoadError("");
    } catch (error) {
      if (seq !== listSeq.current) return;
      setSignals({ total: 0, data: [] });
      setLoadError(error instanceof Error ? error.message : "读取信号列表失败");
    } finally {
      if (seq === listSeq.current) setListLoading(false);
    }
  }, [filters, range, signalPage]);

  const loadEpisodes = useCallback(async () => {
    const seq = ++listSeq.current;
    setListLoading(true);
    try {
      const result = await fetchEpisodes({
        pageIndex: episodePage,
        pageSize: EPISODE_PAGE_SIZE,
        instanceKey: filters.instanceKey || undefined,
        instrument: filters.instrument || undefined,
        accountLabels: csv(filters.accountLabels),
        exitKinds: csv(filters.exitKinds),
        strategyOnly: filters.strategyOnly ? 1 : undefined,
        timeField: filters.timeField,
        order: "ts_desc",
        ...range,
      });
      if (seq !== listSeq.current) return;
      setEpisodes(result);
      setLoadError("");
    } catch (error) {
      if (seq !== listSeq.current) return;
      setEpisodes(null);
      setLoadError(error instanceof Error ? error.message : "读取持仓列表失败");
    } finally {
      if (seq === listSeq.current) setListLoading(false);
    }
  }, [filters, range, episodePage]);

  useEffect(() => {
    if (bootstrapping) return;
    if (view === "signal") void loadSignals();
    if (view === "episode") void loadEpisodes();
  }, [bootstrapping, view, loadSignals, loadEpisodes]);

  // ── 统计 ───────────────────────────────────────────────────────────────────

  /**
   * 拦截原因分布与出场归因分布一起拉：两块统计挂在同一个窗口上，KPI 条上要同屏
   * 显示「信号侧」和「持仓侧」，分两次拉会出现两块数据窗口不一致的瞬间。
   */
  const loadStats = useCallback(async () => {
    const seq = ++statsSeq.current;
    setStatsLoading(true);
    try {
      const [gate, episode] = await Promise.all([
        fetchGateStats({
          instanceKey: filters.instanceKey || undefined,
          instrument: filters.instrument || undefined,
          accountLabel: csv(filters.accountLabels),
          ...range,
        }),
        fetchEpisodeStats({
          instanceKey: filters.instanceKey || undefined,
          instrument: filters.instrument || undefined,
          accountLabel: csv(filters.accountLabels),
          timeField: filters.timeField,
          ...range,
        }),
      ]);
      if (seq !== statsSeq.current) return;
      setGateStats(gate);
      setEpisodeStats(episode);
    } catch (error) {
      if (seq !== statsSeq.current) return;
      setGateStats(null);
      setEpisodeStats(null);
      setLoadError(error instanceof Error ? error.message : "读取统计失败");
    } finally {
      if (seq === statsSeq.current) setStatsLoading(false);
    }
  }, [filters.instanceKey, filters.instrument, filters.accountLabels, filters.timeField, range]);

  useEffect(() => {
    if (bootstrapping) return;
    void loadStats();
  }, [bootstrapping, loadStats]);

  // ── 切片对比 ───────────────────────────────────────────────────────────────

  /**
   * 切片对比按需加载：它要在信号表与 episode 表上各扫一遍窗口，和统计接口是
   * 同量级的开销。只在切到「切片对比」视角时拉，别让另外两个视角替它买单。
   */
  const loadCompare = useCallback(async () => {
    const seq = ++compareSeq.current;
    setCompareLoading(true);
    try {
      const result = await fetchSliceCompare({
        instanceKey: filters.instanceKey || undefined,
        instrument: filters.instrument || undefined,
        accountLabel: csv(filters.accountLabels),
        timeField: filters.timeField,
        ...range,
      });
      if (seq !== compareSeq.current) return;
      setSliceCompare(result);
      setLoadError("");
    } catch (error) {
      if (seq !== compareSeq.current) return;
      setSliceCompare(null);
      setLoadError(error instanceof Error ? error.message : "读取切片对比失败");
    } finally {
      if (seq === compareSeq.current) setCompareLoading(false);
    }
  }, [filters.instanceKey, filters.instrument, filters.accountLabels, filters.timeField, range]);

  useEffect(() => {
    if (bootstrapping || view !== "compare") return;
    void loadCompare();
  }, [bootstrapping, view, loadCompare]);

  // ── 筛选器 ─────────────────────────────────────────────────────────────────

  const patchFilters = useCallback(
    (patch: Partial<SignalFilters>) => {
      // 页面内改实例要回写全局作用域，否则回到总览页看到的还是上一个实例。
      if (patch.instanceKey !== undefined) setScope(patch.instanceKey);
      setFilters((prev) => {
        const next = { ...prev, ...patch };
        // 切实例会让账户与版本的候选集整体换掉：账户名跨实例重号，版本号也各自独立，
        // 留着上一个实例的选中值会筛出一张空表，且看不出是筛错了。
        if (patch.instanceKey !== undefined && patch.instanceKey !== prev.instanceKey) {
          next.accountLabels = [];
          next.configVersions = [];
        }
        return next;
      });
      setSignalPage(1);
      setEpisodePage(1);
    },
    [setScope],
  );

  const resetFilters = useCallback(() => {
    setFilters((prev) => ({ ...INITIAL_FILTERS, instrument: prev.instrument, instanceKey: prev.instanceKey }));
    setSignalPage(1);
    setEpisodePage(1);
  }, []);

  const refresh = useCallback(() => {
    void loadStats();
    if (view === "signal") void loadSignals();
    if (view === "episode") void loadEpisodes();
    if (view === "compare") void loadCompare();
  }, [view, loadStats, loadSignals, loadEpisodes, loadCompare]);

  /** 派生表新鲜度：episode 列表与统计都会带，取先到的那份。 */
  const rebuild = episodes?.rebuild ?? episodeStats?.rebuild ?? sliceCompare?.rebuild ?? null;

  /** 全部实例视图 = 没锁定实例。此时账户名会重号，逐行必须标实例。 */
  const crossInstance = !filters.instanceKey;

  const episodeRows: Episode[] = episodes?.data ?? [];

  return {
    options,
    exitKinds,
    accountOptions,
    versionOptions,
    filters,
    patchFilters,
    resetFilters,
    view,
    setView,
    crossInstance,
    range,
    signals,
    signalPage,
    setSignalPage,
    episodes,
    episodeRows,
    episodePage,
    setEpisodePage,
    gateStats,
    episodeStats,
    sliceCompare,
    rebuild,
    bootstrapping,
    bootError,
    listLoading,
    statsLoading,
    compareLoading,
    loadError,
    refresh,
  };
}
