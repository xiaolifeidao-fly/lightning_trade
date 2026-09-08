package dto

// 本文件是 episode（持仓生命周期）读侧的出入参定义。
//
// 时间口径与 dto.go 一致：全部是本地墙钟串 `2006-01-02 15:04:05`；
// "没有这个时刻"用**空串**表示（截断头的 openedAt、仍持仓的 closedAt），
// 不用零值时间，前端只判空串即可。

// ─── 查询入参 ────────────────────────────────────────────────────────────────

// EpisodeQueryDTO 持仓 episode 列表的筛选条件。
type EpisodeQueryDTO struct {
	PageIndex int `form:"pageIndex"`
	PageSize  int `form:"pageSize"`

	InstanceKey  string `form:"instanceKey"`
	InstanceKeys string `form:"instanceKeys"`

	Start string `form:"start"`
	End   string `form:"end"`
	// TimeField 窗口按哪个时刻收：opened（默认，决策时刻口径）/ closed / overlap。
	// 同一批持仓按不同口径数出来的条数不同，所以它是显式入参并随返回体回显。
	TimeField string `form:"timeField"`

	Instrument    string `form:"instrument"`
	Instruments   string `form:"instruments"`
	AccountLabel  string `form:"accountLabel"`
	AccountLabels string `form:"accountLabels"`
	Uid           string `form:"uid"`
	Uids          string `form:"uids"`

	Side           string `form:"side"`
	Variants       string `form:"variants"`
	ConfigVersions string `form:"configVersions"`
	// ExitKinds 逗号分隔的出场方式；'open' 是保留值，表示仍持仓（exit_kind IS NULL）。
	ExitKinds string `form:"exitKinds"`
	// Status all（默认）/ open（仍持仓）/ closed（已出场）。
	Status string `form:"status"`
	// StrategyOnly=1 只看计入策略胜率的持仓（剔除交易所侧 / 人工出场与仍持仓）。
	StrategyOnly int `form:"strategyOnly"`

	Order string `form:"order"` // ts_desc（默认）/ ts_asc
}

// EpisodeStatsQueryDTO 出场归因分布与胜率口径的聚合入参。
type EpisodeStatsQueryDTO struct {
	InstanceKey  string `form:"instanceKey"`
	InstanceKeys string `form:"instanceKeys"`
	Instrument   string `form:"instrument"`
	AccountLabel string `form:"accountLabel"`
	Start        string `form:"start"`
	End          string `form:"end"`
	TimeField    string `form:"timeField"`
}

// SliceCompareQueryDTO 按 config_version 与实例切片对比的入参。
type SliceCompareQueryDTO struct {
	InstanceKey  string `form:"instanceKey"`
	InstanceKeys string `form:"instanceKeys"`
	Instrument   string `form:"instrument"`
	AccountLabel string `form:"accountLabel"`
	Start        string `form:"start"`
	End          string `form:"end"`
	TimeField    string `form:"timeField"`
}

// ─── episode 主体 ────────────────────────────────────────────────────────────

// EpisodeDTO 一次完整持仓。字段基本原样透传派生表，派生项只有六个：
// exitLabel / statusLabel / truncatedHead / givebackPct / pnlKnown / depthFidelityLabel。
type EpisodeDTO struct {
	EpisodeID    uint64 `json:"episodeId"`
	InstanceKey  string `json:"instanceKey"`
	Uid          string `json:"uid"`
	AccountLabel string `json:"accountLabel"`
	Instrument   string `json:"instrument"`
	Side         string `json:"side"`

	FirstEventAt string `json:"firstEventAt"`
	OpenedAt     string `json:"openedAt"` // 空串 = 建仓在数据窗口之前（截断头）
	LastEventAt  string `json:"lastEventAt"`
	ClosedAt     string `json:"closedAt"` // 空串 = 仍持仓
	DurationSec  *int   `json:"durationSec"`

	ExitKind    string `json:"exitKind"` // 空串 = 仍持仓
	ExitLabel   string `json:"exitLabel"`
	Status      string `json:"status"` // open / closed
	StatusLabel string `json:"statusLabel"`
	// StrategyAttributable 0 表示这笔不计入策略胜率：external_close /
	// manual_close 是交易所侧或人工操作，仍持仓的结果未定。
	StrategyAttributable bool `json:"strategyAttributable"`
	// TruncatedHead 建仓决策不在数据窗口内，entrySizeTotal 少记了 hiddenSize 张。
	TruncatedHead bool `json:"truncatedHead"`

	AddCount       int     `json:"addCount"`
	ReduceCount    int     `json:"reduceCount"`
	EntrySizeTotal int     `json:"entrySizeTotal"`
	MaxSize        int     `json:"maxSize"`
	OpenSize       float64 `json:"openSize"`
	HiddenSize     float64 `json:"hiddenSize"`

	// MinRoiPctObserved / MaxRoiPctObserved 名字里的 observed 不是修辞：
	// loss_alert 是「ROI < −150% 才发 + 5 分钟冷却」的双重采样，只能给出
	// 真实极值的下界。
	MinRoiPctObserved *float64 `json:"minRoiPctObserved"`
	MaxRoiPctObserved *float64 `json:"maxRoiPctObserved"`
	PeakPct           *float64 `json:"peakPct"`
	ExitRoiPct        *float64 `json:"exitRoiPct"`
	// GivebackPct 回吐比例 (peakPct − exitRoiPct) / peakPct × 100；
	// peakPct 缺失或 ≤ 0（trail 未激活过）时为 nil，不补 0。
	GivebackPct *float64 `json:"givebackPct"`

	Pnl             float64 `json:"pnl"`
	PnlStrategy     float64 `json:"pnlStrategy"`
	UnattributedPnl float64 `json:"unattributedPnl"`
	// PnlKnown 本 episode 期间没有盈亏未知的实现（2026-07-24 之前的反向减仓
	// 只写 size 不写 pnl）。false 时 pnl 偏小且偏差方向未知。
	PnlKnown         bool `json:"pnlKnown"`
	RealizedEvents   int  `json:"realizedEvents"`
	MissingPnlEvents int  `json:"missingPnlEvents"`

	CapSkipCount       int  `json:"capSkipCount"`
	GateBlockCount     int  `json:"gateBlockCount"`
	TrendSkipCount     int  `json:"trendSkipCount"`
	LossAlertCount     int  `json:"lossAlertCount"`
	BalanceSampleCount int  `json:"balanceSampleCount"`
	UplSampleCount     int  `json:"uplSampleCount"`
	PositionGapCount   int  `json:"positionGapCount"`
	HasPositionGap     bool `json:"hasPositionGap"`

	Variant       string `json:"variant"`
	ConfigVersion uint64 `json:"configVersion"`
	// DepthFidelity minute / mixed / alert_sampled，决定浮盈轨迹能画多细。
	DepthFidelity      string `json:"depthFidelity"`
	DepthFidelityLabel string `json:"depthFidelityLabel"`
	RebuiltAt          string `json:"rebuiltAt"`
}

// EpisodeListDTO 列表返回体。分页数据在 Page 里，口径与提示挂在外层。
type EpisodeListDTO struct {
	Total int           `json:"total"`
	Data  []*EpisodeDTO `json:"data"`

	Window    WindowDTO `json:"window"`
	TimeField string    `json:"timeField"`
	// TimeFieldNotice 说清当前口径会漏掉什么。同一批持仓按 opened / closed /
	// overlap 数出来的条数不同，不写明就会变成"页面和报告对不上"。
	TimeFieldNotice string `json:"timeFieldNotice"`
	CrossInstance   bool   `json:"crossInstance"`
	Notice          string `json:"notice"`
	// Rebuild 派生表的新鲜度。episode 目前只能手工重建，页面必须能说出
	// "这批数据派生到哪一刻"，否则滞后会被读成"最近没有持仓"。
	Rebuild EpisodeRebuildDTO `json:"rebuild"`
}

// EpisodeRebuildDTO 派生表的新鲜度探针。
type EpisodeRebuildDTO struct {
	EpisodeCount  int64  `json:"episodeCount"`
	LastRebuiltAt string `json:"lastRebuiltAt"`
	// DerivedThroughTs 派生结果覆盖到的最末事件时刻。
	DerivedThroughTs string `json:"derivedThroughTs"`
	// LatestEventTs 事件表里的最末事件时刻。它明显晚于 DerivedThroughTs 就是滞后。
	LatestEventTs string `json:"latestEventTs"`
	LagSeconds    *int64 `json:"lagSeconds"`
	Stale         bool   `json:"stale"`
	Notice        string `json:"notice"`
}

// ─── episode 详情 ────────────────────────────────────────────────────────────

// EpisodeEntryDTO 一次建仓决策及其决策归集口径下的盈亏。
type EpisodeEntryDTO struct {
	EntryID   uint64 `json:"entryId"`
	DecidedAt string `json:"decidedAt"`
	Side      string `json:"side"`

	AddedSize  int     `json:"addedSize"`
	OrderSize  *int    `json:"orderSize"`
	ClosedSize float64 `json:"closedSize"`
	OpenSize   float64 `json:"openSize"`

	AttributedPnl         float64 `json:"attributedPnl"`
	AttributedPnlStrategy float64 `json:"attributedPnlStrategy"`
	MissingPnlEvents      int     `json:"missingPnlEvents"`
	PnlKnown              bool    `json:"pnlKnown"`

	Variant       string   `json:"variant"`
	ConfigVersion uint64   `json:"configVersion"`
	GapBp         *float64 `json:"gapBp"`
	StrengthLevel string   `json:"strengthLevel"`
	AvgPx         *float64 `json:"avgPx"`
	LastPx        *float64 `json:"lastPx"`
}

// EpisodeRoiPointDTO 一个 ROI% 观测点。
//
// 这些点是**离散观测**不是连续曲线：ROI 只在事件里被写下来
// （gate_block / loss_alert / 出场事件），中间没有任何观测。前端必须画成点
// 或虚线，不能连成实线让人以为中间也测过。
type EpisodeRoiPointDTO struct {
	Ts     string  `json:"ts"`
	RoiPct float64 `json:"roiPct"`
	Event  string  `json:"event"`
	Label  string  `json:"label"`
	// Kind observed（真实观测）/ peak（平仓事件带的 trail 峰值，时刻未知，
	// 挂在出场时刻上只是为了同屏可见）。
	Kind string `json:"kind"`
}

// EpisodeUplPointDTO 一条心跳上的浮盈观测。
//
// upl 是账户级未实现盈亏（单账户只做一个合约，可视同本 episode 的浮盈），
// 单位是 U 不是 %。**不换算成 ROI%**：ROI = upl / 保证金，而保证金没有落库，
// 靠事件里的 (pnl, roi_pct) 反推需要假定持仓期间保证金不变——那正是会造出
// "看起来很精确其实是拟合"的那类伪影。
type EpisodeUplPointDTO struct {
	Ts      string   `json:"ts"`
	Upl     *float64 `json:"upl"`
	Equity  *float64 `json:"equity"`
	NetSize *int     `json:"netSize"`
}

// EpisodeTimelineItemDTO 事件时间轴上的一项。
type EpisodeTimelineItemDTO struct {
	Ts      string `json:"ts"`
	EventID uint64 `json:"eventId"`
	Event   string `json:"event"`
	Label   string `json:"label"`
	// Tone ok / warn / err / mute，供前端选轴点颜色，不让前端各写一套映射。
	Tone      string   `json:"tone"`
	Side      string   `json:"side"`
	NetSize   *int     `json:"netSize"`
	OrderSize *int     `json:"orderSize"`
	AvgPx     *float64 `json:"avgPx"`
	LastPx    *float64 `json:"lastPx"`
	RoiPct    *float64 `json:"roiPct"`
	Pnl       *float64 `json:"pnl"`
	PeakPct   *float64 `json:"peakPct"`
	GapBp     *float64 `json:"gapBp"`
	GateKind  string   `json:"gateKind"`
	GateLabel string   `json:"gateLabel"`
	Reason    string   `json:"reason"`
	// IsDecision 这条事件产生了一笔建仓决策（对应一行 episode_entry）。
	IsDecision bool `json:"isDecision"`
}

// EpisodeExitAttributionDTO 出场归因。
type EpisodeExitAttributionDTO struct {
	ExitKind  string `json:"exitKind"`
	ExitLabel string `json:"exitLabel"`
	// Attributable false 时 CountedInWinRate 也是 false，Reason 说明为什么。
	Attributable     bool     `json:"attributable"`
	CountedInWinRate bool     `json:"countedInWinRate"`
	Reason           string   `json:"reason"`
	ExitRoiPct       *float64 `json:"exitRoiPct"`
	PeakPct          *float64 `json:"peakPct"`
	GivebackPct      *float64 `json:"givebackPct"`
	// GivebackNote peakPct 缺失或 reduce_to_zero 时的口径说明。
	GivebackNote string `json:"givebackNote"`
}

// EpisodeDetailDTO 持仓生命周期抽屉的一次性返回体。
type EpisodeDetailDTO struct {
	Episode  EpisodeDTO                `json:"episode"`
	Exit     EpisodeExitAttributionDTO `json:"exit"`
	Entries  []EpisodeEntryDTO         `json:"entries"`
	Timeline []EpisodeTimelineItemDTO  `json:"timeline"`
	RoiTrack []EpisodeRoiPointDTO      `json:"roiTrack"`
	UplTrack []EpisodeUplPointDTO      `json:"uplTrack"`

	// UplTrackAvailable depth_fidelity = alert_sampled 时恒为 false：
	// 2026-07-21 之前的心跳没有 upl，那段时间的浮盈轨迹**不存在**，
	// 只有 loss_alert 打出来的几个下界点。
	UplTrackAvailable bool   `json:"uplTrackAvailable"`
	UplTrackNotice    string `json:"uplTrackNotice"`
	RoiTrackNotice    string `json:"roiTrackNotice"`
	// EventTruncated 事件条数命中上限，时间轴不完整。
	EventTruncated bool `json:"eventTruncated"`
	// UplTruncated 心跳条数命中上限，浮盈轨迹不完整。
	UplTruncated bool `json:"uplTruncated"`
}

// ─── 出场归因分布 ────────────────────────────────────────────────────────────

// ExitKindBucketDTO 一种出场方式的统计。
type ExitKindBucketDTO struct {
	ExitKind  string  `json:"exitKind"` // 空串 = 仍持仓
	Label     string  `json:"label"`
	Count     int64   `json:"count"`
	Share     float64 `json:"share"`
	Pnl       float64 `json:"pnl"`
	Wins      int64   `json:"wins"`
	WinRate   *float64 `json:"winRate"` // 不计入策略胜率的那两类恒为 nil
	CountedIn bool    `json:"countedInWinRate"`
}

// EpisodeStatsDTO 出场归因分布 + 胜率口径。
type EpisodeStatsDTO struct {
	Window        WindowDTO `json:"window"`
	TimeField     string    `json:"timeField"`
	InstanceKeys  []string  `json:"instanceKeys"`
	CrossInstance bool      `json:"crossInstance"`
	Notice        string    `json:"notice"`

	Total     int64 `json:"total"`
	Closed    int64 `json:"closed"`
	StillOpen int64 `json:"stillOpen"`
	// Attributable 计入策略胜率的条数；Excluded 是被剔除的条数（交易所侧 /
	// 人工出场），它们的盈亏仍计入 Pnl。
	Attributable int64 `json:"attributable"`
	Excluded     int64 `json:"excluded"`
	Wins         int64 `json:"wins"`
	// WinRate 只在 Attributable > 0 时有值；分母是 Attributable，不是 Total。
	WinRate *float64 `json:"winRate"`

	Pnl             float64 `json:"pnl"`
	PnlStrategy     float64 `json:"pnlStrategy"`
	UnattributedPnl float64 `json:"unattributedPnl"`
	// IncompletePnl 期间有盈亏未知实现的 episode 条数；> 0 时 Pnl 偏小。
	IncompletePnl int64 `json:"incompletePnl"`
	// TruncatedHead 建仓在数据窗口之前的 episode 条数。
	TruncatedHead int64 `json:"truncatedHead"`

	ByExitKind []ExitKindBucketDTO `json:"byExitKind"`
	Rebuild    EpisodeRebuildDTO   `json:"rebuild"`
}

// ─── 切片对比 ────────────────────────────────────────────────────────────────

// SliceCompareRowDTO 一个 (实例, 参数版本) 切片的横向对比行。
type SliceCompareRowDTO struct {
	InstanceKey   string `json:"instanceKey"`
	InstanceName  string `json:"instanceName"`
	Registered    bool   `json:"registered"`
	ConfigVersion uint64 `json:"configVersion"`

	// 信号侧（strategy_event）
	Signals   int64    `json:"signals"`
	Opened    int64    `json:"opened"`
	CapSkip   int64    `json:"capSkip"`
	GateBlock int64    `json:"gateBlock"`
	TrendSkip int64    `json:"trendSkip"`
	OpenRate  float64  `json:"openRate"`
	AvgGapBp  *float64 `json:"avgGapBp"`
	Weak      int64    `json:"weak"`
	Medium    int64    `json:"medium"`
	Strong    int64    `json:"strong"`
	FirstTs   string   `json:"firstTs"`
	LastTs    string   `json:"lastTs"`

	// 持仓侧（episode，按开仓时刻的 config_version 归属 = 决策时刻口径）
	Episodes      int64    `json:"episodes"`
	ClosedCount   int64    `json:"closedEpisodes"`
	Attributable  int64    `json:"attributableEpisodes"`
	Wins          int64    `json:"wins"`
	WinRate       *float64 `json:"winRate"`
	Pnl           float64  `json:"pnl"`
	PnlStrategy   float64  `json:"pnlStrategy"`
	AvgPeakPct    *float64 `json:"avgPeakPct"`
	AvgExitRoiPct *float64 `json:"avgExitRoiPct"`
	// EpisodeAvailable 该切片有没有派生出 episode。false 可能是"这段没有持仓"，
	// 也可能是"还没跑重建"，靠外层 Rebuild 区分。
	EpisodeAvailable bool `json:"episodeAvailable"`

	Variants []string `json:"variants"`
}

// SliceCompareDTO 按 config_version 与实例切片对比的返回体。
type SliceCompareDTO struct {
	Window        WindowDTO            `json:"window"`
	TimeField     string               `json:"timeField"`
	Rows          []SliceCompareRowDTO `json:"rows"`
	CrossInstance bool                 `json:"crossInstance"`
	// Notice 跨实例不可比的固定提示：三实例阈值 5/3bp、上限 15/26+8/246、
	// 下单 1/10 全不同，胜率与盈亏跨实例相加没有意义。
	Notice string `json:"notice"`
	// SignalTruncated 信号侧扫描命中行数上限。
	SignalTruncated bool              `json:"signalTruncated"`
	Rebuild         EpisodeRebuildDTO `json:"rebuild"`
}

// ─── 筛选项补充 ──────────────────────────────────────────────────────────────

// ExitKindOptionDTO 出场方式筛选项，带派生表里的真实条数。
type ExitKindOptionDTO struct {
	Value string `json:"value"` // 空串 = 仍持仓
	Label string `json:"label"`
	Count int64  `json:"count"`
	// CountedInWinRate 该出场方式是否计入策略胜率。
	CountedInWinRate bool `json:"countedInWinRate"`
}
