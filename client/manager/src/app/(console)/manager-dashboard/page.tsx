"use client";

import {
  AlertOutlined,
  ApiOutlined,
  CloudServerOutlined,
  FundOutlined,
  LineChartOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import { Alert, Button, Empty, Skeleton, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ReactNode } from "react";

import { flattenAccounts, sumOf, useManagerDashboard, type AccountRow } from "./hooks/useManagerDashboard";

const { Text, Title } = Typography;

/** 取不到就显示这个。不用 0 顶——0 是"真的没有"，和"取不到"是两件事。 */
const EMPTY = "—";

function fmtNum(v: number | null | undefined, digits = 0): string {
  if (v === null || v === undefined || Number.isNaN(v)) return EMPTY;
  return v.toLocaleString("zh-CN", { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

function fmtSigned(v: number | null | undefined, digits = 2): string {
  if (v === null || v === undefined || Number.isNaN(v)) return EMPTY;
  return `${v > 0 ? "+" : ""}${v.toFixed(digits)}`;
}

function fmtPct(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined || Number.isNaN(v)) return EMPTY;
  return `${(v * 100).toFixed(digits)}%`;
}

interface MetricProps {
  label: string;
  value: string;
  unit?: string;
  hint: ReactNode;
  icon: ReactNode;
  tone?: "pos" | "neg";
}

function Metric({ label, value, unit, hint, icon, tone }: MetricProps) {
  return (
    <div className="manager-dashboard-metric">
      <div className="manager-dashboard-metric__topline">
        <Text className="manager-section-label">{label}</Text>
        <span className="manager-dashboard-metric__icon">{icon}</span>
      </div>
      <div className="manager-dashboard-metric__value" data-tone={tone}>
        {value}
        {unit ? <small>{unit}</small> : null}
      </div>
      <Text className="manager-dashboard-metric__hint">{hint}</Text>
    </div>
  );
}

const accountColumns: ColumnsType<AccountRow> = [
  { title: "账户", dataIndex: "accountLabel", key: "accountLabel", width: 220 },
  {
    title: "变体",
    dataIndex: "variant",
    key: "variant",
    width: 220,
    render: (v: string) => (v ? <Tag color="gold">{v}</Tag> : EMPTY),
  },
  {
    title: "最新权益",
    dataIndex: "lastEquity",
    key: "lastEquity",
    align: "right",
    width: 120,
    render: (v: number | null) => (v === null ? EMPTY : v.toFixed(2)),
  },
  {
    title: "余额",
    dataIndex: "lastBalance",
    key: "lastBalance",
    align: "right",
    width: 120,
    render: (v: number | null) => (v === null ? EMPTY : v.toFixed(2)),
  },
  {
    title: "净持仓",
    dataIndex: "lastNetSize",
    key: "lastNetSize",
    align: "right",
    width: 100,
    // net_size 有独立的 known 标记列，服务端取不到时给 null——这里必须显示"—"
    render: (v: number | null) => (v === null ? EMPTY : `${v} 张`),
  },
  { title: "采样时刻", dataIndex: "lastSampleTs", key: "lastSampleTs", width: 170 },
  { title: "所属实例", dataIndex: "instanceKey", key: "instanceKey", width: 180 },
];

export default function ManagerDashboardPage() {
  const dash = useManagerDashboard();
  const instances = dash.summary?.instances ?? [];
  const runtimes = dash.overview?.instances ?? [];
  const accounts = flattenAccounts(dash.summary);

  // 可加的才加：权益/净持仓/信号/成交是可加的。
  const equityTotal = sumOf(instances, (i) => i.equityTotal);
  const netSizeTotal = sumOf(instances, (i) => i.netSizeTotal);
  const realizedPnl = sumOf(instances, (i) => i.realizedPnl);
  const signals = sumOf(instances, (i) => i.signals) ?? 0;
  const opened = sumOf(instances, (i) => i.opened) ?? 0;
  const capSkip = sumOf(instances, (i) => i.capSkip) ?? 0;
  const gateBlock = sumOf(instances, (i) => i.gateBlock) ?? 0;
  // 开仓率必须用 Σ成交/Σ信号 重算，不能把各实例的比率再平均一次。
  const openRate = signals > 0 ? opened / signals : null;

  const online = runtimes.filter((r) => r.online).length;
  const registered = runtimes.length;

  const rebuild = dash.episodes?.rebuild;
  const winRate = dash.episodes?.winRate ?? null;

  return (
    <div className="manager-page-stack manager-dashboard">
      <section className="manager-dashboard-hero">
        <div>
          <Text className="manager-section-label">实盘总览 · 近 {dash.windowHours}H</Text>
          <Title level={1} className="manager-dashboard-hero__title">
            {dash.loading && !dash.summary ? (
              <Skeleton active paragraph={false} title={{ width: 420 }} />
            ) : (
              <>
                权益合计 {fmtNum(equityTotal, 2)} USDT，近 {dash.windowHours} 小时已实现{" "}
                {fmtSigned(realizedPnl)} USDT，{online} / {registered} 个实例在跑。
              </>
            )}
          </Title>
          <Text className="manager-dashboard-hero__subtitle">
            全部数字来自 argus_instance 心跳、strategy_event 事件表与 balance_sample
            采样，跨实例只做并排与可加汇总。取不到的指标一律显示「—」，不用 0 顶。
          </Text>
        </div>
        <div className="manager-dashboard-hero__actions">
          <Button icon={<ReloadOutlined />} loading={dash.loading} onClick={() => void dash.refresh()}>
            刷新
          </Button>
        </div>
      </section>

      {dash.error ? <Alert type="warning" showIcon message="部分数据读取失败" description={dash.error} /> : null}

      {rebuild?.stale ? (
        <Alert
          type="warning"
          showIcon
          message="持仓派生结果已滞后"
          description={rebuild.notice}
        />
      ) : null}

      <section className="manager-dashboard-total-strip">
        <div className="manager-dashboard-total-strip__item">
          <span className="manager-dashboard-total-strip__icon">
            <WalletOutlined />
          </span>
          <div>
            <Text className="manager-dashboard-total-strip__label">权益合计</Text>
            <div>{equityTotal === null ? EMPTY : `${fmtNum(equityTotal, 2)} USDT`}</div>
          </div>
        </div>
        <div className="manager-dashboard-total-strip__item">
          <span className="manager-dashboard-total-strip__icon">
            <ThunderboltOutlined />
          </span>
          <div>
            <Text className="manager-dashboard-total-strip__label">近 {dash.windowHours}H 成交</Text>
            <div>{fmtNum(opened)} 笔</div>
          </div>
        </div>
        <div className="manager-dashboard-total-strip__item">
          <span className="manager-dashboard-total-strip__icon">
            <CloudServerOutlined />
          </span>
          <div>
            <Text className="manager-dashboard-total-strip__label">在跑实例</Text>
            <div>
              {online} / {registered}
            </div>
          </div>
        </div>
      </section>

      <section className="manager-dashboard-metric-grid">
        <Metric
          label={`近 ${dash.windowHours}H 已实现盈亏`}
          value={fmtSigned(realizedPnl)}
          unit=" USDT"
          tone={realizedPnl !== null && realizedPnl < 0 ? "neg" : "pos"}
          hint={`只统计真正实现的：减仓锁利与各类平仓。浮亏告警不计入。`}
          icon={<LineChartOutlined />}
        />
        <Metric
          label="当前净持仓"
          value={fmtNum(netSizeTotal)}
          unit=" 张"
          hint="各账户最新余额样本之和"
          icon={<FundOutlined />}
        />
        <Metric
          label={`近 ${dash.windowHours}H 开仓率`}
          value={fmtPct(openRate)}
          hint={`${fmtNum(opened)} 成交 / ${fmtNum(signals)} 次触发，按 Σ成交÷Σ触发 重算`}
          icon={<ApiOutlined />}
        />
        <Metric
          label="策略胜率"
          value={winRate === null ? EMPTY : `${winRate.toFixed(1)}%`}
          hint={
            rebuild?.stale ? (
              <Tooltip title={rebuild.notice}>
                <span>
                  <Tag color="orange">派生滞后</Tag>
                  需跑 argus-episode-rebuild
                </span>
              </Tooltip>
            ) : (
              `已出场 ${dash.episodes?.closed ?? EMPTY} 笔，交易所侧/人工平仓不计入`
            )
          }
          icon={<SafetyCertificateOutlined />}
        />
      </section>

      <div className="manager-dashboard-main-grid">
        <section className="manager-dashboard-panel manager-dashboard-panel--wide">
          <header className="manager-dashboard-panel__header">
            <span className="manager-dashboard-panel__icon">
              <WalletOutlined />
            </span>
            <div>
              <Text className="manager-section-label">账户明细</Text>
              <div>来自 balance_sample 的最新一条采样，逐账户列出，不做跨账户折叠</div>
            </div>
          </header>
          <Table<AccountRow>
            className="manager-table"
            rowKey={(r) => `${r.instanceKey}|${r.accountLabel}`}
            columns={accountColumns}
            dataSource={accounts}
            loading={dash.loading && !accounts.length}
            pagination={false}
            size="small"
            scroll={{ x: 1130 }}
            locale={{ emptyText: <Empty description="窗口内没有账户采样" /> }}
          />
        </section>

        <section className="manager-dashboard-panel">
          <header className="manager-dashboard-panel__header">
            <span className="manager-dashboard-panel__icon">
              <AlertOutlined />
            </span>
            <div>
              <Text className="manager-section-label">触发结果分布</Text>
              <div>逐次触发判定，不按分钟折叠</div>
            </div>
          </header>
          <div className="manager-dashboard-category-list">
            {[
              { label: "成交开仓", n: opened, color: "#26d07c" },
              { label: "上限跳过", n: capSkip, color: "#f5a623" },
              { label: "门控拦截", n: gateBlock, color: "#4f8cff" },
            ].map((row) => (
              <div className="manager-dashboard-task" key={row.label}>
                <span style={{ color: row.color }}>●</span>
                <span>{row.label}</span>
                <b>{fmtNum(row.n)}</b>
                <span>{signals > 0 ? fmtPct(row.n / signals) : EMPTY}</span>
              </div>
            ))}
          </div>
          {dash.summary?.notice ? (
            <Text className="manager-dashboard-metric__hint">{dash.summary.notice}</Text>
          ) : null}
        </section>
      </div>
    </div>
  );
}
