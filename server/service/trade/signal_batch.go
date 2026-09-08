package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"

	"github.com/sirupsen/logrus"
)

// 本文件是参数组批量扫描（r11）的编排层：一次提交 N 组参数 → 建 N 条 run →
// 共享一份数据并发回放 → 汇总成按 fidelity 分组的对比矩阵。
//
// 三个刻意的设计选择：
//
//  1. **一组参数就是一条完整的 run**，不另建"批量逐组"表。组的参数快照、精度
//     等级、汇总指标、逐笔明细全部落在既有三张表上，因此批量里的任一组都能在
//     既有 run 详情页直接打开、单独重跑，也不需要给 r14 的页面再造一套读接口。
//
//  2. **数据只取一次，全组共享**。事件双写是持续在写的：每组各取一次会让先跑的
//     组和后跑的组吃到不同的触发集，组间差异里就混进了数据差异。共享还顺带省掉
//     N−1 次全表扫（4 天窗口 ≈ 1.6 万事件 + 5760 根 1m）。
//
//  3. **组参数是相对基线的增量**，基线默认取所选实例当前生产参数。用代码缺省当
//     基线会让每组同时偏离生产十几个字段，diff 就没有意义了（见 signal_baseline.go）。
//
// 不做（属 r16 自动寻优）：不自动生成搜索空间、不做统计判定、不输出"最优参数"。

const (
	// DefaultSignalBatchConcurrency 缺省并发组数。回放是纯 CPU 的，放太开会把
	// 管理端进程的 CPU 吃满、拖慢在线接口，3 是"能明显加速又不至于抢死"的折中。
	DefaultSignalBatchConcurrency = 3
	// MaxSignalBatchConcurrency 并发上限。
	MaxSignalBatchConcurrency = 8
	// MaxSignalBatchGroups 单批组数上限。手工对比十几组是常态，上百组就是在做
	// 网格搜索了——那属于 r16 的自动寻优（它有降噪协议与三关判定），不该从这里
	// 绕过去。
	MaxSignalBatchGroups = 64

	// 批次状态。partial = 有组失败也有组成功，必须区别于 done，
	// 否则页面会把"缺了几组的榜"显示成完整结果。
	BatchStatusPending = "pending"
	BatchStatusRunning = "running"
	BatchStatusDone    = "done"
	BatchStatusPartial = "partial"
	BatchStatusFailed  = "failed"
)

// CreateSignalBacktestBatch 校验并创建一次参数组批量扫描，落库后异步并发回放，
// 立即返回 batchId。
func (s *TradeService) CreateSignalBacktestBatch(ctx context.Context, dto tradeDTO.CreateSignalBacktestBatchDTO) (int64, error) {
	start, err := parseSignalWindowTime(dto.StartTime)
	if err != nil {
		return 0, fmt.Errorf("startTime: %w", err)
	}
	end, err := parseSignalWindowTime(dto.EndTime)
	if err != nil {
		return 0, fmt.Errorf("endTime: %w", err)
	}
	if !end.After(start) {
		return 0, fmt.Errorf("endTime 必须晚于 startTime")
	}
	instanceKey := strings.TrimSpace(dto.InstanceKey)
	accountLabel := strings.TrimSpace(dto.AccountLabel)
	if instanceKey == "" {
		return 0, fmt.Errorf("instanceKey 必填：三个实例写同一张事件表，不带实例维度会把不同实验体的触发流混成一条")
	}
	if accountLabel == "" {
		return 0, fmt.Errorf("accountLabel 必填：同一实例可能有多个账户（champion/challenger），少这一维会把两本仓算成一本")
	}
	if len(dto.Groups) == 0 {
		return 0, fmt.Errorf("groups 不能为空：至少提交一组要验的参数")
	}
	if len(dto.Groups) > MaxSignalBatchGroups {
		return 0, fmt.Errorf("groups 最多 %d 组，收到 %d 组：更大的搜索空间请走自动寻优（r16，带降噪协议与三关判定），不要用批量扫描绕过去",
			MaxSignalBatchGroups, len(dto.Groups))
	}

	platform := strings.TrimSpace(dto.PlatformCode)
	if platform == "" {
		platform = DefaultSignalPlatform
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	concurrency := normalizeBatchConcurrency(dto.Concurrency)

	// ① 基线：默认取所选实例当前生产参数；请求显式给了就用请求的。
	baseline, err := s.resolveBatchBaseline(ctx, dto, instanceKey, accountLabel, symbol)
	if err != nil {
		return 0, err
	}
	if err := baseline.Params.Validate(); err != nil {
		return 0, fmt.Errorf("基线参数非法（口径同实盘启动校验），整批扫描无参照物: %w", err)
	}
	baselineSnapshot, err := json.Marshal(baseline.Params)
	if err != nil {
		return 0, err
	}

	// ② 先把**所有**组的参数算出来并逐组校验，全部通过了再落库。
	//    否则第 20 组参数写错会留下 19 条已建的 run 和一个半残批次。
	type plannedGroup struct {
		Label      string
		Params     signal.Params
		IsBaseline bool
	}
	planned := make([]plannedGroup, 0, len(dto.Groups)+1)
	if dto.IncludeBaselineRun == nil || *dto.IncludeBaselineRun {
		planned = append(planned, plannedGroup{Label: "baseline", Params: baseline.Params, IsBaseline: true})
	}
	for i, g := range dto.Groups {
		params := applySignalParamKnobs(baseline.Params, g.SignalBacktestParamsDTO)
		if err := params.Validate(); err != nil {
			return 0, fmt.Errorf("第 %d 组（%s）参数非法（口径同实盘启动校验）: %w", i+1, groupHint(g, i), err)
		}
		label := strings.TrimSpace(g.Label)
		if label == "" {
			label = AutoGroupLabel(DiffSignalParams(baseline.Params, params))
		}
		planned = append(planned, plannedGroup{Label: label, Params: params})
	}

	batch := &tradeRepository.TradeBacktestBatch{
		Name:             strings.TrimSpace(dto.Name),
		InstanceKey:      instanceKey,
		AccountLabel:     accountLabel,
		PlatformCode:     platform,
		CoinCode:         strings.ToUpper(strings.TrimSpace(dto.CoinCode)),
		Symbol:           symbol,
		StartTime:        start,
		EndTime:          end,
		BaselineSnapshot: string(baselineSnapshot),
		BaselineSource:   baseline.Source,
		BaselineNote:     truncate(baseline.Note(), 1024),
		Concurrency:      concurrency,
		GroupCount:       len(planned),
		Status:           BatchStatusPending,
	}
	if err := s.tradeBacktestBatchRepository.CreateBatch(batch); err != nil {
		return 0, err
	}
	batchID := int64(batch.Id)

	var baselineRunID int64
	for _, g := range planned {
		run, err := s.createBatchRun(batch, g.Label, g.Params, g.IsBaseline)
		if err != nil {
			_ = s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, BatchStatusFailed,
				truncate(fmt.Sprintf("建组 %s 失败: %v", g.Label, err), 500))
			return 0, err
		}
		if g.IsBaseline {
			baselineRunID = int64(run.Id)
			if err := s.tradeBacktestBatchRepository.UpdateBatchBaselineRun(batchID, baselineRunID); err != nil {
				logrus.Warnf("[signal-batch] batch=%d 回填 baseline_run_id 失败: %v", batchID, err)
			}
		}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[signal-batch] batch=%d panic: %v", batchID, r)
				_ = s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, BatchStatusFailed,
					truncate(fmt.Sprintf("panic: %v", r), 500))
			}
		}()
		if err := s.RunSignalBacktestBatch(batchID); err != nil {
			logrus.Warnf("[signal-batch] batch=%d 执行失败: %v", batchID, err)
		}
	}()
	return batchID, nil
}

// resolveBatchBaseline 定基线：请求显式给了用请求的，否则读所选实例当前生产参数。
func (s *TradeService) resolveBatchBaseline(ctx context.Context, dto tradeDTO.CreateSignalBacktestBatchDTO,
	instanceKey, accountLabel, symbol string) (*SignalBaseline, error) {

	if dto.BaselineParams != nil {
		return &SignalBaseline{
			Params: applySignalParamKnobs(signal.DefaultParams(), *dto.BaselineParams),
			Source: BaselineSourceRequest,
			Notes: []string{
				"基线由请求体显式给出（baselineParams），不是所选实例的当前生产参数：与生产的偏差需自行确认",
			},
		}, nil
	}
	return s.ResolveInstanceBaseline(ctx, instanceKey, accountLabel, symbol)
}

// createBatchRun 为一组参数建一条 run。字段口径与单次回测
// （CreateSignalBacktestRun）完全一致，只多了批次归属三列。
func (s *TradeService) createBatchRun(batch *tradeRepository.TradeBacktestBatch,
	label string, params signal.Params, isBaseline bool) (*tradeRepository.TradeBacktestRun, error) {

	snapshot, err := json.Marshal(params.Normalize())
	if err != nil {
		return nil, err
	}
	fid := signal.Classify(params)
	name := label
	if batch.Name != "" {
		name = batch.Name + " / " + label
	}
	run := &tradeRepository.TradeBacktestRun{
		Name:           truncate(name, 128),
		PlatformCode:   batch.PlatformCode,
		CoinCode:       batch.CoinCode,
		Symbol:         batch.Symbol,
		PriceInterval:  "1m",
		PriceSource:    batch.PlatformCode,
		StartTime:      batch.StartTime,
		EndTime:        batch.EndTime,
		ParamsSnapshot: string(snapshot),
		Status:         "pending",
		EngineKind:     EngineKindSignal,
		InstanceKey:    batch.InstanceKey,
		AccountLabel:   batch.AccountLabel,
		SignalSource:   SignalSourceStrategyEvent,
		Fidelity:       fid.Level,
		FidelityNote:   truncate(fid.Note(), 1024),
		BatchID:        int64(batch.Id),
		GroupLabel:     truncate(label, 128),
	}
	if isBaseline {
		run.IsBaseline = 1
	}
	if err := s.tradeBacktestRunRepository.CreateRun(run); err != nil {
		return nil, err
	}
	return run, nil
}

// RunSignalBacktestBatch 执行一次批量扫描：取一份数据 → 并发回放全部组 → 汇总状态。
func (s *TradeService) RunSignalBacktestBatch(batchID int64) error {
	batch, err := s.tradeBacktestBatchRepository.FindBatchByID(batchID)
	if err != nil {
		return err
	}
	runs, err := s.tradeBacktestRunRepository.FindRunsByBatch(batchID)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, BatchStatusFailed, "批次下没有任何参数组")
	}
	_ = s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, BatchStatusRunning, "")

	// 只要有一组是频率级就得取 dev_sample（推 λ(θ)）；全是事件级则跳过，
	// 省一次全表扫。fidelity 在建组时已按参数算过并落库，这里直接读。
	needDev := false
	for _, run := range runs {
		if run.Fidelity == signal.FidelityFrequency {
			needDev = true
			break
		}
	}
	ds, err := s.loadSignalDataset(signalWindow{
		InstanceKey:  batch.InstanceKey,
		AccountLabel: batch.AccountLabel,
		Symbol:       batch.Symbol,
		PlatformCode: batch.PlatformCode,
		Start:        batch.StartTime,
		End:          batch.EndTime,
	}, needDev)
	if err != nil {
		// 取数失败是批次级失败：每一组都跑不了，逐组也标失败，否则页面上会有
		// 一堆永远 pending 的组等不到任何解释。
		msg := truncate(err.Error(), 500)
		for _, run := range runs {
			_ = s.tradeBacktestRunRepository.UpdateRunStatus(int64(run.Id), "failed", msg)
		}
		_ = s.tradeBacktestBatchRepository.BumpBatchProgress(batchID, 0, len(runs))
		_ = s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, BatchStatusFailed, msg)
		return err
	}

	concurrency := normalizeBatchConcurrency(batch.Concurrency)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, failed := 0, 0

	for _, run := range runs {
		wg.Add(1)
		go func(run *tradeRepository.TradeBacktestRun) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				// 单组 panic 不能带走整批：其余组的结果仍然有效。
				if r := recover(); r != nil {
					logrus.Errorf("[signal-batch] batch=%d run=%d panic: %v", batchID, run.Id, r)
					_ = s.tradeBacktestRunRepository.UpdateRunStatus(int64(run.Id), "failed", fmt.Sprintf("panic: %v", r))
					mu.Lock()
					failed++
					mu.Unlock()
					_ = s.tradeBacktestBatchRepository.BumpBatchProgress(batchID, 0, 1)
				}
			}()
			if err := s.runBatchGroup(run, ds); err != nil {
				logrus.Warnf("[signal-batch] batch=%d run=%d 组执行失败: %v", batchID, run.Id, err)
				mu.Lock()
				failed++
				mu.Unlock()
				_ = s.tradeBacktestBatchRepository.BumpBatchProgress(batchID, 0, 1)
				return
			}
			mu.Lock()
			done++
			mu.Unlock()
			_ = s.tradeBacktestBatchRepository.BumpBatchProgress(batchID, 1, 0)
		}(run)
	}
	wg.Wait()

	status := BatchStatusDone
	errMsg := ""
	switch {
	case failed == len(runs):
		status = BatchStatusFailed
		errMsg = fmt.Sprintf("全部 %d 组执行失败，详见各组的 errorMsg", failed)
	case failed > 0:
		status = BatchStatusPartial
		errMsg = fmt.Sprintf("%d/%d 组执行失败：对比矩阵不完整，缺失的组不代表结果差", failed, len(runs))
	}
	return s.tradeBacktestBatchRepository.UpdateBatchStatus(batchID, status, errMsg)
}

// runBatchGroup 用共享数据跑一组并落库。落库步骤与 RunSignalBacktest 一致，
// 唯一区别是数据来自批次共享的 dataset 而不是自己再查一次。
//
// 已经 done 的组直接跳过：逐笔是 BatchCreate（不是 upsert），重跑一遍会把同一组
// 的明细写两份，汇总与明细就此对不上。批次执行器目前只被创建流程调用一次，
// 这道护栏是给后续复用（r16 的寻优会按批次跑）留的。
func (s *TradeService) runBatchGroup(run *tradeRepository.TradeBacktestRun, ds *signalDataset) error {
	runID := int64(run.Id)
	if run.Status == "done" {
		logrus.Infof("[signal-batch] run=%d 已完成，跳过重跑（逐笔非幂等）", runID)
		return nil
	}
	_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "running", "")

	params, err := signalParamsFromSnapshot(run.ParamsSnapshot)
	if err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
		return err
	}
	res, err := replaySignalDataset(ds, params)
	if err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
		return err
	}
	_ = s.tradeBacktestRunRepository.UpdateRunKlineInfo(runID, ds.KlineCount, ds.KlineStart, ds.KlineEnd)
	if err := s.tradeBacktestRunRepository.UpdateSignalRunResult(runID, res.Fidelity.Level,
		truncate(res.Fidelity.Note(), 1024), res.SignalReplayed); err != nil {
		logrus.Warnf("[signal-batch] run=%d 回填精度信息失败: %v", runID, err)
	}
	if trades := signalTradeEntities(runID, params, res); len(trades) > 0 {
		if err := s.tradeBacktestTradeRepository.BatchCreate(trades); err != nil {
			_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
			return err
		}
	}
	metric := signalMetricEntity(runID, signal.Aggregate(res))
	if err := s.tradeBacktestMetricRepository.UpsertMetrics([]*tradeRepository.TradeBacktestMetric{metric}); err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
		return err
	}
	return s.tradeBacktestRunRepository.UpdateRunStatus(runID, "done", "")
}

// ─── 读侧 ────────────────────────────────────────────────────────────────────

// ListSignalBacktestBatches 分页查询批次列表。
func (s *TradeService) ListSignalBacktestBatches(q tradeDTO.SignalBacktestBatchQueryDTO) (*tradeDTO.SignalBacktestBatchListDTO, error) {
	rows, total, err := s.tradeBacktestBatchRepository.FindBatches(
		strings.TrimSpace(q.InstanceKey), strings.TrimSpace(q.AccountLabel), q.Page, q.PageSize)
	if err != nil {
		return nil, err
	}
	list := make([]tradeDTO.SignalBacktestBatchDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, signalBatchToDTO(r))
	}
	return &tradeDTO.SignalBacktestBatchListDTO{Total: total, List: list}, nil
}

// GetSignalBacktestBatchDetail 批次详情：批次头 + 基线 + 按 fidelity 分组的对比矩阵。
//
// 与基线的差异一律按**批次冻结的基线快照**算，不重新读生产配置：生产参数随时会被
// 热更（r5/r7 的整条链路就是为热更做的），事后再读会让同一个批次今天和明天算出
// 不同的差异。
func (s *TradeService) GetSignalBacktestBatchDetail(batchID int64) (*tradeDTO.SignalBacktestBatchDetailDTO, error) {
	batch, err := s.tradeBacktestBatchRepository.FindBatchByID(batchID)
	if err != nil {
		return nil, err
	}
	runs, err := s.tradeBacktestRunRepository.FindRunsByBatch(batchID)
	if err != nil {
		return nil, err
	}
	runIDs := make([]int64, 0, len(runs))
	for _, r := range runs {
		runIDs = append(runIDs, int64(r.Id))
	}
	metricByRun := map[int64]*tradeRepository.TradeBacktestMetric{}
	if metrics, err := s.tradeBacktestMetricRepository.FindByRuns(runIDs); err == nil {
		for _, m := range metrics {
			if m.CalcMode == CalcModeSignal {
				metricByRun[m.RunID] = m
			}
		}
	} else {
		logrus.Warnf("[signal-batch] batch=%d 取汇总指标失败: %v", batchID, err)
	}

	baselineParams, err := signalParamsFromSnapshot(batch.BaselineSnapshot)
	if err != nil {
		return nil, fmt.Errorf("批次基线快照不可用，无法计算差异: %w", err)
	}
	groups, warnings := buildComparison(baselineParams, runs, metricByRun, batch.BaselineRunID)
	if batch.Status == BatchStatusRunning || batch.Status == BatchStatusPending {
		warnings = append(warnings, fmt.Sprintf("批次仍在执行（%d/%d 组已完成）：现在看到的排序还会变",
			batch.DoneCount+batch.FailedCount, batch.GroupCount))
	}

	return &tradeDTO.SignalBacktestBatchDetailDTO{
		Batch:    signalBatchToDTO(batch),
		Baseline: baselineDTOFromBatch(batch, baselineParams),
		Groups:   groups,
		Warnings: warnings,
	}, nil
}

// GetSignalBaselinePreview 表单在提交批次之前先看清基线是什么：哪些字段真的来自
// DB、哪些是兜底。让人在提交前就知道 diff 的参照物可不可信。
func (s *TradeService) GetSignalBaselinePreview(ctx context.Context, q tradeDTO.SignalBaselineQueryDTO) (*tradeDTO.SignalBaselineDTO, error) {
	symbol := strings.ToUpper(strings.TrimSpace(q.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	baseline, err := s.ResolveInstanceBaseline(ctx, q.InstanceKey, q.AccountLabel, symbol)
	if err != nil {
		return nil, err
	}
	dto := &tradeDTO.SignalBaselineDTO{
		Source: baseline.Source,
		Params: paramsToMap(baseline.Params),
		Notes:  baseline.Notes,
		FromDB: baseline.FromDB,
	}
	return dto, nil
}

// ─── 映射 ────────────────────────────────────────────────────────────────────

func signalBatchToDTO(b *tradeRepository.TradeBacktestBatch) tradeDTO.SignalBacktestBatchDTO {
	return tradeDTO.SignalBacktestBatchDTO{
		ID:             int64(b.Id),
		Name:           b.Name,
		InstanceKey:    b.InstanceKey,
		AccountLabel:   b.AccountLabel,
		PlatformCode:   b.PlatformCode,
		CoinCode:       b.CoinCode,
		Symbol:         b.Symbol,
		StartTime:      fmtTime(b.StartTime),
		EndTime:        fmtTime(b.EndTime),
		Status:         b.Status,
		ErrorMsg:       b.ErrorMsg,
		Concurrency:    b.Concurrency,
		GroupCount:     b.GroupCount,
		DoneCount:      b.DoneCount,
		FailedCount:    b.FailedCount,
		BaselineRunID:  b.BaselineRunID,
		BaselineSource: b.BaselineSource,
		CreatedTime:    fmtTime(b.CreatedTime),
	}
}

func baselineDTOFromBatch(b *tradeRepository.TradeBacktestBatch, params signal.Params) tradeDTO.SignalBaselineDTO {
	notes := make([]string, 0, 4)
	for _, n := range strings.Split(b.BaselineNote, " | ") {
		if n = strings.TrimSpace(n); n != "" {
			notes = append(notes, n)
		}
	}
	return tradeDTO.SignalBaselineDTO{
		Source: b.BaselineSource,
		Params: paramsToMap(params),
		Notes:  notes,
	}
}

// paramsToMap 把参数按 signal.Params 的 json tag 转成 map，直接回给前端。
// 不再手抄一份 DTO：参数字段会随寻优需求增删，抄一份必然漂移。
func paramsToMap(p signal.Params) map[string]interface{} {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func normalizeBatchConcurrency(v int) int {
	if v <= 0 {
		return DefaultSignalBatchConcurrency
	}
	if v > MaxSignalBatchConcurrency {
		return MaxSignalBatchConcurrency
	}
	return v
}

// groupHint 参数非法时给出的组定位信息：优先用组标签，没标签就报序号。
func groupHint(g tradeDTO.SignalBacktestGroupDTO, index int) string {
	if label := strings.TrimSpace(g.Label); label != "" {
		return label
	}
	return fmt.Sprintf("未命名组#%d", index+1)
}
