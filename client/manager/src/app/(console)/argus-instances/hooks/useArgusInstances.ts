"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchArgusInstanceOverview, type ArgusInstanceOverview } from "@/components/argus/argus-instance.api";
import { fetchPublishedArgusConfig, type ArgusConfigSnapshot } from "../../argus-config/api/argus-config.api";
import { fetchSignalFilterOptions } from "../../argus-signals/api/argus-signals.api";
import { fetchInstanceSummary, type InstanceSummaryResult } from "../../argus-dashboard/api/argus-dashboard.api";
import { resolveRange } from "../../argus-dashboard/constants";

interface InstancesData {
  overview: ArgusInstanceOverview | null;
  summary: InstanceSummaryResult | null;
  snapshots: Record<string, ArgusConfigSnapshot | null>;
}

const emptyData: InstancesData = { overview: null, summary: null, snapshots: {} };

/** 实例页统一拉「注册 + 运行 + 快照」三种只读状态，避免每张卡各发一次请求。 */
export function useArgusInstances() {
  const [data, setData] = useState<InstancesData>(emptyData);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const generationRef = useRef(0);

  const refresh = useCallback(async () => {
    const generation = ++generationRef.current;
    setLoading(true);
    setError("");
    const [overviewResult, optionsResult] = await Promise.allSettled([fetchArgusInstanceOverview(false), fetchSignalFilterOptions()]);
    if (generation !== generationRef.current) return;
    const overview = overviewResult.status === "fulfilled" ? overviewResult.value : null;
    const options = optionsResult.status === "fulfilled" ? optionsResult.value : null;
    const errors = [overviewResult, optionsResult]
      .filter((result): result is PromiseRejectedResult => result.status === "rejected")
      .map((result) => toError(result.reason));

    if (!overview) {
      setData(emptyData);
      setError(errors.join("；") || "无法读取实例状态");
      setLoading(false);
      return;
    }
    const range = options ? resolveRange("d1", options.dataRange.end) : {};
    const [summaryResult, ...snapshotsResult] = await Promise.allSettled([
      fetchInstanceSummary({ instrument: options?.instruments[0], ...range }),
      ...overview.instances.map((item) => fetchPublishedArgusConfig(item.instanceKey)),
    ]);
    if (generation !== generationRef.current) return;
    const snapshots: Record<string, ArgusConfigSnapshot | null> = {};
    overview.instances.forEach((item, index) => {
      const result = snapshotsResult[index];
      snapshots[item.instanceKey] = result?.status === "fulfilled" ? result.value : null;
      if (result?.status === "rejected") errors.push(`${item.instanceKey}: ${toError(result.reason)}`);
    });
    if (summaryResult.status === "rejected") errors.push(toError(summaryResult.reason));
    setData({ overview, summary: summaryResult.status === "fulfilled" ? summaryResult.value : null, snapshots });
    setError(errors.join("；"));
    setLoading(false);
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);
  return { ...data, loading, error, refresh };
}

function toError(error: unknown): string {
  return error instanceof Error ? error.message : "读取数据失败";
}
