"use client";

import { Descriptions, Typography } from "antd";
import type { MarketTimeline } from "../api/argus-market.api";
import { PLATFORM_LABELS } from "../constants";

const { Text } = Typography;

interface StorageNotesProps {
  timeline: MarketTimeline | null;
}

const pct = (value: number | undefined) => (value == null ? "—" : `${value.toFixed(1)}%`);

/**
 * 存储口径与当前窗口的覆盖率。
 *
 * 放在图旁边不是装饰：这一页看到的"缺口"有两种，一种是行情真没落库（可以回填），
 * 一种是这个周期本来就不常驻（只有 1m/5m/1h/1d 双源常驻，秒级只在触发点 ±1min）。
 * 两者混淆会让人一直点回填却补不出东西。
 */
export function StorageNotes({ timeline }: StorageNotesProps) {
  return (
    <div className="manager-argus-panel">
      <div className="manager-argus-panel__head">
        <span className="manager-argus-panel__title">存储口径与覆盖率</span>
      </div>

      <Descriptions
        column={1}
        size="small"
        items={[
          {
            key: "window",
            label: "当前窗口",
            children: (
              <span className="manager-argus-mono">
                {timeline?.window?.start || "—"} ~ {timeline?.window?.end || "—"}
              </span>
            ),
          },
          {
            key: "resolved",
            label: "窗口来源",
            children:
              timeline?.window?.resolved === "latest-data"
                ? "按已入库数据的最新时刻回推"
                : timeline?.window?.resolved === "explicit"
                  ? "按所选时间范围"
                  : "窗口为空",
          },
          {
            key: "primary",
            label: `${PLATFORM_LABELS[timeline?.platformCode ?? ""] ?? timeline?.platformCode ?? "主源"} 覆盖率`,
            children: (
              <span className="manager-argus-mono">
                {pct(timeline?.coverage?.coveragePct)}（缺 {timeline?.coverage?.missing ?? 0} 根 /
                应有 {timeline?.coverage?.expected ?? 0} 根）
              </span>
            ),
          },
          ...(timeline?.comparePlatformCode
            ? [
                {
                  key: "compare",
                  label: `${PLATFORM_LABELS[timeline.comparePlatformCode] ?? timeline.comparePlatformCode} 覆盖率`,
                  children: (
                    <span className="manager-argus-mono">
                      {pct(timeline.compareCoverage?.coveragePct)}（缺 {timeline.compareCoverage?.missing ?? 0} 根）
                    </span>
                  ),
                },
              ]
            : []),
          { key: "resident", label: "常驻周期", children: "1m / 5m / 1h / 1d（双源）" },
          { key: "slice", label: "秒级切片", children: "仅信号前后 ±1min，90 天滚动保留" },
        ]}
      />

      <Text type="secondary" className="manager-market-note">
        不落全时段秒级行情：单币双源按行存 1s 约 18 GB/年，已否决。历史 1m/5m/1h/1d
        两家都能批量回填——币安 <span className="manager-argus-mono">/fapi/v1/klines</span>、DeepCoin{" "}
        <span className="manager-argus-mono">/deepcoin/market/candles</span>。
      </Text>
    </div>
  );
}
