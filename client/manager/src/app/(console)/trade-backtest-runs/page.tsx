"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Card,
  DatePicker,
  Drawer,
  Empty,
  Form,
  Input,
  Modal,
  Radio,
  Segmented,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import { BarChartOutlined, PlusOutlined, ReloadOutlined, SwapOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import dayjs, { type Dayjs } from "dayjs";
import {
  createBacktestRun,
  fetchBacktestMetrics,
  fetchBacktestRunDetail,
  fetchBacktestRuns,
  fetchKlineRange,
  fetchPredictionDetail,
  fetchStrategyOptions,
  type BacktestMetric,
  type BacktestRun,
  type BacktestRunDetail,
  type BacktestTrade,
  type KlinePoint,
  type PredictionCandle,
  type PredictionDetail,
  type StrategyOption,
} from "./api/backtest.api";

const { Text, Title } = Typography;
const { RangePicker } = DatePicker;

// 时间段快速预设：点击即把范围设为「距今 N 时长 ~ 现在」。
const RANGE_PRESETS: { label: string; value: () => [Dayjs, Dayjs] }[] = [
  { label: "最近4小时", value: () => [dayjs().subtract(4, "hour"), dayjs()] },
  { label: "最近8小时", value: () => [dayjs().subtract(8, "hour"), dayjs()] },
  { label: "最近12小时", value: () => [dayjs().subtract(12, "hour"), dayjs()] },
  { label: "最近1天", value: () => [dayjs().subtract(1, "day"), dayjs()] },
  { label: "最近3天", value: () => [dayjs().subtract(3, "day"), dayjs()] },
  { label: "最近1周", value: () => [dayjs().subtract(7, "day"), dayjs()] },
];

const PREDICTION_INTERVALS = [
  { label: "1小时", value: "1h" },
  { label: "4小时", value: "4h" },
  { label: "12小时", value: "12h" },
  { label: "1日", value: "1d" },
];

const PRICE_INTERVALS = [
  { label: "1分钟", value: "1m" },
  { label: "5分钟", value: "5m" },
  { label: "15分钟", value: "15m" },
  { label: "1小时", value: "1h" },
  { label: "4小时", value: "4h" },
  { label: "1日", value: "1d" },
];

const TRADING_PERIODS = [
  { label: "1小时", value: "1h" },
  { label: "4小时", value: "4h" },
  { label: "12小时", value: "12h" },
  { label: "1日", value: "1d" },
];

const CALC_MODE_LABEL: Record<string, string> = {
  prediction: "预测周期",
  trading: "交易周期",
};

const STATUS_META: Record<string, { text: string; color: string }> = {
  pending: { text: "排队中", color: "default" },
  running: { text: "运行中", color: "processing" },
  done: { text: "已完成", color: "success" },
  failed: { text: "失败", color: "error" },
};

const REASON_META: Record<string, { text: string; color: string }> = {
  tp: { text: "止盈", color: "green" },
  sl: { text: "止损", color: "red" },
  timeout: { text: "超时", color: "orange" },
  expired: { text: "未成交", color: "default" },
};

function statusTag(status: string) {
  const meta = STATUS_META[status] ?? { text: status, color: "default" };
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

function pct(value: number) {
  if (!Number.isFinite(value)) return "-";
  return `${(value * 100).toFixed(1)}%`;
}

function money(value: number) {
  if (!Number.isFinite(value)) return "-";
  return value.toFixed(4);
}

function num(value: number, digits = 2) {
  if (!Number.isFinite(value)) return "-";
  return value.toFixed(digits);
}

// holdMaxAdverse 持仓期间最大亏损：取持仓内最不利价(多头看最低价、空头看最高价)相对成交价的逆向幅度。
// 返回 { pricePct, levPct, adversePx }，均为 ≤0 的百分比(亏损)；未成交/缺数据返回 null。
function holdMaxAdverse(t: BacktestTrade): { pricePct: number; levPct: number; adversePx: number } | null {
  const entry = t.openPrice;
  if (!(entry > 0)) return null;
  const adversePx = t.direction === "long" ? t.minPriceDuringHold : t.maxPriceDuringHold;
  if (!(adversePx > 0)) return null;
  const pricePct = ((t.direction === "long" ? adversePx - entry : entry - adversePx) / entry) * 100;
  const lev = t.leverage && t.leverage > 0 ? t.leverage : 1;
  return { pricePct, levPct: pricePct * lev, adversePx };
}

export default function TradeBacktestRunsPage() {
  const [form] = Form.useForm();
  const [strategies, setStrategies] = useState<StrategyOption[]>([]);
  const [runs, setRuns] = useState<BacktestRun[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selectedRunIds, setSelectedRunIds] = useState<number[]>([]);

  // 详情抽屉
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<BacktestRunDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // 对比弹窗
  const [compareOpen, setCompareOpen] = useState(false);
  const [compareMetrics, setCompareMetrics] = useState<BacktestMetric[]>([]);

  // 详情结算口径：全局切换 + 行级覆盖（仅当 run 选了交易周期时可切）
  const [calcMode, setCalcMode] = useState<string>("prediction");
  const [rowModes, setRowModes] = useState<Record<number, string>>({});

  // K线详情弹窗
  const [klineOpen, setKlineOpen] = useState(false);
  const [klineLoading, setKlineLoading] = useState(false);
  const [klineData, setKlineData] = useState<KlinePoint[]>([]);
  const [klineTrade, setKlineTrade] = useState<BacktestTrade | null>(null);
  // 预测增强：复合方向 + 预测周期 K 线（与 K 线并行拉取）
  const [predDetail, setPredDetail] = useState<PredictionDetail | null>(null);
  const [predLoading, setPredLoading] = useState(false); // 预测增强单独的加载态，区分「加载中」与「加载失败」
  const [predError, setPredError] = useState(false);

  const strategyMap = useMemo(() => {
    const m = new Map<number, StrategyOption>();
    strategies.forEach((s) => m.set(s.id, s));
    return m;
  }, [strategies]);

  const loadStrategies = useCallback(async () => {
    try {
      const res = await fetchStrategyOptions({ coinCode: "BTC" });
      setStrategies(res.list ?? []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "策略列表加载失败");
    }
  }, []);

  const loadRuns = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await fetchBacktestRuns({ page: 1, pageSize: 100 });
      setRuns(res.list ?? []);
    } catch (err) {
      if (!silent) message.error(err instanceof Error ? err.message : "回测任务加载失败");
    } finally {
      if (!silent) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadStrategies();
    void loadRuns();
  }, [loadStrategies, loadRuns]);

  // 有任务在排队/运行时，每 3s 静默刷新列表，直到全部完结。
  const hasPending = useMemo(() => runs.some((r) => r.status === "pending" || r.status === "running"), [runs]);
  useEffect(() => {
    if (!hasPending) return;
    const timer = window.setInterval(() => void loadRuns(true), 3000);
    return () => window.clearInterval(timer);
  }, [hasPending, loadRuns]);

  // autoBacktestName 任务名留空时自动命名：交易对/周期 -- 备注 -- 预测周期 -- 价格周期。
  const autoBacktestName = (values: {
    strategyId: number;
    predictionInterval: string;
    priceInterval: string;
  }) => {
    const s = strategyMap.get(values.strategyId);
    const segments = [
      s ? `${s.symbol}/${s.interval}` : undefined,
      s?.remark,
      values.predictionInterval,
      values.priceInterval,
    ].filter(Boolean);
    return segments.join(" -- ") || undefined;
  };

  const onCreate = async () => {
    try {
      const values = await form.validateFields();
      const range = values.range as [Dayjs, Dayjs];
      setSubmitting(true);
      await createBacktestRun({
        name: values.name?.trim() || autoBacktestName(values),
        platformCode: "binance",
        coinCode: "BTC",
        symbol: "BTCUSDT",
        predictionInterval: values.predictionInterval,
        priceInterval: values.priceInterval,
        tradingPeriod: values.tradingPeriod || undefined,
        startTime: range[0].format("YYYY-MM-DD HH:mm:ss"),
        endTime: range[1].format("YYYY-MM-DD HH:mm:ss"),
        strategyId: values.strategyId,
      });
      message.success("回测任务已提交，后台执行中");
      void loadRuns();
    } catch (err) {
      if (err instanceof Error) message.error(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const openDetail = async (id: number) => {
    setDetailOpen(true);
    setDetailLoading(true);
    setDetail(null);
    setCalcMode("prediction");
    setRowModes({});
    try {
      setDetail(await fetchBacktestRunDetail(id));
    } catch (err) {
      message.error(err instanceof Error ? err.message : "详情加载失败");
    } finally {
      setDetailLoading(false);
    }
  };

  // 打开某笔交易的「K线详情」：拉取该笔从信号到收尾(未成交则到数据末尾)区间内的 K 线。
  const openKlineDetail = async (trade: BacktestTrade) => {
    if (!detail) return;
    const run = detail.run;
    const start = trade.requestedAt;
    const end = trade.closedAt || run.klineEnd || run.endTime;
    setKlineTrade(trade);
    setKlineOpen(true);
    setKlineLoading(true);
    setKlineData([]);
    setPredDetail(null);
    setPredError(false);
    setPredLoading(true);
    // 入场价基准：成交价优先，未成交回退期望价，让各周期利润潜力可比。
    const entry = trade.openPrice > 0 ? trade.openPrice : trade.plannedEntryPrice;
    try {
      // 只拉价格周期(细，如1m/15m)走势；预测周期走势用「预测K线」单独展示。
      setKlineData(await fetchKlineRange({ symbol: run.symbol, interval: run.priceInterval, start, end }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : "K线加载失败");
    } finally {
      setKlineLoading(false);
    }
    // 预测增强独立拉取：失败不影响 K 线展示，置错误态供 UI 区分「加载中 / 加载失败」。
    try {
      setPredDetail(
        await fetchPredictionDetail({
          platform: run.platformCode,
          coin: run.coinCode,
          interval: run.predictionInterval,
          signal: trade.requestedAt,
          start,
          end,
          entry,
        }),
      );
    } catch {
      setPredDetail(null);
      setPredError(true);
    } finally {
      setPredLoading(false);
    }
  };

  const openCompare = async () => {
    if (selectedRunIds.length < 2) {
      message.warning("请至少勾选 2 个回测任务进行对比");
      return;
    }
    try {
      const metrics = await fetchBacktestMetrics(selectedRunIds);
      if (metrics.length === 0) {
        message.info("所选任务暂无已完成的指标");
        return;
      }
      setCompareMetrics(metrics);
      setCompareOpen(true);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "对比数据加载失败");
    }
  };

  const strategyLabel = (id: number) => {
    const s = strategyMap.get(id);
    if (!s) return `#${id}`;
    return `#${id} ${s.symbol}/${s.interval}${s.remark ? ` · ${s.remark}` : ""}`;
  };

  const runColumns: ColumnsType<BacktestRun> = [
    { title: "ID", dataIndex: "id", width: 64 },
    {
      title: "名称",
      dataIndex: "name",
      width: 140,
      render: (v: string) => v || <Text type="secondary">未命名</Text>,
    },
    { title: "交易对", dataIndex: "symbol", width: 96 },
    {
      title: "预测/价格周期",
      key: "interval",
      width: 150,
      render: (_: unknown, r: BacktestRun) => (
        <span>
          {r.predictionInterval} / {r.priceInterval}
          {r.tradingPeriod ? <Tag style={{ marginLeft: 6 }} color="purple">交易{r.tradingPeriod}</Tag> : null}
        </span>
      ),
    },
    {
      title: "策略",
      dataIndex: "strategyId",
      width: 180,
      render: (id: number) => <Text style={{ fontSize: 12 }}>{strategyLabel(id)}</Text>,
    },
    {
      title: "时间段",
      key: "range",
      width: 200,
      render: (_: unknown, r: BacktestRun) => (
        <Text style={{ fontSize: 12, color: "#8c8c8c" }}>
          {r.startTime} ~ {r.endTime}
        </Text>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 90,
      render: (s: string, r: BacktestRun) =>
        r.status === "failed" && r.errorMsg ? (
          <Tooltip title={r.errorMsg}>{statusTag(s)}</Tooltip>
        ) : (
          statusTag(s)
        ),
    },
    {
      title: "操作",
      key: "action",
      width: 90,
      fixed: "right",
      render: (_: unknown, r: BacktestRun) => (
        <Button type="link" size="small" disabled={r.status !== "done"} onClick={() => openDetail(r.id)}>
          查看
        </Button>
      ),
    },
  ];

  // 是否有交易周期口径（决定能否切换结算口径）。
  const hasTrading = !!detail?.run.tradingPeriod;

  const tradeColumns: ColumnsType<BacktestTrade> = [
    ...(hasTrading
      ? [
          {
            title: "口径",
            key: "calcMode",
            width: 116,
            fixed: "left" as const,
            render: (_: unknown, r: BacktestTrade) => (
              <Segmented
                size="small"
                value={rowModes[r.predictionId] ?? calcMode}
                onChange={(v) =>
                  setRowModes((prev) => ({ ...prev, [r.predictionId]: v as string }))
                }
                options={[
                  { label: "预测", value: "prediction" },
                  { label: "交易", value: "trading" },
                ]}
              />
            ),
          },
        ]
      : []),
    {
      title: "方向",
      dataIndex: "direction",
      width: 64,
      render: (d: string) => <Tag color={d === "long" ? "green" : "red"}>{d === "long" ? "多" : "空"}</Tag>,
    },
    {
      title: "预测周期",
      key: "predWindow",
      width: 180,
      render: (_: unknown, r: BacktestTrade) =>
        r.requestedAt || r.predictTime ? (
          <Tooltip title="该笔关联预测覆盖的时间窗：信号时刻 ~ 预测目标时刻(predict_time)">
            <Text style={{ fontSize: 12 }}>
              {r.requestedAt ? r.requestedAt.slice(5, 16) : "-"} ~ {r.predictTime ? r.predictTime.slice(5, 16) : "-"}
            </Text>
          </Tooltip>
        ) : (
          "-"
        ),
    },
    {
      title: "入场",
      dataIndex: "entryMode",
      width: 80,
      render: (m: string) => (m === "pullback" ? "回踩限价" : "市价"),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 80,
      render: (s: string) => {
        if (s === "expired") return <Tag color="default">未成交</Tag>;
        if (s === "open") return <Tag color="blue">持仓</Tag>;
        return <Tag color="default">{s}</Tag>;
      },
    },
    {
      title: "结果",
      dataIndex: "closeReason",
      width: 80,
      render: (r: string) => {
        if (!r) return "-";
        const meta = REASON_META[r] ?? { text: r, color: "default" };
        return <Tag color={meta.color}>{meta.text}</Tag>;
      },
    },
    {
      title: "预测区间",
      key: "predRange",
      width: 170,
      render: (_: unknown, r: BacktestTrade) =>
        r.predLow || r.predHigh ? (
          <Tooltip title="关联预测的区间下沿~上沿；期望价由它推导">
            <Text style={{ fontSize: 12 }}>
              {num(r.predLow, 2)} ~ {num(r.predHigh, 2)}
            </Text>
          </Tooltip>
        ) : (
          "-"
        ),
    },
    {
      title: "压力面(低~高)",
      key: "pressureRange",
      width: 180,
      render: (_: unknown, r: BacktestTrade) =>
        r.pressureLow || r.pressureHigh ? (
          <Tooltip title="本笔信号时刻最近一次压力面：最低=关键支撑 / 最高=关键阻力">
            <Text style={{ fontSize: 12 }}>
              {r.pressureLow ? num(r.pressureLow, 2) : "-"} ~ {r.pressureHigh ? num(r.pressureHigh, 2) : "-"}
            </Text>
          </Tooltip>
        ) : (
          <Text type="secondary">无</Text>
        ),
    },
    {
      title: "预测收盘",
      dataIndex: "predClose",
      width: 110,
      render: (v: number) => (v ? <Text type="secondary">{num(v, 2)}</Text> : "-"),
    },
    {
      title: "期望价",
      dataIndex: "plannedEntryPrice",
      width: 110,
      render: (v: number, r: BacktestTrade) =>
        r.entryMode === "pullback" && v ? (
          <Tooltip title="回踩限价挂单的目标价：价格触及才成交">{num(v, 2)}</Tooltip>
        ) : (
          "-"
        ),
    },
    { title: "成交价", dataIndex: "openPrice", width: 110, render: (v: number) => (v ? num(v, 2) : "-") },
    {
      title: "平仓价",
      dataIndex: "closePrice",
      width: 110,
      render: (v: number, r: BacktestTrade) => {
        if (v) return num(v, 2);
        // 持仓中：没有平仓价，用当前最新价(标记价)代替并标注「最新」，便于区分浮动口径。
        if (r.status === "open" && r.markPrice > 0) {
          return (
            <Tooltip title="当前最新价(标记价)：该笔仍在持仓，按最新行情标记浮动盈亏">
              <Text style={{ color: "#1677ff" }}>
                {num(r.markPrice, 2)} <Text type="secondary" style={{ fontSize: 11 }}>最新</Text>
              </Text>
            </Tooltip>
          );
        }
        return "-";
      },
    },
    {
      title: "区间(低~高)",
      key: "window",
      width: 170,
      render: (_: unknown, r: BacktestTrade) =>
        r.windowLow || r.windowHigh ? (
          <Text style={{ fontSize: 12 }}>
            {num(r.windowLow, 2)} ~ {num(r.windowHigh, 2)}
          </Text>
        ) : (
          "-"
        ),
    },
    {
      title: "区间开→收",
      key: "windowOC",
      width: 170,
      render: (_: unknown, r: BacktestTrade) =>
        r.windowOpen || r.windowClose ? (
          <Tooltip title="该区间实际开盘价 → 收盘价">
            <Text style={{ fontSize: 12 }}>
              {num(r.windowOpen, 2)} → {num(r.windowClose, 2)}
            </Text>
          </Tooltip>
        ) : (
          "-"
        ),
    },
    {
      title: "净盈亏",
      dataIndex: "netPnl",
      width: 120,
      render: (v: number, r: BacktestTrade) => {
        // 持仓中：用按最新价标记的浮动净盈亏，附「浮动」标识区分已实现盈亏。
        if (r.status === "open" && r.markPrice > 0) {
          const u = r.unrealizedNetPnl;
          return (
            <Tooltip title="按当前最新价标记的浮动净盈亏(含预估往返手续费)：该笔仍在持仓，尚未实现">
              <Text style={{ color: u > 0 ? "#3f8600" : u < 0 ? "#cf1322" : undefined }}>{money(u)}</Text>
              <Tag color="blue" style={{ marginLeft: 4 }}>浮动</Tag>
            </Tooltip>
          );
        }
        return (
          <Text style={{ color: v > 0 ? "#3f8600" : v < 0 ? "#cf1322" : undefined }}>{money(v)}</Text>
        );
      },
    },
    {
      title: "盈亏率%",
      dataIndex: "pnlRate",
      width: 100,
      render: (v: number, r: BacktestTrade) => {
        if (r.status === "closed") {
          return (
            <Tooltip title="含杠杆的盈亏率 = 价差/开仓价×杠杆×100（与张数无关，张数只影响净盈亏金额）">
              <Text style={{ color: v > 0 ? "#3f8600" : v < 0 ? "#cf1322" : undefined }}>
                {v > 0 ? "+" : ""}
                {num(v, 2)}%
              </Text>
            </Tooltip>
          );
        }
        // 持仓中：按最新价标记的浮动盈亏率(含杠杆)。
        if (r.status === "open" && r.markPrice > 0) {
          const u = r.unrealizedPnlRate;
          return (
            <Tooltip title="按当前最新价标记的浮动盈亏率(含杠杆) = (最新价-成交价)/成交价×杠杆×100，尚未实现">
              <Text style={{ color: u > 0 ? "#3f8600" : u < 0 ? "#cf1322" : undefined }}>
                {u > 0 ? "+" : ""}
                {num(u, 2)}%
              </Text>
            </Tooltip>
          );
        }
        return "-";
      },
    },
    {
      title: "持仓最大亏损%",
      key: "maxAdverse",
      width: 130,
      render: (_: unknown, r: BacktestTrade) => {
        const mae = holdMaxAdverse(r);
        if (!mae) return "-";
        return (
          <Tooltip title={`持仓期间逆向最大回撤(含杠杆) = 最不利价/成交价逆向幅度×杠杆。${r.direction === "long" ? "最低价" : "最高价"} ${num(mae.adversePx, 2)} · 成交价 ${num(r.openPrice, 2)} · 价格幅度 ${num(mae.pricePct, 2)}%`}>
            <Text style={{ color: mae.levPct < 0 ? "#cf1322" : undefined }}>{num(mae.levPct, 2)}%</Text>
          </Tooltip>
        );
      },
    },
    { title: "置信", dataIndex: "confidence", width: 70, render: (v: number) => num(v, 2) },
    { title: "效率", dataIndex: "efficiency", width: 70, render: (v: number) => num(v, 2) },
    { title: "成交时间", dataIndex: "openedAt", width: 150, render: (v: string) => v || "-" },
    {
      title: "K线",
      key: "klineAction",
      width: 80,
      fixed: "right",
      render: (_: unknown, r: BacktestTrade) => (
        <Button type="link" size="small" onClick={() => void openKlineDetail(r)}>
          K线详情
        </Button>
      ),
    },
  ];

  // 当前全局口径对应的汇总指标。
  const metric =
    detail?.metrics.find((m) => m.calcMode === calcMode) ?? detail?.metrics[0];

  // 逐笔按预测分组：每条预测一行，按「行级覆盖 ?? 全局口径」挑选要展示的那条结算结果。
  const displayedTrades = useMemo<BacktestTrade[]>(() => {
    const all = detail?.trades ?? [];
    const byPred = new Map<number, Record<string, BacktestTrade>>();
    const order: number[] = [];
    all.forEach((t) => {
      if (!byPred.has(t.predictionId)) {
        byPred.set(t.predictionId, {});
        order.push(t.predictionId);
      }
      byPred.get(t.predictionId)![t.calcMode || "prediction"] = t;
    });
    return order.map((pid) => {
      const group = byPred.get(pid)!;
      const mode = rowModes[pid] ?? calcMode;
      return group[mode] ?? group.prediction ?? Object.values(group)[0];
    });
  }, [detail?.trades, rowModes, calcMode]);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <Text className="manager-section-label">STRATEGY BACKTEST · COMPARE</Text>
        <Title level={3} style={{ margin: "4px 0 0" }}>
          回测对比
        </Title>
        <Text type="secondary">
          选时间段 + 预测周期 + 价格周期 + 策略，回放历史行情跑回测，落库后横向对比哪个策略更有效。
        </Text>
      </div>

      <Card size="small" title={<Space><PlusOutlined />新建回测任务</Space>}>
        <Form form={form} layout="inline" style={{ rowGap: 12, columnGap: 12, flexWrap: "wrap" }}>
          <Form.Item label="策略" name="strategyId" rules={[{ required: true, message: "请选择策略" }]}>
            <Select
              style={{ width: 240 }}
              placeholder={strategies.length ? "选择要回测的策略" : "暂无策略，请先创建"}
              options={strategies.map((s) => ({ value: s.id, label: strategyLabel(s.id) }))}
              showSearch
              optionFilterProp="label"
            />
          </Form.Item>
          <Form.Item label="预测周期" name="predictionInterval" initialValue="1h" rules={[{ required: true }]}>
            <Select style={{ width: 110 }} options={PREDICTION_INTERVALS} />
          </Form.Item>
          <Form.Item label="价格周期" name="priceInterval" initialValue="1m" rules={[{ required: true }]}>
            <Select style={{ width: 110 }} options={PRICE_INTERVALS} />
          </Form.Item>
          <Form.Item
            label="交易周期"
            name="tradingPeriod"
            tooltip="可选。选了则额外按交易周期再算一套：入场不变，只把持仓上限拉长到该周期(TP/SL 照常)，详情可切换查看。不选=仅按预测周期(现状)。"
          >
            <Select style={{ width: 120 }} options={TRADING_PERIODS} placeholder="不选=现状" allowClear />
          </Form.Item>
          <Form.Item label="时间段" name="range" rules={[{ required: true, message: "请选择时间段" }]}>
            <RangePicker showTime format="YYYY-MM-DD HH:mm" presets={RANGE_PRESETS} />
          </Form.Item>
          <Form.Item label="名称" name="name">
            <Input style={{ width: 220 }} placeholder="留空自动命名：交易对/周期--备注--预测--价格" allowClear />
          </Form.Item>
          <Form.Item>
            <Button type="primary" loading={submitting} onClick={onCreate}>
              开始回测
            </Button>
          </Form.Item>
        </Form>
      </Card>

      <Card
        size="small"
        title={<Space><BarChartOutlined />回测任务</Space>}
        extra={
          <Space>
            <Button icon={<SwapOutlined />} disabled={selectedRunIds.length < 2} onClick={openCompare}>
              对比选中 ({selectedRunIds.length})
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => void loadRuns()} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      >
        <Table<BacktestRun>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={runs}
          columns={runColumns}
          scroll={{ x: 1000 }}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
          rowSelection={{
            selectedRowKeys: selectedRunIds,
            onChange: (keys) => setSelectedRunIds(keys as number[]),
            getCheckboxProps: (r) => ({ disabled: r.status !== "done" }),
          }}
        />
      </Card>

      <Drawer
        title={detail ? `回测详情 · #${detail.run.id} ${detail.run.name || ""}` : "回测详情"}
        width={920}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        loading={detailLoading}
      >
        {detail?.run && (
          <div style={{ marginBottom: 16 }}>
            {hasTrading ? (
              <Space>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  结算口径
                </Text>
                <Radio.Group
                  size="small"
                  optionType="button"
                  buttonStyle="solid"
                  value={calcMode}
                  onChange={(e) => {
                    setCalcMode(e.target.value);
                    setRowModes({}); // 切全局口径时清掉行级覆盖
                  }}
                  options={[
                    { label: "按预测周期", value: "prediction" },
                    { label: `按交易周期(${detail.run.tradingPeriod})`, value: "trading" },
                  ]}
                />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  每行也可单独切换
                </Text>
              </Space>
            ) : (
              <Text type="secondary" style={{ fontSize: 12 }}>
                结算口径：按预测周期（未设置交易周期）
              </Text>
            )}
          </div>
        )}

        {metric ? (
          <>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(4, 1fr)",
                gap: 16,
                marginBottom: 16,
              }}
            >
              <Statistic
                title="净盈亏 (USDT)"
                value={money(metric.netPnl)}
                valueStyle={{ color: metric.netPnl >= 0 ? "#3f8600" : "#cf1322" }}
              />
              <Statistic title="单笔期望" value={money(metric.expectancy)} />
              <Statistic title="胜率" value={pct(metric.winRate)} />
              <Statistic title="成交率" value={pct(metric.fillRate)} />
              <Statistic title="盈亏比" value={num(metric.profitFactor, 2)} />
              <Statistic title="最大回撤" value={money(metric.maxDrawdown)} />
              <Statistic title="夏普" value={num(metric.sharpe, 2)} />
              <Statistic title="平均持仓(分)" value={num(metric.avgHoldSecs / 60, 1)} />
            </div>
            <Space style={{ marginBottom: 16 }} wrap>
              <Tag>信号 {metric.tradeCount}</Tag>
              <Tag color="blue">成交 {metric.fillCount}</Tag>
              <Tag>未成交 {metric.expiredCount}</Tag>
              <Tag color="green">止盈 {metric.tpCount}</Tag>
              <Tag color="red">止损 {metric.slCount}</Tag>
              <Tag color="orange">超时 {metric.timeoutCount}</Tag>
            </Space>
          </>
        ) : detail ? (
          <Text type="secondary">该任务暂无汇总指标。</Text>
        ) : null}

        {detail?.run && (
          <Text
            type={detail.run.klineCount > 0 ? "secondary" : "danger"}
            style={{ display: "block", marginBottom: 12, fontSize: 12 }}
          >
            K线数据：
            {detail.run.klineCount > 0
              ? `回放 ${detail.run.klineCount} 根 · 实际区间 ${detail.run.klineStart} ~ ${detail.run.klineEnd}`
              : "无（该时间段未取到价格K线，无法回放）"}
          </Text>
        )}

        <Table<BacktestTrade>
          rowKey="predictionId"
          size="small"
          dataSource={displayedTrades}
          columns={tradeColumns}
          scroll={{ x: 1000 }}
          pagination={{ pageSize: 20, size: "small" }}
        />
      </Drawer>

      <Modal
        title="策略横向对比"
        open={compareOpen}
        onCancel={() => setCompareOpen(false)}
        footer={null}
        width={1000}
      >
        <CompareTable metrics={compareMetrics} runs={runs} strategyLabel={strategyLabel} />
      </Modal>

      <Modal
        title={
          klineTrade
            ? `K线详情 · ${detail?.run.symbol ?? ""}/${detail?.run.priceInterval ?? ""} · ${klineTrade.direction === "long" ? "多" : "空"}`
            : "K线详情"
        }
        open={klineOpen}
        onCancel={() => setKlineOpen(false)}
        footer={null}
        width={1040}
      >
        <KlineDetail
          loading={klineLoading}
          data={klineData}
          priceInterval={detail?.run.priceInterval ?? ""}
          tradingPeriod={detail?.run.tradingPeriod ?? ""}
          predictionInterval={detail?.run.predictionInterval ?? ""}
          trade={klineTrade}
          predDetail={predDetail}
          predLoading={predLoading}
          predError={predError}
        />
      </Modal>
    </div>
  );
}

// CompareTable 把多个 run 的指标按净利降序并排，最高一行高亮——一眼看出哪个策略更有效。
function CompareTable({
  metrics,
  runs,
  strategyLabel,
}: {
  metrics: BacktestMetric[];
  runs: BacktestRun[];
  strategyLabel: (id: number) => string;
}) {
  const runMap = useMemo(() => {
    const m = new Map<number, BacktestRun>();
    runs.forEach((r) => m.set(r.id, r));
    return m;
  }, [runs]);

  const rows = useMemo(() => [...metrics].sort((a, b) => b.netPnl - a.netPnl), [metrics]);
  const bestRunId = rows[0]?.runId;

  const columns: ColumnsType<BacktestMetric> = [
    {
      title: "任务",
      dataIndex: "runId",
      render: (id: number) => {
        const run = runMap.get(id);
        return (
          <div>
            <div>#{id} {run?.name || ""}</div>
            <Text style={{ fontSize: 12, color: "#8c8c8c" }}>{run ? strategyLabel(run.strategyId) : ""}</Text>
          </div>
        );
      },
    },
    {
      title: "净盈亏",
      dataIndex: "netPnl",
      render: (v: number) => (
        <Text strong style={{ color: v >= 0 ? "#3f8600" : "#cf1322" }}>{money(v)}</Text>
      ),
    },
    { title: "期望", dataIndex: "expectancy", render: (v: number) => money(v) },
    { title: "胜率", dataIndex: "winRate", render: (v: number) => pct(v) },
    { title: "成交率", dataIndex: "fillRate", render: (v: number) => pct(v) },
    { title: "盈亏比", dataIndex: "profitFactor", render: (v: number) => num(v, 2) },
    { title: "最大回撤", dataIndex: "maxDrawdown", render: (v: number) => money(v) },
    { title: "夏普", dataIndex: "sharpe", render: (v: number) => num(v, 2) },
    { title: "成交/信号", key: "fill", render: (_: unknown, m: BacktestMetric) => `${m.fillCount}/${m.tradeCount}` },
  ];

  return (
    <Table<BacktestMetric>
      rowKey="runId"
      size="small"
      dataSource={rows}
      columns={columns}
      pagination={false}
      rowClassName={(r) => (r.runId === bestRunId ? "manager-row-highlight" : "")}
    />
  );
}

// 各预测周期的标识色(用于区分不同周期的预测 K 线)。
const PRED_PERIOD_COLOR: Record<string, string> = {
  "1h": "#722ed1",
  "4h": "#fa8c16",
  "12h": "#13c2c2",
  "1d": "#1677ff",
};
const PRED_PERIOD_LABEL: Record<string, string> = {
  "1h": "1小时",
  "4h": "4小时",
  "12h": "12小时",
  "1d": "1日",
};
// dirText 方向中文 + 颜色。
function dirText(d: string): { label: string; color: string } {
  if (d === "long") return { label: "多", color: "#3f8600" };
  if (d === "short") return { label: "空", color: "#cf1322" };
  return { label: "中性", color: "#8c8c8c" };
}

// predCandleToKline 把「预测 K 线」转成图表用的 KlinePoint(时间取发起时刻=该周期开盘)。
function predCandleToKline(c: PredictionCandle): KlinePoint {
  return { time: c.openTime, open: c.open, high: c.high, low: c.low, close: c.close, volume: 0 };
}

// CompositeDirectionPanel 复合方向：对各高周期(>本周期)用「预测极值×置信度」估利润，胜出者定最终方向。
function CompositeDirectionPanel({ composite }: { composite: PredictionDetail["composite"] }) {
  const rows = composite.rows ?? [];
  if (rows.length === 0) return null;
  const rec = dirText(composite.recommendedDirection);
  const own = dirText(composite.ownDirection);
  const hasRec = !!composite.recommendedDirection;

  return (
    <div style={{ border: "1px solid #303030", borderRadius: 6, padding: "8px 12px", display: "flex", flexDirection: "column", gap: 6 }}>
      <Space size={8} wrap>
        <Text strong style={{ fontSize: 13 }}>复合方向</Text>
        {hasRec ? (
          <>
            <Tag color={composite.recommendedDirection === "long" ? "green" : "red"} style={{ fontWeight: 600 }}>
              建议 {rec.label}
            </Tag>
            <Text type="secondary" style={{ fontSize: 12 }}>
              由 {PRED_PERIOD_LABEL[composite.dominantInterval] ?? composite.dominantInterval} 主导
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>·</Text>
            <Text style={{ fontSize: 12, color: composite.agree ? "#52c41a" : "#fa8c16" }}>
              {composite.agree ? "与本笔方向一致 ✓" : `与本笔方向冲突(本笔 ${own.label})`}
            </Text>
          </>
        ) : (
          <Text type="secondary" style={{ fontSize: 12 }}>高周期暂无预测，无法给出复合方向</Text>
        )}
        {composite.entry > 0 && (
          <Text type="secondary" style={{ fontSize: 12 }}>· 入场基准 {num(composite.entry, 2)}</Text>
        )}
      </Space>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
        <thead>
          <tr style={{ color: "#8c8c8c", textAlign: "right" }}>
            <th style={{ textAlign: "left", fontWeight: 400, padding: "2px 6px" }}>周期</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>方向</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>置信度</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>预测区间(低~高)</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>距上限/距下限</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>有利极值</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>利润潜力</th>
            <th style={{ fontWeight: 400, padding: "2px 6px" }}>加权得分</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const d = dirText(r.direction);
            return (
              <tr
                key={r.interval}
                style={{ textAlign: "right", background: r.dominant ? "rgba(82,196,26,0.12)" : undefined }}
              >
                <td style={{ textAlign: "left", padding: "2px 6px", color: PRED_PERIOD_COLOR[r.interval] }}>
                  ● {PRED_PERIOD_LABEL[r.interval] ?? r.interval}
                  {r.dominant ? " ★" : ""}
                </td>
                <td style={{ padding: "2px 6px", color: d.color }}>{r.hasData ? d.label : "-"}</td>
                <td style={{ padding: "2px 6px" }}>{r.hasData ? num(r.confidence, 2) : "-"}</td>
                <td style={{ padding: "2px 6px" }}>
                  {r.hasData && (r.predLow > 0 || r.predHigh > 0)
                    ? `${r.predLow > 0 ? num(r.predLow, 2) : "-"} ~ ${r.predHigh > 0 ? num(r.predHigh, 2) : "-"}`
                    : "-"}
                </td>
                <td style={{ padding: "2px 6px" }}>
                  {r.hasData && composite.entry > 0 ? (
                    <Tooltip title={`相对入场基准 ${num(composite.entry, 2)}：距预测上限/下限的幅度(预测区间×杠杆前)`}>
                      <span>
                        <Text style={{ fontSize: 12, color: "#52c41a" }}>
                          {r.predHigh > 0 ? `+${num(((r.predHigh - composite.entry) / composite.entry) * 100, 2)}%` : "-"}
                        </Text>
                        <Text type="secondary" style={{ fontSize: 12 }}> / </Text>
                        <Text style={{ fontSize: 12, color: "#ff4d4f" }}>
                          {r.predLow > 0 ? `${num(((r.predLow - composite.entry) / composite.entry) * 100, 2)}%` : "-"}
                        </Text>
                      </span>
                    </Tooltip>
                  ) : (
                    "-"
                  )}
                </td>
                <td style={{ padding: "2px 6px" }}>{r.hasData && r.favorableExtreme > 0 ? num(r.favorableExtreme, 2) : "-"}</td>
                <td style={{ padding: "2px 6px" }}>{r.hasData ? `${num(r.profitPct, 2)}%` : "-"}</td>
                <td style={{ padding: "2px 6px", fontWeight: r.dominant ? 600 : 400 }}>{r.hasData ? num(r.score, 3) : "-"}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <Text type="secondary" style={{ fontSize: 11 }}>
        距上限/距下限 = 预测区间上沿、下沿相对入场基准的幅度%；利润潜力 = 多看预测最高、空看预测最低相对入场价的有利空间%；加权得分 = 利润潜力 × 置信度，最高者定方向。
      </Text>
    </div>
  );
}

// PredictionCharts 预测周期 K 线区：两张图——①本周期的预测K线(开盘→交易周期末)；
// ②各高周期(>本周期)的预测K线合并到一张柱状图，悬停看 开/收/高/低/开盘收盘时间。
function PredictionCharts({
  predDetail,
  trade,
}: {
  predDetail: PredictionDetail;
  trade: BacktestTrade | null;
}) {
  const own = predDetail.ownSeries;
  const ownColor = PRED_PERIOD_COLOR[own.interval] ?? "#722ed1";
  const ownCandles = own.candles.map(predCandleToKline);
  const highers = (predDetail.higherSeries ?? []).filter((s) => s.candles.length > 0);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 4 }}>
      <div style={{ borderLeft: `3px solid ${ownColor}`, paddingLeft: 8 }}>
        <Text style={{ fontSize: 12, color: ownColor }}>
          ● 预测周期 K 线（{PRED_PERIOD_LABEL[own.interval] ?? own.interval}）· 预测值 · 共 {ownCandles.length} 根（开盘 → 交易周期末）
        </Text>
        {ownCandles.length ? (
          <KlineChart data={ownCandles} trade={trade} />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该窗口无本周期预测" />
        )}
      </div>
      <div style={{ borderLeft: "3px solid #595959", paddingLeft: 8 }}>
        <Text style={{ fontSize: 12, color: "#bfbfbf" }}>
          ● 高周期预测 K 线（{highers.map((s) => PRED_PERIOD_LABEL[s.interval] ?? s.interval).join(" / ") || "无"}）· 含预测开始之前那根
        </Text>
        {highers.length ? (
          <HigherPredChart series={highers} />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无高周期预测" />
        )}
      </div>
    </div>
  );
}

// HigherPredChart 把多个高周期的预测K线合并到一张图：各候选按时间排列成蜡烛，按周期着色，
// 悬停显示 周期/开/收/高/低/开盘时间/收盘时间。
function HigherPredChart({ series }: { series: PredictionDetail["higherSeries"] }) {
  const [hover, setHover] = useState<number | null>(null);
  // 展平：每根候选带上其周期，按开盘时间升序排列。
  const items = useMemo(() => {
    const flat = series.flatMap((s) => s.candles.map((c) => ({ ...c, interval: s.interval })));
    return flat.sort((a, b) => (a.openTime < b.openTime ? -1 : a.openTime > b.openTime ? 1 : 0));
  }, [series]);

  const W = 1000;
  const H = 300;
  const padL = 64;
  const padR = 16;
  const padT = 16;
  const padB = 28;
  const chartW = W - padL - padR;
  const chartH = H - padT - padB;

  if (items.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无高周期预测" />;

  const prices: number[] = [];
  items.forEach((k) => prices.push(k.high, k.low, k.open, k.close));
  let min = Math.min(...prices.filter((p) => p > 0));
  let max = Math.max(...prices);
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) {
    min = (min || 1) - 1;
    max = (max || 1) + 1;
  }
  const padP = (max - min) * 0.05;
  min -= padP;
  max += padP;
  const y = (p: number) => padT + ((max - p) / (max - min)) * chartH;
  const slot = chartW / items.length;
  const bodyW = Math.max(2, Math.min(slot * 0.6, 22));

  const presentIntervals = Array.from(new Set(items.map((i) => i.interval)));

  return (
    <div style={{ width: "100%" }}>
      <Space size={12} wrap style={{ marginBottom: 2 }}>
        {presentIntervals.map((it) => (
          <span key={it} style={{ fontSize: 11, color: PRED_PERIOD_COLOR[it] ?? "#8c8c8c" }}>
            ● {PRED_PERIOD_LABEL[it] ?? it}
          </span>
        ))}
      </Space>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" style={{ display: "block" }}>
        <rect x={padL} y={padT} width={chartW} height={chartH} fill="none" stroke="#303030" />
        {[max, (max + min) / 2, min].map((p, i) => (
          <g key={`g-${i}`}>
            <line x1={padL} y1={y(p)} x2={padL + chartW} y2={y(p)} stroke="#1f1f1f" />
            <text x={padL - 6} y={y(p) + 3} textAnchor="end" fontSize="10" fill="#8c8c8c">
              {p.toFixed(1)}
            </text>
          </g>
        ))}
        {items.map((k, i) => {
          const cx = padL + (i + 0.5) * slot;
          const color = PRED_PERIOD_COLOR[k.interval] ?? "#8c8c8c";
          const up = k.close >= k.open;
          const top = Math.min(y(k.open), y(k.close));
          const hgt = Math.max(1, Math.abs(y(k.close) - y(k.open)));
          return (
            <g key={`hc-${i}`}>
              <line x1={cx} y1={y(k.high)} x2={cx} y2={y(k.low)} stroke={color} strokeWidth={1} />
              <rect
                x={cx - bodyW / 2}
                y={top}
                width={bodyW}
                height={hgt}
                fill={up ? color : "#141414"}
                stroke={color}
                strokeWidth={1.2}
              />
              <rect
                x={padL + i * slot}
                y={padT}
                width={slot}
                height={chartH}
                fill="transparent"
                onMouseEnter={() => setHover(i)}
                onMouseLeave={() => setHover((h) => (h === i ? null : h))}
              />
            </g>
          );
        })}
        <text x={padL} y={H - 8} fontSize="10" fill="#8c8c8c">
          {items[0].openTime.slice(5, 16)}
        </text>
        <text x={padL + chartW} y={H - 8} textAnchor="end" fontSize="10" fill="#8c8c8c">
          {items[items.length - 1].closeTime.slice(5, 16)}
        </text>
        {hover != null && items[hover] && (() => {
          const k = items[hover];
          const cx = padL + (hover + 0.5) * slot;
          const boxW = 188;
          const boxH = 118;
          const boxX = cx < padL + chartW / 2 ? cx + 10 : cx - 10 - boxW;
          const boxY = padT + 6;
          const up = k.close >= k.open;
          const dt = dirText(k.trend);
          const rows: { label: string; value: string; color: string }[] = [
            { label: "周期", value: `${PRED_PERIOD_LABEL[k.interval] ?? k.interval} · ${dt.label}`, color: PRED_PERIOD_COLOR[k.interval] ?? "#bfbfbf" },
            { label: "开", value: num(k.open, 2), color: "#bfbfbf" },
            { label: "高", value: num(k.high, 2), color: "#3f8600" },
            { label: "低", value: num(k.low, 2), color: "#cf1322" },
            { label: "收", value: num(k.close, 2), color: up ? "#3f8600" : "#cf1322" },
            { label: "开盘时间", value: k.openTime.slice(5, 16), color: "#8c8c8c" },
            { label: "收盘时间", value: k.closeTime.slice(5, 16), color: "#8c8c8c" },
          ];
          return (
            <g pointerEvents="none">
              <rect x={padL + hover * slot} y={padT} width={slot} height={chartH} fill="#ffffff" fillOpacity={0.06} />
              <line x1={cx} y1={padT} x2={cx} y2={padT + chartH} stroke="#595959" strokeWidth={0.6} />
              <rect x={boxX} y={boxY} width={boxW} height={boxH} rx={3} fill="#000000" fillOpacity={0.85} stroke="#434343" />
              {rows.map((r, ri) => (
                <g key={`htt-${ri}`}>
                  <text x={boxX + 8} y={boxY + 17 + ri * 15} fontSize="10" fill="#8c8c8c">
                    {r.label}
                  </text>
                  <text x={boxX + boxW - 8} y={boxY + 17 + ri * 15} textAnchor="end" fontSize="10" fill={r.color}>
                    {r.value}
                  </text>
                </g>
              ))}
            </g>
          );
        })()}
      </svg>
    </div>
  );
}

// KlineDetail 某笔交易窗口内的 K 线情况：K 线图(叠加预测区间/期望价/成交平仓参考线) + OHLC 明细表。
function KlineDetail({
  loading,
  data,
  priceInterval,
  tradingPeriod,
  predictionInterval,
  trade,
  predDetail,
  predLoading,
  predError,
}: {
  loading: boolean;
  data: KlinePoint[];
  priceInterval: string;
  tradingPeriod: string;
  predictionInterval: string;
  trade: BacktestTrade | null;
  predDetail: PredictionDetail | null;
  predLoading: boolean;
  predError: boolean;
}) {
  if (loading) {
    return (
      <div style={{ textAlign: "center", padding: 48 }}>
        <Spin />
      </div>
    );
  }
  if (!data.length) {
    return <Empty description="该区间无K线数据（可能未回填，或该时间段无行情）" />;
  }
  const first = data[0];
  const last = data[data.length - 1];
  const low = Math.min(...data.map((k) => k.low));
  const high = Math.max(...data.map((k) => k.high));

  // 持仓期间最高浮盈：多头看持仓最高价、空头看持仓最低价相对成交价的有利幅度。
  // 优先展示含杠杆口径(与策略兜底%、列表盈亏率%同口径)，括号里附价格幅度。
  const entry = trade?.openPrice ?? 0;
  const peakPx = trade ? (trade.direction === "long" ? trade.maxPriceDuringHold : trade.minPriceDuringHold) : 0;
  const maxProfitPct = entry > 0 && peakPx > 0 ? ((trade!.direction === "long" ? peakPx - entry : entry - peakPx) / entry) * 100 : null;
  const lev = trade?.leverage && trade.leverage > 0 ? trade.leverage : 1;
  const maxProfitLevPct = maxProfitPct !== null ? maxProfitPct * lev : null;
  // 持仓期间最大亏损：多头看持仓最低价、空头看持仓最高价相对成交价的逆向幅度(同口径含杠杆)。
  const maxAdverse = trade ? holdMaxAdverse(trade) : null;
  // 交易周期预测区间：选了交易周期且与预测周期不同时，取复合方向里该周期那条预测的上下沿(B 口径止盈止损依据)。
  const tpBandRow =
    tradingPeriod && tradingPeriod !== predictionInterval
      ? predDetail?.composite.rows.find((r) => r.interval === tradingPeriod)
      : undefined;

  const columns: ColumnsType<KlinePoint> = [
    { title: "时间", dataIndex: "time", width: 150, render: (v: string) => v?.slice(5, 16) },
    { title: "开", dataIndex: "open", width: 96, render: (v: number) => num(v, 2) },
    { title: "高", dataIndex: "high", width: 96, render: (v: number) => num(v, 2) },
    { title: "低", dataIndex: "low", width: 96, render: (v: number) => num(v, 2) },
    {
      title: "收",
      dataIndex: "close",
      width: 96,
      render: (v: number, r: KlinePoint) => (
        <Text style={{ color: r.close >= r.open ? "#3f8600" : "#cf1322" }}>{num(v, 2)}</Text>
      ),
    },
    { title: "量", dataIndex: "volume", width: 110, render: (v: number) => num(v, 3) },
  ];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {predDetail ? (
        <CompositeDirectionPanel composite={predDetail.composite} />
      ) : predLoading ? (
        <div style={{ border: "1px solid #303030", borderRadius: 6, padding: "8px 12px" }}>
          <Space size={8}>
            <Spin size="small" />
            <Text type="secondary" style={{ fontSize: 12 }}>复合方向加载中…</Text>
          </Space>
        </div>
      ) : predError ? (
        <div style={{ border: "1px solid #303030", borderRadius: 6, padding: "8px 12px" }}>
          <Text type="secondary" style={{ fontSize: 12 }}>复合方向加载失败（高周期预测增强接口异常）</Text>
        </div>
      ) : null}
      <Text type="secondary" style={{ fontSize: 12 }}>
        开盘 {num(first.open, 2)} → 收盘 {num(last.close, 2)} · 区间 {num(low, 2)} ~ {num(high, 2)} · 共{" "}
        {data.length} 根 · {first.time} ~ {last.time}
      </Text>
      {trade && (trade.requestedAt || trade.predictTime) && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          预测时间段 ·{" "}
          <Text style={{ color: "#722ed1", fontSize: 12 }}>
            {trade.requestedAt || "-"} ~ {trade.predictTime || "-"}
          </Text>{" "}
          （信号时刻 ~ 预测目标时刻）
        </Text>
      )}
      {trade && (trade.predLow > 0 || trade.predHigh > 0) && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          预测区间 ·{" "}
          <Text style={{ color: "#8c8c8c", fontSize: 12 }}>
            {trade.predLow ? num(trade.predLow, 2) : "-"} ~ {trade.predHigh ? num(trade.predHigh, 2) : "-"}
          </Text>{" "}
          （区间下沿 ~ 上沿；预测收盘 {trade.predClose ? num(trade.predClose, 2) : "-"}）
        </Text>
      )}
      {tradingPeriod && tradingPeriod !== predictionInterval && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          交易周期预测区间（{PRED_PERIOD_LABEL[tradingPeriod] ?? tradingPeriod}）·{" "}
          {tpBandRow?.hasData && (tpBandRow.predLow > 0 || tpBandRow.predHigh > 0) ? (
            <Text style={{ color: "#722ed1", fontSize: 12 }}>
              {tpBandRow.predLow > 0 ? num(tpBandRow.predLow, 2) : "-"} ~{" "}
              {tpBandRow.predHigh > 0 ? num(tpBandRow.predHigh, 2) : "-"}
            </Text>
          ) : (
            <Text type="secondary" style={{ fontSize: 12 }}>暂无该周期预测</Text>
          )}{" "}
          （交易周期口径下止盈止损的依据；入场仍按预测周期信号）
        </Text>
      )}
      {trade && (trade.pressureLow > 0 || trade.pressureHigh > 0) && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          压力面 · 关键支撑(最低){" "}
          <Text style={{ color: "#13c2c2", fontSize: 12 }}>{trade.pressureLow ? num(trade.pressureLow, 2) : "-"}</Text>{" "}
          ~ 关键阻力(最高){" "}
          <Text style={{ color: "#fa8c16", fontSize: 12 }}>{trade.pressureHigh ? num(trade.pressureHigh, 2) : "-"}</Text>
        </Text>
      )}
      {maxProfitPct !== null && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          持仓期间最高浮盈曾达{" "}
          <Text style={{ color: "#52c41a", fontSize: 12 }}>{num(maxProfitLevPct!, 2)}%</Text>
          （含{num(lev, 0)}x杠杆；价格幅度 {num(maxProfitPct, 2)}%，{trade!.direction === "long" ? "最高价" : "最低价"}{" "}
          {num(peakPx, 2)} · 成交价 {num(entry, 2)}）
        </Text>
      )}
      {trade && maxAdverse && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          持仓期间最大亏损曾达{" "}
          <Text style={{ color: "#cf1322", fontSize: 12 }}>{num(maxAdverse.levPct, 2)}%</Text>
          （含{num(lev, 0)}x杠杆；价格幅度 {num(maxAdverse.pricePct, 2)}%，{trade.direction === "long" ? "最低价" : "最高价"}{" "}
          {num(maxAdverse.adversePx, 2)} · 成交价 {num(entry, 2)}）
        </Text>
      )}
      <Text type="secondary" style={{ fontSize: 12 }}>
        价格周期 K 线（{priceInterval || "-"}）
      </Text>
      <KlineChart data={data} trade={trade} />
      {predDetail ? (
        <PredictionCharts predDetail={predDetail} trade={trade} />
      ) : predLoading ? (
        <Space size={8}>
          <Spin size="small" />
          <Text type="secondary" style={{ fontSize: 12 }}>预测周期 K 线加载中…</Text>
        </Space>
      ) : (
        <Text type="secondary" style={{ fontSize: 12 }}>
          预测周期 K 线加载失败（该窗口暂无预测数据或接口异常）
        </Text>
      )}
      <Table<KlinePoint>
        rowKey="time"
        size="small"
        columns={columns}
        dataSource={data}
        pagination={{ pageSize: 50, size: "small", hideOnSinglePage: true }}
        scroll={{ y: 240 }}
      />
    </div>
  );
}

// KlineChart 纯 SVG 蜡烛图：绿涨红跌，叠加预测区间/期望价/成交平仓价参考线，直观看「预测 vs 实际」。
function KlineChart({ data, trade }: { data: KlinePoint[]; trade: BacktestTrade | null }) {
  const [hover, setHover] = useState<number | null>(null);
  const W = 1000;
  const H = 360;
  const padL = 64;
  const padR = 16;
  const padT = 16;
  const padB = 28;
  const chartW = W - padL - padR;
  const chartH = H - padT - padB;

  const overlays = [
    { v: trade?.predHigh ?? 0, color: "#8c8c8c", label: "预测上沿", dash: "4 3" },
    { v: trade?.predLow ?? 0, color: "#8c8c8c", label: "预测下沿", dash: "4 3" },
    { v: trade?.plannedEntryPrice ?? 0, color: "#1677ff", label: "期望价", dash: "5 3" },
    { v: trade?.openPrice ?? 0, color: "#3f8600", label: "成交价", dash: "" },
    { v: trade?.closePrice ?? 0, color: "#cf1322", label: "平仓价", dash: "" },
    { v: trade?.takeProfitPrice ?? 0, color: "#52c41a", label: "止盈价", dash: "6 3" },
    { v: trade?.stopLossPrice ?? 0, color: "#ff4d4f", label: "止损价", dash: "6 3" },
    { v: trade?.pressureHigh ?? 0, color: "#fa8c16", label: "压力面阻力", dash: "2 4" },
    { v: trade?.pressureLow ?? 0, color: "#13c2c2", label: "压力面支撑", dash: "2 4" },
  ].filter((o) => o.v > 0);

  const prices: number[] = [];
  data.forEach((k) => prices.push(k.high, k.low));
  overlays.forEach((o) => prices.push(o.v));
  let min = Math.min(...prices);
  let max = Math.max(...prices);
  if (min === max) {
    min -= 1;
    max += 1;
  }
  const padP = (max - min) * 0.05;
  min -= padP;
  max += padP;

  const y = (p: number) => padT + ((max - p) / (max - min)) * chartH;
  const slot = chartW / data.length;
  const bodyW = Math.max(1, Math.min(slot * 0.7, 14));

  // 平仓时刻在 x 轴的位置：定位触发平仓那根 K 线(time===closedAt，否则取 ≤closedAt 的最后一根)。
  const closeAt = trade?.closedAt ?? "";
  let closeIdx = -1;
  if (closeAt) {
    closeIdx = data.findIndex((k) => k.time === closeAt);
    if (closeIdx < 0) {
      for (let i = 0; i < data.length; i++) {
        if (data[i].time <= closeAt) closeIdx = i;
      }
    }
  }
  const closeX = closeIdx >= 0 ? padL + (closeIdx + 0.5) * slot : -1;

  // 预测目标时刻在 x 轴的位置：定位 predict_time 那根 K 线(否则取 ≤predictTime 的最后一根)，
  // 与信号时刻(数据起点)一起框定本笔关联预测覆盖的时间窗。
  const predictAt = trade?.predictTime ?? "";
  let predIdx = -1;
  if (predictAt) {
    predIdx = data.findIndex((k) => k.time === predictAt);
    if (predIdx < 0) {
      for (let i = 0; i < data.length; i++) {
        if (data[i].time <= predictAt) predIdx = i;
      }
    }
  }
  const predX = predIdx >= 0 ? padL + (predIdx + 0.5) * slot : -1;

  // 利润线：以成交价(未成交则期望价)为盈亏分界。做空→线下方为盈利区、做多→线上方为盈利区(浅绿)。
  const dir = trade?.direction ?? "";
  const entryPx = (trade?.openPrice ?? 0) > 0 ? (trade?.openPrice ?? 0) : trade?.plannedEntryPrice ?? 0;
  const profitPx = entryPx > 0 && (dir === "long" || dir === "short") ? entryPx : 0;
  const yProfit = profitPx > 0 ? y(profitPx) : 0;
  const profitArea =
    profitPx > 0
      ? dir === "short"
        ? { y: yProfit, h: padT + chartH - yProfit } // 空头：利润线下方到底部
        : { y: padT, h: yProfit - padT } // 多头：顶部到利润线上方
      : null;

  // 贴线标签防重叠：价位相近时这些参考线的文字会叠成一团。
  // 做法：把右侧标签按价位(y)排序，自上而下强制最小行距错开，再自下而上回推保证落在可视区内；
  // 文字被挪离原价位时用一条同色引导线连回，并垫一层深色底，叠在蜡烛/网格上也能看清。
  const labelGap = 13;
  const labelTop = padT + 6;
  const labelBot = padT + chartH - 2;
  const overlayLabels = overlays
    .map((o) => ({ ...o, lineY: y(o.v), labelY: y(o.v) }))
    .sort((a, b) => a.lineY - b.lineY);
  let prevLabelY = labelTop - labelGap;
  for (const l of overlayLabels) {
    l.labelY = Math.max(l.labelY, prevLabelY + labelGap);
    prevLabelY = l.labelY;
  }
  let nextLabelY = labelBot + labelGap;
  for (let i = overlayLabels.length - 1; i >= 0; i--) {
    overlayLabels[i].labelY = Math.min(overlayLabels[i].labelY, nextLabelY - labelGap);
    nextLabelY = overlayLabels[i].labelY;
  }
  const labelX = padL + chartW - 4;

  return (
    <div style={{ width: "100%" }}>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" style={{ display: "block" }}>
        <rect x={padL} y={padT} width={chartW} height={chartH} fill="none" stroke="#303030" />
        {profitArea && profitArea.h > 0 && (
          <rect x={padL} y={profitArea.y} width={chartW} height={profitArea.h} fill="#52c41a" fillOpacity={0.1} />
        )}
        {[max, (max + min) / 2, min].map((p, i) => (
          <g key={`grid-${i}`}>
            <line x1={padL} y1={y(p)} x2={padL + chartW} y2={y(p)} stroke="#1f1f1f" />
            <text x={padL - 6} y={y(p) + 3} textAnchor="end" fontSize="10" fill="#8c8c8c">
              {p.toFixed(1)}
            </text>
          </g>
        ))}
        {data.map((k, i) => {
          const cx = padL + (i + 0.5) * slot;
          const up = k.close >= k.open;
          const color = up ? "#3f8600" : "#cf1322";
          const top = Math.min(y(k.open), y(k.close));
          const hgt = Math.max(1, Math.abs(y(k.close) - y(k.open)));
          return (
            <g key={`c-${i}`}>
              <line x1={cx} y1={y(k.high)} x2={cx} y2={y(k.low)} stroke={color} strokeWidth={1} />
              <rect x={cx - bodyW / 2} y={top} width={bodyW} height={hgt} fill={color} />
              {/* 透明命中区：覆盖整列，鼠标好对准(细蜡烛不易悬停) */}
              <rect
                x={padL + i * slot}
                y={padT}
                width={slot}
                height={chartH}
                fill="transparent"
                onMouseEnter={() => setHover(i)}
                onMouseLeave={() => setHover((h) => (h === i ? null : h))}
              />
            </g>
          );
        })}
        {overlayLabels.map((o, i) => (
          <line
            key={`oline-${i}`}
            x1={padL}
            y1={o.lineY}
            x2={padL + chartW}
            y2={o.lineY}
            stroke={o.color}
            strokeWidth={1}
            strokeDasharray={o.dash || undefined}
          />
        ))}
        {profitPx > 0 && (
          <g>
            <line x1={padL} y1={yProfit} x2={padL + chartW} y2={yProfit} stroke="#52c41a" strokeWidth={1.5} />
            <text x={padL + 4} y={yProfit - 3} textAnchor="start" fontSize="10" fill="#52c41a">
              利润线 {profitPx.toFixed(1)}
            </text>
          </g>
        )}
        {predX >= 0 && (
          <g>
            <line
              x1={predX}
              y1={padT}
              x2={predX}
              y2={padT + chartH}
              stroke="#722ed1"
              strokeWidth={1}
              strokeDasharray="3 3"
            />
            <text x={predX} y={padT + 10} textAnchor="middle" fontSize="10" fill="#722ed1">
              预测目标 {data[predIdx].time.slice(5, 16)}
            </text>
          </g>
        )}
        {closeX >= 0 && (
          <g>
            <line
              x1={closeX}
              y1={padT}
              x2={closeX}
              y2={padT + chartH}
              stroke="#cf1322"
              strokeWidth={1}
              strokeDasharray="3 3"
            />
            <text x={closeX} y={padT - 4} textAnchor="middle" fontSize="10" fill="#cf1322">
              平仓 {data[closeIdx].time.slice(5, 16)}
            </text>
          </g>
        )}
        {overlayLabels.map((o, i) => {
          const text = `${o.label} ${o.v.toFixed(1)}`;
          // 估算文字宽度：CJK 约 10px、其余约 6px，用于给标签垫底色。
          let w = 8;
          for (const ch of text) w += ch.charCodeAt(0) > 255 ? 10 : 6;
          const moved = Math.abs(o.labelY - o.lineY) > 1;
          return (
            <g key={`olabel-${i}`}>
              {moved && (
                <polyline
                  points={`${padL + chartW},${o.lineY} ${labelX + 2},${o.lineY} ${labelX + 2},${o.labelY}`}
                  fill="none"
                  stroke={o.color}
                  strokeWidth={0.6}
                  strokeOpacity={0.6}
                />
              )}
              <rect
                x={labelX - w}
                y={o.labelY - 9}
                width={w}
                height={12}
                rx={2}
                fill="#000000"
                fillOpacity={0.55}
              />
              <text x={labelX} y={o.labelY} textAnchor="end" fontSize="10" fill={o.color}>
                {text}
              </text>
            </g>
          );
        })}
        <text x={padL} y={H - 8} fontSize="10" fill="#8c8c8c">
          {data[0].time.slice(5, 16)}
        </text>
        <text x={padL + chartW} y={H - 8} textAnchor="end" fontSize="10" fill="#8c8c8c">
          {data[data.length - 1].time.slice(5, 16)}
        </text>
        {hover != null && data[hover] && (() => {
          const k = data[hover];
          const cx = padL + (hover + 0.5) * slot;
          const boxW = 118;
          const boxH = 90;
          const boxX = cx < padL + chartW / 2 ? cx + 10 : cx - 10 - boxW; // 靠右半区则把框放左侧，避免出界
          const boxY = padT + 6;
          const up = k.close >= k.open;
          const rows: { label: string; value: string; color: string }[] = [
            { label: "时间", value: k.time.slice(5, 16), color: "#bfbfbf" },
            { label: "开", value: num(k.open, 2), color: "#bfbfbf" },
            { label: "高", value: num(k.high, 2), color: "#3f8600" },
            { label: "低", value: num(k.low, 2), color: "#cf1322" },
            { label: "收", value: num(k.close, 2), color: up ? "#3f8600" : "#cf1322" },
          ];
          return (
            <g pointerEvents="none">
              {/* 悬停列高亮 */}
              <rect x={padL + hover * slot} y={padT} width={slot} height={chartH} fill="#ffffff" fillOpacity={0.06} />
              <line x1={cx} y1={padT} x2={cx} y2={padT + chartH} stroke="#595959" strokeWidth={0.6} />
              {/* 提示框 */}
              <rect x={boxX} y={boxY} width={boxW} height={boxH} rx={3} fill="#000000" fillOpacity={0.82} stroke="#434343" />
              {rows.map((r, ri) => (
                <g key={`tt-${ri}`}>
                  <text x={boxX + 8} y={boxY + 17 + ri * 16} fontSize="10" fill="#8c8c8c">
                    {r.label}
                  </text>
                  <text x={boxX + boxW - 8} y={boxY + 17 + ri * 16} textAnchor="end" fontSize="10" fill={r.color}>
                    {r.value}
                  </text>
                </g>
              ))}
            </g>
          );
        })()}
      </svg>
    </div>
  );
}
