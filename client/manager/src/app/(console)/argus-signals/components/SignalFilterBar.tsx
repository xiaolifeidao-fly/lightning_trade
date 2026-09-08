"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Button, Checkbox, Select, Space, Tooltip } from "antd";
import type {
  EventAccountOption,
  EventConfigVersionOption,
  ExitKindOption,
  SignalFilterOptions,
} from "../api/argus-signals.api";
import { RANGE_OPTIONS, TIME_FIELD_OPTIONS } from "../constants";
import type { SignalFilters, SignalView } from "../hooks/useArgusSignals";

interface SignalFilterBarProps {
  view: SignalView;
  options: SignalFilterOptions | null;
  accountOptions: EventAccountOption[];
  versionOptions: EventConfigVersionOption[];
  exitKinds: ExitKindOption[];
  filters: SignalFilters;
  loading: boolean;
  onChange: (patch: Partial<SignalFilters>) => void;
  onReset: () => void;
  onRefresh: () => void;
}

/**
 * 三个视角共用的筛选条。
 *
 * 「全部实例」是刻意保留的选项（与参数页相反）：复盘就是要横着看三个实例同一
 * 时刻的判定差异。代价是账户名跨实例重号，所以账户选项在全部实例视图下带实例
 * 前缀，列表里也逐行标实例归属。
 */
export function SignalFilterBar({
  view,
  options,
  accountOptions,
  versionOptions,
  exitKinds,
  filters,
  loading,
  onChange,
  onReset,
  onRefresh,
}: SignalFilterBarProps) {
  const instances = options?.instances ?? [];
  const showSignalFilters = view === "signal";
  const showEpisodeFilters = view === "episode";
  const showTimeField = view !== "signal";

  /** 结果筛选 = 两个聚合值 + 全部具体门控种类，与服务端 Result 参数同口径。 */
  const resultOptions = [
    { value: "", label: "全部结果" },
    { value: "open", label: "成功开仓" },
    { value: "blocked", label: "被条件挡住（全部）" },
    ...(options?.gateKinds ?? []).map((item) => ({ value: item.value, label: item.label })),
  ];

  return (
    <div className="manager-argus-signalbar">
      <Select
        className="manager-filter-input"
        style={{ minWidth: 220 }}
        value={filters.instanceKey}
        onChange={(value) => onChange({ instanceKey: value })}
        options={[
          { value: "", label: "全部实例（跨实例对照）" },
          ...instances.map((item) => ({
            value: item.instanceKey,
            label: `${item.instanceName || item.instanceKey}${item.registered ? "" : "（未注册）"}`,
          })),
        ]}
      />

      <Select
        className="manager-filter-input"
        style={{ minWidth: 130 }}
        value={filters.instrument || undefined}
        placeholder="全部币种"
        allowClear
        onChange={(value) => onChange({ instrument: value ?? "" })}
        options={(options?.instruments ?? []).map((item) => ({ value: item, label: item }))}
      />

      <Select
        className="manager-filter-input"
        style={{ minWidth: 200 }}
        mode="multiple"
        maxTagCount="responsive"
        allowClear
        placeholder="全部账户"
        value={filters.accountLabels}
        onChange={(value) => onChange({ accountLabels: value })}
        options={accountOptions.map((item) => ({
          value: item.accountLabel,
          // 全部实例视图下带上实例前缀：光看 account1 分不出是哪个实例的账户。
          label: filters.instanceKey
            ? `${item.accountLabel}${item.variant ? ` · ${item.variant}` : ""}`
            : `${item.instanceKey} / ${item.accountLabel}`,
        }))}
      />

      {showSignalFilters ? (
        <>
          <Select
            className="manager-filter-input"
            style={{ minWidth: 190 }}
            mode="multiple"
            maxTagCount="responsive"
            allowClear
            placeholder={filters.category === "trigger" ? "四类触发事件" : "全部事件类型"}
            value={filters.events}
            onChange={(value) => onChange({ events: value })}
            options={(options?.eventKinds ?? []).map((item) => ({ value: item.value, label: item.label }))}
          />
          <Select
            className="manager-filter-input"
            style={{ minWidth: 100 }}
            value={filters.category}
            disabled={filters.events.length > 0}
            onChange={(value) => onChange({ category: value })}
            options={[
              { value: "trigger", label: "触发类" },
              { value: "exit", label: "出场类" },
              { value: "all", label: "全部" },
            ]}
          />
          <Select
            className="manager-filter-input"
            style={{ minWidth: 190 }}
            value={filters.result}
            onChange={(value) => onChange({ result: value })}
            options={resultOptions}
          />
          <Select
            className="manager-filter-input"
            style={{ minWidth: 140 }}
            value={filters.strength}
            onChange={(value) => onChange({ strength: value })}
            options={[
              { value: "", label: "全部强度" },
              ...(options?.strengths ?? []).map((item) => ({ value: item.value, label: item.label })),
            ]}
          />
          <Select
            className="manager-filter-input"
            style={{ minWidth: 170 }}
            mode="multiple"
            maxTagCount="responsive"
            allowClear
            placeholder="全部参数版本"
            value={filters.configVersions}
            onChange={(value) => onChange({ configVersions: value })}
            options={versionOptions.map((item) => ({
              value: String(item.configVersion),
              label: filters.instanceKey
                ? `v${item.configVersion}（${item.eventCount} 条）`
                : `${item.instanceKey} v${item.configVersion}`,
            }))}
          />
        </>
      ) : null}

      {showEpisodeFilters ? (
        <>
          <Select
            className="manager-filter-input"
            style={{ minWidth: 210 }}
            mode="multiple"
            maxTagCount="responsive"
            allowClear
            placeholder="全部出场方式"
            value={filters.exitKinds}
            onChange={(value) => onChange({ exitKinds: value })}
            options={exitKinds.map((item) => ({
              value: item.value,
              label: `${item.label}（${item.count}）${item.countedInWinRate ? "" : " · 不计胜率"}`,
            }))}
          />
          <Tooltip title="只保留计入策略胜率的持仓：剔除交易所侧平仓、人工平仓与仍持仓。它们的盈亏仍会出现在净盈亏里。">
            <Checkbox
              checked={filters.strategyOnly}
              onChange={(event) => onChange({ strategyOnly: event.target.checked })}
              style={{ whiteSpace: "nowrap" }}
            >
              仅计入胜率的持仓
            </Checkbox>
          </Tooltip>
        </>
      ) : null}

      {showTimeField ? (
        <Tooltip title="同一批持仓按建仓 / 平仓 / 有过持仓三种口径数出来的条数不同，所以这里必须显式选，返回体也会回显它。">
          <Select
            className="manager-filter-input"
            style={{ minWidth: 220 }}
            value={filters.timeField}
            onChange={(value) => onChange({ timeField: value })}
            options={TIME_FIELD_OPTIONS}
          />
        </Tooltip>
      ) : null}

      <Select
        className="manager-filter-input"
        style={{ minWidth: 200 }}
        value={filters.rangeKey}
        onChange={(value) => onChange({ rangeKey: value })}
        options={RANGE_OPTIONS.map((item) => ({ value: item.key, label: item.label }))}
      />

      <span className="manager-argus-controls__spacer" />
      <Space>
        <Button size="small" onClick={onReset}>
          清空筛选
        </Button>
        <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
          刷新
        </Button>
      </Space>
    </div>
  );
}
