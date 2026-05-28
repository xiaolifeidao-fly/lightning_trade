"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from "antd";
import { EditOutlined, PlusOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import {
  fetchUserAssets,
  fetchUserPositions,
  fetchUserPositionAnalysis,
  fetchLoginRecords,
  upsertUserAsset,
  updateUserAsset,
  upsertUserPosition,
  updateUserPosition,
  createUserPositionAnalysis,
  updateUserPositionAnalysis,
  type CoinUserRecord,
  type CoinUserAssetRecord,
  type CoinUserPositionRecord,
  type CoinUserPositionAnalysisRecord,
  type CoinUserLoginRecord,
  type CoinUserAssetPayload,
  type CoinUserPositionPayload,
  type CoinUserPositionAnalysisPayload,
} from "../api/coin-user.api";

const { Text } = Typography;

interface CoinUserDetailDrawerProps {
  user: CoinUserRecord | null;
  open: boolean;
  onClose: () => void;
}

type ModalKind = "asset" | "position" | "analysis";
type EditingRecord = CoinUserAssetRecord | CoinUserPositionRecord | CoinUserPositionAnalysisRecord | null;

function kycTag(value: string) {
  const color = value === "approved" ? "green" : value === "rejected" ? "red" : "orange";
  const label = value === "approved" ? "已认证" : value === "rejected" ? "已拒绝" : "待审核";
  return <Tag color={color}>{label}</Tag>;
}

function userStatusTag(value: string) {
  const color = value === "active" ? "green" : value === "locked" ? "orange" : "red";
  const label = value === "active" ? "正常" : value === "locked" ? "锁定" : "冻结";
  return <Tag color={color}>{label}</Tag>;
}

function sideTag(value: unknown) {
  const v = String(value ?? "").toLowerCase();
  return <Tag color={v === "buy" || v === "long" ? "green" : "red"}>{String(value || "-")}</Tag>;
}

function positionStatusTag(value: unknown) {
  return String(value) === "open" ? <Tag color="green">持仓中</Tag> : <Tag color="default">已平仓</Tag>;
}

export function CoinUserDetailDrawer({ user, open, onClose }: CoinUserDetailDrawerProps) {
  const [form] = Form.useForm();
  const [assets, setAssets] = useState<CoinUserAssetRecord[]>([]);
  const [positions, setPositions] = useState<CoinUserPositionRecord[]>([]);
  const [analysis, setAnalysis] = useState<CoinUserPositionAnalysisRecord[]>([]);
  const [loginRecords, setLoginRecords] = useState<CoinUserLoginRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [modalKind, setModalKind] = useState<ModalKind | null>(null);
  const [editingRecord, setEditingRecord] = useState<EditingRecord>(null);

  const loadData = useCallback(async () => {
    if (!user) return;
    setLoading(true);
    try {
      const [assetRows, positionRows, analysisRows, loginRows] = await Promise.all([
        fetchUserAssets(user.id),
        fetchUserPositions(user.id),
        fetchUserPositionAnalysis(user.id),
        fetchLoginRecords(user.id, 30),
      ]);
      setAssets(Array.isArray(assetRows) ? assetRows : []);
      setPositions(Array.isArray(positionRows) ? positionRows : []);
      setAnalysis(Array.isArray(analysisRows) ? analysisRows : []);
      setLoginRecords(Array.isArray(loginRows) ? loginRows : []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载用户详情失败");
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => {
    if (open) void loadData();
  }, [loadData, open]);

  const openModal = (kind: ModalKind, record: EditingRecord = null) => {
    setModalKind(kind);
    setEditingRecord(record);
    form.resetFields();
    form.setFieldsValue({ userId: user?.id, ...record });
  };

  const closeModal = () => {
    setModalKind(null);
    setEditingRecord(null);
    form.resetFields();
  };

  const handleSave = async () => {
    if (!user || !modalKind) return;
    const values = await form.validateFields();
    setSaving(true);
    try {
      if (modalKind === "asset") {
        const payload = values as CoinUserAssetPayload;
        if (editingRecord) {
          await updateUserAsset(editingRecord.id, payload);
        } else {
          await upsertUserAsset(payload);
        }
      }
      if (modalKind === "position") {
        const payload = values as CoinUserPositionPayload;
        if (editingRecord) {
          await updateUserPosition(editingRecord.id, payload);
        } else {
          await upsertUserPosition(payload);
        }
      }
      if (modalKind === "analysis") {
        const payload = values as CoinUserPositionAnalysisPayload;
        if (editingRecord) {
          await updateUserPositionAnalysis(editingRecord.id, payload);
        } else {
          await createUserPositionAnalysis(payload);
        }
      }
      message.success("保存成功");
      closeModal();
      await loadData();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  };

  const assetColumns = useMemo<ColumnsType<CoinUserAssetRecord>>(
    () => [
      { title: "币种", dataIndex: "coinCode", width: 90 },
      { title: "可用", dataIndex: "available", width: 120, render: (v) => Number(v).toFixed(6) },
      { title: "冻结", dataIndex: "frozen", width: 120, render: (v) => Number(v).toFixed(6) },
      { title: "总额", dataIndex: "total", width: 120, render: (v) => Number(v).toFixed(6) },
      { title: "充值地址", dataIndex: "address", ellipsis: true },
      {
        title: "提现",
        dataIndex: "withdrawEnable",
        width: 80,
        render: (v) => (Number(v) === 1 ? <Tag color="green">允许</Tag> : <Tag color="red">禁止</Tag>),
      },
      {
        title: "操作",
        width: 80,
        render: (_, record) => (
          <Button type="text" icon={<EditOutlined />} onClick={() => openModal("asset", record)} />
        ),
      },
    ],
    [],
  );

  const positionColumns = useMemo<ColumnsType<CoinUserPositionRecord>>(
    () => [
      { title: "交易对", dataIndex: "symbol", width: 120 },
      { title: "持仓量", dataIndex: "amount", width: 120, render: (v) => Number(v).toFixed(6) },
      { title: "均价", dataIndex: "avgCostPrice", width: 120, render: (v) => Number(v).toFixed(4) },
      { title: "总成本", dataIndex: "totalCost", width: 120, render: (v) => Number(v).toFixed(4) },
      { title: "状态", dataIndex: "status", width: 90, render: (v) => positionStatusTag(v) },
      { title: "更新时间", dataIndex: "updatedTime", width: 160, render: (v) => (v ? String(v).slice(0, 19) : "-") },
      {
        title: "操作",
        width: 80,
        render: (_, record) => (
          <Button type="text" icon={<EditOutlined />} onClick={() => openModal("position", record)} />
        ),
      },
    ],
    [],
  );

  const analysisColumns = useMemo<ColumnsType<CoinUserPositionAnalysisRecord>>(
    () => [
      { title: "交易对", dataIndex: "symbol", width: 110 },
      { title: "方向", dataIndex: "side", width: 80, render: (v) => sideTag(v) },
      { title: "均价", dataIndex: "avgPrice", width: 110, render: (v) => Number(v).toFixed(4) },
      { title: "爆仓价", dataIndex: "liquidationPrice", width: 110, render: (v) => Number(v).toFixed(4) },
      { title: "杠杆", dataIndex: "leverage", width: 80, render: (v) => `${Number(v)}x` },
      { title: "保证金", dataIndex: "margin", width: 110, render: (v) => Number(v).toFixed(4) },
      {
        title: "AI建议",
        dataIndex: "aiAdvice",
        ellipsis: true,
        render: (v) => (v ? <Text ellipsis={{ tooltip: String(v) }}>{String(v)}</Text> : "-"),
      },
      {
        title: "操作",
        width: 80,
        render: (_, record) => (
          <Button type="text" icon={<EditOutlined />} onClick={() => openModal("analysis", record)} />
        ),
      },
    ],
    [],
  );

  const loginColumns: ColumnsType<CoinUserLoginRecord> = [
    { title: "IP", dataIndex: "ip", width: 130 },
    { title: "设备", dataIndex: "device", ellipsis: true },
    { title: "位置", dataIndex: "location", width: 140 },
    {
      title: "结果",
      dataIndex: "success",
      width: 80,
      render: (v) => (Number(v) === 1 ? <Tag color="green">成功</Tag> : <Tag color="red">失败</Tag>),
    },
    { title: "时间", dataIndex: "createdTime", width: 160, render: (v) => (v ? String(v).slice(0, 19) : "-") },
  ];

  const modalTitle = modalKind === "asset" ? "资产管控" : modalKind === "position" ? "仓位管控" : "合约分析";

  if (!user) return null;

  return (
    <Drawer
      title={
        <Space>
          <span>用户详情</span>
          <Text style={{ color: "var(--manager-text-soft)", fontWeight: 400, fontSize: 13 }}>{user.username}</Text>
        </Space>
      }
      open={open}
      onClose={onClose}
      width={960}
      styles={{ body: { padding: "16px 24px" } }}
    >
      <Spin spinning={loading}>
        <Descriptions
          bordered
          size="small"
          column={3}
          style={{ marginBottom: 20 }}
          items={[
            { label: "用户名", children: user.username },
            { label: "平台", children: user.platformCode || "-" },
            { label: "邮箱", children: user.email || "-" },
            { label: "手机", children: user.phone || "-" },
            { label: "余额(USDT)", children: <strong style={{ color: "var(--manager-primary)" }}>{Number(user.balance).toFixed(4)}</strong> },
            { label: "KYC状态", children: kycTag(user.kycStatus) },
            { label: "账户状态", children: userStatusTag(user.status) },
            { label: "最后登录IP", children: user.lastLoginIp || "-" },
            { label: "2FA", children: Number(user.twoFaEnabled) === 1 ? <Tag color="green">已开启</Tag> : <Tag>未开启</Tag> },
          ]}
        />

        <Tabs
          items={[
            {
              key: "assets",
              label: `资产 (${assets.length})`,
              children: (
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal("asset")}>新增资产</Button>
                  <Table rowKey="id" dataSource={assets} columns={assetColumns} pagination={false} size="small" scroll={{ x: 820 }} />
                </Space>
              ),
            },
            {
              key: "positions",
              label: `仓位 (${positions.length})`,
              children: (
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal("position")}>新增仓位</Button>
                  <Table rowKey="id" dataSource={positions} columns={positionColumns} pagination={false} size="small" scroll={{ x: 900 }} />
                </Space>
              ),
            },
            {
              key: "analysis",
              label: `持仓分析 (${analysis.length})`,
              children: (
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => openModal("analysis")}>新增分析</Button>
                  <Table rowKey="id" dataSource={analysis} columns={analysisColumns} pagination={false} size="small" scroll={{ x: 1000 }} />
                </Space>
              ),
            },
            {
              key: "login",
              label: `登录记录 (${loginRecords.length})`,
              children: <Table rowKey="id" dataSource={loginRecords} columns={loginColumns} pagination={false} size="small" scroll={{ x: 700 }} />,
            },
          ]}
        />
      </Spin>

      <Modal
        title={editingRecord ? `编辑${modalTitle}` : `新增${modalTitle}`}
        open={Boolean(modalKind)}
        okText="保存"
        cancelText="取消"
        confirmLoading={saving}
        onCancel={closeModal}
        onOk={() => void handleSave()}
        destroyOnClose
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="userId" label="用户ID" rules={[{ required: true, message: "请输入用户ID" }]}>
            <InputNumber min={1} precision={0} style={{ width: "100%" }} disabled />
          </Form.Item>
          {modalKind === "asset" ? (
            <>
              <Form.Item name="coinId" label="币种ID" rules={[{ required: !editingRecord, message: "请输入币种ID" }]}>
                <InputNumber min={1} precision={0} style={{ width: "100%" }} disabled={Boolean(editingRecord)} />
              </Form.Item>
              <Form.Item name="coinCode" label="币种代码" rules={[{ required: !editingRecord, message: "请输入币种代码" }]}>
                <Input disabled={Boolean(editingRecord)} />
              </Form.Item>
              <Form.Item name="available" label="可用余额"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="frozen" label="冻结余额"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="total" label="总余额"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="address" label="充值地址"><Input /></Form.Item>
              <Form.Item name="withdrawEnable" label="允许提现"><Select options={[{ label: "允许", value: 1 }, { label: "禁止", value: 0 }]} /></Form.Item>
            </>
          ) : null}
          {modalKind === "position" ? (
            <>
              <Form.Item name="symbol" label="交易对" rules={[{ required: !editingRecord, message: "请输入交易对" }]}><Input disabled={Boolean(editingRecord)} /></Form.Item>
              <Form.Item name="baseCoinCode" label="基础币"><Input disabled={Boolean(editingRecord)} /></Form.Item>
              <Form.Item name="quoteCoinCode" label="计价币"><Input disabled={Boolean(editingRecord)} /></Form.Item>
              <Form.Item name="amount" label="持仓量" rules={[{ required: true, message: "请输入持仓量" }]}><InputNumber precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="avgCostPrice" label="平均成本"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="totalCost" label="总成本"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="status" label="状态"><Select options={[{ label: "持仓中", value: "open" }, { label: "已平仓", value: "closed" }]} /></Form.Item>
            </>
          ) : null}
          {modalKind === "analysis" ? (
            <>
              <Form.Item name="positionId" label="关联仓位ID"><InputNumber min={0} precision={0} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="symbol" label="交易对" rules={[{ required: !editingRecord, message: "请输入交易对" }]}><Input disabled={Boolean(editingRecord)} /></Form.Item>
              <Form.Item name="side" label="方向"><Select options={[{ label: "多", value: "long" }, { label: "空", value: "short" }]} disabled={Boolean(editingRecord)} /></Form.Item>
              <Form.Item name="avgPrice" label="开仓均价"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="liquidationPrice" label="爆仓价"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="leverage" label="杠杆"><InputNumber min={1} precision={2} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="contracts" label="合约数"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="margin" label="保证金"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="balanceAtOpen" label="开仓余额"><InputNumber min={0} precision={8} style={{ width: "100%" }} /></Form.Item>
              <Form.Item name="aiAdvice" label="分析建议"><Input.TextArea rows={4} /></Form.Item>
            </>
          ) : null}
        </Form>
      </Modal>
    </Drawer>
  );
}
