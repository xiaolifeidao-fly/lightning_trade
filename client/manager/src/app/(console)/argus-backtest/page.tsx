"use client";

import {
  BarChartOutlined,
  CheckCircleOutlined,
  DatabaseOutlined,
  InfoCircleOutlined,
  ReloadOutlined,
  SendOutlined,
  SettingOutlined,
  ThunderboltFilled,
} from "@ant-design/icons";
import { useRouter } from "next/navigation";
import {
  Alert,
  Button,
  Card,
  Col,
  Collapse,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
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
import { useEffect, useMemo, useState } from "react";
import type { BacktestTrade, ComparisonRow, SignalBacktestParams } from "./api/argus-backtest.api";
import { EquityCurve } from "./components/EquityCurve";
import {
  ALL_KNOBS,
  BATCH_STATUS_LABEL,
  EXIT_COLORS,
  FIDELITY_META,
  PARAM_KNOB_GROUPS,
  EMPTY,
  findKnob,
  fidelityMeta,
  fmtDuration,
  fmtInt,
  fmtNum,
  fmtPct,
  fmtSigned,
  shortTs,
  signColor,
} from "./constants";
import { useSignalBacktest } from "./hooks/useSignalBacktest";

const { Text, Title } = Typography;

type FormValues = Partial<Record<keyof SignalBacktestParams, number | string>>;

const EXIT_LABELS: Record<string, string> = {
  trail: "移动止盈",
  tp: "止盈",
  sl: "兜底止损",
  reduceClose: "反向减仓平仓",
  timeout: "超时",
  eodOpen: "窗口结束未平",
};

function sourceLabel(instanceKey: string, accountLabel: string, instrument: string) {
  return `${instanceKey} · ${accountLabel} · ${instrument}`;
}

function numericValue(value: number | string | undefined): number | string | undefined {
  if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
  if (typeof value === "string") return value.trim() ? value : undefined;
  return undefined;
}

function sameParam(left: number | string | undefined, right: number | string | undefined): boolean {
  if (typeof left === "number" && typeof right === "number") return Math.abs(left - right) < 1e-9;
  return left === right;
}

/** 只转出 catalog 明确能落入配置页的参数；研究口径绝不静默伪装成生产参数。 */
function buildConfigPrefill(row: ComparisonRow): Record<string, number> {
  const values: Record<string, number> = {};
  for (const item of row.paramDiff ?? []) {
    const knob = findKnob(item.field);
    const value = Number(item.value);
    if (knob?.configParamKey && Number.isFinite(value)) values[knob.configParamKey] = value;
  }
  return values;
}

export default function ArgusBacktestPage() {
  const router = useRouter();
  const [form] = Form.useForm<FormValues>();
  const [batchName, setBatchName] = useState("");
  const [selectedRunId, setSelectedRunId] = useState<number | null>(null);
  const [tradeDrawerOpen, setTradeDrawerOpen] = useState(false);
  const {
    sources,
    source,
    selectedSourceKey,
    selectSource,
    thresholdCandidates,
    bootstrapping,
    bootError,
    start,
    setStart,
    end,
    setEnd,
    symbol,
    setSymbol,
    platformCode,
    setPlatformCode,
    baseline,
    baselineForm,
    baselineLoading,
    baselineError,
    reloadBaseline,
    drafts,
    addDraft,
    removeDraft,
    clearDrafts,
    batches,
    batchId,
    detail,
    detailLoading,
    detailError,
    selectBatch,
    refreshDetail,
    runDetails,
    submitting,
    submit,
  } = useSignalBacktest();

  useEffect(() => {
    form.setFieldsValue(baselineForm);
  }, [baselineForm, form]);

  const rows = useMemo(
    () => detail?.groups.flatMap((group) => group.rows.map((row) => ({ ...row, group }))) ?? [],
    [detail],
  );
  const selectedRow = useMemo(() => rows.find((item) => item.runId === selectedRunId) ?? null, [rows, selectedRunId]);
  const selectedDetail = selectedRunId ? runDetails[selectedRunId] : undefined;
  const labels = useMemo(
    () => Object.fromEntries(rows.map((item) => [item.runId, item.isBaseline ? "生产基线" : item.groupLabel])),
    [rows],
  );

  useEffect(() => {
    if (!detail) return;
    const available = rows.find((item) => item.pnlAvailable && item.status === "done");
    if (available && !rows.some((item) => item.runId === selectedRunId)) setSelectedRunId(available.runId);
  }, [detail, rows, selectedRunId]);

  const exitRows = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const trade of selectedDetail?.trades ?? []) {
      const key = trade.closeReason || "unknown";
      counts[key] = (counts[key] ?? 0) + 1;
    }
    const total = Object.values(counts).reduce((sum, value) => sum + value, 0);
    return Object.entries(counts)
      .map(([key, count]) => ({ key, count, total, pct: total > 0 ? (count / total) * 100 : 0 }))
      .sort((left, right) => right.count - left.count);
  }, [selectedDetail]);

  const addCurrentGroup = () => {
    const values = form.getFieldsValue();
    const changed: Record<string, number | string> = {};
    const labels: string[] = [];
    for (const knob of ALL_KNOBS) {
      const next = numericValue(values[knob.field]);
      const base = baselineForm[knob.field];
      if (next === undefined || sameParam(next, base)) continue;
      changed[knob.field] = next;
      labels.push(`${knob.label}=${next}`);
    }
    if (labels.length === 0) {
      message.warning("这组参数与生产基线没有差异；基线会在提交时单独作为参照运行");
      return;
    }
    addDraft(labels.slice(0, 2).join(" · ") + (labels.length > 2 ? ` +${labels.length - 2}` : ""), changed as SignalBacktestParams);
    message.success("已加入扫描队列；尚未提交回放");
  };

  const submitBatch = () => {
    void submit(batchName.trim() || `盘口信号参数扫描 ${start} ~ ${end}`, true, 3)
      .then((id) => {
        setBatchName("");
        message.success(`批量回放 #${id} 已提交，输入信号集与生产基线已冻结`);
      })
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "提交回放失败"));
  };

  const openPublishPrefill = (row: ComparisonRow) => {
    if (!source) return;
    const values = buildConfigPrefill(row);
    if (Object.keys(values).length === 0) {
      message.warning("此组只改了研究口径或没有可映射的生产参数，不能带入参数草稿");
      return;
    }
    const query = new URLSearchParams({
      instance: source.instanceKey,
      account: source.accountLabel,
      symbol,
      source: `批量回测 #${batchId ?? "—"} · ${row.groupLabel}`,
      note: `来自批量回测 #${batchId ?? "—"} 的参数草稿；请审阅差异后再决定是否发布。`,
      prefill: JSON.stringify(values),
    });
    router.push(`/argus-config?${query.toString()}`);
  };

  const columns = useMemo<ColumnsType<ComparisonRow>>(
    () => [
      {
        title: "参数组",
        dataIndex: "groupLabel",
        width: 220,
        render: (value: string, row) => (
          <Space direction="vertical" size={2}>
            <Space size={6}>
              <b>{row.isBaseline ? "生产基线" : value}</b>
              {row.isBaseline ? <Tag color="gold">基线</Tag> : null}
            </Space>
            <Text type="secondary" className="manager-argus-mono manager-argus-table-note">
              {row.paramDiff.length ? row.paramDiff.map((item) => `${item.key}: ${item.baseline} → ${item.value}`).join("；") : "与基线相同"}
            </Text>
          </Space>
        ),
      },
      {
        title: "精度",
        dataIndex: "fidelity",
        width: 132,
        render: (value: string) => {
          const meta = fidelityMeta(value);
          return <Tag color={meta.tone === "success" ? "green" : "orange"}>{meta.label}</Tag>;
        },
      },
      {
        title: "净盈亏",
        width: 112,
        render: (_, row) =>
          row.pnlAvailable ? (
            <span style={{ color: signColor(row.metric?.netPnl) }}>{fmtSigned(row.metric?.netPnl, 2, " U")}</span>
          ) : (
            <Tooltip title="频率级结果没有逐次触发时刻，不能产出 PnL">
              <span>{EMPTY}</span>
            </Tooltip>
          ),
      },
      {
        title: "最大回撤",
        width: 104,
        render: (_, row) => (row.pnlAvailable ? fmtNum(row.metric?.maxDrawdown, 2, " U") : EMPTY),
      },
      {
        title: "信号 / λ",
        width: 118,
        render: (_, row) =>
          row.pnlAvailable ? fmtInt(row.metric?.signalCount) : `${fmtNum(row.metric?.lambdaPerDay, 2)} / 日`,
      },
      {
        title: "相对基线",
        width: 142,
        render: (_, row) =>
          row.metricDiff ? (
            <Space direction="vertical" size={0}>
              <span style={{ color: signColor(row.metricDiff.netPnl) }}>{fmtSigned(row.metricDiff.netPnl, 2, " U")}</span>
              <Text type="secondary">回撤 {fmtSigned(row.metricDiff.maxDrawdown, 2, " U")}</Text>
            </Space>
          ) : (
            <Tooltip title={row.diffBlockedReason || "本行是参照基线"}>
              <Text type="secondary">不可比较</Text>
            </Tooltip>
          ),
      },
      {
        title: "操作",
        width: 190,
        render: (_, row) => (
          <Space size={4} onClick={(event) => event.stopPropagation()}>
            <Button
              size="small"
              disabled={!row.pnlAvailable || row.status !== "done"}
              onClick={() => {
                setSelectedRunId(row.runId);
                setTradeDrawerOpen(true);
              }}
            >
              逐笔
            </Button>
            <Button size="small" disabled={row.isBaseline || row.paramDiff.length === 0} onClick={() => openPublishPrefill(row)}>
              仅预填
            </Button>
          </Space>
        ),
      },
    ],
    [batchId, source, symbol],
  );

  const tradeColumns = useMemo<ColumnsType<BacktestTrade>>(
    () => [
      { title: "方向", dataIndex: "direction", width: 72 },
      { title: "开仓", dataIndex: "openedAt", width: 160 },
      { title: "平仓", dataIndex: "closedAt", width: 160 },
      { title: "出场", dataIndex: "closeReason", width: 116, render: (value: string) => EXIT_LABELS[value] ?? value ?? "—" },
      { title: "张数", dataIndex: "contracts", width: 72, render: (value: number) => fmtInt(value) },
      { title: "加仓", dataIndex: "addCount", width: 72, render: (value: number) => fmtInt(value) },
      { title: "峰值 ROI", dataIndex: "peakPct", width: 96, render: (value: number) => fmtNum(value, 2, "%") },
      { title: "净盈亏", dataIndex: "netPnl", width: 100, render: (value: number) => <span style={{ color: signColor(value) }}>{fmtSigned(value, 2, " U")}</span> },
    ],
    [],
  );

  if (bootError && !source) {
    return <Alert className="manager-argus-alert" type="error" showIcon message="无法读取可回测的信号源" description={bootError} />;
  }

  return (
    <div className="manager-page-stack manager-argus">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS SIGNAL REPLAY</Text>
          <Title level={2} className="manager-argus-hero__title">盘口信号回测与参数对比</Title>
          <Text className="manager-argus-hero__desc">
            从所选实例的已发布生产参数出发，只提交差异参数；同一批次共享同一段 strategy_event 触发流，结果按精度等级隔离，避免把近似结果当成结论。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon"><ThunderboltFilled style={{ color: "var(--manager-primary)" }} />事件驱动回放</span>
          <span className="manager-argus-beacon"><DatabaseOutlined />{source ? `${source.eventCount.toLocaleString()} 条可用信号` : "读取信号源中"}</span>
        </div>
      </section>

      <Alert
        className="manager-argus-alert"
        type="warning"
        showIcon
        message="不要从 1m K 线反推盘口信号"
        description="信号来自 DeepCoin last 相对 mark 的偏离，K 线没有 mark。实测 28.7% 的信号分钟内触发 ≥2 次，承载 48.6% 的全部触发；1m 只能用于持仓路径回放。回放峰值 ROI（peakPct）会因 60 秒采样偏高；同一根同时触及止盈与止损时按不利方向（先止损）结算。"
      />

      {baselineError ? <Alert className="manager-argus-alert" type="error" showIcon message="生产基线读取失败" description={baselineError} /> : null}
      {detailError ? <Alert className="manager-argus-alert" type="error" showIcon message="批次详情读取失败" description={detailError} /> : null}

      {bootstrapping ? (
        <div className="manager-argus-panel"><Skeleton active paragraph={{ rows: 9 }} /></div>
      ) : (
        <>
          <Card className="manager-argus-panel manager-argus-backtest-controls" bordered={false}>
            <Row gutter={[14, 14]} align="bottom">
              <Col xs={24} xl={7}>
                <Text type="secondary">信号源（实例 · 账户 · 合约）</Text>
                <Select
                  value={selectedSourceKey || undefined}
                  style={{ width: "100%", marginTop: 6 }}
                  loading={bootstrapping}
                  options={sources.map((item) => ({ value: `${item.instanceKey}|${item.accountLabel}|${item.instrument}`, label: sourceLabel(item.instanceKey, item.accountLabel, item.instrument) }))}
                  onChange={selectSource}
                />
              </Col>
              <Col xs={12} sm={6} xl={3}><Text type="secondary">回放平台</Text><Select value={platformCode} style={{ width: "100%", marginTop: 6 }} options={[{ value: "deepcoin", label: "DeepCoin" }, { value: "binance", label: "Binance" }]} onChange={setPlatformCode} /></Col>
              <Col xs={12} sm={6} xl={3}><Text type="secondary">合约</Text><Input value={symbol} style={{ marginTop: 6 }} onChange={(event) => setSymbol(event.target.value.toUpperCase())} /></Col>
              <Col xs={12} sm={6} xl={4}><Text type="secondary">起始（本地墙钟）</Text><Input value={start} style={{ marginTop: 6 }} onChange={(event) => setStart(event.target.value)} /></Col>
              <Col xs={12} sm={6} xl={4}><Text type="secondary">结束（本地墙钟）</Text><Input value={end} style={{ marginTop: 6 }} onChange={(event) => setEnd(event.target.value)} /></Col>
              <Col xs={24} xl={3}><Button block icon={<ReloadOutlined />} loading={baselineLoading} onClick={() => void reloadBaseline()}>重载生产基线</Button></Col>
            </Row>
            {baseline ? (
              <div className="manager-argus-baseline-strip">
                <CheckCircleOutlined />
                <span>基线来源：<b>{baseline.source === "instance_published" ? "所选实例已发布配置" : baseline.source}</b></span>
                {baseline.notes?.length ? <Tooltip title={baseline.notes.join("\n")}><Tag color="orange">含 {baseline.notes.length} 项兜底口径</Tag></Tooltip> : <Tag color="green">已读取生产快照</Tag>}
              </div>
            ) : null}
          </Card>

          <div className="manager-argus-backtest-workbench">
            <Card className="manager-argus-panel" title={<Space><SettingOutlined />参数编辑</Space>} extra={<Text type="secondary">默认 = 所选实例生产参数</Text>} bordered={false}>
              <Form form={form} layout="vertical" className="manager-argus-param-form">
                <Collapse
                  defaultActiveKey={["signal", "risk"]}
                  items={PARAM_KNOB_GROUPS.map((group) => ({
                    key: group.key,
                    label: <span>{group.name}<Text type="secondary"> · {group.desc}</Text></span>,
                    children: (
                      <Row gutter={10}>
                        {group.knobs.map((knob) => (
                          <Col span={24} key={knob.field}>
                            <Form.Item
                              name={knob.field}
                              label={<span>{knob.label} <Tooltip title={<span><b>真实参数键</b><br />{knob.propertyKey}<br /><b>配置落点</b><br />{knob.storeKey}{knob.hint ? <><br /><br />{knob.hint}</> : null}</span>}><InfoCircleOutlined className="manager-argus-info-icon" /></Tooltip></span>}
                            >
                              {knob.type === "select" ? (
                                <Select options={knob.options} />
                              ) : (
                                <InputNumber min={knob.min} step={knob.step} precision={knob.precision} addonAfter={knob.unit} style={{ width: "100%" }} />
                              )}
                            </Form.Item>
                          </Col>
                        ))}
                      </Row>
                    ),
                  }))}
                />
              </Form>
              {thresholdCandidates.length ? <Text className="manager-argus-table-note">dev_sample 可校准阈值：{thresholdCandidates.join(" / ")} bp</Text> : null}
              <Space style={{ marginTop: 14, width: "100%" }}>
                <Button type="primary" icon={<SendOutlined />} onClick={addCurrentGroup}>加入扫描队列</Button>
                <Button onClick={() => form.setFieldsValue(baselineForm)}>重置为生产基线</Button>
              </Space>
            </Card>

            <Card className="manager-argus-panel" title={<Space><BarChartOutlined />扫描队列</Space>} bordered={false}>
              {drafts.length ? (
                <div className="manager-argus-draft-list">
                  {drafts.map((draft, index) => (
                    <div key={draft.localId}>
                      <span className="manager-argus-draft-index">{index + 1}</span>
                      <div><b>{draft.label}</b><Text type="secondary"> {Object.keys(draft.overrides).length} 项参数差异</Text></div>
                      <Button size="small" type="text" danger onClick={() => removeDraft(draft.localId)}>移除</Button>
                    </div>
                  ))}
                </div>
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="把一组差异参数加入这里；生产基线会自动并行作为参照" />}
              <Space direction="vertical" style={{ width: "100%", marginTop: 14 }}>
                <Input value={batchName} placeholder="批次名称（可选）" onChange={(event) => setBatchName(event.target.value)} />
                <Button type="primary" block icon={<ThunderboltFilled />} loading={submitting} disabled={!drafts.length || !source} onClick={submitBatch}>提交 {drafts.length || ""} 组批量回放</Button>
                <Button block disabled={!drafts.length} onClick={clearDrafts}>清空未提交队列</Button>
              </Space>
              <div className="manager-argus-hint" style={{ marginTop: 14 }}><span>提交后只创建异步回放任务，不会发布、热加载或修改任何生产参数。</span></div>
            </Card>
          </div>

          <Card className="manager-argus-panel" title="参数组对比" bordered={false} extra={
            <Select
              value={batchId ?? undefined}
              placeholder="选择历史批次"
              style={{ minWidth: 235 }}
              options={batches.map((batch) => ({ value: batch.id, label: `#${batch.id} · ${batch.name || "未命名"} · ${BATCH_STATUS_LABEL[batch.status] ?? batch.status}` }))}
              onChange={selectBatch}
            />
          }>
            {!detail && !detailLoading ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="提交或选择一个批量回放后，在这里查看按精度隔离的对比矩阵" /> : null}
            {detailLoading ? <Skeleton active paragraph={{ rows: 5 }} /> : null}
            {detail ? (
              <>
                <div className="manager-argus-batch-progress">
                  <Space><Tag color={detail.batch.status === "done" ? "green" : detail.batch.status === "failed" ? "red" : "gold"}>{BATCH_STATUS_LABEL[detail.batch.status] ?? detail.batch.status}</Tag><Text>{detail.batch.doneCount} / {detail.batch.groupCount} 组已完成</Text></Space>
                  <Progress percent={detail.batch.groupCount ? Math.round((detail.batch.doneCount / detail.batch.groupCount) * 100) : 0} size="small" status={detail.batch.status === "failed" ? "exception" : detail.batch.status === "done" ? "success" : "active"} />
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => void refreshDetail()}>刷新</Button>
                </div>
                {detail.warnings.map((warning) => <Alert key={warning} className="manager-argus-inline-alert" type="warning" showIcon message={warning} />)}
                {detail.groups.map((group) => {
                  const meta = fidelityMeta(group.fidelity);
                  return (
                    <section key={group.fidelity} className="manager-argus-fidelity-group">
                      <div className="manager-argus-fidelity-group__head">
                        <div><Tag color={meta.tone === "success" ? "green" : "orange"}>{group.fidelityLabel || meta.label}</Tag><Text>{meta.desc}</Text></div>
                        <Text type="secondary">组内按 {group.sortedBy === "netPnl" ? "净盈亏" : "λ（每日触发频率）"} 排序</Text>
                      </div>
                      {group.notes?.map((note) => <Text key={note} className="manager-argus-table-note">{note}</Text>)}
                      <Table rowKey="runId" size="small" scroll={{ x: 1100 }} columns={columns} dataSource={group.rows} pagination={false} rowClassName={(row) => row.runId === selectedRunId ? "manager-argus-selected-row" : ""} onRow={(row) => ({ onClick: () => row.pnlAvailable && row.status === "done" && setSelectedRunId(row.runId) })} />
                    </section>
                  );
                })}
              </>
            ) : null}
          </Card>

          <div className="manager-argus-backtest-results">
            <Card className="manager-argus-panel" title="累计已实现净值曲线" bordered={false}>
              <EquityCurve details={runDetails} labels={labels} selectedRunId={selectedRunId} />
            </Card>
            <Card className="manager-argus-panel" title="选中组：出场方式分布" bordered={false}>
              {selectedDetail ? (
                <div className="manager-argus-exit-list">
                  {exitRows.map((item) => (
                    <div key={item.key}>
                      <div><span><i style={{ background: EXIT_COLORS[item.key] ?? "var(--manager-text-faint)" }} />{EXIT_LABELS[item.key] ?? item.key}</span><b>{item.count} 笔</b></div>
                      <Progress percent={item.pct} showInfo={false} strokeColor={EXIT_COLORS[item.key] ?? "#848E9C"} />
                    </div>
                  ))}
                  {!exitRows.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前组没有已平仓逐笔" /> : null}
                </div>
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择一个已完成事件级组查看出场分布" />}
            </Card>
            <Card className="manager-argus-panel" title="与生产基线的差异" bordered={false}>
              {selectedRow?.metricDiff ? (
                <Row gutter={[8, 12]} className="manager-argus-diff-stats">
                  <Col span={12}><Statistic title="净盈亏差异" value={selectedRow.metricDiff.netPnl} precision={2} suffix="U" valueStyle={{ color: signColor(selectedRow.metricDiff.netPnl) }} /></Col>
                  <Col span={12}><Statistic title="最大回撤差异" value={selectedRow.metricDiff.maxDrawdown} precision={2} suffix="U" valueStyle={{ color: signColor(selectedRow.metricDiff.maxDrawdown) }} /></Col>
                  <Col span={12}><Statistic title="胜率差异" value={selectedRow.metricDiff.winRate * 100} precision={1} suffix="%" valueStyle={{ color: signColor(selectedRow.metricDiff.winRate) }} /></Col>
                  <Col span={12}><Statistic title="最大持仓差异" value={selectedRow.metricDiff.maxStack} precision={0} suffix="张" valueStyle={{ color: signColor(selectedRow.metricDiff.maxStack) }} /></Col>
                </Row>
              ) : <Alert type="info" showIcon message={selectedRow?.diffBlockedReason || "选择完成的事件级组后显示与生产基线的同精度差异"} />}
              {selectedRow && !selectedRow.isBaseline ? <Button style={{ marginTop: 16 }} onClick={() => openPublishPrefill(selectedRow)}>带这组参数去发布（仅预填）</Button> : null}
            </Card>
          </div>
        </>
      )}

      <Drawer title={`逐笔明细 · ${selectedRow?.isBaseline ? "生产基线" : selectedRow?.groupLabel ?? "—"}`} width={980} open={tradeDrawerOpen} onClose={() => setTradeDrawerOpen(false)}>
        <Alert type="warning" showIcon message="peakPct 是按 1m 路径回放的峰值，因实盘每 5 秒检查而倾向偏高；同根冲突按不利方向结算。" style={{ marginBottom: 16 }} />
        <Table rowKey="id" size="small" columns={tradeColumns} dataSource={selectedDetail?.trades ?? []} scroll={{ x: 950 }} pagination={{ pageSize: 20 }} />
      </Drawer>
    </div>
  );
}
