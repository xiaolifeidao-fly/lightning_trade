"use client";

import { LineSeries, type IChartApi, type ISeriesApi, type UTCTimestamp } from "lightweight-charts";
import { useEffect } from "react";
import { LightweightChart } from "@/components/charts/LightweightChart";
import { chartPalette, wallClockToChartTime } from "@/components/charts/chartTheme";
import type { EquitySeries } from "../api/argus-dashboard.api";
import { EQUITY_SERIES_COLORS } from "../constants";

interface EquityChartProps {
  series: EquitySeries[];
}

interface EquityChartPoint {
  time: UTCTimestamp;
  value: number;
}

/**
 * 单实例的权益轨迹。
 *
 * 同一实例可能有 champion / challenger 两个账户，直接比较绝对权益会被账户本金放大；
 * 因此前端画服务端已经给出的「相对首个权益的变动百分比」，把各账户放到同一尺度。
 */
export function EquityChart({ series }: EquityChartProps) {
  return (
    <LightweightChart
      height={318}
      options={{
        rightPriceScale: { scaleMargins: { top: 0.14, bottom: 0.14 } },
        timeScale: { timeVisible: true, secondsVisible: false },
      }}
    >
      {(chart) => <EquitySeriesLayer chart={chart} series={series} />}
    </LightweightChart>
  );
}

function EquitySeriesLayer({ chart, series }: EquityChartProps & { chart: IChartApi }) {
  useEffect(() => {
    const created: ISeriesApi<"Line">[] = [];

    series.forEach((item, index) => {
      const points = toPoints(item);
      if (points.length === 0) return;
      const line = chart.addSeries(LineSeries, {
        color: EQUITY_SERIES_COLORS[index % EQUITY_SERIES_COLORS.length],
        lineWidth: 2,
        priceLineVisible: false,
        lastValueVisible: true,
        crosshairMarkerVisible: true,
        title: item.variant || item.accountLabel,
        priceFormat: { type: "price", precision: 2, minMove: 0.01 },
      });
      line.setData(points);
      created.push(line);
    });

    if (created.length > 0) chart.timeScale().fitContent();
    return () => {
      created.forEach((line) => chart.removeSeries(line));
    };
  }, [chart, series]);

  return null;
}

function toPoints(series: EquitySeries): EquityChartPoint[] {
  const byTime = new Map<number, EquityChartPoint>();
  for (const item of series.points ?? []) {
    const time = wallClockToChartTime(item.time);
    if (time === null || item.changePct === null || !Number.isFinite(item.changePct)) continue;
    byTime.set(time, { time, value: item.changePct });
  }
  return Array.from(byTime.values()).sort((left, right) => left.time - right.time);
}

/** 保留这个导出供未来图例复用，也让「0% 基线」的颜色有单一来源。 */
export const EQUITY_BASELINE_COLOR = chartPalette.neutral;
