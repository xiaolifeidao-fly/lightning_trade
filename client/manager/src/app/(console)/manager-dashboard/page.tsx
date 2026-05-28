"use client";

import {
  AlertOutlined,
  AuditOutlined,
  BankOutlined,
  DatabaseOutlined,
  FileDoneOutlined,
  TeamOutlined,
} from "@ant-design/icons";
import { Button, Progress, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";

const { Text, Title } = Typography;

interface StationRecord {
  key: string;
  station: string;
  area: string;
  salesmen: string;
  farmers: string;
  records: string;
  weight: string;
  amount: string;
  status: "normal" | "warn";
}

const stationData: StationRecord[] = [
  {
    key: "north",
    station: "北城粮站",
    area: "周口示范区",
    salesmen: "6 人",
    farmers: "28 户",
    records: "42 笔",
    weight: "62.4 吨",
    amount: "¥27.8万",
    status: "normal",
  },
  {
    key: "south",
    station: "南环粮站",
    area: "南环收购片区",
    salesmen: "4 人",
    farmers: "16 户",
    records: "25 笔",
    weight: "38.2 吨",
    amount: "¥16.9万",
    status: "warn",
  },
  {
    key: "west",
    station: "西郊粮站",
    area: "西郊临时点",
    salesmen: "3 人",
    farmers: "7 户",
    records: "19 笔",
    weight: "26.2 吨",
    amount: "¥11.7万",
    status: "normal",
  },
];

const columns: ColumnsType<StationRecord> = [
  {
    title: "粮站",
    dataIndex: "station",
    width: 180,
    render: (value, record) => (
      <div className="manager-dashboard-table-title">
        {value}
        <Text className="manager-dashboard-table-subtitle">{record.area}</Text>
      </div>
    ),
  },
  { title: "业务员", dataIndex: "salesmen", width: 120 },
  { title: "农户数", dataIndex: "farmers", width: 120 },
  { title: "录入笔数", dataIndex: "records", width: 120 },
  { title: "收粮量", dataIndex: "weight", width: 120 },
  {
    title: "金额",
    dataIndex: "amount",
    width: 120,
    render: (value) => <span className="manager-dashboard-money">{value}</span>,
  },
  {
    title: "状态",
    dataIndex: "status",
    width: 120,
    render: (value: StationRecord["status"]) =>
      value === "warn" ? <Tag color="gold">4 笔待补</Tag> : <Tag color="green">正常</Tag>,
  },
];

const stats = [
  { label: "今日收粮量", value: "126.8", unit: "吨", hint: "较昨日 +9.6%" },
  { label: "今日金额", value: "56.4", unit: "万", hint: "均价 2.22 元/kg" },
  { label: "待复核记录", value: "18", unit: "笔", hint: "其中 4 笔资料待补" },
  { label: "建档农户", value: "1,286", unit: "户", hint: "今日新增 9 户" },
];

const todos = [
  { type: "资料", title: "张秀兰银行卡照片待补", desc: "业务员赵敏提交，影响后期开票与付款核对" },
  { type: "价格", title: "陈玉山小麦单价偏高", desc: "系统计算 1.42 元/斤，超过预警线" },
];

export default function ManagerDashboardPage() {
  return (
    <div className="manager-page-stack manager-dashboard">
      <section className="manager-dashboard-hero">
        <div>
          <Text className="manager-section-label">今日全局概览</Text>
          <Title level={1} className="manager-dashboard-hero__title">
            今日已录入 86 笔，覆盖 51 户农户，合计收粮 126.8 吨。
          </Title>
          <Text className="manager-dashboard-hero__subtitle">
            按粮站、业务员、农户、品类汇总收粮业务数据，异常提交与价格波动在管理端集中处理。
          </Text>
        </div>
        <Space wrap className="manager-dashboard-hero__actions">
          <Button type="primary" icon={<DatabaseOutlined />}>
            查看收粮记录
          </Button>
          <Button icon={<AuditOutlined />}>处理待审核</Button>
        </Space>
      </section>

      <section className="manager-dashboard-metric-grid">
        {stats.map((item) => (
          <div key={item.label} className="manager-dashboard-metric">
            <div className="manager-dashboard-metric__topline">
              <span>{item.label}</span>
              <FileDoneOutlined className="manager-dashboard-metric__icon" />
            </div>
            <div className="manager-dashboard-metric__value">
              {item.value}
              <span>{item.unit}</span>
            </div>
            <Text className="manager-dashboard-metric__hint">{item.hint}</Text>
          </div>
        ))}
      </section>

      <section className="manager-dashboard-main-grid">
        <div className="manager-dashboard-panel manager-dashboard-panel--wide">
          <div className="manager-dashboard-panel__header">
            <BankOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>今日粮站表现</h3>
              <Text>管理员可跨粮站查看汇总，粮站角色只显示本站。</Text>
            </div>
          </div>
          <div className="manager-table">
            <Table<StationRecord>
              rowKey="key"
              columns={columns}
              dataSource={stationData}
              pagination={false}
              scroll={{ x: 900 }}
            />
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <AlertOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>待办</h3>
              <Text>需要管理端处理或关注的异常。</Text>
            </div>
          </div>
          <div className="manager-dashboard-category-list">
            {todos.map((todo) => (
              <div key={todo.title} className="manager-dashboard-task">
                <Tag color={todo.type === "资料" ? "blue" : "gold"}>{todo.type}</Tag>
                <strong>{todo.title}</strong>
                <Text>{todo.desc}</Text>
              </div>
            ))}
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <TeamOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>品类占比</h3>
              <Text>今日已录入粮食品类结构。</Text>
            </div>
          </div>
          <div className="manager-dashboard-category-list">
            <Category name="小麦" value={62} />
            <Category name="玉米" value={31} />
            <Category name="其他" value={7} />
          </div>
        </div>

        <div className="manager-dashboard-panel">
          <div className="manager-dashboard-panel__header">
            <AuditOutlined className="manager-dashboard-panel__icon" />
            <div>
              <h3>权限说明</h3>
              <Text>管理端按角色控制数据范围。</Text>
            </div>
          </div>
          <div className="manager-dashboard-role-list">
            <Role label="管理员" value="全量操作" tag="全局" />
            <Role label="粮站" value="站点数据" tag="受限" />
            <Role label="业务员" value="本人数据" tag="小程序" />
          </div>
        </div>
      </section>
    </div>
  );
}

function Category({ name, value }: { name: string; value: number }) {
  return (
    <div className="manager-dashboard-category-row">
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}>
        <span className="manager-dashboard-category-name">{name}</span>
        <span>{value}%</span>
      </div>
      <Progress percent={value} showInfo={false} strokeColor="#237a4b" trailColor="#e1e7dc" />
    </div>
  );
}

function Role({ label, value, tag }: { label: string; value: string; tag: string }) {
  return (
    <div className="manager-dashboard-role-row">
      <span>{label}</span>
      <strong>{value}</strong>
      <Tag color={tag === "全局" ? "green" : tag === "受限" ? "blue" : "gold"}>{tag}</Tag>
    </div>
  );
}
