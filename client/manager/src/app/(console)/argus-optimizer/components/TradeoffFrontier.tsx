"use client";

import { Empty } from "antd";
import { useMemo } from "react";
import type { OptimizeCell } from "../api/argus-optimizer.api";

interface TradeoffFrontierProps {
  cells: OptimizeCell[];
}

interface Point extends OptimizeCell {
  x: number;
  y: number;
}

function domain(values: number[]): [number, number] {
  const low = Math.min(...values);
  const high = Math.max(...values);
  if (low === high) return [low - 1, high + 1];
  const padding = (high - low) * 0.12;
  return [low - padding, high + padding];
}

/**
 * 前沿是“收益 / 风险的可见取舍”，不是最优参数排行榜：每一个点都保留三关判定，
 * 用坐标轴揭示中位收益与 p90 回撤之间的张力。
 */
export function TradeoffFrontier({ cells }: TradeoffFrontierProps) {
  const points = useMemo<Point[]>(
    () => cells.filter((item) => item.status === "done").map((item) => ({ ...item, x: item.p90MaxDrawdown, y: item.medPnl28 })),
    [cells],
  );
  const xDomain = useMemo(() => (points.length ? domain(points.map((item) => item.x)) : [0, 1]), [points]);
  const yDomain = useMemo(() => (points.length ? domain(points.map((item) => item.y)) : [0, 1]), [points]);

  if (!points.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="完成精算后显示收益 / 回撤权衡前沿" />;

  const width = 640;
  const height = 310;
  const left = 58;
  const right = 24;
  const top = 20;
  const bottom = 44;
  const scaleX = (value: number) => left + ((value - xDomain[0]) / (xDomain[1] - xDomain[0])) * (width - left - right);
  const scaleY = (value: number) => height - bottom - ((value - yDomain[0]) / (yDomain[1] - yDomain[0])) * (height - top - bottom);
  const frontier = points.filter((item) => item.onDdFrontier || item.onBearFrontier).sort((a, b) => a.x - b.x);

  return (
    <div className="manager-argus-frontier">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="精算格的中位收益与 p90 最大回撤权衡散点图">
        {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
          const y = top + ratio * (height - top - bottom);
          const value = yDomain[1] - ratio * (yDomain[1] - yDomain[0]);
          return <g key={`y-${ratio}`}><line x1={left} x2={width - right} y1={y} y2={y} /><text x={left - 8} y={y + 4} textAnchor="end">{value.toFixed(0)}</text></g>;
        })}
        {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
          const x = left + ratio * (width - left - right);
          const value = xDomain[0] + ratio * (xDomain[1] - xDomain[0]);
          return <g key={`x-${ratio}`}><line x1={x} x2={x} y1={top} y2={height - bottom} /><text x={x} y={height - bottom + 18} textAnchor="middle">{value.toFixed(0)}</text></g>;
        })}
        {frontier.length > 1 ? <polyline points={frontier.map((item) => `${scaleX(item.x)},${scaleY(item.y)}`).join(" ")} className="manager-argus-frontier__line" /> : null}
        {points.map((item) => {
          const color = item.isIncumbent ? "#F0B90B" : item.passed ? "#0ECB81" : item.onDdFrontier || item.onBearFrontier ? "#4D7EFF" : "#848E9C";
          return (
            <g key={item.id} className="manager-argus-frontier__point">
              <title>{`${item.key}：中位 ${item.y.toFixed(1)}U，p90 回撤 ${item.x.toFixed(1)}U，三关 ${item.passCount}/3`}</title>
              <circle cx={scaleX(item.x)} cy={scaleY(item.y)} r={item.isIncumbent ? 7 : 5} fill={color} />
              {item.isIncumbent ? <circle cx={scaleX(item.x)} cy={scaleY(item.y)} r={10} fill="none" stroke={color} /> : null}
            </g>
          );
        })}
        <text x={(left + width - right) / 2} y={height - 7} textAnchor="middle">p90 最大回撤（U，越左越稳）</text>
        <text x={15} y={(top + height - bottom) / 2} transform={`rotate(-90 15 ${(top + height - bottom) / 2})`} textAnchor="middle">中位 28 日 PnL（U，越上越高）</text>
      </svg>
      <div className="manager-argus-frontier__legend"><span><i className="is-incumbent" />现行配置</span><span><i className="is-pass" />三关全过</span><span><i className="is-frontier" />权衡前沿</span><span><i className="is-other" />其他精算格</span></div>
    </div>
  );
}
