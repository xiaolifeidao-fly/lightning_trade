"use client";

import { RocketOutlined, UndoOutlined } from "@ant-design/icons";
import { Button, Empty, Input, Popconfirm, Select, Space, Tag, Tooltip, Typography } from "antd";
import type { ArgusInstance } from "../api/argus-config.api";
import { formatParamValue, type ParamChange } from "../params/catalog";

const { Text } = Typography;

interface ChangePreviewProps {
  changes: ParamChange[];
  releaseNote: string;
  onReleaseNoteChange: (value: string) => void;
  instance: ArgusInstance | null;
  otherInstances: ArgusInstance[];
  syncTarget: string;
  onSyncTargetChange: (value: string) => void;
  saving: boolean;
  onReset: () => void;
  onPublish: () => void;
  onSyncDraft: () => void;
}

export function ChangePreview({
  changes,
  releaseNote,
  onReleaseNoteChange,
  instance,
  otherInstances,
  syncTarget,
  onSyncTargetChange,
  saving,
  onReset,
  onPublish,
  onSyncDraft,
}: ChangePreviewProps) {
  const needRestart = changes.filter((change) => !change.param.hot);
  const pendingRuntime = changes.filter((change) => change.param.pendingRuntime);
  const critical = changes.filter((change) => change.param.critical);

  return (
    <div className="manager-argus-panel">
      <div className="manager-argus-panel__head">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <RocketOutlined />
          </span>
          改动预览
        </div>
        <Space size={8}>
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            {changes.length ? `${changes.length} 项变更` : "暂无变更"}
          </Text>
          <Popconfirm
            title="放弃当前改动？"
            description="表单将恢复为该实例最近一次已发布的配置内容。"
            okText="放弃"
            cancelText="取消"
            disabled={!changes.length || saving}
            onConfirm={onReset}
          >
            <Button size="small" icon={<UndoOutlined />} disabled={!changes.length || saving}>
              放弃改动
            </Button>
          </Popconfirm>
        </Space>
      </div>

      {changes.length === 0 ? (
        <div className="manager-argus-empty">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="草稿与当前已发布版本一致，改任意参数后这里会列出逐项差异" />
        </div>
      ) : (
        <div className="manager-argus-difflist">
          {changes.map((change) => (
            <div className="manager-argus-diff" key={`${change.param.key}-${change.scopeLabel}`}>
              <span className="manager-argus-diff__key">
                {change.param.label}
                <small className="manager-argus-mono">{change.param.storeKey}</small>
                <small className="manager-argus-diff__scope">{change.scopeLabel}</small>
              </span>
              <span className="manager-argus-diff__old manager-argus-mono">{formatParamValue(change.param, change.from)}</span>
              <span className="manager-argus-diff__arrow">→</span>
              <span className="manager-argus-diff__new manager-argus-mono">{formatParamValue(change.param, change.to)}</span>
            </div>
          ))}
        </div>
      )}

      {needRestart.length ? (
        <div className="manager-argus-hint manager-argus-hint--warn">
          <span>
            <b>{needRestart.map((change) => change.param.label).join("、")}</b> 属于结构性参数，发布后<b>不会热生效</b>，需要重启 argus_single。
          </span>
        </div>
      ) : null}
      {pendingRuntime.length ? (
        <div className="manager-argus-hint manager-argus-hint--warn">
          <span>
            <b>{pendingRuntime.map((change) => change.param.label).join("、")}</b> 会写进配置版本快照，但 argus_single 运行时还没消费这些值，改完
            <b>不会改变策略行为</b>；消费侧由 r5「配置面完整收敛到 DB」打通。
          </span>
        </div>
      ) : null}
      {critical.length ? (
        <div className="manager-argus-hint manager-argus-hint--danger">
          <span>
            包含 <b>{critical.length}</b> 项关键参数，发布前需要二次确认。
          </span>
        </div>
      ) : null}

      <div className="manager-argus-publish">
        <label className="manager-argus-publish__label">发布说明（必填）</label>
        <Input.TextArea
          rows={2}
          maxLength={500}
          showCount
          value={releaseNote}
          disabled={saving}
          placeholder="例如：极端行情临时收紧仓位上限"
          onChange={(event) => onReleaseNoteChange(event.target.value)}
        />
        <div className="manager-argus-publish__actions">
          <Tooltip title={changes.length ? "" : "当前没有待发布的改动"}>
            <Button type="primary" icon={<RocketOutlined />} loading={saving} disabled={!changes.length} onClick={onPublish}>
              发布到 {instance?.instanceName || instance?.instanceKey || "当前实例"}
            </Button>
          </Tooltip>
          <span className="manager-argus-controls__spacer" />
          {otherInstances.length ? (
            <Space size={6}>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                同步到
              </Text>
              <Select
                size="small"
                value={syncTarget || undefined}
                placeholder="选择实例"
                style={{ minWidth: 168 }}
                onChange={onSyncTargetChange}
                options={otherInstances.map((item) => ({
                  value: item.instanceKey,
                  label: item.instanceName || item.instanceKey,
                }))}
              />
              <Tooltip title="只在目标实例生成草稿，不发布：目标实例的账户与凭证不同，必须到那个实例上确认后再发布">
                <Button size="small" disabled={!changes.length || !syncTarget || saving} onClick={onSyncDraft}>
                  生成草稿
                </Button>
              </Tooltip>
            </Space>
          ) : (
            <Tag>仅注册了一个实例</Tag>
          )}
        </div>
      </div>
    </div>
  );
}
