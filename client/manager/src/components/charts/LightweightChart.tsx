"use client";

import type { ChartOptions, DeepPartial, IChartApi } from "lightweight-charts";
import { useRef, type ReactNode } from "react";
import { useLightweightChart } from "./useLightweightChart";

interface LightweightChartProps {
  /** 图表总高度（多 pane 时是所有 pane 的总高）。 */
  height: number;
  options?: DeepPartial<ChartOptions>;
  className?: string;
  /**
   * 图表就绪后渲染的子内容。图表实例通过 children 函数传出去，
   * 由调用方在自己的 effect 里建序列——这样序列的生命周期跟着它自己的数据走。
   */
  children?: (chart: IChartApi) => ReactNode;
}

/**
 * lightweight-charts 的容器组件。
 *
 * 用法：
 * ```tsx
 * <LightweightChart height={520}>
 *   {(chart) => <CandleLayer chart={chart} data={klines} />}
 * </LightweightChart>
 * ```
 * 只需要图表实例、不需要子组件时直接用 {@link useLightweightChart}。
 */
export function LightweightChart({ height, options, className, children }: LightweightChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chart = useLightweightChart(containerRef, height, options);

  return (
    <div className={className} style={{ position: "relative", width: "100%" }}>
      <div ref={containerRef} style={{ width: "100%", height }} />
      {chart ? children?.(chart) : null}
    </div>
  );
}
