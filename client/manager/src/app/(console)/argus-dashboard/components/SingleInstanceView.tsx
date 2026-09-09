"use client";

import {
  AreaChartOutlined,
  CheckCircleFilled,
  CloudServerOutlined,
  DatabaseOutlined,
  ExclamationCircleFilled,
  FundOutlined,
  HeartFilled,
  RadarChartOutlined,
} from "@ant-design/icons";
import { Alert, Empty, Progress, Skeleton, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ReactNode } from "react";
import type { ArgusInstanceRuntime } from "@/components/argus/argus-instance.api";
import { effectStateMeta } from "@/components/argus/argus-instance.api";
import type { ArgusConfigSnapshot } from "../../argus-config/api/argus-config.api";
import type { MarketTimeline } from "../../argus-market/api/argus-market.api";
import { EMPTY, fmtInt, fmtSigned, shortTs, signColor } from "../../argus-signals/constants";
import type { GateStats, SignalEvent } from "../../argus-signals/api/argus-signals.api";
import type { EquityCurve, InstanceSummary } from "../api/argus-dashboard.api";
import { COVERAGE_ERROR_PCT, COVERAGE_WARN_PCT, fmtAge, wallClockAgeSeconds } from "../constants";
import { EquityChart } from "./EquityChart";
import { formatRatioAsBp } from "@/components/argus/units";

const { Text } = Typography;

interface SingleInstanceViewProps {
  instanceKey: string;
  runtime?: ArgusInstanceRuntime;
  summary?: InstanceSummary;
  equity: EquityCurve | null;
  gateStats: GateStats | null;
  signals: SignalEvent[];
  snapshot: ArgusConfigSnapshot | null;
  timeline: MarketTimeline | null;
  loading: boolean;
}

/** 单实例工作台：只读呈现运行与数据健康，不提供参数编辑、热加载或进程控制入口。 */
export function SingleInstanceView(props: SingleInstanceViewProps) {
  const meta = effectStateMeta(props.runtime?.effectState ?? "unknown");
  const resultTotal = props.gateStats?.byResult.reduce((sum, item) => sum + item.count, 0) ?? 0;
  const signalColumns: ColumnsType<SignalEvent> = [
    { title: "时间", dataIndex: "ts", width: 105, render: (value: string) => <span className="manager-argus-mono">{shortTs(value)}</span> },
    { title: "账户 / 变体", width: 195, render: (_, row) => <SignalAccount row={row} /> },
    { title: "判定", width: 130, render: (_, row) => <Tag color={row.resultKind === "open" ? "green" : row.resultKind === "blocked" ? "gold" : "blue"}>{row.eventLabel}</Tag> },
    { title: "偏离", align: "right", width: 98, render: (_, row) => <span className="manager-argus-mono">{row.gapBp === null ? EMPTY : fmtSigned(row.gapBp, 2, " bp")}</span> },
    { title: "净仓", align: "right", width: 82, render: (_, row) => <span className="manager-argus-mono">{fmtInt(row.size)} 张</span> },
  ];

  return (
    <>
      <section className="manager-dashboard-runtimebar">
        <div className="manager-dashboard-runtimebar__state">
          <span className={`manager-dashboard-status manager-dashboard-status--${meta.tone}`}>
            {props.runtime?.online ? <CheckCircleFilled /> : <ExclamationCircleFilled />}
            {props.runtime?.online ? "进程在线" : "心跳超时"}
          </span>
          <b>{props.runtime?.instanceName || props.instanceKey}</b>
          <span className="manager-argus-mono">{props.instanceKey}</span>
        </div>
        <div className="manager-dashboard-runtimebar__facts">
          <span>心跳 {fmtAge(props.runtime?.heartbeatAgeSeconds)}</span>
          <span>已发 v{props.runtime?.publishedVersion || "—"}</span>
          <span>运行 v{props.runtime?.runningVersion || "—"}</span>
          <Tag color={meta.tone === "ok" ? "green" : meta.tone === "warn" ? "gold" : meta.tone === "err" ? "red" : "default"}>{meta.label}</Tag>
        </div>
      </section>

      <div className="manager-argus-tiles manager-dashboard-kpis">
        <Metric icon={<RadarChartOutlined />} label="窗口内信号" value={fmtInt(props.summary?.signals)} hint={`成交 ${fmtInt(props.summary?.opened)} · 开仓率 ${((props.summary?.openRate ?? 0) * 100).toFixed(1)}%`} />
        <Metric icon={<FundOutlined />} label="当前净持仓" value={`${fmtInt(props.summary?.netSizeTotal)} 张`} hint="各账户最新余额样本之和，仅限本实例" />
        <Metric icon={<AreaChartOutlined />} label="已实现盈亏" value={fmtSigned(props.summary?.realizedPnl, 2, " U")} valueColor={signColor(props.summary?.realizedPnl)} hint="按事件窗口归集，不跨实例合计" />
        <Metric icon={<HeartFilled />} label="最新权益" value={props.summary?.equityTotal === null || props.summary?.equityTotal === undefined ? EMPTY : `${props.summary.equityTotal.toFixed(2)} U`} hint="账户最新样本合计；权益曲线按账户相对变化展示" />
      </div>

      <div className="manager-dashboard-two-col">
        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><AreaChartOutlined /></span>权益曲线</div>
            <Text type="secondary">相对各账户首个权益的变化（%）</Text>
          </div>
          {props.loading && !props.equity ? <Skeleton active paragraph={{ rows: 8 }} /> : props.equity?.series?.length ? <EquityChart series={props.equity.series} /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前窗口没有权益样本" />}
        </section>

        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><RadarChartOutlined /></span>触发点结果分布</div>
            <Text type="secondary">逐次触发判定，不按分钟折叠</Text>
          </div>
          {props.gateStats?.byResult?.length ? (
            <div className="manager-dashboard-distribution">
              {props.gateStats.byResult.map((item) => {
                const percent = resultTotal > 0 ? (item.count / resultTotal) * 100 : 0;
                return (
                  <div className="manager-dashboard-distribution__row" key={item.event}>
                    <span>{item.label}</span>
                    <Progress percent={Number(percent.toFixed(1))} showInfo={false} strokeColor={resultTone(item.event)} trailColor="#2B3139" />
                    <b className="manager-argus-mono">{item.count} · {percent.toFixed(1)}%</b>
                  </div>
                );
              })}
            </div>
          ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前窗口没有触发判定" />}
        </section>
      </div>

      <div className="manager-dashboard-two-col">
        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><DatabaseOutlined /></span>最近信号流</div>
            <Text type="secondary">只读 · 最近 12 条</Text>
          </div>
          {props.signals.length ? <Table<SignalEvent> rowKey="eventId" columns={signalColumns} dataSource={props.signals} pagination={false} size="small" scroll={{ x: 610 }} /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前窗口没有信号事件" />}
        </section>

        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><CloudServerOutlined /></span>当前生效参数摘要</div>
            <Text type="secondary">只读快照</Text>
          </div>
          <ParameterSummary snapshot={props.snapshot} runtime={props.runtime} />
        </section>
      </div>

      <section className="manager-argus-panel">
        <div className="manager-argus-panel__head">
          <div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><HeartFilled /></span>数据健康清单</div>
          <Text type="secondary">缺失或漂移必须显式展示，不用 0 填充</Text>
        </div>
        <HealthList runtime={props.runtime} summary={props.summary} timeline={props.timeline} />
      </section>
    </>
  );
}

function Metric({ icon, label, value, hint, valueColor }: { icon: ReactNode; label: string; value: string; hint: string; valueColor?: string }) {
  return <div className="manager-argus-tile"><span className="manager-argus-tile__label">{icon}{label}</span><div className="manager-argus-tile__value" style={{ color: valueColor }}>{value}</div><Text className="manager-argus-tile__hint">{hint}</Text></div>;
}

function SignalAccount({ row }: { row: SignalEvent }) {
  return <div className="manager-dashboard-instance"><b>{row.accountLabel || EMPTY}</b><span>{row.variant || EMPTY}</span></div>;
}

function ParameterSummary({ snapshot, runtime }: { snapshot: ArgusConfigSnapshot | null; runtime?: ArgusInstanceRuntime }) {
  if (!snapshot) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未读取到已发布参数快照" />;
  return (
    <div className="manager-dashboard-parameters">
      <SummaryItem label="已发布版本" value={`v${snapshot.version?.version || runtime?.publishedVersion || "—"}`} />
      <SummaryItem label="默认下单量" value={`${snapshot.config?.defaultOrderSize ?? "—"} 张`} />
      <SummaryItem label="信号阈值" value={snapshot.monitorSymbols?.map((item) => `${item.symbol || "—"} ${formatRatioAsBp(item.signalThreshold)}`).join(" · ") || EMPTY} />
      <SummaryItem label="账户上限" value={snapshot.accountRisks?.map((risk) => `${risk.maxContracts ?? "—"} 张`).join(" / ") || EMPTY} />
      <SummaryItem label="配置账户" value={snapshot.accounts?.map((account) => account.accountName || "—").join(" / ") || EMPTY} />
    </div>
  );
}

function SummaryItem({ label, value }: { label: string; value: string }) {
  return <div className="manager-dashboard-parameters__item"><span>{label}</span><b className="manager-argus-mono">{value}</b></div>;
}

function HealthList({ runtime, summary, timeline }: { runtime?: ArgusInstanceRuntime; summary?: InstanceSummary; timeline: MarketTimeline | null }) {
  const eventAge = wallClockAgeSeconds(summary?.lastTs || "");
  const coverage = timeline?.coverage?.coveragePct;
  const compareCoverage = timeline?.compareCoverage?.coveragePct;
  const health = [
    { label: "进程心跳", detail: runtime?.online ? `最后上报 ${fmtAge(runtime.heartbeatAgeSeconds)}` : "15 秒 TTL 内没有有效心跳，无法确认实例状态", tone: runtime?.online ? "ok" : "err" },
    { label: "参数生效", detail: effectStateMeta(runtime?.effectState ?? "unknown").label, tone: effectStateMeta(runtime?.effectState ?? "unknown").tone },
    { label: "事件新鲜度", detail: summary?.lastTs ? `最后事件 ${summary.lastTs}（${fmtAge(eventAge)}）` : "窗口内没有事件", tone: eventAge !== null && eventAge < 24 * 3600 ? "ok" : "warn" },
    { label: "DeepCoin 1m K 线", detail: coverage === undefined ? "尚未读取覆盖率" : `${coverage.toFixed(2)}% · 缺 ${timeline?.coverage?.missing ?? 0} 根`, tone: coverageTone(coverage) },
    { label: "Binance 对比 K 线", detail: compareCoverage === undefined ? "尚未读取覆盖率" : `${compareCoverage.toFixed(2)}% · 缺 ${timeline?.compareCoverage?.missing ?? 0} 根`, tone: coverageTone(compareCoverage) },
  ];
  return <div className="manager-dashboard-health">{health.map((item) => <div className="manager-dashboard-health__item" key={item.label}><span className={`manager-dashboard-status manager-dashboard-status--${item.tone}`}><i />{item.label}</span><b>{item.detail}</b></div>)}</div>;
}

function coverageTone(coverage?: number): "ok" | "warn" | "err" | "mute" {
  if (coverage === undefined) return "mute";
  if (coverage < COVERAGE_ERROR_PCT) return "err";
  if (coverage < COVERAGE_WARN_PCT) return "warn";
  return "ok";
}

function resultTone(event: string) {
  if (event === "open") return "#0ECB81";
  if (event === "cap_skip") return "#FF9F43";
  if (event === "gate_block") return "#A78BFA";
  return "#4D7EFF";
}
