"use client";

import {
  AlertOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  AuditOutlined,
  DollarCircleOutlined,
  FundOutlined,
  LineChartOutlined,
  RiseOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import { Button, Progress, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";

const { Text, Title } = Typography;

type Direction = "long" | "short" | "neutral";
type StrategyStatus = "running" | "paused" | "warn";

interface StrategyRecord {
  key: string;
  name: string;
  category: string;
  symbol: string;
  leverage: string;
  direction: Direction;
  notional: string;
  unrealizedPnl: string;
  unrealizedPnlPct: number;
  winRate: number;
  status: StrategyStatus;
}

const strategyData: StrategyRecord[] = [
  {
    key: "grid-btc",
    name: "网格 · BTC 主力",
    category: "区间网格",
    symbol: "BTC/USDT",
    leverage: "5x",
    direction: "long",
    notional: "$2,486,210",
    unrealizedPnl: "+$48,210",
    unrealizedPnlPct: 1.94,
    winRate: 68,
    status: "running",
  },
  {
    key: "trend-eth",
    name: "趋势跟随 · ETH",
    category: "动量趋势",
    symbol: "ETH/USDT",
    leverage: "10x",
    direction: "short",
    notional: "$1,820,450",
    unrealizedPnl: "+$22,860",
    unrealizedPnlPct: 1.26,
    winRate: 57,
    status: "running",
  },
  {
    key: "arb-sol",
    name: "跨期套利 · SOL",
    category: "期现套利",
    symbol: "SOL/USDT",
    leverage: "3x",
    direction: "neutral",
    notional: "$986,310",
    unrealizedPnl: "+$6,420",
    unrealizedPnlPct: 0.65,
    winRate: 82,
    status: "running",
  },
  {
    key: "funding-bnb",
    name: "资金费率套利 · BNB",
    category: "资金费",
    symbol: "BNB/USDT",
    leverage: "2x",
    direction: "long",
    notional: "$642,100",
    unrealizedPnl: "-$1,840",
    unrealizedPnlPct: -0.29,
    winRate: 74,
    status: "warn",
  },
  {
    key: "ma-doge",
    name: "均线突破 · DOGE",
    category: "动量趋势",
    symbol: "DOGE/USDT",
    leverage: "5x",
    direction: "short",
    notional: "$284,560",
    unrealizedPnl: "-$3,210",
    unrealizedPnlPct: -1.13,
    winRate: 49,
    status: "paused",
  },
];

const directionMeta: Record<Direction, { label: string; color: string; bg: string }> = {
  long: { label: "多", color: "#0ECB81", bg: "rgba(14,203,129,0.14)" },
  short: { label: "空", color: "#F6465D", bg: "rgba(246,70,93,0.14)" },
  neutral: { label: "中性", color: "#4D7EFF", bg: "rgba(77,126,255,0.14)" },
};

const statusMeta: Record<StrategyStatus, { label: string; color: string }> = {
  running: { label: "运行中", color: "green" },
  paused: { label: "已暂停", color: "default" },
  warn: { label: "需关注", color: "gold" },
};

const columns: ColumnsType<StrategyRecord> = [
  {
    title: "策略",
    dataIndex: "name",
    width: 220,
    render: (value, record) => (
      <div className="manager-dashboard-table-title">
        {value}
        <Text className="manager-dashboard-table-subtitle">{record.category}</Text>
      </div>
    ),
  },
  {
    title: "交易对",
    dataIndex: "symbol",
    width: 140,
    render: (value, record) => (
      <Space size={6}>
        <span style={{ fontFamily: '"JetBrains Mono", "SF Mono", monospace', fontWeight: 600 }}>{value}</span>
        <Tag style={{ marginInlineEnd: 0, borderColor: "#2B3139", background: "#14171D", color: "#B7BDC6" }}>
          {record.leverage}
        </Tag>
      </Space>
    ),
  },
  {
    title: "方向",
    dataIndex: "direction",
    width: 90,
    render: (value: Direction) => {
      const meta = directionMeta[value];
      return (
        <Tag
          style={{
            marginInlineEnd: 0,
            color: meta.color,
            background: meta.bg,
            border: `1px solid ${meta.color}33`,
            fontWeight: 700,
          }}
        >
          {meta.label}
        </Tag>
      );
    },
  },
  { title: "名义本金", dataIndex: "notional", width: 140 },
  {
    title: "未实现盈亏",
    dataIndex: "unrealizedPnl",
    width: 160,
    render: (value: string, record) => {
      const up = record.unrealizedPnlPct >= 0;
      return (
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <span
            style={{
              color: up ? "#0ECB81" : "#F6465D",
              fontWeight: 700,
              fontVariantNumeric: "tabular-nums",
            }}
          >
            {value}
          </span>
          <Text style={{ color: up ? "#0ECB81" : "#F6465D", fontSize: 12 }}>
            {up ? <ArrowUpOutlined /> : <ArrowDownOutlined />} {Math.abs(record.unrealizedPnlPct).toFixed(2)}%
          </Text>
        </div>
      );
    },
  },
  {
    title: "胜率",
    dataIndex: "winRate",
    width: 160,
    render: (value: number) => (
      <div className="manager-dashboard-rate-cell">
        <Progress
          percent={value}
          showInfo={false}
          strokeColor={{ from: "#FCD535", to: "#F0B90B" }}
          trailColor="#2B3139"
          size="small"
        />
        <span style={{ color: "var(--manager-text)", fontVariantNumeric: "tabular-nums" }}>{value}%</span>
      </div>
    ),
  },
  {
    title: "状态",
    dataIndex: "status",
    width: 110,
    render: (value: StrategyStatus) => {
      const meta = statusMeta[value];
      return <Tag color={meta.color}>{meta.label}</Tag>;
    },
  },
];

const stats = [
  {
    label: "24h 已实现 PnL",
    value: "+184,520",
    unit: "USDT",
    hint: "净胜率 62.4% · 收益率 +1.48%",
    icon: <LineChartOutlined />,
    positive: true,
  },
  {
    label: "持仓总市值",
    value: "6.22",
    unit: "M USDT",
    hint: "多头 58% · 空头 42%",
    icon: <FundOutlined />,
    positive: true,
  },
  {
    label: "历史夏普比率",
    value: "2.18",
    unit: "",
    hint: "近 90 日年化波动 12.6%",
    icon: <RiseOutlined />,
    positive: true,
  },
  {
    label: "最大回撤",
    value: "-6.4",
    unit: "%",
    hint: "近 30 日 · 触发于 04-19",
    icon: <AlertOutlined />,
    positive: false,
  },
];

const totals = [
  {
    label: "资产总额 AUM",
    value: "$12,486,320",
    icon: <WalletOutlined />,
  },
  {
    label: "今日成交笔数",
    value: "1,286",
    icon: <ThunderboltOutlined />,
  },
  {
    label: "在跑策略",
    value: "14 / 18",
    icon: <SafetyCertificateOutlined />,
  },
];

const alerts = [
  {
    level: "高风险",
    color: "red",
    title: "BTC/USDT 主力网格 接近止损线",
    desc: "未实现亏损 -1.92%，距强平价 ¥1,840，建议人工复核。",
  },
  {
    level: "资金费",
    color: "gold",
    title: "BNB 永续 资金费率连续 3 期为负",
    desc: "近 8 小时累计成本 +0.18%，可考虑减仓或切换合约。",
  },
  {
    level: "API",
    color: "blue",
    title: "Binance API 调用速率达到 78%",
    desc: "近 5 分钟权重 1,560 / 2,400，临近限额。",
  },
];

const allocation = [
  { name: "BTC", value: 42, color: "#F7931A" },
  { name: "ETH", value: 26, color: "#627EEA" },
  { name: "SOL", value: 14, color: "#14F195" },
  { name: "BNB", value: 10, color: "#F0B90B" },
  { name: "其他", value: 8, color: "#848E9C" },
];

export default function ManagerDashboardPage() {
  return (
    <div className="manager-page-stack manager-dashboard">
      <section className="manager-dashboard-hero">
        <div>
          <Text className="manager-section-label">实盘控制台 · 24H Overview</Text>
          <Title level={1} className="manager-dashboard-hero__title">
            净值 +1.48%，已实现盈亏 +184,520 USDT，14 条策略持续运行中。
          </Title>
          <Text className="manager-dashboard-hero__subtitle">
            汇总交易所账户、策略实盘、风控告警于一屏。多空敞口、资金费率与回撤指标实时同步。
          </Text>
        </div>
        <Space wrap className="manager-dashboard-hero__actions">
          <Button type="primary" icon={<LineChartOutlined />}>
            查看实时净值
          </Button>
          <Button icon={<AuditOutlined />}>风控复核台</Button>
        </Space>
      </section>

      <section className="manager-dashboard-total-strip">
        {totals.map((item) => (
          <div key={item.label} className="manager-dashboard-total-strip__item">
            <span className="manager-dashboard-total-strip__icon">{item.icon}</span>
            <div>
              <span className="manager-dashboard-total-strip__label">{item.label}</span>
              <strong>{item.value}</strong>
            </div>
          </div>
        ))}
      </section>

      <section className="manager-dashboard-metric-grid">
        {stats.map((item) => (
          <div key={item.label} className="manager-dashboard-metric">
            <div className="manager-dashboard-metric__topline">
              <span>{item.label}</span>
              <span className="manager-dashboard-metric__icon">{item.icon}</span>
            </div>
            <div
              className="manager-dashboard-metric__value"
              style={{ color: item.positive ? "#EAECEF" : "#F6465D" }}
            >
              {item.value}
              {item.unit ? <span>{item.unit}</span> : null}
            </div>
            <Text className="manager-dashboard-metric__hint">{item.hint}</Text>
          </div>
        ))}
      </section>

      <section className="manager-dashboard-main-grid">
        <div className="manager-dashboard-panel manager-dashboard-panel--wide">
          <div className="manager-dashboard-panel__header">
            <DollarCircleOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>策略实盘表现</h3>
              <Text>按策略、交易对、方向汇总持仓与未实现盈亏；管理员可查看全部账户。</Text>
            </div>
          </div>
          <div className="manager-table">
            <Table<StrategyRecord>
              rowKey="key"
              columns={columns}
              dataSource={strategyData}
              pagination={false}
              scroll={{ x: 980 }}
            />
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <AlertOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>风控告警</h3>
              <Text>仓位、资金费率、接口配额异常会聚合到此。</Text>
            </div>
          </div>
          <div className="manager-dashboard-category-list">
            {alerts.map((alert) => (
              <div key={alert.title} className="manager-dashboard-task">
                <Tag color={alert.color}>{alert.level}</Tag>
                <strong>{alert.title}</strong>
                <Text>{alert.desc}</Text>
              </div>
            ))}
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <FundOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>持仓分布</h3>
              <Text>按底层资产汇总名义本金占比。</Text>
            </div>
          </div>
          <div className="manager-dashboard-category-list">
            {allocation.map((item) => (
              <Allocation key={item.name} name={item.name} value={item.value} color={item.color} />
            ))}
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <SafetyCertificateOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>权限说明</h3>
              <Text>管理端按角色控制资金与策略操作范围。</Text>
            </div>
          </div>
          <div className="manager-dashboard-role-list">
            <Role label="超管" value="全账户 · 资金划转" tag="全局" tagColor="gold" />
            <Role label="交易员" value="策略启停 · 改参数" tag="受限" tagColor="blue" />
            <Role label="风控" value="只读 · 强平干预" tag="只读" tagColor="green" />
          </div>
        </div>
      </section>
    </div>
  );
}

function Allocation({ name, value, color }: { name: string; value: number; color: string }) {
  return (
    <div className="manager-dashboard-category-row">
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}>
        <span className="manager-dashboard-category-name" style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span
            style={{
              width: 8,
              height: 8,
              borderRadius: 999,
              background: color,
              boxShadow: `0 0 10px ${color}aa`,
              display: "inline-block",
            }}
          />
          {name}
        </span>
        <span style={{ color: "var(--manager-text)", fontVariantNumeric: "tabular-nums", fontWeight: 600 }}>
          {value}%
        </span>
      </div>
      <Progress percent={value} showInfo={false} strokeColor={color} trailColor="#2B3139" />
    </div>
  );
}

function Role({
  label,
  value,
  tag,
  tagColor,
}: {
  label: string;
  value: string;
  tag: string;
  tagColor: string;
}) {
  return (
    <div className="manager-dashboard-role-row">
      <span>{label}</span>
      <strong>{value}</strong>
      <Tag color={tagColor}>{tag}</Tag>
    </div>
  );
}
