package eventlog

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// Sink 事件旁路投递口：JSONL 落盘之后，把同一条事件交给外部消费者
// （当前唯一实现是 pkg/eventstore 的 MySQL 双写 writer）。
//
// 实现方必须满足两条硬约束，否则会直接伤到下单/平仓路径：
//  1. Emit 非阻塞——本包被 trade/monitor 的交易 goroutine 直接调用；
//  2. Emit 自行吞掉全部错误——写库失败只告警，绝不向上冒泡。
//
// 本包是 leaf 包（trade 与 monitor 共同引用），因此这里只放接口，绝不引入
// gorm / DB 驱动；具体实现放 pkg/eventstore，由 initialization 组装时注册。
type Sink interface {
	Emit(e Event)
}

var (
	sinkMu sync.RWMutex
	sinks  []Sink
)

// RegisterSink 注册一个旁路 sink，可重复调用（按注册顺序投递）。
func RegisterSink(s Sink) {
	if s == nil {
		return
	}
	sinkMu.Lock()
	sinks = append(sinks, s)
	sinkMu.Unlock()
}

// ResetSinks 清空已注册 sink（进程收尾与测试隔离用）。
func ResetSinks() {
	sinkMu.Lock()
	sinks = nil
	sinkMu.Unlock()
}

// SinkCount 已注册的 sink 数量（自检与测试用）。
func SinkCount() int {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return len(sinks)
}

// emitToSinks 把事件投递给全部 sink。单个 sink 的 panic 只记 error——
// 旁路投递永远不能把 JSONL 落盘或交易主流程带崩。
func emitToSinks(e Event) {
	sinkMu.RLock()
	list := sinks
	sinkMu.RUnlock()
	for _, s := range list {
		emitOne(s, e)
	}
}

func emitOne(s Sink, e Event) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[eventlog] sink 投递 panic（不影响交易）: %v", r)
		}
	}()
	s.Emit(e)
}
