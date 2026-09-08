"use client";

import { CheckCircleFilled, ExclamationCircleFilled, RightOutlined } from "@ant-design/icons";
import { Alert, Button, Empty, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ArgusInstanceOverview, ArgusInstanceRuntime } from "@/components/argus/argus-instance.api";
import { effectStateMeta } from "@/components/argus/argus-instance.api";
import type { InstanceSummary, InstanceSummaryResult } from "../api/argus-dashboard.api";
import { fmtAge } from "../constants";
import { EMPTY, fmtInt, fmtSigned, signColor } from "../../argus-signals/constants";

const { Text } = Typography;

interface AllInstancesViewProps {
  overview: ArgusInstanceOverview | null;
  summary: InstanceSummaryResult | null;
  loading: boolean;
  onSelectInstance: (instanceKey: string) => void;
}

interface InstanceTableRow {
  key: string;
  runtime?: ArgusInstanceRuntime;
  summary?: InstanceSummary;
  instanceKey: string;
  instanceName: string;
}

/**
 * 「全部实例」只提供并排观察，不创造任何合计行：阈值、仓位上限与下单张数不同，
 * 把信号或盈亏相加会把 champion / challenger 的实验结论读错。
 */
export function AllInstancesView({ overview, summary, loading, onSelectInstance }: AllInstancesViewProps) {
  const rows = buildRows(overview, summary);
  const columns: ColumnsType<InstanceTableRow> = [
    {
      title: "实例",
      dataIndex: "instanceName",
      width: 210,
      render: (_, row) => (
        <div className="manager-dashboard-instance">
          <b>{row.instanceName || row.instanceKey}</b>
          <span className="manager-argus-mono">{row.instanceKey}</span>
          {row.summary?.registered === false ? <Tag color="red">未注册</Tag> : null}
        </div>
      ),
    },
    {
      title: "心跳 / 运行状态",
      width: 152,
      render: (_, row) => <HeartbeatCell runtime={row.runtime} />,
    },
    {
      title: "参数版本",
      width: 148,
      render: (_, row) => <VersionCell runtime={row.runtime} />,
    },
    {
      title: "策略变体",
      width: 205,
      render: (_, row) => <Text className="manager-dashboard-cellwrap">{row.summary?.variants?.join(" / ") || EMPTY}</Text>,
    },
    {
      title: "今日信号",
      align: "right",
      width: 108,
      render: (_, row) => <span className="manager-argus-mono">{fmtInt(row.summary?.signals)}</span>,
    },
    {
      title: "净持仓",
      align: "right",
      width: 108,
      render: (_, row) => <span className="manager-argus-mono">{fmtInt(row.summary?.netSizeTotal,)} 张</span>,
    },
    {
      title: "今日盈亏",
      align: "right",
      width: 126,
      render: (_, row) => (
        <span className="manager-argus-mono" style={{ color: signColor(row.summary?.realizedPnl) }}>
          {fmtSigned(row.summary?.realizedPnl, 2, " U")}
        </span>
      ),
    },
    {
      title: "",
      width: 106,
      render: (_, row) => (
        <Button type="link" size="small" onClick={() => onSelectInstance(row.instanceKey)}>
          查看 <RightOutlined />
        </Button>
      ),
    },
  ];

  return (
    <>
      <Alert
        className="manager-argus-alert"
        type="warning"
        showIcon
        message="全部实例仅用于并排巡检，禁止合计"
        description={
          summary?.notice ||
          "三实例的信号阈值为 5bp / 3bp / 3bp，仓位上限为 15 / 26+8 / 246，下单量为 1 / 1 / 10 张。它们是独立实验体，信号、净持仓、胜率与盈亏均不可跨实例相加。"
        }
      />

      {overview?.duplicateInstanceKeys?.length ? (
        <Alert
          className="manager-argus-alert"
          type="error"
          showIcon
          message="检测到重复实例键"
          description={`重复键：${overview.duplicateInstanceKeys.join("、")}。配置发布与心跳可能串到错误实例，必须先修复注册数据。`}
        />
      ) : null}

      {overview?.driftInstanceKeys?.length ? (
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          message="有实例的已发布版本尚未由心跳确认"
          description={`实例：${overview.driftInstanceKeys.join("、")}。版本号只在本实例内比对，不能用其他实例的 vN 作基准。`}
        />
      ) : null}

      <section className="manager-argus-panel manager-dashboard-tablepanel">
        <div className="manager-argus-panel__head">
          <div className="manager-argus-panel__title">三实例状态表</div>
          <Text type="secondary">窗口内数据逐实例统计 · 不显示合计</Text>
        </div>
        {rows.length === 0 && !loading ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂未发现已注册实例或事件数据" />
        ) : (
          <Table<InstanceTableRow>
            rowKey="key"
            columns={columns}
            dataSource={rows}
            loading={loading}
            pagination={false}
            scroll={{ x: 1150 }}
            size="middle"
          />
        )}
      </section>
    </>
  );
}

function buildRows(overview: ArgusInstanceOverview | null, summary: InstanceSummaryResult | null): InstanceTableRow[] {
  const summaryByKey = new Map((summary?.instances ?? []).map((item) => [item.instanceKey, item]));
  const runtimeByKey = new Map((overview?.instances ?? []).map((item) => [item.instanceKey, item]));
  const keys = new Set([...Array.from(runtimeByKey.keys()), ...Array.from(summaryByKey.keys())]);
  return Array.from(keys)
    .sort()
    .map((instanceKey) => {
      const runtime = runtimeByKey.get(instanceKey);
      const item = summaryByKey.get(instanceKey);
      return {
        key: instanceKey,
        instanceKey,
        runtime,
        summary: item,
        instanceName: runtime?.instanceName || item?.instanceName || instanceKey,
      };
    });
}

function HeartbeatCell({ runtime }: { runtime?: ArgusInstanceRuntime }) {
  if (!runtime) return <Tag>未注册</Tag>;
  if (!runtime.online) return <Tag color="red">心跳超时</Tag>;
  return (
    <Tooltip title={runtime.heartbeatAt || "心跳时间未知"}>
      <span className="manager-dashboard-status manager-dashboard-status--ok">
        <CheckCircleFilled /> {fmtAge(runtime.heartbeatAgeSeconds)}
      </span>
    </Tooltip>
  );
}

function VersionCell({ runtime }: { runtime?: ArgusInstanceRuntime }) {
  if (!runtime) return <span className="manager-argus-mono">—</span>;
  const meta = effectStateMeta(runtime.effectState);
  const tone = meta.tone === "ok" ? "green" : meta.tone === "warn" ? "gold" : meta.tone === "err" ? "red" : "default";
  return (
    <div className="manager-dashboard-version">
      <span className="manager-argus-mono">已发 v{runtime.publishedVersion || "—"}</span>
      <span className="manager-argus-mono">运行 v{runtime.runningVersion || "—"}</span>
      <Tag color={tone} icon={meta.tone === "err" ? <ExclamationCircleFilled /> : undefined}>
        {meta.label}
      </Tag>
    </div>
  );
}
