package monitor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	"common/utils"

	"argus_single/pkg/trade"
)

// closeFailTracker 平仓连败计数（P3/R4）：按仓位 key 计数，成功清零；
// 仓位消失（外部平仓/自家平仓）时随生命周期清零——防"旧仓2败+新仓首败=误升级"。
type closeFailTracker struct {
	mu    sync.Mutex
	fails map[string]closeFailState
	now   func() time.Time
}

type closeFailState struct {
	count   int
	firstAt time.Time
}

func newCloseFailTracker(now func() time.Time) *closeFailTracker {
	return &closeFailTracker{fails: make(map[string]closeFailState), now: now}
}

// recordFail 记一次失败，返回当前连败次数与自首败以来的时长。
func (t *closeFailTracker) recordFail(key string) (int, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.fails[key]
	if !ok {
		st = closeFailState{firstAt: t.now()}
	}
	st.count++
	t.fails[key] = st
	return st.count, t.now().Sub(st.firstAt)
}

// recordSuccess 平仓成功清零。
func (t *closeFailTracker) recordSuccess(key string) {
	t.mu.Lock()
	delete(t.fails, key)
	t.mu.Unlock()
}

// clearMissing 生命周期清零：该账户前缀下、不在 liveKeys 的计数删除。
func (t *closeFailTracker) clearMissing(prefix string, live map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.fails {
		if strings.HasPrefix(k, prefix) && !live[k] {
			delete(t.fails, k)
		}
	}
}

// escalateCloseFail 升级告警（P3）：连败 ≥3（count>0）或配置/数据故障（count=0 立即）。
// 独立节流 key（复用 5min 冷却机制），与普通盈亏告警互不影响。
func (am *AccountMonitor) escalateCloseFail(acc trade.AccountConfig, pos utils.PositionInfo, key, reason string, count int, since time.Duration) {
	if !am.passAlertCooldown(key + "#close_fail_escalation") {
		return
	}
	roi := "?"
	if pnl, ok := normalizePositionPnl(pos); ok {
		if margin, err := decimal.NewFromString(strings.TrimSpace(pos.UseMargin)); err == nil && margin.IsPositive() {
			roi = pnl.Div(margin).Mul(decimal.NewFromInt(100)).StringFixed(1)
		}
	}
	head := "🆘 平仓连败升级告警"
	detail := fmt.Sprintf("连续失败: %d 次\n首败至今: %s", count, since.Round(time.Second))
	if count == 0 {
		head = "🆘 平仓配置故障告警"
		detail = "配置/数据故障，跳过连败计数直接升级"
	}
	msg := fmt.Sprintf("%s\n⏰ %s\n\n账户: %s\n持仓: %s %s %s张\n当前ROI: %s%%\n原因: %s\n%s\n\n⚠️ 开/平/兜底共用同一Web会话，请立即检查会话有效性",
		head, time.Now().Format("2006-01-02 15:04:05"), acc.Name, pos.InstId, pos.PosSide, pos.Pos, roi, reason, detail)
	logrus.Error(msg)
	if _, err := am.telegramClient.SendMessage(msg); err != nil {
		logrus.Errorf("[升级告警] 发送失败: %v", err)
	}
}

