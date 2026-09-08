"use client";

import { LineChartOutlined } from "@ant-design/icons";
import { Alert, Button, Empty, Skeleton, Typography, message } from "antd";
import { useCallback, useState } from "react";
import { chartPalette } from "@/components/charts/chartTheme";
import { MarketChart } from "./components/MarketChart";
import { MarketToolbar } from "./components/MarketToolbar";
import { SliceDrawer } from "./components/SliceDrawer";
import { StorageNotes } from "./components/StorageNotes";
import { TriggerTable } from "./components/TriggerTable";
import { PLATFORM_LABELS, TRIGGER_KINDS } from "./constants";
import { useArgusMarket } from "./hooks/useArgusMarket";

const { Text, Title } = Typography;

/**
 * 历史行情与触发点主视图（r12）。
 *
 * 一屏回答三个问题：什么时候触发、当时行情什么样、为什么触发或没触发。
 * 页面只读，唯一的写动作是「回填缺口」——它写 trade_kline，不碰实盘链路。
 */
export default function ArgusMarketPage() {
  const {
    instances,
    instanceKey,
    setInstanceKey,
    instruments,
    signalThresholdBp,
    positionCap,
    filters,
    updateFilters,
    toggleKind,
    timeline,
    triggers,
    visibleTriggers,
    triggerTotal,
    triggersTruncated,
    selectedEventId,
    setSelectedEventId,
    selectedTrigger,
    sliceOpen,
    slice,
    sliceLoading,
    sliceError,
    openSlice,
    closeSlice,
    bootstrapping,
    loading,
    backfilling,
    loadError,
    bootError,
    refresh,
    backfill,
  } = useArgusMarket();

  // 同一根 K 线上超出叠放上限被折叠掉的条数。静默丢弃会让人以为那根只触发了 5 次。
  const [markerOverflow, setMarkerOverflow] = useState(0);

  const onBackfill = useCallback(() => {
    void backfill()
      .then((result) => {
        const upserted = result.items.reduce((sum, item) => sum + (item.upserted || 0), 0);
        const capped = result.items.filter((item) => item.capped);
        message.success(`回填完成：${result.succeeded}/${result.total} 组，写入 ${upserted} 根`);
        if (capped.length > 0) {
          // 交易所的「最近 N 根」接口够不到太老的窗口，这时补不动不是失败，要说清楚。
          message.warning(`${capped.map((item) => `${item.platformCode} ${item.interval}`).join("、")} 窗口过老，单次拉取够不到左界`);
        }
        if (result.failed > 0) {
          message.error(`${result.failed} 组回填失败，详见服务端日志`);
        }
      })
      .catch((error: unknown) => message.error(error instanceof Error ? error.message : "回填失败"));
  }, [backfill]);

  const onRefresh = useCallback(() => {
    void refresh().catch(() => undefined);
  }, [refresh]);

  if (bootError) {
    return (
      <div className="manager-page-stack manager-argus manager-market">
        <Alert
          type="error"
          showIcon
          className="manager-argus-alert"
          message="没有可用的实例"
          description={bootError}
        />
      </div>
    );
  }

  const klines = timeline?.klines ?? [];
  const coverage = timeline?.coverage;
  const compareCoverage = timeline?.compareCoverage;
  const hasGap = (coverage?.missing ?? 0) > 0 || (compareCoverage?.missing ?? 0) > 0;

  return (
    <div className="manager-page-stack manager-argus manager-market">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS MARKET</Text>
          <Title level={2} className="manager-argus-hero__title">
            历史行情与触发点
          </Title>
          <Text className="manager-argus-hero__desc">
            K 线主图叠四类触发点，配 last-vs-mark 偏离副图与净持仓阶梯；点任意一个触发点下钻到它前后
            ±60 秒的秒级切片。阈值线与上限线跟随所选实例的已发布参数，不跨实例混排。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon">
            <LineChartOutlined style={{ color: "var(--manager-primary)" }} />
            窗口内触发 {triggerTotal} 次
          </span>
          <div className="manager-market-legend">
            {TRIGGER_KINDS.map((kind) => (
              <span key={kind.key}>
                <i style={{ background: kind.color }} />
                {kind.label}
              </span>
            ))}
            <span>
              <i style={{ background: chartPalette.accent }} />
              对比源收盘
            </span>
          </div>
        </div>
      </section>

      <MarketToolbar
        instances={instances}
        instanceKey={instanceKey}
        onInstanceChange={setInstanceKey}
        instruments={instruments}
        filters={filters}
        onFiltersChange={updateFilters}
        onToggleKind={toggleKind}
        triggers={triggers}
        thresholdBp={signalThresholdBp}
        positionCap={positionCap}
        loading={loading}
        backfilling={backfilling}
        onRefresh={onRefresh}
        onBackfill={onBackfill}
      />

      {loadError ? (
        <Alert
          type="error"
          showIcon
          className="manager-argus-alert"
          message="加载行情与触发点失败"
          description={loadError}
          action={
            <Button size="small" loading={loading} onClick={onRefresh}>
              重试
            </Button>
          }
        />
      ) : null}

      {triggersTruncated ? (
        <Alert
          type="warning"
          showIcon
          className="manager-argus-alert"
          message={`窗口内共 ${triggerTotal} 个触发点，当前只取回前 ${triggers.length} 个`}
          description="图上标记与下方表格都不是全量。缩小时间范围可以看全——这里不静默截断，是为了避免把「少了一半触发」当成「行情本来就没触发」。"
        />
      ) : null}

      {markerOverflow > 0 ? (
        <Alert
          type="info"
          showIcon
          className="manager-argus-alert"
          message={`有 ${markerOverflow} 个触发点因同一根 K 线叠放已满未画在图上`}
          description="实测同一分钟最多 5 次触发，图上每根按这个上限叠放；被折叠的仍然在下方表格里，可以点开秒级切片。换到更细的周期能把它们分开。"
        />
      ) : null}

      <div className="manager-argus-panel manager-market-chart">
        <div className="manager-argus-panel__head">
          <span className="manager-argus-panel__title">
            {timeline?.symbol || filters.instrument} · {filters.interval} ·{" "}
            {PLATFORM_LABELS[timeline?.platformCode ?? ""] ?? timeline?.platformCode ?? "—"}
            {timeline?.comparePlatformCode
              ? ` vs ${PLATFORM_LABELS[timeline.comparePlatformCode] ?? timeline.comparePlatformCode}`
              : ""}
          </span>
          <Text type="secondary">
            {klines.length} 根 · 触发点 {visibleTriggers.length} / {triggers.length}
          </Text>
        </div>

        {bootstrapping || (loading && !timeline) ? (
          <Skeleton active paragraph={{ rows: 10 }} />
        ) : klines.length === 0 ? (
          <div className="manager-market-empty">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                <>
                  <b>该数据源在此周期暂无落库数据。</b>
                  <br />
                  行情缺口没补齐时触发点无法对齐到时间轴上，所以这里不画半张图。
                  历史 K 线两家都能批量回填。
                </>
              }
            >
              <Button type="primary" loading={backfilling} onClick={onBackfill}>
                回填当前窗口的 K 线
              </Button>
            </Empty>
          </div>
        ) : (
          <>
            {hasGap ? (
              <Alert
                type="warning"
                showIcon
                className="manager-argus-alert manager-market-gap"
                message={`当前窗口存在行情缺口：${PLATFORM_LABELS[timeline?.platformCode ?? ""] ?? timeline?.platformCode} 缺 ${
                  coverage?.missing ?? 0
                } 根${timeline?.comparePlatformCode ? `，${PLATFORM_LABELS[timeline.comparePlatformCode] ?? timeline.comparePlatformCode} 缺 ${compareCoverage?.missing ?? 0} 根` : ""}`}
                description="缺口段的触发点在图上会挂不到 K 线，先回填再看。"
                action={
                  <Button size="small" type="primary" loading={backfilling} onClick={onBackfill}>
                    一键回填
                  </Button>
                }
              />
            ) : null}
            <MarketChart
              interval={filters.interval}
              platformCode={timeline?.platformCode ?? ""}
              klines={klines}
              comparePlatformCode={timeline?.comparePlatformCode ?? ""}
              compareKlines={timeline?.compareKlines ?? []}
              buckets={timeline?.buckets ?? []}
              triggers={triggers}
              visibleTriggers={visibleTriggers}
              thresholdBp={signalThresholdBp}
              positionCap={positionCap}
              selectedEventId={selectedEventId}
              onSelectTrigger={(eventId) => {
                const target = triggers.find((item) => item.eventId === eventId);
                if (target) openSlice(target);
              }}
              onMarkerOverflow={setMarkerOverflow}
            />
            <div className="manager-market-panes">
              <span>上：K 线主图与触发点</span>
              <span>中：last-vs-mark 偏离（bp，带符号，&gt;0 = UP），每根取绝对值最大的一次触发</span>
              <span>下：净持仓阶梯（该实例全部账户之和，桶末口径）</span>
            </div>
          </>
        )}
      </div>

      <div className="manager-market-bottom">
        <div className="manager-argus-panel">
          <div className="manager-argus-panel__head">
            <span className="manager-argus-panel__title">当前视图内的触发点</span>
            <Text type="secondary">点任意一行在图上定位，并下钻到触发瞬间的秒级切片</Text>
          </div>
          <TriggerTable
            triggers={visibleTriggers}
            selectedEventId={selectedEventId}
            onSelect={setSelectedEventId}
            onOpenSlice={openSlice}
            loading={loading}
          />
        </div>
        <StorageNotes timeline={timeline} />
      </div>

      <SliceDrawer
        open={sliceOpen}
        trigger={selectedTrigger}
        slice={slice}
        loading={sliceLoading}
        error={sliceError}
        thresholdBp={signalThresholdBp}
        onClose={closeSlice}
      />
    </div>
  );
}
