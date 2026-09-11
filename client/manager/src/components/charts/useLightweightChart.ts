"use client";

import { createChart, type ChartOptions, type DeepPartial, type IChartApi } from "lightweight-charts";
import { useEffect, useRef, useState, type RefObject } from "react";
import { markChartDisposed } from "./chartLifecycle";
import { baseChartOptions } from "./chartTheme";

/**
 * 创建并托管一个 lightweight-charts 实例。
 *
 * 职责只有三件：建图（挂管理端深色主题）、跟随容器宽度、卸载时销毁。序列怎么建、
 * 数据怎么灌由调用方自己在 `useEffect([chart, data])` 里做——图表实例只建一次，
 * 数据变化不重建，缩放与十字光标位置就不会被刷掉。
 *
 * @param containerRef 图表挂载的 DOM 容器
 * @param height 图表总高度（多 pane 时是所有 pane 的总高）
 * @param options 覆盖基础主题的配置
 */
export function useLightweightChart(
  containerRef: RefObject<HTMLDivElement>,
  height: number,
  options?: DeepPartial<ChartOptions>,
): IChartApi | null {
  const [chart, setChart] = useState<IChartApi | null>(null);
  // 建图只跑一次，但要用到最新的 options，用 ref 兜住避免把它塞进依赖。
  const optionsRef = useRef(options);
  optionsRef.current = options;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const instance = createChart(container, {
      ...baseChartOptions(),
      ...optionsRef.current,
      width: container.clientWidth || 600,
      height,
    });
    setChart(instance);

    // ResizeObserver 而不是 window.resize：侧边栏折叠、Tab 切换都会改容器宽度，
    // 但不触发 window.resize。
    const observer = new ResizeObserver((entries) => {
      const width = Math.floor(entries[0]?.contentRect.width ?? 0);
      if (width > 0) instance.applyOptions({ width });
    });
    observer.observe(container);

    return () => {
      observer.disconnect();
      setChart(null);
      // 先打标记再销毁：卸载时本 cleanup 早于子组件的 cleanup，子组件那边靠这个
      // 标记跳过 removeSeries；否则它会对着已销毁的图表删序列，抛异常整页崩。
      markChartDisposed(instance);
      instance.remove();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [containerRef]);

  // 高度与其余配置走 applyOptions 增量更新，不重建图表。
  useEffect(() => {
    if (!chart) return;
    chart.applyOptions({ ...options, height });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chart, height]);

  return chart;
}
