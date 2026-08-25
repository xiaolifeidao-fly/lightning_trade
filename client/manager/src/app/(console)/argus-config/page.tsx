"use client";

import { DashboardOutlined, SettingOutlined, ThunderboltFilled } from "@ant-design/icons";
import { Alert, Button, Skeleton, Tabs, Typography, message } from "antd";
import { useEffect, useState } from "react";
import type { ArgusConfigDraft } from "./api/argus-config.api";
import { ConfigEditor } from "./components/ConfigEditor";
import { RuntimeOverview, type ControlAction } from "./components/RuntimeOverview";
import { VersionRecord } from "./components/VersionRecord";
import { useArgusConfig } from "./hooks/useArgusConfig";

const { Text, Title } = Typography;

const actionLabel: Record<ControlAction, string> = {
  start: "启动",
  stop: "停止",
  restart: "重启",
  reload: "重新加载",
};

export default function ArgusConfigPage() {
  const {
    snapshot,
    runtime,
    loading,
    refreshing,
    saving,
    controlAction,
    lastResult,
    lastSyncAt,
    loadError,
    autoRefresh,
    setAutoRefresh,
    refresh,
    publishAndReload,
    runControl,
  } = useArgusConfig();
  const [dirty, setDirty] = useState(false);
  const [activeView, setActiveView] = useState<"runtime" | "config">("runtime");

  useEffect(() => {
    if (!dirty) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirty]);

  const onPublish = async (draft: ArgusConfigDraft) => {
    try {
      const version = await publishAndReload(draft);
      message.success(`配置 v${version.version} 已发布，并已下发热加载指令`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "发布配置失败");
      throw error;
    }
  };

  const onControl = async (action: ControlAction) => {
    try {
      const result = await runControl(action);
      message.success(result.output || `${actionLabel[action]}请求已完成`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : `${actionLabel[action]}失败`);
    }
  };

  const onRefresh = () => {
    void refresh()
      .then(() => message.success("状态已刷新"))
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "刷新失败"));
  };

  const retryLoad = () => {
    void refresh().catch(() => undefined);
  };

  const online = runtime?.online === true;

  return (
    <div className="manager-page-stack manager-argus">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS OPERATIONS</Text>
          <Title level={2} className="manager-argus-hero__title">配置与运行控制</Title>
          <Text className="manager-argus-hero__desc">
            集中维护 Argus 运行配置、账户与会话，发布即生成不可变版本并安全热加载，同时通过心跳实时掌控进程状态。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className={`manager-argus-beacon ${online ? "manager-argus-beacon--online" : "manager-argus-beacon--offline"}`}>
            <span className="manager-argus-beacon__dot" />
            {loading ? "状态获取中" : online ? "Argus 运行中" : "Argus 已停止 / 心跳超时"}
          </span>
          <span className="manager-argus-beacon">
            <ThunderboltFilled style={{ color: "var(--manager-primary)" }} />
            当前发布版本 v{snapshot?.version.version ?? "—"}
          </span>
        </div>
      </section>

      {loading && !snapshot ? (
        <>
          <div className="manager-argus-panel"><Skeleton active paragraph={{ rows: 4 }} /></div>
          <div className="manager-argus-panel"><Skeleton active paragraph={{ rows: 10 }} /></div>
        </>
      ) : (
        <>
          {!snapshot && loadError ? (
            <Alert
              className="manager-argus-alert"
              type="warning"
              showIcon
              message="暂时无法读取 Argus 配置"
              description="管理服务可能仍在启动，或暂时不可用。请稍后重试。"
              action={<Button size="small" loading={refreshing} onClick={retryLoad}>重试</Button>}
            />
          ) : null}
          <Tabs
            className="manager-argus-maintabs"
            activeKey={activeView}
            onChange={(key) => setActiveView(key as "runtime" | "config")}
            items={[
              {
                key: "runtime",
                label: (
                  <span className="manager-argus-maintab">
                    <DashboardOutlined />
                    运行状态
                    <span className={`manager-argus-maintab__dot${online ? " manager-argus-maintab__dot--on" : ""}`} />
                  </span>
                ),
                children: (
                  <div className="manager-argus-view">
                    <RuntimeOverview
                      runtime={runtime}
                      currentVersion={snapshot?.version.version}
                      lastResult={lastResult}
                      action={controlAction}
                      refreshing={refreshing}
                      autoRefresh={autoRefresh}
                      lastSyncAt={lastSyncAt}
                      onAutoRefreshChange={setAutoRefresh}
                      onRefresh={onRefresh}
                      onAction={(action) => void onControl(action)}
                    />
                    <VersionRecord version={snapshot?.version} />
                  </div>
                ),
              },
              {
                key: "config",
                label: (
                  <span className="manager-argus-maintab">
                    <SettingOutlined />
                    配置维护
                    {dirty ? <span className="manager-argus-maintab__badge">未发布</span> : null}
                  </span>
                ),
                children: (
                  <div className="manager-argus-view">
                    {!loadError || snapshot ? (
                      <ConfigEditor snapshot={snapshot} saving={saving} dirty={dirty} onDirtyChange={setDirty} onPublish={onPublish} />
                    ) : (
                      <div className="manager-argus-panel">
                        <div className="manager-argus-empty">配置暂不可用，请先重试加载。</div>
                      </div>
                    )}
                  </div>
                ),
              },
            ]}
          />
        </>
      )}
    </div>
  );
}
