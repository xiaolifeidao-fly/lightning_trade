"use client";

import {
  CheckCircleFilled,
  CloudServerOutlined,
  CodeOutlined,
  SyncOutlined,
  WarningFilled,
} from "@ant-design/icons";
import { Button, Space, Tag, Tooltip, Typography } from "antd";
import { useEffect, useState } from "react";
import type { ArgusConfigVersion, ArgusControlResult, ArgusHeartbeat, ArgusRuntimeStatus } from "../api/argus-config.api";
import type { EffectState } from "../hooks/useArgusConfig";

const { Text } = Typography;

interface RuntimeOverviewProps {
  instanceKey: string;
  runtime: ArgusRuntimeStatus | null;
  heartbeat?: ArgusHeartbeat;
  publishedVersion?: ArgusConfigVersion;
  effectState: EffectState;
  lastResult: ArgusControlResult | null;
  reloading: boolean;
  refreshing: boolean;
  lastSyncAt: number | null;
  onRefresh: () => void;
  onReload: () => void;
}

const effectMeta: Record<EffectState, { label: string; tone: "ok" | "warn" | "err" | "mute"; desc: string }> = {
  effective: { label: "已生效", tone: "ok", desc: "心跳回报的版本与校验和都与已发布版本一致" },
  awaiting: { label: "等待程序确认", tone: "warn", desc: "已发布成功，但程序还没回报读到这一版；未确认前不显示已生效" },
  drift: { label: "运行的是旧版本", tone: "err", desc: "程序心跳里的版本与已发布版本不一致，可能漏了广播消息" },
  offline: { label: "心跳超时", tone: "err", desc: "15 秒内没有心跳，无法确认程序实际在用哪一版配置" },
  unknown: { label: "尚无已发布配置", tone: "mute", desc: "该实例还没有发布过配置版本" },
};

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatRelative(from?: string | number | null, now = Date.now()) {
  if (from === undefined || from === null) return "—";
  const time = typeof from === "number" ? from : new Date(from).getTime();
  if (Number.isNaN(time)) return "—";
  const diff = Math.max(0, now - time);
  const second = Math.floor(diff / 1000);
  if (second < 60) return `${second} 秒前`;
  const minute = Math.floor(second / 60);
  if (minute < 60) return `${minute} 分钟前`;
  const hour = Math.floor(minute / 60);
  if (hour < 24) return `${hour} 小时前`;
  return `${Math.floor(hour / 24)} 天前`;
}

function formatUptime(startedAt?: string, now = Date.now()) {
  if (!startedAt) return "—";
  const start = new Date(startedAt).getTime();
  if (Number.isNaN(start)) return "—";
  const total = Math.max(0, Math.floor((now - start) / 1000));
  const day = Math.floor(total / 86400);
  const hour = Math.floor((total % 86400) / 3600);
  const minute = Math.floor((total % 3600) / 60);
  if (day > 0) return `${day} 天 ${hour} 小时`;
  if (hour > 0) return `${hour} 小时 ${minute} 分`;
  return `${minute} 分 ${total % 60} 秒`;
}

function shorten(value?: string, keep = 8) {
  if (!value) return "—";
  return value.length <= keep * 2 ? value : `${value.slice(0, keep)}…${value.slice(-6)}`;
}

export function RuntimeOverview({
  instanceKey,
  runtime,
  heartbeat,
  publishedVersion,
  effectState,
  lastResult,
  reloading,
  refreshing,
  lastSyncAt,
  onRefresh,
  onReload,
}: RuntimeOverviewProps) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const online = runtime?.online === true;
  const meta = effectMeta[effectState];
  const reloadError = heartbeat?.lastReloadError;
  // 心跳没带 checksum 说明实例还是旧构建：此时生效判定退化成只比版本号，必须说清楚。
  const checksumReported = Boolean(heartbeat?.configChecksum);

  const steps = [
    { title: "管理端保存草稿", detail: "POST /argus-config/drafts", done: Boolean(publishedVersion) },
    { title: "发布新版本", detail: "POST /argus-config/versions/:id/publish", done: Boolean(publishedVersion) },
    {
      title: "写 Redis 快照并广播",
      detail: `argus:config:${instanceKey || "<instance>"}:snapshot`,
      done: Boolean(publishedVersion?.snapshotChecksum),
    },
    {
      // 进程启动时就读到最新版本的话不会有 reload 记录，此时「已生效」本身即证明加载成功。
      title: "argus_single 热加载",
      detail: "ReplaceManager + ReloadMonitor",
      done: effectState === "effective" || (online && heartbeat?.lastReloadSuccess === true),
    },
    {
      title: "心跳回报已生效版本",
      detail: `argus:heartbeat:${instanceKey || "<instance>"}`,
      done: effectState === "effective",
    },
  ];

  return (
    <section className="manager-argus-console">
      <div className="manager-argus-panel manager-argus-status">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <CloudServerOutlined />
          </span>
          进程状态
        </div>
        <div className="manager-argus-status__headline">
          {online ? "运行中" : "已停止"}
          <small>{online ? `已持续运行 ${formatUptime(heartbeat?.startedAt, now)}` : "无有效心跳"}</small>
        </div>
        <div className="manager-argus-status__rows">
          <div className="manager-argus-status__row">
            <span>实例 ID</span>
            <Tooltip title={heartbeat?.instanceId || instanceKey || "暂无实例"}>
              <b className="manager-argus-mono">{heartbeat?.instanceId || instanceKey || "—"}</b>
            </Tooltip>
          </div>
          <div className="manager-argus-status__row">
            <span>进程心跳</span>
            <b>{formatRelative(heartbeat?.updatedAt, now)}</b>
          </div>
          <div className="manager-argus-status__row">
            <span>启动时间</span>
            <b>{formatDate(heartbeat?.startedAt)}</b>
          </div>
          <div className="manager-argus-status__row">
            <span>最后热加载</span>
            <b>{formatRelative(heartbeat?.lastReloadAt, now)}</b>
          </div>
          <div className="manager-argus-status__row">
            <span>状态同步</span>
            <b>{lastSyncAt ? formatRelative(lastSyncAt, now) : "—"}</b>
          </div>
        </div>
      </div>

      <div className="manager-argus-panel">
        <div className="manager-argus-panel__head">
          <div className="manager-argus-panel__title">
            <span className="manager-argus-panel__icon">
              <SyncOutlined />
            </span>
            生效链路
          </div>
          <Space size={8}>
            <Tooltip title="立即拉取一次心跳与配置状态">
              <Button size="small" icon={<SyncOutlined spin={refreshing} />} onClick={onRefresh}>
                刷新
              </Button>
            </Tooltip>
            <Tooltip title={online ? "不中断进程，直接下发最新已发布配置" : "进程离线，热加载指令不会被消费"}>
              <Button
                size="small"
                type={effectState === "drift" ? "primary" : "default"}
                icon={<SyncOutlined />}
                loading={reloading}
                disabled={!online}
                onClick={onReload}
              >
                立即热加载
              </Button>
            </Tooltip>
          </Space>
        </div>

        <div className="manager-argus-tiles">
          <div className={`manager-argus-tile${effectState === "effective" ? "" : " manager-argus-tile--drift"}`}>
            <span className="manager-argus-tile__label">
              {effectState === "effective" ? <CheckCircleFilled /> : <WarningFilled />}
              程序实际读到
            </span>
            <div className="manager-argus-tile__value">
              {heartbeat?.version !== undefined ? `v${heartbeat.version}` : "—"}
              <Tag
                color={meta.tone === "ok" ? "green" : meta.tone === "warn" ? "gold" : meta.tone === "err" ? "red" : "default"}
                style={{ marginLeft: 8, verticalAlign: "middle" }}
              >
                {meta.label}
              </Tag>
            </div>
            <Text className="manager-argus-tile__hint">{meta.desc}</Text>
          </div>
          <div className="manager-argus-tile">
            <span className="manager-argus-tile__label">已发布版本</span>
            <div className="manager-argus-tile__value">
              {publishedVersion ? `v${publishedVersion.version}` : "—"}
            </div>
            <Text className="manager-argus-tile__hint">
              {publishedVersion?.publishedAt ? `发布于 ${formatDate(publishedVersion.publishedAt)}` : "该实例尚未发布配置"}
            </Text>
          </div>
          <div className="manager-argus-tile">
            <span className="manager-argus-tile__label">快照校验和</span>
            <div className="manager-argus-tile__value manager-argus-tile__value--sm manager-argus-mono">
              <Tooltip title={heartbeat?.configChecksum || publishedVersion?.snapshotChecksum || "暂无校验和"}>
                {shorten(heartbeat?.configChecksum ?? publishedVersion?.snapshotChecksum)}
              </Tooltip>
            </div>
            <Text className="manager-argus-tile__hint">
              {checksumReported
                ? "心跳回报的校验和，与已发布快照逐字节对齐才算生效"
                : "该实例心跳未上报校验和（旧构建），当前只按版本号判定"}
            </Text>
          </div>
        </div>

        <div className="manager-argus-pipeline">
          {steps.map((step, index) => (
            <div key={step.title} className={`manager-argus-step${step.done ? " manager-argus-step--done" : ""}`}>
              <div className="manager-argus-step__head">
                <span className="manager-argus-step__index">{step.done ? "✓" : index + 1}</span>
                <span className="manager-argus-step__title">{step.title}</span>
              </div>
              <div className="manager-argus-step__detail manager-argus-mono">{step.detail}</div>
            </div>
          ))}
        </div>

        <div className="manager-argus-hint">
          <span>
            <b>兜底：</b>pub/sub 是「发过就没了」，广播那一刻订阅正好断线重连的话消息会永久丢失且不报错。因此 argus_single
            另有一个 <b>30 秒轮询</b>：比对 <span className="manager-argus-mono">argus:config:{instanceKey || "<instance>"}:version</span>
            （Redis 不可用时直接查 DB 的 published 版本），发现与当前 version 不一致就自行 Reload。正常秒级生效，漏消息时最坏 30
            秒补上。轮询本身由 r5「配置面完整收敛到 DB + 生效兜底轮询」交付；在它上线前，漏消息只能靠上方「立即热加载」手动补。
          </span>
        </div>

        {reloadError ? (
          <div className="manager-argus-log manager-argus-log--error">
            <span className="manager-argus-log__tag" style={{ color: "inherit" }}>
              RELOAD ERROR
            </span>
            <span>{reloadError}</span>
          </div>
        ) : lastResult?.output ? (
          <div className="manager-argus-log">
            <span className="manager-argus-log__tag">
              <CodeOutlined /> {lastResult.action.toUpperCase()}
            </span>
            <span>{lastResult.output}</span>
          </div>
        ) : null}
      </div>
    </section>
  );
}
