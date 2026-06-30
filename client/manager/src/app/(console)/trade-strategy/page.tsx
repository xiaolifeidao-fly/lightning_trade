"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Button,
  Card,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from "antd";
import { EditOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import {
  createStrategy,
  deleteStrategy,
  fetchStrategies,
  updateStrategy,
  type Strategy,
  type StrategyPayload,
} from "./api/strategy.api";

const { Text, Title } = Typography;

const INTERVALS = ["5m", "15m", "1h", "4h"].map((v) => ({ label: v, value: v }));
const ENTRY_MODES = [
  { label: "市价 (开盘价)", value: "market" },
  { label: "回踩限价 (pullback)", value: "pullback" },
];
const TREND_FILTERS = [
  { label: "双向", value: "both" },
  { label: "仅做多", value: "long" },
  { label: "仅做空", value: "short" },
];
const VARIANTS = [
  { label: "原始预测", value: "raw" },
  { label: "校准后", value: "calibrated" },
];
const EXIT_SOURCES = [
  { label: "百分比 (离入场价)", value: "percent" },
  { label: "AI 预测", value: "predict" },
  { label: "AI 压力面", value: "pressure" },
];

function entryModeTag(mode: string) {
  return mode === "pullback" ? <Tag color="purple">回踩限价</Tag> : <Tag color="blue">市价</Tag>;
}

// exitSourceTag 把止盈/止损来源 + 对应百分比渲染成一个标签。
function exitSourceTag(kind: "止盈" | "止损", source: string, r: Strategy) {
  if (source === "percent") {
    const pct = kind === "止盈" ? r.takeProfitPct : r.stopLossPct;
    return <Tag color="blue">{kind}比例{pct > 0 ? ` ${pct}%` : ""}</Tag>;
  }
  if (source === "pressure") {
    return (
      <Tag color={kind === "止盈" ? "cyan" : "volcano"}>
        {kind}压力面{r.pressureBufferPct > 0 ? ` ±${r.pressureBufferPct}%` : ""}
      </Tag>
    );
  }
  // predict
  const extra = kind === "止盈" ? `γ${r.exitGamma}` : r.predictSlBufferPct > 0 ? `+${r.predictSlBufferPct}%` : "";
  return <Tag color="gold">{kind}AI预测{extra ? ` ${extra}` : ""}</Tag>;
}

export default function TradeStrategyPage() {
  const [form] = Form.useForm<StrategyPayload>();
  const [rows, setRows] = useState<Strategy[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Strategy | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchStrategies({ page: 1, pageSize: 200 });
      setRows(res.list ?? []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "策略加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({
      platformCode: "binance",
      coinCode: "BTC",
      symbol: "BTCUSDT",
      interval: "15m",
      enabled: 1,
      minConfidence: 0.6,
      minMovePct: 0.5,
      trendFilter: "both",
      maxOpenPositions: 1,
      entryMode: "market",
      entryAlpha: 0.15,
      exitGamma: 0.6,
      entryTtl: 1800,
      efficiencyRoute: 0,
      predictionVariant: "raw",
      holdDuration: 14400,
      maxHoldDuration: 86400,
      takeProfitPct: 0,
      stopLossPct: 0,
      takeProfitSource: "predict",
      stopLossSource: "pressure",
      predictSlBufferPct: 0,
      pressureBufferPct: 0,
      leverage: 10,
      contracts: 1,
      makerFeeRate: 0.0002,
      takerFeeRate: 0.0005,
    } as unknown as StrategyPayload);
    setModalOpen(true);
  };

  const openEdit = (row: Strategy) => {
    setEditing(row);
    form.resetFields();
    form.setFieldsValue({
      ...row,
      holdDuration: row.holdDuration as unknown as string,
      maxHoldDuration: row.maxHoldDuration as unknown as string,
    });
    setModalOpen(true);
  };

  const onSubmit = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      // holdDuration/maxHoldDuration 后端按字符串(秒或"4h")解析，统一转成字符串。
      const payload: StrategyPayload = {
        ...values,
        holdDuration: values.holdDuration != null ? String(values.holdDuration) : undefined,
        maxHoldDuration: values.maxHoldDuration != null ? String(values.maxHoldDuration) : undefined,
      };
      if (editing) {
        await updateStrategy(editing.id, payload);
        message.success("策略已更新");
      } else {
        await createStrategy(payload);
        message.success("策略已创建");
      }
      setModalOpen(false);
      void load();
    } catch (err) {
      if (err instanceof Error) message.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const onDelete = async (id: number) => {
    try {
      await deleteStrategy(id);
      message.success("已删除");
      void load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "删除失败");
    }
  };

  const columns: ColumnsType<Strategy> = [
    { title: "ID", dataIndex: "id", width: 60 },
    {
      title: "交易对/周期",
      key: "dim",
      width: 130,
      render: (_: unknown, r: Strategy) => `${r.symbol} / ${r.interval}`,
    },
    { title: "入场", dataIndex: "entryMode", width: 100, render: (m: string) => entryModeTag(m) },
    {
      title: "α / γ",
      key: "ag",
      width: 100,
      render: (_: unknown, r: Strategy) => `${r.entryAlpha} / ${r.exitGamma}`,
    },
    {
      title: "效率路由",
      dataIndex: "efficiencyRoute",
      width: 90,
      render: (v: number) => (v > 0 ? v : <Text type="secondary">关</Text>),
    },
    {
      title: "止盈/止损来源",
      key: "exitSource",
      width: 220,
      render: (_: unknown, r: Strategy) => (
        <Space size={4} wrap>
          {exitSourceTag("止盈", r.takeProfitSource, r)}
          {exitSourceTag("止损", r.stopLossSource, r)}
        </Space>
      ),
    },
    { title: "置信门槛", dataIndex: "minConfidence", width: 90 },
    { title: "方向", dataIndex: "trendFilter", width: 80 },
    { title: "杠杆", dataIndex: "leverage", width: 70 },
    {
      title: "预测",
      dataIndex: "predictionVariant",
      width: 90,
      render: (v: string) => (v === "calibrated" ? <Tag color="gold">校准</Tag> : <Tag>原始</Tag>),
    },
    {
      title: "启用",
      dataIndex: "enabled",
      width: 70,
      render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
    },
    {
      title: "操作",
      key: "action",
      width: 120,
      fixed: "right",
      render: (_: unknown, r: Strategy) => (
        <Space>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Popconfirm title="确认删除该策略?" onConfirm={() => onDelete(r.id)}>
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <Text className="manager-section-label">STRATEGY MANAGEMENT</Text>
        <Title level={3} style={{ margin: "4px 0 0" }}>
          策略管理
        </Title>
        <Text type="secondary">配置策略的入场方式(市价/回踩)、α/γ 分位、效率路由与风控参数，供实盘与回测共用。</Text>
      </div>

      <Card
        size="small"
        extra={
          <Space>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              新建策略
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      >
        <Table<Strategy>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={rows}
          columns={columns}
          scroll={{ x: 1100 }}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
        />
      </Card>

      <Modal
        title={editing ? `编辑策略 #${editing.id}` : "新建策略"}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={onSubmit}
        confirmLoading={saving}
        width={720}
        okText="保存"
        cancelText="取消"
      >
        <Form form={form} layout="vertical" style={{ marginTop: 8 }}>
          <Divider orientation="left" style={{ margin: "0 0 12px" }}>基础</Divider>
          <Space wrap size={12}>
            <Form.Item label="平台" name="platformCode" rules={[{ required: true }]}>
              <Input style={{ width: 130 }} disabled={!!editing} />
            </Form.Item>
            <Form.Item label="币种" name="coinCode" rules={[{ required: true }]}>
              <Input style={{ width: 100 }} disabled={!!editing} />
            </Form.Item>
            <Form.Item label="交易对" name="symbol" rules={[{ required: true }]}>
              <Input style={{ width: 130 }} disabled={!!editing} />
            </Form.Item>
            <Form.Item label="预测周期" name="interval" rules={[{ required: true }]}>
              <Select style={{ width: 100 }} options={INTERVALS} disabled={!!editing} />
            </Form.Item>
            <Form.Item label="启用" name="enabled" valuePropName="checked" getValueFromEvent={(c) => (c ? 1 : 0)}>
              <Switch defaultChecked />
            </Form.Item>
          </Space>

          <Divider orientation="left" style={{ margin: "4px 0 12px" }}>入场策略</Divider>
          <Space wrap size={12}>
            <Form.Item label="入场方式" name="entryMode">
              <Select style={{ width: 170 }} options={ENTRY_MODES} />
            </Form.Item>
            <Form.Item label="入场分位 α" name="entryAlpha" tooltip="限价离区间下沿(多)/上沿(空)的比例，越小越靠近沿">
              <InputNumber style={{ width: 110 }} min={0} max={1} step={0.05} />
            </Form.Item>
            <Form.Item label="挂单有效期(秒)" name="entryTtl" tooltip="回踩限价多久没成交就放弃">
              <InputNumber style={{ width: 130 }} min={1} />
            </Form.Item>
            <Form.Item label="效率路由阈值" name="efficiencyRoute" tooltip="0=不路由；>0 时低于阈值走回踩、高于走市价">
              <InputNumber style={{ width: 130 }} min={0} max={1} step={0.05} />
            </Form.Item>
            <Form.Item label="预测变体" name="predictionVariant">
              <Select style={{ width: 130 }} options={VARIANTS} />
            </Form.Item>
          </Space>

          <Divider orientation="left" style={{ margin: "4px 0 12px" }}>开仓门槛</Divider>
          <Space wrap size={12}>
            <Form.Item label="最低置信度" name="minConfidence">
              <InputNumber style={{ width: 110 }} min={0} max={1} step={0.05} />
            </Form.Item>
            <Form.Item label="最低幅度%" name="minMovePct">
              <InputNumber style={{ width: 110 }} min={0} step={0.1} />
            </Form.Item>
            <Form.Item label="方向过滤" name="trendFilter">
              <Select style={{ width: 110 }} options={TREND_FILTERS} />
            </Form.Item>
            <Form.Item label="最大持仓数" name="maxOpenPositions">
              <InputNumber style={{ width: 110 }} min={1} />
            </Form.Item>
          </Space>

          <Divider orientation="left" style={{ margin: "4px 0 12px" }}>出场与仓位</Divider>
          <Space wrap size={12}>
            <Form.Item label="止盈来源" name="takeProfitSource" tooltip="percent=离入场价固定%；predict=AI预测区间分位γ；pressure=到对侧关键压力位(多→关键阻力、空→关键支撑)。注：回测选了交易周期时，「按交易周期」结算口径下 predict/pressure 会改用交易周期那条预测的区间，使止盈止损贴合持仓周期（入场仍按预测周期信号）">
              <Select options={EXIT_SOURCES} style={{ width: 150 }} />
            </Form.Item>
            {/* 止盈来源对应的参数 */}
            <Form.Item noStyle shouldUpdate={(p, c) => p.takeProfitSource !== c.takeProfitSource}>
              {({ getFieldValue }) => {
                const src = getFieldValue("takeProfitSource");
                if (src === "percent")
                  return (
                    <Form.Item label="止盈%(离入场价)" name="takeProfitPct" tooltip="止盈价=入场价×(1±该%)">
                      <InputNumber style={{ width: 140 }} min={0} step={0.1} />
                    </Form.Item>
                  );
                if (src === "predict")
                  return (
                    <Form.Item label="止盈分位 γ" name="exitGamma" tooltip="止盈价离预测区间对沿的比例(0~1)，越小越早止盈">
                      <InputNumber style={{ width: 140 }} min={0} max={1} step={0.05} />
                    </Form.Item>
                  );
                return null; // pressure 用下方共用的「压力面缓冲%」
              }}
            </Form.Item>
            <Form.Item label="止损来源" name="stopLossSource" tooltip="percent=离入场价固定%；predict=跟AI失效价(可带缓冲%)；pressure=突破关键压力位缓冲%后止损(空→关键阻力上方、多→关键支撑下方)">
              <Select options={EXIT_SOURCES} style={{ width: 150 }} />
            </Form.Item>
            {/* 止损来源对应的参数 */}
            <Form.Item noStyle shouldUpdate={(p, c) => p.stopLossSource !== c.stopLossSource}>
              {({ getFieldValue }) => {
                const src = getFieldValue("stopLossSource");
                if (src === "percent")
                  return (
                    <Form.Item label="止损%(离入场价)" name="stopLossPct" tooltip="止损价=入场价×(1∓该%)">
                      <InputNumber style={{ width: 140 }} min={0} step={0.1} />
                    </Form.Item>
                  );
                if (src === "predict")
                  return (
                    <Form.Item label="失效价缓冲%" name="predictSlBufferPct" tooltip="价格突破AI失效价该%后止损，0=贴着失效价">
                      <InputNumber style={{ width: 140 }} min={0} step={0.1} />
                    </Form.Item>
                  );
                return null; // pressure 用下方共用的「压力面缓冲%」
              }}
            </Form.Item>
            {/* 压力面缓冲%：止盈或止损任一用压力面就显示一次(两者共用) */}
            <Form.Item
              noStyle
              shouldUpdate={(p, c) =>
                p.takeProfitSource !== c.takeProfitSource || p.stopLossSource !== c.stopLossSource
              }
            >
              {({ getFieldValue }) =>
                getFieldValue("takeProfitSource") === "pressure" ||
                getFieldValue("stopLossSource") === "pressure" ? (
                  <Form.Item label="压力面缓冲%" name="pressureBufferPct" tooltip="离关键结构位的缓冲%：止损突破该%后触发、止盈提前该%了结，0=贴着结构位">
                    <InputNumber style={{ width: 140 }} min={0} step={0.1} />
                  </Form.Item>
                ) : null
              }
            </Form.Item>
            <Form.Item label="持仓时长(秒)" name="holdDuration">
              <InputNumber style={{ width: 130 }} min={1} />
            </Form.Item>
            <Form.Item label="最长持仓(秒)" name="maxHoldDuration">
              <InputNumber style={{ width: 130 }} min={1} />
            </Form.Item>
            <Form.Item label="杠杆" name="leverage">
              <InputNumber style={{ width: 90 }} min={1} max={125} />
            </Form.Item>
            <Form.Item label="张数" name="contracts">
              <InputNumber style={{ width: 90 }} min={1} />
            </Form.Item>
          </Space>

          <Form.Item label="备注" name="remark">
            <Input.TextArea rows={2} maxLength={255} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
