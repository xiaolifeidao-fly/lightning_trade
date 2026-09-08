"use client";

import { CheckCircleFilled, ExclamationCircleFilled, PauseCircleFilled } from "@ant-design/icons";
import { Empty, Tag, Tooltip, Typography } from "antd";
import type { ArgusInstanceRuntime } from "@/components/argus/argus-instance.api";
import { effectStateMeta } from "@/components/argus/argus-instance.api";
import type { InstanceSummary } from "../../argus-dashboard/api/argus-dashboard.api";
import { fmtAge } from "../../argus-dashboard/constants";
import { EMPTY, fmtInt, fmtSigned, signColor } from "../../argus-signals/constants";

const { Text } = Typography;

interface InstanceCardsProps {
  instances: ArgusInstanceRuntime[];
  summaries: InstanceSummary[];
}

/** 每张卡只描述一个部署单元；不把三个实验实例压成一个「总状态」。 */
export function InstanceCards({ instances, summaries }: InstanceCardsProps) {
  const summaryByKey = new Map(summaries.map((item) => [item.instanceKey, item]));
  if (instances.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无已登记的 Argus 实例" />;
  return (
    <div className="manager-instances-cards">
      {instances.map((runtime) => <InstanceCard key={runtime.instanceKey} runtime={runtime} summary={summaryByKey.get(runtime.instanceKey)} />)}
    </div>
  );
}

function InstanceCard({ runtime, summary }: { runtime: ArgusInstanceRuntime; summary?: InstanceSummary }) {
  const meta = effectStateMeta(runtime.effectState);
  const tagColor = meta.tone === "ok" ? "green" : meta.tone === "warn" ? "gold" : meta.tone === "err" ? "red" : "default";
  const stateIcon = runtime.online ? <CheckCircleFilled /> : runtime.enabled ? <ExclamationCircleFilled /> : <PauseCircleFilled />;
  return (
    <section className={`manager-argus-panel manager-instances-card manager-instances-card--${meta.tone}`}>
      <div className="manager-argus-panel__head">
        <div>
          <div className="manager-argus-panel__title">{runtime.instanceName || runtime.instanceKey}</div>
          <Text className="manager-argus-mono manager-instances-card__key">{runtime.instanceKey}</Text>
        </div>
        <Tag color={tagColor} icon={stateIcon}>{runtime.online ? meta.label : runtime.enabled ? "心跳超时" : "已停用"}</Tag>
      </div>
      <div className="manager-instances-card__facts">
        <Fact label="配置来源" value={runtime.configSource || EMPTY} mono />
        <Fact label="心跳" value={runtime.online ? fmtAge(runtime.heartbeatAgeSeconds) : "—"} />
        <Fact label="参数版本" value={`已发 v${runtime.publishedVersion || "—"} · 运行 v${runtime.runningVersion || "—"}`} mono />
        <Fact label="运行构建" value={runtime.buildVersion || EMPTY} mono />
        <Fact label="窗口内变体" value={summary?.variants?.join(" / ") || EMPTY} />
        <Fact label="当前净仓" value={`${fmtInt(summary?.netSizeTotal)} 张`} mono />
        <Fact label="窗口内信号" value={fmtInt(summary?.signals)} mono />
        <Fact label="窗口内盈亏" value={fmtSigned(summary?.realizedPnl, 2, " U")} mono color={signColor(summary?.realizedPnl)} />
      </div>
      {runtime.description ? <Text className="manager-instances-card__description">{runtime.description}</Text> : null}
      {runtime.lastReloadError ? <Tooltip title={runtime.lastReloadError}><div className="manager-instances-card__error">最近重载报错：{runtime.lastReloadError}</div></Tooltip> : null}
    </section>
  );
}

function Fact({ label, value, mono, color }: { label: string; value: string; mono?: boolean; color?: string }) {
  return <div className="manager-instances-card__fact"><span>{label}</span><b className={mono ? "manager-argus-mono" : undefined} style={{ color }}>{value}</b></div>;
}
