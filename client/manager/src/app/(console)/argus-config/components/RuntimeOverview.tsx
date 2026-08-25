"use client";

import {
  ApiOutlined,
  CheckCircleFilled,
  CloudServerOutlined,
  CodeOutlined,
  ExclamationCircleFilled,
  PoweroffOutlined,
  ReloadOutlined,
  SyncOutlined,
  ThunderboltOutlined,
  WarningFilled,
} from "@ant-design/icons";
import { Button, Popconfirm, Space, Switch, Tag, Tooltip, Typography } from "antd";
import { useEffect, useState } from "react";
import type { ArgusControlResult, ArgusRuntimeStatus } from "../api/argus-config.api";

const { Text } = Typography;

export type ControlAction = "start" | "stop" | "restart" | "reload";

interface RuntimeOverviewProps {
  runtime: ArgusRuntimeStatus | null;
  currentVersion?: number;
  lastResult: ArgusControlResult | null;
  action: string | null;
  refreshing: boolean;
  autoRefresh: boolean;
  lastSyncAt: number | null;
  onAutoRefreshChange: (value: boolean) => void;
  onRefresh: () => void;
  onAction: (action: ControlAction) => void;
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatRelative(from?: string | number | null, now = Date.now()) {
  if (from === undefined || from === null) return "";
  const time = typeof from === "number" ? from : new Date(from).getTime();
  if (Number.isNaN(time)) return "";
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

export function RuntimeOverview({
  runtime,
  currentVersion,
  lastResult,
  action,
  refreshing,
  autoRefresh,
  lastSyncAt,
  onAutoRefreshChange,
  onRefresh,
  onAction,
}: RuntimeOverviewProps) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const online = runtime?.online === true;
  const busy = action !== null;
  const heartbeat = runtime?.heartbeat;
  const reloadError = heartbeat?.lastReloadError;
  const runningVersion = heartbeat?.version;
  const drift = online && runningVersion !== undefined && currentVersion !== undefined && runningVersion !== currentVersion;

  const disabledTip = (enabled: boolean, reason: string) => (enabled ? "" : reason);

  return (
    <section className="manager-argus-console">
      <div className="manager-argus-panel manager-argus-status">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon"><CloudServerOutlined /></span>
          进程状态
        </div>
        <div className="manager-argus-status__headline">
          {online ? "运行中" : "已停止"}
          <small>{online ? `已持续运行 ${formatUptime(heartbeat?.startedAt, now)}` : "无有效心跳"}</small>
        </div>
        <div className="manager-argus-status__rows">
          <div className="manager-argus-status__row">
            <span>实例 ID</span>
            <Tooltip title={heartbeat?.instanceId || "暂无实例"}>
              <b className="manager-argus-mono">{heartbeat?.instanceId || "—"}</b>
            </Tooltip>
          </div>
          <div className="manager-argus-status__row">
            <span>启动时间</span>
            <b>{formatDate(heartbeat?.startedAt)}</b>
          </div>
          <div className="manager-argus-status__row">
            <span>最后热加载</span>
            <b>{heartbeat?.reloadedAt ? formatRelative(heartbeat.reloadedAt, now) : "—"}</b>
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
            <span className="manager-argus-panel__icon"><ThunderboltOutlined /></span>
            运行控制
          </div>
          <Space size={8}>
            <Text type="secondary" style={{ fontSize: 12.5 }}>自动刷新</Text>
            <Switch size="small" checked={autoRefresh} onChange={onAutoRefreshChange} />
            <Tooltip title="立即拉取一次心跳与配置状态">
              <Button size="small" icon={<SyncOutlined spin={refreshing} />} onClick={onRefresh} disabled={busy}>
                刷新
              </Button>
            </Tooltip>
          </Space>
        </div>

        <div className="manager-argus-tiles">
          <div className={`manager-argus-tile${drift ? " manager-argus-tile--drift" : ""}`}>
            <span className="manager-argus-tile__label"><ApiOutlined />运行版本</span>
            <div className="manager-argus-tile__value">
              {runningVersion !== undefined ? `v${runningVersion}` : "—"}
            </div>
            <Text className="manager-argus-tile__hint">Argus 进程当前实际加载的配置版本</Text>
          </div>
          <div className={`manager-argus-tile${drift ? " manager-argus-tile--drift" : ""}`}>
            <span className="manager-argus-tile__label"><CheckCircleFilled />已发布版本</span>
            <div className="manager-argus-tile__value">
              {currentVersion !== undefined ? `v${currentVersion}` : "—"}
              {drift ? <Tag color="gold" style={{ marginLeft: 8, verticalAlign: "middle" }}>待生效</Tag> : null}
            </div>
            <Text className="manager-argus-tile__hint">
              {drift ? "与运行版本不一致，请执行重新加载" : "运行版本与发布版本一致"}
            </Text>
          </div>
          <div className={`manager-argus-tile${reloadError ? " manager-argus-tile--drift" : ""}`}>
            <span className="manager-argus-tile__label">
              {reloadError ? <WarningFilled /> : <ReloadOutlined />}热加载结果
            </span>
            <div className="manager-argus-tile__value manager-argus-tile__value--sm" style={reloadError ? { color: "var(--manager-danger)" } : undefined}>
              {reloadError ? "失败" : heartbeat?.reloadedAt ? "成功" : "暂无记录"}
            </div>
            <Text className="manager-argus-tile__hint">{formatDate(heartbeat?.reloadedAt)}</Text>
          </div>
        </div>

        <div className="manager-argus-controls">
          <Tooltip title={disabledTip(!online && !busy, online ? "进程已在运行中" : "")}>
            <Button
              type="primary"
              icon={<PoweroffOutlined />}
              loading={action === "start"}
              disabled={busy || online}
              onClick={() => onAction("start")}
            >
              启动
            </Button>
          </Tooltip>
          <Popconfirm
            title="确认停止 Argus？"
            description="将优雅停止交易与监控进程，停止期间不会产生新的委托。"
            okText="确认停止"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            disabled={busy || !online}
            onConfirm={() => onAction("stop")}
          >
            <Button danger icon={<PoweroffOutlined />} loading={action === "stop"} disabled={busy || !online}>
              停止
            </Button>
          </Popconfirm>
          <Popconfirm
            title="确认重启 Argus？"
            description="进程会先优雅停止，再以最新已发布配置重新启动。"
            okText="确认重启"
            cancelText="取消"
            disabled={busy || !online}
            onConfirm={() => onAction("restart")}
          >
            <Button icon={<ReloadOutlined />} loading={action === "restart"} disabled={busy || !online}>
              重启
            </Button>
          </Popconfirm>
          <Tooltip title={online ? "不中断进程，直接下发最新已发布配置" : "进程离线，无法热加载"}>
            <Button
              icon={<SyncOutlined />}
              loading={action === "reload"}
              disabled={busy || !online}
              onClick={() => onAction("reload")}
              type={drift ? "primary" : "default"}
              ghost={drift}
            >
              重新加载
            </Button>
          </Tooltip>
          <span className="manager-argus-controls__spacer" />
          {drift ? (
            <Text style={{ color: "var(--manager-primary)", fontSize: 12.5 }}>
              <ExclamationCircleFilled /> 已发布 v{currentVersion} 尚未在运行中生效
            </Text>
          ) : null}
        </div>

        {reloadError ? (
          <div className="manager-argus-log manager-argus-log--error">
            <span className="manager-argus-log__tag" style={{ color: "inherit" }}>ERROR</span>
            <span>{reloadError}</span>
          </div>
        ) : lastResult?.output ? (
          <div className="manager-argus-log">
            <span className="manager-argus-log__tag"><CodeOutlined /> {lastResult.action.toUpperCase()}</span>
            <span>{lastResult.output}</span>
          </div>
        ) : null}
      </div>
    </section>
  );
}
