package trade

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// LoginScheduler 按配置的时刻（hour:minute）每天定时刷新所有密码型账户的登录凭证。
// 只对 login_type=password 的账户生效；静态 cookie 账户不处理。
type LoginScheduler struct {
	manager *TradeManager
	hour    int
	minute  int
	stop    chan struct{}
}

func newLoginScheduler(manager *TradeManager, hour, minute int) *LoginScheduler {
	return &LoginScheduler{
		manager: manager,
		hour:    hour,
		minute:  minute,
		stop:    make(chan struct{}),
	}
}

func (s *LoginScheduler) Start() {
	for {
		d := durationUntilDailyTime(s.hour, s.minute)
		logrus.Infof("⏰ 定时登录调度器：下次登录刷新将在 %s 后（目标时刻 %s）",
			d.Round(time.Second),
			time.Now().Add(d).Format("2006-01-02 15:04:05"))

		select {
		case <-time.After(d):
			s.refreshAllLogins()
		case <-s.stop:
			logrus.Info("🛑 定时登录调度器已停止")
			return
		}
	}
}

func (s *LoginScheduler) Stop() {
	close(s.stop)
}

func (s *LoginScheduler) refreshAllLogins() {
	logrus.Info("🔄 定时登录：开始刷新所有密码型账户的登录凭证")

	s.manager.mu.RLock()
	snapshot := make(map[string]*DirectWebClient, len(s.manager.webClients))
	for k, v := range s.manager.webClients {
		snapshot[k] = v
	}
	s.manager.mu.RUnlock()

	refreshed := 0
	for name, client := range snapshot {
		lp, ok := client.userProvider.(*LoginUserProvider)
		if !ok {
			continue // 静态 cookie 账户跳过
		}

		lp.Invalidate()

		ctx, cancel := context.WithTimeout(context.Background(), defaultLoginTimeout+30*time.Second)
		_, err := lp.GetUser(ctx)
		cancel()

		if err != nil {
			logrus.Errorf("❌ 定时登录失败 account=%s: %v", name, err)
		} else {
			logrus.Infof("✅ 定时登录成功 account=%s", name)
			refreshed++
		}
	}

	logrus.Infof("🔄 定时登录完成：%d/%d 个账户刷新成功", refreshed, len(snapshot))
}

// durationUntilDailyTime 计算距离下一次 hour:minute 触发的等待时长。
// 若今天的触发时刻已过，则安排到明天同一时刻。
func durationUntilDailyTime(hour, minute int) time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}
