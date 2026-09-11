"use client";

import type { IChartApi, ISeriesApi, SeriesType } from "lightweight-charts";

/**
 * 图表销毁标记。
 *
 * 为什么需要它：React 卸载一棵子树时，**父组件的 effect cleanup 先于子组件**执行。
 * 我们的图表是「父建图、子建序列」的结构：
 *
 *   LightweightChart(useLightweightChart)   ← cleanup 里 instance.remove()
 *     └─ XxxSeriesLayer                     ← cleanup 里 chart.removeSeries(...)
 *
 * 卸载时父先把图表销毁，子再去删序列，lightweight-charts 的 ensureDefined 直接抛
 * `Error: Value is undefined`。这个异常会冒泡到全站错误边界，把整页换成错误页——
 * 实测路径：轮询失败 → equity 变 null → `{equity?.series?.length ? <图表/> : <空态/>}`
 * 走空态分支 → 图表卸载 → 整页崩。也就是说**一次网络抖动就能把页面打死**。
 *
 * 序列本来就属于图表：图表销毁时序列跟着没了，子组件那次 removeSeries 是多余的。
 * 所以这里只做一件事——让子组件能问出「图表还在不在」。
 */
const DISPOSED = Symbol.for("manager.chart.disposed");

type DisposableChart = IChartApi & { [DISPOSED]?: boolean };

/** 由建图方在 remove() 之前调用。放在 remove() 之前是为了 remove() 抛错时标记也已落下。 */
export function markChartDisposed(chart: IChartApi): void {
  (chart as DisposableChart)[DISPOSED] = true;
}

export function isChartDisposed(chart: IChartApi | null | undefined): boolean {
  return !chart || (chart as DisposableChart)[DISPOSED] === true;
}

/**
 * 删除序列的安全版本，图表已销毁时整体跳过。
 *
 * 所有「子组件在 cleanup 里删自己建的序列」的地方都必须走它，不要直接调
 * chart.removeSeries —— 直接调的那一版会在卸载时抛异常整页崩。
 */
export function removeChartSeries(
  chart: IChartApi,
  ...series: Array<ISeriesApi<SeriesType> | null | undefined>
): void {
  if (isChartDisposed(chart)) return;
  for (const item of series) {
    if (item) chart.removeSeries(item);
  }
}
