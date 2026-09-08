// Package episode 把 strategy_event 里的离散事件归集成一次完整持仓（episode），
// 并把每笔已实现盈亏平摊回各张仓位**建仓时刻**的决策上。
//
// 三条硬不变量（改代码前先读）：
//  1. 派生是纯函数：输入只有 strategy_event / balance_sample 两张事实表，
//     输出只有本包这两张派生表。episode 可随时整表 DROP 并重建，重建结果
//     必须逐字节一致——所以派生过程不得读时钟、不得依赖上一次的派生结果。
//  2. 归集口径是**决策时刻**，不是平仓时刻。设计文档
//     `docs/argus_single/2026-07-27-波动率状态自适应研究设计.md` §10.3 记录了
//     反例：按平仓时刻归集得出"高波动期收益是低波动期 8–11 倍、16/16 路径同号"
//     的极强信号，实为伪影（高波动期本来就更容易触发移动止盈，测的是"平仓更
//     容易发生在哪"）；改决策归集后差异塌缩到 0.001–0.065。任何按状态/条件
//     分层的收益归因都必须走 episode_entry，不能拿 episode.closed_at 分桶。
//  3. external_close 与 manual_close 是交易所侧或人工操作，**不计入策略胜率**
//     （strategy_attributable=0），但它们的盈亏是真金白银，仍如实记进 pnl；
//     只把"策略自己决定的那部分"单独存进 pnl_strategy 供分母口径选择。
package episode

import "time"

// 六种出场方式（设计文档 §6.2）。前五种由对应的平仓事件直接给出，
// reduce_to_zero 没有任何平仓事件——反向减仓一张一张磨到 0，实测占
// 账户A 13% / 账户B 19%。前端若只认平仓事件，这些 episode 会显示成
// "永远没结束"。
const (
	ExitTrailingClose   = "trailing_close"
	ExitCatastropheStop = "catastrophe_stop"
	ExitFixedClose      = "fixed_close"
	ExitExternalClose   = "external_close"
	ExitManualClose     = "manual_close"
	ExitReduceToZero    = "reduce_to_zero"
)

// 扛单深度的保真度两档 + 混合档（设计文档 §6.4）。
//
// 深度的数据源有两个：balance_sample.upl 的分钟心跳（2026-07-21 起才有），
// 以及 loss_alert——后者是双重过滤的采样（ROI < −150% 才发 + 5 分钟告警冷却），
// 只能给出真实最深值的**下界**。两段拼在一起而不标注，会给人虚假的精确感。
const (
	DepthMinute       = "minute"        // 全程有分钟级 upl，可画真正细的水下曲线
	DepthAlertSampled = "alert_sampled" // 只有 loss_alert：0 ~ −150% 区间完全空白
	DepthMixed        = "mixed"         // 跨越 upl 上线时点，或心跳有缺口
)

// Episode 一次完整持仓（派生物化，可从 strategy_event + balance_sample 纯函数重建）。
//
// 归属键是 (instance_key, account_label)：uid 从来没进过事件、只能靠 properties
// 映射补齐，配置没填时为空；account_label 则每条业务事件都有。两个实例里都存在
// 叫"账户A-…"的标签但指向不同真实账户，所以 instance_key 必须进键。
type Episode struct {
	Id           uint64 `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	InstanceKey  string `gorm:"column:instance_key;type:varchar(64);not null;uniqueIndex:uk_episode_anchor,priority:1;index:idx_episode_inst_opened,priority:1" description:"实例键，对应 argus_instance.instance_key"`
	UID          string `gorm:"column:uid;type:varchar(32);not null;default:''" description:"数字 uid；配置未填时为空"`
	AccountLabel string `gorm:"column:account_label;type:varchar(128);not null;default:'';uniqueIndex:uk_episode_anchor,priority:2" description:"JSONL 原 account 串，账户归属的唯一可靠依据"`
	Instrument   string `gorm:"column:instrument;type:varchar(24);not null;default:'';uniqueIndex:uk_episode_anchor,priority:3" description:"归一化合约（BTCUSDT）"`
	Side         string `gorm:"column:side;type:varchar(8);not null;default:''" description:"该 episode 的持仓方向"`

	// FirstEventAt 是唯一键的锚点而不是 opened_at：开头被数据窗口截断时
	// opened_at 为 NULL，而 MySQL 唯一键里的 NULL 互不相等，等于没去重。
	FirstEventAt time.Time  `gorm:"column:first_event_at;type:datetime;not null;uniqueIndex:uk_episode_anchor,priority:4" description:"本 episode 观测到的首条事件时刻"`
	OpenedAt     *time.Time `gorm:"column:opened_at;type:datetime;index:idx_episode_inst_opened,priority:2" description:"建仓时刻；NULL=开头被数据窗口截断"`
	LastEventAt  time.Time  `gorm:"column:last_event_at;type:datetime;not null" description:"本 episode 观测到的末条事件时刻；仍持仓时即窗口右端"`
	ClosedAt     *time.Time `gorm:"column:closed_at;type:datetime;index:idx_episode_closed" description:"出场时刻；NULL=仍持仓中"`
	DurationSec  *int       `gorm:"column:duration_sec;type:int;size:32" description:"持仓时长秒；opened_at/closed_at 任一为空则为 NULL"`

	ExitKind      *string `gorm:"column:exit_kind;type:varchar(24);index:idx_episode_exit" description:"六种出场方式之一；NULL=仍持仓"`
	ExitEventHash []byte  `gorm:"column:exit_event_hash;type:binary(16)" description:"出场那条 strategy_event 的 event_hash，可回溯原始事件"`
	// StrategyAttributable external_close / manual_close 是交易所侧或人工操作，
	// 不计入策略胜率；仍持仓的 episode 也不计入（结果未定）。
	StrategyAttributable uint8 `gorm:"column:strategy_attributable;type:tinyint unsigned;not null;default:0" description:"是否计入策略胜率"`

	AddCount       int     `gorm:"column:add_count;type:int;size:32;not null;default:0" description:"加仓（含首仓）决策次数"`
	ReduceCount    int     `gorm:"column:reduce_count;type:int;size:32;not null;default:0" description:"反向减仓次数"`
	EntrySizeTotal int     `gorm:"column:entry_size_total;type:int;size:32;not null;default:0" description:"本 episode 累计建仓张数，决策归集的分母"`
	MaxSize        int     `gorm:"column:max_size;type:int;size:32;not null;default:0" description:"峰值净仓张数"`
	OpenSize       float64 `gorm:"column:open_size;type:decimal(20,8);not null;default:0" description:"重建时点仍在场的张数；已出场则为 0"`
	// HiddenSize 在场但建仓决策不在数据窗口内的张数（截断头）。它进归集分母，
	// 但没有对应的 episode_entry 行，所以 sum(entry.open_size) 会比 open_size 小这么多。
	HiddenSize float64 `gorm:"column:hidden_size;type:decimal(20,8);not null;default:0" description:"在场但建仓决策不可见的张数"`

	MinRoiPctObserved *float64 `gorm:"column:min_roi_pct_observed;type:decimal(14,6)" description:"观测最深 ROI%，是真实值的下界（loss_alert 5 分钟采样）"`
	MaxRoiPctObserved *float64 `gorm:"column:max_roi_pct_observed;type:decimal(14,6)" description:"观测最高 ROI%，同样只是观测值不是真实极值"`
	PeakPct           *float64 `gorm:"column:peak_pct;type:decimal(14,6)" description:"平仓事件带的 trail 峰值（trail 激活过才有）"`
	ExitRoiPct        *float64 `gorm:"column:exit_roi_pct;type:decimal(14,6)" description:"出场那一刻的 ROI%"`

	Pnl         float64 `gorm:"column:pnl;type:decimal(20,8);not null;default:0" description:"本 episode 已知的已实现盈亏合计（含减仓锁利）"`
	PnlStrategy float64 `gorm:"column:pnl_strategy;type:decimal(20,8);not null;default:0" description:"其中由策略自身决定出场的部分（剔除 external/manual）"`
	// RealizedEvents / MissingPnlEvents 一起构成盈亏的已知性标记：
	// 2026-07-24 之前的反向减仓只写 size 不写 pnl，那部分实现盈亏是**未知**
	// 而不是 0，把它当 0 会凭空造出一个"这次减仓不赚不亏"的假样本。
	// UnattributedPnl 本 episode 已实现盈亏里落在"建仓决策不可见"那部分张数上的
	// 金额。它计入 pnl，但不会出现在任何 episode_entry 里——按决策分层统计时，
	// 这部分只能整体剔除或单列，不能硬摊给看得见的那几笔。
	UnattributedPnl  float64 `gorm:"column:unattributed_pnl;type:decimal(20,8);not null;default:0" description:"落不到任何建仓决策上的已实现盈亏"`
	RealizedEvents   int     `gorm:"column:realized_events;type:int;size:32;not null;default:0" description:"带已知盈亏的实现次数"`
	MissingPnlEvents int     `gorm:"column:missing_pnl_events;type:int;size:32;not null;default:0" description:"实现了但盈亏未知的次数；>0 时 pnl 不完整"`

	CapSkipCount       int `gorm:"column:cap_skip_count;type:int;size:32;not null;default:0" description:"持仓期内被仓位上限挡掉的信号数"`
	GateBlockCount     int `gorm:"column:gate_block_count;type:int;size:32;not null;default:0" description:"持仓期内被反向门控挡掉的信号数"`
	TrendSkipCount     int `gorm:"column:trend_skip_count;type:int;size:32;not null;default:0" description:"持仓期内被趋势闸挡掉的信号数"`
	LossAlertCount     int `gorm:"column:loss_alert_count;type:int;size:32;not null;default:0" description:"持仓期内的深度告警条数"`
	BalanceSampleCount int `gorm:"column:balance_sample_count;type:int;size:32;not null;default:0" description:"窗口内的余额心跳总条数"`
	UplSampleCount     int `gorm:"column:upl_sample_count;type:int;size:32;not null;default:0" description:"其中带 upl 的条数；两者之比决定 depth_fidelity"`
	PositionGapCount   int `gorm:"column:position_gap_count;type:int;size:32;not null;default:0" description:"|Δsize| != orderSize 的次数"`
	// HasPositionGap 仓位跳变只记标记、不断开 episode（设计文档 §6.5 / D6）：
	// 实测 8/1016 = 0.8%，成因各不相同（未记录的外部平仓、信号突发累加一次下单
	// 多张、两账户同秒导致的持仓快照滞后）。断开会造出假 episode，比标记更糟。
	HasPositionGap uint8 `gorm:"column:has_position_gap;type:tinyint unsigned;not null;default:0" description:"仓位跳变标记"`

	// Variant / ConfigVersion 取**开仓时刻**（设计文档 §6.3）。variant 历史上变过
	// 5 次，跨越变更点的 episode 必然有歧义，明确选决策时刻。
	Variant       *string `gorm:"column:variant;type:varchar(64);index:idx_episode_variant" description:"开仓时刻的 variant（决策时刻口径）"`
	ConfigVersion uint64  `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"开仓时刻生效的配置版本号"`

	DepthFidelity string    `gorm:"column:depth_fidelity;type:varchar(16);not null;default:''" description:"minute / alert_sampled / mixed"`
	RebuiltAt     time.Time `gorm:"column:rebuilt_at;type:datetime(3);not null" description:"本行的重建时刻"`

	// Entries 不入库，只在派生过程里把两张表的行串起来。
	Entries []*EpisodeEntry `gorm:"-"`
}

func (Episode) TableName() string { return "episode" }

// EpisodeEntry 一次建仓决策，以及归给这次决策的已实现盈亏（决策归集口径的载体）。
//
// 为什么不做成"一张合约一行"：实例2 的 cap 是 246 张、order_size 是 10，
// 按张展开会把行数放大一到两个数量级，而决策的粒度本来就是"这一笔 open 事件"。
// 一笔 open 事件加了 n 张，这 n 张同生共死、状态标签也完全相同。
//
// 为什么 closed_size / open_size 是小数：合约在交易所是按均价净额记账的，
// 一次减仓 k 张实现的盈亏，经济上就是整个仓位浮盈的 k/N 切片，与"具体减掉
// 的是哪几张"无关。因此退场按各笔决策的在场张数**等比例**摊，而不是 FIFO——
// FIFO 会系统性地让早期决策先被减仓退场、拿不到后续平仓的归因，凭空造出
// "越早建仓越不赚钱"的假结论。等比例摊在整仓平掉时退化成"每张平分"，
// 与 §10.3 复现脚本 vol_state_phase0.py 的 `share = pnl / len(open_adds)` 完全一致。
type EpisodeEntry struct {
	Id           uint64 `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	EpisodeId    uint64 `gorm:"column:episode_id;type:bigint unsigned;not null;index:idx_episode_entry_ep" description:"所属 episode"`
	InstanceKey  string `gorm:"column:instance_key;type:varchar(64);not null;index:idx_episode_entry_inst_decided,priority:1" description:"实例键"`
	UID          string `gorm:"column:uid;type:varchar(32);not null;default:''" description:"数字 uid"`
	AccountLabel string `gorm:"column:account_label;type:varchar(128);not null;default:''" description:"JSONL 原 account 串"`
	Instrument   string `gorm:"column:instrument;type:varchar(24);not null;default:''" description:"归一化合约"`
	Side         string `gorm:"column:side;type:varchar(8);not null;default:''" description:"建仓方向（= episode 方向）"`

	// DecidedAt 就是"决策时刻"：所有按状态/条件分层的收益归因都按它分桶。
	DecidedAt time.Time `gorm:"column:decided_at;type:datetime;not null;index:idx_episode_entry_inst_decided,priority:2" description:"建仓决策时刻"`
	EventHash []byte    `gorm:"column:event_hash;type:binary(16);not null;uniqueIndex:uk_episode_entry_event" description:"对应那条 open 事件的 event_hash，一笔决策一行"`

	AddedSize  int     `gorm:"column:added_size;type:int;size:32;not null;default:0" description:"本次决策加的张数（按仓位快照实算，可能 != orderSize）"`
	OrderSize  *int    `gorm:"column:order_size;type:int;size:32" description:"事件里写的下单张数，与 added_size 不等即仓位跳变"`
	ClosedSize float64 `gorm:"column:closed_size;type:decimal(20,8);not null;default:0" description:"已退场张数（等比例摊，故为小数）"`
	OpenSize   float64 `gorm:"column:open_size;type:decimal(20,8);not null;default:0" description:"重建时点仍在场张数"`

	AttributedPnl         float64 `gorm:"column:attributed_pnl;type:decimal(20,8);not null;default:0" description:"归给本次决策的已实现盈亏"`
	AttributedPnlStrategy float64 `gorm:"column:attributed_pnl_strategy;type:decimal(20,8);not null;default:0" description:"其中剔除 external/manual 出场后的部分"`
	// MissingPnlEvents 本次决策在场期间发生过、但盈亏未知的实现次数。
	// >0 表示 attributed_pnl 偏小且偏差方向未知，分层统计要么剔除要么标注。
	MissingPnlEvents int   `gorm:"column:missing_pnl_events;type:int;size:32;not null;default:0" description:"在场期间盈亏未知的实现次数"`
	PnlKnown         uint8 `gorm:"column:pnl_known;type:tinyint unsigned;not null;default:0" description:"已完全出场且期间没有未知盈亏；0=归因不完整"`

	// 决策时刻的市场与配置上下文，直接从 open 事件带过来，省掉下游回表。
	Variant       *string  `gorm:"column:variant;type:varchar(64);index:idx_episode_entry_variant" description:"决策时刻的 variant"`
	ConfigVersion uint64   `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"决策时刻生效的配置版本号"`
	GapBp         *float64 `gorm:"column:gap_bp;type:decimal(12,4)" description:"触发本次建仓的偏离 bp（带符号，>0=UP）"`
	AvgPx         *float64 `gorm:"column:avg_px;type:decimal(20,8)" description:"决策时刻的持仓均价"`
	LastPx        *float64 `gorm:"column:last_px;type:decimal(20,8)" description:"决策时刻的最新价"`

	// 出场口径冗余一份到决策行上：分层统计的过滤条件（"只看策略自己平掉的"）
	// 直接落在本表索引上，不必每次回 join episode。
	ExitKind             *string   `gorm:"column:exit_kind;type:varchar(24)" description:"所属 episode 的出场方式；NULL=仍持仓"`
	StrategyAttributable uint8     `gorm:"column:strategy_attributable;type:tinyint unsigned;not null;default:0" description:"所属 episode 是否计入策略胜率"`
	RebuiltAt            time.Time `gorm:"column:rebuilt_at;type:datetime(3);not null" description:"本行的重建时刻"`
}

func (EpisodeEntry) TableName() string { return "episode_entry" }

// Models 两张派生表，供 AutoMigrate 与读侧共用。
func Models() []interface{} {
	return []interface{}{&Episode{}, &EpisodeEntry{}}
}
