"use client";

import { Button, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { Episode } from "../api/argus-signals.api";
import {
  EMPTY,
  EPISODE_PAGE_SIZE,
  EXIT_KIND_COLORS,
  fmtDuration,
  fmtNum,

  fmtSigned,
  shortTs,
  signColor,
} from "../constants";

const { Text } = Typography;

interface EpisodeTableProps {
  rows: Episode[];
  total: number;
  page: number;
  loading: boolean;
  crossInstance: boolean;
  onPageChange: (page: number) => void;
  onOpenDetail: (row: Episode) => void;
}

/** 持仓 episode 列表。一行 = 一次完整持仓（从建仓到归零），由 r10 离线派生。 */
export function EpisodeTable({ rows, total, page, loading, crossInstance, onPageChange, onOpenDetail }: EpisodeTableProps) {
  const columns: ColumnsType<Episode> = [
    {
      title: "Episode",
      dataIndex: "episodeId",
      width: 110,
      render: (id: number, row) => (
        <div>
          <span className="manager-argus-mono">#{id}</span>
          {row.truncatedHead ? (
            <Tooltip title={`建仓决策不在数据窗口内，建仓总张数少记了 ${row.hiddenSize} 张`}>
              <Tag bordered={false} color="warning" style={{ marginInlineStart: 6, fontSize: 11 }}>
                截断头
              </Tag>
            </Tooltip>
          ) : null}
        </div>
      ),
    },
    ...(crossInstance
      ? ([
          {
            title: "实例",
            dataIndex: "instanceKey",
            width: 150,
            render: (key: string) => (
              <Tag bordered={false} className="manager-argus-mono" style={{ fontSize: 11 }}>
                {key || EMPTY}
              </Tag>
            ),
          },
        ] as ColumnsType<Episode>)
      : []),
    {
      title: "账户 / 变体",
      dataIndex: "accountLabel",
      width: 180,
      render: (label: string, row) => (
        <div>
          <div>{label || EMPTY}</div>
          <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
            {row.variant || row.uid || EMPTY} · v{row.configVersion}
          </Text>
        </div>
      ),
    },
    {
      title: "方向",
      dataIndex: "side",
      width: 80,
      render: (side: string) => (
        <Tag bordered={false} color={side === "long" ? "success" : "error"}>
          {side || EMPTY}
        </Tag>
      ),
    },
    {
      title: "建仓时刻",
      dataIndex: "openedAt",
      width: 150,
      render: (openedAt: string, row) => (
        <span className="manager-argus-mono">{shortTs(openedAt || row.firstEventAt)}</span>
      ),
    },
    {
      title: "峰值张数",
      dataIndex: "maxSize",
      align: "right",
      width: 100,
      render: (value: number, row) => (
        <Tooltip title={`建仓 ${row.addCount} 次 / 减仓 ${row.reduceCount} 次 · 建仓总张数 ${row.entrySizeTotal}`}>
          <span className="manager-argus-mono">{value}</span>
        </Tooltip>
      ),
    },
    {
      title: "峰值浮盈",
      dataIndex: "peakPct",
      align: "right",
      width: 110,
      render: (value: number | null) => (
        <Tooltip title="trail 激活后由平仓事件带回的峰值 ROI%；trail 没激活过时为空。">
          <span className="manager-argus-mono" style={{ color: "var(--manager-primary)" }}>
            {fmtSigned(value, 1, "%")}
          </span>
        </Tooltip>
      ),
    },
    {
      title: "出场 ROI",
      dataIndex: "exitRoiPct",
      align: "right",
      width: 130,
      render: (value: number | null, row) => (
        <div>
          <span className="manager-argus-mono" style={{ color: signColor(value) }}>
            {fmtSigned(value, 1, "%")}
          </span>
          {row.givebackPct === null ? null : (
            <div className="manager-argus-mono" style={{ fontSize: 11, color: "var(--argus-faint)" }}>
              回吐 {fmtNum(row.givebackPct, 0, "%")}
            </div>
          )}
        </div>
      ),
    },
    {
      title: "净盈亏",
      dataIndex: "pnl",
      align: "right",
      width: 110,
      render: (value: number, row) => (
        <Tooltip title={row.pnlKnown ? undefined : "期间有盈亏未知的实现，这个数偏小且偏差方向未知"}>
          <span className="manager-argus-mono" style={{ color: signColor(value) }}>
            {fmtSigned(value, 1, " U")}
            {row.pnlKnown ? "" : " ?"}
          </span>
        </Tooltip>
      ),
    },
    {
      title: "出场方式",
      dataIndex: "exitLabel",
      width: 150,
      render: (label: string, row) => (
        <div>
          <Tag bordered={false} style={{ color: EXIT_KIND_COLORS[row.exitKind] ?? undefined }}>
            {row.closedAt ? label : row.statusLabel}
          </Tag>
          {row.strategyAttributable ? null : (
            <Tooltip title="交易所侧 / 人工出场或仍持仓，不计入策略胜率；盈亏仍计入净盈亏。">
              <Tag bordered={false} style={{ fontSize: 11 }}>
                不计胜率
              </Tag>
            </Tooltip>
          )}
        </div>
      ),
    },
    {
      title: "持续",
      dataIndex: "durationSec",
      width: 100,
      render: (value: number | null) => (
        <span className="manager-argus-mono" style={{ color: "var(--argus-faint)" }}>
          {fmtDuration(value)}
        </span>
      ),
    },
    {
      title: "剩余张数",
      dataIndex: "openSize",
      align: "right",
      width: 100,
      render: (value: number) => (
        <span className="manager-argus-mono" style={{ color: value > 0 ? "var(--manager-primary)" : "var(--argus-faint)" }}>
          {value > 0 ? value : EMPTY}
        </span>
      ),
    },
    {
      title: "",
      dataIndex: "episodeId",
      key: "action",
      width: 100,
      align: "right",
      render: (_: number, row) => (
        <Button size="small" onClick={() => onOpenDetail(row)}>
          生命周期
        </Button>
      ),
    },
  ];

  return (
    <Table<Episode>
      className="manager-table"
      size="small"
      rowKey="episodeId"
      loading={loading}
      columns={columns}
      dataSource={rows}
      scroll={{ x: "max-content" }}
      onRow={(row) => ({ onClick: () => onOpenDetail(row), style: { cursor: "pointer" } })}
      pagination={{
        current: page,
        pageSize: EPISODE_PAGE_SIZE,
        total,
        showSizeChanger: false,
        onChange: onPageChange,
        showTotal: (value) => `共 ${value} 笔持仓`,
      }}
    />
  );
}
