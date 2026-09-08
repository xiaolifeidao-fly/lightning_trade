"use client";

import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  ExperimentOutlined,
  InfoCircleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltFilled,
} from "@ant-design/icons";
import { useRouter } from "next/navigation";
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Input,
  Progress,
  Row,
  Select,
  Skeleton,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useMemo, useState } from "react";
import type { OptimizeCell } from "./api/argus-optimizer.api";
import { TradeoffFrontier } from "./components/TradeoffFrontier";
import { useOptimizer } from "./hooks/useOptimizer";

const { Text, Title } = Typography;

const PARAM_TO_CONFIG_KEY: Record<string, string> = {
  signalThresholdBp: "signal_threshold",
  budgetPct: "risk_budget",
  catastropheStopPct: "catastrophic_stop_loss",
  ceiling: "max_contracts",
  orderSize: "default_order_size",
  gateMinProfitPct: "reverse_gate_min_profit_pct",
  trendGateWindowHours: "trend_gate_window_hours",
  trendGateThresholdPct: "trend_gate_threshold_pct",
  smallActivatePct: "trail_small_activate",
  smallGiveback: "trail_small_giveback",
  mediumActivatePct: "trail_medium_activate",
  mediumGiveback: "trail_medium_giveback",
  largeActivatePct: "trail_large_activate",
  largeGiveback: "trail_large_giveback",
  tierSmallRatio: "trail_tier_small_ratio",
  tierLargeRatio: "trail_tier_large_ratio",
};

function sourceLabel(instanceKey: string, accountLabel: string, instrument: string) {
  return `${instanceKey} · ${accountLabel} · ${instrument}`;
}

function asNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function asText(value: unknown): string {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map(asText).join(" · ");
  return "—";
}

function mapEntries(value: Record<string, unknown> | null | undefined): { key: string; value: string }[] {
  return Object.entries(value ?? {}).map(([key, item]) => ({ key, value: asText(item) }));
}

function gateColor(passed: boolean) {
  return passed ? "green" : "red";
}

function GateResult({ passed, label, value, threshold }: { passed: boolean; label: string; value: string; threshold: string }) {
  return (
    <Tooltip title={`观测值 ${value}；预注册阈值 ${threshold}`}>
      <Tag color={gateColor(passed)} icon={passed ? <CheckCircleOutlined /> : <CloseCircleOutlined />}>{label}</Tag>
    </Tooltip>
  );
}

export default function ArgusOptimizerPage() {
  const router = useRouter();
  const [studyName, setStudyName] = useState("");
  const {
    sources,
    source,
    selectedSourceKey,
    selectSource,
    bootstrapping,
    bootError,
    defaults,
    defaultsError,
    start,
    setStart,
    end,
    setEnd,
    symbol,
    setSymbol,
    platformCode,
    setPlatformCode,
    studies,
    studyId,
    detail,
    detailLoading,
    detailError,
    selectStudy,
    refreshDetail,
    submitting,
    submit,
  } = useOptimizer();

  const gates = detail?.gates ?? defaults?.gates ?? {};
  const signMin = asNumber(gates.signMin);
  const bearMin = asNumber(gates.bearNetP10Min);
  const drawdownMax = asNumber(gates.maxDrawdownMax);
  const fineCells = detail?.fine ?? [];
  const displayCells = fineCells.length ? fineCells : detail?.coarse ?? [];
  const allDoneCells = useMemo(() => [...(detail?.coarse ?? []), ...(detail?.fine ?? [])].filter((item) => item.status === "done"), [detail]);
  const totalCells = (detail?.study.coarseCellCount ?? 0) + (detail?.study.fineCellCount ?? 0);
  const doneCells = (detail?.study.doneCellCount ?? 0) + (detail?.study.failedCellCount ?? 0) + (detail?.study.skipCellCount ?? 0);
  const percent = totalCells > 0 ? Math.min(100, Math.round((doneCells / totalCells) * 100)) : 0;
  const noSolution = detail?.study.status === "done" && detail.study.passedCellCount === 0;

  const launchStudy = () => {
    void submit(studyName.trim() || `参数寻优 ${start} ~ ${end}`)
      .then((id) => {
        setStudyName("");
        message.success(`寻优任务 #${id} 已提交；搜索空间、协议和三关阈值已冻结`);
      })
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "提交寻优失败"));
  };

  const prefillCell = (cell: OptimizeCell) => {
    if (!source) return;
    const values: Record<string, number> = {};
    for (const [field, configKey] of Object.entries(PARAM_TO_CONFIG_KEY)) {
      const value = asNumber(cell.params?.[field]);
      if (value !== null) values[configKey] = value;
    }
    if (!Object.keys(values).length) {
      message.warning("该精算格没有可映射的生产参数；研究口径不会被伪装成线上配置");
      return;
    }
    const query = new URLSearchParams({
      instance: source.instanceKey,
      account: source.accountLabel,
      symbol,
      source: `自动寻优 #${detail?.study.id ?? "—"} · ${cell.key}`,
      note: `来自自动寻优 #${detail?.study.id ?? "—"} 的精算格 ${cell.key}；只预填草稿，请按三关与 OOS 纪律复核后再决定是否发布。`,
      prefill: JSON.stringify(values),
    });
    router.push(`/argus-config?${query.toString()}`);
  };

  const columns = useMemo<ColumnsType<OptimizeCell>>(
    () => [
      {
        title: "精算格",
        dataIndex: "key",
        width: 180,
        render: (key: string, row) => <Space direction="vertical" size={0}><b>{key}</b><Text type="secondary">{row.pathCount || "—"} 条路径 · {row.isIncumbent ? "现行配置" : "候选格"}</Text></Space>,
      },
      { title: "中位 28 日 PnL", dataIndex: "medPnl28", width: 120, render: (value: number) => `${value.toFixed(1)} U` },
      { title: "IQR", dataIndex: "iqrPnl28", width: 92, render: (value: number) => `${value.toFixed(1)} U` },
      {
        title: "关 1 · 符号一致",
        width: 130,
        render: (_, row) => <GateResult passed={row.okSign} label={`${(row.signRatio * 100).toFixed(0)}%`} value={`${(row.signRatio * 100).toFixed(1)}%`} threshold={signMin === null ? "—" : `≥ ${(signMin * 100).toFixed(0)}%`} />,
      },
      {
        title: "关 2 · 熊市 p10",
        width: 132,
        render: (_, row) => <GateResult passed={row.okBear} label={`${row.bearP10.toFixed(1)} U`} value={`${row.bearP10.toFixed(2)} U`} threshold={bearMin === null ? "—" : `≥ ${bearMin.toFixed(1)} U`} />,
      },
      {
        title: "关 3 · p90 回撤",
        width: 136,
        render: (_, row) => <GateResult passed={row.okDd} label={`${row.p90MaxDrawdown.toFixed(1)} U`} value={`${row.p90MaxDrawdown.toFixed(2)} U`} threshold={drawdownMax === null ? "—" : `≤ ${drawdownMax.toFixed(1)} U`} />,
      },
      {
        title: "结论",
        width: 148,
        render: (_, row) => <Tooltip title={row.verdictNote || "三关必须各自独立通过"}><Tag color={row.passed ? "green" : "red"}>{row.passed ? "三关全过" : `未过 ${3 - row.passCount} 关`}</Tag></Tooltip>,
      },
      {
        title: "操作",
        width: 170,
        render: (_, row) => <Button size="small" disabled={row.status !== "done"} onClick={() => prefillCell(row)}>带去发布（仅预填）</Button>,
      },
    ],
    [bearMin, drawdownMax, signMin, source, symbol, detail],
  );

  if (bootError && !source) {
    return <Alert className="manager-argus-alert" type="error" showIcon message="无法读取寻优初始化数据" description={bootError} />;
  }

  return (
    <div className="manager-page-stack manager-argus">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS ROBUST SEARCH</Text>
          <Title level={2} className="manager-argus-hero__title">后台自动参数寻优</Title>
          <Text className="manager-argus-hero__desc">
            先跑粗网格，再自动收敛精算格；每一格由路径分布和预注册的三关共同判定。这里不提供单一参数结论，也不会修改线上配置。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon"><ExperimentOutlined style={{ color: "var(--manager-primary)" }} />粗网格 → 精算</span>
          <span className="manager-argus-beacon"><SafetyCertificateOutlined />三关预注册</span>
        </div>
      </section>

      <Alert
        className="manager-argus-alert"
        type="warning"
        showIcon
        message="寻优不是一键采纳：扫描窗口内的收益必然偏乐观"
        description="单路径混沌约 ±70U，点估计不可作为结论。参数锁定后，必须用锁定之后新进的信号流每周复算；不得在同一窗口反复调参、换阈值直到“通过”为止。"
      />
      {defaultsError ? <Alert className="manager-argus-alert" type="error" showIcon message="寻优默认协议读取失败" description={defaultsError} /> : null}
      {detailError ? <Alert className="manager-argus-alert" type="error" showIcon message="寻优详情读取失败" description={detailError} /> : null}

      {bootstrapping ? <div className="manager-argus-panel"><Skeleton active paragraph={{ rows: 10 }} /></div> : (
        <>
          <Card className="manager-argus-panel manager-argus-backtest-controls" bordered={false}>
            <Row gutter={[14, 14]} align="bottom">
              <Col xs={24} xl={7}><Text type="secondary">信号源（实例 · 账户 · 合约）</Text><Select value={selectedSourceKey || undefined} style={{ width: "100%", marginTop: 6 }} options={sources.map((item) => ({ value: `${item.instanceKey}|${item.accountLabel}|${item.instrument}`, label: sourceLabel(item.instanceKey, item.accountLabel, item.instrument) }))} onChange={selectSource} /></Col>
              <Col xs={12} sm={6} xl={3}><Text type="secondary">回放平台</Text><Select value={platformCode} style={{ width: "100%", marginTop: 6 }} options={[{ value: "deepcoin", label: "DeepCoin" }, { value: "binance", label: "Binance" }]} onChange={setPlatformCode} /></Col>
              <Col xs={12} sm={6} xl={3}><Text type="secondary">合约</Text><Input value={symbol} style={{ marginTop: 6 }} onChange={(event) => setSymbol(event.target.value.toUpperCase())} /></Col>
              <Col xs={12} sm={6} xl={4}><Text type="secondary">扫描起始（本地墙钟）</Text><Input value={start} style={{ marginTop: 6 }} onChange={(event) => setStart(event.target.value)} /></Col>
              <Col xs={12} sm={6} xl={4}><Text type="secondary">扫描结束（本地墙钟）</Text><Input value={end} style={{ marginTop: 6 }} onChange={(event) => setEnd(event.target.value)} /></Col>
              <Col xs={24} xl={3}><Button type="primary" block icon={<ThunderboltFilled />} disabled={!source} loading={submitting} onClick={launchStudy}>启动后台扫描</Button></Col>
            </Row>
            <Space style={{ marginTop: 14, width: "100%" }}><Input value={studyName} placeholder="任务名称（可选）" onChange={(event) => setStudyName(event.target.value)} /><Text type="secondary">发起后阈值与协议即锁定，不能在结果出来后修改。</Text></Space>
          </Card>

          <div className="manager-argus-optimizer-method">
            <Card className="manager-argus-panel" title="搜索空间" extra={<Tag color="gold">{defaults?.coarseCellCount ?? "—"} 格粗网格</Tag>} bordered={false}>
              <Descriptions size="small" column={1} items={mapEntries(defaults?.space).map((item) => ({ key: item.key, label: item.key, children: <span className="manager-argus-mono">{item.value}</span> }))} />
              <Text className="manager-argus-table-note">搜索空间随任务冻结；这里不包含 signal_threshold，阈值改动只能做频率级估计，不能与事件级 PnL 混排。</Text>
            </Card>
            <Card className="manager-argus-panel" title="降噪协议" extra={<Tag color="blue">精算 {defaults?.finePathCount ?? "—"} 条路径 / 格</Tag>} bordered={false}>
              <Descriptions size="small" column={1} items={mapEntries(defaults?.protocol).slice(0, 8).map((item) => ({ key: item.key, label: item.key, children: <span className="manager-argus-mono">{item.value}</span> }))} />
              <Text className="manager-argus-table-note">抖动轴包含 close / 悲观 bar 内判定、0–3 日起点偏移与 5% 信号丢弃；费用与过冲口径由服务端冻结。</Text>
            </Card>
            <Card className="manager-argus-panel" title="三关判定（发起时预注册）" extra={<Tooltip title={defaults?.gateNote}><InfoCircleOutlined /></Tooltip>} bordered={false}>
              <Descriptions size="small" column={1} items={mapEntries(gates).map((item) => ({ key: item.key, label: item.key, children: <b className="manager-argus-mono">{item.value}</b> }))} />
              <Text className="manager-argus-table-note">必须逐项独立通过：符号一致率、熊市月净 p10、p90 最大回撤。不能以较高收益抵消任一风险门槛。</Text>
            </Card>
          </div>

          <Card className="manager-argus-panel" title="扫描任务" bordered={false} extra={<Select value={studyId ?? undefined} placeholder="选择历史寻优任务" style={{ minWidth: 245 }} options={studies.map((study) => ({ value: study.id, label: `#${study.id} · ${study.name || "未命名"} · ${study.status}` }))} onChange={selectStudy} />}>
            {!detail && !detailLoading ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="启动或选择一个寻优任务，查看冻结的方法、进度与精算结论" /> : null}
            {detailLoading ? <Skeleton active paragraph={{ rows: 5 }} /> : null}
            {detail ? <>
              <Row gutter={[16, 12]} className="manager-argus-study-kpis">
                <Col xs={12} lg={5}><Statistic title="阶段" value={detail.study.stage === "coarse" ? "粗网格" : detail.study.stage === "fine" ? "精算" : "已结论"} /></Col>
                <Col xs={12} lg={5}><Statistic title="触发 / K 线" value={`${detail.study.signalCount.toLocaleString()} / ${detail.study.klineCount.toLocaleString()}`} /></Col>
                <Col xs={12} lg={5}><Statistic title="已完成格" value={`${detail.study.doneCellCount} / ${totalCells || "—"}`} /></Col>
                <Col xs={12} lg={5}><Statistic title="三关全过" value={detail.study.passedCellCount} suffix="格" valueStyle={{ color: detail.study.passedCellCount ? "#0ECB81" : "#F6465D" }} /></Col>
                <Col xs={24} lg={4}><Tag color={detail.study.status === "done" ? "green" : detail.study.status === "failed" ? "red" : "gold"}>{detail.study.status}</Tag><br /><Text type="secondary">阈值锁定 {detail.study.gateLockedAt || "—"}</Text></Col>
              </Row>
              <div className="manager-argus-batch-progress"><Progress percent={percent} size="small" status={detail.study.status === "failed" ? "exception" : detail.study.status === "done" ? "success" : "active"} /><Button size="small" icon={<ReloadOutlined />} onClick={() => void refreshDetail()}>刷新</Button></div>
              {detail.study.convergeNote ? <Alert className="manager-argus-inline-alert" type="info" showIcon message="精算格收敛记录" description={detail.study.convergeNote} /> : null}
              {detail.warnings.map((warning) => <Alert key={warning} className="manager-argus-inline-alert" type="warning" showIcon message={warning} />)}
            </> : null}
          </Card>

          {detail ? <>
            <Card className={`manager-argus-panel manager-argus-verdict ${noSolution ? "is-no-solution" : ""}`} title={noSolution ? "结论：无解分支" : "结论：三关结果"} extra={<Tag color={noSolution ? "red" : "blue"}>{detail.study.passedCellCount} 格三关全过</Tag>} bordered={false}>
              {noSolution ? (
                <Alert type="error" showIcon icon={<CloseCircleOutlined />} message="当前预注册阈值下，精算格没有可被宣布为结论的参数包" description="这不是要求放宽门槛来凑出答案。请保留权衡前沿与各格失败原因，按 OOS 纪律继续积累锁定后的新信号流；扫描内收益不能替代风险门槛。" />
              ) : (
                <Alert type="info" showIcon message="通过三关只表示本次冻结窗口内满足预注册标准，不等于唯一答案或上线建议。" description="仍须执行 OOS 周度复算，并由人工审阅规模不变性、实例隔离与生产基线差异。" />
              )}
              {mapEntries(detail.conclusion).filter((item) => item.value !== "—").slice(0, 4).map((item) => <Text key={item.key} className="manager-argus-table-note">{item.key}：{item.value}</Text>)}
            </Card>

            <Card className="manager-argus-panel" title="精算结果：三关逐项独立判定" extra={<Text type="secondary">{fineCells.length ? "仅精算格；粗网格不作为最终结论" : "粗网格进行中"}</Text>} bordered={false}>
              {!displayCells.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="等待后台产出精算格" /> : <Table rowKey="id" size="small" scroll={{ x: 1200 }} columns={columns} dataSource={displayCells} pagination={false} />}
            </Card>

            <div className="manager-argus-optimizer-results">
              <Card className="manager-argus-panel" title="收益 / 回撤权衡前沿" extra={<Text type="secondary">不是排行；左上角代表相对取舍</Text>} bordered={false}><TradeoffFrontier cells={allDoneCells} /></Card>
              <Card className="manager-argus-panel" title="OOS 纪律" bordered={false}>
                <ol className="manager-argus-oos-list">
                  <li><b>锁定后再看：</b>扫描完成后固定候选格与三关阈值，开始记录新的信号流。</li>
                  <li><b>每周复算：</b>只用锁定之后新进的信号，不回头调搜索空间、协议或门槛。</li>
                  <li><b>分开报告：</b>in-sample 与 out-of-sample 结果并列，OOS 未积累前不把扫描收益写成发现。</li>
                </ol>
                <Alert type="warning" showIcon message="规模不变性已破缺" description="cap 的摊平动态依赖绝对张数；小额 challenger 只能验证单一效应方向，不能把大账户结论直接按比例搬过去。" />
              </Card>
            </div>
          </> : null}
        </>
      )}
    </div>
  );
}
