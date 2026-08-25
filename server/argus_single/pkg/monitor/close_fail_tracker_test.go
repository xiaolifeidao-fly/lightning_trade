package monitor

import (
	"testing"
	"time"
)

func TestCloseFailTrackerEscalation(t *testing.T) {
	now := time.Now()
	tr := newCloseFailTracker(func() time.Time { return now })

	k := "账户A:BTC-USDT-SWAP:long"
	if c, _ := tr.recordFail(k); c != 1 {
		t.Fatalf("首败 count=1, got %d", c)
	}
	now = now.Add(10 * time.Second)
	if c, _ := tr.recordFail(k); c != 2 {
		t.Fatalf("二败 count=2, got %d", c)
	}
	now = now.Add(10 * time.Second)
	c, since := tr.recordFail(k)
	if c != 3 {
		t.Fatalf("三败 count=3, got %d", c)
	}
	if since < 19*time.Second || since > 21*time.Second {
		t.Fatalf("since 应≈20s（自首败起算）, got %v", since)
	}
	// 成功清零
	tr.recordSuccess(k)
	if c, _ := tr.recordFail(k); c != 1 {
		t.Fatalf("成功后应清零重计, got %d", c)
	}
}

func TestCloseFailTrackerClearMissing(t *testing.T) {
	// R4: 旧仓2败 + 外部平仓重开新仓 → 新仓首败必须是 1 不是 3
	tr := newCloseFailTracker(time.Now)
	k := "账户A:BTC-USDT-SWAP:long"
	tr.recordFail(k)
	tr.recordFail(k)
	// 仓位消失（外部平仓）→ 生命周期清零
	tr.clearMissing("账户A:", map[string]bool{}) // liveKeys 空 = 该账户无存活仓
	if c, _ := tr.recordFail(k); c != 1 {
		t.Fatalf("仓位消失后计数应清零, got %d", c)
	}
	// 其他账户的计数不受影响
	kb := "账户B:BTC-USDT-SWAP:short"
	tr.recordFail(kb)
	tr.clearMissing("账户A:", map[string]bool{})
	if c, _ := tr.recordFail(kb); c != 2 {
		t.Fatalf("clearMissing 不得跨账户清零, got %d", c)
	}
	// 仓位仍存活则保留
	tr.clearMissing("账户B:", map[string]bool{kb: true})
	if c, _ := tr.recordFail(kb); c != 3 {
		t.Fatalf("存活仓位计数应保留, got %d", c)
	}
}

