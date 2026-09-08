"use client";

import { Alert, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { SliceCompare, SliceCompareRow } from "../api/argus-signals.api";
import { EMPTY, fmtNum, fmtSigned, shortTs, signColor } from "../constants";

const { Text } = Typography;

interface SliceCompareTableProps {
  data: SliceCompare | null;
  loading: boolean;
}

/**
 * 按 (实例, 参数版本) 切片的横向对比：换了参数之后有没有变化。
 *
 * 两条口径写在表上而不是只写在代码里：
 *  1. 信号侧按事件自带的 config_version 归属；持仓侧按 episode 的 config_version
 *     归属，那是**开仓时刻**的版本。跨越发布点的持仓算在做建仓决策的那个版本上。
 *  2. 跨实例的行不能相加——三实例阈值 5/3bp、上限 15/26+8/246、下单 1/10 全不同。
 */
export function SliceCompareTable({ data, loading }: SliceCompareTableProps) {
  const columns: ColumnsType<SliceCompareRow> = [
    {
      title: "实例 / 参数版本",
      dataIndex: "configVersion",
      width: 220,
      fixed: "left",
      render: (version: number, row) => (
        <div>
          <div>
            {row.instanceName || row.instanceKey}
            <Tag bordered={false} className="manager-argus-mono" style={{ marginInlineStart: 6 }}>
              v{version}
            </Tag>
          </div>
          <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
            {row.instanceKey}
            {row.registered ? "" : " · 未注册"}
          </Text>
        </div>
      ),
    },
    {
      title: "变体",
      dataIndex: "variants",
      width: 180,
      render: (variants: string[]) => (
        <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
          {variants?.length ? variants.join(" / ") : EMPTY}
        </Text>
      ),
    },
    {
      title: "信号",
      dataIndex: "signals",
      align: "right",
      width: 90,
      render: (value: number) => <span className="manager-argus-mono">{value}</span>,
    },
    {
      title: "成交 / 成交率",
      dataIndex: "opened",
      align: "right",
      width: 130,
      render: (value: number, row) => (
        <span className="manager-argus-mono">
          {value} · {fmtNum(row.openRate, 1, "%")}
        </span>
      ),
    },
    {
      title: "拦截构成",
      dataIndex: "capSkip",
      width: 190,
      render: (_: number, row) => (
        <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
          上限 {row.capSkip} · 门控 {row.gateBlock} · 趋势 {row.trendSkip}
        </Text>
      ),
    },
    {
      title: "均值 gap_bp",
      dataIndex: "avgGapBp",
      align: "right",
      width: 110,
      render: (value: number | null) => <span className="manager-argus-mono">{fmtNum(value, 2)}</span>,
    },
    {
      title: "强度构成",
      dataIndex: "weak",
      width: 160,
      render: (_: number, row) => (
        <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
          弱 {row.weak} · 中 {row.medium} · 强 {row.strong}
        </Text>
      ),
    },
    {
      title: "持仓 / 已出场",
      dataIndex: "episodes",
      align: "right",
      width: 130,
      render: (value: number, row) =>
        row.episodeAvailable ? (
          <span className="manager-argus-mono">
            {value} / {row.closedEpisodes}
          </span>
        ) : (
          <Tooltip title="这个切片没有派生出 episode：可能是这段确实没有持仓，也可能是还没跑重建（看下方派生新鲜度）。">
            <span className="manager-argus-mono" style={{ color: "var(--argus-faint)" }}>
              {EMPTY}
            </span>
          </Tooltip>
        ),
    },
    {
      title: "策略胜率",
      dataIndex: "winRate",
      align: "right",
      width: 130,
      render: (value: number | null, row) => (
        <Tooltip title={`分母是计入策略胜率的 ${row.attributableEpisodes} 笔，不是全部持仓`}>
          <span className="manager-argus-mono">
            {value === null ? EMPTY : fmtNum(value, 1, "%")}
            <span style={{ color: "var(--argus-faint)" }}> ({row.wins}/{row.attributableEpisodes})</span>
          </span>
        </Tooltip>
      ),
    },
    {
      title: "净盈亏 / 策略盈亏",
      dataIndex: "pnl",
      align: "right",
      width: 160,
      render: (value: number, row) => (
        <span className="manager-argus-mono">
          <span style={{ color: signColor(value) }}>{fmtSigned(value, 1, " U")}</span>
          <span style={{ color: "var(--argus-faint)" }}> / {fmtSigned(row.pnlStrategy, 1)}</span>
        </span>
      ),
    },
    {
      title: "均值峰值 / 出场 ROI",
      dataIndex: "avgPeakPct",
      align: "right",
      width: 170,
      render: (value: number | null, row) => (
        <span className="manager-argus-mono">
          {fmtNum(value, 1, "%")} / {fmtNum(row.avgExitRoiPct, 1, "%")}
        </span>
      ),
    },
    {
      title: "覆盖区间",
      dataIndex: "firstTs",
      width: 190,
      render: (value: string, row) => (
        <Text type="secondary" className="manager-argus-mono" style={{ fontSize: 11 }}>
          {shortTs(value)} ~ {shortTs(row.lastTs)}
        </Text>
      ),
    },
  ];

  return (
    <>
      {data?.notice ? (
        <Alert className="manager-argus-alert" type="warning" showIcon message="跨实例不可比" description={data.notice} />
      ) : null}
      {data?.signalTruncated ? (
        <Alert
          className="manager-argus-alert"
          type="error"
          showIcon
          message="信号侧扫描命中行数上限"
          description="这张表只覆盖窗口内的一部分事件，请缩小时间范围后重看，不要按当前数字下结论。"
        />
      ) : null}
      <Table<SliceCompareRow>
        className="manager-table"
        size="small"
        rowKey={(row) => `${row.instanceKey}#${row.configVersion}`}
        loading={loading}
        columns={columns}
        dataSource={data?.rows ?? []}
        pagination={false}
        scroll={{ x: "max-content" }}
      />
      <div className="manager-argus-hint">
        <span>
          信号侧按事件自带的 <span className="manager-argus-mono">config_version</span> 归属；持仓侧按 episode 的
          版本归属，那是<b>开仓时刻</b>的版本——一条跨越发布点的持仓算在做建仓决策的那个版本上，
          按平仓时刻归集会造出「高波动期收益是低波动期 8–11 倍」那类伪影。
        </span>
      </div>
    </>
  );
}
