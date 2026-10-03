package eventstore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/marketslice"
)

var (
	defaultMu    sync.RWMutex
	defaultStore *Store
)

// Setup 组装事件双写：建连接 → 建表 → 注册进 eventlog → 启动 writer。
//
// 调用方（initialization）必须把它当"可失败的增强"而不是启动前置：sqlconn
// 为空或 DB 不可用时返回 error，此时不注册 sink，进程完全退化成今天只写
// JSONL 的行为，交易照常。DB 问题绝不能让实盘进程起不来。
func Setup(ctx context.Context, opts Options) (*Store, error) {
	store, err := Open(opts)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureTable(); err != nil {
		store.Close(time.Second)
		return nil, err
	}
	store.Start(ctx)
	eventlog.RegisterSink(store)
	// 秒级切片走独立投递口（r3）：它不是 eventlog 事件，也不进 JSONL。
	marketslice.RegisterSink(store)
	store.StartSlicePurge(ctx)

	defaultMu.Lock()
	defaultStore = store
	defaultMu.Unlock()

	logrus.Infof("[eventstore] 策略事件已双写 MySQL: instanceKey=%s（JSONL 仍是真源，保留至 %s）；"+
		"秒级切片入 signal_slice，保留 %.0f 天",
		opts.InstanceKey, JSONLDualWriteUntil, SliceRetention.Hours()/24)
	return store, nil
}

// 后台重试的退避：1→2→4→8→10 分钟封顶。
const (
	retryBackoffInitial = time.Minute
	retryBackoffMax     = 10 * time.Minute
)

// nextBackoff 下一次重试间隔（纯函数，便于测试）。
func nextBackoff(prev time.Duration) time.Duration {
	if prev <= 0 {
		return retryBackoffInitial
	}
	next := prev * 2
	if next > retryBackoffMax {
		return retryBackoffMax
	}
	return next
}

// SetupWithRetry 先同步 Setup 一次；失败且不是"未配置"时，在后台按退避重试直到成功
// 或 ctx 结束。成功（无论首次还是重试）都回调 onReady（调用方在里面灌配置版本与账户映射）。
//
// 10-03 事故：Setup 只在启动时跑一次，一把锁让它失败后，进程这一辈子都只写 JSONL，
// 65 分钟没有任何自愈。重试期间的事件只进 JSONL（真源），事后可用 argus-event-import 回灌。
func SetupWithRetry(ctx context.Context, opts Options, onReady func(*Store)) (*Store, error) {
	store, err := Setup(ctx, opts)
	if err == nil {
		if onReady != nil {
			onReady(store)
		}
		return store, nil
	}
	if IsDisabled(err) {
		return nil, err
	}
	go retrySetup(ctx, opts, onReady)
	return nil, err
}

func retrySetup(ctx context.Context, opts Options, onReady func(*Store)) {
	wait := nextBackoff(0)
	for attempt := 1; ; attempt++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		store, err := Setup(ctx, opts)
		if err == nil {
			logrus.Infof("[eventstore] 第 %d 次重试后双写恢复", attempt)
			if onReady != nil {
				onReady(store)
			}
			return
		}
		wait = nextBackoff(wait)
		logrus.Warnf("[eventstore] 第 %d 次重试仍失败，%s 后再试（期间只写 JSONL，不影响交易）: %v", attempt, wait, err)
	}
}

// Default 当前默认 store（未装配时为 nil，全部方法都能安全地在 nil 上调用）。
func Default() *Store {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultStore
}

// IsDisabled 判断 Setup 的失败是不是"没配 sqlconn"这种预期内的关闭态。
func IsDisabled(err error) bool { return errors.Is(err, ErrDSNEmpty) }

// SetConfigVersion 更新默认 store 的配置版本号。
func SetConfigVersion(version uint64) { Default().SetConfigVersion(version) }

// SetAccounts 更新默认 store 的 account 标签 → uid 映射。
func SetAccounts(identities []Identity) { Default().SetAccounts(identities) }

// Shutdown 停掉默认 store 并注销 sink。
func Shutdown(timeout time.Duration) {
	defaultMu.Lock()
	store := defaultStore
	defaultStore = nil
	defaultMu.Unlock()
	if store == nil {
		return
	}
	eventlog.ResetSinks()
	marketslice.ResetSinks()
	store.Close(timeout)
}
