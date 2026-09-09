"use client";

import { AimOutlined, FilterOutlined, PieChartOutlined, ThunderboltOutlined } from "@ant-design/icons";
import { Empty, Skeleton, Tag, Tooltip, Typography } from "antd";
import type { EpisodeStats, GateStats } from "../api/argus-signals.api";
import { EVENT_COLORS, EXIT_KIND_COLORS, STRENGTH_COLORS, fmtNum, fmtRate, fmtSigned, signColor } from "../constants";

const { Text } = Typography;

interface ReviewStatsProps {
  gateStats: GateStats | null;
  episodeStats: EpisodeStats | null;
  loading: boolean;
}

/**
 * 复盘页顶部的四块 KPI + 三张分布图。
 *
 * 分布图的意义在于回答一句原来根本答不上来的话：「今天最常被什么条件挡住」。
 * 原先 reason 只有一句自由文本（"盈利不足 ROI=-108.7% < 8%"），无法聚合；
 * 落库时拆成 gate_kind / threshold / actual 之后才有下面这张分布。
 */
export function ReviewStats({ gateStats, episodeStats, loading }: ReviewStatsProps) {
  if (loading && !gateStats && !episodeStats) {
    return (
      <div className="manager-argus-panel">
        <Skeleton active paragraph={{ rows: 5 }} />
      </div>
    );
  }

  const blocked = gateStats?.blocked ?? 0;
  const winRate = episodeStats?.winRate ?? null;

  return (
    <>
      <div className="manager-argus-tiles manager-argus-tiles--4">
        <div className="manager-argus-tile">
          <div className="manager-argus-tile__label">
            <ThunderboltOutlined /> 信号触发总数
          </div>
          <div className="manager-argus-tile__value">{gateStats?.totalTriggers ?? 0}</div>
          <span className="manager-argus-tile__hint">
            {gateStats?.window.start || "—"} 至 {gateStats?.window.end || "—"}
          </span>
        </div>

        <div className="manager-argus-tile">
          <div className="manager-argus-tile__label">
            <AimOutlined /> 成功开仓 / 成交率
          </div>
          <div className="manager-argus-tile__value" style={{ color: "var(--manager-success)" }}>
            {gateStats?.opened ?? 0}
            <span style={{ fontSize: 15, color: "var(--argus-faint)", marginInlineStart: 8 }}>
              {fmtRate(gateStats?.openRate)}
            </span>
          </div>
          <span className="manager-argus-tile__hint">其余 {blocked} 条被条件挡住</span>
        </div>

        <div className="manager-argus-tile">
          <div className="manager-argus-tile__label">
            <PieChartOutlined /> 持仓 episode
          </div>
          <div className="manager-argus-tile__value">
            {episodeStats?.closed ?? 0}
            <span style={{ fontSize: 15, color: "var(--argus-faint)", marginInlineStart: 8 }}>
              / 共 {episodeStats?.total ?? 0}
            </span>
          </div>
          <span className="manager-argus-tile__hint">
            另有 {episodeStats?.stillOpen ?? 0} 笔仍持仓
            {episodeStats?.truncatedHead ? ` · ${episodeStats.truncatedHead} 笔建仓在窗口之前` : ""}
          </span>
        </div>

        <div className="manager-argus-tile">
          <div className="manager-argus-tile__label">
            <FilterOutlined /> 策略胜率 / 净盈亏
          </div>
          <div className="manager-argus-tile__value">
            {winRate === null ? "—" : fmtNum(winRate, 1, "%")}
            <span
              style={{ fontSize: 15, marginInlineStart: 8, color: signColor(episodeStats?.pnl) ?? "var(--argus-faint)" }}
            >
              {fmtSigned(episodeStats?.pnl, 1, " U")}
            </span>
          </div>
          <span className="manager-argus-tile__hint">
            胜率分母是 {episodeStats?.attributable ?? 0} 笔计入策略的持仓；
            {episodeStats?.excluded ?? 0} 笔交易所侧 / 人工出场不计入胜率，盈亏仍计入。
          </span>
        </div>
      </div>

      <div className="manager-argus-view--split">
        <div className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title">
              <span className="manager-argus-panel__icon">
                <FilterOutlined />
              </span>
              拦截原因分布
            </div>
            <Text type="secondary" style={{ fontSize: 12.5 }}>
              结构化字段：gate_kind + threshold + actual
            </Text>
          </div>

          {(gateStats?.byGate ?? []).filter((item) => item.count > 0).length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这段窗口内没有被挡住的触发" />
          ) : (
            (gateStats?.byGate ?? [])
              .filter((item) => item.count > 0)
              .map((item) => (
                <div className="manager-argus-bar" key={item.gateKind}>
                  <div className="manager-argus-bar__label">
                    <div>{item.label}</div>
                    <div className="manager-argus-bar__detail manager-argus-mono">
                      {item.avgActual === null && item.avgThreshold === null
                        ? item.sampleReason || item.gateKind
                        : `均值 ${fmtNum(item.avgActual, 2)} vs 阈值 ${fmtNum(item.avgThreshold, 2)}`}
                    </div>
                  </div>
                  <div className="manager-argus-bar__track">
                    <div
                      className="manager-argus-bar__fill"
                      style={{
                        width: `${blocked > 0 ? (item.count / blocked) * 100 : 0}%`,
                        background: EVENT_COLORS[item.gateKind] ?? "var(--manager-primary)",
                      }}
                    />
                  </div>
                  <Tooltip title={item.sampleReason || undefined}>
                    <span className="manager-argus-mono manager-argus-bar__value">{item.count}</span>
                  </Tooltip>
                </div>
              ))
          )}

          <div className="manager-argus-hint">
            <span>
              原先只有一句自由文本 <span className="manager-argus-mono">「盈利不足 ROI=-108.7% &lt; 8%」</span>
              ，无法聚合。落库时拆成 <b>gate_kind / threshold / actual</b> 之后，这张图才回答得了「今天最常被什么挡住」。
            </span>
          </div>
        </div>

        <div className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <div className="manager-argus-panel__title">
              <span className="manager-argus-panel__icon">
                <ThunderboltOutlined />
              </span>
              信号强度分级
            </div>
            <Text type="secondary" style={{ fontSize: 12.5 }}>
              按 |gap_bp| 分档，看「强信号是不是更容易成交」
            </Text>
          </div>

          {(gateStats?.byStrength ?? []).map((item) => (
            <div className="manager-argus-bar" key={item.level}>
              <div className="manager-argus-bar__label">
                <div>{item.label}</div>
                <div className="manager-argus-bar__detail">
                  开仓 {item.opened} / {item.count} · 成交率 {fmtRate(item.openRate)}
                </div>
              </div>
              <div className="manager-argus-bar__track">
                <div
                  className="manager-argus-bar__fill"
                  style={{
                    width: `${
                      gateStats && gateStats.totalTriggers > 0 ? (item.count / gateStats.totalTriggers) * 100 : 0
                    }%`,
                    background: STRENGTH_COLORS[item.level] ?? "var(--manager-primary)",
                  }}
                />
              </div>
              <span className="manager-argus-mono manager-argus-bar__value">{item.count}</span>
            </div>
          ))}

          <div className="manager-argus-hint manager-argus-hint--warn">
            <span>
              gap_bp 被生产 <span className="manager-argus-mono">signal_threshold</span> 结构性截断（实测最小
              5.01 bp），分布主体在这里看不到。完整分布要看无条件采样的{" "}
              <span className="manager-argus-mono">dev_sample</span>，不是本表。
            </span>
          </div>
        </div>
      </div>

      <div className="manager-argus-panel">
        <div className="manager-argus-panel__head">
          <div className="manager-argus-panel__title">
            <span className="manager-argus-panel__icon">
              <PieChartOutlined />
            </span>
            出场归因分布
          </div>
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            窗口口径：{episodeStats?.timeField === "closed" ? "按平仓时刻" : episodeStats?.timeField === "overlap" ? "按窗口内有过持仓" : "按建仓时刻（决策归集）"}
          </Text>
        </div>

        {(episodeStats?.byExitKind ?? []).length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这段窗口内没有派生出持仓" />
        ) : (
          (episodeStats?.byExitKind ?? []).map((item) => (
            <div className="manager-argus-bar" key={item.exitKind || "open"}>
              <div className="manager-argus-bar__label">
                <div>
                  {item.label}
                  {item.countedInWinRate ? null : (
                    <Tag bordered={false} style={{ marginInlineStart: 6, fontSize: 11 }}>
                      不计胜率
                    </Tag>
                  )}
                </div>
                <div className="manager-argus-bar__detail">
                  盈亏 <span style={{ color: signColor(item.pnl) }}>{fmtSigned(item.pnl, 1, " U")}</span>
                  {item.winRate === null ? "" : ` · 胜率 ${fmtNum(item.winRate, 1, "%")}（${item.wins} 胜）`}
                </div>
              </div>
              <div className="manager-argus-bar__track">
                <div
                  className="manager-argus-bar__fill"
                  style={{
                    width: `${item.share}%`,
                    background: EXIT_KIND_COLORS[item.exitKind] ?? "var(--manager-primary)",
                  }}
                />
              </div>
              <span className="manager-argus-mono manager-argus-bar__value">{item.count}</span>
            </div>
          ))
        )}

        <div className="manager-argus-hint manager-argus-hint--warn">
          <span>
            <b>归因口径护栏：</b>
            <span className="manager-argus-mono">external_close</span> 与{" "}
            <span className="manager-argus-mono">manual_close</span>
            是交易所侧或人工操作，<b>不计入策略胜率</b>——否则会把非策略行为记成策略成绩；它们的盈亏如实计入净盈亏，不做隐藏过滤。仍持仓的结果未定，同样不进胜率。
          </span>
        </div>
        {episodeStats && episodeStats.incompletePnl > 0 ? (
          <div className="manager-argus-hint manager-argus-hint--danger">
            <span>
              {episodeStats.incompletePnl} 笔持仓期间有盈亏未知的实现（2026-07-24 之前的反向减仓只写 size 不写
              pnl），净盈亏偏小且偏差方向未知。
            </span>
          </div>
        ) : null}
      </div>
    </>
  );
}
