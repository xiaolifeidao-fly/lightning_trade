"use client";

import { LightweightChart } from "@/components/charts/LightweightChart";
import { chartPalette, wallClockToChartTime } from "@/components/charts/chartTheme";
import { Empty } from "antd";
import { LineSeries, type IChartApi, type LineData, type UTCTimestamp } from "lightweight-charts";
import { useEffect, useMemo } from "react";
import type { BacktestRunDetail } from "../api/argus-backtest.api";
import { seriesColor } from "../constants";
import { removeChartSeries } from "@/components/charts/chartLifecycle";

interface EquityCurveProps {
  details: Record<number, BacktestRunDetail>;
  labels: Record<number, string>;
  selectedRunId: number | null;
}

interface EquitySeries {
  id: number;
  label: string;
  color: string;
  data: LineData<UTCTimestamp>[];
  emphasis: boolean;
}

/**
 * 累计净值按每笔持仓的平仓时刻累加；未平仓的浮动盈亏不会伪装成已实现净值。
 * 同秒关仓时合并为最后一个累计值，符合 lightweight-charts 的严格递增要求。
 */
export function EquityCurve({ details, labels, selectedRunId }: EquityCurveProps) {
  const series = useMemo<EquitySeries[]>(() => {
    return Object.values(details)
      .map((detail, index) => {
        const rows = [...(detail.trades ?? [])]
          .filter((trade) => trade.closedAt && trade.status !== "open")
          .sort((left, right) => left.closedAt.localeCompare(right.closedAt));
        let cumulative = 0;
        const points: LineData<UTCTimestamp>[] = [];
        let lastTime = -1;
        for (const trade of rows) {
          const time = wallClockToChartTime(trade.closedAt);
          if (time === null || time < lastTime) continue;
          cumulative += trade.netPnl ?? 0;
          const point = { time, value: cumulative };
          if (time === lastTime) points[points.length - 1] = point;
          else points.push(point);
          lastTime = time;
        }
        return {
          id: detail.run.id,
          label: labels[detail.run.id] ?? `run #${detail.run.id}`,
          color: seriesColor(index),
          data: points,
          emphasis: detail.run.id === selectedRunId,
        };
      })
      .filter((item) => item.data.length > 0);
  }, [details, labels, selectedRunId]);

  if (series.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="完成含已平仓逐笔后，这里显示累计已实现净值" />;
  }

  return (
    <div>
      <div className="manager-argus-curve-legend">
        {series.map((item) => (
          <span key={item.id} className={item.emphasis ? "is-active" : undefined}>
            <i style={{ background: item.color }} />
            {item.label}
          </span>
        ))}
      </div>
      <LightweightChart
        height={284}
        options={{
          timeScale: { timeVisible: true, secondsVisible: false, borderColor: chartPalette.border },
          localization: { priceFormatter: (value: number) => `${value.toFixed(2)} U` },
        }}
      >
        {(chart) => <EquitySeriesLayer chart={chart} series={series} />}
      </LightweightChart>
    </div>
  );
}

function EquitySeriesLayer({ chart, series }: { chart: IChartApi; series: EquitySeries[] }) {
  useEffect(() => {
    const created = series.map((item) => {
      const line = chart.addSeries(LineSeries, {
        color: item.color,
        lineWidth: item.emphasis ? 3 : 2,
        priceLineVisible: false,
        lastValueVisible: true,
        crosshairMarkerVisible: item.emphasis,
        title: item.label,
      });
      line.setData(item.data);
      return line;
    });
    chart.timeScale().fitContent();
    return () => removeChartSeries(chart, ...created);
  }, [chart, series]);

  return null;
}
