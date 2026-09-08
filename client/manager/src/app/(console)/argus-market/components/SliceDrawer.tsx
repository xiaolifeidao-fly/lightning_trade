"use client";

import { Alert, Descriptions, Drawer, Empty, Skeleton, Spin, Tag, Typography, type DescriptionsProps } from "antd";
import {
  LineSeries,
  LineStyle,
  createSeriesMarkers,
  type IChartApi,
  type UTCTimestamp,
} from "lightweight-charts";
import { useEffect } from "react";
import { LightweightChart } from "@/components/charts/LightweightChart";
import { chartPalette, wallClockToChartTime } from "@/components/charts/chartTheme";
import type { MarketKline, SignalEvent, SignalSlice } from "../api/argus-market.api";
import { PLATFORM_LABELS, TRIGGER_KIND_MAP } from "../constants";

const { Text } = Typography;

interface SliceDrawerProps {
  open: boolean;
  trigger: SignalEvent | null;
  slice: SignalSlice | null;
  loading: boolean;
  error: string;
  thresholdBp: number | null;
  onClose: () => void;
}

const fmtPx = (value: number | null | undefined) =>
  value == null ? "—" : value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/**
 * 触发瞬间的秒级切片抽屉。
 *
 * 三条序列：DC last（信号源）、DC mark（信号基准）、币安（参照）。信号源是
 * **同一交易所内 last 相对 mark 的偏离**——币安价格只用来判断"这是全市场的动，
 * 还是只有 DeepCoin 自己在动"，它不参与任何判定。
 */
export function SliceDrawer({ open, trigger, slice, loading, error, thresholdBp, onClose }: SliceDrawerProps) {
  const meta = trigger ? TRIGGER_KIND_MAP[trigger.event] : undefined;
  const gap = trigger?.gapBp ?? null;
  const overshoot = gap != null && thresholdBp != null ? Math.abs(gap) - thresholdBp : null;

  return (
    <Drawer
      open={open}
      onClose={onClose}
      width={860}
      title={
        <div className="manager-market-drawer__title">
          <span>触发瞬间秒级切片</span>
          {trigger ? <Tag className="manager-argus-mono">#{trigger.eventId}</Tag> : null}
          {meta ? (
            <span className="manager-market-kind manager-market-kind--static">
              <i className="manager-market-kind__dot" style={{ background: meta.color }} />
              {meta.label}
            </span>
          ) : null}
          {trigger?.direction ? (
            <Tag color={trigger.direction === "UP" ? "green" : "red"}>
              {trigger.direction} → {trigger.side || "—"}
            </Tag>
          ) : null}
          {trigger?.configVersion ? <Tag>参数 v{trigger.configVersion}</Tag> : null}
        </div>
      }
    >
      {error ? <Alert type="error" showIcon message="读取切片失败" description={error} /> : null}

      {loading && !slice ? (
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : !slice ? (
        !error ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有可展示的切片" /> : null
      ) : (
        <Spin spinning={loading}>
          <div className="manager-market-drawer__body">
            {!slice.tickComplete ? (
              <Alert
                type="warning"
                showIcon
                message="这段切片不是逐秒完整的"
                description={
                  <span>
                    当前来源是 <b className="manager-argus-mono">{slice.tickSource}</b>
                    ：{slice.degradedReason || "只有触发那一秒有真实观测，中间没有采样点。"}
                    图上的点是真实观测，点与点之间的连线只是视觉连接，不代表那段时间被测过。
                  </span>
                }
              />
            ) : slice.degradedReason ? (
              // 逐秒完整但仍有供数限制（例如窗口被收窄），这条也要说，不能因为
              // tickComplete=true 就把服务端的自曝吞掉。
              <Alert type="info" showIcon message="供数说明" description={slice.degradedReason} />
            ) : null}

            <section>
              <div className="manager-market-drawer__head">
                <b>触发前后 ±{slice.windowSeconds} 秒 · 逐秒价格</b>
                <div className="manager-market-legend">
                  <span>
                    <i style={{ background: chartPalette.primary }} />
                    DC last（信号源）
                  </span>
                  <span>
                    <i style={{ background: chartPalette.accent }} />
                    DC mark（信号基准）
                  </span>
                  <span>
                    <i style={{ background: chartPalette.neutral }} />
                    {PLATFORM_LABELS.binance}
                    {slice.binPoints > 0 ? " last（参照）" : "（1m 收盘参照）"}
                  </span>
                </div>
              </div>
              {slice.points.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="窗口内没有任何秒级观测" />
              ) : (
                <LightweightChart
                  height={240}
                  options={{ timeScale: { timeVisible: true, secondsVisible: true } }}
                >
                  {(chart) => <SliceSeries chart={chart} slice={slice} dashed={!slice.tickComplete} />}
                </LightweightChart>
              )}
              <Text type="secondary" className="manager-market-note">
                信号源是 <b>DeepCoin last 相对 mark 的偏离</b>（同一交易所内），偏离 ={" "}
                <span className="manager-argus-mono">(last − mark) / mark × 10000</span> bp。
                {slice.binPoints > 0 ? (
                  <>
                    {PLATFORM_LABELS.binance}那条是切片里的逐秒 last（{slice.binPoints} 个点），
                    只作对照、不参与任何判定——它用来分辨「全市场在动」还是「只有 DeepCoin 在动」。
                  </>
                ) : (
                  <>
                    这段切片里没有币安逐秒报价，图上那条灰线退回
                    <span className="manager-argus-mono"> {slice.klineSource || "—"} </span>
                    的 1m 收盘，只能看大致走向，不能按秒对齐。
                  </>
                )}
              </Text>
            </section>

            <div className="manager-market-drawer__grid">
              <Descriptions title="信号快照" column={1} size="small" bordered items={snapshotItems(trigger, slice, gap, overshoot)} />
              <Descriptions title="判定结果" column={1} size="small" bordered items={verdictItems(trigger, slice)} />
            </div>
          </div>
        </Spin>
      )}
    </Drawer>
  );
}

/** 信号快照：这一秒被观测到的原始量，全部来自 strategy_event 的报价快照列。 */
function snapshotItems(
  trigger: SignalEvent | null,
  slice: SignalSlice,
  gap: number | null,
  overshoot: number | null,
): DescriptionsProps["items"] {
  const devMax = slice.devSamples.length === 0 ? null : Math.max(...slice.devSamples.map((item) => item.devMaxBp ?? 0));
  return [
    { key: "ts", label: "事件时刻", children: <span className="manager-argus-mono">{trigger?.ts || slice.ts}</span> },
    ...(slice.sliceAnchorTs
      ? [
          {
            key: "anchor",
            label: "偏离穿越时刻",
            children: (
              <span className="manager-argus-mono">
                {slice.sliceAnchorTs}
                {slice.anchorLagSec ? `（事件晚 ${slice.anchorLagSec}s）` : ""}
              </span>
            ),
          },
        ]
      : []),
    { key: "sigLast", label: "sigLast", children: <span className="manager-argus-mono">{fmtPx(trigger?.sigLast)}</span> },
    { key: "sigMark", label: "sigMark", children: <span className="manager-argus-mono">{fmtPx(trigger?.sigMark)}</span> },
    {
      key: "gap",
      label: "gapBp",
      children: (
        <span className={`manager-argus-mono ${(gap ?? 0) >= 0 ? "manager-market-up" : "manager-market-down"}`}>
          {gap == null ? "—" : `${gap > 0 ? "+" : ""}${gap.toFixed(2)} bp`}
        </span>
      ),
    },
    {
      key: "overshoot",
      label: "越阈幅度",
      children: (
        <span className="manager-argus-mono">{overshoot == null ? "—（该实例阈值未知）" : `${overshoot.toFixed(2)} bp`}</span>
      ),
    },
    {
      key: "points",
      label: "秒级点位",
      children: (
        <span className="manager-argus-mono">
          DC {slice.dcPoints} · {PLATFORM_LABELS.binance} {slice.binPoints}
        </span>
      ),
    },
    {
      key: "dev",
      label: "无条件偏离采样",
      // dev_sample 不受生产阈值截断，是判断「这次触发在当时的偏离里算不算极端」的唯一参照。
      children: devMax == null ? "窗口内无 dev_sample" : `${slice.devSamples.length} 个窗口 · 最大 ${devMax.toFixed(2)} bp`,
    },
  ];
}

/** 判定结果：成交与被拦截走两套字段，不给缺失值补 0（补 0 会显示成一个假价格）。 */
function verdictItems(trigger: SignalEvent | null, slice: SignalSlice): DescriptionsProps["items"] {
  const meta = trigger ? TRIGGER_KIND_MAP[trigger.event] : undefined;
  const head = [{ key: "result", label: "结果", children: meta?.label || trigger?.eventLabel || "—" }];
  const body =
    trigger?.event === "open"
      ? [
          {
            key: "orderSize",
            label: "下单张数",
            children: <span className="manager-argus-mono">{trigger.orderSize ?? trigger.size ?? "—"}</span>,
          },
          { key: "avgPx", label: "成交均价", children: <span className="manager-argus-mono">{fmtPx(trigger.avgPx)}</span> },
          {
            key: "net",
            label: "开仓后净仓",
            children: (
              <span className="manager-argus-mono">
                {trigger.size == null ? "—" : `${trigger.size} 张`}
                {trigger.netSide ? ` · ${trigger.netSide}` : ""}
              </span>
            ),
          },
        ]
      : [
          { key: "gateKind", label: "门控类型", children: trigger?.gateLabel || trigger?.gateKind || "—" },
          {
            key: "gateValue",
            label: "实测 vs 阈值",
            children: (
              <span className="manager-argus-mono">
                {trigger?.gateActual == null ? "—" : trigger.gateActual}
                {" / "}
                {trigger?.gateThreshold == null ? "—" : trigger.gateThreshold}
              </span>
            ),
          },
          {
            key: "reason",
            label: "原始文本",
            children: <Text type="secondary">{trigger?.reason || "—"}</Text>,
          },
        ];
  return [
    ...head,
    ...body,
    {
      key: "source",
      label: "数据来源",
      children: (
        <>
          {trigger?.sourceLabel || "—"} · 实例 <span className="manager-argus-mono">{slice.instanceKey}</span>
        </>
      ),
    },
  ];
}

/** 切片图的三条序列 + 触发时刻标记；只跑副作用，不渲染 DOM。 */
function SliceSeries({ chart, slice, dashed }: { chart: IChartApi; slice: SignalSlice; dashed: boolean }) {
  useEffect(() => {
    const style = dashed ? LineStyle.Dotted : LineStyle.Solid;
    const dcLast = chart.addSeries(LineSeries, {
      color: chartPalette.primary,
      lineWidth: 2,
      lineStyle: style,
      pointMarkersVisible: true,
      priceLineVisible: false,
      lastValueVisible: false,
      title: "DC last",
    });
    dcLast.setData(pickSeries(slice, (point) => point.dcLast));

    const dcMark = chart.addSeries(LineSeries, {
      color: chartPalette.accent,
      lineWidth: 1,
      lineStyle: style,
      pointMarkersVisible: true,
      priceLineVisible: false,
      lastValueVisible: false,
      title: "DC mark",
    });
    dcMark.setData(pickSeries(slice, (point) => point.dcMark));

    // 币安：有逐秒 last 就画逐秒（r3 的 signal_slice），没有才退回 1m 收盘参照。
    // 两者精度差一个数量级，用同一条线画但样式不同，免得把 1m 折线当成秒级观测。
    const perSecondBinance = slice.binPoints > 0;
    const reference = chart.addSeries(LineSeries, {
      color: chartPalette.neutral,
      lineWidth: 1,
      lineStyle: perSecondBinance ? style : LineStyle.Dashed,
      pointMarkersVisible: perSecondBinance,
      priceLineVisible: false,
      lastValueVisible: false,
      title: PLATFORM_LABELS.binance,
    });
    reference.setData(perSecondBinance ? pickSeries(slice, (point) => point.binLast) : toKlineLine(slice.klines));

    // T0 标在**切片锚点**（偏离穿越那一秒）上，不是事件 ts：走 signal_slice 时窗口
    // 本来就以穿越瞬间为原点，事件 ts 会晚 anchorLagSec 秒（信号延迟调度 + 下单往返）。
    const anchor = wallClockToChartTime(slice.sliceAnchorTs || slice.ts);
    const markers = anchor == null
      ? []
      : [
          {
            time: anchor,
            position: "aboveBar" as const,
            shape: "arrowDown" as const,
            color: chartPalette.primary,
            text: "T0",
          },
        ];
    const plugin = createSeriesMarkers(dcLast, markers);
    chart.timeScale().fitContent();

    return () => {
      plugin.detach();
      chart.removeSeries(dcLast);
      chart.removeSeries(dcMark);
      chart.removeSeries(reference);
    };
  }, [chart, slice, dashed]);

  return null;
}

function pickSeries(slice: SignalSlice, pick: (point: SignalSlice["points"][number]) => number | null) {
  const byTime = new Map<number, { time: UTCTimestamp; value: number }>();
  for (const point of slice.points) {
    const value = pick(point);
    if (value == null) continue;
    const time = wallClockToChartTime(point.ts);
    if (time == null) continue;
    byTime.set(time, { time, value });
  }
  return Array.from(byTime.values()).sort((a, b) => a.time - b.time);
}

function toKlineLine(klines: MarketKline[]) {
  const byTime = new Map<number, { time: UTCTimestamp; value: number }>();
  for (const item of klines) {
    const time = wallClockToChartTime(item.time);
    if (time == null) continue;
    byTime.set(time, { time, value: item.close });
  }
  return Array.from(byTime.values()).sort((a, b) => a.time - b.time);
}
