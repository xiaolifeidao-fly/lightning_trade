package eventstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/marketslice"
)

// 队列与批量的默认档位。事件总量实测约 9.6 万条/天（心跳 61% + dev_sample 30%），
// r3 把 dev_sample 提到 10 秒后约 10.3 万条/天，峰值仍集中在信号成簇的那几秒，
// 因此队列按"扛住几十秒积压"取，批量按一次 INSERT 不超过几百行取（单行列宽小，
// 200 行的 SQL 仍在几十 KB 量级）。
//
// 例外是 signal_slice：单行带三条 121 点的 JSON 序列、约 4KB，200 行会攒出
// 800KB 的 SQL 直接撞 max_allowed_packet。它按 sliceBatchSize 单独分批，
// 频率上也不需要大批量（每次触发一条，实测 613 条/4 天）。
const (
	defaultQueueSize     = 8192
	defaultBatchSize     = 200
	defaultFlushInterval = 2 * time.Second
	defaultCloseTimeout  = 5 * time.Second
	dropWarnEvery        = 200 // 丢弃告警节流：每 200 条打一次，避免刷屏
	sliceBatchSize       = 20  // signal_slice 单行约 4KB，一次 INSERT 控制在 ~80KB
)

// Identity 账户身份。JSONL 里只有 account 标签（如"账户A-xxx@qq.com"），
// 数字 uid 从来没进过事件；实测 9.9 万条历史事件里零出现，任何字符串解析都
// 不可能从标签恢复出 uid，只能靠配置构建映射。
//
// 账户唯一性口径：uid 优先；uid 为空时回退 (instance_key, account_label)
// ——实例1 与实例3 各有一个"account1"，是两个不同的真实账户。
type Identity struct {
	Label string
	UID   string
}

// Options Store 的构造参数。
type Options struct {
	DSN           string // 原始 sqlconn，内部会强制 loc/parseTime/charset/超时
	InstanceKey   string // argus.instance.id，决定每行的实例归属
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
}

// Stats 写入侧计数快照。dropped/failed 非零即说明有事件只留在 JSONL 里，
// 需要靠 r6 的回灌补齐——这两个数是双写一致性的第一手证据。
type Stats struct {
	Accepted     uint64 // 成功入队
	Dropped      uint64 // 队列满被丢（交易路径绝不阻塞的代价）
	BadTs        uint64 // ts 无法解析
	BadHash      uint64 // 事件无法序列化
	UnknownEvent uint64 // 未识别的事件类型
	Inserted     uint64 // 提交给 MySQL 的行数（含被唯一键去重的）
	Failed       uint64 // 写库失败被丢弃的行数
	Flushes      uint64
}

type storeState struct {
	configVersion uint64
	accounts      map[string]Identity
}

// Store 事件双写 writer：非阻塞入队 + 批量 INSERT ... ON DUPLICATE KEY UPDATE。
type Store struct {
	db            *gorm.DB
	instanceKey   string
	batchSize     int
	flushInterval time.Duration
	queue         chan Envelope
	state         atomic.Value // *storeState

	accepted, dropped         uint64
	badTs, badHash, unknownEv uint64
	inserted, failed, flushes uint64

	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
	purgeOnce sync.Once
}

// Open 建立事件写库的专属连接。
//
// 为什么不复用 common/middleware/db 的全局 Db：
//  1. 那条连接的 DSN 直接来自 properties，loc 写没写全靠人——本包必须保证
//     loc=Local（见 NormalizeDSN），不能依赖配置写对；
//  2. 事件写入是持续的小批量写，和配置读走同一个 500 连接的大池，会让
//     交易路径的配置查询与事件写入互相排队；
//  3. 这里要设死超时，DB 卡住时 writer 自己失败退出，不牵连别人。
//
// sqlconn 为空返回 ErrDSNEmpty，调用方据此完全退化成只写 JSONL 的行为。
func Open(opts Options) (*Store, error) {
	if opts.InstanceKey == "" {
		return nil, fmt.Errorf("eventstore: instance key is required")
	}
	gdb, err := OpenDB(opts.DSN)
	if err != nil {
		return nil, err
	}
	return newStore(gdb, opts), nil
}

// OpenDB 按本包口径建一条事件库连接：DSN 经 NormalizeDSN 强制 loc/parseTime/
// charset/超时，连接池按"持续小批量写"取小值。
//
// 导出它是给回灌与一致性巡检工具（pkg/eventstore/backfill）用的：那条路径
// 不需要 writer goroutine，但必须和直写路径共用同一套 DSN 口径，否则读回的
// ts 会差 8 小时，对账每行都报不一致。
func OpenDB(rawDSN string) (*gorm.DB, error) {
	dsn, err := NormalizeDSN(rawDSN)
	if err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Error),
		SkipDefaultTransaction:                   true, // 单条批量 INSERT 不需要外层事务
	})
	if err != nil {
		return nil, fmt.Errorf("eventstore: open event store connection: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("eventstore: resolve sql handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	return gdb, nil
}

// newStore 只做结构组装，不碰连接——Open 与单测共用，单测用 DryRun 连接注入。
func newStore(gdb *gorm.DB, opts Options) *Store {
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	flushInterval := opts.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	s := &Store{
		db:            gdb,
		instanceKey:   opts.InstanceKey,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		queue:         make(chan Envelope, queueSize),
		done:          make(chan struct{}),
	}
	s.state.Store(&storeState{accounts: map[string]Identity{}})
	return s
}

// EnsureTable 建表/补列。三张事实表是 append-only 的，AutoMigrate 只会加列
// 加索引，不会动已有数据。
func (s *Store) EnsureTable() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("eventstore: store is not initialized")
	}
	return s.db.AutoMigrate(Models()...)
}

// Start 启动写入 goroutine（幂等）。
func (s *Store) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		childCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		go s.run(childCtx)
	})
}

// SetConfigVersion 更新"当前生效的配置版本号"，由配置热加载回调驱动。
func (s *Store) SetConfigVersion(version uint64) {
	if s == nil {
		return
	}
	prev := s.loadState()
	s.state.Store(&storeState{configVersion: version, accounts: prev.accounts})
}

// SetAccounts 更新 account 标签 → uid 映射，由配置热加载回调驱动。
func (s *Store) SetAccounts(identities []Identity) {
	if s == nil {
		return
	}
	accounts := make(map[string]Identity, len(identities))
	for _, id := range identities {
		if id.Label == "" {
			continue
		}
		accounts[id.Label] = id
	}
	prev := s.loadState()
	s.state.Store(&storeState{configVersion: prev.configVersion, accounts: accounts})
}

// Emit 实现 eventlog.Sink：把事件塞进队列后立刻返回。
//
// 队列满时直接丢弃并计数——这是"写库失败只告警、绝不阻断交易路径"的落点。
// 丢掉的事件仍在 JSONL 真源里，可由 r6 的回灌补齐。
func (s *Store) Emit(e eventlog.Event) {
	if s == nil {
		return
	}
	st := s.loadState()
	env := Envelope{
		Event:         e,
		InstanceKey:   s.instanceKey,
		ConfigVersion: st.configVersion,
		Source:        SourceLive,
	}
	if id, ok := st.accounts[e.Account]; ok {
		env.UID = id.UID
	}
	select {
	case s.queue <- env:
		atomic.AddUint64(&s.accepted, 1)
	default:
		if n := atomic.AddUint64(&s.dropped, 1); n%dropWarnEvery == 1 {
			logrus.Errorf("[eventstore] 写入队列已满，事件只留在 JSONL（累计丢弃 %d 条，可用回灌补齐）", n)
		}
	}
}

// EmitSlice 实现 marketslice.Sink：把一条秒级切片塞进同一条队列。
//
// 与 Emit 同规矩——非阻塞、队列满即丢并计数。切片丢了只是这一次触发的下钻图
// 少一条，没有任何交易后果，绝不为它阻塞行情 goroutine。
// 注意切片没有 JSONL 兜底（见 marketslice.Slice 注释），丢掉即永久缺失，
// 因此这里的告警文案与事件侧刻意不同。
func (s *Store) EmitSlice(sl marketslice.Slice) {
	if s == nil {
		return
	}
	st := s.loadState()
	env := Envelope{
		InstanceKey:   s.instanceKey,
		ConfigVersion: st.configVersion,
		Source:        SourceLive,
		Slice:         &sl,
	}
	select {
	case s.queue <- env:
		atomic.AddUint64(&s.accepted, 1)
	default:
		if n := atomic.AddUint64(&s.dropped, 1); n%dropWarnEvery == 1 {
			logrus.Errorf("[eventstore] 写入队列已满，秒级切片被丢弃且无 JSONL 兜底（累计丢弃 %d 条）", n)
		}
	}
}

// Stats 计数快照。
func (s *Store) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	return Stats{
		Accepted:     atomic.LoadUint64(&s.accepted),
		Dropped:      atomic.LoadUint64(&s.dropped),
		BadTs:        atomic.LoadUint64(&s.badTs),
		BadHash:      atomic.LoadUint64(&s.badHash),
		UnknownEvent: atomic.LoadUint64(&s.unknownEv),
		Inserted:     atomic.LoadUint64(&s.inserted),
		Failed:       atomic.LoadUint64(&s.failed),
		Flushes:      atomic.LoadUint64(&s.flushes),
	}
}

// Close 停写并尽力把队列里剩余事件落库。
func (s *Store) Close(timeout time.Duration) {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
			if timeout <= 0 {
				timeout = defaultCloseTimeout
			}
			select {
			case <-s.done:
			case <-time.After(timeout):
				logrus.Warnf("[eventstore] 收尾超时 %s，剩余事件仅保留在 JSONL", timeout)
			}
		}
		if sqlDB, err := s.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		logrus.Infof("[eventstore] 已停止: %+v", s.Stats())
	})
}

func (s *Store) loadState() *storeState {
	if st, ok := s.state.Load().(*storeState); ok && st != nil {
		return st
	}
	return &storeState{accounts: map[string]Identity{}}
}

func (s *Store) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()
	batch := make([]Envelope, 0, s.batchSize)
	for {
		select {
		case <-ctx.Done():
			// 收尾：把已积压的事件尽量写掉，再退出。
			for {
				select {
				case env := <-s.queue:
					batch = append(batch, env)
					if len(batch) >= s.batchSize {
						s.flush(batch)
						batch = batch[:0]
					}
					continue
				default:
				}
				break
			}
			if len(batch) > 0 {
				s.flush(batch)
			}
			return
		case env := <-s.queue:
			batch = append(batch, env)
			if len(batch) >= s.batchSize {
				s.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// flush 把一批事件转成行并落库。任何失败只记 error 并计数：JSONL 是真源，
// 丢掉的行可由回灌补齐，绝不重试到把队列堵死。
func (s *Store) flush(batch []Envelope) {
	if len(batch) == 0 {
		return
	}
	atomic.AddUint64(&s.flushes, 1)
	ingestedAt := time.Now()
	rows := Rows{}
	for _, env := range batch {
		converted, kind := Convert(env, ingestedAt)
		switch kind {
		case ErrNone:
			rows.Strategy = append(rows.Strategy, converted.Strategy...)
			rows.Balance = append(rows.Balance, converted.Balance...)
			rows.Dev = append(rows.Dev, converted.Dev...)
		case ErrBadTs:
			if n := atomic.AddUint64(&s.badTs, 1); n%dropWarnEvery == 1 {
				logrus.Errorf("[eventstore] 事件时间戳无法解析，已丢弃（累计 %d 条）: ts=%q event=%s", n, env.Event.Ts, env.Event.Event)
			}
		case ErrBadHash:
			atomic.AddUint64(&s.badHash, 1)
		case ErrUnknownEvent:
			if n := atomic.AddUint64(&s.unknownEv, 1); n%dropWarnEvery == 1 {
				logrus.Warnf("[eventstore] 未识别的事件类型，已丢弃（累计 %d 条）: event=%s", n, env.Event.Event)
			}
		}
	}
	if rows.Len() == 0 {
		return
	}
	s.insert(len(rows.Strategy), func(tx *gorm.DB) error { return tx.CreateInBatches(rows.Strategy, s.batchSize).Error }, "strategy_event")
	s.insert(len(rows.Balance), func(tx *gorm.DB) error { return tx.CreateInBatches(rows.Balance, s.batchSize).Error }, "balance_sample")
	s.insert(len(rows.Dev), func(tx *gorm.DB) error { return tx.CreateInBatches(rows.Dev, s.batchSize).Error }, "dev_sample")
	s.insert(len(rows.Slice), func(tx *gorm.DB) error { return tx.CreateInBatches(rows.Slice, sliceBatchSize).Error }, "signal_slice")
}

func (s *Store) insert(count int, do func(tx *gorm.DB) error, table string) {
	if count == 0 {
		return
	}
	// DoNothing 在 mysql 驱动上会译成 ON DUPLICATE KEY UPDATE id=id：
	// 幂等哈希撞键时静默跳过，让回灌与补录可以反复重放。
	tx := s.db.Clauses(clause.OnConflict{DoNothing: true})
	if err := do(tx); err != nil {
		atomic.AddUint64(&s.failed, uint64(count))
		logrus.Errorf("[eventstore] %s 批量写入失败，已丢弃 %d 行（不影响交易，JSONL 仍是真源）: %v", table, count, err)
		return
	}
	atomic.AddUint64(&s.inserted, uint64(count))
}
