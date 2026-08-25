package trade

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// botCloseKey 注册表键：instId 归一到基础币——trade 侧用 trade_inst(BTCUSDT) 打标、
// monitor 侧用 swap inst(BTC-USDT-SWAP) 消费，必须落到同一键（P2-A）。
// review fix#2：键含 posId 绑定仓位实例——净仓模式下平仓后 5s 内同向重开会让
// account:base:side 键持续存活，旧标记 10min 内可吞掉新仓的 external_close；
// posId 使标记只匹配被平的那个仓（双侧 posId 均空时退化为旧语义，防御性一致）。
func botCloseKey(account, instId, posSide, posId string) string {
	return fmt.Sprintf("%s:%s:%s:%s", account, instrumentBase(instId), strings.ToLower(strings.TrimSpace(posSide)), strings.TrimSpace(posId))
}

// botCloseRegistry "本机器人平仓"标记表：平仓成功打标，快照对账消费；
// 未被消费的仓位消失 = 外部平仓（external_close）。TTL 兜底防标记泄漏。
type botCloseRegistry struct {
	mu    sync.Mutex
	marks map[string]time.Time
	now   func() time.Time
	ttl   time.Duration
}

func newBotCloseRegistry(now func() time.Time, ttl time.Duration) *botCloseRegistry {
	return &botCloseRegistry{marks: make(map[string]time.Time), now: now, ttl: ttl}
}

func (r *botCloseRegistry) mark(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for k, t := range r.marks { // 顺带清扫过期项，防泄漏
		if now.Sub(t) > r.ttl {
			delete(r.marks, k)
		}
	}
	r.marks[key] = now
}

func (r *botCloseRegistry) consume(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.marks[key]
	if !ok {
		return false
	}
	delete(r.marks, key)
	return r.now().Sub(t) <= r.ttl
}

var defaultBotClose = newBotCloseRegistry(time.Now, 10*time.Minute)

// MarkBotClose 本机器人平仓成功后打标（monitor 平仓路径 / trade 减仓到 0 调用）。
func MarkBotClose(account, instId, posSide, posId string) {
	defaultBotClose.mark(botCloseKey(account, instId, posSide, posId))
}

// ConsumeBotClose 快照对账时消费标记（posId 取消失仓位的最后快照）；
// 命中=本机器人平仓，不落 external_close。
func ConsumeBotClose(account, instId, posSide, posId string) bool {
	return defaultBotClose.consume(botCloseKey(account, instId, posSide, posId))
}

