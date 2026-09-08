package repository

import (
	"fmt"
	"strings"
	"time"

	"argus_single/pkg/eventstore"

	"gorm.io/gorm"
)

// 本文件是盘口信号回测（r8）的读侧：从 argus_single 双写进来的三张事实表
// （strategy_event / dev_sample，见 argus_single/pkg/eventstore/model.go）里
// 取回放输入。
//
// 为什么直接复用 eventstore 的 gorm 模型而不在本包再定义一遍实体：
// 表结构由写侧（r4）拥有，两边各写一份必然漂移；service 模块的 go.mod 已经
// replace argus_single，import 它零成本。
//
// 注意：这三张表是 append-only 事实表，**不带** BaseEntity 的 active 列，
// 所以查询里不能带 `active = 1`（会直接报未知列）。

// eventReader 只提供 Db 与 SetDb。eventstore 的事实表不嵌 db.BaseEntity
// （r4 的设计决定：append-only、不需要逻辑删除与 created_by，且要精确控制
// binary(16)/datetime 列型），因此套不进 db.Repository[T]——那个泛型基类要求
// Entity 实现 Init()。这里不去改写侧的模型迁就泛型基类，而是只取
// db.GetRepository 需要的 SetDb 接口。
type eventReader struct {
	Db *gorm.DB
}

// SetDb 供 db.GetRepository 注入全局连接。
func (r *eventReader) SetDb(d *gorm.DB) { r.Db = d }

// ─── strategy_event ─────────────────────────────────────────────────────────

type StrategyEventRepository struct {
	eventReader
}

// EnsureTable 建表。写侧（argus_single 进程）也会 AutoMigrate 同一张表；
// 管理端先起来时这里负责建出来，好让回测页在事件还没进来时也能查出空集。
func (r *StrategyEventRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&eventstore.StrategyEvent{})
}

// SignalEventFilter 信号流查询条件。instanceKey 与 accountLabel 缺一不可：
// 三个部署实例写同一张表，且实例1 有两个账户，少一个维度就会把不同实验体的
// 触发流混成一条，回测结果直接失真。
type SignalEventFilter struct {
	InstanceKey  string
	AccountLabel string
	Instrument   string // 归一化合约（BTCUSDT）；空=不限
	Start        time.Time
	End          time.Time
	Events       []string // 空=不限
}

// ListEvents 按实例 + 账户 + 时间区间取事件，按 (ts, id) 升序。
// 同秒多条触发是常态（实测单分钟最多 5 次），必须按 id 二次排序保证可复现，
// 且**不去重**——去重会丢掉真实触发。
func (r *StrategyEventRepository) ListEvents(f SignalEventFilter) ([]*eventstore.StrategyEvent, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&eventstore.StrategyEvent{}).
		Where("instance_key = ? AND ts >= ? AND ts <= ?", f.InstanceKey, f.Start, f.End)
	if label := strings.TrimSpace(f.AccountLabel); label != "" {
		q = q.Where("account_label = ?", label)
	}
	if inst := strings.TrimSpace(f.Instrument); inst != "" {
		q = q.Where("instrument = ?", strings.ToUpper(inst))
	}
	if len(f.Events) > 0 {
		q = q.Where("event IN ?", f.Events)
	}
	var rows []*eventstore.StrategyEvent
	if err := q.Order("ts ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountEvents 同条件下的事件数。回测的"零丢失零重复"验收就是拿它与
// 引擎回放到的触发数对账，所以它必须与 ListEvents 共用同一套 where。
func (r *StrategyEventRepository) CountEvents(f SignalEventFilter) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&eventstore.StrategyEvent{}).
		Where("instance_key = ? AND ts >= ? AND ts <= ?", f.InstanceKey, f.Start, f.End)
	if label := strings.TrimSpace(f.AccountLabel); label != "" {
		q = q.Where("account_label = ?", label)
	}
	if inst := strings.TrimSpace(f.Instrument); inst != "" {
		q = q.Where("instrument = ?", strings.ToUpper(inst))
	}
	if len(f.Events) > 0 {
		q = q.Where("event IN ?", f.Events)
	}
	var total int64
	err := q.Count(&total).Error
	return total, err
}

// SignalSource 一个可选的信号源（实例 + 账户），供回测表单枚举。
type SignalSource struct {
	InstanceKey  string    `json:"instanceKey"`
	AccountLabel string    `json:"accountLabel"`
	Variant      string    `json:"variant"`
	Instrument   string    `json:"instrument"`
	EventCount   int64     `json:"eventCount"`
	FirstTs      time.Time `json:"firstTs"`
	LastTs       time.Time `json:"lastTs"`
}

// ListSignalSources 枚举已入库的信号源（含覆盖区间与事件数），
// 让回测表单只能选真实存在的实例×账户，而不是让人手打字符串。
func (r *StrategyEventRepository) ListSignalSources(events []string) ([]*SignalSource, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&eventstore.StrategyEvent{}).
		Select("instance_key, account_label, MAX(variant) AS variant, instrument, " +
			"COUNT(*) AS event_count, MIN(ts) AS first_ts, MAX(ts) AS last_ts").
		Group("instance_key, account_label, instrument").
		Order("instance_key ASC, account_label ASC")
	if len(events) > 0 {
		q = q.Where("event IN ?", events)
	}
	var rows []*SignalSource
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ─── dev_sample ─────────────────────────────────────────────────────────────

type DevSampleRepository struct {
	eventReader
}

func (r *DevSampleRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&eventstore.DevSample{})
}

// ListSamples 取区间内的无条件偏离采样窗口，按 ts 升序。
// 它是频率级回测（改 signal_threshold）唯一可信的输入：DevCross 不经阈值
// 过滤，直接就是 λ(θ) 的测量值。
func (r *DevSampleRepository) ListSamples(instanceKey, instrument string, start, end time.Time) ([]*eventstore.DevSample, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&eventstore.DevSample{}).Where("ts >= ? AND ts <= ?", start, end)
	if key := strings.TrimSpace(instanceKey); key != "" {
		q = q.Where("instance_key = ?", key)
	}
	if inst := strings.TrimSpace(instrument); inst != "" {
		q = q.Where("instrument = ?", strings.ToUpper(inst))
	}
	var rows []*eventstore.DevSample
	if err := q.Order("ts ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
