package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"

	"github.com/sirupsen/logrus"
)

// 本文件是后台自动参数寻优（r16）的编排层。方法学内核在
// strategy/signal/{study,regime,gates,conclusion}.go，那边是纯计算；这里只做
// 取数、按阶段调度、把冻结快照与结果落库。
//
// 与 r11 的批量扫描（signal_batch.go）的区别不是"格子多一点"：
//
//	r11  一组参数 = 一条 run = 一次回放 = 一个点估计，人自己看着比
//	r16  一格     = N 条抖动路径的分布 + 预注册三关判定 + 无解分支
//
// 所以 r16 不复用 trade_backtest_run（640 条 run 里没有一条是决策依据），
// 而是落 trade_optimize_study / trade_optimize_cell 两张表。
//
// 三条不可动摇的规则：
//
//  1. **阈值预注册**：三关阈值在创建时写入 gate_snapshot 并盖上 gate_locked_at，
//     执行期任何路径都不写这两列。要换阈值 → 建新任务。
//  2. **不输出最优参数**、不改任何线上参数。结论只有"候选 + 待 OOS 验证"或
//     "无解 + 权衡前沿"两种形态（见 signal.Conclude）。
//  3. **粗网格不做判定**：4 条路径的符号一致率只有 5 档，分辨不出 0.69 与 0.80，
//     而那正是决策线。粗网格只用于收敛，判定全在精算阶段。

const (
	// OptimizeEffectiveTakerFee 有效 taker 费率 0.012%/边（万 6 × 20%，80% 返佣）。
	// 寻优一律按它算：真实费率下门控/兜底的重校准结论与万 6 完全不同
	// （设计文档 §3.4 把它列为"对旧回测的一处修正"）。
	OptimizeEffectiveTakerFee = 0.00012
	// OptimizeOvershootRoiPts 兜底成交过冲惩罚 5 ROI 点（实测 3~10 点）。
	// 对高 λ 格子影响不可忽略，不加它会系统性高估紧兜底配置。
	OptimizeOvershootRoiPts = 5.0

	// DefaultOptimizeConcurrency / MaxOptimizeConcurrency 并发格数。
	// 单格内 16 条路径顺序跑，并发放在格级已足够吃满 CPU。
	DefaultOptimizeConcurrency = 3
	MaxOptimizeConcurrency     = 8

	// MaxOptimizeCoarseCells 粗网格格数上限。缺省空间是 55 格；留出余量给
	// 加轴，但不允许提交一个会跑几万次回放的空间。
	MaxOptimizeCoarseCells = 200

	// 阶段。
	OptimizeStageCoarse    = "coarse"
	OptimizeStageFine      = "fine"
	OptimizeStageConcluded = "concluded"

	// 任务状态。partial = 有格子失败也有格子成功；结论仍会出，但必须标明不完整。
	OptimizeStatusPending = "pending"
	OptimizeStatusRunning = "running"
	OptimizeStatusDone    = "done"
	OptimizeStatusPartial = "partial"
	OptimizeStatusFailed  = "failed"

	// 格子状态。skipped = 参数在实盘校验下非法（例如 order_size 超过本格上限），
	// 不是失败——它本来就装不进这个账户，混进 failed 会让"失败格数"失去意义。
	OptimizeCellPending = "pending"
	OptimizeCellRunning = "running"
	OptimizeCellDone    = "done"
	OptimizeCellFailed  = "failed"
	OptimizeCellSkipped = "skipped"

	// 样本口径。
	SampleKindInSample  = "in_sample"
	SampleKindOutSample = "out_of_sample"
)

// optimizeProtocolPair 任务冻结的两套降噪协议。粗/精分别落库而不是"精算协议
// 派生粗协议"：派生规则一改，历史任务的粗网格就无法复现。
type optimizeProtocolPair struct {
	Fine   signal.Protocol `json:"fine"`
	Coarse signal.Protocol `json:"coarse"`
}

// CreateSignalOptimizeStudy 校验并创建一次自动寻优任务，落库后异步执行，
// 立即返回 studyId。
func (s *TradeService) CreateSignalOptimizeStudy(ctx context.Context, dto tradeDTO.CreateSignalOptimizeStudyDTO) (int64, error) {
	instanceKey := strings.TrimSpace(dto.InstanceKey)
	accountLabel := strings.TrimSpace(dto.AccountLabel)
	if instanceKey == "" {
		return 0, fmt.Errorf("instanceKey 必填：三个实例写同一张事件表，不带实例维度会把不同实验体的触发流混成一条")
	}
	if accountLabel == "" {
		return 0, fmt.Errorf("accountLabel 必填：同一实例可能有多个账户（champion/challenger），少这一维会把两本仓算成一本")
	}
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

	platform := strings.TrimSpace(dto.PlatformCode)
	if platform == "" {
		platform = DefaultSignalPlatform
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}

	var oosBase *tradeRepository.TradeOptimizeStudy
	if dto.OosBaseStudyID > 0 {
		oosBase, err = s.loadOosBaseStudy(dto, start)
		if err != nil {
			return 0, err
		}
	}

	// ① 基线：不参与寻优的那些旋钮（trail 档位、面值、order_size、risk_equity）
	//    从这里来，四个搜索轴覆盖在它之上。
	baseline, err := s.resolveOptimizeBaseline(ctx, dto, oosBase, instanceKey, accountLabel, symbol)
	if err != nil {
		return 0, err
	}
	if err := baseline.Params.Validate(); err != nil {
		return 0, fmt.Errorf("基线参数非法（口径同实盘启动校验），寻优没有参照物: %w", err)
	}

	// ② 搜索空间 / 降噪协议 / 收敛规则 / 三关阈值：OOS 任务一律继承基准任务的冻结值。
	space, protocols, converge, gates, err := s.resolveOptimizeConfig(dto, oosBase, baseline.Params)
	if err != nil {
		return 0, err
	}

	incumbent := resolveIncumbentCell(dto, oosBase, baseline.Params)

	// ③ 决定第一阶段：显式给了精算格（或 OOS 继承了精算格）→ 跳过粗网格。
	stage := OptimizeStageCoarse
	cells := space.CoarseCells()
	explicitFine, err := s.resolveExplicitFineCells(dto, oosBase)
	if err != nil {
		return 0, err
	}
	if len(explicitFine) > 0 {
		stage = OptimizeStageFine
		cells = explicitFine
	}
	if len(cells) == 0 {
		return 0, fmt.Errorf("搜索空间展开为 0 格：检查 netCaps/stopPcts 是否都被过滤掉了")
	}
	if stage == OptimizeStageCoarse && len(cells) > MaxOptimizeCoarseCells {
		return 0, fmt.Errorf("粗网格展开出 %d 格，超过上限 %d：缩小搜索空间，或用 fineCells 显式指定要精算的格子",
			len(cells), MaxOptimizeCoarseCells)
	}

	gateLockedAt := time.Now()
	study := &tradeRepository.TradeOptimizeStudy{
		Name:         strings.TrimSpace(dto.Name),
		InstanceKey:  instanceKey,
		AccountLabel: accountLabel,
		PlatformCode: platform,
		CoinCode:     strings.ToUpper(strings.TrimSpace(dto.CoinCode)),
		Symbol:       symbol,
		StartTime:    start,
		EndTime:      end,
		SampleKind:   SampleKindInSample,
		SampleNote: "in_sample：参数是在这段数据上挑的，窗口内的收益必然高估。" +
			"锁定后请以本任务为基准建 out_of_sample 任务，用锁定之后新进的信号流复算，不要重新调参",
		GateLockedAt: gateLockedAt,
		GateNote:     truncate(gates.Describe(), 512),
		Stage:        stage,
		Concurrency:  normalizeOptimizeConcurrency(dto.Concurrency),
		Status:       OptimizeStatusPending,
	}
	if oosBase != nil {
		study.SampleKind = SampleKindOutSample
		study.OosBaseID = int64(oosBase.Id)
		study.SampleNote = fmt.Sprintf(
			"out_of_sample：继承任务 #%d 于 %s 锁定的三关阈值、降噪协议与精算格，只换数据窗口，不重新调参。"+
				"本窗口起点 %s 晚于该锁定时刻，因此是真正的样本外复算",
			oosBase.Id, fmtTime(oosBase.GateLockedAt), fmtTime(start))
		study.GateLockedAt = oosBase.GateLockedAt // 锁定时刻沿用基准任务：阈值不是这次定的
	}
	if stage == OptimizeStageCoarse {
		study.CoarseCellCount = len(cells)
	} else {
		study.FineCellCount = len(cells)
		if oosBase == nil {
			study.ConvergeNote = "精算格由请求显式给出（fineCells），跳过粗网格：用于复现指定的精算清单"
		} else {
			study.ConvergeNote = fmt.Sprintf("精算格继承基准任务 #%d，跳过粗网格：OOS 复算不允许重新选格", oosBase.Id)
		}
	}
	if study.SpaceSnapshot, err = marshalJSON(space); err != nil {
		return 0, err
	}
	if study.ProtocolSnapshot, err = marshalJSON(protocols); err != nil {
		return 0, err
	}
	if study.ConvergeSnapshot, err = marshalJSON(converge); err != nil {
		return 0, err
	}
	if study.GateSnapshot, err = marshalJSON(gates); err != nil {
		return 0, err
	}
	if study.BaselineSnapshot, err = marshalJSON(baseline.Params); err != nil {
		return 0, err
	}
	study.BaselineSource = baseline.Source
	study.BaselineNote = truncate(baseline.Note(), 1024)
	if incumbent != nil {
		if study.IncumbentSnapshot, err = marshalJSON(incumbent); err != nil {
			return 0, err
		}
	}
	if err := s.tradeOptimizeStudyRepository.CreateStudy(study); err != nil {
		return 0, err
	}
	studyID := int64(study.Id)

	rows, skipped := buildOptimizeCellRows(studyID, stage, cells, baseline.Params)
	if skipped == len(rows) {
		msg := "全部格子的参数在实盘校验下都非法：搜索空间与该账户不兼容（常见原因是 order_size 大于格子上限），任务无从执行"
		_ = s.tradeOptimizeCellRepository.BatchCreateCells(rows)
		_ = s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, OptimizeStatusFailed, msg)
		return studyID, fmt.Errorf("%s", msg)
	}
	if err := s.tradeOptimizeCellRepository.BatchCreateCells(rows); err != nil {
		_ = s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, OptimizeStatusFailed,
			truncate("建格失败: "+err.Error(), 500))
		return 0, err
	}
	if skipped > 0 {
		_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 0, 0, skipped, 0)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[signal-optimize] study=%d panic: %v", studyID, r)
				_ = s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, OptimizeStatusFailed,
					truncate(fmt.Sprintf("panic: %v", r), 500))
			}
		}()
		if err := s.RunSignalOptimizeStudy(studyID); err != nil {
			logrus.Warnf("[signal-optimize] study=%d 执行失败: %v", studyID, err)
		}
	}()
	return studyID, nil
}

// loadOosBaseStudy 取 OOS 基准任务并校验"样本外"这件事真的成立。
func (s *TradeService) loadOosBaseStudy(dto tradeDTO.CreateSignalOptimizeStudyDTO, start time.Time) (*tradeRepository.TradeOptimizeStudy, error) {
	base, err := s.tradeOptimizeStudyRepository.FindStudyByID(dto.OosBaseStudyID)
	if err != nil {
		return nil, fmt.Errorf("基准扫描 #%d 不存在: %w", dto.OosBaseStudyID, err)
	}
	if base.Stage != OptimizeStageConcluded {
		return nil, fmt.Errorf("基准扫描 #%d 还没出结论（stage=%s）：没有锁定的精算格可以继承", base.Id, base.Stage)
	}
	if base.GateLockedAt.IsZero() {
		return nil, fmt.Errorf("基准扫描 #%d 没有阈值锁定时刻，无法判断样本外", base.Id)
	}
	if start.Before(base.GateLockedAt) {
		return nil, fmt.Errorf("窗口起点 %s 早于基准扫描 #%d 的阈值锁定时刻 %s：这段数据在定参数时就见过，不是样本外。"+
			"OOS 纪律要求用锁定之后新进的信号流复算", fmtTime(start), base.Id, fmtTime(base.GateLockedAt))
	}
	if dto.Gates != nil {
		return nil, fmt.Errorf("out_of_sample 任务不允许给 gates：阈值必须继承基准扫描 #%d 的冻结值，改阈值就不是 OOS 验证了", base.Id)
	}
	if dto.Space != nil || len(dto.FineCells) > 0 {
		return nil, fmt.Errorf("out_of_sample 任务不允许给 space/fineCells：精算格必须继承基准扫描 #%d，重新选格等于又调了一次参", base.Id)
	}
	return base, nil
}

// resolveOptimizeBaseline 定基线，并把寻优的费用口径盖上去。
func (s *TradeService) resolveOptimizeBaseline(ctx context.Context, dto tradeDTO.CreateSignalOptimizeStudyDTO,
	oosBase *tradeRepository.TradeOptimizeStudy, instanceKey, accountLabel, symbol string) (*SignalBaseline, error) {

	if oosBase != nil {
		params, err := signalParamsFromSnapshot(oosBase.BaselineSnapshot)
		if err != nil {
			return nil, fmt.Errorf("基准扫描 #%d 的基线快照不可用: %w", oosBase.Id, err)
		}
		return &SignalBaseline{Params: params, Source: oosBase.BaselineSource,
			Notes: []string{fmt.Sprintf("基线继承基准扫描 #%d 的冻结快照，未重新读取生产配置", oosBase.Id)}}, nil
	}

	var b *SignalBaseline
	if dto.BaselineParams != nil {
		b = &SignalBaseline{
			Params: applySignalParamKnobs(signal.DefaultParams(), *dto.BaselineParams),
			Source: BaselineSourceRequest,
			Notes: []string{
				"基线由请求体显式给出（baselineParams），不是所选实例的当前生产参数：与生产的偏差需自行确认",
			},
		}
	} else {
		resolved, err := s.ResolveInstanceBaseline(ctx, instanceKey, accountLabel, symbol)
		if err != nil {
			return nil, err
		}
		b = resolved
	}

	// 寻优的费用与执行模型是研究口径（设计文档 §3.4），不是回测缺省。
	// 显式给了就尊重请求，没给一律盖上——费率错了，门控与兜底的结论全错。
	if dto.BaselineParams == nil || dto.BaselineParams.TakerFee == nil {
		b.Params.TakerFee = OptimizeEffectiveTakerFee
		b.Notes = append(b.Notes, fmt.Sprintf("寻优口径覆盖：有效 taker 费率 %.5f（0.012%%/边 = 万6 × 20%%，80%% 返佣），与单次回测的 0.06%% 缺省不同",
			OptimizeEffectiveTakerFee))
	}
	if dto.BaselineParams == nil || dto.BaselineParams.CatastropheOvershootRoiPts == nil {
		b.Params.CatastropheOvershootRoiPts = OptimizeOvershootRoiPts
		b.Notes = append(b.Notes, fmt.Sprintf("寻优口径覆盖：兜底成交过冲惩罚 +%.0f ROI 点（实测 3~10 点）。不加它会系统性高估紧兜底（高 λ）配置",
			OptimizeOvershootRoiPts))
	}
	if b.Params.OrderSize <= 0 {
		b.Notes = append(b.Notes, "基线未给 order_size：按事件自带张数回放（缺省 1 张）。这会让「每格上限多少张」与实盘的下单粒度不完全对齐")
	}
	b.Params = b.Params.Normalize()
	return b, nil
}

// resolveOptimizeConfig 组装（或继承）搜索空间、降噪协议、收敛规则与三关阈值。
func (s *TradeService) resolveOptimizeConfig(dto tradeDTO.CreateSignalOptimizeStudyDTO,
	oosBase *tradeRepository.TradeOptimizeStudy, baseParams signal.Params) (
	signal.SearchSpace, optimizeProtocolPair, signal.Convergence, signal.Gates, error) {

	space := signal.DefaultSearchSpace()
	protocols := optimizeProtocolPair{Fine: signal.FineProtocol(), Coarse: signal.CoarseProtocol()}
	converge := signal.DefaultConvergence()
	gates := signal.DefaultGates()

	if oosBase != nil {
		if err := json.Unmarshal([]byte(oosBase.SpaceSnapshot), &space); err != nil {
			return space, protocols, converge, gates, fmt.Errorf("基准扫描的搜索空间快照解析失败: %w", err)
		}
		if err := json.Unmarshal([]byte(oosBase.ProtocolSnapshot), &protocols); err != nil {
			return space, protocols, converge, gates, fmt.Errorf("基准扫描的降噪协议快照解析失败: %w", err)
		}
		if err := json.Unmarshal([]byte(oosBase.ConvergeSnapshot), &converge); err != nil {
			return space, protocols, converge, gates, fmt.Errorf("基准扫描的收敛规则快照解析失败: %w", err)
		}
		if err := json.Unmarshal([]byte(oosBase.GateSnapshot), &gates); err != nil {
			return space, protocols, converge, gates, fmt.Errorf("基准扫描的三关阈值快照解析失败: %w", err)
		}
		return space.Normalize(), normalizeProtocolPair(protocols), converge.Normalize(), gates.Normalize(), nil
	}

	if in := dto.Space; in != nil {
		if len(in.NetCaps) > 0 {
			space.NetCaps = in.NetCaps
		}
		if len(in.DualCaps) > 0 {
			space.DualCaps = in.DualCaps
		}
		if len(in.StopPcts) > 0 {
			space.StopPcts = in.StopPcts
		}
		if len(in.GatePcts) > 0 {
			space.GatePcts = in.GatePcts
		}
		if in.IncludeDual != nil {
			space.IncludeDual = *in.IncludeDual
		}
		if in.CoarseGatePct > 0 {
			space.CoarseGatePct = in.CoarseGatePct
		}
	}
	if in := dto.Protocol; in != nil {
		f := protocols.Fine
		if len(in.EvalModes) > 0 {
			f.EvalModes = in.EvalModes
		}
		if len(in.OffsetDays) > 0 {
			f.OffsetDays = in.OffsetDays
		}
		if len(in.DropSeeds) > 0 {
			f.DropSeeds = in.DropSeeds
		}
		if in.DropRate > 0 {
			f.DropRate = in.DropRate
		}
		if in.NormalizeDays > 0 {
			f.NormalizeDays = in.NormalizeDays
		}
		if in.ScenarioDays > 0 {
			f.ScenarioDays = in.ScenarioDays
		}
		if in.BootstrapMode != "" {
			f.BootstrapMode = in.BootstrapMode
		}
		if in.BootstrapDraws > 0 {
			f.BootstrapDraws = in.BootstrapDraws
		}
		if in.BootstrapSeed != 0 {
			f.BootstrapSeed = in.BootstrapSeed
		}
		if in.LambdaDenom != "" {
			f.LambdaDenom = in.LambdaDenom
		}
		if in.LambdaMonthDays > 0 {
			f.LambdaMonthDays = in.LambdaMonthDays
		}
		if in.TrendAbsRetPct > 0 {
			f.Regime.TrendAbsRetPct = in.TrendAbsRetPct
		}
		if in.VolRangePct > 0 {
			f.Regime.VolRangePct = in.VolRangePct
		}
		protocols.Fine = f
		// 粗网格沿用精算的全部口径，只换抖动轴：两阶段的费用/情景/λ 口径必须一致，
		// 否则"粗筛掉的格子"与"精算留下的格子"不是同一个量在比较。
		c := f
		c.OffsetDays = []int{0, 2}
		c.DropSeeds = []*int64{nil}
		if len(in.CoarseOffsetDays) > 0 {
			c.OffsetDays = in.CoarseOffsetDays
		}
		if len(in.CoarseDropSeeds) > 0 {
			c.DropSeeds = in.CoarseDropSeeds
		}
		protocols.Coarse = c
	}
	if in := dto.Converge; in != nil {
		if in.TopK > 0 {
			converge.TopK = in.TopK
		}
		if in.MaxCells > 0 {
			converge.MaxCells = in.MaxCells
		}
		if in.KeepAllPositive != nil {
			converge.KeepAllPositive = *in.KeepAllPositive
		}
		if in.ExpandGateAxis != nil {
			converge.ExpandGateAxis = *in.ExpandGateAxis
		}
	}
	if in := dto.Gates; in != nil {
		if in.RiskEquity > 0 {
			gates.RiskEquity = in.RiskEquity
		}
		if in.BearBudgetPct > 0 {
			gates.BearBudgetPct = in.BearBudgetPct
		}
		if in.DdMaxPct > 0 {
			gates.DdMaxPct = in.DdMaxPct
		}
		if in.SignMin > 0 {
			gates.SignMin = in.SignMin
		}
		gates.BearNetP10Min = in.BearNetP10Min
		gates.MaxDrawdownMax = in.MaxDrawdownMax
	} else if baseParams.RiskEquity > 0 {
		// 阈值没给时，本金取基线的 risk_equity 而不是研究里那个 375.73：
		// 两个绝对阈值都是"占本金百分比"，用别人的本金算出来的 U 值毫无意义。
		gates.RiskEquity = baseParams.RiskEquity
	}
	return space.Normalize(), normalizeProtocolPair(protocols), converge.Normalize(), gates.Normalize(), nil
}

func normalizeProtocolPair(p optimizeProtocolPair) optimizeProtocolPair {
	p.Fine = p.Fine.Normalize()
	p.Coarse = p.Coarse.Normalize()
	return p
}

// resolveIncumbentCell 现行线上配置对应的格。它会被强制纳入精算格——
// "现行配置是否被支配"是本任务必须回答的问题，不能因为排名靠后就缺席。
func resolveIncumbentCell(dto tradeDTO.CreateSignalOptimizeStudyDTO,
	oosBase *tradeRepository.TradeOptimizeStudy, baseParams signal.Params) *signal.CellSpec {

	if oosBase != nil && strings.TrimSpace(oosBase.IncumbentSnapshot) != "" {
		var c signal.CellSpec
		if err := json.Unmarshal([]byte(oosBase.IncumbentSnapshot), &c); err == nil && c.Cap > 0 {
			return &c
		}
	}
	if in := dto.Incumbent; in != nil && in.Cap > 0 {
		c := signal.CellSpec{Mode: in.Mode, Cap: in.Cap, StopPct: in.StopPct, GatePct: in.GatePct}
		if c.Mode == "" {
			c.Mode = signal.ModeNet
		}
		return &c
	}
	cap := baseParams.CapOverride
	if cap <= 0 {
		cap = baseParams.Ceiling
	}
	if cap <= 0 {
		return nil
	}
	return &signal.CellSpec{Mode: baseParams.Mode, Cap: cap,
		StopPct: baseParams.CatastropheStopPct, GatePct: baseParams.GateMinProfitPct}
}

// resolveExplicitFineCells 显式精算格（请求给的，或 OOS 从基准任务继承的）。
func (s *TradeService) resolveExplicitFineCells(dto tradeDTO.CreateSignalOptimizeStudyDTO,
	oosBase *tradeRepository.TradeOptimizeStudy) ([]signal.CellSpec, error) {

	if oosBase != nil {
		rows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(int64(oosBase.Id), OptimizeStageFine)
		if err != nil {
			return nil, fmt.Errorf("取基准扫描 #%d 的精算格失败: %w", oosBase.Id, err)
		}
		out := make([]signal.CellSpec, 0, len(rows))
		for _, r := range rows {
			if r.Status == OptimizeCellSkipped {
				continue
			}
			out = append(out, signal.CellSpec{Mode: r.Mode, Cap: r.Cap, StopPct: r.StopPct, GatePct: r.GatePct})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("基准扫描 #%d 没有可继承的精算格", oosBase.Id)
		}
		return out, nil
	}
	out := make([]signal.CellSpec, 0, len(dto.FineCells))
	for i, c := range dto.FineCells {
		if c.Cap <= 0 {
			return nil, fmt.Errorf("fineCells[%d] 的 cap 必须 ≥1", i)
		}
		spec := signal.CellSpec{Mode: c.Mode, Cap: c.Cap, StopPct: c.StopPct, GatePct: c.GatePct}
		if spec.Mode == "" {
			spec.Mode = signal.ModeNet
		}
		out = append(out, spec)
	}
	return out, nil
}

// buildOptimizeCellRows 建格行。参数在实盘校验下非法的格子建成 skipped 而不是
// 直接丢掉：搜索空间的某个角落装不进这个账户，是需要被看见的事实
// （例如实例3 的 order_size=10 让 cap 5/7 的格子物理上开不进一张）。
func buildOptimizeCellRows(studyID int64, stage string, cells []signal.CellSpec, base signal.Params) ([]*tradeRepository.TradeOptimizeCell, int) {
	rows := make([]*tradeRepository.TradeOptimizeCell, 0, len(cells))
	skipped := 0
	for _, spec := range cells {
		params := spec.Params(base)
		row := &tradeRepository.TradeOptimizeCell{
			StudyID: studyID,
			Stage:   stage,
			CellKey: spec.Key(),
			Mode:    params.Mode,
			Cap:     spec.Cap,
			StopPct: spec.StopPct,
			GatePct: spec.GatePct,
			Status:  OptimizeCellPending,
			// 寻优不动 signal_threshold（改它会把精度降到频率级、与事件级格子
			// 不可混排比较），所以每一格恒为事件级。
			Fidelity: signal.Classify(params).Level,
		}
		if snapshot, err := marshalJSON(params); err == nil {
			row.ParamsSnapshot = snapshot
		}
		if err := params.Validate(); err != nil {
			row.Status = OptimizeCellSkipped
			row.ErrorMsg = truncate("参数在实盘校验下非法，本格跳过（不是失败）: "+err.Error(), 500)
			skipped++
		}
		rows = append(rows, row)
	}
	return rows, skipped
}

// RunSignalOptimizeStudy 执行一次寻优：取一份数据 → 粗网格 → 自动收敛 → 精算 →
// 用冻结阈值判定 → 出结论。
func (s *TradeService) RunSignalOptimizeStudy(studyID int64) error {
	study, err := s.tradeOptimizeStudyRepository.FindStudyByID(studyID)
	if err != nil {
		return err
	}
	base, err := signalParamsFromSnapshot(study.BaselineSnapshot)
	if err != nil {
		return s.failOptimizeStudy(studyID, fmt.Errorf("基线快照不可用: %w", err))
	}
	var protocols optimizeProtocolPair
	if err := json.Unmarshal([]byte(study.ProtocolSnapshot), &protocols); err != nil {
		return s.failOptimizeStudy(studyID, fmt.Errorf("降噪协议快照不可用: %w", err))
	}
	protocols = normalizeProtocolPair(protocols)
	space := signal.DefaultSearchSpace()
	_ = json.Unmarshal([]byte(study.SpaceSnapshot), &space)
	space = space.Normalize()
	converge := signal.DefaultConvergence()
	_ = json.Unmarshal([]byte(study.ConvergeSnapshot), &converge)
	converge = converge.Normalize()
	gates := signal.DefaultGates()
	if err := json.Unmarshal([]byte(study.GateSnapshot), &gates); err != nil {
		return s.failOptimizeStudy(studyID, fmt.Errorf("三关阈值快照不可用，判定无从进行: %w", err))
	}
	gates = gates.Normalize()
	var incumbent *signal.CellSpec
	if strings.TrimSpace(study.IncumbentSnapshot) != "" {
		var c signal.CellSpec
		if err := json.Unmarshal([]byte(study.IncumbentSnapshot), &c); err == nil && c.Cap > 0 {
			incumbent = &c
		}
	}

	_ = s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, OptimizeStatusRunning, "")

	// 数据只取一次，全部格子、全部路径共享。事件双写是持续在写的：分别取会让
	// 先跑的格和后跑的格吃到不同的触发集，格间差异里就混进了数据差异。
	// 寻优不动 signal_threshold，所以不需要 dev_sample。
	ds, err := s.loadSignalDataset(signalWindow{
		InstanceKey:  study.InstanceKey,
		AccountLabel: study.AccountLabel,
		Symbol:       study.Symbol,
		PlatformCode: study.PlatformCode,
		Start:        study.StartTime,
		End:          study.EndTime,
	}, false)
	if err != nil {
		s.failAllOptimizeCells(studyID, truncate(err.Error(), 500))
		return s.failOptimizeStudy(studyID, err)
	}
	labels := signal.DayLabels(ds.Bars, protocols.Fine.Regime)
	_ = s.tradeOptimizeStudyRepository.UpdateStudyDataFacts(studyID, len(ds.Signals), ds.KlineCount,
		signal.CountLabel(labels, signal.RegimeTrend), signal.CountLabel(labels, signal.RegimeVol))

	baseInput := signal.Input{Signals: ds.Signals, Bars: ds.Bars, Seed: ds.Seed}
	concurrency := normalizeOptimizeConcurrency(study.Concurrency)

	// ─ 粗网格阶段 ─
	coarseRows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, OptimizeStageCoarse)
	if err != nil {
		return s.failOptimizeStudy(studyID, err)
	}
	if len(coarseRows) > 0 {
		s.runOptimizeCells(studyID, coarseRows, baseInput, study.StartTime, labels, protocols.Coarse, concurrency)
		coarseStats, err := s.reloadCellStats(studyID, OptimizeStageCoarse)
		if err != nil {
			return s.failOptimizeStudy(studyID, err)
		}
		if len(coarseStats) == 0 {
			return s.failOptimizeStudy(studyID, fmt.Errorf("粗网格没有任何一格跑通：精算无从收敛，详见各格 errorMsg"))
		}
		conv := signal.Converge(coarseStats, space, converge, incumbent)
		fineRows, skipped := buildOptimizeCellRows(studyID, OptimizeStageFine, conv.Cells, base)
		if err := s.tradeOptimizeCellRepository.BatchCreateCells(fineRows); err != nil {
			return s.failOptimizeStudy(studyID, fmt.Errorf("建精算格失败: %w", err))
		}
		if skipped > 0 {
			_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 0, 0, skipped, 0)
		}
		note := conv.Note
		if len(conv.Dropped) > 0 {
			note += " | 被截断的格子: " + strings.Join(conv.Dropped, ",")
		}
		_ = s.tradeOptimizeStudyRepository.UpdateStudyStage(studyID, OptimizeStageFine, len(fineRows), truncate(note, 1024))
	}

	// ─ 精算阶段 ─
	fineRows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, OptimizeStageFine)
	if err != nil {
		return s.failOptimizeStudy(studyID, err)
	}
	if len(fineRows) == 0 {
		return s.failOptimizeStudy(studyID, fmt.Errorf("没有任何精算格：收敛规则或显式清单为空"))
	}
	s.runOptimizeCells(studyID, fineRows, baseInput, study.StartTime, labels, protocols.Fine, concurrency)

	return s.concludeOptimizeStudy(studyID, base, gates, incumbent, ds)
}

// runOptimizeCells 并发跑一批格子。每格内部的 N 条路径顺序跑：一格的统计量
// 必须来自同一批路径，拆开并发只会把内存放大而不提速（并发已在格级）。
func (s *TradeService) runOptimizeCells(studyID int64, rows []*tradeRepository.TradeOptimizeCell,
	baseInput signal.Input, windowStart time.Time, labels map[string]string,
	proto signal.Protocol, concurrency int) {

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, row := range rows {
		if row.Status == OptimizeCellSkipped || row.Status == OptimizeCellDone {
			continue // skipped 本来不跑；done 的不重跑（重跑会把 paths 快照写两遍口径）
		}
		wg.Add(1)
		go func(row *tradeRepository.TradeOptimizeCell) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cellID := int64(row.Id)
			defer func() {
				// 单格 panic 不能带走整个任务：其余格子的结果仍然有效。
				if r := recover(); r != nil {
					logrus.Errorf("[signal-optimize] study=%d cell=%s panic: %v", studyID, row.CellKey, r)
					_ = s.tradeOptimizeCellRepository.UpdateCellStatus(cellID, OptimizeCellFailed, fmt.Sprintf("panic: %v", r))
					_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 0, 1, 0, 0)
				}
			}()
			if err := s.runOptimizeCell(studyID, row, baseInput, windowStart, labels, proto); err != nil {
				logrus.Warnf("[signal-optimize] study=%d cell=%s 失败: %v", studyID, row.CellKey, err)
				_ = s.tradeOptimizeCellRepository.UpdateCellStatus(cellID, OptimizeCellFailed, truncate(err.Error(), 500))
				_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 0, 1, 0, 0)
			}
		}(row)
	}
	wg.Wait()
}

// runOptimizeCell 跑一格：展开 N 条抖动路径 → 聚合 → 落库。
func (s *TradeService) runOptimizeCell(studyID int64, row *tradeRepository.TradeOptimizeCell,
	baseInput signal.Input, windowStart time.Time, labels map[string]string, proto signal.Protocol) error {

	cellID := int64(row.Id)
	_ = s.tradeOptimizeCellRepository.UpdateCellStatus(cellID, OptimizeCellRunning, "")

	params, err := signalParamsFromSnapshot(row.ParamsSnapshot)
	if err != nil {
		return err
	}
	spec := signal.CellSpec{Mode: row.Mode, Cap: row.Cap, StopPct: row.StopPct, GatePct: row.GatePct}
	in := baseInput
	in.Params = params

	paths := proto.Paths()
	outcomes := make([]signal.PathOutcome, 0, len(paths))
	failures := make([]string, 0, 2)
	for _, ps := range paths {
		o, err := signal.RunPath(in, ps, windowStart, labels, proto)
		if err != nil {
			// 单条路径失败（典型：偏移过大导致没有 K 线）不废掉整格，但必须
			// 记下来——路径数变了，中位数与一致率的分辨率就跟着变。
			failures = append(failures, fmt.Sprintf("%s: %v", ps.Label(), err))
			continue
		}
		outcomes = append(outcomes, *o)
	}
	if len(outcomes) == 0 {
		return fmt.Errorf("全部 %d 条抖动路径都失败: %s", len(paths), strings.Join(failures, "; "))
	}
	st := signal.AggregateCell(spec, outcomes, labels, proto)
	if len(failures) > 0 {
		st.Notes = append(st.Notes, fmt.Sprintf("本格只跑通 %d/%d 条路径，符号一致率的分辨率因此下降：%s",
			len(outcomes), len(paths), strings.Join(failures, "; ")))
	}

	fields := cellStatsFields(st)
	if snapshot, err := marshalJSON(st.Paths); err == nil {
		fields["paths_snapshot"] = snapshot
	}
	if snapshot, err := marshalJSON(st.Notes); err == nil {
		fields["note_snapshot"] = snapshot
	}
	fields["status"] = OptimizeCellDone
	fields["error_msg"] = ""
	if err := s.tradeOptimizeCellRepository.SaveCellStats(cellID, fields); err != nil {
		return err
	}
	_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 1, 0, 0, len(outcomes))
	return nil
}

// cellStatsFields 把一格的统计量摊成列。落库前统一舍入到 8 位，避免 JSON 里
// 出现 1e-17 这类浮点噪声位被当成有效数字读。
func cellStatsFields(st signal.CellStats) map[string]interface{} {
	f := map[string]interface{}{
		"path_count":       st.PathCount,
		"med_pnl28":        signal.RoundTo(st.MedPnl28, 8),
		"p25_pnl28":        signal.RoundTo(st.P25Pnl28, 8),
		"p75_pnl28":        signal.RoundTo(st.P75Pnl28, 8),
		"iqr_pnl28":        signal.RoundTo(st.IQRPnl28, 8),
		"min_pnl28":        signal.RoundTo(st.MinPnl28, 8),
		"max_pnl28":        signal.RoundTo(st.MaxPnl28, 8),
		"sign_ratio":       signal.RoundTo(st.SignRatio, 6),
		"lambda_bear":      signal.RoundTo(st.LambdaBearPerMonth, 6),
		"mean_stop_loss":   signal.RoundTo(st.MeanStopLoss, 8),
		"stop_budget":      signal.RoundTo(st.StopBudget, 8),
		"stop_count":       st.StopCount,
		"p90_max_drawdown": signal.RoundTo(st.P90MaxDrawdown, 8),
		"max_stack":        st.MaxStack,
		"med_fee":          signal.RoundTo(st.MedFee, 8),
		"med_days":         signal.RoundTo(st.MedDays, 6),
		"med_signal_run":   st.MedSignalRun,
		"fidelity":         st.Fidelity,
	}
	for _, sc := range st.Scenarios {
		switch sc.Scenario {
		case signal.ScenarioBear:
			f["bear_pool_size"] = sc.PoolSize
			f["bear_p10"] = signal.RoundTo(sc.P10, 8)
			f["bear_p50"] = signal.RoundTo(sc.P50, 8)
			f["bear_p90"] = signal.RoundTo(sc.P90, 8)
		case signal.ScenarioChop:
			f["chop_p10"] = signal.RoundTo(sc.P10, 8)
			f["chop_p50"] = signal.RoundTo(sc.P50, 8)
		case signal.ScenarioMixed:
			f["mixed_p10"] = signal.RoundTo(sc.P10, 8)
			f["mixed_p50"] = signal.RoundTo(sc.P50, 8)
		}
	}
	return f
}

// reloadCellStats 从库里把某阶段跑通的格子读回成统计量。
//
// 刻意走"落库再读回"而不是把内存里的结果传下去：断点续跑时已 done 的格子不会
// 重跑，那时唯一的真值就是库里的行。两条路径共用同一份重建逻辑，才不会出现
// "首次执行"与"续跑"算出不同收敛结果。
func (s *TradeService) reloadCellStats(studyID int64, stage string) ([]signal.CellStats, error) {
	rows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, stage)
	if err != nil {
		return nil, err
	}
	out := make([]signal.CellStats, 0, len(rows))
	for _, r := range rows {
		if r.Status != OptimizeCellDone {
			continue
		}
		out = append(out, cellStatsFromRow(r))
	}
	return out, nil
}

// cellStatsFromRow 行 → 统计量（判定与收敛用到的字段全部齐备）。
func cellStatsFromRow(r *tradeRepository.TradeOptimizeCell) signal.CellStats {
	st := signal.CellStats{
		Spec:               signal.CellSpec{Mode: r.Mode, Cap: r.Cap, StopPct: r.StopPct, GatePct: r.GatePct},
		Key:                r.CellKey,
		PathCount:          r.PathCount,
		MedPnl28:           r.MedPnl28,
		P25Pnl28:           r.P25Pnl28,
		P75Pnl28:           r.P75Pnl28,
		IQRPnl28:           r.IqrPnl28,
		MinPnl28:           r.MinPnl28,
		MaxPnl28:           r.MaxPnl28,
		SignRatio:          r.SignRatio,
		LambdaBearPerMonth: r.LambdaBear,
		MeanStopLoss:       r.MeanStopLoss,
		StopBudget:         r.StopBudget,
		StopCount:          r.StopCount,
		P90MaxDrawdown:     r.P90MaxDrawdown,
		MaxStack:           r.MaxStack,
		MedFee:             r.MedFee,
		MedDays:            r.MedDays,
		MedSignalRun:       r.MedSignalRun,
		Fidelity:           r.Fidelity,
		Scenarios: []signal.ScenarioResult{
			{Scenario: signal.ScenarioBear, PoolSize: r.BearPoolSize, P10: r.BearP10, P50: r.BearP50, P90: r.BearP90},
			{Scenario: signal.ScenarioChop, P10: r.ChopP10, P50: r.ChopP50},
			{Scenario: signal.ScenarioMixed, P10: r.MixedP10, P50: r.MixedP50},
		},
	}
	if strings.TrimSpace(r.NoteSnapshot) != "" {
		var notes []string
		if err := json.Unmarshal([]byte(r.NoteSnapshot), &notes); err == nil {
			st.Notes = notes
		}
	}
	return st
}

// concludeOptimizeStudy 用**任务冻结的**阈值统一判定全部精算格，出结论。
//
// 判定放在这里而不是每格跑完就判：阈值是任务级的，逐格判会给"中途改了阈值"
// 留下缝隙；而且支配关系与权衡前沿本身就需要看到全部格子。
func (s *TradeService) concludeOptimizeStudy(studyID int64, base signal.Params, gates signal.Gates,
	incumbent *signal.CellSpec, ds *signalDataset) error {

	rows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, OptimizeStageFine)
	if err != nil {
		return s.failOptimizeStudy(studyID, err)
	}
	stats := make([]signal.CellStats, 0, len(rows))
	rowByKey := map[string]*tradeRepository.TradeOptimizeCell{}
	failed, skipped := 0, 0
	for _, r := range rows {
		switch r.Status {
		case OptimizeCellDone:
			stats = append(stats, cellStatsFromRow(r))
			rowByKey[r.CellKey] = r
		case OptimizeCellSkipped:
			skipped++
		default:
			failed++
		}
	}
	if len(stats) == 0 {
		return s.failOptimizeStudy(studyID, fmt.Errorf("精算阶段没有任何一格跑通，无从判定，详见各格 errorMsg"))
	}

	verdicts := make(map[string]signal.CellVerdict, len(stats))
	passed := 0
	for _, st := range stats {
		v := signal.Judge(st, gates)
		verdicts[st.Key] = v
		if v.Passed {
			passed++
		}
	}
	conclusion := signal.Conclude(stats, verdicts, gates, incumbent)
	frontierByKey := map[string]signal.FrontierPoint{}
	for _, p := range conclusion.Frontier {
		frontierByKey[p.Key] = p
	}

	// 规模不变性破缺：用**账户自己的**风险本金核一遍每格的 cap 能不能实例化。
	// 用研究本金 E 去核对毫无意义——问题恰恰是"小额账户装不下大 cap"。
	scaleEquity := base.RiskEquity
	scaleNote := "核查本金取基线 risk_equity（该账户的真实本金）"
	if scaleEquity <= 0 {
		scaleEquity = gates.RiskEquity
		scaleNote = "基线没有 risk_equity（配置面收敛 r5 未完成），核查本金退回三关阈值里的研究本金：结论只说明「这个本金下能不能装下」，不代表该账户"
	}
	price := 0.0
	if len(ds.Bars) > 0 {
		price = signal.SortBars(ds.Bars)[0].Close
	}
	scaleChecks := signal.CheckScaleInvariance(stats, base, scaleEquity, price)

	incumbentKey := ""
	if incumbent != nil {
		incumbentKey = incumbent.Key()
	}
	for _, st := range stats {
		row := rowByKey[st.Key]
		if row == nil {
			continue
		}
		v := verdicts[st.Key]
		fields := map[string]interface{}{
			"ok_sign":      boolToInt(v.OkSign),
			"ok_bear":      boolToInt(v.OkBear),
			"ok_dd":        boolToInt(v.OkDd),
			"ok_budget":    boolToInt(v.OkStopBudget),
			"passed":       boolToInt(v.Passed),
			"pass_count":   v.PassCnt,
			"verdict_note": truncate(strings.Join(v.Reasons, " | "), 1024),
			"dominated_by": truncate(strings.Join(conclusion.DominatedBy[st.Key], ","), 512),
			"is_incumbent": boolToInt(st.Key == incumbentKey),
		}
		if p, ok := frontierByKey[st.Key]; ok {
			fields["on_dd_frontier"] = boolToInt(p.OnDdFrontier)
			fields["on_bear_frontier"] = boolToInt(p.OnBearFrontier)
		}
		if err := s.tradeOptimizeCellRepository.SaveCellVerdict(int64(row.Id), fields); err != nil {
			logrus.Warnf("[signal-optimize] study=%d cell=%s 回填判定失败: %v", studyID, st.Key, err)
		}
	}

	conclusion.Lines = append(conclusion.Lines, scaleInvarianceLines(scaleChecks, scaleNote)...)
	conclusionJSON, _ := marshalJSON(conclusion)
	frontierJSON, _ := marshalJSON(conclusion.Frontier)
	scaleJSON, _ := marshalJSON(scaleChecks)
	if err := s.tradeOptimizeStudyRepository.UpdateStudyConclusion(studyID,
		conclusion.Verdict, passed, conclusionJSON, frontierJSON, scaleJSON); err != nil {
		return s.failOptimizeStudy(studyID, err)
	}
	_ = s.tradeOptimizeStudyRepository.UpdateStudyStage(studyID, OptimizeStageConcluded, len(rows), "")

	status, errMsg := OptimizeStatusDone, ""
	if failed > 0 {
		status = OptimizeStatusPartial
		errMsg = fmt.Sprintf("%d 个精算格执行失败、%d 个因参数非法跳过：结论建立在剩下的 %d 格上，缺失的格子不代表结果差",
			failed, skipped, len(stats))
	} else if skipped > 0 {
		errMsg = fmt.Sprintf("%d 个格子因参数在实盘校验下非法而跳过（不是失败）：它们本就装不进这个账户", skipped)
	}
	// UpdateStudyStage 会把 converge_note 清空，所以状态放在最后写，
	// 避免把 partial 的说明也一起冲掉。
	return s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, status, truncate(errMsg, 500))
}

// scaleInvarianceLines 把规模核查折成结论里的一两句话。
func scaleInvarianceLines(checks []signal.ScaleInvarianceCheck, note string) []string {
	broken := make([]string, 0, 4)
	for _, c := range checks {
		if !c.Feasible {
			broken = append(broken, fmt.Sprintf("%s(需%d张/公式%d张)", c.Key, c.CellCap, c.FormulaCap))
		}
	}
	if len(broken) == 0 {
		return []string{fmt.Sprintf("规模不变性：全部精算格的上限在当前本金下都能按公式实例化（%s）", note)}
	}
	return []string{fmt.Sprintf(
		"规模不变性破缺（%s）：%d 格的上限在当前本金下装不下 —— %s。cap 的摊平动态依赖**绝对张数**，"+
			"在这个账户上只能验证 S / gate 效应的方向性，完整参数包的收益要等本金到位才能兑现",
		note, len(broken), strings.Join(broken, "、"))}
}

func (s *TradeService) failOptimizeStudy(studyID int64, err error) error {
	_ = s.tradeOptimizeStudyRepository.UpdateStudyStatus(studyID, OptimizeStatusFailed, truncate(err.Error(), 500))
	return err
}

// failAllOptimizeCells 取数失败是任务级失败：逐格也标失败，否则页面上会有一堆
// 永远 pending 的格子等不到任何解释。
func (s *TradeService) failAllOptimizeCells(studyID int64, msg string) {
	rows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, "")
	if err != nil {
		return
	}
	n := 0
	for _, r := range rows {
		if r.Status == OptimizeCellSkipped {
			continue
		}
		_ = s.tradeOptimizeCellRepository.UpdateCellStatus(int64(r.Id), OptimizeCellFailed, msg)
		n++
	}
	if n > 0 {
		_ = s.tradeOptimizeStudyRepository.BumpStudyProgress(studyID, 0, n, 0, 0)
	}
}

// ─── 读侧 ────────────────────────────────────────────────────────────────────

// ListSignalOptimizeStudies 分页查询寻优任务。
func (s *TradeService) ListSignalOptimizeStudies(q tradeDTO.SignalOptimizeStudyQueryDTO) (*tradeDTO.SignalOptimizeStudyListDTO, error) {
	rows, total, err := s.tradeOptimizeStudyRepository.FindStudies(
		strings.TrimSpace(q.InstanceKey), strings.TrimSpace(q.AccountLabel), strings.TrimSpace(q.SampleKind),
		q.Page, q.PageSize)
	if err != nil {
		return nil, err
	}
	list := make([]tradeDTO.SignalOptimizeStudyDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, optimizeStudyToDTO(r))
	}
	return &tradeDTO.SignalOptimizeStudyListDTO{Total: total, List: list}, nil
}

// GetSignalOptimizeStudyDetail 任务详情：冻结的空间/协议/阈值 + 粗网格 + 精算结果
// + 结论（含无解分支的权衡前沿）。
//
// 结果矩阵刻意分成 coarse / fine 两段返回，不合并成一张榜：粗网格只有 4 条路径，
// 与精算格放同一张表里排序，等于邀请人拿分辨率不够的数字下结论。
func (s *TradeService) GetSignalOptimizeStudyDetail(studyID int64, withPaths bool) (*tradeDTO.SignalOptimizeStudyDetailDTO, error) {
	study, err := s.tradeOptimizeStudyRepository.FindStudyByID(studyID)
	if err != nil {
		return nil, err
	}
	rows, err := s.tradeOptimizeCellRepository.FindCellsByStudy(studyID, "")
	if err != nil {
		return nil, err
	}
	out := &tradeDTO.SignalOptimizeStudyDetailDTO{Study: optimizeStudyToDTO(study)}
	out.Space = jsonToMap(study.SpaceSnapshot)
	out.Protocol = jsonToMap(study.ProtocolSnapshot)
	out.Converge = jsonToMap(study.ConvergeSnapshot)
	out.Gates = jsonToMap(study.GateSnapshot)
	out.Incumbent = jsonToMap(study.IncumbentSnapshot)
	out.Conclusion = jsonToMap(study.Conclusion)
	out.Frontier = jsonToMapSlice(study.FrontierSnapshot)
	out.ScaleChecks = jsonToMapSlice(study.ScaleSnapshot)

	if params, err := signalParamsFromSnapshot(study.BaselineSnapshot); err == nil {
		notes := make([]string, 0, 4)
		for _, n := range strings.Split(study.BaselineNote, " | ") {
			if n = strings.TrimSpace(n); n != "" {
				notes = append(notes, n)
			}
		}
		out.Baseline = tradeDTO.SignalBaselineDTO{
			Source: study.BaselineSource, Params: paramsToMap(params), Notes: notes,
		}
	}

	for _, r := range rows {
		row := optimizeCellToDTO(r, withPaths)
		if r.Stage == OptimizeStageFine {
			out.Fine = append(out.Fine, row)
		} else {
			out.Coarse = append(out.Coarse, row)
		}
	}
	sortOptimizeRows(out.Coarse)
	sortOptimizeRows(out.Fine)

	out.Warnings = optimizeWarnings(study, out.Fine)
	return out, nil
}

// sortOptimizeRows 组内排序：先按通过的关数降序（判定优先于收益），再按中位
// PnL 降序。**不允许只按中位 PnL 排**——那正是"按 PnL 排序会选出纯噪声格子"
// 的那条路。未跑通的行沉到末尾。
func sortOptimizeRows(rows []tradeDTO.SignalOptimizeCellRowDTO) {
	rank := func(r tradeDTO.SignalOptimizeCellRowDTO) int {
		if r.Status == OptimizeCellDone {
			return 0
		}
		return 1
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			a, b := rows[j-1], rows[j]
			if rank(a) != rank(b) {
				if rank(a) > rank(b) {
					rows[j-1], rows[j] = b, a
					continue
				}
				break
			}
			if a.PassCount != b.PassCount {
				if a.PassCount < b.PassCount {
					rows[j-1], rows[j] = b, a
					continue
				}
				break
			}
			if b.MedPnl28 > a.MedPnl28 {
				rows[j-1], rows[j] = b, a
				continue
			}
			break
		}
	}
}

// optimizeWarnings 整任务级提醒。这些不是装饰：其中任意一条被忽略，读到的
// 数字就会被当成比实际更强的证据。
func optimizeWarnings(study *tradeRepository.TradeOptimizeStudy, fine []tradeDTO.SignalOptimizeCellRowDTO) []string {
	w := make([]string, 0, 8)
	w = append(w, "本任务不输出「最优参数」、不修改任何线上参数：三关是否决式的，通过只代表「没被否掉」")
	if study.SampleKind == SampleKindInSample {
		w = append(w, "in_sample：窗口内的收益必然高估（参数是在这段数据上挑的）。锁定后请以本任务为基准建 out_of_sample 任务复算")
	} else {
		w = append(w, fmt.Sprintf("out_of_sample：阈值与精算格继承任务 #%d，未重新调参", study.OosBaseID))
	}
	if study.GateNote != "" {
		w = append(w, "预注册阈值（"+fmtTime(study.GateLockedAt)+" 锁定，跑完不可改）："+study.GateNote)
	}
	if study.Status == OptimizeStatusRunning || study.Status == OptimizeStatusPending {
		w = append(w, fmt.Sprintf("任务仍在执行（%d 格已完成 / %d 格失败）：现在看到的判定与前沿都会变",
			study.DoneCellCount, study.FailedCellCount))
	}
	if study.Status == OptimizeStatusPartial {
		w = append(w, "任务部分失败：结论只建立在跑通的格子上，缺失的格子不代表结果差")
	}
	if study.SkipCellCount > 0 {
		w = append(w, fmt.Sprintf("%d 个格子因参数在实盘校验下非法而跳过（不是失败）：它们本就装不进这个账户", study.SkipCellCount))
	}
	if study.TrendDayCount > 0 && study.TrendDayCount < 10 {
		w = append(w, fmt.Sprintf("窗口内只有 %d 个单边日：熊市月情景与 λ_bear 都建立在这几天上，样本极薄，结论按方向读、不按数值读", study.TrendDayCount))
	}
	if study.Verdict == signal.VerdictNoSolution {
		w = append(w, "无解分支：没有格子通过三关。按设计不放宽标准硬凑参数，交付的是权衡前沿 + 结构性方案立项建议")
	}
	if study.BaselineNote != "" {
		w = append(w, "基线取值说明（含配置面收敛未完成导致的兜底）："+study.BaselineNote)
	}
	frequency := 0
	for _, r := range fine {
		if r.Fidelity == signal.FidelityFrequency {
			frequency++
		}
	}
	if frequency > 0 {
		w = append(w, fmt.Sprintf("有 %d 格被判为频率级精度：寻优不该动 signal_threshold（改它只能推 λ(θ)、推不出触发时刻），这些格子不可与事件级格子比大小", frequency))
	}
	return w
}

// GetSignalOptimizeDefaults 发起表单的缺省预览：空间会展开成多少格、降噪协议
// 是哪几条路径、三关阈值多少、预估要跑多少次回放。
func (s *TradeService) GetSignalOptimizeDefaults() *tradeDTO.SignalOptimizeDefaultsDTO {
	space := signal.DefaultSearchSpace().Normalize()
	fine := signal.FineProtocol().Normalize()
	coarse := signal.CoarseProtocol().Normalize()
	conv := signal.DefaultConvergence().Normalize()
	gates := signal.DefaultGates().Normalize()

	coarseCells := space.CoarseCells()
	keys := make([]string, 0, len(coarseCells))
	for _, c := range coarseCells {
		keys = append(keys, c.Key())
	}
	finePaths, coarsePaths := fine.Paths(), coarse.Paths()
	fineLabels := make([]string, 0, len(finePaths))
	for _, p := range finePaths {
		fineLabels = append(fineLabels, p.Label())
	}
	coarseLabels := make([]string, 0, len(coarsePaths))
	for _, p := range coarsePaths {
		coarseLabels = append(coarseLabels, p.Label())
	}
	fineCells := conv.TopK
	if conv.ExpandGateAxis && len(space.GatePcts) > 1 {
		fineCells *= len(space.GatePcts)
	}
	if fineCells > conv.MaxCells {
		fineCells = conv.MaxCells
	}

	return &tradeDTO.SignalOptimizeDefaultsDTO{
		Space:            structToMap(space),
		CoarseCellCount:  len(coarseCells),
		CoarseCells:      keys,
		Protocol:         structToMap(optimizeProtocolPair{Fine: fine, Coarse: coarse}),
		FinePathCount:    len(finePaths),
		FinePaths:        fineLabels,
		CoarsePathCount:  len(coarsePaths),
		CoarsePaths:      coarseLabels,
		Converge:         structToMap(conv),
		Gates:            structToMap(gates),
		GateNote:         gates.Describe(),
		EstimatedReplays: len(coarseCells)*len(coarsePaths) + fineCells*len(finePaths),
		Notes: []string{
			"搜索空间取自 docs/argus_single/2026-07-02-风险参数联合重推研究设计.md §3.3；trail 档位固定 champion 值以防组合爆炸",
			"signal_threshold 不在搜索空间内：改它会把精度降到频率级（只能推 λ(θ)、推不出触发时刻），与事件级格子不可混排比较",
			"降噪协议取自同一文档 §3.2；决策只依据「符号一致率 ≥ 阈值且中位数达标」，不看单路径点估计",
			"三关阈值在任务创建时锁定并落库，跑完不接受修改；要换阈值只能建新任务",
			fmt.Sprintf("费用与执行模型：有效 taker %.5f（0.012%%/边）、兜底成交过冲 +%.0f ROI 点、执行摩擦按 0（实测中位 −0.71bp）",
				OptimizeEffectiveTakerFee, OptimizeOvershootRoiPts),
			"cap=999 是「∞ 上限」对照格，只用来验证回撤关会不会被违反，不是可上线配置",
		},
	}
}

// ─── 映射 ────────────────────────────────────────────────────────────────────

func optimizeStudyToDTO(r *tradeRepository.TradeOptimizeStudy) tradeDTO.SignalOptimizeStudyDTO {
	return tradeDTO.SignalOptimizeStudyDTO{
		ID:              int64(r.Id),
		Name:            r.Name,
		InstanceKey:     r.InstanceKey,
		AccountLabel:    r.AccountLabel,
		PlatformCode:    r.PlatformCode,
		CoinCode:        r.CoinCode,
		Symbol:          r.Symbol,
		StartTime:       fmtTime(r.StartTime),
		EndTime:         fmtTime(r.EndTime),
		SampleKind:      r.SampleKind,
		SampleNote:      r.SampleNote,
		OosBaseID:       r.OosBaseID,
		Stage:           r.Stage,
		Status:          r.Status,
		ErrorMsg:        r.ErrorMsg,
		Concurrency:     r.Concurrency,
		CoarseCellCount: r.CoarseCellCount,
		FineCellCount:   r.FineCellCount,
		DoneCellCount:   r.DoneCellCount,
		FailedCellCount: r.FailedCellCount,
		SkipCellCount:   r.SkipCellCount,
		ReplayCount:     r.ReplayCount,
		ConvergeNote:    r.ConvergeNote,
		SignalCount:     r.SignalCount,
		KlineCount:      r.KlineCount,
		TrendDayCount:   r.TrendDayCount,
		VolDayCount:     r.VolDayCount,
		GateLockedAt:    fmtTime(r.GateLockedAt),
		GateNote:        r.GateNote,
		Verdict:         r.Verdict,
		PassedCellCount: r.PassedCellCount,
		BaselineSource:  r.BaselineSource,
		CreatedTime:     fmtTime(r.CreatedTime),
	}
}

func optimizeCellToDTO(r *tradeRepository.TradeOptimizeCell, withPaths bool) tradeDTO.SignalOptimizeCellRowDTO {
	out := tradeDTO.SignalOptimizeCellRowDTO{
		ID:             int64(r.Id),
		Stage:          r.Stage,
		Key:            r.CellKey,
		Mode:           r.Mode,
		Cap:            r.Cap,
		StopPct:        r.StopPct,
		GatePct:        r.GatePct,
		Fidelity:       r.Fidelity,
		Status:         r.Status,
		ErrorMsg:       r.ErrorMsg,
		PathCount:      r.PathCount,
		MedPnl28:       r.MedPnl28,
		P25Pnl28:       r.P25Pnl28,
		P75Pnl28:       r.P75Pnl28,
		IqrPnl28:       r.IqrPnl28,
		MinPnl28:       r.MinPnl28,
		MaxPnl28:       r.MaxPnl28,
		SignRatio:      r.SignRatio,
		LambdaBear:     r.LambdaBear,
		MeanStopLoss:   r.MeanStopLoss,
		StopBudget:     r.StopBudget,
		StopCount:      r.StopCount,
		P90MaxDrawdown: r.P90MaxDrawdown,
		MaxStack:       r.MaxStack,
		MedFee:         r.MedFee,
		MedDays:        r.MedDays,
		MedSignalRun:   r.MedSignalRun,
		BearPoolSize:   r.BearPoolSize,
		BearP10:        r.BearP10,
		BearP50:        r.BearP50,
		BearP90:        r.BearP90,
		ChopP10:        r.ChopP10,
		ChopP50:        r.ChopP50,
		MixedP10:       r.MixedP10,
		MixedP50:       r.MixedP50,
		OkSign:         r.OkSign == 1,
		OkBear:         r.OkBear == 1,
		OkDd:           r.OkDd == 1,
		OkBudget:       r.OkBudget == 1,
		Passed:         r.Passed == 1,
		PassCount:      r.PassCount,
		VerdictNote:    r.VerdictNote,
		IsIncumbent:    r.IsIncumbent == 1,
		OnDdFrontier:   r.OnDdFrontier == 1,
		OnBearFrontier: r.OnBearFrontier == 1,
	}
	if r.DominatedBy != "" {
		out.DominatedBy = strings.Split(r.DominatedBy, ",")
	}
	if strings.TrimSpace(r.NoteSnapshot) != "" {
		var notes []string
		if err := json.Unmarshal([]byte(r.NoteSnapshot), &notes); err == nil {
			out.Notes = notes
		}
	}
	if params, err := signalParamsFromSnapshot(r.ParamsSnapshot); err == nil {
		out.Params = paramsToMap(params)
	}
	if withPaths {
		out.Paths = jsonToMapSlice(r.PathsSnapshot)
	}
	return out
}

func normalizeOptimizeConcurrency(v int) int {
	if v <= 0 {
		return DefaultOptimizeConcurrency
	}
	if v > MaxOptimizeConcurrency {
		return MaxOptimizeConcurrency
	}
	return v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func marshalJSON(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func structToMap(v interface{}) map[string]interface{} {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func jsonToMap(s string) map[string]interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func jsonToMapSlice(s string) []map[string]interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []map[string]interface{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
