"use client";

import { FileTextOutlined, HistoryOutlined, SafetyCertificateOutlined, UserOutlined } from "@ant-design/icons";
import { Tag, Tooltip, Typography } from "antd";
import type { ArgusConfigVersion } from "../api/argus-config.api";

const { Text } = Typography;

interface VersionRecordProps {
  version?: ArgusConfigVersion;
}

const statusMeta: Record<string, { label: string; color: string }> = {
  published: { label: "已发布", color: "green" },
  draft: { label: "草稿", color: "default" },
  archived: { label: "已归档", color: "default" },
};

function shorten(value: string, keep = 10) {
  if (!value) return "—";
  return value.length <= keep * 2 ? value : `${value.slice(0, keep)}…${value.slice(-6)}`;
}

export function VersionRecord({ version }: VersionRecordProps) {
  if (!version) {
    return (
      <div className="manager-argus-meta">
        <span className="manager-argus-meta__item"><HistoryOutlined />暂无已发布配置，请在下方完成首次发布</span>
      </div>
    );
  }

  const status = statusMeta[version.status?.toLowerCase()] ?? { label: version.status || "—", color: "blue" };

  return (
    <div className="manager-argus-meta">
      <span className="manager-argus-meta__item">
        <HistoryOutlined />
        发布记录
        <Tag color="gold" style={{ marginInlineEnd: 0 }}>v{version.version}</Tag>
        <Tag color={status.color} style={{ marginInlineEnd: 0 }}>{status.label}</Tag>
      </span>
      <span className="manager-argus-meta__item">
        <UserOutlined />
        <b>{version.publishedBy || "—"}</b>
        <Text type="secondary" style={{ fontSize: 12.5 }}>
          {version.publishedAt ? new Date(version.publishedAt).toLocaleString("zh-CN", { hour12: false }) : "—"}
        </Text>
      </span>
      <span className="manager-argus-meta__item">
        <FileTextOutlined />
        <Tooltip title={version.releaseNote || "本次发布未填写说明"}>
          <b style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {version.releaseNote || "未填写发布说明"}
          </b>
        </Tooltip>
      </span>
      <span className="manager-argus-meta__spacer" />
      <span className="manager-argus-meta__item">
        <SafetyCertificateOutlined />
        <Tooltip title={version.snapshotChecksum || "暂无快照校验值"}>
          <b className="manager-argus-mono">{shorten(version.snapshotChecksum)}</b>
        </Tooltip>
      </span>
    </div>
  );
}
