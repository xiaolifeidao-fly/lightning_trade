"use client";

import { HistoryOutlined, RollbackOutlined } from "@ant-design/icons";
import { Empty, Popconfirm, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ArgusConfigVersion } from "../api/argus-config.api";

const { Text } = Typography;

interface VersionHistoryProps {
  versions: ArgusConfigVersion[];
  saving: boolean;
  onRollback: (version: ArgusConfigVersion) => void;
}

const statusMeta: Record<string, { label: string; color: string }> = {
  published: { label: "生效中", color: "green" },
  draft: { label: "草稿", color: "default" },
  archived: { label: "已归档", color: "default" },
};

function shorten(value?: string, keep = 8) {
  if (!value) return "—";
  return value.length <= keep * 2 ? value : `${value.slice(0, keep)}…${value.slice(-6)}`;
}

export function VersionHistory({ versions, saving, onRollback }: VersionHistoryProps) {
  const columns: ColumnsType<ArgusConfigVersion> = [
    {
      title: "版本",
      dataIndex: "version",
      width: 84,
      render: (value: number, row) => (
        <b className={`manager-argus-mono${row.status === "published" ? " manager-argus-version--live" : ""}`}>v{value}</b>
      ),
    },
    {
      title: "发布说明",
      dataIndex: "releaseNote",
      render: (value: string, row) => (
        <div>
          <div>{value || <Text type="secondary">未填写发布说明</Text>}</div>
          <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11.5 }}>
            {row.publishedAt ? new Date(row.publishedAt).toLocaleString("zh-CN", { hour12: false }) : "未发布"}
            {row.publishedBy ? ` · ${row.publishedBy}` : ""}
          </Text>
        </div>
      ),
    },
    {
      title: "校验和",
      dataIndex: "snapshotChecksum",
      width: 132,
      render: (value: string) => (
        <Tooltip title={value || "暂无快照校验值"}>
          <span className="manager-argus-mono">{shorten(value)}</span>
        </Tooltip>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 92,
      render: (value: string) => {
        const meta = statusMeta[value?.toLowerCase()] ?? { label: value || "—", color: "blue" };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    {
      title: "操作",
      width: 92,
      render: (_, row) =>
        row.status === "archived" ? (
          <Popconfirm
            title={`回滚到 v${row.version}？`}
            description="该历史版本的不可变快照会被重新推上生效槽位并广播，不会新建版本号。"
            okText="确认回滚"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            disabled={saving}
            onConfirm={() => onRollback(row)}
          >
            <a className="manager-argus-rollback">
              <RollbackOutlined /> 回滚
            </a>
          </Popconfirm>
        ) : null,
    },
  ];

  return (
    <div className="manager-argus-panel">
      <div className="manager-argus-panel__head">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <HistoryOutlined />
          </span>
          版本历史
        </div>
        <Text type="secondary" style={{ fontSize: 12.5 }}>
          仅本实例，可回滚到任意已归档版本
        </Text>
      </div>
      {versions.length === 0 ? (
        <div className="manager-argus-empty">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该实例还没有配置版本" />
        </div>
      ) : (
        <Table
          rowKey="id"
          size="small"
          pagination={false}
          scroll={{ y: 320 }}
          columns={columns}
          dataSource={versions}
        />
      )}
    </div>
  );
}
