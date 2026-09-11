"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ALL_INSTANCES, useArgusInstanceScope } from "@/components/argus/instanceScope";
import { fetchArgusInstanceOverview, type ArgusInstanceOverview } from "@/components/argus/argus-instance.api";
import { fetchPublishedArgusConfig, type ArgusConfigSnapshot } from "../../argus-config/api/argus-config.api";
import { fetchMarketTimeline, type MarketTimeline } from "../../argus-market/api/argus-market.api";
import { fetchGateStats, fetchSignalFilterOptions, fetchSignals, type GateStats, type SignalEvent, type SignalFilterOptions } from "../../argus-signals/api/argus-signals.api";
import { fetchEquityCurve, fetchInstanceSummary, type EquityCurve, type InstanceSummaryResult } from "../api/argus-dashboard.api";
import { EQUITY_BUCKET_SECONDS, OVERVIEW_REFRESH_INTERVAL, RECENT_SIGNAL_LIMIT, resolveRange } from "../constants";

type RangeKey = "d1" | "d3" | "d7" | "d30" | "d90" | "latest";

interface DashboardData {
  overview: ArgusInstanceOverview | null;
  options: SignalFilterOptions | null;
  summary: InstanceSummaryResult | null;
  equity: EquityCurve | null;
  gateStats: GateStats | null;
  signals: SignalEvent[];
  snapshot: ArgusConfigSnapshot | null;
  timeline: MarketTimeline | null;
}

const emptyData: DashboardData = { overview: null, options: null, summary: null, equity: null, gateStats: null, signals: [], snapshot: null, timeline: null };

/**
 * 这份数据属于哪个（实例, 窗口）。
 *
 * 单次刷新失败时要沿用上一轮的值（见 refresh 里的 keep），但**只能在口径没变时沿用**：
 * 换了实例还接着显示上一个实例的曲线，等于把 A 的持仓安在 B 头上，比空着危险得多。
 */
type DataScope = string;
const scopeOf = (scope: string, rangeKey: string): DataScope => `${scope}|${rangeKey}`;

/** 总览页数据编排：所有读取都经已有 API 模块，页面本身不直接发 HTTP。 */
export function useArgusDashboard() {
  const [scope, setScope] = useArgusInstanceScope();
  const [rangeKey, setRangeKey] = useState<RangeKey>("d1");
  const [data, setData] = useState<DashboardData>(emptyData);
  // loading 只管**首屏与换口径**：显示骨架、把表格打回加载态。
  // refreshing 管后台轮询与手动刷新：只让刷新按钮转一下，页面内容不动。
  //
  // 这两件事原来共用一个 loading，于是每 10 秒整页闪一次骨架——这页是挂在屏幕上
  // 长时间不动的巡检页，闪烁比"数据晚一分钟"难受得多，而且接口本身要 1~3 秒，
  // 一轮里有相当一段时间页面都是加载态。
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const [error, setError] = useState("");
  const generationRef = useRef(0);
  // 上一轮数据对应的口径。换实例或换窗口时它对不上，沿用逻辑自动失效。
  const dataScopeRef = useRef<DataScope>("");

  const refresh = useCallback(async (run?: { silent?: boolean }) => {
    const generation = ++generationRef.current;
    // 只有"这一轮要的口径和屏幕上已有的数据是同一个"时，才算后台刷新。
    // 换了实例或窗口，屏幕上的数据就是别人的了，必须走加载态而不是静默替换。
    const background = run?.silent === true && dataScopeRef.current === scopeOf(scope, rangeKey);
    if (background) setRefreshing(true);
    else setLoading(true);
    const finish = () => {
      setLoading(false);
      setRefreshing(false);
      setUpdatedAt(Date.now());
    };
    setError("");

    const [overviewResult, optionsResult] = await Promise.allSettled([fetchArgusInstanceOverview(), fetchSignalFilterOptions()]);
    if (generation !== generationRef.current) return;
    const overview = overviewResult.status === "fulfilled" ? overviewResult.value : null;
    const options = optionsResult.status === "fulfilled" ? optionsResult.value : null;
    const bootstrapErrors = [overviewResult, optionsResult]
      .filter((result): result is PromiseRejectedResult => result.status === "rejected")
      .map((result) => errorMessage(result.reason));

    if (!options) {
      setData({ ...emptyData, overview });
      setError(bootstrapErrors.join("；") || "无法读取 Argus 数据范围");
      finish();
      return;
    }

    const range = resolveRange(rangeKey, options.dataRange.end);
    const summaryPromise = fetchInstanceSummary({ instrument: options.instruments[0], ...range });
    if (scope === ALL_INSTANCES) {
      const summaryResult = await summaryPromise.then(
        (value) => ({ value, error: "" }),
        (reason: unknown) => ({ value: null, error: errorMessage(reason) }),
      );
      if (generation !== generationRef.current) return;
      const sameScope = dataScopeRef.current === scopeOf(scope, rangeKey);
      setData((previous) => ({
        ...emptyData,
        overview: overview ?? (sameScope ? previous.overview : null),
        options,
        summary: summaryResult.value ?? (sameScope ? previous.summary : null),
      }));
      dataScopeRef.current = scopeOf(scope, rangeKey);
      setError([...bootstrapErrors, summaryResult.error].filter(Boolean).join("；"));
      finish();
      return;
    }

    const instrument = options.instruments[0] || "BTCUSDT";
    const [summaryResult, equityResult, gateResult, signalsResult, snapshotResult, timelineResult] = await Promise.allSettled([
      summaryPromise,
      fetchEquityCurve({ instanceKey: scope, ...range, bucketSeconds: EQUITY_BUCKET_SECONDS }),
      fetchGateStats({ instanceKey: scope, instrument, ...range }),
      fetchSignals({ instanceKey: scope, ...range, category: "all", order: "ts_desc", pageIndex: 1, pageSize: RECENT_SIGNAL_LIMIT }),
      fetchPublishedArgusConfig(scope),
      fetchMarketTimeline({ instanceKey: scope, instrument, interval: "1m", platformCode: "deepcoin", comparePlatformCode: "binance", start: range.start || options.dataRange.start, end: range.end || options.dataRange.end }),
    ]);
    if (generation !== generationRef.current) return;
    const rejected = [summaryResult, equityResult, gateResult, signalsResult, snapshotResult, timelineResult]
      .filter((result): result is PromiseRejectedResult => result.status === "rejected")
      .map((result) => errorMessage(result.reason));
    // 本轮某一项失败时沿用上一轮的值，不要清成 null。
    //
    // 这页是挂在屏幕上长时间不动的监控页，10 秒一轮；一次网络抖动就把已经拿到的
    // 数据抹掉，页面会整块塌掉再长回来（权益曲线那块还会因此**卸载重建**）。
    // 失败本身由上方的告警条如实说明，数据保持上一轮的，比闪成空白有用。
    const sameScope = dataScopeRef.current === scopeOf(scope, rangeKey);
    const keep = <T,>(next: T | null, previous: T): T | null => (next ?? (sameScope ? previous : null));
    setData((previous) => ({
      overview: keep(overview, previous.overview),
      options: keep(options, previous.options),
      summary: keep(valueOf(summaryResult), previous.summary),
      equity: keep(valueOf(equityResult), previous.equity),
      gateStats: keep(valueOf(gateResult), previous.gateStats),
      signals: valueOf(signalsResult)?.data ?? (sameScope ? previous.signals : []),
      snapshot: keep(valueOf(snapshotResult), previous.snapshot),
      timeline: keep(valueOf(timelineResult), previous.timeline),
    }));
    dataScopeRef.current = scopeOf(scope, rangeKey);
    setError([...bootstrapErrors, ...rejected].join("；"));
    finish();
  }, [rangeKey, scope]);

  useEffect(() => { void refresh(); }, [refresh]);

  useEffect(() => {
    const timer = window.setInterval(() => { void refresh({ silent: true }); }, OVERVIEW_REFRESH_INTERVAL);
    return () => window.clearInterval(timer);
  }, [refresh]);

  return { scope, setScope, rangeKey, setRangeKey, loading, refreshing, updatedAt, error, refresh, ...data };
}

function valueOf<T>(result: PromiseSettledResult<T>): T | null {
  return result.status === "fulfilled" ? result.value : null;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "读取数据失败";
}
