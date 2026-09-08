// Package dto 是 argus_event 读服务的出入参定义。
//
// 时间口径（全包统一，改之前先读这段）：入参与出参的时间一律是**本地墙钟串**
// `2006-01-02 15:04:05`，与 logs/ 下 JSONL、Telegram 消息、strategy_event.ts
// 逐字一致，不做任何时区换算。理由见 service.go 顶部的 parseEventTime 注释。
package dto

// ─── 查询入参 ────────────────────────────────────────────────────────────────

// SignalQueryDTO 信号列表的多维筛选条件。
//
// 复数维度统一用逗号分隔的字符串接收（如 accountLabels=A,B），而不是重复
// query 参数：管理端现有前端全部走 URLSearchParams 拼串，逗号形式在既有
// trade 接口里也已经在用（parseIDList）。
type SignalQueryDTO struct {
	PageIndex int `form:"pageIndex"`
	PageSize  int `form:"pageSize"`

	// InstanceKey 单实例；InstanceKeys 多实例（逗号分隔）。两者都空 = 全部实例，
	// 此时每行仍会带 instanceKey，前端不得把不同实例的账户合并成一个。
	InstanceKey  string `form:"instanceKey"`
	InstanceKeys string `form:"instanceKeys"`

	Start string `form:"start"` // 空 = 不限（列表靠分页收口）
	End   string `form:"end"`

	Instrument  string `form:"instrument"`  // 币种，归一化合约 BTCUSDT
	Instruments string `form:"instruments"` // 逗号分隔

	AccountLabel  string `form:"accountLabel"`
	AccountLabels string `form:"accountLabels"`
	Uid           string `form:"uid"`
	Uids          string `form:"uids"`

	// Category 事件大类：trigger（默认，四类触发事件）/ exit（六类出场）/ all。
	// Events 显式给出事件类型时覆盖 Category。
	Category string `form:"category"`
	Events   string `form:"events"`

	// Result 结果筛选：open / blocked / 具体 gate_kind（cap、reverse_gate_profit…）。
	Result string `form:"result"`

	// Strength 信号强度分级：weak / medium / strong；GapBpMin/Max 是按
	// |gap_bp| 的显式区间，给出时覆盖 Strength。
	Strength string   `form:"strength"`
	GapBpMin *float64 `form:"gapBpMin"`
	GapBpMax *float64 `form:"gapBpMax"`

	ConfigVersions string `form:"configVersions"` // 逗号分隔的版本号
	Variants       string `form:"variants"`
	Side           string `form:"side"`   // long / short
	Source         string `form:"source"` // 1=直写 2=回灌，逗号分隔

	Order string `form:"order"` // ts_desc（默认）/ ts_asc
}

// SliceQueryDTO 触发瞬间秒级切片的窗口参数。
type SliceQueryDTO struct {
	WindowSeconds int `form:"windowSeconds"`
}

// TimelineQueryDTO 与 K 线对齐的时间轴聚合入参。instanceKey 必填：净持仓阶梯
// 与开仓率跨实例相加没有意义（需求大纲 §4.3）。
type TimelineQueryDTO struct {
	InstanceKey  string `form:"instanceKey"`
	Instrument   string `form:"instrument"`
	Interval     string `form:"interval"`     // 1m/5m/15m/1h/4h/1d，默认 1m
	PlatformCode string `form:"platformCode"` // binance / deepcoin，默认 binance
	Start        string `form:"start"`
	End          string `form:"end"`
	AccountLabel string `form:"accountLabel"` // 空 = 该实例全部账户

	// ComparePlatformCode 双源对比时的第二个行情平台（r12 行情主视图的「双源对比」）。
	// 给出且与 PlatformCode 不同时才返回 CompareKlines。
	//
	// 之所以做进本接口而不是让前端再调一次 /klines/range：那个接口把 open_time 按
	// **UTC** 格式化，本接口按**本地墙钟**（与事件 ts 同口径），两者叠在同一条时间轴上
	// 会整体错开时区偏移。时间格式化口径必须只有一处，否则对比线永远是错位的。
	ComparePlatformCode string `form:"comparePlatformCode"`
}

// EquityQueryDTO 权益曲线入参。
type EquityQueryDTO struct {
	InstanceKey   string `form:"instanceKey"`
	AccountLabels string `form:"accountLabels"`
	Start         string `form:"start"`
	End           string `form:"end"`
	BucketSeconds int    `form:"bucketSeconds"` // 降采样粒度，默认 600
}

// GateStatsQueryDTO 拦截原因聚合入参。instanceKey 允许为空（全部实例），
// 但返回体会带 crossInstance 标记与不可比提示。
type GateStatsQueryDTO struct {
	InstanceKey  string `form:"instanceKey"`
	InstanceKeys string `form:"instanceKeys"`
	Instrument   string `form:"instrument"`
	AccountLabel string `form:"accountLabel"`
	Start        string `form:"start"`
	End          string `form:"end"`
}

// InstanceSummaryQueryDTO 跨实例汇总对比入参。
type InstanceSummaryQueryDTO struct {
	Instrument string `form:"instrument"`
	Start      string `form:"start"`
	End        string `form:"end"`
}

// ─── 通用出参片段 ────────────────────────────────────────────────────────────

// WindowDTO 一次查询实际生效的时间窗口。入参没给时由服务按**已入库数据的最新
// 时刻**回推（不是服务器当前时间），并通过 Resolved 说明是怎么来的。
type WindowDTO struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Resolved string `json:"resolved"` // explicit / latest-data / empty / signal-slice（以切片锚点为原点）
}

// OptionDTO 枚举项，供前端下拉框直接渲染。
type OptionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ─── 信号列表 ────────────────────────────────────────────────────────────────

// SignalEventDTO 一条事件行 = 一个账户在一次触发上的判定结果。
// 列表刻意**不按分钟聚合**：实测 28.7% 的信号分钟内触发 ≥2 次、单分钟最多 5 次，
// 聚合会直接丢掉一半触发（需求大纲 §3.3）。
type SignalEventDTO struct {
	EventID       uint64 `json:"eventId"`
	Ts            string `json:"ts"` // 秒精度本地墙钟
	InstanceKey   string `json:"instanceKey"`
	ConfigVersion uint64 `json:"configVersion"`
	Uid           string `json:"uid"`
	AccountLabel  string `json:"accountLabel"`
	Variant       string `json:"variant"`

	Event      string `json:"event"`
	EventLabel string `json:"eventLabel"`
	Instrument string `json:"instrument"`
	InstIdRaw  string `json:"instIdRaw"`

	// ResultKind：open（成交）/ blocked（被条件挡住）/ exit（出场）/ alert。
	ResultKind string `json:"resultKind"`

	Side    string `json:"side"`
	NetSide string `json:"netSide"`
	// Direction 信号方向 UP/DOWN，由 gap_bp 符号推出；无报价快照时为空。
	Direction string `json:"direction"`

	Size      *int `json:"size"`
	OrderSize *int `json:"orderSize"`

	AvgPx       *float64 `json:"avgPx"`
	LastPx      *float64 `json:"lastPx"`
	RoiPct      *float64 `json:"roiPct"`
	Pnl         *float64 `json:"pnl"`
	PeakPct     *float64 `json:"peakPct"`
	SigLast     *float64 `json:"sigLast"`
	SigMark     *float64 `json:"sigMark"`
	GapBp       *float64 `json:"gapBp"`
	TrendMomPct *float64 `json:"trendMomPct"`

	StrengthLevel string   `json:"strengthLevel"` // weak/medium/strong；无 gap_bp 时为空
	GateKind      string   `json:"gateKind"`
	GateLabel     string   `json:"gateLabel"`
	GateThreshold *float64 `json:"gateThreshold"`
	GateActual    *float64 `json:"gateActual"`
	Reason        string   `json:"reason"`

	Source      int8   `json:"source"` // 1=直写 2=回灌
	SourceLabel string `json:"sourceLabel"`
}

// ─── 信号详情 ────────────────────────────────────────────────────────────────

// AccountDecisionDTO 一次触发里某个账户的判定结果，对应 TG 消息的 [n] / [跳过n] 行。
type AccountDecisionDTO struct {
	EventID       uint64   `json:"eventId"`
	Ts            string   `json:"ts"`
	TsOffsetSec   int      `json:"tsOffsetSec"` // 相对锚点事件的秒偏移（下单往返会晚几秒）
	AccountLabel  string   `json:"accountLabel"`
	Uid           string   `json:"uid"`
	Variant       string   `json:"variant"`
	Result        string   `json:"result"` // 事件类型原值
	ResultLabel   string   `json:"resultLabel"`
	ResultKind    string   `json:"resultKind"`
	Side          string   `json:"side"`
	NetSide       string   `json:"netSide"`
	OrderSize     *int     `json:"orderSize"`
	NetSize       *int     `json:"netSize"`
	AvgPx         *float64 `json:"avgPx"`
	LastPx        *float64 `json:"lastPx"`
	RoiPct        *float64 `json:"roiPct"`
	Pnl           *float64 `json:"pnl"`
	GateKind      string   `json:"gateKind"`
	GateLabel     string   `json:"gateLabel"`
	GateThreshold *float64 `json:"gateThreshold"`
	GateActual    *float64 `json:"gateActual"`
	Reason        string   `json:"reason"`
	ConfigVersion uint64   `json:"configVersion"`
}

// SignalDetailDTO 一次触发的完整判定，取代「导出 TG 历史消息交给 AI 读」。
type SignalDetailDTO struct {
	SignalID    uint64 `json:"signalId"` // 锚点事件 id，稳定可回链
	Ts          string `json:"ts"`
	InstanceKey string `json:"instanceKey"`
	Instrument  string `json:"instrument"`
	InstIdRaw   string `json:"instIdRaw"`

	Direction     string   `json:"direction"`
	Side          string   `json:"side"`
	SigLast       *float64 `json:"sigLast"`
	SigMark       *float64 `json:"sigMark"`
	GapBp         *float64 `json:"gapBp"`
	StrengthLevel string   `json:"strengthLevel"`
	TrendMomPct   *float64 `json:"trendMomPct"`

	ConfigVersions []uint64 `json:"configVersions"`
	Variants       []string `json:"variants"`

	AccountCount  int `json:"accountCount"`
	OpenedCount   int `json:"openedCount"`
	BlockedCount  int `json:"blockedCount"`
	TotalOrderQty int `json:"totalOrderQty"`

	Accounts []AccountDecisionDTO `json:"accounts"`

	// TelegramLines 按 TG 消息原格式还原的 [n] / [跳过n] 明细行。
	TelegramLines []string `json:"telegramLines"`
	// FillPriceAvailable 成交价/委托均价是否可还原。开仓事件的这两项来自交易所
	// 下单回执，埋点未落库（见设计过程文档「已知缺口」），因此恒为 false，
	// 前端必须显示「—」而不是 0。
	FillPriceAvailable bool   `json:"fillPriceAvailable"`
	Notice             string `json:"notice"`

	// GroupWindowSec 同一次触发的判定聚合窗口（秒）。
	GroupWindowSec int `json:"groupWindowSec"`
}

// ─── 秒级切片 ────────────────────────────────────────────────────────────────

// SlicePointDTO 切片窗口内的一个秒级观测点。
//
// 三个价位一律"该秒没有 tick 就是 null"，绝不插值或补零：切片的用途就是看清
// 触发瞬间到底发生了什么，补出来的点会把断流画成一条平滑的假线。
type SlicePointDTO struct {
	Ts        string   `json:"ts"`
	OffsetSec int      `json:"offsetSec"`
	DcLast    *float64 `json:"dcLast"`
	DcMark    *float64 `json:"dcMark"`
	// BinLast 币安 last（r3 的秒级切片才有；降级来源里不存在币安报价）。
	BinLast *float64 `json:"binLast"`
	// GapBp = (dcLast-dcMark)/dcMark*10000，带符号（>0=UP）。逐秒切片下它由
	// 该秒的两个价位现算，与 strategy_event.gap_bp 同一公式。
	GapBp       *float64 `json:"gapBp"`
	Events      []string `json:"events"`
	IsTriggerTs bool     `json:"isTriggerTs"`
}

// SliceKlineDTO 切片窗口覆盖到的 1m K 线（上下文参照）。
type SliceKlineDTO struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// SliceDevSampleDTO 切片窗口覆盖到的无条件偏离采样窗口。
type SliceDevSampleDTO struct {
	Ts        string   `json:"ts"`
	DevTicks  int      `json:"devTicks"`
	DevMaxBp  *float64 `json:"devMaxBp"`
	DevMeanBp *float64 `json:"devMeanBp"`
}

// SignalSliceDTO 触发瞬间 ±windowSeconds 的秒级切片。
//
// TickSource 说明数据来源：
//   - signal_slice：r3 采集的逐秒切片，121 点逐秒完整（含币安 last）；
//   - strategy_event：降级来源，只有"有触发发生的那一秒"才有点位（切片过了
//     90 天保留期、或采集器还没上线时走这条），窗口内其余秒没有数据，
//     前端不得把这些稀疏点连成连续曲线。
//
// 时间锚有两个，不要混：Ts 是被点开的那条事件的时刻；SliceAnchorTs 是切片自身
// 的锚点，即**偏离穿越那一秒**。两者差 AnchorLagSec（信号延迟调度 + 下单往返），
// 走 signal_slice 时窗口与 OffsetSec 以 SliceAnchorTs 为原点——切片本来就是围绕
// 穿越瞬间采的，按事件 ts 重新对齐会让右半窗凭空缺掉 AnchorLagSec 秒。
type SignalSliceDTO struct {
	SignalID      uint64    `json:"signalId"`
	Ts            string    `json:"ts"`
	InstanceKey   string    `json:"instanceKey"`
	Instrument    string    `json:"instrument"`
	WindowSeconds int       `json:"windowSeconds"` // 实际生效的半窗（可能小于请求值，见 DegradedReason）
	Window        WindowDTO `json:"window"`

	TickSource     string `json:"tickSource"`     // signal_slice / strategy_event（降级）
	TickComplete   bool   `json:"tickComplete"`   // points 在 window 内是否逐秒完整
	DegradedReason string `json:"degradedReason"` // 供数受限的说明（降级来源 / 窗口被收窄），可与 TickComplete=true 并存

	// SliceAnchorTs 切片锚点（偏离穿越时刻）；无逐秒切片时为空。
	SliceAnchorTs string `json:"sliceAnchorTs"`
	// AnchorLagSec 事件 ts − 切片锚点，单位秒；无逐秒切片时为 0。
	AnchorLagSec int `json:"anchorLagSec"`
	// DcPoints / BinPoints 两条来源各自的非空秒数，用来判断某一侧是否断流。
	DcPoints  int `json:"dcPoints"`
	BinPoints int `json:"binPoints"`

	Points      []SlicePointDTO     `json:"points"`
	DevSamples  []SliceDevSampleDTO `json:"devSamples"`
	Klines      []SliceKlineDTO     `json:"klines"`
	KlineSource string              `json:"klineSource"` // platformCode/symbol/interval
}

// ─── 时间轴聚合 ──────────────────────────────────────────────────────────────

// TimelineBucketDTO 一根 K 线对应的触发点聚合。
type TimelineBucketDTO struct {
	Time        string   `json:"time"` // 与 K 线 openTime 同一网格
	Total       int      `json:"total"`
	Open        int      `json:"open"`
	CapSkip     int      `json:"capSkip"`
	GateBlock   int      `json:"gateBlock"`
	TrendSkip   int      `json:"trendSkip"`
	Exit        int      `json:"exit"`
	MaxAbsGapBp *float64 `json:"maxAbsGapBp"`
	// NetSizeEnd 桶末该实例（或指定账户）的净持仓张数，来自各账户最近一次
	// 带净仓快照的事件；桶内无事件时沿用上一桶的值，形成阶梯。
	NetSizeEnd    int      `json:"netSizeEnd"`
	RealizedPnl   *float64 `json:"realizedPnl"`
	ConfigVersion uint64   `json:"configVersion"`
}

// TimelineCoverageDTO K 线覆盖率，用于前端的「缺口回填」空状态。
type TimelineCoverageDTO struct {
	Expected    int     `json:"expected"`
	Actual      int     `json:"actual"`
	CoveragePct float64 `json:"coveragePct"`
	Missing     int     `json:"missing"`
}

// TimelineDTO 与 K 线对齐的时间轴。
type TimelineDTO struct {
	InstanceKey  string    `json:"instanceKey"`
	Instrument   string    `json:"instrument"`
	Symbol       string    `json:"symbol"`
	Interval     string    `json:"interval"`
	PlatformCode string    `json:"platformCode"`
	Window       WindowDTO `json:"window"`

	Klines   []SliceKlineDTO     `json:"klines"`
	Buckets  []TimelineBucketDTO `json:"buckets"`
	Coverage TimelineCoverageDTO `json:"coverage"`

	// ComparePlatformCode / CompareKlines / CompareCoverage 只在入参给了
	// comparePlatformCode 时有值，供双源对比叠加第二条收盘线。
	ComparePlatformCode string              `json:"comparePlatformCode"`
	CompareKlines       []SliceKlineDTO     `json:"compareKlines"`
	CompareCoverage     TimelineCoverageDTO `json:"compareCoverage"`

	EventTotal int  `json:"eventTotal"`
	Truncated  bool `json:"truncated"` // 命中行数上限，聚合不完整
}

// ─── 权益曲线 ────────────────────────────────────────────────────────────────

// EquityPointDTO 一个降采样桶内的权益观测。
type EquityPointDTO struct {
	Time      string   `json:"time"`
	Balance   *float64 `json:"balance"`
	Equity    *float64 `json:"equity"`
	Upl       *float64 `json:"upl"`
	MinEquity *float64 `json:"minEquity"`
	MaxEquity *float64 `json:"maxEquity"`
	Samples   int      `json:"samples"`
	// ChangePct 相对本序列首个已知权益的变动百分比，125x 下两个账户才能同屏可比。
	ChangePct *float64 `json:"changePct"`
}

// EquitySeriesDTO 一个账户的权益序列。账户唯一性是 (instanceKey, accountLabel)，
// 只按账户名归集会串实例。
type EquitySeriesDTO struct {
	InstanceKey  string           `json:"instanceKey"`
	AccountLabel string           `json:"accountLabel"`
	Uid          string           `json:"uid"`
	Variant      string           `json:"variant"`
	FirstEquity  *float64         `json:"firstEquity"`
	LastEquity   *float64         `json:"lastEquity"`
	ChangePct    *float64         `json:"changePct"`
	Points       []EquityPointDTO `json:"points"`
}

// EquityCurveDTO 权益曲线返回体。
type EquityCurveDTO struct {
	InstanceKey   string            `json:"instanceKey"`
	Window        WindowDTO         `json:"window"`
	BucketSeconds int               `json:"bucketSeconds"`
	Series        []EquitySeriesDTO `json:"series"`
	Notice        string            `json:"notice"`
}

// ─── 拦截原因聚合 ────────────────────────────────────────────────────────────

// ResultBucketDTO 按结果（事件类型）分组的计数。
type ResultBucketDTO struct {
	Event string  `json:"event"`
	Label string  `json:"label"`
	Count int64   `json:"count"`
	Share float64 `json:"share"`
}

// GateBucketDTO 按门控种类分组的拦截统计。
type GateBucketDTO struct {
	GateKind     string   `json:"gateKind"`
	Label        string   `json:"label"`
	Count        int64    `json:"count"`
	Share        float64  `json:"share"`
	AvgThreshold *float64 `json:"avgThreshold"`
	AvgActual    *float64 `json:"avgActual"`
	LastTs       string   `json:"lastTs"`
	SampleReason string   `json:"sampleReason"`
}

// StrengthBucketDTO 按 |gap_bp| 分级的信号强度统计。
type StrengthBucketDTO struct {
	Level    string   `json:"level"`
	Label    string   `json:"label"`
	MinAbsBp *float64 `json:"minAbsBp"`
	MaxAbsBp *float64 `json:"maxAbsBp"`
	Count    int64    `json:"count"`
	Opened   int64    `json:"opened"`
	OpenRate float64  `json:"openRate"`
}

// GateStatsDTO 拦截原因聚合返回体。
type GateStatsDTO struct {
	Window        WindowDTO           `json:"window"`
	InstanceKeys  []string            `json:"instanceKeys"`
	CrossInstance bool                `json:"crossInstance"`
	Notice        string              `json:"notice"`
	TotalTriggers int64               `json:"totalTriggers"`
	Opened        int64               `json:"opened"`
	Blocked       int64               `json:"blocked"`
	OpenRate      float64             `json:"openRate"`
	ByResult      []ResultBucketDTO   `json:"byResult"`
	ByGate        []GateBucketDTO     `json:"byGate"`
	ByStrength    []StrengthBucketDTO `json:"byStrength"`
	// Truncated 命中单次扫描行数上限，聚合只覆盖窗口内的一部分事件。
	// 不静默截断：真发生时必须让页面显示"请缩小时间窗口"。
	Truncated bool `json:"truncated"`
}

// ─── 跨实例汇总对比 ──────────────────────────────────────────────────────────

// InstanceAccountDTO 实例下的一个账户及其最近权益。
type InstanceAccountDTO struct {
	AccountLabel string   `json:"accountLabel"`
	Uid          string   `json:"uid"`
	Variant      string   `json:"variant"`
	LastEquity   *float64 `json:"lastEquity"`
	LastBalance  *float64 `json:"lastBalance"`
	LastNetSize  *int     `json:"lastNetSize"`
	LastSampleTs string   `json:"lastSampleTs"`
}

// InstanceSummaryDTO 单个实例在窗口内的汇总。
type InstanceSummaryDTO struct {
	InstanceKey  string `json:"instanceKey"`
	InstanceName string `json:"instanceName"`
	Enabled      uint8  `json:"enabled"`
	Registered   bool   `json:"registered"` // 事件里出现过但 argus_instance 未注册时为 false

	Signals   int64 `json:"signals"`
	Opened    int64 `json:"opened"`
	CapSkip   int64 `json:"capSkip"`
	GateBlock int64 `json:"gateBlock"`
	TrendSkip int64 `json:"trendSkip"`
	Exits     int64 `json:"exits"`

	OpenRate      float64  `json:"openRate"`
	RealizedPnl   *float64 `json:"realizedPnl"`
	Variants      []string `json:"variants"`
	ConfigVersion uint64   `json:"configVersion"` // 窗口内出现的最大版本号
	FirstTs       string   `json:"firstTs"`
	LastTs        string   `json:"lastTs"`

	Accounts     []InstanceAccountDTO `json:"accounts"`
	EquityTotal  *float64             `json:"equityTotal"`
	NetSizeTotal *int                 `json:"netSizeTotal"`
}

// InstanceSummaryResultDTO 跨实例汇总对比返回体。
type InstanceSummaryResultDTO struct {
	Window    WindowDTO            `json:"window"`
	Instances []InstanceSummaryDTO `json:"instances"`
	// Notice 固定提示：三实例参数不同（阈值 5/3bp、上限 15/26+8/246、下单 1/10），
	// 胜率与盈亏跨实例相加没有意义。
	Notice string `json:"notice"`
	// Truncated 同 GateStatsDTO.Truncated。
	Truncated bool `json:"truncated"`
}

// ─── 筛选项 ──────────────────────────────────────────────────────────────────

// InstanceOptionDTO 有事件数据的实例及其覆盖区间。
type InstanceOptionDTO struct {
	InstanceKey  string `json:"instanceKey"`
	InstanceName string `json:"instanceName"`
	Enabled      uint8  `json:"enabled"`
	Registered   bool   `json:"registered"`
	EventCount   int64  `json:"eventCount"`
	FirstTs      string `json:"firstTs"`
	LastTs       string `json:"lastTs"`
}

// AccountOptionDTO 账户维度的筛选项，带实例键——账户名会跨实例重号。
type AccountOptionDTO struct {
	InstanceKey  string `json:"instanceKey"`
	AccountLabel string `json:"accountLabel"`
	Uid          string `json:"uid"`
	Variant      string `json:"variant"`
	Instrument   string `json:"instrument"`
	EventCount   int64  `json:"eventCount"`
	FirstTs      string `json:"firstTs"`
	LastTs       string `json:"lastTs"`
}

// ConfigVersionOptionDTO 参数版本筛选项。
type ConfigVersionOptionDTO struct {
	InstanceKey   string `json:"instanceKey"`
	ConfigVersion uint64 `json:"configVersion"`
	EventCount    int64  `json:"eventCount"`
	FirstTs       string `json:"firstTs"`
	LastTs        string `json:"lastTs"`
}

// FilterOptionsDTO 四个页面共用的一次性筛选项拉取。
type FilterOptionsDTO struct {
	Instances      []InstanceOptionDTO      `json:"instances"`
	Accounts       []AccountOptionDTO       `json:"accounts"`
	Instruments    []string                 `json:"instruments"`
	Variants       []string                 `json:"variants"`
	ConfigVersions []ConfigVersionOptionDTO `json:"configVersions"`
	EventKinds     []OptionDTO              `json:"eventKinds"`
	GateKinds      []OptionDTO              `json:"gateKinds"`
	Strengths      []StrengthOptionDTO      `json:"strengths"`
	Sources        []OptionDTO              `json:"sources"`
	DataRange      WindowDTO                `json:"dataRange"`
}

// StrengthOptionDTO 信号强度分级的档位定义，前端不要再自己写死区间。
type StrengthOptionDTO struct {
	Value    string   `json:"value"`
	Label    string   `json:"label"`
	MinAbsBp *float64 `json:"minAbsBp"`
	MaxAbsBp *float64 `json:"maxAbsBp"`
}
