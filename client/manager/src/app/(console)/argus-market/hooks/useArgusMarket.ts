"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useArgusInstanceScope } from "@/components/argus/instanceScope";
import {
  fetchArgusInstances,
  fetchPublishedArgusConfig,
  type ArgusConfigSnapshot,
  type ArgusInstance,
} from "../../argus-config/api/argus-config.api";
import {
  backfillKlineRange,
  fetchMarketFilterOptions,
  fetchMarketTimeline,
  fetchSignalSlice,
  fetchTriggerPage,
  type BackfillRangeResult,
  type MarketTimeline,
  type SignalEvent,
  type SignalSlice,
} from "../api/argus-market.api";
import {
  MAX_TRIGGER_PAGES,
  RANGE_OPTIONS,
  TRIGGER_PAGE_SIZE,
  type MarketInterval,
  type MarketSource,
  type TriggerKind,
} from "../constants";

/** 本地墙钟串 `YYYY-MM-DD HH:mm:ss`，与 argus-event 接口的时间口径一致。 */
function toWallClock(date: Date): string {
  const pad = (v: number) => String(v).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
}

/** 墙钟串 → 本地 Date。回填接口要 RFC3339，只能从这里换算。 */
export function wallClockToDate(wallClock: string): Date | null {
  const matched = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/.exec(wallClock.trim());
  if (!matched) return null;
  const [, y, mo, d, h, mi, s] = matched;
  return new Date(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s));
}

export interface MarketFilters {
  instrument: string;
  interval: MarketInterval;
  source: MarketSource;
  rangeKey: string;
  kinds: Record<TriggerKind, boolean>;
}

/** 单源 = 该源的蜡烛；双源 = 币安蜡烛 + DeepCoin 收盘对比线。 */
function resolvePlatforms(source: MarketSource): { primary: string; compare?: string } {
  if (source === "deepcoin") return { primary: "deepcoin" };
  if (source === "binance") return { primary: "binance" };
  return { primary: "binance", compare: "deepcoin" };
}

export function useArgusMarket() {
  // 实例选择走全局作用域，与顶栏选择器和其余 Argus 页面共用一份。本页不接受
  // 「全部实例」：净持仓阶梯、阈值线与上限线都是实例内的量，混排读不出结论。
  const [instanceKey, setInstanceKey] = useArgusInstanceScope();
  const [instances, setInstances] = useState<ArgusInstance[]>([]);
  const [instruments, setInstruments] = useState<string[]>([]);
  const [snapshot, setSnapshot] = useState<ArgusConfigSnapshot | null>(null);
  const [timeline, setTimeline] = useState<MarketTimeline | null>(null);
  const [triggers, setTriggers] = useState<SignalEvent[]>([]);
  const [triggerTotal, setTriggerTotal] = useState(0);
  const [bootstrapping, setBootstrapping] = useState(true);
  const [loading, setLoading] = useState(false);
  const [backfilling, setBackfilling] = useState(false);
  const [loadError, setLoadError] = useState<string>("");
  const [bootError, setBootError] = useState<string>("");
  // 下钻状态：选中的触发点与它的秒级切片。selectedEventId 同时驱动图上标记的高亮，
  // 所以它跟切片抽屉的开关是两件事——关掉抽屉不该把图上的选中也一起清掉。
  const [selectedEventId, setSelectedEventId] = useState<number | null>(null);
  const [sliceOpen, setSliceOpen] = useState(false);
  const [slice, setSlice] = useState<SignalSlice | null>(null);
  const [sliceLoading, setSliceLoading] = useState(false);
  const [sliceError, setSliceError] = useState<string>("");
  const [filters, setFilters] = useState<MarketFilters>({
    instrument: "",
    interval: "1m",
    source: "both",
    rangeKey: "latest",
    kinds: { open: true, gate_block: true, cap_skip: true, trend_skip: true },
  });

  // 并发保护：切周期/切实例时旧请求可能后到，用序号丢弃过期结果。
  const requestSeq = useRef(0);
  // bootstrap 只跑一次，用 ref 读当前作用域；把 instanceKey 放进它的依赖会让切实例
  // 重新拉一遍筛选项，而筛选项本来就与实例无关。
  const instanceKeyRef = useRef(instanceKey);
  instanceKeyRef.current = instanceKey;

  /**
   * 实例与币种优先取 /argus-event/filter-options——它列的是**真有事件数据**的实例，
   * 空实例进不了选择器就不会出现「选了半天全是空图」。事件表还没数据时回退到
   * argus_instance 注册表，页面仍然可用（只是全是空态）。
   */
  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const [options, registry] = await Promise.all([
          fetchMarketFilterOptions().catch(() => null),
          fetchArgusInstances().catch(() => [] as ArgusInstance[]),
        ]);
        if (!alive) return;
        const fromEvents: ArgusInstance[] = (options?.instances ?? []).map((item) => {
          const registered = registry.find((r) => r.instanceKey === item.instanceKey);
          return (
            registered ?? ({ id: 0, instanceKey: item.instanceKey, instanceName: item.instanceName || item.instanceKey, enabled: 1 } as ArgusInstance)
          );
        });
        const merged = fromEvents.length > 0 ? fromEvents : registry;
        if (merged.length === 0) {
          setBootError("没有可用的实例：argus_instance 未导入，事件表里也还没有任何数据。");
        }
        setInstances(merged);
        // 全局作用域停在「全部实例」或指向一个本页没有数据的实例时，落到第一个可用实例
        // 并把这个选择写回全局——否则页面显示的实例和顶栏选择器对不上。
        if (!merged.some((item) => item.instanceKey === instanceKeyRef.current) && merged[0]) {
          setInstanceKey(merged[0].instanceKey);
        }
        const symbols = options?.instruments ?? [];
        setInstruments(symbols);
        setFilters((prev) => ({ ...prev, instrument: prev.instrument || symbols[0] || "BTCUSDT" }));
      } catch (error) {
        if (alive) setBootError(error instanceof Error ? error.message : "读取实例与币种失败");
      } finally {
        if (alive) setBootstrapping(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  // 阈值线与上限线跟随实例：值取该实例**已发布**的配置快照，不在前端写死
  // 5bp / 3bp——三实例的阈值本来就不同，写死等于把对照组画错。
  useEffect(() => {
    if (!instanceKey) {
      setSnapshot(null);
      return;
    }
    let alive = true;
    void fetchPublishedArgusConfig(instanceKey)
      .then((data) => {
        if (alive) setSnapshot(data);
      })
      .catch(() => {
        if (alive) setSnapshot(null);
      });
    return () => {
      alive = false;
    };
  }, [instanceKey]);

  const load = useCallback(async () => {
    if (!instanceKey || !filters.instrument) return;
    const seq = ++requestSeq.current;
    setLoading(true);
    setLoadError("");
    const range = RANGE_OPTIONS.find((item) => item.key === filters.rangeKey) ?? RANGE_OPTIONS[0];
    // hours=null 时不传时间界，由服务端按「已入库数据的最新时刻」回推——
    // 历史数据集（如 8.18–8.21）用 now 去框会框到空窗口。
    const now = new Date();
    const start = range.hours == null ? "" : toWallClock(new Date(now.getTime() - range.hours * 3600_000));
    const end = range.hours == null ? "" : toWallClock(now);
    const { primary, compare } = resolvePlatforms(filters.source);

    try {
      const nextTimeline = await fetchMarketTimeline({
        instanceKey,
        instrument: filters.instrument,
        interval: filters.interval,
        platformCode: primary,
        comparePlatformCode: compare,
        start,
        end,
      });
      if (seq !== requestSeq.current) return;
      setTimeline(nextTimeline);

      // 触发点按服务端解析出来的实际窗口取，保证与 K 线是同一段时间；
      // 逐页拉到上限为止，不做分钟级折叠（同一分钟内多次触发必须各自成点）。
      const windowStart = nextTimeline.window?.start ?? "";
      const windowEnd = nextTimeline.window?.end ?? "";
      if (!windowStart || !windowEnd) {
        setTriggers([]);
        setTriggerTotal(0);
        return;
      }
      const collected: SignalEvent[] = [];
      let total = 0;
      for (let pageIndex = 1; pageIndex <= MAX_TRIGGER_PAGES; pageIndex += 1) {
        const page = await fetchTriggerPage({
          instanceKey,
          instrument: filters.instrument,
          start: windowStart,
          end: windowEnd,
          pageIndex,
          pageSize: TRIGGER_PAGE_SIZE,
        });
        if (seq !== requestSeq.current) return;
        total = page.total;
        collected.push(...page.data);
        if (collected.length >= page.total || page.data.length < TRIGGER_PAGE_SIZE) break;
      }
      setTriggers(collected);
      setTriggerTotal(total);
    } catch (error) {
      if (seq !== requestSeq.current) return;
      setTimeline(null);
      setTriggers([]);
      setTriggerTotal(0);
      setLoadError(error instanceof Error ? error.message : "加载行情与触发点失败");
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [instanceKey, filters.instrument, filters.interval, filters.rangeKey, filters.source]);

  useEffect(() => {
    void load();
  }, [load]);

  /**
   * 回填当前视图窗口内缺失的 K 线。按窗口覆盖率补，不是「补最近 N 根」——
   * 后者对整段落在过去的窗口一根都补不动。
   */
  const backfill = useCallback(async (): Promise<BackfillRangeResult> => {
    if (!timeline) throw new Error("当前没有可回填的窗口");
    const startAt = wallClockToDate(timeline.window?.start ?? "");
    const endAt = wallClockToDate(timeline.window?.end ?? "");
    if (!startAt || !endAt) throw new Error("当前窗口为空，先选一个有数据的时间范围");
    const { primary, compare } = resolvePlatforms(filters.source);
    setBackfilling(true);
    try {
      const result = await backfillKlineRange({
        platformCodes: compare ? [primary, compare] : [primary],
        symbol: timeline.symbol || filters.instrument,
        intervals: [filters.interval],
        // 这个接口按 RFC3339 解析；传裸墙钟串会被当成 UTC，整体错开时区偏移。
        start: startAt.toISOString(),
        end: endAt.toISOString(),
      });
      await load();
      return result;
    } finally {
      setBackfilling(false);
    }
  }, [timeline, filters.source, filters.interval, filters.instrument, load]);

  const instance = useMemo(
    () => instances.find((item) => item.instanceKey === instanceKey) ?? null,
    [instances, instanceKey],
  );

  /** 信号阈值（bp）：取该实例当前币种的 monitor symbol 行。 */
  const signalThresholdBp = useMemo(() => {
    if (!snapshot) return null;
    const symbols = snapshot.monitorSymbols ?? [];
    const matched =
      symbols.find((item) => (item.symbol || "").toUpperCase() === filters.instrument.toUpperCase()) ?? symbols[0];
    return matched?.signalThreshold ?? null;
  }, [snapshot, filters.instrument]);

  /**
   * 净持仓上限线：净持仓阶梯是该实例**全部账户之和**（timeline 的口径），
   * 所以上限线也必须是各账户 maxContracts 之和，不能只取某一个账户的值。
   */
  const positionCap = useMemo(() => {
    const risks = snapshot?.accountRisks ?? [];
    if (risks.length === 0) return null;
    const total = risks.reduce((sum, item) => sum + (item.maxContracts || 0), 0);
    return total > 0 ? total : null;
  }, [snapshot]);

  /** 图上标记只画勾选中的那几类；偏离副图与计数徽标仍用全量，不受勾选影响。 */
  const visibleTriggers = useMemo(
    () => triggers.filter((item) => filters.kinds[item.event as TriggerKind] === true),
    [triggers, filters.kinds],
  );

  const selectedTrigger = useMemo(
    () => triggers.find((item) => item.eventId === selectedEventId) ?? null,
    [triggers, selectedEventId],
  );

  /** 下钻到触发瞬间的 ±60 秒切片。切片按 eventId 取，同一分钟内的第二次触发是另一个 id。 */
  const openSlice = useCallback((event: SignalEvent) => {
    setSelectedEventId(event.eventId);
    setSliceOpen(true);
    setSlice(null);
    setSliceError("");
    setSliceLoading(true);
    void fetchSignalSlice(event.eventId)
      .then((data) => setSlice(data))
      .catch((error: unknown) => setSliceError(error instanceof Error ? error.message : "读取秒级切片失败"))
      .finally(() => setSliceLoading(false));
  }, []);

  const closeSlice = useCallback(() => setSliceOpen(false), []);

  const updateFilters = useCallback((patch: Partial<MarketFilters>) => {
    setFilters((prev) => ({ ...prev, ...patch }));
  }, []);

  const toggleKind = useCallback((kind: TriggerKind) => {
    setFilters((prev) => ({ ...prev, kinds: { ...prev.kinds, [kind]: !prev.kinds[kind] } }));
  }, []);

  return {
    instances,
    instance,
    instanceKey,
    setInstanceKey,
    instruments,
    snapshot,
    signalThresholdBp,
    positionCap,
    filters,
    updateFilters,
    toggleKind,
    timeline,
    triggers,
    visibleTriggers,
    triggerTotal,
    selectedEventId,
    setSelectedEventId,
    selectedTrigger,
    sliceOpen,
    slice,
    sliceLoading,
    sliceError,
    openSlice,
    closeSlice,
    /** 命中了单次拉取上限，图上与表里都不是窗口内的全部触发点。 */
    triggersTruncated: triggerTotal > triggers.length,
    bootstrapping,
    loading,
    backfilling,
    loadError,
    bootError,
    refresh: load,
    backfill,
  };
}
