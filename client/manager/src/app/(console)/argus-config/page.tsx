"use client";

import { AuditOutlined, SettingOutlined, SlidersOutlined, ThunderboltFilled } from "@ant-design/icons";
import { Alert, Button, Select, Skeleton, Tabs, Tag, Typography, message } from "antd";
import { useCallback, useEffect, useState } from "react";
import {
  fetchPublishedArgusConfig,
  type ArgusConfigDraft,
  type ArgusConfigVersion,
} from "./api/argus-config.api";
import { ConfigEditor } from "./components/ConfigEditor";
import { CoveragePanel } from "./components/CoveragePanel";
import { ParamConsole } from "./components/ParamConsole";
import { RuntimeOverview } from "./components/RuntimeOverview";
import { SessionAudit } from "./components/SessionAudit";
import { VersionHistory } from "./components/VersionHistory";
import { useArgusConfig } from "./hooks/useArgusConfig";
import { buildDraft, splitApplicableOverrides, type ParamOverride, type ParamPrefill } from "./params/catalog";

const { Text, Title } = Typography;

type ViewKey = "params" | "audit" | "full";

/**
 * 从地址栏读一次「把这组参数带去发布」的预填。
 *
 * 用 `window.location.search` 而不是 `useSearchParams()`：后者会让整页在构建时
 * 退化成 CSR bailout，而本页本来就是纯客户端页面，没必要为一个可选入参付这个代价。
 * 解析失败一律当作没有预填——地址栏里的东西不可信，坏参数不能让参数页打不开。
 */
function readPrefillFromUrl(): { instanceKey: string; prefill: ParamPrefill } | null {
  if (typeof window === "undefined") return null;
  const params = new URLSearchParams(window.location.search);
  const raw = params.get("prefill");
  const instanceKey = params.get("instance") ?? "";
  if (!raw || !instanceKey) return null;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    const values: Record<string, number> = {};
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
      const num = typeof value === "number" ? value : Number(value);
      if (Number.isFinite(num)) values[key] = num;
    }
    if (Object.keys(values).length === 0) return null;
    return {
      instanceKey,
      prefill: {
        values,
        accountLabel: params.get("account") ?? undefined,
        symbol: params.get("symbol") ?? undefined,
        note: params.get("note") ?? undefined,
        source: params.get("source") ?? undefined,
      },
    };
  } catch {
    return null;
  }
}

export default function ArgusConfigPage() {
  const {
    instances,
    instance,
    instanceKey,
    setInstanceKey,
    instanceError,
    snapshot,
    versions,
    runtime,
    heartbeat,
    effectState,
    loading,
    refreshing,
    saving,
    reloading,
    lastResult,
    lastSyncAt,
    loadError,
    refresh,
    publish,
    saveDraftOnly,
    rollback,
    reload,
  } = useArgusConfig();
  const [activeView, setActiveView] = useState<ViewKey>("params");
  const [fullConfigDirty, setFullConfigDirty] = useState(false);
  const [prefill, setPrefill] = useState<ParamPrefill | null>(null);

  // 带参数进来时先把编辑实例切到来源实例：参数按实例分域，落错实例就是「改 A 误伤 B」。
  useEffect(() => {
    const incoming = readPrefillFromUrl();
    if (!incoming) return;
    setActiveView("params");
    setInstanceKey(incoming.instanceKey);
    setPrefill(incoming.prefill);
  }, [setInstanceKey]);

  /**
   * 预填被消费后就把地址栏洗干净。留着 query 的话，刷新页面会把用户已经放弃的
   * 那组参数又填回来，而这组参数是可以直接影响实盘的。
   */
  const onPrefillConsumed = useCallback(() => {
    setPrefill(null);
    if (typeof window === "undefined") return;
    window.history.replaceState(null, "", window.location.pathname);
  }, []);

  // 完整配置表单里有未发布改动时拦一下关闭：这份表单包含账户与凭证，重填代价高。
  useEffect(() => {
    if (!fullConfigDirty) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [fullConfigDirty]);

  const onPublish = (draft: ArgusConfigDraft) => publish(draft);

  const onFullConfigPublish = async (draft: ArgusConfigDraft) => {
    try {
      const version = await publish(draft);
      setFullConfigDirty(false);
      message.success(`配置 v${version.version} 已发布，等待程序确认生效`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "发布配置失败");
      throw error;
    }
  };

  /**
   * 跨实例同步：基于目标实例自己的已发布快照生成草稿，只搬参数值，不碰账户与凭证；
   * 目标实例账户数不同而落不下的改动显式报出来，不静默丢弃。
   */
  const onSyncDraft = async (targetKey: string, releaseNote: string, overrides: Record<string, ParamOverride>) => {
    const target = instances.find((item) => item.instanceKey === targetKey);
    const targetSnapshot = await fetchPublishedArgusConfig(targetKey);
    if (!targetSnapshot) throw new Error("目标实例还没有已发布配置，无法生成草稿");
    const { applicable, skipped } = splitApplicableOverrides(targetSnapshot, overrides);
    if (Object.keys(applicable).length === 0) throw new Error("没有一项改动能落到目标实例上");
    const version = await saveDraftOnly(buildDraft(targetSnapshot, applicable, releaseNote, targetKey));
    const name = target?.instanceName || targetKey;
    message.success(`已在 ${name} 生成草稿 v${version.version}，需切换到该实例确认后再发布`);
    if (skipped.length) {
      message.warning(`${skipped.map((param) => param.label).join("、")} 未同步：目标实例没有对应的账户或币种行`);
    }
  };

  const onRollback = async (version: ArgusConfigVersion) => {
    try {
      await rollback(version.id, `回滚到 v${version.version}`);
      message.success(`已回滚到 v${version.version} 并广播，等待程序确认生效`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "回滚失败");
    }
  };

  const onReload = () => {
    void reload()
      .then((result) => message.success(result.output || "热加载指令已下发"))
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "下发热加载指令失败"));
  };

  const onRefresh = () => {
    void refresh()
      .then(() => message.success("状态已刷新"))
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "刷新失败"));
  };

  const online = runtime?.online === true;

  if (instanceError) {
    return (
      <div className="manager-page-stack manager-argus">
        <Alert
          className="manager-argus-alert"
          type="error"
          showIcon
          message="无法读取实例注册表"
          description="参数按实例分域，读不到实例列表就不能编辑任何参数——否则服务端会按默认实例兜底，那正是「改 A 误伤 B」的入口。请确认 manager-api 已启动且 argus_instance 表已完成导入。"
        />
      </div>
    );
  }

  return (
    <div className="manager-page-stack manager-argus">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS OPERATIONS</Text>
          <Title level={2} className="manager-argus-hero__title">
            参数与运行控制
          </Title>
          <Text className="manager-argus-hero__desc">
            按实例维护 Argus 策略参数，发布即生成不可变版本并定向热加载；生效与否以该实例心跳回报的版本与校验和为准。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <div className="manager-argus-scope">
            <span className="manager-argus-scope__label">编辑实例</span>
            <Select
              value={instanceKey || undefined}
              placeholder="请选择实例"
              style={{ minWidth: 220 }}
              options={instances.map((item) => ({
                value: item.instanceKey,
                label: item.instanceName ? `${item.instanceName}（${item.instanceKey}）` : item.instanceKey,
              }))}
              onChange={setInstanceKey}
            />
          </div>
          <span
            className={`manager-argus-beacon ${online ? "manager-argus-beacon--online" : "manager-argus-beacon--offline"}`}
          >
            <span className="manager-argus-beacon__dot" />
            {loading ? "状态获取中" : online ? "Argus 运行中" : "Argus 已停止 / 心跳超时"}
          </span>
          <span className="manager-argus-beacon">
            <ThunderboltFilled style={{ color: "var(--manager-primary)" }} />
            已发布 v{snapshot?.version.version ?? "—"}
          </span>
        </div>
      </section>

      <Alert
        className="manager-argus-alert"
        type="warning"
        showIcon
        message="参数按实例分域，发布只影响当前选中的这一个实例"
        description={
          <span>
            当前编辑的是 <b>{instance?.instanceName || instanceKey || "（未选择）"}</b>
            <Tag className="manager-argus-mono" style={{ marginInlineStart: 8 }}>
              {instanceKey || "—"}
            </Tag>
            {instance?.configSource ? <Tag>配置来源 {instance.configSource}</Tag> : null}
            。这里没有「全部实例」这个选项——champion / challenger 实验体系要求三个实例的参数互相隔离，一次发布波及多个实例就等于把对照组毁掉。要改别的实例，用上方选择器切换。
          </span>
        }
      />

      {loading && !snapshot ? (
        <>
          <div className="manager-argus-panel">
            <Skeleton active paragraph={{ rows: 4 }} />
          </div>
          <div className="manager-argus-panel">
            <Skeleton active paragraph={{ rows: 10 }} />
          </div>
        </>
      ) : (
        <>
          {!snapshot && loadError ? (
            <Alert
              className="manager-argus-alert"
              type="warning"
              showIcon
              message="暂时无法读取该实例的配置"
              description="管理服务可能仍在启动，或该实例尚未导入配置。请稍后重试。"
              action={
                <Button size="small" loading={refreshing} onClick={() => void refresh().catch(() => undefined)}>
                  重试
                </Button>
              }
            />
          ) : null}

          <RuntimeOverview
            instanceKey={instanceKey}
            runtime={runtime}
            heartbeat={heartbeat}
            publishedVersion={snapshot?.version}
            effectState={effectState}
            lastResult={lastResult}
            reloading={reloading}
            refreshing={refreshing}
            lastSyncAt={lastSyncAt}
            onRefresh={onRefresh}
            onReload={onReload}
          />

          <Tabs
            className="manager-argus-maintabs"
            activeKey={activeView}
            onChange={(key) => setActiveView(key as ViewKey)}
            items={[
              {
                key: "params",
                label: (
                  <span className="manager-argus-maintab">
                    <SlidersOutlined />
                    参数与发布
                  </span>
                ),
                children: (
                  <div className="manager-argus-view">
                    <ParamConsole
                      instance={instance}
                      instances={instances}
                      snapshot={snapshot}
                      saving={saving}
                      onPublish={onPublish}
                      onSyncDraft={onSyncDraft}
                      prefill={prefill}
                      onPrefillConsumed={onPrefillConsumed}
                    />
                    <VersionHistory versions={versions} saving={saving} onRollback={(version) => void onRollback(version)} />
                  </div>
                ),
              },
              {
                key: "audit",
                label: (
                  <span className="manager-argus-maintab">
                    <AuditOutlined />
                    配置面巡检
                  </span>
                ),
                children: (
                  <div className="manager-argus-view manager-argus-view--split">
                    <CoveragePanel />
                    <SessionAudit
                      sessions={snapshot?.sessions ?? []}
                      accounts={snapshot?.accounts ?? []}
                      instanceKey={instanceKey}
                      onRotated={() => void refresh().catch(() => undefined)}
                    />
                  </div>
                ),
              },
              {
                key: "full",
                label: (
                  <span className="manager-argus-maintab">
                    <SettingOutlined />
                    完整配置
                    {fullConfigDirty ? <span className="manager-argus-maintab__badge">未发布</span> : null}
                  </span>
                ),
                children: (
                  <div className="manager-argus-view">
                    {snapshot ? (
                      <ConfigEditor
                        instanceKey={instanceKey}
                        snapshot={snapshot}
                        saving={saving}
                        dirty={fullConfigDirty}
                        onDirtyChange={setFullConfigDirty}
                        onPublish={onFullConfigPublish}
                      />
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
