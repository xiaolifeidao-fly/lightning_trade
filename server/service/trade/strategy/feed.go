package strategy

import (
	"context"
	"time"
)

// Clock 抽象当前时间：实盘=墙钟，回测=回放推进的仿真时钟。
type Clock interface {
	Now() time.Time
}

// PriceFeed 抽象价格来源：实盘=实时 tick 流，回测=历史 K 线回放。
// 这是「两个驱动器」的输入侧接缝——引擎不关心实现：
//
//	实盘实现：订阅 hub tick，Next 阻塞等下一个 tick(High=Low=Price)；
//	回测实现：按 price_interval 顺序回放区间内历史 K 线，放完置 done=true。
//
// ★ 两个真实实现都尚未落地，是 B 阶段刻意留出的接缝；引擎本身已可独立单测。
type PriceFeed interface {
	// Next 返回下一条价格观测；done=true 表示流结束(回测放完 / 实盘停止)。
	Next(ctx context.Context) (q Quote, done bool, err error)
}

// Sink 抽象结果去向：实盘=写 trade_strategy_position，回测=写 trade_backtest_trade。
// 这是「两个驱动器」的输出侧接缝。Order 状态每变化一次调用一次 OnChange。
//
// ★ 两个真实实现(落库)尚未落地，待 A 阶段建表后再接。
type Sink interface {
	OnChange(o *Order) error
}

// NoopSink 丢弃所有变更，用于演示 / 干跑 / 单测。
type NoopSink struct{}

// OnChange 不做任何事。
func (NoopSink) OnChange(*Order) error { return nil }

// SliceFeed 用切片顺序回放的 PriceFeed，供回测 K 线回放与单测复用。
type SliceFeed struct {
	quotes []Quote
	i      int
}

// NewSliceFeed 用一段有序行情构造回放源(回测把窗口内 K 线转成 []Quote 传入)。
func NewSliceFeed(quotes []Quote) *SliceFeed { return &SliceFeed{quotes: quotes} }

// Next 顺序吐出下一条行情，放完置 done。
func (f *SliceFeed) Next(context.Context) (Quote, bool, error) {
	if f.i >= len(f.quotes) {
		return Quote{}, true, nil
	}
	q := f.quotes[f.i]
	f.i++
	return q, false, nil
}

// SystemClock 返回墙钟实现(实盘默认)。
func SystemClock() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Run 驱动单笔 Order 走到终态：从 feed 取价 → Step 推进 → 有变化则落 Sink。
//
// 实盘与回测都复用本函数——差异只在传入的 PriceFeed / Sink 不同，引擎逻辑零差异。
// 这是「一个引擎、两个驱动器」可复用的最小证明点：
//
//	实盘： strategy.Run(ctx, order, hubTickFeed, positionSink)
//	回测： strategy.Run(ctx, order, klineReplayFeed, backtestSink)
func Run(ctx context.Context, o *Order, feed PriceFeed, sink Sink) error {
	for !o.Terminal() {
		q, done, err := feed.Next(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if o.Step(q) {
			if err := sink.OnChange(o); err != nil {
				return err
			}
		}
	}
	return nil
}
