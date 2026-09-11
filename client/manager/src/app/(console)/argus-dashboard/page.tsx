"use client";

import { ClusterOutlined, ReloadOutlined } from "@ant-design/icons";
import { Alert, Button, Segmented, Typography } from "antd";
import { ALL_INSTANCES } from "@/components/argus/instanceScope";
import { RANGE_OPTIONS } from "../argus-signals/constants";
import { AllInstancesView } from "./components/AllInstancesView";
import { SingleInstanceView } from "./components/SingleInstanceView";
import { useArgusDashboard } from "./hooks/useArgusDashboard";

const { Text, Title } = Typography;

/**
 * Argus 总览。
 *
 * 选择器位于 ManagerShell，所有 Argus 页面共用同一份作用域；这里仅根据作用域切换
 * 「三实例巡检」与「单实例运行画像」，不额外造一套页面内选择器。
 */
export default function ArgusDashboardPage() {
  const dashboard = useArgusDashboard();
  const singleSummary = dashboard.summary?.instances.find((item) => item.instanceKey === dashboard.scope);
  const runtime = dashboard.overview?.instances.find((item) => item.instanceKey === dashboard.scope);

  return (
    <div className="manager-page-stack manager-argus manager-dashboard">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS COMMAND VIEW</Text>
          <Title level={2} className="manager-argus-hero__title">多实例状态总览</Title>
          <Text className="manager-argus-hero__desc">
            顶栏实例选择贯穿总览、实例对比、行情、复盘与参数页，并在刷新后保持。全部实例只做并排巡检；锁定一个实例后再看权益、触发结果与数据健康。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon"><ClusterOutlined style={{ color: "var(--manager-primary)" }} />{dashboard.scope === ALL_INSTANCES ? "全部实例巡检" : `已锁定 ${dashboard.scope}`}</span>
          {/* 后台轮询不再闪骨架了，那就得有个地方说清数据取自什么时候，
              否则一页静止不动的数字看不出是新的还是卡住了。 */}
          <Text type="secondary" style={{ fontSize: 12 }}>
            {dashboard.updatedAt ? `${clockOf(dashboard.updatedAt)} 更新 · 每 60 秒自动刷新` : "读取中…"}
          </Text>
          <Button
            size="small"
            icon={<ReloadOutlined />}
            loading={dashboard.loading || dashboard.refreshing}
            // 手动刷新也走静默：按钮自己会转，没必要把已经在看的内容打回骨架。
            onClick={() => void dashboard.refresh({ silent: true })}
          >
            刷新状态
          </Button>
        </div>
      </section>

      <div className="manager-dashboard-toolbar manager-argus-panel">
        <div>
          <span className="manager-dashboard-toolbar__label">统计窗口</span>
          <Segmented
            value={dashboard.rangeKey}
            options={RANGE_OPTIONS.map((item) => ({ value: item.key, label: item.label }))}
            onChange={(value) => dashboard.setRangeKey(value as typeof dashboard.rangeKey)}
          />
        </div>
        <Text type="secondary">实际窗口：{dashboard.summary?.window.start || dashboard.options?.dataRange.start || "—"} ~ {dashboard.summary?.window.end || dashboard.options?.dataRange.end || "—"}</Text>
      </div>

      {dashboard.error ? <Alert className="manager-argus-alert" type="warning" showIcon message="部分总览数据读取失败" description={`${dashboard.error}。其余成功读取的区块仍可查看；缺失值以「—」显示，不按 0 处理。`} /> : null}

      {dashboard.scope === ALL_INSTANCES ? (
        <AllInstancesView overview={dashboard.overview} summary={dashboard.summary} loading={dashboard.loading} onSelectInstance={dashboard.setScope} />
      ) : (
        <SingleInstanceView
          instanceKey={dashboard.scope}
          runtime={runtime}
          summary={singleSummary}
          equity={dashboard.equity}
          gateStats={dashboard.gateStats}
          signals={dashboard.signals}
          snapshot={dashboard.snapshot}
          timeline={dashboard.timeline}
          loading={dashboard.loading}
        />
      )}
    </div>
  );
}

/** 只取时分秒：这页要回答的是"这批数字有多新"，日期在实际窗口那一行已经有了。 */
function clockOf(timestamp: number): string {
  const date = new Date(timestamp);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}
