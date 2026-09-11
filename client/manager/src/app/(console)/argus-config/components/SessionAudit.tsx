"use client";

import { KeyOutlined } from "@ant-design/icons";
import { Alert, Button, Collapse, Empty, Form, Input, Modal, Table, Tag, Tooltip, Typography, message } from "antd";
import type { ColumnsType } from "antd/es/table";
import { useState } from "react";
import {
  rotateArgusSession,
  type ArgusAccount,
  type ArgusRuntimeSession,
  type ArgusSessionRotatePayload,
} from "../api/argus-config.api";

const { Text } = Typography;

interface SessionAuditProps {
  sessions: ArgusRuntimeSession[];
  accounts: ArgusAccount[];
  instanceKey: string;
  /** 写入成功后让页面重新拉快照，否则表里还是旧的长度与时间。 */
  onRotated?: () => void;
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

interface RotateFormValues {
  cookie: string;
  token: string;
  otoken?: string;
  sentryRelease?: string;
  sentryPublicKey?: string;
  baggage?: string;
}

/**
 * 账户会话巡检与凭证更新。
 *
 * 表格**只回长度不回明文**（服务端 sessionDTO 一律走 maskSecret），这一点没变。
 * 更新走的是**写入式**表单：打开时永远是空的，填什么就换成什么，不存在"在现有值
 * 上编辑"——既保住了不回显明文这条约束，又把唯一缺的能力补上。
 *
 * 为什么必须有这个入口：账户是 login_type=config（静态凭证），
 * trade.BuildUserProvider 走 StaticUserProvider，**进程永远不会自己重登**
 * （只有 password 模式才会去调 pl-instance 的无头登录，而生产上没部署它）。
 * 所以 cookie 过期后既没有自动续期，此前也没有产品化入口，只能直接改库
 * （argus_runtime_session 里还留着 rotate-drill 这类手工痕迹）。
 */
export function SessionAudit({ sessions, accounts, instanceKey, onRotated }: SessionAuditProps) {
  const [form] = Form.useForm<RotateFormValues>();
  const [editing, setEditing] = useState<ArgusRuntimeSession | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [pasted, setPasted] = useState("");

  const accountName = (accountId: number) =>
    accounts.find((account) => account.id === accountId)?.accountName ?? `账户 #${accountId}`;

  const openRotate = (row: ArgusRuntimeSession) => {
    form.resetFields();
    setPasted("");
    setEditing(row);
  };

  /**
   * 从整份 session.json 自动填充。
   *
   * 运维手上拿到的就是这个文件（浏览器导出或 pl-instance 产出），逐个字段复制
   * 一段 900+ 字符的 cookie 最容易出错。按账户名或 uid 找对应条目，找不到就明说，
   * **绝不**在只有一个条目时"顺手用它"——那正是把 A 的凭证写到 B 头上的路径。
   */
  const applyPasted = () => {
    const raw = pasted.trim();
    if (!raw) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch {
      message.error("不是合法的 JSON");
      return;
    }
    const container = parsed as { accounts?: Record<string, Record<string, unknown>> };
    const entries = container?.accounts;
    if (!entries || typeof entries !== "object") {
      message.error("JSON 里没有 accounts 字段，确认粘贴的是 session.json");
      return;
    }
    const wantName = editing ? accountName(editing.accountId) : "";
    const wantUid = accounts.find((item) => item.id === editing?.accountId)?.uid ?? "";
    const hit = Object.entries(entries).find(([key, value]) => {
      const uid = String(value?.uid ?? "").trim();
      if (wantUid && uid && uid === wantUid) return true;
      return key === wantName || String(value?.accountName ?? "").trim() === wantName;
    });
    if (!hit) {
      message.error(`这份 session.json 里没有「${wantName}」（按 uid ${wantUid || "未知"} 与账户名都没找到）`);
      return;
    }
    const value = hit[1];
    form.setFieldsValue({
      cookie: String(value.cookie ?? ""),
      token: String(value.token ?? ""),
      otoken: String(value.otoken ?? ""),
      sentryRelease: String(value.sentryRelease ?? ""),
      sentryPublicKey: String(value.sentryPublicKey ?? ""),
      baggage: String(value.baggage ?? ""),
    });
    message.success(`已填入「${wantName}」的凭证，提交前请确认长度`);
  };

  const submit = async () => {
    if (!editing) return;
    const values = await form.validateFields();
    const payload: ArgusSessionRotatePayload = {
      accountId: editing.accountId,
      cookie: values.cookie.trim(),
      token: values.token.trim(),
      otoken: values.otoken?.trim() || undefined,
      sentryRelease: values.sentryRelease?.trim() || undefined,
      sentryPublicKey: values.sentryPublicKey?.trim() || undefined,
      baggage: values.baggage?.trim() || undefined,
    };
    setSubmitting(true);
    try {
      const result = await rotateArgusSession(instanceKey, payload);
      if (result.action === "unchanged") {
        message.info("提交的凭证与库里现有的一致，未写入（避免白白触发一次热加载）");
      } else {
        message.success(
          `已更新 ${result.accountName}：cookie ${result.cookieLength} 字符 / token ${result.tokenLength} 字符。` +
            (result.notified ? "已通知实例立即重载。" : "通知未送达，实例将在 60 秒内自行生效。"),
        );
      }
      setEditing(null);
      onRotated?.();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新失败");
    } finally {
      setSubmitting(false);
    }
  };

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
    {
      title: "操作",
      key: "action",
      width: 108,
      fixed: "right",
      render: (_, row) => (
        <Button size="small" type="link" onClick={() => openRotate(row)}>
          更新凭证
        </Button>
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
        <Tag>运行时状态 · argus_runtime_session</Tag>
      </div>
      {sessions.length === 0 ? (
        <div className="manager-argus-empty">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该实例暂无会话回写记录" />
        </div>
      ) : (
        <Table rowKey="id" size="small" pagination={false} columns={columns} dataSource={sessions} scroll={{ x: 1080 }} />
      )}
      <div className="manager-argus-hint">
        <span>
          <b>为什么不放进参数编辑表单：</b>cookie / token 不是参数，它们在
          <span className="manager-argus-mono"> argus_runtime_session </span>
          单独一张表，改它不该新建配置版本——否则版本历史里全是凭证轮换的噪音。所以更新走上面的「更新凭证」，
          写完直接通知实例热加载。<b>表格与接口一律只回长度不回明文</b>，更新表单也是写入式的：打开永远是空的，
          填什么换什么，不做「在现有值上编辑」。
        </span>
      </div>

      <Modal
        open={editing !== null}
        title={`更新会话凭证 · ${editing ? accountName(editing.accountId) : ""}`}
        okText="确认更新"
        cancelText="取消"
        confirmLoading={submitting}
        onOk={() => void submit()}
        onCancel={() => setEditing(null)}
        width={720}
        destroyOnClose
      >
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="写完会立刻生效，并触发一次热加载"
          description="热加载会重建交易管理器与监控器。当前有持仓时，挑信号间隙操作。（这次热加载躲不掉：内容指纹变了，就算不发通知，实例也会在 60 秒内自行重载。）"
        />
        <Collapse
          size="small"
          style={{ marginBottom: 16 }}
          items={[
            {
              key: "paste",
              label: "从整份 session.json 粘贴（推荐）",
              children: (
                <>
                  <Input.TextArea
                    rows={4}
                    value={pasted}
                    onChange={(event) => setPasted(event.target.value)}
                    placeholder='{"accounts": {"账户A-...": {"uid": "...", "cookie": "...", "token": "..."}}}'
                  />
                  <Button size="small" style={{ marginTop: 8 }} onClick={applyPasted}>
                    按本账户的 uid / 名字自动填充
                  </Button>
                </>
              ),
            },
          ]}
        />
        <Form form={form} layout="vertical" requiredMark>
          <Form.Item
            name="cookie"
            label="Cookie"
            rules={[{ required: true, message: "cookie 必填" }]}
            extra="整串 Cookie 头，通常 800~1400 字符"
          >
            <Input.TextArea rows={4} placeholder="粘贴新的 Cookie" />
          </Form.Item>
          <Form.Item name="token" label="Token" rules={[{ required: true, message: "token 必填" }]}>
            <Input placeholder="粘贴新的 Token" />
          </Form.Item>
          <Form.Item name="otoken" label="OToken" extra="留空即不设置。运行时优先用 OToken，没有才回退 Token。">
            <Input placeholder="可选" />
          </Form.Item>
          <Collapse
            size="small"
            items={[
              {
                key: "advanced",
                label: "Sentry 与 Baggage（可选）",
                children: (
                  <>
                    <Form.Item name="sentryRelease" label="Sentry Release">
                      <Input placeholder="可选" />
                    </Form.Item>
                    <Form.Item name="sentryPublicKey" label="Sentry Public Key">
                      <Input placeholder="可选" />
                    </Form.Item>
                    <Form.Item name="baggage" label="Baggage" style={{ marginBottom: 0 }}>
                      <Input.TextArea rows={2} placeholder="可选" />
                    </Form.Item>
                  </>
                ),
              },
            ]}
          />
        </Form>
      </Modal>
    </div>
  );
}
