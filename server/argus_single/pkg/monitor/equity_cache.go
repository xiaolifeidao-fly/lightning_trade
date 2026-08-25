package monitor

import (
	"strings"
	"time"

	"common/utils"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
)

// sumAccountUpl 求账户未实现盈亏合计（符号经 normalizePositionPnl 归一）。
// 第二返回值=完整性：任一 live 仓位 UPL 不可解析、或 Pos 行不可解析 → false，
// 调用方保留旧缓存（review fix#1：瞬空写 0 会让 equity=balance，恰好隐藏
// 最需要记录的深水浮亏）。dead 行（Pos=0）跳过不影响完整性；空仓 = (0, true)。
func sumAccountUpl(positions []utils.PositionInfo) (float64, bool) {
	total := 0.0
	for _, pos := range positions {
		switch classifyPosLiveness(pos.Pos) {
		case posDead:
			continue
		case posUnknown:
			return 0, false
		}
		pnl, ok := normalizePositionPnl(pos)
		if !ok {
			return 0, false
		}
		total += pnl.InexactFloat64()
	}
	return total, true
}

// buildBalanceEvent 构造 balance 事件；hasUpl=true 时补 Equity=balance+upl 与 Upl
// （UPL 缓存正常 ≤5s 滞后、上界 3×轮询间隔，两字段可自校验）。hasUpl=false 有两种：
// 启动后首查早于首次持仓轮询（缓存无值）、缓存陈旧（持仓查询持续异常，uplFresh 判定）
// ——均诚实省略 equity/upl，报告侧回退 balance。netSize 为当前净仓张数快照
// （review fix#3：给报告"最大张数"提供窗口起点基线，顺带成为每分钟仓位序列）。
// 纯函数（P2-C/E）。
func buildBalanceEvent(acc trade.AccountConfig, bal, upl float64, hasUpl bool, netSize int) eventlog.Event {
	e := eventlog.Event{Account: acc.Name, Variant: acc.Variant, Event: eventlog.EvBalance, Balance: bal, Size: netSize}
	if hasUpl {
		e.Equity = bal + upl
		e.Upl = upl
		e.EquityKnown = true // 显式有效标记：equity=0（浮亏恰抵平余额）时 omitempty 会省略值本身
	}
	return e
}

// accountNetSize 该账户当前净仓张数（快照口径，≤5s 旧；净仓模式通常单条）。
func (am *AccountMonitor) accountNetSize(account string) int {
	prefix := account + ":"
	total := 0
	am.snapMu.RLock()
	for k, s := range am.snapshots {
		if strings.HasPrefix(k, prefix) {
			total += s.Size
		}
	}
	am.snapMu.RUnlock()
	return total
}

// uplSample UPL 缓存样本：值 + 采样时刻。持仓查询失败/不完整轮保留旧值（fix#1），
// 但旧值不得无限期冒充实时——读侧按 uplFresh 判新鲜度，陈旧即省略 equity/upl。
type uplSample struct {
	Value      float64
	ObservedAt time.Time
}

// setUpl / getUpl：monitor 内部 UPL 缓存（持仓轮询每 5s 更新，余额循环读取）。
func (am *AccountMonitor) setUpl(account string, upl float64, at time.Time) {
	am.uplMu.Lock()
	am.uplByAccount[account] = uplSample{Value: upl, ObservedAt: at}
	am.uplMu.Unlock()
}

func (am *AccountMonitor) getUpl(account string) (uplSample, bool) {
	am.uplMu.RLock()
	defer am.uplMu.RUnlock()
	s, ok := am.uplByAccount[account]
	return s, ok
}

// uplFresh 新鲜度判定（纯函数）：最大允许年龄 = 3×轮询间隔（5s 轮询即 15s 上界）。
// 3× 给瞬断/查询抖动留冗余、同时把陈旧上界钉死在秒级；按年龄描述而非失败次数
// ——边界相位下两者不一一对应（两次连败后年龄可达 15s 仍在界内，第三次起才必然超界）。
func uplFresh(s uplSample, now time.Time, pollInterval time.Duration) bool {
	return now.Sub(s.ObservedAt) <= 3*pollInterval
}

