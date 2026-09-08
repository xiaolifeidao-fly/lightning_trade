"use client";

import { Alert, Drawer, Skeleton, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { AccountDecision, SignalDetail } from "../api/argus-signals.api";
import {
  DETAIL_DRAWER_WIDTH,
  EMPTY,
  EVENT_COLORS,
  fmtInt,
  fmtNum,
  fmtPrice,
  fmtSigned,
  signColor,
} from "../constants";

const { Text } = Typography;

interface SignalDetailDrawerProps {
  open: boolean;
  loading: boolean;
  detail: SignalDetail | null;
  error: string;
  onClose: () => void;
}

/**
 * 信号详情抽屉：把一次触发在各账户上的判定摊成一张表，还原 Telegram 消息里的
 * `[n]`（成交）与 `[跳过n]`（被挡住）结构。
 *
 * 这一屏取代的就是「每周导出 TG 历史消息交给 AI 读」那套流程——同样的信息
 * 全部来自 strategy_event，可筛选、可聚合、可回测。
 */
export function SignalDetailDrawer({ open, loading, detail, error, onClose }: SignalDetailDrawerProps) {
  const columns: ColumnsType<AccountDecision> = [
    {
      title: "账户",
      dataIndex: "accountLabel",
      width: 170,
      render: (label: string, row) => (
        <div>
          <div>{label || EMPTY}</div>
          <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
            {row.uid || EMPTY}
            {row.variant ? ` · ${row.variant}` : ""}
          </Text>
        </div>
      ),
    },
    {
      title: "结果",
      dataIndex: "resultLabel",
      width: 140,
      render: (label: string, row) => (
        <div>
          <Tag bordered={false} style={{ color: EVENT_COLORS[row.result] ?? undefined }}>
            {label || row.result}
          </Tag>
          {row.tsOffsetSec !== 0 ? (
            <Tooltip title="相对锚点事件的秒偏移：成交类要等下单往返，实测会比拦截类晚几秒。">
              <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
                {fmtSigned(row.tsOffsetSec, 0, "s")}
              </Text>
            </Tooltip>
          ) : null}
        </div>
      ),
    },
    {
      title: "张数",
      dataIndex: "orderSize",
      align: "right",
      width: 90,
      render: (value: number | null, row) => (
        <span className="manager-argus-mono">
          {fmtInt(value)}
          {row.netSize === null ? "" : ` / 净 ${row.netSize}`}
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
      title: "成交",
      dataIndex: "lastPx",
      align: "right",
      width: 110,
      render: (value: number | null) => <span className="manager-argus-mono">{fmtPrice(value)}</span>,
    },
    {
      title: "说明",
      dataIndex: "reason",
      render: (reason: string, row) =>
        row.resultKind === "open" ? (
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            正常成交
          </Text>
        ) : (
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            <span className="manager-argus-mono">{row.gateLabel || row.gateKind || EMPTY}</span>{" "}
            {row.gateActual === null && row.gateThreshold === null
              ? reason
              : `实测 ${fmtSigned(row.gateActual, 2)} vs 阈值 ${fmtSigned(row.gateThreshold, 2)}`}
            {row.roiPct === null ? "" : ` · ROI ${fmtSigned(row.roiPct, 1, "%")}`}
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
      title="盘口信号详情"
      extra={
        detail ? (
          <Tag bordered={false} className="manager-argus-mono">
            #{detail.signalId}
          </Tag>
        ) : null
      }
    >
      {error ? <Alert type="error" showIcon message="读取信号详情失败" description={error} /> : null}
      {loading || !detail ? (
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : (
        <div className="manager-page-stack manager-argus">
          <div className="manager-argus-controls" style={{ marginTop: 0, paddingTop: 0, borderTop: "none" }}>
            <Tag bordered={false} className="manager-argus-mono">
              {detail.ts}
            </Tag>
            <Tag bordered={false} className="manager-argus-mono">
              {detail.instanceKey}
            </Tag>
            <Tag bordered={false}>{detail.instrument}</Tag>
            {detail.direction ? (
              <Tag bordered={false} color={detail.direction === "UP" ? "success" : "error"}>
                {detail.direction} → {detail.side || EMPTY}
              </Tag>
            ) : null}
            {detail.configVersions.map((version) => (
              <Tag bordered={false} className="manager-argus-mono" key={version}>
                v{version}
              </Tag>
            ))}
          </div>

          <div className="manager-argus-tiles manager-argus-tiles--4">
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">参考价 / 标记价</div>
              <div className="manager-argus-tile__value manager-argus-tile__value--sm">
                {fmtPrice(detail.sigLast)} / {fmtPrice(detail.sigMark)}
              </div>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">偏离 gap_bp</div>
              <div className="manager-argus-tile__value" style={{ color: signColor(detail.gapBp) }}>
                {fmtSigned(detail.gapBp, 2)}
              </div>
              <span className="manager-argus-tile__hint">
                强度分档 {detail.strengthLevel || EMPTY}
                {detail.trendMomPct === null ? "" : ` · 24h 动量 ${fmtNum(detail.trendMomPct, 2, "%")}`}
              </span>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">成交账户</div>
              <div className="manager-argus-tile__value" style={{ color: "var(--manager-success)" }}>
                {detail.openedCount}
              </div>
              <span className="manager-argus-tile__hint">共下单 {detail.totalOrderQty} 张</span>
            </div>
            <div className="manager-argus-tile">
              <div className="manager-argus-tile__label">被拦截账户</div>
              <div className="manager-argus-tile__value" style={{ color: "var(--manager-primary)" }}>
                {detail.blockedCount}
              </div>
              <span className="manager-argus-tile__hint">该次触发共判定 {detail.accountCount} 个账户</span>
            </div>
          </div>

          <div className="manager-argus-panel">
            <div className="manager-argus-panel__head">
              <div className="manager-argus-panel__title">逐账户判定</div>
              <Text type="secondary" style={{ fontSize: 12.5 }}>
                对应 TG 消息里的 [n] / [跳过n] 明细行 · 归组窗口 {detail.groupWindowSec} 秒
              </Text>
            </div>
            <Table<AccountDecision>
              className="manager-table"
              size="small"
              rowKey="eventId"
              columns={columns}
              dataSource={detail.accounts}
              pagination={false}
              scroll={{ x: "max-content" }}
            />
            {detail.fillPriceAvailable ? null : (
              <div className="manager-argus-hint manager-argus-hint--warn">
                <span>
                  成交价与委托均价来自交易所下单回执，埋点没有落库，所以开仓行的这两列恒为「—」。
                  <b>不要把它读成 0</b>——那是「没有这个观测」，不是「成交在 0」。
                </span>
              </div>
            )}
            {detail.notice ? (
              <div className="manager-argus-hint">
                <span>{detail.notice}</span>
              </div>
            ) : null}
          </div>

          {detail.telegramLines.length > 0 ? (
            <div className="manager-argus-panel">
              <div className="manager-argus-panel__head">
                <div className="manager-argus-panel__title">还原的 Telegram 消息</div>
                <Text type="secondary" style={{ fontSize: 12.5 }}>
                  按原格式逐行还原，用来核对「页面和当时那条消息说的是不是一回事」
                </Text>
              </div>
              <pre className="manager-argus-log manager-argus-tglines">{detail.telegramLines.join("\n")}</pre>
            </div>
          ) : null}

          <div className="manager-argus-hint">
            <span>
              这一屏取代原先「导出 TG 历史消息 → 交给 AI 读」的流程：同样的信息全部来自{" "}
              <span className="manager-argus-mono">strategy_event</span>，可筛选、可聚合、可回测。
            </span>
          </div>
        </div>
      )}
    </Drawer>
  );
}
