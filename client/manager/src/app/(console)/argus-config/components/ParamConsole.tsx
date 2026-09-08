"use client";

import { Alert, Empty, Modal, Segmented, Select, Space, Typography, message } from "antd";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ArgusConfigDraft, ArgusConfigSnapshot, ArgusConfigVersion, ArgusInstance } from "../api/argus-config.api";
import {
  ALL_PARAMS,
  buildDraft,
  findParam,
  formatParamValue,
  overrideKey,
  readAllParams,
  readParam,
  resolvePrefill,
  type ParamChange,
  type ParamContext,
  type ParamDef,
  type ParamOverride,
  type ParamPrefill,
  type PrefillSkip,
  type QuickAction,
} from "../params/catalog";
import { ChangePreview } from "./ChangePreview";
import { ParamForm } from "./ParamForm";
import { QuickActions } from "./QuickActions";

const { Text } = Typography;

interface ParamConsoleProps {
  instance: ArgusInstance | null;
  instances: ArgusInstance[];
  snapshot: ArgusConfigSnapshot | null;
  saving: boolean;
  onPublish: (draft: ArgusConfigDraft) => Promise<ArgusConfigVersion>;
  /**
   * 跨实例同步只生成草稿。草稿必须基于目标实例自己的已发布快照生成——直接把源
   * 实例的快照搬过去会连账户、凭证、会话一起覆盖，所以这里只把改动传出去，
   * 拉取目标快照与套用改动交给页面层完成。
   */
  onSyncDraft: (targetKey: string, releaseNote: string, overrides: Record<string, ParamOverride>) => Promise<void>;
  /**
   * 回测 / 寻优页带过来的参数预填。**只预填，不发布**——填完仍要人写发布说明、
   * 看 diff、过关键参数二次确认，与手工改参数走完全相同的那条路。
   */
  prefill?: ParamPrefill | null;
  /** 预填已被消费（或被判定落不下）时回调，页面据此清掉地址栏里的参数。 */
  onPrefillConsumed?: () => void;
}

export function ParamConsole({
  instance,
  instances,
  snapshot,
  saving,
  onPublish,
  onSyncDraft,
  prefill,
  onPrefillConsumed,
}: ParamConsoleProps) {
  const [overrides, setOverrides] = useState<Record<string, ParamOverride>>({});
  const [releaseNote, setReleaseNote] = useState("");
  const [riskIndex, setRiskIndex] = useState(0);
  const [symbolIndex, setSymbolIndex] = useState(0);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [syncTarget, setSyncTarget] = useState("");
  const [prefillSource, setPrefillSource] = useState("");
  const [prefillSkips, setPrefillSkips] = useState<PrefillSkip[]>([]);
  const appliedPrefillRef = useRef("");

  const ctx: ParamContext = useMemo(() => ({ symbolIndex, riskIndex }), [riskIndex, symbolIndex]);

  const baseline = useMemo(() => (snapshot ? readAllParams(snapshot, ctx) : {}), [ctx, snapshot]);

  // 当前上下文下的显示值：基线叠加落在这个账户 / 币种上的改动。
  const values = useMemo(() => {
    const merged = { ...baseline };
    for (const param of ALL_PARAMS) {
      const override = overrides[overrideKey(param, ctx)];
      if (override) merged[param.key] = override.value;
    }
    return merged;
  }, [baseline, ctx, overrides]);

  const accountLabel = useCallback(
    (index: number) => {
      const risk = snapshot?.accountRisks?.[index];
      if (!risk) return `风控 #${index + 1}`;
      const account = snapshot?.accounts?.find((item) => item.id === risk.accountId);
      return account?.accountName || `账户 #${risk.accountId}`;
    },
    [snapshot],
  );

  const scopeLabel = useCallback(
    (param: ParamDef, target: ParamContext) => {
      if (param.scope === "account") return accountLabel(target.riskIndex);
      if (param.scope === "symbol") return snapshot?.monitorSymbols?.[target.symbolIndex]?.symbol || "币种";
      return "实例全局";
    },
    [accountLabel, snapshot],
  );

  const changes: ParamChange[] = useMemo(() => {
    if (!snapshot) return [];
    const result: ParamChange[] = [];
    for (const override of Object.values(overrides)) {
      const param = findParam(override.paramKey);
      if (!param) continue;
      const from = readParam(param, snapshot, override.ctx);
      if (from === override.value) continue;
      result.push({ param, ctx: override.ctx, scopeLabel: scopeLabel(param, override.ctx), from, to: override.value });
    }
    return result;
  }, [overrides, scopeLabel, snapshot]);

  const setParam = useCallback(
    (param: ParamDef, value: number, target: ParamContext) => {
      setOverrides((previous) => ({
        ...previous,
        [overrideKey(param, target)]: { paramKey: param.key, ctx: target, value },
      }));
    },
    [],
  );

  const reset = useCallback(() => {
    setOverrides({});
    setReleaseNote("");
    setPrefillSource("");
    setPrefillSkips([]);
  }, []);

  /**
   * 消费一次预填。只在 (实例 + 预填内容) 变化时套用一次：套完之后用户的手工编辑
   * 与「放弃改动」都不能被这个 effect 重新覆盖掉。
   */
  useEffect(() => {
    if (!prefill || !snapshot) return;
    const token = `${snapshot.instanceKey}|${JSON.stringify(prefill)}`;
    if (appliedPrefillRef.current === token) return;
    appliedPrefillRef.current = token;

    const resolved = resolvePrefill(snapshot, prefill);
    if (resolved.symbolResolved) setSymbolIndex(resolved.ctx.symbolIndex);
    if (resolved.accountResolved) setRiskIndex(resolved.ctx.riskIndex);
    setOverrides(resolved.overrides);
    setReleaseNote(prefill.note?.trim() || "");
    setPrefillSource(prefill.source || "回测结果");
    setPrefillSkips(resolved.skipped);
    if (Object.keys(resolved.overrides).length === 0) {
      message.warning("这组参数没有一项能落到当前实例上，草稿未改动");
    } else {
      message.warning(`已预填 ${resolved.applied.length} 项改动，确认后自行发布——本页不会替你发布任何参数`);
    }
    onPrefillConsumed?.();
  }, [onPrefillConsumed, prefill, snapshot]);

  const applyQuickAction = useCallback(
    (action: QuickAction) => {
      const param = findParam(action.paramKey);
      if (!param) return;
      setParam(param, action.value, ctx);
      setReleaseNote((previous) => previous || `极端行情人工干预：${param.label} → ${formatParamValue(param, action.value)}`);
      message.warning("已填入草稿，确认改动后再发布");
    },
    [ctx, setParam],
  );

  const submit = useCallback(async () => {
    if (!snapshot || !instance) return;
    setConfirmOpen(false);
    const draft = buildDraft(snapshot, overrides, releaseNote.trim(), instance.instanceKey);
    try {
      const version = await onPublish(draft);
      reset();
      message.success(`配置 v${version.version} 已发布到 ${instance.instanceName || instance.instanceKey}，等待程序确认生效`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "发布配置失败");
    }
  }, [instance, onPublish, overrides, releaseNote, reset, snapshot]);

  const tryPublish = useCallback(() => {
    if (!releaseNote.trim()) {
      message.error("请填写发布说明");
      return;
    }
    if (changes.some((change) => change.param.critical)) {
      setConfirmOpen(true);
      return;
    }
    void submit();
  }, [changes, releaseNote, submit]);

  const syncDraft = useCallback(async () => {
    if (!snapshot || !syncTarget) return;
    if (!releaseNote.trim()) {
      message.error("请先填写发布说明，草稿会沿用它");
      return;
    }
    const note = `${releaseNote.trim()}（从 ${instance?.instanceName || instance?.instanceKey} 同步）`;
    try {
      await onSyncDraft(syncTarget, note, overrides);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "生成跨实例草稿失败");
    }
  }, [instance, onSyncDraft, overrides, releaseNote, snapshot, syncTarget]);

  if (!snapshot) {
    return (
      <div className="manager-argus-panel">
        <div className="manager-argus-empty">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该实例还没有已发布配置，请先用 argus-config-import 导入一份" />
        </div>
      </div>
    );
  }

  const riskOptions = (snapshot.accountRisks ?? []).map((_, index) => ({ value: index, label: accountLabel(index) }));
  const symbolOptions = (snapshot.monitorSymbols ?? []).map((symbol, index) => ({
    value: index,
    label: symbol.symbol || `币种 #${index + 1}`,
  }));
  const criticalChanges = changes.filter((change) => change.param.critical);

  return (
    <>
      <QuickActions disabled={saving} onApply={applyQuickAction} />

      {prefillSource ? (
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message={`参数来自「${prefillSource}」，已填入草稿但尚未发布`}
          description={
            <span>
              回测与寻优给出的是统计结论，不是可直接上线的决定——
              <b>本页不会替你发布任何参数</b>。请逐条核对下方 diff、写清发布说明，关键参数还会再要一次确认。
              {prefillSkips.length ? (
                <>
                  <br />
                  以下 {prefillSkips.length} 项没有填入：
                  {prefillSkips.map((skip) => (
                    <span key={skip.key} style={{ display: "block" }}>
                      · <span className="manager-argus-mono">{skip.param?.label || skip.key}</span>：{skip.reason}
                    </span>
                  ))}
                </>
              ) : null}
            </span>
          }
        />
      ) : null}

      <div className="manager-argus-paramlayout">
        <div className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title">参数编辑</div>
            <Space size={10} wrap>
              {symbolOptions.length > 1 ? (
                <Space size={6}>
                  <Text type="secondary" style={{ fontSize: 12.5 }}>
                    币种
                  </Text>
                  <Select size="small" value={symbolIndex} options={symbolOptions} onChange={setSymbolIndex} style={{ minWidth: 120 }} />
                </Space>
              ) : null}
              {riskOptions.length > 1 ? (
                <Segmented
                  size="small"
                  value={riskIndex}
                  options={riskOptions}
                  onChange={(value) => setRiskIndex(Number(value))}
                />
              ) : null}
            </Space>
          </div>
          {riskOptions.length > 1 ? (
            <Alert
              className="manager-argus-alert"
              type="info"
              showIcon
              style={{ marginBottom: 12 }}
              message={`账户级参数当前编辑的是「${accountLabel(riskIndex)}」，切换账户不会丢失已改动的值`}
            />
          ) : null}
          <ParamForm
            values={values}
            baseline={baseline}
            disabled={saving}
            onChange={(param, value) => setParam(param, value, ctx)}
          />
        </div>

        <ChangePreview
          changes={changes}
          releaseNote={releaseNote}
          onReleaseNoteChange={setReleaseNote}
          instance={instance}
          otherInstances={instances.filter((item) => item.instanceKey !== instance?.instanceKey)}
          syncTarget={syncTarget}
          onSyncTargetChange={setSyncTarget}
          saving={saving}
          onReset={reset}
          onPublish={tryPublish}
          onSyncDraft={() => void syncDraft()}
        />
      </div>

      <Modal
        title="确认发布关键参数改动"
        open={confirmOpen}
        okText="确认发布"
        cancelText="取消"
        confirmLoading={saving}
        width={560}
        onCancel={() => setConfirmOpen(false)}
        onOk={() => void submit()}
      >
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={`本次包含 ${criticalChanges.length} 项关键参数改动`}
          description="它们会立即影响实盘下单与风控行为。发布后可随时在版本历史里一键回滚到上一版。"
        />
        <div className="manager-argus-difflist">
          {criticalChanges.map((change) => (
            <div className="manager-argus-diff" key={`${change.param.key}-${change.scopeLabel}`}>
              <span className="manager-argus-diff__key">
                {change.param.label}
                <small className="manager-argus-diff__scope">{change.scopeLabel}</small>
              </span>
              <span className="manager-argus-diff__old manager-argus-mono">{formatParamValue(change.param, change.from)}</span>
              <span className="manager-argus-diff__arrow">→</span>
              <span className="manager-argus-diff__new manager-argus-mono">{formatParamValue(change.param, change.to)}</span>
            </div>
          ))}
        </div>
      </Modal>
    </>
  );
}
