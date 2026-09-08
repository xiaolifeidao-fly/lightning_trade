"use client";

import { Button, Empty, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { SignalEvent } from "../api/argus-market.api";
import { TRIGGER_KIND_MAP } from "../constants";

const { Text } = Typography;

interface TriggerTableProps {
  triggers: SignalEvent[];
  selectedEventId: number | null;
  onSelect: (eventId: number) => void;
  onOpenSlice: (event: SignalEvent) => void;
  loading: boolean;
}

const fmtPx = (value: number | null | undefined) =>
  value == null ? "—" : value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const fmtSigned = (value: number | null | undefined, digits = 2) =>
  value == null ? "—" : `${value > 0 ? "+" : ""}${value.toFixed(digits)}`;

/**
 * 当前视图内的触发点。
 *
 * 一行 = 一个账户在一次触发上的判定结果，**不按分钟折叠**：实测 28.7% 的信号在
 * 同一分钟内触发 ≥2 次、承载 48.6% 的全部触发，折叠会直接丢掉约一半。所以同一分钟
 * 里的几次触发在这张表里是几行，各自可点、各自能下钻。
 */
export function TriggerTable({ triggers, selectedEventId, onSelect, onOpenSlice, loading }: TriggerTableProps) {
  const columns: ColumnsType<SignalEvent> = [
    {
      title: "时间",
      dataIndex: "ts",
      width: 152,
      // 秒必须显示：同一分钟内的多次触发只有秒能把它们分开。
      render: (ts: string) => <span className="manager-argus-mono">{ts.slice(5)}</span>,
    },
    {
      title: "账户",
      dataIndex: "accountLabel",
      width: 108,
      render: (label: string, row) => (
        <Text type="secondary">
          {label || "—"}
          {row.variant ? <span className="manager-market-variant">{row.variant}</span> : null}
        </Text>
      ),
    },
    {
      title: "信号",
      width: 128,
      render: (_, row) =>
        row.direction ? (
          <Tag color={row.direction === "UP" ? "green" : "red"}>
            {row.direction} → {row.side || "—"}
          </Tag>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
    {
      title: "参考价",
      align: "right",
      width: 112,
      render: (_, row) => <span className="manager-argus-mono">{fmtPx(row.sigMark ?? row.lastPx)}</span>,
    },
    {
      title: "偏离 bp",
      align: "right",
      width: 96,
      render: (_, row) => (
        <span className={`manager-argus-mono ${(row.gapBp ?? 0) >= 0 ? "manager-market-up" : "manager-market-down"}`}>
          {fmtSigned(row.gapBp)}
        </span>
      ),
    },
    {
      title: "结果",
      width: 116,
      render: (_, row) => {
        const meta = TRIGGER_KIND_MAP[row.event];
        return meta ? (
          <span className="manager-market-kind manager-market-kind--static">
            <i className="manager-market-kind__dot" style={{ background: meta.color }} />
            {meta.label}
          </span>
        ) : (
          <Text type="secondary">{row.eventLabel || row.event}</Text>
        );
      },
    },
    {
      title: "原因 / 明细",
      render: (_, row) =>
        row.event === "open" ? (
          <Text type="secondary">
            {row.orderSize ?? row.size ?? "—"} 张
            {row.avgPx != null ? ` · 均价 ${fmtPx(row.avgPx)}` : ""}
          </Text>
        ) : (
          <Text type="secondary" className="manager-market-reason">
            {row.gateLabel ? `${row.gateLabel}：` : ""}
            {row.reason || "—"}
          </Text>
        ),
    },
    {
      title: "",
      width: 108,
      align: "right",
      render: (_, row) => (
        <Button
          size="small"
          onClick={(event) => {
            event.stopPropagation();
            onOpenSlice(row);
          }}
        >
          秒级切片
        </Button>
      ),
    },
  ];

  return (
    <Table<SignalEvent>
      className="manager-table manager-market-table"
      rowKey="eventId"
      size="small"
      loading={loading}
      columns={columns}
      dataSource={triggers}
      pagination={{ pageSize: 20, showSizeChanger: false, size: "small" }}
      rowClassName={(row) => (row.eventId === selectedEventId ? "manager-market-row--selected" : "")}
      onRow={(row) => ({ onClick: () => onSelect(row.eventId) })}
      locale={{
        emptyText: (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="当前窗口内没有符合筛选条件的触发点：放开上方的「触发点类型」，或换一个时间范围。"
          />
        ),
      }}
    />
  );
}
