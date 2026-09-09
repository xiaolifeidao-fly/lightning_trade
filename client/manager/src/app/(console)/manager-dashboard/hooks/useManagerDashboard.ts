"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { fetchArgusInstanceOverview, type ArgusInstanceOverview } from "@/components/argus/argus-instance.api";
import {
  fetchInstanceSummary,
  type InstanceSummaryResult,
  type InstanceAccount,
} from "../../argus-dashboard/api/argus-dashboard.api";
import { fetchEpisodeStats, type EpisodeStats } from "../../argus-signals/api/argus-signals.api";

/** 30 秒轮询：这一页是落地页，会被长时间挂在屏幕上。 */
const REFRESH_INTERVAL = 30_000;

/** 本页统计窗口：近 24 小时。 */
const WINDOW_HOURS = 24;

export interface AccountRow extends InstanceAccount {
  instanceKey: string;
  instanceName: string;
}

export interface ManagerDashboardData {
  overview: ArgusInstanceOverview | null;
  summary: InstanceSummaryResult | null;
  episodes: EpisodeStats | null;
}

const empty: ManagerDashboardData = { overview: null, summary: null, episodes: null };

/** 把本地时间格式化成后端要的 'YYYY-MM-DD HH:mm:ss'（事件接口全程按串走，不带时区）。 */
function wallClock(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/**
 * 数据总览的数据编排。
 *
 * 口径上的两条硬规矩，别为了凑一个大数字破掉：
 *
 *  1. **可加的才加**。权益、净持仓、信号数、成交数是可加的；胜率与开仓率不是
 *     ——开仓率必须用 Σ成交/Σ信号 重算，绝不能把各实例的比率再平均一次。
 *     胜率直接用 episode-stats 的跨实例口径（它自己会说明哪些不计入）。
 *  2. **拿不到就显示"—"**，不要用 0 顶。0 是一个有含义的值（真的没成交），
 *     和"这个数取不到"必须区分开，否则页面会重新变成假数据。
 */
export function useManagerDashboard() {
  const [data, setData] = useState<ManagerDashboardData>(empty);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const generationRef = useRef(0);

  const refresh = useCallback(async () => {
    const generation = ++generationRef.current;
    setLoading(true);

    const end = new Date();
    const start = new Date(end.getTime() - WINDOW_HOURS * 3600_000);
    const range = { start: wallClock(start), end: wallClock(end) };

    // onlyEnabled=false：要能算出「在跑 / 已登记」，停用的实例也得进分母。
    const [overviewRes, summaryRes, episodeRes] = await Promise.allSettled([
      fetchArgusInstanceOverview(false),
      fetchInstanceSummary(range),
      fetchEpisodeStats(range),
    ]);
    if (generation !== generationRef.current) return;

    const failed: string[] = [];
    const pick = <T,>(r: PromiseSettledResult<T>, label: string): T | null => {
      if (r.status === "fulfilled") return r.value;
      failed.push(`${label}（${r.reason instanceof Error ? r.reason.message : String(r.reason)}）`);
      return null;
    };

    setData({
      overview: pick(overviewRes, "实例总览"),
      summary: pick(summaryRes, "事件汇总"),
      episodes: pick(episodeRes, "持仓统计"),
    });
    setError(failed.length ? `部分数据读取失败：${failed.join("；")}。缺失项以「—」显示，不按 0 处理。` : "");
    setLoading(false);
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const timer = window.setInterval(() => void refresh(), REFRESH_INTERVAL);
    return () => window.clearInterval(timer);
  }, [refresh]);

  return { ...data, loading, error, refresh, windowHours: WINDOW_HOURS };
}

/** sumOf 对可空字段求和：全为 null 时返回 null，而不是 0。 */
export function sumOf<T>(rows: T[], get: (row: T) => number | null | undefined): number | null {
  let seen = false;
  let total = 0;
  for (const row of rows) {
    const v = get(row);
    if (v === null || v === undefined || Number.isNaN(v)) continue;
    seen = true;
    total += v;
  }
  return seen ? total : null;
}

/** 把各实例的账户拍平，带上实例信息，供明细表使用。 */
export function flattenAccounts(summary: InstanceSummaryResult | null): AccountRow[] {
  if (!summary) return [];
  return summary.instances.flatMap((inst) =>
    (inst.accounts ?? []).map((acc) => ({ ...acc, instanceKey: inst.instanceKey, instanceName: inst.instanceName })),
  );
}
