"use client";

import { Button, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { SignalEvent } from "../api/argus-signals.api";
import {
  EMPTY,
  EVENT_COLORS,
  RESULT_KIND_COLORS,
  SIGNAL_PAGE_SIZE,
  fmtInt,
  fmtPrice,
  fmtSigned,
  shortTs,
  signColor,
} from "../constants";

const { Text } = Typography;

interface SignalTableProps {
  rows: SignalEvent[];
  total: number;
  page: number;
  loading: boolean;
  /** 全部实例视图：每行都要标出实例归属，账户名会跨实例重号。 */
  crossInstance: boolean;
  onPageChange: (page: number) => void;
  onOpenDetail: (row: SignalEvent) => void;
}

/**
 * 信号列表。
 *
 * **一行 = 一个账户在一次触发上的判定结果**，刻意不按分钟折叠：实测 28.7% 的
 * 信号在同一分钟内触发 ≥2 次、承载 48.6% 的全部触发，折叠会直接丢掉约一半。
 */
export function SignalTable({
  rows,
  total,
  page,
  loading,
  crossInstance,
  onPageChange,
  onOpenDetail,
}: SignalTableProps) {
  const columns: ColumnsType<SignalEvent> = [
    {
      title: "时间",
      dataIndex: "ts",
      width: 150,
      render: (ts: string) => <span className="manager-argus-mono">{shortTs(ts)}</span>,
    },
    ...(crossInstance
      ? ([
          {
            title: "实例",
            dataIndex: "instanceKey",
            width: 150,
            render: (key: string) => (
              <Tag bordered={false} className="manager-argus-mono" style={{ fontSize: 11 }}>
                {key || EMPTY}
              </Tag>
            ),
          },
        ] as ColumnsType<SignalEvent>)
      : []),
    {
      title: "账户",
      dataIndex: "accountLabel",
      width: 150,
      render: (label: string, row) => (
        <div>
          <div>{label || EMPTY}</div>
          <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
            {row.variant || row.uid || EMPTY}
          </Text>
        </div>
      ),
    },
    {
      title: "信号",
      dataIndex: "direction",
      width: 130,
      render: (direction: string, row) =>
        direction ? (
          <Tag bordered={false} color={direction === "UP" ? "success" : "error"}>
            {direction} → {row.side || EMPTY}
          </Tag>
        ) : (
          <Tag bordered={false}>{row.side || EMPTY}</Tag>
        ),
    },
    {
      title: "参考价",
      dataIndex: "sigLast",
      align: "right",
      width: 110,
      render: (value: number | null, row) => (
        <span className="manager-argus-mono">{fmtPrice(value ?? row.lastPx)}</span>
      ),
    },
    {
      title: "偏离 bp",
      dataIndex: "gapBp",
      align: "right",
      width: 100,
      render: (value: number | null, row) => (
        <Tooltip title={row.strengthLevel ? `强度分档：${row.strengthLevel}` : "无报价快照，算不出 gap_bp"}>
          <span className="manager-argus-mono" style={{ color: signColor(value) }}>
            {fmtSigned(value, 2)}
          </span>
        </Tooltip>
      ),
    },
    {
      title: "结果",
      dataIndex: "eventLabel",
      width: 130,
      render: (label: string, row) => (
        <Tag bordered={false} color={undefined} style={{ color: EVENT_COLORS[row.event] ?? RESULT_KIND_COLORS[row.resultKind] }}>
          {label || row.event}
        </Tag>
      ),
    },
    {
      title: "门控 / 成交明细",
      dataIndex: "reason",
      render: (reason: string, row) =>
        row.resultKind === "open" ? (
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            {fmtInt(row.orderSize)} 张 · 净仓 {fmtInt(row.size)} 张
          </Text>
        ) : (
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            <span className="manager-argus-mono">{row.gateLabel || row.gateKind || EMPTY}</span>{" "}
            {row.gateActual === null && row.gateThreshold === null
              ? reason
              : `${fmtSigned(row.gateActual, 2)} vs 阈值 ${fmtSigned(row.gateThreshold, 2)}`}
          </Text>
        ),
    },
    {
      title: "参数版本",
      dataIndex: "configVersion",
      width: 100,
      render: (version: number, row) => (
        <span className="manager-argus-mono" style={{ color: "var(--argus-faint)" }}>
          v{version}
          {row.source === 2 ? (
            <Tag bordered={false} style={{ marginInlineStart: 6, fontSize: 11 }}>
              {row.sourceLabel}
            </Tag>
          ) : null}
        </span>
      ),
    },
    {
      title: "",
      dataIndex: "eventId",
      width: 80,
      align: "right",
      render: (_: number, row) => (
        <Button size="small" onClick={() => onOpenDetail(row)}>
          详情
        </Button>
      ),
    },
  ];

  return (
    <Table<SignalEvent>
      className="manager-table"
      size="small"
      rowKey="eventId"
      loading={loading}
      columns={columns}
      dataSource={rows}
      scroll={{ x: "max-content" }}
      onRow={(row) => ({ onClick: () => onOpenDetail(row), style: { cursor: "pointer" } })}
      pagination={{
        current: page,
        pageSize: SIGNAL_PAGE_SIZE,
        total,
        showSizeChanger: false,
        onChange: onPageChange,
        showTotal: (value) => `共 ${value} 条判定（一行 = 一个账户在一次触发上的结果）`,
      }}
    />
  );
}
