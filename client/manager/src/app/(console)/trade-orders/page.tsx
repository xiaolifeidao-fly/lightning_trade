"use client";

import { useEffect, useState } from "react";
import {
  Button,
  Input,
  InputNumber,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from "antd";
import { ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import {
  fetchTradeOrders,
  fetchTradeDetails,
  fetchUserSummary,
  fetchUserPnl,
  type TradeOrderRecord,
  type TradeDetailRecord,
  type TradeUserSummaryRecord,
  type TradeUserPnlRecord,
} from "./api/trade.api";

const { Text } = Typography;

const PAGE_SIZE = 15;

function sideTag(value: unknown) {
  const v = String(value ?? "").toLowerCase();
  return <Tag color={v === "buy" ? "green" : "red"}>{String(value ?? "-").toUpperCase()}</Tag>;
}

function orderStatusTag(value: unknown) {
  const v = String(value ?? "").toLowerCase();
  const colorMap: Record<string, string> = {
    pending: "blue",
    partial_filled: "cyan",
    filled: "green",
    canceled: "default",
    rejected: "red",
  };
  return <Tag color={colorMap[v] ?? "default"}>{String(value ?? "-")}</Tag>;
}

function pnlCell(value: unknown) {
  const n = Number(value);
  if (n > 0) return <span style={{ color: "#0ecb81", fontWeight: 600 }}>+{n.toFixed(4)}</span>;
  if (n < 0) return <span style={{ color: "#f6465d", fontWeight: 600 }}>{n.toFixed(4)}</span>;
  return <span>{n.toFixed(4)}</span>;
}

function pnlRateCell(value: unknown) {
  const n = Number(value);
  const formatted = `${(n * 100).toFixed(2)}%`;
  if (n > 0) return <span style={{ color: "#0ecb81", fontWeight: 600 }}>+{formatted}</span>;
  if (n < 0) return <span style={{ color: "#f6465d", fontWeight: 600 }}>{formatted}</span>;
  return <span>{formatted}</span>;
}

// --- Orders Tab ---
function OrdersTab() {
  const [records, setRecords] = useState<TradeOrderRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({
    userId: undefined as number | undefined,
    symbol: "",
    side: "",
    status: "",
    tradeCategory: "",
  });

  const load = async (nextPage = page, f = filters) => {
    setLoading(true);
    try {
      const result = await fetchTradeOrders({
        pageIndex: nextPage,
        pageSize: PAGE_SIZE,
        userId: f.userId,
        symbol: f.symbol || undefined,
        side: f.side || undefined,
        status: f.status || undefined,
        tradeCategory: f.tradeCategory || undefined,
      });
      setRecords(result.data);
      setTotal(result.total);
      setPage(nextPage);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(1); }, []);

  const columns: ColumnsType<TradeOrderRecord> = [
    { title: "ID", dataIndex: "id", width: 70, fixed: "left" },
    {
      title: "订单号",
      dataIndex: "orderNo",
      width: 180,
      render: (v) => <Text copyable style={{ color: "var(--manager-text)", fontSize: 12 }}>{String(v)}</Text>,
    },
    { title: "用户ID", dataIndex: "userId", width: 90 },
    { title: "平台", dataIndex: "platformCode", width: 100 },
    { title: "类型", dataIndex: "tradeCategory", width: 90 },
    { title: "交易对", dataIndex: "symbol", width: 120 },
    { title: "方向", dataIndex: "side", width: 80, render: (v) => sideTag(v) },
    { title: "订单类型", dataIndex: "orderType", width: 90 },
    { title: "价格", dataIndex: "price", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "数量", dataIndex: "amount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "成交量", dataIndex: "filledAmount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "状态", dataIndex: "status", width: 110, render: (v) => orderStatusTag(v) },
    { title: "来源", dataIndex: "source", width: 90 },
    {
      title: "提交时间",
      dataIndex: "submittedTime",
      width: 160,
      render: (v) => (v ? String(v).slice(0, 19) : "-"),
    },
  ];

  return (
    <div className="manager-page-stack">
      <section className="manager-data-card">
        <Space wrap size={12}>
          <InputNumber
            placeholder="用户ID"
            min={1}
            precision={0}
            value={filters.userId}
            onChange={(v) => setFilters((f) => ({ ...f, userId: v ?? undefined }))}
            style={{ width: 120 }}
          />
          <Input
            placeholder="交易对"
            value={filters.symbol}
            onChange={(e) => setFilters((f) => ({ ...f, symbol: e.target.value }))}
            style={{ width: 140 }}
          />
          <Select
            allowClear
            placeholder="方向"
            value={filters.side || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, side: v ?? "" }))}
            options={[{ label: "买入", value: "buy" }, { label: "卖出", value: "sell" }]}
            style={{ width: 100 }}
          />
          <Select
            allowClear
            placeholder="状态"
            value={filters.status || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, status: v ?? "" }))}
            options={[
              { label: "待成交", value: "pending" },
              { label: "部分成交", value: "partial_filled" },
              { label: "已成交", value: "filled" },
              { label: "已取消", value: "canceled" },
              { label: "已拒绝", value: "rejected" },
            ]}
            style={{ width: 130 }}
          />
          <Select
            allowClear
            placeholder="交易类型"
            value={filters.tradeCategory || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, tradeCategory: v ?? "" }))}
            options={[
              { label: "现货", value: "spot" },
              { label: "合约", value: "futures" },
              { label: "杠杆", value: "margin" },
            ]}
            style={{ width: 120 }}
          />
          <Button
            type="primary"
            icon={<SearchOutlined />}
            onClick={() => void load(1)}
          >
            查询
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load(1)}>刷新</Button>
          <Tag style={{ color: "var(--manager-primary)", background: "var(--manager-gold-soft)", border: "1px solid rgba(240,185,11,0.28)" }}>
            共 {total} 条
          </Tag>
        </Space>
      </section>
      <section className="manager-data-card manager-table">
        <Table
          rowKey="id"
          loading={loading}
          dataSource={records}
          columns={columns}
          scroll={{ x: 1500 }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showSizeChanger: false,
            onChange: (p) => void load(p),
          }}
          size="small"
        />
      </section>
    </div>
  );
}

// --- Details Tab ---
function DetailsTab() {
  const [records, setRecords] = useState<TradeDetailRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ userId: undefined as number | undefined, symbol: "", orderNo: "" });

  const load = async (nextPage = page, f = filters) => {
    setLoading(true);
    try {
      const result = await fetchTradeDetails({
        pageIndex: nextPage,
        pageSize: PAGE_SIZE,
        userId: f.userId,
        symbol: f.symbol || undefined,
        orderNo: f.orderNo || undefined,
      });
      setRecords(result.data);
      setTotal(result.total);
      setPage(nextPage);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(1); }, []);

  const columns: ColumnsType<TradeDetailRecord> = [
    { title: "ID", dataIndex: "id", width: 70, fixed: "left" },
    {
      title: "成交号",
      dataIndex: "tradeNo",
      width: 160,
      render: (v) => <Text copyable style={{ color: "var(--manager-text)", fontSize: 12 }}>{String(v || "-")}</Text>,
    },
    { title: "用户ID", dataIndex: "userId", width: 90 },
    { title: "交易对", dataIndex: "symbol", width: 120 },
    { title: "方向", dataIndex: "side", width: 80, render: (v) => sideTag(v) },
    { title: "开仓方向", dataIndex: "openDirection", width: 90 },
    { title: "价格", dataIndex: "price", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "数量", dataIndex: "amount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "盈亏", dataIndex: "pnl", width: 110, render: (v) => pnlCell(v) },
    { title: "盈亏率", dataIndex: "pnlRate", width: 100, render: (v) => pnlRateCell(v) },
    { title: "手续费", dataIndex: "fee", width: 100, render: (v) => Number(v).toFixed(6) },
    { title: "杠杆", dataIndex: "leverage", width: 80, render: (v) => `${Number(v)}x` },
    { title: "保证金", dataIndex: "margin", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "成交时间", dataIndex: "tradeTime", width: 160, render: (v) => (v ? String(v).slice(0, 19) : "-") },
  ];

  return (
    <div className="manager-page-stack">
      <section className="manager-data-card">
        <Space wrap size={12}>
          <InputNumber
            placeholder="用户ID"
            min={1}
            precision={0}
            value={filters.userId}
            onChange={(v) => setFilters((f) => ({ ...f, userId: v ?? undefined }))}
            style={{ width: 120 }}
          />
          <Input
            placeholder="交易对"
            value={filters.symbol}
            onChange={(e) => setFilters((f) => ({ ...f, symbol: e.target.value }))}
            style={{ width: 140 }}
          />
          <Input
            placeholder="订单号"
            value={filters.orderNo}
            onChange={(e) => setFilters((f) => ({ ...f, orderNo: e.target.value }))}
            style={{ width: 180 }}
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={() => void load(1)}>查询</Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load(1)}>刷新</Button>
          <Tag style={{ color: "var(--manager-primary)", background: "var(--manager-gold-soft)", border: "1px solid rgba(240,185,11,0.28)" }}>
            共 {total} 条
          </Tag>
        </Space>
      </section>
      <section className="manager-data-card manager-table">
        <Table
          rowKey="id"
          loading={loading}
          dataSource={records}
          columns={columns}
          scroll={{ x: 1400 }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showSizeChanger: false,
            onChange: (p) => void load(p),
          }}
          size="small"
        />
      </section>
    </div>
  );
}

// --- Summary Tab ---
function SummaryTab() {
  const [records, setRecords] = useState<TradeUserSummaryRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ userId: undefined as number | undefined, coinCode: "", tradeCategory: "" });

  const load = async (nextPage = page, f = filters) => {
    setLoading(true);
    try {
      const result = await fetchUserSummary({
        pageIndex: nextPage,
        pageSize: PAGE_SIZE,
        userId: f.userId,
        coinCode: f.coinCode || undefined,
        tradeCategory: f.tradeCategory || undefined,
      });
      setRecords(result.data);
      setTotal(result.total);
      setPage(nextPage);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(1); }, []);

  const columns: ColumnsType<TradeUserSummaryRecord> = [
    { title: "ID", dataIndex: "id", width: 70 },
    { title: "用户ID", dataIndex: "userId", width: 90 },
    { title: "平台", dataIndex: "platformCode", width: 100 },
    { title: "币种", dataIndex: "coinCode", width: 90 },
    { title: "类型", dataIndex: "tradeCategory", width: 90 },
    { title: "日期", dataIndex: "tradeDate", width: 110 },
    { title: "总订单", dataIndex: "totalOrders", width: 90 },
    { title: "买入量", dataIndex: "buyAmount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "卖出量", dataIndex: "sellAmount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "买入额", dataIndex: "buyTotal", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "卖出额", dataIndex: "sellTotal", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "手续费", dataIndex: "totalFee", width: 100, render: (v) => Number(v).toFixed(6) },
    { title: "总成交量", dataIndex: "totalVolume", width: 110, render: (v) => Number(v).toFixed(4) },
  ];

  return (
    <div className="manager-page-stack">
      <section className="manager-data-card">
        <Space wrap size={12}>
          <InputNumber
            placeholder="用户ID"
            min={1}
            precision={0}
            value={filters.userId}
            onChange={(v) => setFilters((f) => ({ ...f, userId: v ?? undefined }))}
            style={{ width: 120 }}
          />
          <Input
            placeholder="币种"
            value={filters.coinCode}
            onChange={(e) => setFilters((f) => ({ ...f, coinCode: e.target.value }))}
            style={{ width: 120 }}
          />
          <Select
            allowClear
            placeholder="交易类型"
            value={filters.tradeCategory || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, tradeCategory: v ?? "" }))}
            options={[
              { label: "现货", value: "spot" },
              { label: "合约", value: "futures" },
              { label: "杠杆", value: "margin" },
            ]}
            style={{ width: 120 }}
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={() => void load(1)}>查询</Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load(1)}>刷新</Button>
          <Tag style={{ color: "var(--manager-primary)", background: "var(--manager-gold-soft)", border: "1px solid rgba(240,185,11,0.28)" }}>
            共 {total} 条
          </Tag>
        </Space>
      </section>
      <section className="manager-data-card manager-table">
        <Table
          rowKey="id"
          loading={loading}
          dataSource={records}
          columns={columns}
          scroll={{ x: 1200 }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showSizeChanger: false,
            onChange: (p) => void load(p),
          }}
          size="small"
        />
      </section>
    </div>
  );
}

// --- PNL Tab ---
function PnlTab() {
  const [records, setRecords] = useState<TradeUserPnlRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ userId: undefined as number | undefined, coinCode: "", tradeCategory: "" });

  const load = async (nextPage = page, f = filters) => {
    setLoading(true);
    try {
      const result = await fetchUserPnl({
        pageIndex: nextPage,
        pageSize: PAGE_SIZE,
        userId: f.userId,
        coinCode: f.coinCode || undefined,
        tradeCategory: f.tradeCategory || undefined,
      });
      setRecords(result.data);
      setTotal(result.total);
      setPage(nextPage);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(1); }, []);

  const columns: ColumnsType<TradeUserPnlRecord> = [
    { title: "ID", dataIndex: "id", width: 70 },
    { title: "用户ID", dataIndex: "userId", width: 90 },
    { title: "平台", dataIndex: "platformCode", width: 100 },
    { title: "币种", dataIndex: "coinCode", width: 90 },
    { title: "类型", dataIndex: "tradeCategory", width: 90 },
    { title: "日期", dataIndex: "tradeDate", width: 110 },
    { title: "已实现盈亏", dataIndex: "realizedPnl", width: 120, render: (v) => pnlCell(v) },
    { title: "未实现盈亏", dataIndex: "unrealizedPnl", width: 120, render: (v) => pnlCell(v) },
    {
      title: "总盈亏",
      dataIndex: "totalPnl",
      width: 120,
      render: (v) => {
        const n = Number(v);
        const color = n > 0 ? "#0ecb81" : n < 0 ? "#f6465d" : undefined;
        return (
          <strong style={{ color, fontSize: 14 }}>
            {n > 0 ? "+" : ""}{n.toFixed(4)}
          </strong>
        );
      },
    },
    { title: "盈亏率", dataIndex: "pnlRate", width: 100, render: (v) => pnlRateCell(v) },
    { title: "持仓量", dataIndex: "positionAmount", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "持仓成本", dataIndex: "positionCost", width: 110, render: (v) => Number(v).toFixed(4) },
    { title: "持仓价值", dataIndex: "positionValue", width: 110, render: (v) => Number(v).toFixed(4) },
  ];

  return (
    <div className="manager-page-stack">
      <section className="manager-data-card">
        <Space wrap size={12}>
          <InputNumber
            placeholder="用户ID"
            min={1}
            precision={0}
            value={filters.userId}
            onChange={(v) => setFilters((f) => ({ ...f, userId: v ?? undefined }))}
            style={{ width: 120 }}
          />
          <Input
            placeholder="币种"
            value={filters.coinCode}
            onChange={(e) => setFilters((f) => ({ ...f, coinCode: e.target.value }))}
            style={{ width: 120 }}
          />
          <Select
            allowClear
            placeholder="交易类型"
            value={filters.tradeCategory || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, tradeCategory: v ?? "" }))}
            options={[
              { label: "现货", value: "spot" },
              { label: "合约", value: "futures" },
              { label: "杠杆", value: "margin" },
            ]}
            style={{ width: 120 }}
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={() => void load(1)}>查询</Button>
          <Button icon={<ReloadOutlined />} onClick={() => void load(1)}>刷新</Button>
          <Tag style={{ color: "var(--manager-primary)", background: "var(--manager-gold-soft)", border: "1px solid rgba(240,185,11,0.28)" }}>
            共 {total} 条
          </Tag>
        </Space>
      </section>
      <section className="manager-data-card manager-table">
        <Table
          rowKey="id"
          loading={loading}
          dataSource={records}
          columns={columns}
          scroll={{ x: 1400 }}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            showSizeChanger: false,
            onChange: (p) => void load(p),
          }}
          size="small"
        />
      </section>
    </div>
  );
}

// --- Main Page ---
export default function TradeOrdersPage() {
  const [activeTab, setActiveTab] = useState("orders");

  return (
    <div className="manager-page-stack">
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          { key: "orders", label: "订单列表" },
          { key: "details", label: "成交明细" },
          { key: "summary", label: "用户汇总" },
          { key: "pnl", label: "盈亏分析" },
        ]}
        style={{ marginBottom: 0 }}
      />
      {activeTab === "orders" && <OrdersTab />}
      {activeTab === "details" && <DetailsTab />}
      {activeTab === "summary" && <SummaryTab />}
      {activeTab === "pnl" && <PnlTab />}
    </div>
  );
}
