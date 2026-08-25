"use client";

import {
  ApiOutlined,
  BellOutlined,
  DeleteOutlined,
  ExperimentOutlined,
  InfoCircleOutlined,
  KeyOutlined,
  LineChartOutlined,
  LockOutlined,
  PlusOutlined,
  RocketOutlined,
  SettingOutlined,
  TeamOutlined,
  UndoOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Switch,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import type { ArgusConfigDraft, ArgusConfigSnapshot } from "../api/argus-config.api";

const { Text } = Typography;
const sensitivePlaceholder = "已安全保存，不回显；输入新值覆盖";

interface ConfigEditorProps {
  snapshot: ArgusConfigSnapshot | null;
  saving: boolean;
  dirty: boolean;
  onDirtyChange: (dirty: boolean) => void;
  onPublish: (draft: ArgusConfigDraft) => Promise<void>;
}

type TabKey = "basic" | "ai" | "accounts" | "symbols" | "notification" | "sessions";

const fieldTabMap: Record<string, TabKey> = {
  config: "basic",
  accounts: "accounts",
  accountRisks: "accounts",
  monitorSymbols: "symbols",
  notification: "notification",
  sessions: "sessions",
};

const aiFieldPrefixes = ["aiClose", "aiOpen"];

function Section({ title, desc, extra, children }: { title: string; desc?: string; extra?: ReactNode; children: ReactNode }) {
  return (
    <div className="manager-argus-section">
      <div className="manager-argus-section__head">
        <span className="manager-argus-section__bar" />
        <span className="manager-argus-section__title">{title}</span>
        {desc ? <span className="manager-argus-section__desc">{desc}</span> : null}
        <span style={{ flex: 1 }} />
        {extra}
      </div>
      {children}
    </div>
  );
}

function Hint({ children }: { children: ReactNode }) {
  return (
    <div className="manager-argus-hint">
      <InfoCircleOutlined />
      <span>{children}</span>
    </div>
  );
}

function NumberField({
  name,
  label,
  min = 0,
  max,
  precision = 0,
  suffix,
  tooltip,
}: {
  name: string;
  label: string;
  min?: number;
  max?: number;
  precision?: number;
  suffix?: string;
  tooltip?: string;
}) {
  return (
    <Form.Item name={["config", name]} label={label} tooltip={tooltip}>
      <InputNumber min={min} max={max} precision={precision} addonAfter={suffix} style={{ width: "100%" }} />
    </Form.Item>
  );
}

function BooleanField({ name, label, tooltip }: { name: string; label: string; tooltip?: string }) {
  return (
    <Form.Item
      name={["config", name]}
      label={label}
      tooltip={tooltip}
      valuePropName="checked"
      getValueProps={(value: number | boolean) => ({ checked: value === 1 || value === true })}
      normalize={(value: boolean) => (value ? 1 : 0)}
    >
      <Switch checkedChildren="启用" unCheckedChildren="停用" />
    </Form.Item>
  );
}

function SecretField({ name, label, tooltip }: { name: (string | number)[]; label: string; tooltip?: string }) {
  return (
    <Form.Item name={name} label={label} tooltip={tooltip ?? "服务端不回显该字段，留空即保留当前已保存的值"}>
      <Input.Password prefix={<LockOutlined style={{ color: "var(--manager-text-faint)" }} />} placeholder={sensitivePlaceholder} autoComplete="new-password" />
    </Form.Item>
  );
}

function TabLabel({ icon, text, count, active }: { icon: ReactNode; text: string; count?: number; active?: boolean }) {
  return (
    <span className="manager-argus-tab-label">
      {icon}
      <span style={{ flex: 1 }}>{text}</span>
      {count === undefined ? null : (
        <span className={`manager-argus-tab-label__count${active ? " manager-argus-tab-label__count--on" : ""}`}>{count}</span>
      )}
    </span>
  );
}

export function ConfigEditor({ snapshot, saving, dirty, onDirtyChange, onPublish }: ConfigEditorProps) {
  const [form] = Form.useForm<ArgusConfigDraft>();
  const [activeTab, setActiveTab] = useState<TabKey>("basic");
  const [publishOpen, setPublishOpen] = useState(false);

  const watchedAccounts = Form.useWatch("accounts", form);
  const watchedRisks = Form.useWatch("accountRisks", form);
  const watchedSymbols = Form.useWatch("monitorSymbols", form);
  const watchedSessions = Form.useWatch("sessions", form);

  const accounts = watchedAccounts ?? snapshot?.accounts;
  const accountRisks = watchedRisks ?? snapshot?.accountRisks;
  const monitorSymbols = watchedSymbols ?? snapshot?.monitorSymbols;
  const sessions = watchedSessions ?? snapshot?.sessions;

  const initialValues: ArgusConfigDraft | undefined = useMemo(
    () =>
      snapshot
        ? {
            config: snapshot.config,
            accounts: snapshot.accounts,
            accountRisks: snapshot.accountRisks,
            monitorSymbols: snapshot.monitorSymbols,
            notification: snapshot.notification,
            sessions: snapshot.sessions,
            releaseNote: snapshot.version.releaseNote,
          }
        : undefined,
    [snapshot],
  );

  const focusError = (errorFields: { name: (string | number)[] }[]) => {
    const first = errorFields[0];
    if (!first) return;
    const root = String(first.name[0]);
    const child = String(first.name[1] ?? "");
    let target = fieldTabMap[root] ?? "basic";
    if (root === "config" && aiFieldPrefixes.some((prefix) => child.startsWith(prefix))) target = "ai";
    setActiveTab(target);
    form.scrollToField(first.name, { behavior: "smooth", block: "center" });
  };

  const submit = async () => {
    try {
      const values = await form.validateFields();
      await onPublish(values);
      setPublishOpen(false);
      onDirtyChange(false);
    } catch (error) {
      const fields = (error as { errorFields?: { name: (string | number)[] }[] }).errorFields;
      if (fields?.length) {
        setPublishOpen(false);
        focusError(fields);
      }
    }
  };

  const reset = () => {
    form.resetFields();
    onDirtyChange(false);
  };

  const tabs = [
    {
      key: "basic",
      label: <TabLabel icon={<SettingOutlined />} text="主配置" active={activeTab === "basic"} />,
      children: <BasicConfig />,
    },
    {
      key: "ai",
      label: <TabLabel icon={<ExperimentOutlined />} text="AI 决策" active={activeTab === "ai"} />,
      children: <AiConfig />,
    },
    {
      key: "accounts",
      label: (
        <TabLabel icon={<TeamOutlined />} text="账户与风控" count={accounts?.length ?? 0} active={activeTab === "accounts"} />
      ),
      children: <AccountsAndRisk accountCount={accounts?.length ?? 0} riskCount={accountRisks?.length ?? 0} />,
    },
    {
      key: "symbols",
      label: (
        <TabLabel icon={<LineChartOutlined />} text="监控币种" count={monitorSymbols?.length ?? 0} active={activeTab === "symbols"} />
      ),
      children: <MonitorSymbols />,
    },
    {
      key: "notification",
      label: <TabLabel icon={<BellOutlined />} text="通知" active={activeTab === "notification"} />,
      children: <NotificationConfig />,
    },
    {
      key: "sessions",
      label: <TabLabel icon={<KeyOutlined />} text="会话状态" count={sessions?.length ?? 0} active={activeTab === "sessions"} />,
      children: <Sessions />,
    },
  ];

  return (
    <Form<ArgusConfigDraft>
      form={form}
      layout="vertical"
      initialValues={initialValues}
      key={snapshot?.version.id ?? "empty"}
      onValuesChange={() => onDirtyChange(true)}
      scrollToFirstError
    >
      <div className="manager-argus-editor">
        <div className="manager-argus-toolbar">
          <div>
            <div className="manager-argus-toolbar__title">
              <span className="manager-argus-panel__icon"><ApiOutlined /></span>
              配置维护
            </div>
            <Text className="manager-argus-toolbar__desc">
              发布将创建不可变版本、同步运行快照，并通知在线 Argus 热加载。
            </Text>
          </div>
          <div className="manager-argus-toolbar__actions">
            {dirty ? <span className="manager-argus-dirty">● 有未发布修改</span> : null}
            <Popconfirm
              title="放弃当前修改？"
              description="表单将恢复为最近一次已发布的配置内容。"
              okText="放弃修改"
              cancelText="取消"
              disabled={!dirty || saving}
              onConfirm={reset}
            >
              <Button icon={<UndoOutlined />} disabled={!dirty || saving}>重置</Button>
            </Popconfirm>
            <Tooltip title={snapshot ? "" : "尚未加载配置快照"}>
              <Button type="primary" icon={<RocketOutlined />} loading={saving} disabled={!snapshot} onClick={() => setPublishOpen(true)}>
                发布并热加载
              </Button>
            </Tooltip>
          </div>
        </div>

        <div className="manager-argus-body">
          <Tabs
            className="manager-argus-tabs"
            tabPosition="left"
            activeKey={activeTab}
            onChange={(key) => setActiveTab(key as TabKey)}
            items={tabs}
          />
        </div>
      </div>

      <Modal
        title="发布配置并热加载"
        open={publishOpen}
        onCancel={() => setPublishOpen(false)}
        onOk={() => void submit()}
        okText="确认发布"
        cancelText="取消"
        confirmLoading={saving}
        destroyOnClose={false}
        width={560}
      >
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="发布后立即生效"
          description="系统会保存草稿、生成新的不可变版本，并向在线 Argus 下发热加载指令；交易与监控行为会随之改变。"
        />
        <Form.Item
          name="releaseNote"
          label="发布说明"
          rules={[{ max: 500, message: "发布说明最多 500 个字符" }]}
          extra="建议写清本次调整的内容与原因，便于回溯版本。"
        >
          <Input.TextArea rows={3} placeholder="例如：调高 BTC 信号阈值，关闭 AI 自动开仓。" showCount maxLength={500} />
        </Form.Item>
      </Modal>
    </Form>
  );
}

function BasicConfig() {
  return (
    <>
      <Section title="服务运行" desc="进程监听与日志落盘位置">
        <Row gutter={16}>
          <Col xs={24} md={8}>
            <Form.Item name={["config", "serverPort"]} label="服务端口" rules={[{ required: true, message: "请输入服务端口" }]}>
              <InputNumber min={1} max={65535} style={{ width: "100%" }} />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}><Form.Item name={["config", "requestPath"]} label="请求路径"><Input placeholder="/argus" /></Form.Item></Col>
          <Col xs={24} md={8}><Form.Item name={["config", "logDir"]} label="日志目录"><Input placeholder="/var/log/argus" /></Form.Item></Col>
        </Row>
      </Section>

      <Section title="总开关" desc="紧急情况下可仅关闭交易，保留监控">
        <Row gutter={16}>
          <Col xs={12} md={6}><BooleanField name="enabled" label="Argus 总开关" tooltip="关闭后监控与交易全部停止" /></Col>
          <Col xs={12} md={6}><BooleanField name="tradeEnabled" label="交易开关" tooltip="关闭后仅监控与告警，不再下单" /></Col>
          <Col xs={12} md={6}><NumberField name="defaultOrderSize" label="默认下单量" suffix="张" /></Col>
          <Col xs={12} md={6}><NumberField name="monitorIntervalSecond" label="监控间隔" min={1} suffix="秒" /></Col>
        </Row>
      </Section>

      <Section title="止盈止损" desc="全局阈值，账户风控可进一步覆盖">
        <Row gutter={16}>
          <Col xs={12} md={6}><NumberField name="profitThreshold" label="止盈阈值" precision={4} /></Col>
          <Col xs={12} md={6}><NumberField name="lossThreshold" label="止损阈值" precision={4} /></Col>
          <Col xs={12} md={6}><NumberField name="sessionMaxAgeDay" label="会话最长保留" suffix="天" /></Col>
        </Row>
      </Section>

      <Section title="定时登录" desc="用于维持交易所会话有效期">
        <Row gutter={16}>
          <Col xs={12} md={6}><BooleanField name="loginScheduledEnabled" label="定时登录" /></Col>
          <Col xs={12} md={6}><NumberField name="loginScheduledHour" label="登录时刻（时）" min={0} max={23} suffix="时" /></Col>
          <Col xs={12} md={6}><NumberField name="loginScheduledMinute" label="登录时刻（分）" min={0} max={59} suffix="分" /></Col>
        </Row>
      </Section>

      <Section title="扩展配置" desc="仅在需要下发结构化附加参数时填写">
        <Form.Item
          name={["config", "extraConfigJson"]}
          label="扩展配置 JSON"
          rules={[
            {
              validator: (_, value: string) => {
                if (!value) return Promise.resolve();
                try {
                  JSON.parse(value);
                  return Promise.resolve();
                } catch {
                  return Promise.reject(new Error("请输入合法的 JSON"));
                }
              },
            },
          ]}
        >
          <Input.TextArea rows={4} placeholder='{"featureFlag": true}' className="manager-argus-mono" />
        </Form.Item>
      </Section>
    </>
  );
}

function AiConfig() {
  return (
    <>
      <Hint>AI 决策相关密钥由服务端加密保存，不会回显；留空表示保留当前值。</Hint>

      <Section title="AI 平仓" desc="按持仓状态触发的智能减仓决策">
        <Row gutter={16}>
          <Col xs={12} md={6}><BooleanField name="aiCloseEnabled" label="启用 AI 平仓" /></Col>
          <Col xs={12} md={6}><Form.Item name={["config", "aiCloseProvider"]} label="服务商"><Input placeholder="openai / deepseek" /></Form.Item></Col>
          <Col xs={24} md={12}><Form.Item name={["config", "aiCloseApiUrl"]} label="API 地址"><Input placeholder="https://api.example.com/v1/chat/completions" /></Form.Item></Col>
          <Col xs={24} md={12}><SecretField name={["config", "aiCloseApiKey"]} label="API Key" /></Col>
          <Col xs={12} md={6}><Form.Item name={["config", "aiCloseModel"]} label="模型"><Input /></Form.Item></Col>
          <Col xs={12} md={6}><NumberField name="aiCloseTimeoutSecond" label="超时" suffix="秒" /></Col>
          <Col xs={12} md={6}><NumberField name="aiCloseMaxTokens" label="最大 Token" /></Col>
          <Col xs={12} md={6}><NumberField name="aiCloseTemperature" label="温度" precision={2} tooltip="值越高结果越发散，建议 0 ~ 1" /></Col>
        </Row>
      </Section>

      <Section title="AI 开仓" desc="模型信号驱动的自动开仓与仓位约束">
        <Row gutter={16}>
          <Col xs={12} md={6}><BooleanField name="aiOpenEnabled" label="启用 AI 开仓" /></Col>
          <Col xs={12} md={6}><BooleanField name="aiOpenAutoTrade" label="AI 自动交易" tooltip="关闭时仅产生建议，不自动下单" /></Col>
          <Col xs={24} md={12}><Form.Item name={["config", "aiOpenApiUrl"]} label="API 地址"><Input /></Form.Item></Col>
          <Col xs={24} md={12}><SecretField name={["config", "aiOpenApiKey"]} label="API Key" /></Col>
          <Col xs={12} md={6}><Form.Item name={["config", "aiOpenModel"]} label="模型"><Input /></Form.Item></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenTimeoutSecond" label="超时" suffix="秒" /></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenMaxTokens" label="最大 Token" /></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenTemperature" label="温度" precision={2} /></Col>
        </Row>
      </Section>

      <Section title="开仓风控边界" desc="限制单笔与总持仓规模，防止模型异常放大风险">
        <Row gutter={16}>
          <Col xs={12} md={6}><NumberField name="aiOpenMinOrderContracts" label="最小合约数" suffix="张" /></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenMaxOrderContracts" label="单笔最大合约数" suffix="张" /></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenMaxTotalContracts" label="最大总合约数" suffix="张" /></Col>
          <Col xs={12} md={6}><NumberField name="aiOpenCooldownMinute" label="冷却时间" suffix="分钟" /></Col>
        </Row>
      </Section>
    </>
  );
}

function AccountsAndRisk({ accountCount, riskCount }: { accountCount: number; riskCount: number }) {
  return (
    <>
      <Hint>用户名、密码、API Key 等敏感信息由服务端加密存储，页面不会回显原值。</Hint>
      <Row gutter={16}>
        <Col xs={24} xl={14}>
          <Form.List name="accounts">
            {(fields, { add, remove }) => (
              <Section
                title="交易账户"
                desc={`共 ${accountCount} 个`}
                extra={<Button size="small" type="primary" ghost icon={<PlusOutlined />} onClick={() => add({ enabled: 1, loginHeadless: 1 })}>新增账户</Button>}
              >
                {fields.length === 0 ? (
                  <div className="manager-argus-empty">
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未配置交易账户" />
                  </div>
                ) : null}
                {fields.map((field, index) => (
                  <div className="manager-argus-item" key={field.key}>
                    <div className="manager-argus-item__head">
                      <span className="manager-argus-item__index">{index + 1}</span>
                      <div className="manager-argus-item__title">
                        <Form.Item name={[field.name, "accountName"]} rules={[{ required: true, message: "请输入账户名称" }]} noStyle>
                          <Input placeholder="账户名称" />
                        </Form.Item>
                      </div>
                      <Form.Item
                        name={[field.name, "enabled"]}
                        noStyle
                        valuePropName="checked"
                        getValueProps={(value: number) => ({ checked: value === 1 })}
                        normalize={(value: boolean) => (value ? 1 : 0)}
                      >
                        <Switch size="small" checkedChildren="启用" unCheckedChildren="停用" />
                      </Form.Item>
                      <Popconfirm title="删除该账户？" description="发布后该账户将不再参与交易。" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(field.name)}>
                        <Button type="text" danger icon={<DeleteOutlined />} />
                      </Popconfirm>
                    </div>
                    <Row gutter={12}>
                      <Col span={12}><Form.Item name={[field.name, "platform"]} label="平台"><Input placeholder="deepcoin / binance" /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "uid"]} label="UID"><Input /></Form.Item></Col>
                      <Col span={24}><Form.Item name={[field.name, "url"]} label="登录地址"><Input placeholder="https://" /></Form.Item></Col>
                      <Col span={12}><SecretField name={[field.name, "username"]} label="用户名" /></Col>
                      <Col span={12}><SecretField name={[field.name, "password"]} label="密码" /></Col>
                      <Col span={8}><Form.Item name={[field.name, "positionMode"]} label="仓位模式"><Input placeholder="cross / isolated" /></Form.Item></Col>
                      <Col span={8}><Form.Item name={[field.name, "positionSide"]} label="持仓方向"><Input placeholder="long / short / both" /></Form.Item></Col>
                      <Col span={8}><Form.Item name={[field.name, "initialBalance"]} label="初始余额"><InputNumber min={0} addonAfter="USDT" style={{ width: "100%" }} /></Form.Item></Col>
                      <Col span={12}><SecretField name={[field.name, "apiKey"]} label="API Key" /></Col>
                      <Col span={12}><SecretField name={[field.name, "secretKey"]} label="Secret Key" /></Col>
                    </Row>
                  </div>
                ))}
              </Section>
            )}
          </Form.List>
        </Col>
        <Col xs={24} xl={10}>
          <Form.List name="accountRisks">
            {(fields, { add, remove }) => (
              <Section
                title="账户风控"
                desc={`共 ${riskCount} 条`}
                extra={<Button size="small" icon={<PlusOutlined />} onClick={() => add({ reverseGateEnabled: 0 })}>新增风控</Button>}
              >
                {fields.length === 0 ? (
                  <div className="manager-argus-empty">
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未配置账户风控" />
                  </div>
                ) : null}
                {fields.map((field, index) => (
                  <div className="manager-argus-item" key={field.key}>
                    <div className="manager-argus-item__head">
                      <span className="manager-argus-item__index">{index + 1}</span>
                      <span className="manager-argus-item__title" style={{ color: "var(--manager-text)", fontWeight: 600 }}>风控规则</span>
                      <Popconfirm title="删除该风控规则？" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(field.name)}>
                        <Button type="text" danger icon={<DeleteOutlined />} />
                      </Popconfirm>
                    </div>
                    <Row gutter={12}>
                      <Col span={12}><Form.Item name={[field.name, "accountId"]} label="账户 ID"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "maxContracts"]} label="最大合约数"><InputNumber min={0} addonAfter="张" style={{ width: "100%" }} /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "takeProfitMode"]} label="止盈模式"><Input /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "stopLossMode"]} label="止损模式"><Input /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "riskBudget"]} label="风险预算"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
                      <Col span={12}><Form.Item name={[field.name, "catastrophicStopLoss"]} label="灾难止损"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
                    </Row>
                  </div>
                ))}
              </Section>
            )}
          </Form.List>
        </Col>
      </Row>
    </>
  );
}

function MonitorSymbols() {
  return (
    <Form.List name="monitorSymbols">
      {(fields, { add, remove }) => (
        <Section
          title="监控币种"
          desc="行情标识用于取价，交易标识用于下单"
          extra={<Button size="small" type="primary" ghost icon={<PlusOutlined />} onClick={() => add({ enabled: 1 })}>新增币种</Button>}
        >
          {fields.length === 0 ? (
            <div className="manager-argus-empty">
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未配置监控币种" />
            </div>
          ) : null}
          {fields.map((field, index) => (
            <div className="manager-argus-item" key={field.key}>
              <div className="manager-argus-item__head">
                <span className="manager-argus-item__index">{index + 1}</span>
                <Form.Item shouldUpdate noStyle>
                  {({ getFieldValue }) => (
                    <span className="manager-argus-item__title manager-argus-mono" style={{ color: "var(--manager-text)", fontWeight: 700 }}>
                      {getFieldValue(["monitorSymbols", field.name, "symbol"]) || "新币种"}
                    </span>
                  )}
                </Form.Item>
                <Form.Item
                  name={[field.name, "enabled"]}
                  noStyle
                  valuePropName="checked"
                  getValueProps={(value: number) => ({ checked: value === 1 })}
                  normalize={(value: boolean) => (value ? 1 : 0)}
                >
                  <Switch size="small" checkedChildren="监控" unCheckedChildren="停用" />
                </Form.Item>
                <Popconfirm title="删除该监控币种？" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(field.name)}>
                  <Button type="text" danger icon={<DeleteOutlined />} />
                </Popconfirm>
              </div>
              <Row gutter={12}>
                <Col xs={24} md={6}>
                  <Form.Item name={[field.name, "symbol"]} label="币种" rules={[{ required: true, message: "请输入币种" }]}>
                    <Input placeholder="BTC" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={6}><Form.Item name={[field.name, "deepInstrument"]} label="行情标识"><Input placeholder="BTC-USDT-SWAP" /></Form.Item></Col>
                <Col xs={24} md={6}><Form.Item name={[field.name, "tradeInstrument"]} label="交易标识"><Input placeholder="BTCUSDT" /></Form.Item></Col>
                <Col xs={12} md={3}><Form.Item name={[field.name, "spreadThreshold"]} label="价差阈值"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
                <Col xs={12} md={3}><Form.Item name={[field.name, "signalThreshold"]} label="信号阈值"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
              </Row>
            </div>
          ))}
        </Section>
      )}
    </Form.List>
  );
}

function NotificationConfig() {
  return (
    <Section title="Telegram 通知" desc="用于推送成交、风控与异常告警">
      <Hint>Bot Token 与 Chat ID 属于敏感信息，保存后不再回显；如需更换请直接输入新值。</Hint>
      <Row gutter={16}>
        <Col xs={12} md={6}>
          <Form.Item
            name={["notification", "telegramEnabled"]}
            label="Telegram 通知"
            valuePropName="checked"
            getValueProps={(value: number) => ({ checked: value === 1 })}
            normalize={(value: boolean) => (value ? 1 : 0)}
          >
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
        </Col>
        <Col xs={24} md={9}><SecretField name={["notification", "telegramBotToken"]} label="Bot Token" /></Col>
        <Col xs={24} md={9}><SecretField name={["notification", "telegramChatId"]} label="Chat ID" /></Col>
      </Row>
    </Section>
  );
}

function Sessions() {
  return (
    <Form.List name="sessions">
      {(fields, { add, remove }) => (
        <Section
          title="会话状态"
          desc="由 Argus 自动回写"
          extra={<Button size="small" icon={<PlusOutlined />} onClick={() => add({ valid: 1 })}>新增会话</Button>}
        >
          <Hint>正常情况下无需手工维护；仅在迁移环境或会话失效需要人工覆盖时填写 Cookie / Token。</Hint>
          {fields.length === 0 ? (
            <div className="manager-argus-empty">
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无会话记录" />
            </div>
          ) : null}
          {fields.map((field, index) => (
            <div className="manager-argus-item" key={field.key}>
              <div className="manager-argus-item__head">
                <span className="manager-argus-item__index">{index + 1}</span>
                <Form.Item shouldUpdate noStyle>
                  {({ getFieldValue }) => (
                    <span className="manager-argus-item__title" style={{ color: "var(--manager-text)", fontWeight: 600 }}>
                      账户 #{getFieldValue(["sessions", field.name, "accountId"]) ?? "—"}
                      {getFieldValue(["sessions", field.name, "valid"]) === 1 ? (
                        <Tag color="green" style={{ marginLeft: 8 }}>有效</Tag>
                      ) : (
                        <Tag style={{ marginLeft: 8 }}>失效</Tag>
                      )}
                    </span>
                  )}
                </Form.Item>
                <Popconfirm title="删除该会话记录？" okText="删除" okButtonProps={{ danger: true }} cancelText="取消" onConfirm={() => remove(field.name)}>
                  <Button type="text" danger icon={<DeleteOutlined />} />
                </Popconfirm>
              </div>
              <Row gutter={12}>
                <Col xs={24} md={6}><Form.Item name={[field.name, "accountId"]} label="账户 ID"><InputNumber min={0} style={{ width: "100%" }} /></Form.Item></Col>
                <Col xs={24} md={9}><Form.Item name={[field.name, "loginUrl"]} label="登录回跳地址"><Input /></Form.Item></Col>
                <Col xs={24} md={9}><Form.Item name={[field.name, "finalUrl"]} label="完成地址"><Input /></Form.Item></Col>
                <Col xs={24} md={8}><SecretField name={[field.name, "cookie"]} label="Cookie" /></Col>
                <Col xs={24} md={8}><SecretField name={[field.name, "token"]} label="Token" /></Col>
                <Col xs={24} md={8}><SecretField name={[field.name, "otoken"]} label="OToken" /></Col>
              </Row>
            </div>
          ))}
        </Section>
      )}
    </Form.List>
  );
}
