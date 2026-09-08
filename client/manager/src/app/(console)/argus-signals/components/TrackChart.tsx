"use client";

import { LightweightChart } from "@/components/charts/LightweightChart";
import { chartPalette, wallClockToChartTime } from "@/components/charts/chartTheme";
import { Empty } from "antd";
import { LineSeries, LineStyle, type IChartApi, type LineData, type UTCTimestamp } from "lightweight-charts";
import { useEffect, useMemo } from "react";

export interface TrackPoint {
  ts: string;
  value: number | null;
}

interface TrackChartProps {
  points: TrackPoint[];
  height?: number;
  color?: string;
  /** 数值后缀，%（ROI 轨迹）或 U（浮盈心跳）。 */
  unit?: string;
  /**
   * discrete=true 时画成虚线 + 点标记。
   *
   * ROI% 只在事件里被离散地写下来（gate_block / loss_alert / 出场事件），中间
   * 没有任何观测。画成实线会让人以为那段也测过——那正是"看着精确其实是拟合"的
   * 那类伪影。upl 来自约 30 秒一条的心跳，才配画实线。
   */
  discrete?: boolean;
  /** 零轴参考线，浮盈类曲线要有。 */
  zeroLine?: boolean;
  emptyText?: string;
}

/** 一条挂在墙钟时间轴上的轨迹。ROI 与 upl 各画一张，两者不同单位、绝不叠在一起。 */
export function TrackChart({
  points,
  height = 190,
  color = chartPalette.up,
  unit = "%",
  discrete = false,
  zeroLine = true,
  emptyText = "这段区间没有可用观测",
}: TrackChartProps) {
  const data = useMemo<LineData<UTCTimestamp>[]>(() => {
    const rows: LineData<UTCTimestamp>[] = [];
    let lastTime = -1;
    for (const point of points) {
      if (point.value === null || point.value === undefined) continue;
      const time = wallClockToChartTime(point.ts);
      if (time === null) continue;
      // lightweight-charts 要求时间严格递增；同秒内的多条观测只保留最后一条，
      // 数据本身仍在时间轴表格里，图上不需要重复点。
      if (time === lastTime) {
        rows[rows.length - 1] = { time, value: point.value };
        continue;
      }
      if (time < lastTime) continue;
      rows.push({ time, value: point.value });
      lastTime = time;
    }
    return rows;
  }, [points]);

  if (data.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} style={{ margin: "24px 0" }} />;
  }

  return (
    <LightweightChart
      height={height}
      options={{
        timeScale: { timeVisible: true, secondsVisible: false, borderColor: chartPalette.border },
        localization: { priceFormatter: (value: number) => `${value.toFixed(1)}${unit}` },
      }}
    >
      {(chart) => (
        <TrackSeries chart={chart} data={data} color={color} discrete={discrete} zeroLine={zeroLine} unit={unit} />
      )}
    </LightweightChart>
  );
}

interface TrackSeriesProps {
  chart: IChartApi;
  data: LineData<UTCTimestamp>[];
  color: string;
  discrete: boolean;
  zeroLine: boolean;
  unit: string;
}

/** 序列建在自己的 effect 里，图表实例只建一次，换数据不会刷掉缩放与十字光标。 */
function TrackSeries({ chart, data, color, discrete, zeroLine, unit }: TrackSeriesProps) {
  useEffect(() => {
    const series = chart.addSeries(LineSeries, {
      color,
      lineWidth: 2,
      lineStyle: discrete ? LineStyle.Dashed : LineStyle.Solid,
      pointMarkersVisible: discrete,
      pointMarkersRadius: discrete ? 3 : undefined,
      lastValueVisible: true,
      priceLineVisible: false,
      priceFormat: { type: "custom", formatter: (value: number) => `${value.toFixed(1)}${unit}` },
    });
    series.setData(data);
    if (zeroLine) {
      series.createPriceLine({
        price: 0,
        color: chartPalette.border,
        lineWidth: 1,
        lineStyle: LineStyle.Dotted,
        axisLabelVisible: false,
        title: "",
      });
    }
    chart.timeScale().fitContent();
    return () => {
      chart.removeSeries(series);
    };
  }, [chart, data, color, discrete, zeroLine, unit]);

  return null;
}
