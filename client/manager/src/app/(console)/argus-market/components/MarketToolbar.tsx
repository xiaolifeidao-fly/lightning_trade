"use client";

import { ReloadOutlined, ThunderboltOutlined } from "@ant-design/icons";
import { Button, Segmented, Select, Space, Tooltip, Typography } from "antd";
import type { SignalEvent } from "../api/argus-market.api";
import {
  MARKET_INTERVALS,
  PLATFORM_LABELS,
  RANGE_OPTIONS,
  TRIGGER_KINDS,
  type MarketInterval,
  type MarketSource,
  type TriggerKind,
} from "../constants";
import type { MarketFilters } from "../hooks/useArgusMarket";

const { Text } = Typography;

interface MarketToolbarProps {
  instances: { instanceKey: string; instanceName: string }[];
  instanceKey: string;
  onInstanceChange: (key: string) => void;
  instruments: string[];
  filters: MarketFilters;
  onFiltersChange: (patch: Partial<MarketFilters>) => void;
  onToggleKind: (kind: TriggerKind) => void;
  /** 窗口内的全部触发点，用于每类的计数徽标（不受勾选影响）。 */
  triggers: SignalEvent[];
  thresholdBp: number | null;
  positionCap: number | null;
  loading: boolean;
  backfilling: boolean;
  onRefresh: () => void;
  onBackfill: () => void;
}

/**
 * 行情主视图的筛选条。
 *
 * 实例选择器放在最前面且没有「全部实例」选项：阈值线、上限线、净持仓阶梯全都是
 * 实例内的量，混着看等于把 champion / challenger 的对照组画进同一张图。
 */
export function MarketToolbar({
  instances,
  instanceKey,
  onInstanceChange,
  instruments,
  filters,
  onFiltersChange,
  onToggleKind,
  triggers,
  thresholdBp,
  positionCap,
  loading,
  backfilling,
  onRefresh,
  onBackfill,
}: MarketToolbarProps) {
  const countOf = (kind: TriggerKind) => triggers.filter((item) => item.event === kind).length;

  return (
    <div className="manager-argus-panel manager-market-toolbar">
      <div className="manager-market-toolbar__row">
        <label className="manager-market-field">
          <span className="manager-market-field__label">实例</span>
          <Select
            value={instanceKey || undefined}
            placeholder="请选择实例"
            style={{ minWidth: 240 }}
            options={instances.map((item) => ({
              value: item.instanceKey,
              label: item.instanceName ? `${item.instanceName}（${item.instanceKey}）` : item.instanceKey,
            }))}
            onChange={onInstanceChange}
          />
        </label>

        <label className="manager-market-field">
          <span className="manager-market-field__label">币种</span>
          <Select
            value={filters.instrument || undefined}
            style={{ minWidth: 140 }}
            options={(instruments.length > 0 ? instruments : [filters.instrument || "BTCUSDT"]).map((item) => ({
              value: item,
              label: item,
            }))}
            onChange={(value) => onFiltersChange({ instrument: value })}
          />
        </label>

        <label className="manager-market-field">
          <span className="manager-market-field__label">周期</span>
          <Segmented
            value={filters.interval}
            options={MARKET_INTERVALS.map((item) => ({ value: item, label: item }))}
            onChange={(value) => onFiltersChange({ interval: value as MarketInterval })}
          />
        </label>

        <label className="manager-market-field">
          <span className="manager-market-field__label">数据源</span>
          <Segmented
            value={filters.source}
            options={[
              { value: "binance", label: PLATFORM_LABELS.binance },
              { value: "deepcoin", label: PLATFORM_LABELS.deepcoin },
              { value: "both", label: "双源对比" },
            ]}
            onChange={(value) => onFiltersChange({ source: value as MarketSource })}
          />
        </label>

        <label className="manager-market-field">
          <span className="manager-market-field__label">时间范围</span>
          <Select
            value={filters.rangeKey}
            style={{ minWidth: 210 }}
            options={RANGE_OPTIONS.map((item) => ({ value: item.key, label: item.label }))}
            onChange={(value) => onFiltersChange({ rangeKey: value })}
          />
        </label>

        <div className="manager-market-toolbar__actions">
          <Space size={8}>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
              刷新
            </Button>
            <Tooltip title="按当前视图窗口的覆盖率补齐两个数据源的 K 线，只写 trade_kline，不碰实盘">
              <Button icon={<ThunderboltOutlined />} loading={backfilling} onClick={onBackfill}>
                回填缺口
              </Button>
            </Tooltip>
          </Space>
        </div>
      </div>

      <div className="manager-market-toolbar__row manager-market-toolbar__row--tight">
        <span className="manager-market-field__label">触发点类型</span>
        {TRIGGER_KINDS.map((kind) => {
          const on = filters.kinds[kind.key];
          return (
            <button
              key={kind.key}
              type="button"
              className={`manager-market-kind ${on ? "" : "manager-market-kind--off"}`}
              onClick={() => onToggleKind(kind.key)}
            >
              <i className="manager-market-kind__dot" style={{ background: kind.color }} />
              {kind.label}
              <b className="manager-market-kind__count">{countOf(kind.key)}</b>
            </button>
          );
        })}

        <span className="manager-market-toolbar__spacer" />
        <Text className="manager-market-hint">
          信号阈值 {thresholdBp == null ? "读取中 / 该实例未发布配置" : `±${thresholdBp.toFixed(1)} bp`}
        </Text>
        <Text className="manager-market-hint">净持仓上限 {positionCap == null ? "—" : `${positionCap} 张`}</Text>
      </div>
    </div>
  );
}
