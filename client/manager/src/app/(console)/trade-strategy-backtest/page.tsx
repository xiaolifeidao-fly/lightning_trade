"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Col,
  Collapse,
  Divider,
  InputNumber,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import {
  ExperimentOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import {
  fetchTradeStrategyBacktest,
  TradeStrategyBacktestRecord,
  type TradeStrategyBacktestCell,
} from "../trade-orders/api/trade.api";

const { Text, Title, Paragraph } = Typography;

// AI 预测目前只跑 BTC，其它币种无预测数据，因此只提供 BTC。
const DEFAULT_COINS = [
  { label: "BTC / USDT", value: "BTC" },
];

const INTERVAL_OPTIONS = [
  { label: "5分钟", value: "5m" },
  { label: "15分钟", value: "15m" },
  { label: "1小时", value: "1h" },
  { label: "4小时", value: "4h" },
];

type Filters = {
  platformCode: string;
  coinCode: string;
  interval: string;
  limit: number;
  holdBars: number;
  minConfidence: number;
  minMovePct: number;
  takerFeeRate: number;
  fundingRate: number;
  leverage: number;
  tpList: string;
  slList: string;
};

const DEFAULT_FILTERS: Filters = {
  platformCode: "binance",
  coinCode: "BTC",
  interval: "1h",
  limit: 500,
  holdBars: 1,
  minConfidence: 0,
  minMovePct: 1,
  takerFeeRate: 0.05,
  fundingRate: 0.01,
  leverage: 1,
  tpList: "1,1.5,2,2.5,3",
  slList: "0.5,1,1.5,2",
};

// 期望值映射到热力色：正向绿、负向红，深浅按相对最大绝对期望。
function expectancyColor(value: number, maxAbs: number) {
  if (maxAbs <= 0) return "rgba(255,255,255,0)";
  const ratio = Math.min(Math.abs(value) / maxAbs, 1);
  const alpha = 0.12 + ratio * 0.6;
  if (value >= 0) return `rgba(34,197,94,${alpha.toFixed(3)})`;
  return `rgba(239,68,68,${alpha.toFixed(3)})`;
}

function pct(value: number) {
  return `${value >= 0 ? "" : ""}${value.toFixed(2)}%`;
}

export default function TradeStrategyBacktestPage() {
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<TradeStrategyBacktestRecord | null>(null);

  const load = useCallback(async (f: Filters) => {
    setLoading(true);
    try {
      const res = await fetchTradeStrategyBacktest(f);
      setData(res ?? null);
    } catch (err) {
      message.error("回测加载失败，请稍后重试");
      // eslint-disable-next-line no-console
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(DEFAULT_FILTERS);
  }, [load]);

  const coinOptions = data?.coinOptions?.length ? data.coinOptions : DEFAULT_COINS;

  const maxAbsExpectancy = useMemo(() => {
    if (!data?.cells?.length) return 0;
    return data.cells.reduce((m, c) => Math.max(m, Math.abs(c.expectancy)), 0);
  }, [data]);

  const cellByKey = useMemo(() => {
    const map = new Map<string, TradeStrategyBacktestCell>();
    data?.cells?.forEach((c) => map.set(`${c.takeProfitPct}_${c.stopLossPct}`, c));
    return map;
  }, [data]);

  const best = data?.best ?? null;

  const updateFilter = <K extends keyof Filters>(key: K, value: Filters[K]) =>
    setFilters((prev) => ({ ...prev, [key]: value }));

  const detailColumns: ColumnsType<TradeStrategyBacktestCell> = [
    { title: "止盈%", dataIndex: "takeProfitPct", width: 80, render: (v: number) => `${v}%` },
    { title: "止损%", dataIndex: "stopLossPct", width: 80, render: (v: number) => `${v}%` },
    {
      title: "单笔期望",
      dataIndex: "expectancy",
      width: 100,
      sorter: (a, b) => a.expectancy - b.expectancy,
      defaultSortOrder: "descend",
      render: (v: number) => (
        <Text strong style={{ color: v >= 0 ? "#16a34a" : "#dc2626" }}>
          {pct(v)}
        </Text>
      ),
    },
    { title: "胜率", dataIndex: "winRate", width: 90, render: (v: number) => `${v}%` },
    { title: "盈亏比", dataIndex: "payoff", width: 90, render: (v: number) => (v ? v.toFixed(2) : "-") },
    {
      title: "盈利因子",
      dataIndex: "profitFactor",
      width: 100,
      render: (v: number) => (v >= 999 ? "∞" : v ? v.toFixed(2) : "-"),
    },
    { title: "触止盈", dataIndex: "tpRate", width: 90, render: (v: number) => `${v}%` },
    { title: "触止损", dataIndex: "slRate", width: 90, render: (v: number) => `${v}%` },
    { title: "到期", dataIndex: "timeoutRate", width: 90, render: (v: number) => `${v}%` },
    {
      title: "累计净收益",
      dataIndex: "totalReturn",
      width: 110,
      render: (v: number) => (
        <Text style={{ color: v >= 0 ? "#16a34a" : "#dc2626" }}>{pct(v)}</Text>
      ),
    },
    { title: "最大回撤", dataIndex: "maxDrawdown", width: 110, render: (v: number) => `-${v.toFixed(2)}%` },
  ];

  return (
    <div style={{ padding: 16, display: "flex", flexDirection: "column", gap: 16 }}>
      <Card>
        <Space align="center" style={{ marginBottom: 4 }}>
          <ExperimentOutlined style={{ fontSize: 20, color: "#6366f1" }} />
          <Title level={4} style={{ margin: 0 }}>
            策略回测 · 止盈×止损期望矩阵
          </Title>
        </Space>
        <Paragraph type="secondary" style={{ margin: 0 }}>
          以「方向 + 预测幅度阈值 + 置信度」筛选历史 AI 预测信号，用其后续的真实 K
          线回测不同止盈/止损组合，扣除手续费与资金费率后，给出每个组合的单笔期望、胜率、盈亏比，帮助你判断这套开仓策略是否正期望、止盈止损该设多少。
        </Paragraph>
      </Card>

      {/* 筛选条件 */}
      <Card title="回测参数" size="small">
        <Row gutter={[16, 16]}>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Text type="secondary">交易对</Text>
            <Select
              style={{ width: "100%" }}
              value={filters.coinCode}
              options={coinOptions}
              onChange={(v) => updateFilter("coinCode", v)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Text type="secondary">主周期（预测 horizon）</Text>
            <Select
              style={{ width: "100%" }}
              value={filters.interval}
              options={INTERVAL_OPTIONS}
              onChange={(v) => updateFilter("interval", v)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="开仓后向前看几根 K 线作为持仓窗口，到期未触发止盈止损则按窗口末收盘价平仓。1 = 只看下一根（与 AI 预测窗口一致）。">
              <Text type="secondary">持仓窗口（根）</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={1}
              max={24}
              value={filters.holdBars}
              onChange={(v) => updateFilter("holdBars", Number(v) || 1)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="只回测 |预测价-参考价|/参考价 ≥ 该阈值 的信号。例如 3 表示只在 AI 预测涨/跌幅 ≥3% 时才开仓。">
              <Text type="secondary">预测幅度阈值 %</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={0}
              step={0.5}
              value={filters.minMovePct}
              onChange={(v) => updateFilter("minMovePct", Number(v) || 0)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="只回测置信度 ≥ 该值的信号，范围 0~1。">
              <Text type="secondary">置信度下限</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={0}
              max={1}
              step={0.05}
              value={filters.minConfidence}
              onChange={(v) => updateFilter("minConfidence", Number(v) || 0)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="单边吃单手续费率（%）。开+平两次计入成本。币安合约约 0.05%。">
              <Text type="secondary">手续费率 %（单边）</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={0}
              step={0.01}
              value={filters.takerFeeRate}
              onChange={(v) => updateFilter("takerFeeRate", Number(v) || 0)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="每根持仓周期的资金费率（%），按持仓窗口根数累计计入成本。">
              <Text type="secondary">资金费率 %/根</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={0}
              step={0.01}
              value={filters.fundingRate}
              onChange={(v) => updateFilter("fundingRate", Number(v) || 0)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="杠杆，仅用于把名义收益换算成保证金回报(ROE)展示，不影响期望矩阵本身。">
              <Text type="secondary">杠杆</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={1}
              max={125}
              value={filters.leverage}
              onChange={(v) => updateFilter("leverage", Number(v) || 1)}
            />
          </Col>
          <Col xs={12} sm={8} md={6} lg={4}>
            <Tooltip title="参与回测的最近 K 线根数上限，最多 1000。">
              <Text type="secondary">样本 K 线数</Text>
            </Tooltip>
            <InputNumber
              style={{ width: "100%" }}
              min={50}
              max={1000}
              step={50}
              value={filters.limit}
              onChange={(v) => updateFilter("limit", Number(v) || 500)}
            />
          </Col>
          <Col xs={24} sm={12} md={9} lg={6}>
            <Tooltip title="要扫描的止盈幅度列表（%），逗号分隔。">
              <Text type="secondary">止盈档位 %</Text>
            </Tooltip>
            <input
              className="ant-input"
              style={{ width: "100%", height: 32, padding: "4px 11px", borderRadius: 6, border: "1px solid #d9d9d9" }}
              value={filters.tpList}
              onChange={(e) => updateFilter("tpList", e.target.value)}
            />
          </Col>
          <Col xs={24} sm={12} md={9} lg={6}>
            <Tooltip title="要扫描的止损幅度列表（%），逗号分隔。">
              <Text type="secondary">止损档位 %</Text>
            </Tooltip>
            <input
              className="ant-input"
              style={{ width: "100%", height: 32, padding: "4px 11px", borderRadius: 6, border: "1px solid #d9d9d9" }}
              value={filters.slList}
              onChange={(e) => updateFilter("slList", e.target.value)}
            />
          </Col>
          <Col xs={24} md={6} lg={4} style={{ display: "flex", alignItems: "flex-end", gap: 8 }}>
            <Button type="primary" icon={<ThunderboltOutlined />} loading={loading} onClick={() => load(filters)}>
              运行回测
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => { setFilters(DEFAULT_FILTERS); load(DEFAULT_FILTERS); }}>
              重置
            </Button>
          </Col>
        </Row>
      </Card>

      {/* 概览 */}
      {data && (
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6} lg={4}>
            <Card size="small"><Statistic title="区间预测总数" value={data.totalPredictions} /></Card>
          </Col>
          <Col xs={12} md={6} lg={4}>
            <Card size="small"><Statistic title="合格信号数" value={data.qualifiedSignals} /></Card>
          </Col>
          <Col xs={12} md={6} lg={4}>
            <Card size="small">
              <Statistic title="方向正确率" value={data.directionAccuracy} suffix="%" precision={2}
                valueStyle={{ color: data.directionAccuracy >= 50 ? "#16a34a" : "#dc2626" }} />
            </Card>
          </Col>
          <Col xs={12} md={6} lg={4}>
            <Card size="small"><Statistic title="平均预测幅度" value={data.avgPredictMovePct} suffix="%" precision={2} /></Card>
          </Col>
          <Col xs={12} md={6} lg={4}>
            <Card size="small"><Statistic title="单笔成本" value={data.costPerTrade} suffix="%" precision={3} /></Card>
          </Col>
          <Col xs={12} md={6} lg={4}>
            <Card size="small">
              <div style={{ color: "rgba(0,0,0,0.45)", fontSize: 14, marginBottom: 4 }}>样本区间</div>
              <Text style={{ fontSize: 13 }}>{data.rangeStart || "-"} ~ {data.rangeEnd || "-"}</Text>
            </Card>
          </Col>
        </Row>
      )}

      {data && data.qualifiedSignals === 0 && (
        <Alert
          type="warning"
          showIcon
          message="该参数下没有合格信号"
          description="可能是预测幅度阈值过高、置信度下限过高，或该周期历史预测/K线数据不足。可降低阈值或更换周期后重试。"
        />
      )}

      {/* 最优组合 */}
      {best && data && data.qualifiedSignals > 0 && (
        <Card size="small" title="期望最优组合（按单笔期望排序）">
          <Row gutter={[16, 8]}>
            <Col xs={12} md={6} lg={3}><Statistic title="止盈" value={best.takeProfitPct} suffix="%" /></Col>
            <Col xs={12} md={6} lg={3}><Statistic title="止损" value={best.stopLossPct} suffix="%" /></Col>
            <Col xs={12} md={6} lg={3}>
              <Statistic title="单笔期望" value={best.expectancy} suffix="%" precision={2}
                valueStyle={{ color: best.expectancy >= 0 ? "#16a34a" : "#dc2626" }} />
            </Col>
            <Col xs={12} md={6} lg={3}>
              <Statistic title={`ROE×${data.leverage}`} value={best.expectancyRoe} suffix="%" precision={2}
                valueStyle={{ color: best.expectancyRoe >= 0 ? "#16a34a" : "#dc2626" }} />
            </Col>
            <Col xs={12} md={6} lg={3}><Statistic title="胜率" value={best.winRate} suffix="%" precision={2} /></Col>
            <Col xs={12} md={6} lg={3}><Statistic title="盈亏比" value={best.payoff} precision={2} /></Col>
            <Col xs={12} md={6} lg={3}>
              <div style={{ color: "rgba(0,0,0,0.45)", fontSize: 14, marginBottom: 4 }}>盈利因子</div>
              <Text strong style={{ fontSize: 24 }}>{best.profitFactor >= 999 ? "∞" : best.profitFactor.toFixed(2)}</Text>
            </Col>
            <Col xs={12} md={6} lg={3}><Statistic title="最大回撤" value={best.maxDrawdown} prefix="-" suffix="%" precision={2} /></Col>
          </Row>
        </Card>
      )}

      {/* 期望热力矩阵 */}
      {data && data.qualifiedSignals > 0 && (
        <Card
          size="small"
          title="单笔期望热力矩阵（行=止损，列=止盈；单元=扣费后名义%）"
          extra={<Text type="secondary">悬停查看明细 · 绿=正期望 红=负期望</Text>}
        >
          <div style={{ overflowX: "auto" }}>
            <table style={{ borderCollapse: "collapse", minWidth: 480 }}>
              <thead>
                <tr>
                  <th style={{ padding: 8, textAlign: "left", fontWeight: 600 }}>止损 ＼ 止盈</th>
                  {data.tpPercents.map((tp) => (
                    <th key={tp} style={{ padding: 8, textAlign: "center" }}>{tp}%</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.slPercents.map((sl) => (
                  <tr key={sl}>
                    <td style={{ padding: 8, fontWeight: 600 }}>{sl}%</td>
                    {data.tpPercents.map((tp) => {
                      const cell = cellByKey.get(`${tp}_${sl}`);
                      if (!cell) return <td key={tp} style={{ padding: 8 }}>-</td>;
                      const isBest = best && cell.takeProfitPct === best.takeProfitPct && cell.stopLossPct === best.stopLossPct;
                      return (
                        <td
                          key={tp}
                          style={{
                            padding: 0,
                            textAlign: "center",
                            border: isBest ? "2px solid #6366f1" : "1px solid #f0f0f0",
                            background: expectancyColor(cell.expectancy, maxAbsExpectancy),
                          }}
                        >
                          <Tooltip
                            title={
                              <div style={{ lineHeight: 1.7 }}>
                                <div>止盈 {cell.takeProfitPct}% / 止损 {cell.stopLossPct}%</div>
                                <div>单笔期望：{pct(cell.expectancy)}（ROE {pct(cell.expectancyRoe)}）</div>
                                <div>胜率 {cell.winRate}% · 盈亏比 {cell.payoff.toFixed(2)}</div>
                                <div>触止盈 {cell.tpRate}% / 触止损 {cell.slRate}% / 到期 {cell.timeoutRate}%</div>
                                <div>盈利因子 {cell.profitFactor >= 999 ? "∞" : cell.profitFactor.toFixed(2)} · 累计 {pct(cell.totalReturn)}</div>
                                <div>最大回撤 -{cell.maxDrawdown}%</div>
                              </div>
                            }
                          >
                            <div style={{ padding: "10px 14px", cursor: "default" }}>
                              <Text strong style={{ color: cell.expectancy >= 0 ? "#15803d" : "#b91c1c" }}>
                                {cell.expectancy >= 0 ? "+" : ""}{cell.expectancy.toFixed(2)}%
                              </Text>
                              <div style={{ fontSize: 11, color: "#6b7280" }}>胜{cell.winRate}%</div>
                            </div>
                          </Tooltip>
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {/* 明细表 */}
      {data && data.qualifiedSignals > 0 && (
        <Card size="small" title="全部组合明细">
          <Table
            rowKey={(r) => `${r.takeProfitPct}_${r.stopLossPct}`}
            columns={detailColumns}
            dataSource={data.cells}
            size="small"
            pagination={false}
            scroll={{ x: 1000 }}
          />
        </Card>
      )}

      {/* 概念与公式 */}
      <Card size="small" title="概念与计算公式说明">
        <Collapse
          defaultActiveKey={["flow"]}
          items={[
            {
              key: "flow",
              label: "回测做了什么（整体流程）",
              children: (
                <Paragraph style={{ marginBottom: 0 }}>
                  <ol style={{ paddingLeft: 18, lineHeight: 2 }}>
                    <li><b>挑信号</b>：遍历该「币种×周期」的历史 AI 预测，保留 <Text code>方向=long/short</Text>、<Text code>置信度 ≥ 下限</Text>、<Text code>预测幅度 ≥ 阈值</Text> 的信号。</li>
                    <li><b>开仓</b>：在预测对应 K 线的收盘价（参考价）按预测方向开仓。</li>
                    <li><b>持仓 & 出场</b>：向前看 N 根（持仓窗口）真实 K 线，先触发止盈或止损就以该价平仓；到期都没触发，则按窗口末根收盘价平仓。</li>
                    <li><b>扣成本</b>：每笔扣除开+平两次手续费及持仓期间资金费率。</li>
                    <li><b>聚合</b>：对每个「止盈×止损」组合统计胜率、盈亏比、单笔期望等，形成期望矩阵。</li>
                  </ol>
                </Paragraph>
              ),
            },
            {
              key: "concept",
              label: "关键概念",
              children: (
                <div style={{ lineHeight: 2 }}>
                  <p><b>预测幅度</b>：AI 预测价相对当前参考价的涨跌幅，<Text code>|predict_price − ref_price| / ref_price</Text>。它是开仓闸门，不是止盈目标。</p>
                  <p><b>方向正确率</b>：合格信号中，持仓窗口末收盘价方向与预测方向一致的占比。注意：<b>方向对 ≠ 涨够目标</b>，方向对仍可能因回撤先打止损而亏损。</p>
                  <p><b>名义收益 vs ROE</b>：矩阵里的收益都按「价格变动百分比（名义）」计，与杠杆无关；ROE = 名义收益 × 杠杆，仅用于直观感受保证金回报。</p>
                  <p><b>同根 K 线先后假设</b>：若同一根 K 线内止盈价、止损价都被触及，无法判定先后，<b>保守按先触止损处理</b>（结果偏悲观，更安全）。</p>
                </div>
              ),
            },
            {
              key: "formula",
              label: "计算公式",
              children: (
                <div style={{ lineHeight: 2.1, fontFamily: "var(--font-mono, monospace)", fontSize: 13 }}>
                  <p>开仓价 entry = 预测 K 线收盘价（参考价）</p>
                  <p>做多：止盈价 = entry × (1 + 止盈% / 100)，止损价 = entry × (1 − 止损% / 100)</p>
                  <p>做空：止盈价 = entry × (1 − 止盈% / 100)，止损价 = entry × (1 + 止损% / 100)</p>
                  <Divider style={{ margin: "8px 0" }} />
                  <p>单笔成本（名义%） = 2 × 手续费率 + 资金费率 × 持仓根数</p>
                  <p>单笔净收益 = 毛收益 − 单笔成本</p>
                  <p>　· 触止盈 → 毛收益 = +止盈%</p>
                  <p>　· 触止损 → 毛收益 = −止损%</p>
                  <p>　· 到期未触发 → 毛收益 = 方向 × (末根收盘价 − entry) / entry × 100</p>
                  <Divider style={{ margin: "8px 0" }} />
                  <p>胜率 = 净收益&gt;0 的笔数 / 总笔数</p>
                  <p>平均盈利 avgWin = Σ(盈利笔净收益) / 盈利笔数</p>
                  <p>平均亏损 avgLoss = Σ|亏损笔净收益| / 亏损笔数</p>
                  <p><b>盈亏比 = avgWin / avgLoss</b></p>
                  <p><b>单笔期望 = Σ(每笔净收益) / 总笔数</b>　← 最核心指标，&gt;0 才正期望</p>
                  <p>　等价于：胜率 × avgWin − (1 − 胜率) × avgLoss</p>
                  <p>盈利因子 = Σ(盈利) / Σ|亏损|，&gt;1 才赚钱</p>
                  <p>最大回撤 = 按信号顺序累计净收益曲线的峰值到谷值最大跌幅</p>
                </div>
              ),
            },
            {
              key: "caveat",
              label: "使用须知与局限",
              children: (
                <div style={{ lineHeight: 2 }}>
                  <p>· 回测是<b>机械执行固定止盈止损</b>的结果，未含滑点、未含实际下单深度/部分成交。</p>
                  <p>· 同根 K 线先后保守按止损优先，真实表现可能略好。</p>
                  <p>· 样本越少（合格信号数小）结论越不稳，建议合格信号 ≥ 100 再参考。</p>
                  <p>· 期望为正只是<b>必要条件</b>；还要看最大回撤是否可承受、样本是否跨越不同行情。</p>
                </div>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
