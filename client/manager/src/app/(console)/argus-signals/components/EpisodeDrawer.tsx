"use client";

import { Alert, Drawer, Skeleton, Table, Tag, Timeline, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { useMemo } from "react";
import type { EpisodeDetail, EpisodeEntry } from "../api/argus-signals.api";
import {
  DETAIL_DRAWER_WIDTH,
  EMPTY,
  EXIT_KIND_COLORS,
  TONE_COLORS,
  fmtDuration,
  fmtInt,
  fmtNum,
  fmtPrice,
  fmtSigned,
  shortTs,
  signColor,
} from "../constants";
import { TrackChart } from "./TrackChart";

const { Text } = Typography;

interface EpisodeDrawerProps {
  open: boolean;
  loading: boolean;
  detail: EpisodeDetail | null;
  error: string;
  onClose: () => void;
}

/**
 * 持仓生命周期抽屉：浮盈轨迹 + 事件时间轴 + trail 峰值与回吐比例 + 出场归因。
 *
 * 两条轨迹分开画且**不互相换算**：ROI% 只在事件里被离散写下来（gate_block /
 * loss_alert / 出场事件），画成虚线加点；upl 来自约 30 秒一条的心跳，单位是 U，
 * 换算成 ROI% 需要假定持仓期间保证金不变——那正是会造出"看着精确其实是拟合"的
 * 那类伪影。
 */
export function EpisodeDrawer({ open, loading, detail, error, onClose }: EpisodeDrawerProps) {
  const episode = detail?.episode;

  const roiPoints = useMemo(
    () => (detail?.roiTrack ?? []).filter((point) => point.kind === "observed").map((point) => ({ ts: point.ts, value: point.roiPct })),
    [detail],
  );
  const uplPoints = useMemo(() => (detail?.uplTrack ?? []).map((point) => ({ ts: point.ts, value: point.upl })), [detail]);

  const entryColumns: ColumnsType<EpisodeEntry> = [
    {
      title: "决策时刻",
      dataIndex: "decidedAt",
      width: 150,
      render: (value: string) => <span className="manager-argus-mono">{shortTs(value)}</span>,
    },
    {
      title: "加仓",
      dataIndex: "addedSize",
      align: "right",
      width: 100,
      render: (value: number, row) => (
        <span className="manager-argus-mono">
          {value} 张{row.orderSize === null ? "" : ` / 委托 ${row.orderSize}`}
        </span>
      ),
    },
    {
      title: "已平 / 未平",
      dataIndex: "closedSize",
      align: "right",
      width: 120,
      render: (value: number, row) => (
        <span className="manager-argus-mono">
          {value} / {row.openSize}
        </span>
      ),
    },
    {
      title: "偏离 bp",
      dataIndex: "gapBp",
      align: "right",
      width: 100,
      render: (value: number | null, row) => (
        <span className="manager-argus-mono" style={{ color: signColor(value) }}>
          {fmtSigned(value, 2)}
          {row.strengthLevel ? ` (${row.strengthLevel})` : ""}
        </span>
      ),
    },
    {
      title: "均价",
      dataIndex: "avgPx",
      align: "right",
      width: 110,
      render: (value: number | null) => <span className="manager-argus-mono">{fmtPrice(value)}</span>,
    },
    {
      title: "归集盈亏",
      dataIndex: "attributedPnl",
      align: "right",
      width: 120,
      render: (value: number, row) => (
        <Tooltip
          title={
            row.pnlKnown
              ? "决策归集口径：这笔已实现盈亏被平摊回它各张仓位建仓时所处的状态。"
              : `本笔决策有 ${row.missingPnlEvents} 条盈亏未知的实现，数值偏小。`
          }
        >
          <span className="manager-argus-mono" style={{ color: signColor(value) }}>
            {fmtSigned(value, 2, " U")}
            {row.pnlKnown ? "" : " ?"}
          </span>
        </Tooltip>
      ),
    },
    {
      title: "版本 / 变体",
      dataIndex: "configVersion",
      width: 200,
      render: (version: number, row) => (
        <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
          v{version}
          {row.variant ? ` · ${row.variant}` : ""}
        </Text>
      ),
    },
  ];

  return (
    <Drawer
      className="manager-form-skin"
      open={open}
      width={DETAIL_DRAWER_WIDTH}
      onClose={onClose}
      title="持仓生命周期（Episode）"
      extra={
        episode ? (
          <Tag bordered={false} className="manager-argus-mono">
            #{episode.episodeId}
          </Tag>
        ) : null
      }
    >
      {error ? <Alert type="error" showIcon message="读取持仓生命周期失败" description={error} /> : null}
      {loading || !detail || !episode ? (
        <Skeleton active paragraph={{ rows: 10 }} />
      ) : (
        <div className="manager-page-stack manager-argus">
          <div className="manager-argus-controls" style={{ marginTop: 0, paddingTop: 0, borderTop: "none" }}>
            <Tag bordered={false} color={episode.side === "long" ? "success" : "error"}>
              {episode.side}
            </Tag>
            <Tag bordered={false} style={{ color: EXIT_KIND_COLORS[episode.exitKind] ?? undefined }}>
              {episode.closedAt ? episode.exitLabel : episode.statusLabel}
            </Tag>
            <Tag bordered={false} className="manager-argus-mono">
              {episode.instanceKey}
            </Tag>
            <Tag bordered={false}>{episode.accountLabel}</Tag>
            <Tag bordered={false} className="manager-argus-mono">
              {episode.variant || EMPTY}
            </Tag>
            <Tag bordered={false} className="manager-argus-mono">
              v{episode.configVersion}
            </Tag>
            <Tooltip title="派生深度：minute = 分钟级心跳可用；alert_sampled = 只有 loss_alert 打出来的几个下界点。">
              <Tag bordered={false}>{episode.depthFidelityLabel || episode.depthFidelity}</Tag>
            </Tooltip>
          </div>

          <div className="manager-argus-tiles manager-argus-tiles--4">
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">峰值张数</div>
              <div className="manager-argus-tile__value">{episode.maxSize}</div>
              <span className="manager-argus-tile__hint">
                建仓 {episode.addCount} 次 / 减仓 {episode.reduceCount} 次 · 持续 {fmtDuration(episode.durationSec)}
              </span>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">trail 峰值浮盈</div>
              <div className="manager-argus-tile__value" style={{ color: "var(--manager-primary)" }}>
                {fmtSigned(episode.peakPct, 1, "%")}
              </div>
              <span className="manager-argus-tile__hint">
                观测极值 {fmtSigned(episode.minRoiPctObserved, 0, "%")} ~ {fmtSigned(episode.maxRoiPctObserved, 0, "%")}
              </span>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">出场 ROI / 回吐比例</div>
              <div className="manager-argus-tile__value" style={{ color: signColor(episode.exitRoiPct) }}>
                {fmtSigned(episode.exitRoiPct, 1, "%")}
              </div>
              <span className="manager-argus-tile__hint">
                {episode.givebackPct === null
                  ? detail.exit.givebackNote || "trail 未激活过，回吐比例无从谈起"
                  : `回吐 ${fmtNum(episode.givebackPct, 1, "%")} = (峰值 − 出场) / 峰值`}
              </span>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">净盈亏</div>
              <div className="manager-argus-tile__value" style={{ color: signColor(episode.pnl) }}>
                {fmtSigned(episode.pnl, 1, " U")}
              </div>
              <span className="manager-argus-tile__hint">
                {episode.pnlKnown
                  ? `${episode.realizedEvents} 次已实现`
                  : `${episode.missingPnlEvents} 条盈亏未知，此数偏小`}
              </span>
            </div>
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">出场归因</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                {episode.openedAt || episode.firstEventAt} → {episode.closedAt || "仍持仓"}
              </Text>
            </div>
            <div className="manager-argus-status__rows">
              <div className="manager-argus-status__row">
                <span>出场方式</span>
                <b style={{ color: EXIT_KIND_COLORS[detail.exit.exitKind] ?? undefined }}>{detail.exit.exitLabel}</b>
              </div>
              <div className="manager-argus-status__row">
                <span>计入策略胜率</span>
                <b>{detail.exit.countedInWinRate ? "是" : "否"}</b>
              </div>
              <div className="manager-argus-status__row">
                <span>判定依据</span>
                <b>{detail.exit.reason || EMPTY}</b>
              </div>
            </div>
            {detail.exit.countedInWinRate ? null : (
              <div className="manager-argus-hint manager-argus-hint--warn">
                <span>
                  <b>归因口径护栏：</b>
                  <span className="manager-argus-mono">external_close</span> /{" "}
                  <span className="manager-argus-mono">manual_close</span>
                  是交易所侧或人工操作，仍持仓的结果未定——这三类<b>不计入策略胜率</b>，否则会把非策略行为算成策略成绩。它们的盈亏仍如实计入净盈亏。
                </span>
              </div>
            )}
            {episode.truncatedHead ? (
              <div className="manager-argus-hint manager-argus-hint--warn">
                <span>
                  建仓决策不在数据窗口内（截断头）：建仓总张数少记了 {episode.hiddenSize} 张，
                  这笔的入场口径不完整，不要拿它做参数对比的样本。
                </span>
              </div>
            ) : null}
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">浮盈 ROI 轨迹（离散观测）</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                回吐比例 = (peakPct − exitRoiPct) / peakPct
              </Text>
            </div>
            <TrackChart
              points={roiPoints}
              color={(episode.exitRoiPct ?? 0) >= 0 ? undefined : "#F6465D"}
              unit="%"
              discrete
              emptyText="这笔持仓期间没有任何 ROI 观测点"
            />
            <div className="manager-argus-hint">
              <span>
                {detail.roiTrackNotice ||
                  "这些是离散观测不是连续曲线：ROI 只在事件里被写下来（gate_block / loss_alert / 出场事件），中间没有任何观测，所以画成虚线加点。"}
              </span>
            </div>
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">浮盈 upl 心跳轨迹（U）</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                {episode.uplSampleCount} 条心跳采样
              </Text>
            </div>
            {detail.uplTrackAvailable ? (
              <TrackChart points={uplPoints} unit="U" emptyText="这段区间没有带 upl 的心跳" />
            ) : (
              <Alert
                type="warning"
                showIcon
                className="manager-argus-alert"
                message="这笔持仓的浮盈轨迹不存在"
                description={
                  detail.uplTrackNotice ||
                  "2026-07-21 之前的心跳没有 upl 字段，那段时间只有 loss_alert 打出来的几个下界点，画不出轨迹。"
                }
              />
            )}
            <div className="manager-argus-hint">
              <span>
                upl 是账户级未实现盈亏，单位 U，<b>刻意不换算成 ROI%</b>——ROI = upl / 保证金，而保证金没有落库，
                靠事件反推需要假定持仓期间保证金不变，那会造出看着精确其实是拟合的曲线。
              </span>
            </div>
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">事件时间轴</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                由 strategy_event 按 episode 状态机切分 · 共 {detail.timeline.length} 条
              </Text>
            </div>
            <Timeline
              className="manager-argus-episode-timeline"
              items={detail.timeline.map((item) => ({
                key: item.eventId,
                color: TONE_COLORS[item.tone] ?? TONE_COLORS.mute,
                children: (
                  <div>
                    <div className="manager-argus-mono" style={{ fontSize: 11, color: "var(--argus-faint)" }}>
                      {item.ts}
                      {item.isDecision ? (
                        <Tag bordered={false} color="processing" style={{ marginInlineStart: 6, fontSize: 11 }}>
                          建仓决策
                        </Tag>
                      ) : null}
                    </div>
                    <div style={{ fontSize: 13, fontWeight: 600 }}>
                      {item.label}
                      {item.orderSize === null ? "" : ` · ${item.orderSize} 张`}
                      {item.netSize === null ? "" : ` · 净仓 ${item.netSize} 张`}
                    </div>
                    <div style={{ fontSize: 12, color: "var(--argus-muted)" }}>
                      {item.gateLabel ? <span className="manager-argus-mono">{item.gateLabel} </span> : null}
                      {item.reason || ""}
                      {item.roiPct === null ? "" : ` ROI ${fmtSigned(item.roiPct, 1, "%")}`}
                      {item.peakPct === null ? "" : ` · 峰值 ${fmtSigned(item.peakPct, 1, "%")}`}
                      {item.pnl === null ? "" : ` · 盈亏 ${fmtSigned(item.pnl, 2, " U")}`}
                      {item.avgPx === null ? "" : ` · 均价 ${fmtPrice(item.avgPx)}`}
                      {item.gapBp === null ? "" : ` · 偏离 ${fmtSigned(item.gapBp, 2)} bp`}
                    </div>
                  </div>
                ),
              }))}
            />
            {detail.eventTruncated ? (
              <div className="manager-argus-hint manager-argus-hint--danger">
                <span>事件条数命中上限，这条时间轴不完整。</span>
              </div>
            ) : null}
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">建仓决策与决策归集盈亏</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                每笔 PnL 平摊回其各张仓位<b>建仓时</b>所处的状态，不按平仓时刻归集
              </Text>
            </div>
            <Table<EpisodeEntry>
              className="manager-table"
              size="small"
              rowKey="entryId"
              columns={entryColumns}
              dataSource={detail.entries}
              pagination={false}
              scroll={{ x: "max-content" }}
            />
            <div className="manager-argus-hint">
              <span>
                拦截计数：上限跳过 {fmtInt(episode.capSkipCount)} · 门控拦截 {fmtInt(episode.gateBlockCount)} · 趋势闸{" "}
                {fmtInt(episode.trendSkipCount)} · 浮亏告警 {fmtInt(episode.lossAlertCount)}
                {episode.hasPositionGap ? ` · 净仓快照缺口 ${episode.positionGapCount} 处` : ""}
              </span>
            </div>
          </div>
        </div>
      )}
    </Drawer>
  );
}
