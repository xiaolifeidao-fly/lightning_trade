"use client";

import { ClusterOutlined, DatabaseOutlined, ReloadOutlined, SafetyCertificateOutlined } from "@ant-design/icons";
import { Alert, Button, Divider, Tag, Typography } from "antd";
import { InstanceCards } from "./components/InstanceCards";
import { ParamMatrix } from "./components/ParamMatrix";
import { useArgusInstances } from "./hooks/useArgusInstances";

const { Text, Title } = Typography;

/**
 * 实例对比页。
 *
 * 页面只解释和巡检配置分域，不承载任何进程控制。配置变更仍须在单实例参数页经历
 * 草稿、确认与心跳回报链路，避免在横向对比时误伤实验对照组。
 */
export default function ArgusInstancesPage() {
  const view = useArgusInstances();
  const instances = view.overview?.instances ?? [];
  const summaries = view.summary?.instances ?? [];

  return (
    <div className="manager-page-stack manager-argus manager-instances">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS INSTANCE TOPOLOGY</Text>
          <Title level={2} className="manager-argus-hero__title">实例与参数对比</Title>
          <Text className="manager-argus-hero__desc">三个部署单元各自拥有版本序列、Redis 快照与心跳回报。这里展示差异与异常，帮助保持 champion / challenger 对照隔离；页面不提供任何参数编辑或进程控制。</Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon"><ClusterOutlined style={{ color: "var(--manager-primary)" }} />{instances.length} 个登记实例</span>
          <Button size="small" icon={<ReloadOutlined />} loading={view.loading} onClick={() => void view.refresh()}>刷新状态</Button>
        </div>
      </section>

      {view.error ? <Alert className="manager-argus-alert" type="warning" showIcon message="部分实例数据读取失败" description={`${view.error}。对应单元格以「未发布」或「—」展示，不用默认值伪造比较。`} /> : null}
      {view.overview?.duplicateInstanceKeys?.length ? <Alert className="manager-argus-alert" type="error" showIcon message="重复实例键告警" description={`检测到 ${view.overview.duplicateInstanceKeys.join("、")}。重复键会让 Redis 心跳、发布版本与事件归属串线，必须先修复注册数据。`} /> : null}
      {view.overview?.driftInstanceKeys?.length ? <Alert className="manager-argus-alert" type="warning" showIcon message="参数版本漂移告警" description={`以下实例的已发布版本尚未由自身心跳确认：${view.overview.driftInstanceKeys.join("、")}。版本号是实例内序列，不能横向比较 vN 大小。`} /> : null}

      <InstanceCards instances={instances} summaries={summaries} />
      <ParamMatrix instances={instances} snapshots={view.snapshots} loading={view.loading} />

      <div className="manager-instances-notes">
        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head"><div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><DatabaseOutlined /></span>配置分域说明</div></div>
          <div className="manager-instances-timeline">
            <Note title="实例注册表" detail="argus_instance 以唯一 instance_key 登记名称、来源与启用状态；检测到历史重复键会直接告警。" />
            <Note title="版本按实例递增" detail="argus_config_version 以 (instance_key, version) 与 (instance_key, published_slot) 分域；实例 A 的 v37 和实例 B 的 v37 没有新旧关系。" />
            <Note title="Redis 快照与广播定向" detail="键名为 argus:config:{instanceKey}:snapshot / version；同一频道的消息带 instanceId，其他实例会忽略。" />
            <Note title="心跳确认生效" detail="argus:heartbeat:{instanceKey} 回报实际加载的版本和校验和；发布成功不等于已生效，必须等本实例心跳对上。" />
          </div>
        </section>
        <section className="manager-argus-panel">
          <div className="manager-argus-panel__head"><div className="manager-argus-panel__title"><span className="manager-argus-panel__icon"><SafetyCertificateOutlined /></span>运行控制可达性</div><Tag color="gold">只读说明</Tag></div>
          <div className="manager-instances-reachability">
            <Reachability action="参数发布 / 热加载" channel="Redis 配置快照与广播" reach="可达（进程在线时）" tone="ok" detail="目标实例按 instanceId 消费并由心跳确认；本页不提供下发入口。" />
            <Reachability action="reload" channel="Redis argus:control" reach="可达（进程在线时）" tone="ok" detail="仅在单实例参数页触发，进程不中断。" />
            <Reachability action="start / stop / restart" channel="本地 control.sh" reach="不可跨服务器" tone="err" detail="管理端已移除这些入口：停进程会失去持仓看管，且远端实例无法靠本机脚本操作。" />
          </div>
        </section>
      </div>
      <Divider />
      <Text type="secondary">页面使用只读接口：实例概览、已发布快照与逐实例事件汇总。矩阵只比较真实快照，不会把缺失参数默认为 0。</Text>
    </div>
  );
}

function Note({ title, detail }: { title: string; detail: string }) {
  return <div className="manager-instances-timeline__item"><b>{title}</b><span>{detail}</span></div>;
}

function Reachability({ action, channel, reach, tone, detail }: { action: string; channel: string; reach: string; tone: "ok" | "err"; detail: string }) {
  return <div className="manager-instances-reachability__row"><div><b>{action}</b><span className="manager-argus-mono">{channel}</span></div><Tag color={tone === "ok" ? "green" : "red"}>{reach}</Tag><p>{detail}</p></div>;
}
