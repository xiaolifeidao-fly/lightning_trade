"use client";

import { AreaChartOutlined, DeploymentUnitOutlined, HistoryOutlined, ThunderboltFilled } from "@ant-design/icons";
import { Alert, Empty, Skeleton, Tabs, Tag, Typography } from "antd";
import { useCallback, useState } from "react";
import {
  fetchEpisodeDetail,
  fetchSignalDetail,
  type Episode,
  type EpisodeDetail,
  type SignalDetail,
  type SignalEvent,
} from "./api/argus-signals.api";
import { EpisodeDrawer } from "./components/EpisodeDrawer";
import { EpisodeTable } from "./components/EpisodeTable";
import { ReviewStats } from "./components/ReviewStats";
import { SignalDetailDrawer } from "./components/SignalDetailDrawer";
import { SignalFilterBar } from "./components/SignalFilterBar";
import { SignalTable } from "./components/SignalTable";
import { SliceCompareTable } from "./components/SliceCompareTable";
import { fmtDuration } from "./constants";
import { useArgusSignals, type SignalView } from "./hooks/useArgusSignals";

const { Text, Title } = Typography;

/**
 * 信号复盘与 episode 详情页（r13）。
 *
 * 它要彻底替代的流程是：每周导出 Telegram 历史消息交给 AI 读。所以页面的三个
 * 视角分别对应那套流程里的三个动作——翻消息找那一次触发（信号视角）、顺着一笔
 * 持仓从建仓读到平仓（episode 视角）、把改参数前后的两段消息摆在一起比
 * （切片对比）。
 *
 * 全页只读：没有任何能影响实盘进程或改数据的入口。
 */
export default function ArgusSignalsPage() {
  const {
    options,
    exitKinds,
    accountOptions,
    versionOptions,
    filters,
    patchFilters,
    resetFilters,
    view,
    setView,
    crossInstance,
    signals,
    signalPage,
    setSignalPage,
    episodes,
    episodeRows,
    episodePage,
    setEpisodePage,
    gateStats,
    episodeStats,
    sliceCompare,
    rebuild,
    bootstrapping,
    bootError,
    listLoading,
    statsLoading,
    compareLoading,
    loadError,
    refresh,
  } = useArgusSignals();

  const [signalDetail, setSignalDetail] = useState<SignalDetail | null>(null);
  const [signalDetailOpen, setSignalDetailOpen] = useState(false);
  const [signalDetailLoading, setSignalDetailLoading] = useState(false);
  const [signalDetailError, setSignalDetailError] = useState("");

  const [episodeDetail, setEpisodeDetail] = useState<EpisodeDetail | null>(null);
  const [episodeDetailOpen, setEpisodeDetailOpen] = useState(false);
  const [episodeDetailLoading, setEpisodeDetailLoading] = useState(false);
  const [episodeDetailError, setEpisodeDetailError] = useState("");

  const openSignalDetail = useCallback((row: SignalEvent) => {
    setSignalDetailOpen(true);
    setSignalDetail(null);
    setSignalDetailError("");
    setSignalDetailLoading(true);
    void fetchSignalDetail(row.eventId)
      .then(setSignalDetail)
      .catch((error: unknown) => setSignalDetailError(error instanceof Error ? error.message : "读取失败"))
      .finally(() => setSignalDetailLoading(false));
  }, []);

  const openEpisodeDetail = useCallback((row: Episode) => {
    setEpisodeDetailOpen(true);
    setEpisodeDetail(null);
    setEpisodeDetailError("");
    setEpisodeDetailLoading(true);
    void fetchEpisodeDetail(row.episodeId)
      .then(setEpisodeDetail)
      .catch((error: unknown) => setEpisodeDetailError(error instanceof Error ? error.message : "读取失败"))
      .finally(() => setEpisodeDetailLoading(false));
  }, []);

  if (bootError && !options) {
    return (
      <div className="manager-page-stack manager-argus">
        <Alert
          className="manager-argus-alert"
          type="error"
          showIcon
          message="无法进入复盘"
          description={bootError}
        />
      </div>
    );
  }

  const dataRange = options?.dataRange;

  return (
    <div className="manager-page-stack manager-argus">
      <section className="manager-argus-hero">
        <div>
          <Text className="manager-section-label">ARGUS REVIEW</Text>
          <Title level={2} className="manager-argus-hero__title">
            信号复盘与持仓生命周期
          </Title>
          <Text className="manager-argus-hero__desc">
            每一次盘口信号在每个账户上的判定、每一笔持仓从建仓到出场的全过程，全部来自
            strategy_event 与 episode 派生表。取代「导出 Telegram 历史消息交给 AI 读」的复盘流程。
          </Text>
        </div>
        <div className="manager-argus-hero__aside">
          <span className="manager-argus-beacon">
            <ThunderboltFilled style={{ color: "var(--manager-primary)" }} />
            数据区间 {dataRange?.start || "—"} ~ {dataRange?.end || "—"}
          </span>
        </div>
      </section>

      {crossInstance ? (
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          message="当前是全部实例视图：账户名会跨实例重号"
          description={
            <span>
              三个实例的账户名不是全局唯一的——
              <b>实例1 的 </b>
              <span className="manager-argus-mono">account1</span>
              <b> 与实例3 的 </b>
              <span className="manager-argus-mono">account1</span>
              <b> 是两个不同账户</b>。所以每一行都标了实例归属，账户唯一性按{" "}
              <span className="manager-argus-mono">(instance_key, account_label)</span> 归集。
              三实例的阈值（5/3 bp）、仓位上限（15 / 26+8 / 246）与下单张数（1/1/10）全不同，
              <b>张数、胜率与盈亏不要跨实例相加</b>。要看单个实例的口径，用上方选择器锁定实例。
            </span>
          }
        />
      ) : null}

      {rebuild?.stale ? (
        <Alert
          className="manager-argus-alert"
          type="warning"
          showIcon
          message="episode 派生表滞后"
          description={
            <span>
              {rebuild.notice || "派生结果没有覆盖到最新事件。"} 派生到{" "}
              <span className="manager-argus-mono">{rebuild.derivedThroughTs || "—"}</span>，事件表最末{" "}
              <span className="manager-argus-mono">{rebuild.latestEventTs || "—"}</span>
              {rebuild.lagSeconds === null ? "" : `（滞后 ${fmtDuration(rebuild.lagSeconds)}）`}。 episode
              目前只能靠 <span className="manager-argus-mono">argus-episode-rebuild</span> 手工整实例重建，
              这段滞后不要读成「最近没有持仓」。
            </span>
          }
        />
      ) : null}

      {gateStats?.truncated ? (
        <Alert
          className="manager-argus-alert"
          type="error"
          showIcon
          message="统计命中单次扫描行数上限"
          description="上面的分布只覆盖窗口内的一部分事件。请缩小时间范围或锁定单个实例后重看，不要按当前数字下结论。"
        />
      ) : null}

      {loadError ? <Alert className="manager-argus-alert" type="error" showIcon message="读取失败" description={loadError} /> : null}

      {bootstrapping ? (
        <div className="manager-argus-panel">
          <Skeleton active paragraph={{ rows: 6 }} />
        </div>
      ) : (
        <>
          <ReviewStats gateStats={gateStats} episodeStats={episodeStats} loading={statsLoading} />

          <div className="manager-argus-panel">
            <SignalFilterBar
              view={view}
              options={options}
              accountOptions={accountOptions}
              versionOptions={versionOptions}
              exitKinds={exitKinds}
              filters={filters}
              loading={listLoading || statsLoading || compareLoading}
              onChange={patchFilters}
              onReset={resetFilters}
              onRefresh={refresh}
            />

            <Tabs
              className="manager-argus-maintabs"
              activeKey={view}
              onChange={(key) => setView(key as SignalView)}
              items={[
                {
                  key: "signal",
                  label: (
                    <span className="manager-argus-maintab">
                      <ThunderboltFilled />
                      信号视角
                    </span>
                  ),
                  children:
                    signals.data.length === 0 && !listLoading ? (
                      <Empty
                        image={Empty.PRESENTED_IMAGE_SIMPLE}
                        description="当前筛选组合下没有判定记录，清空筛选后再看一次"
                      />
                    ) : (
                      <SignalTable
                        rows={signals.data}
                        total={signals.total}
                        page={signalPage}
                        loading={listLoading}
                        crossInstance={crossInstance}
                        onPageChange={setSignalPage}
                        onOpenDetail={openSignalDetail}
                      />
                    ),
                },
                {
                  key: "episode",
                  label: (
                    <span className="manager-argus-maintab">
                      <HistoryOutlined />
                      持仓 episode 视角
                    </span>
                  ),
                  children: (
                    <>
                      {episodes?.timeFieldNotice ? (
                        <div className="manager-argus-hint">
                          <span>{episodes.timeFieldNotice}</span>
                        </div>
                      ) : null}
                      {episodeRows.length === 0 && !listLoading ? (
                        <Empty
                          image={Empty.PRESENTED_IMAGE_SIMPLE}
                          description={
                            rebuild && rebuild.episodeCount === 0
                              ? "派生表里还没有任何 episode：请先跑 argus-episode-rebuild"
                              : "当前筛选组合下没有持仓"
                          }
                        />
                      ) : (
                        <EpisodeTable
                          rows={episodeRows}
                          total={episodes?.total ?? 0}
                          page={episodePage}
                          loading={listLoading}
                          crossInstance={crossInstance}
                          onPageChange={setEpisodePage}
                          onOpenDetail={openEpisodeDetail}
                        />
                      )}
                    </>
                  ),
                },
                {
                  key: "compare",
                  label: (
                    <span className="manager-argus-maintab">
                      <DeploymentUnitOutlined />
                      切片对比
                    </span>
                  ),
                  children: <SliceCompareTable data={sliceCompare} loading={compareLoading} />,
                },
              ]}
            />
          </div>

          <div className="manager-argus-hint">
            <span>
              <AreaChartOutlined style={{ marginInlineEnd: 6 }} />
              本页全部接口只读，页面上不存在任何能停掉实盘进程或改动数据的入口。要在行情图上定位某次触发，
              到「历史行情与触发点」页；要改参数，到「参数与运行控制」页。
            </span>
          </div>
        </>
      )}

      <SignalDetailDrawer
        open={signalDetailOpen}
        loading={signalDetailLoading}
        detail={signalDetail}
        error={signalDetailError}
        onClose={() => setSignalDetailOpen(false)}
      />
      <EpisodeDrawer
        open={episodeDetailOpen}
        loading={episodeDetailLoading}
        detail={episodeDetail}
        error={episodeDetailError}
        onClose={() => setEpisodeDetailOpen(false)}
      />
    </div>
  );
}
