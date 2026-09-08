"use client";

import { KeyOutlined } from "@ant-design/icons";
import { Empty, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ArgusAccount, ArgusRuntimeSession } from "../api/argus-config.api";

const { Text } = Typography;

interface SessionAuditProps {
  sessions: ArgusRuntimeSession[];
  accounts: ArgusAccount[];
}

/** 会话即将过期的判定窗口：不足 12 小时就提前标黄，留出人工重登的时间。 */
const EXPIRING_SOON_MS = 12 * 60 * 60 * 1000;

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function sessionState(session: ArgusRuntimeSession) {
  if (session.valid !== 1) return { label: "失效", color: "red" as const };
  if (!session.expiresAt) return { label: "有效", color: "green" as const };
  const expires = new Date(session.expiresAt).getTime();
  if (Number.isNaN(expires)) return { label: "有效", color: "green" as const };
  if (expires <= Date.now()) return { label: "已过期", color: "red" as const };
  if (expires - Date.now() <= EXPIRING_SOON_MS) return { label: "即将过期", color: "gold" as const };
  return { label: "有效", color: "green" as const };
}

function secretCell(length: number) {
  if (!length) return <Text type="secondary">未回写</Text>;
  return <span className="manager-argus-mono manager-argus-secret">{length} 字符 · 已加密</span>;
}

/**
 * 账户会话只读巡检。cookie / token / sentry 是程序登录后自己刷新的运行时状态，
 * 由 InstallSessionWriteBack() 自动回写，不属于配置版本快照——所以它们既不进参数
 * 编辑表单，这里也只回长度不回明文。
 */
export function SessionAudit({ sessions, accounts }: SessionAuditProps) {
  const accountName = (accountId: number) =>
    accounts.find((account) => account.id === accountId)?.accountName ?? `账户 #${accountId}`;

  const columns: ColumnsType<ArgusRuntimeSession> = [
    { title: "账户", dataIndex: "accountId", width: 160, render: (value: number) => accountName(value) },
    {
      title: "会话",
      dataIndex: "valid",
      width: 104,
      render: (_, row) => {
        const state = sessionState(row);
        return <Tag color={state.color}>{state.label}</Tag>;
      },
    },
    { title: "Cookie", dataIndex: "cookieLength", width: 150, render: (value: number) => secretCell(value) },
    { title: "Token", dataIndex: "tokenLength", width: 150, render: (value: number) => secretCell(value) },
    {
      title: "最后刷新",
      dataIndex: "sessionUpdatedAt",
      width: 176,
      render: (value: string) => <span className="manager-argus-mono">{formatDate(value)}</span>,
    },
    {
      title: "到期",
      dataIndex: "expiresAt",
      width: 176,
      render: (value?: string) => <span className="manager-argus-mono">{formatDate(value)}</span>,
    },
    {
      title: "最近错误",
      dataIndex: "lastError",
      render: (value?: string) =>
        value ? (
          <Tooltip title={value}>
            <Text type="danger" ellipsis style={{ maxWidth: 220, display: "inline-block" }}>
              {value}
            </Text>
          </Tooltip>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
  ];

  return (
    <div className="manager-argus-panel">
      <div className="manager-argus-panel__head">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <KeyOutlined />
          </span>
          账户会话状态
        </div>
        <Tag>运行时状态 · 只读 · argus_runtime_session</Tag>
      </div>
      {sessions.length === 0 ? (
        <div className="manager-argus-empty">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该实例暂无会话回写记录" />
        </div>
      ) : (
        <Table rowKey="id" size="small" pagination={false} columns={columns} dataSource={sessions} />
      )}
      <div className="manager-argus-hint">
        <span>
          <b>为什么不放进参数编辑表单：</b>cookie / token / sentry 是程序登录后自己刷新的运行时状态，由
          <span className="manager-argus-mono"> InstallSessionWriteBack() </span>
          自动回写，不属于配置版本快照（所以 <span className="manager-argus-mono">argus_runtime_session</span> 才和
          <span className="manager-argus-mono"> argus_account </span>
          分表）。手工编辑一段约 950 字符的 cookie 没有实际意义，只会扩大泄露面。这里只做只读巡检，服务端也只回长度不回明文。
        </span>
      </div>
    </div>
  );
}
