// Package eventstore 把 pkg/eventlog 的结构化事件双写进共享 MySQL。
//
// 为什么独立成包：pkg/eventlog 是 leaf 包（trade 与 monitor 共同引用），
// 给它加 gorm 依赖会造成 trade → eventlog → gorm/service 的循环，
// 也会污染 backtest/ 与全部历史分析脚本对 eventlog 的零依赖假设。
// 因此这里只反向依赖 eventlog（实现它的 Sink 接口），gorm 只出现在本包。
//
// 三条硬不变量（写在包头，改代码前先读）：
//  1. 写库失败只告警、绝不阻断交易路径——Emit 非阻塞，队列满即丢并计数；
//  2. JSONL 是真源，双写保留至 JSONLDualWriteUntil，不得提前停写；
//  3. ts 用 DATETIME（秒精度）不用 TIMESTAMP，DSN 强制 loc=Local，
//     否则与 JSONL 的无时区字符串差 8 小时——这个错误极隐蔽（数据全在、
//     图能画、只是整体偏移）。
package eventstore

import "time"

// JSONLDualWriteUntil JSONL 双写的保留下限。入库能力上线后 JSONL 仍是回测平台
// 与全部历史分析脚本的输入格式，停写时点由一致性巡检结果决定（需求大纲 §9.4）。
const JSONLDualWriteUntil = "2026-12-01"

// 数据来源标记（source 列）：直写与回灌必须可区分，一致性校验按它分组。
const (
	SourceLive     = 1 // argus_single 进程内直写
	SourceBackfill = 2 // logs/ 下历史 JSONL 回灌（r6）
)

// StrategyEvent 稀疏业务事件表：open / cap_skip / gate_block / trend_skip /
// trailing_close / catastrophe_stop / fixed_close / manual_close /
// external_close / loss_alert 共 10 类。
//
// 分表依据（需求大纲 §3.1 与实测占比）：balance 心跳 61%、dev_sample 30%、
// 业务事件仅 9%。把三者塞一张表会让业务查询每次都要跨掉 91% 的无关行，
// 且心跳/采样两类的列集与业务事件几乎不重叠（全是 NULL）。
// r3 把 dev_sample 粒度提到 10 秒后它单表就占了 8640 条/天（约全量的 85%），
// 分表这个决定只会更划算。
type StrategyEvent struct {
	Id            uint64    `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	EventHash     []byte    `gorm:"column:event_hash;type:binary(16);not null;uniqueIndex:uk_strategy_event_hash" description:"Go 侧算的幂等哈希，见 hash.go"`
	Ts            time.Time `gorm:"column:ts;type:datetime;not null;index:idx_strategy_event_ts;index:idx_strategy_event_inst_ts,priority:2;index:idx_strategy_event_uid_ts,priority:3;index:idx_strategy_event_inst_event_ts,priority:3;index:idx_strategy_event_gate,priority:2" description:"事件时刻，本地时区口径，与 JSONL 逐字一致"`
	InstanceKey   string    `gorm:"column:instance_key;type:varchar(64);not null;index:idx_strategy_event_inst_ts,priority:1;index:idx_strategy_event_uid_ts,priority:1;index:idx_strategy_event_inst_event_ts,priority:1" description:"实例键，对应 argus_instance.instance_key"`
	ConfigVersion uint64    `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"事件发生时该实例生效的配置版本号"`
	UID           string    `gorm:"column:uid;type:varchar(32);not null;default:'';index:idx_strategy_event_uid_ts,priority:2" description:"数字 uid；配置未填时为空，账户唯一性回退 (instance_key, account_label)"`
	AccountLabel  string    `gorm:"column:account_label;type:varchar(128);not null;default:''" description:"JSONL 原 account 串（含邮箱标签）"`
	Variant       *string   `gorm:"column:variant;type:varchar(64)" description:"策略变体标签，自由文本、会持续新增"`
	Event         string    `gorm:"column:event;type:varchar(24);not null;index:idx_strategy_event_inst_event_ts,priority:2" description:"事件类型"`
	Instrument    string    `gorm:"column:instrument;type:varchar(24);not null;default:''" description:"归一化合约（BTCUSDT）"`
	InstIdRaw     *string   `gorm:"column:inst_id_raw;type:varchar(32)" description:"JSONL 原 instId，对账用"`
	Side          *string   `gorm:"column:side;type:varchar(8)" description:"本次动作方向"`
	NetSide       *string   `gorm:"column:net_side;type:varchar(8)" description:"净仓方向"`
	Size          *int      `gorm:"column:size;type:int;size:32" description:"结果净仓张数（open）或持仓张数（平仓类）"`
	OrderSize     *int      `gorm:"column:order_size;type:int;size:32" description:"本次下单张数"`
	AvgPx         *float64  `gorm:"column:avg_px;type:decimal(20,8)" description:"开仓均价"`
	LastPx        *float64  `gorm:"column:last_px;type:decimal(20,8)" description:"最新价"`
	RoiPct        *float64  `gorm:"column:roi_pct;type:decimal(14,6)" description:"ROI%（含杠杆口径）"`
	Pnl           *float64  `gorm:"column:pnl;type:decimal(20,8)" description:"盈亏"`
	PeakPct       *float64  `gorm:"column:peak_pct;type:decimal(14,6)" description:"平仓时 trail 峰值（激活过才有）"`
	SigLast       *float64  `gorm:"column:sig_last;type:decimal(20,8)" description:"信号时刻 DeepCoin last"`
	SigMark       *float64  `gorm:"column:sig_mark;type:decimal(20,8)" description:"信号时刻 mark"`
	GapBp         *float64  `gorm:"column:gap_bp;type:decimal(12,4)" description:"偏离 bp，带符号（>0=UP）"`
	TrendMomPct   *float64  `gorm:"column:trend_mom_pct;type:decimal(14,6)" description:"趋势闸拦截时刻的窗口动量%"`
	GateKind      *string   `gorm:"column:gate_kind;type:varchar(32);index:idx_strategy_event_gate,priority:1" description:"门控种类，见 gate.go；非拦截事件为 NULL"`
	GateThreshold *float64  `gorm:"column:gate_threshold;type:decimal(20,8)" description:"门控阈值（结构化自 reason）"`
	GateActual    *float64  `gorm:"column:gate_actual;type:decimal(20,8)" description:"门控实际值（结构化自 reason）"`
	Reason        *string   `gorm:"column:reason;type:varchar(255)" description:"原始 reason 文本，保留以便与 TG 消息逐字对账"`
	Source        int8      `gorm:"column:source;type:tinyint;not null" description:"1=直写 2=回灌"`
	Ext           *string   `gorm:"column:ext;type:json" description:"将来新字段落这里，不改表"`
	IngestedAt    time.Time `gorm:"column:ingested_at;type:datetime(3);not null" description:"入库时刻"`
}

func (StrategyEvent) TableName() string { return "strategy_event" }

// BalanceSample 分钟级余额/权益心跳表（balance 事件，实测占全部事件 61%）。
type BalanceSample struct {
	Id            uint64    `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	EventHash     []byte    `gorm:"column:event_hash;type:binary(16);not null;uniqueIndex:uk_balance_sample_hash" description:"幂等哈希"`
	Ts            time.Time `gorm:"column:ts;type:datetime;not null;index:idx_balance_sample_ts;index:idx_balance_sample_inst_uid_ts,priority:3" description:"采样时刻"`
	InstanceKey   string    `gorm:"column:instance_key;type:varchar(64);not null;index:idx_balance_sample_inst_uid_ts,priority:1" description:"实例键"`
	ConfigVersion uint64    `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"生效配置版本号"`
	UID           string    `gorm:"column:uid;type:varchar(32);not null;default:'';index:idx_balance_sample_inst_uid_ts,priority:2" description:"数字 uid"`
	AccountLabel  string    `gorm:"column:account_label;type:varchar(128);not null;default:''" description:"JSONL 原 account 串"`
	Variant       *string   `gorm:"column:variant;type:varchar(64)" description:"策略变体标签"`
	Balance       float64   `gorm:"column:balance;type:decimal(20,8);not null" description:"USDT 余额"`
	Equity        *float64  `gorm:"column:equity;type:decimal(20,8)" description:"balance+UPL；NULL=该口径不可用"`
	Upl           *float64  `gorm:"column:upl;type:decimal(20,8)" description:"未实现盈亏合计"`
	EquityKnown   uint8     `gorm:"column:equity_known;type:tinyint unsigned;not null;default:0" description:"equity 已知性；0/负权益也是已知样本"`
	NetSize       *int      `gorm:"column:net_size;type:int;size:32" description:"净仓张数快照"`
	NetSizeKnown  uint8     `gorm:"column:net_size_known;type:tinyint unsigned;not null;default:0" description:"区分『已知空仓』与『字段未上线』"`
	Source        int8      `gorm:"column:source;type:tinyint;not null" description:"1=直写 2=回灌"`
	IngestedAt    time.Time `gorm:"column:ingested_at;type:datetime(3);not null" description:"入库时刻"`
}

func (BalanceSample) TableName() string { return "balance_sample" }

// DevSample 无条件偏离采样表（dev_sample 事件）。1min 粒度时占全部事件 30%，
// r3 提到 10 秒后是 8640 条/天、约全量的 85%。
//
// 它是市场侧事件、与账户无关（无 uid/account/side/pnl），且载荷是两个
// 阈值→计数的 map；塞进 strategy_event 会让该表 3/4 的行账户列全空、
// 业务查询必须每次带 event != 'dev_sample' 才不被污染。
type DevSample struct {
	Id            uint64    `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	EventHash     []byte    `gorm:"column:event_hash;type:binary(16);not null;uniqueIndex:uk_dev_sample_hash" description:"幂等哈希"`
	Ts            time.Time `gorm:"column:ts;type:datetime;not null;index:idx_dev_sample_ts;index:idx_dev_sample_inst_ts,priority:3" description:"窗口落盘时刻"`
	InstanceKey   string    `gorm:"column:instance_key;type:varchar(64);not null;index:idx_dev_sample_inst_ts,priority:1" description:"实例键"`
	ConfigVersion uint64    `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"生效配置版本号"`
	Instrument    string    `gorm:"column:instrument;type:varchar(24);not null;default:'';index:idx_dev_sample_inst_ts,priority:2" description:"归一化合约"`
	InstIdRaw     *string   `gorm:"column:inst_id_raw;type:varchar(32)" description:"JSONL 原 instId"`
	DevTicks      int       `gorm:"column:dev_ticks;type:int;size:32;not null" description:"窗口内有效 tick 数；非零即表示采样器在工作"`
	DevMaxBp      *float64  `gorm:"column:dev_max_bp;type:decimal(12,4)" description:"窗口内 |dev| 极值"`
	DevMeanBp     *float64  `gorm:"column:dev_mean_bp;type:decimal(12,4)" description:"窗口内 |dev| 均值"`
	DevCrossJson  *string   `gorm:"column:dev_cross_json;type:json" description:"各候选阈值(bp)的穿越次数 → λ(θ)"`
	DevOverJson   *string   `gorm:"column:dev_over_json;type:json" description:"各候选阈值(bp)的超阈 tick 数 → P(|dev|>θ)"`
	Source        int8      `gorm:"column:source;type:tinyint;not null" description:"1=直写 2=回灌"`
	IngestedAt    time.Time `gorm:"column:ingested_at;type:datetime(3);not null" description:"入库时刻"`
}

func (DevSample) TableName() string { return "dev_sample" }

// SignalSlice 触发瞬间 ±60s 秒级行情切片表（r3）。
//
// 一次触发一行，逐秒点位存成三条等长 JSON 数组（DC last / DC mark / 币安 last
// 各 121 点，见 marketslice.Slice）。为什么不按行存秒级点位：±3min 窗口的
// worst case 达 5256 万行/年，与"全时段秒级 K 线"（已否决）同量级——信号成簇，
// 相邻窗口互相覆盖（需求大纲 §3.1）。按 JSON 序列存是 7 万行/年 ≈ 0.44GB/年。
//
// 唯一键直接用 (instance_key, instrument, ts) 而不是像事件表那样另算一个哈希：
// 三列全部 NOT NULL，MySQL 的 "NULL 互不相等" 问题不存在；且一次触发在一个实例
// 上就该只有一条切片，重放/补录必须落在同一行上，不能因为载荷差一个点就插新行。
//
// 与三张事件表的关键差别：**它不是 append-only 真源，是 90 天滚动的派生观测品**
// （见 SliceRetention）。任何要长期回溯的口径都不能只依赖这张表。
type SignalSlice struct {
	Id            uint64    `gorm:"column:id;primaryKey;autoIncrement" description:"主键"`
	Ts            time.Time `gorm:"column:ts;type:datetime;not null;uniqueIndex:uk_signal_slice_anchor,priority:3;index:idx_signal_slice_ts" description:"触发时刻（锚点），与 strategy_event.ts 同口径"`
	InstanceKey   string    `gorm:"column:instance_key;type:varchar(64);not null;uniqueIndex:uk_signal_slice_anchor,priority:1" description:"实例键，对应 argus_instance.instance_key"`
	ConfigVersion uint64    `gorm:"column:config_version;type:bigint unsigned;not null;default:0" description:"生效配置版本号"`
	Instrument    string    `gorm:"column:instrument;type:varchar(24);not null;default:'';uniqueIndex:uk_signal_slice_anchor,priority:2" description:"归一化合约（BTCUSDT）"`
	InstIdRaw     *string   `gorm:"column:inst_id_raw;type:varchar(32)" description:"原始 instId，对账用"`
	StartAt       time.Time `gorm:"column:start_at;type:datetime;not null" description:"窗口左界 = ts - half_window_sec"`
	EndAt         time.Time `gorm:"column:end_at;type:datetime;not null" description:"窗口右界 = ts + half_window_sec"`
	HalfWindowSec int       `gorm:"column:half_window_sec;type:smallint;not null" description:"半窗秒数（当前恒为 60）"`
	SeriesPoints  int       `gorm:"column:series_points;type:smallint;not null" description:"每条序列的点数（2*half+1，当前 121）"`
	DcPoints      int       `gorm:"column:dc_points;type:smallint;not null;default:0" description:"DC 序列非空点数；覆盖率，判断切片是否可用"`
	BinPoints     int       `gorm:"column:bin_points;type:smallint;not null;default:0" description:"币安序列非空点数"`
	DcLastJson    string    `gorm:"column:dc_last_json;type:json;not null" description:"DeepCoin last 逐秒序列，null 元素=该秒无 tick（不插值）"`
	DcMarkJson    string    `gorm:"column:dc_mark_json;type:json;not null" description:"DeepCoin mark 逐秒序列"`
	BinLastJson   string    `gorm:"column:bin_last_json;type:json;not null" description:"币安 last 逐秒序列"`
	Source        int8      `gorm:"column:source;type:tinyint;not null" description:"1=直写 2=回灌"`
	IngestedAt    time.Time `gorm:"column:ingested_at;type:datetime(3);not null" description:"入库时刻"`
}

func (SignalSlice) TableName() string { return "signal_slice" }

// Models 四张表，供 AutoMigrate 与读侧共用，避免两边各写一份表清单。
// 前三张是 append-only 事实表；signal_slice 是 90 天滚动的派生切片表。
func Models() []interface{} {
	return []interface{}{&StrategyEvent{}, &BalanceSample{}, &DevSample{}, &SignalSlice{}}
}
