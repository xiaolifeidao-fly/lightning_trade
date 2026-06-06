package trade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"common/middleware/vipper"
	"common/utils/pc_trade/user"

	"github.com/sirupsen/logrus"
)

const (
	WebCredentialModeConfig   = "config"
	WebCredentialModePassword = "password"

	DefaultDeepCoinLoginURL  = "https://www.deepcoin.com/turbo/zh/login"
	defaultSessionMaxAgeDays = 5
)

type UserProvider interface {
	GetUser(ctx context.Context) (*user.User, error)
}

type StaticUserProvider struct {
	user *user.User
}

func NewStaticUserProvider(acc AccountConfig) *StaticUserProvider {
	sessionStore := DefaultSessionStore()
	if sessionEntry, ok := sessionStore.Get(acc); ok {
		if ready, ok := sessionDataToUser(acc, sessionEntry); ok {
			return &StaticUserProvider{
				user: user.NewUser(ready.Cookie, ready.Token, ready.SentryRelease, ready.SentryPublicKey),
			}
		}
	}

	return &StaticUserProvider{
		user: user.NewUser(acc.Cookie, acc.Token, acc.SentryRelease, acc.SentryPublicKey),
	}
}

func (p *StaticUserProvider) GetUser(_ context.Context) (*user.User, error) {
	if p.user == nil {
		return nil, fmt.Errorf("静态用户凭证为空")
	}
	return p.user, nil
}

type LoginUserProvider struct {
	acc          AccountConfig
	service      *DeepCoinLoginService
	sessionStore *SessionStore

	mu     sync.RWMutex
	cached *user.User
}

func NewLoginUserProvider(acc AccountConfig, service *DeepCoinLoginService, sessionStore *SessionStore) *LoginUserProvider {
	return &LoginUserProvider{
		acc:          acc,
		service:      service,
		sessionStore: sessionStore,
	}
}

func (p *LoginUserProvider) GetUser(ctx context.Context) (*user.User, error) {
	p.mu.RLock()
	if p.cached != nil {
		defer p.mu.RUnlock()
		return p.cached, nil
	}
	p.mu.RUnlock()

	if p.service == nil {
		return nil, fmt.Errorf("DeepCoin 登录服务未初始化")
	}

	// 先用 net-wapi 账户接口检测 session 是否仍然有效，有效则直接复用，跳过 Node 登录
	if p.sessionStore != nil {
		if sessionEntry, ok := p.sessionStore.Get(p.acc); ok && sessionEntry.Cookie != "" {
			maxAgeDays := sessionMaxAgeDays()
			stale, reason := sessionNeedsRefreshByUpdatedAt(sessionEntry, time.Now(), maxAgeDays)
			if stale {
				logrus.Infof("session 已超过刷新周期，强制重新登录 account=%s maxAgeDays=%d reason=%s", p.acc.Name, maxAgeDays, reason)
			} else {
				valid, err := CheckSessionValidViaWAPI(ctx, sessionEntry)
				if err != nil {
					logrus.Warnf("session 有效性检查出错，将重新登录 account=%s: %v", p.acc.Name, err)
				} else if valid {
					token := firstNonEmpty(sessionEntry.OToken, sessionEntry.Token)
					logrus.Infof("✅ session 有效，跳过 Node 登录 account=%s tokenLen=%d", p.acc.Name, len(token))
					u := user.NewUser(
						sessionEntry.Cookie,
						token,
						firstNonEmpty(sessionEntry.SentryRelease, p.acc.SentryRelease),
						firstNonEmpty(sessionEntry.SentryPublicKey, p.acc.SentryPublicKey),
					)
					p.mu.Lock()
					p.cached = u
					p.mu.Unlock()
					return u, nil
				} else {
					logrus.Infof("session 已失效，重新登录 account=%s", p.acc.Name)
				}
			}
		}
	}

	// cookie 无效或不存在，调用 Node 服务重新登录（Node 侧跳过 dashboard 探测，直接进登录页）
	result, err := p.service.Login(ctx, p.acc)
	if err != nil {
		return nil, err
	}

	token := firstNonEmpty(result.OToken, result.Token)
	u := user.NewUser(
		result.Cookie,
		token,
		firstNonEmpty(result.SentryRelease, p.acc.SentryRelease),
		firstNonEmpty(result.SentryPublicKey, p.acc.SentryPublicKey),
	)
	if p.sessionStore != nil {
		if err := p.sessionStore.SaveFromLoginResult(p.acc, result); err != nil {
			return nil, fmt.Errorf("登录成功但保存 session 失败: %w", err)
		}
	}

	p.mu.Lock()
	p.cached = u
	p.mu.Unlock()

	return u, nil
}

// Invalidate 清除缓存的登录凭证，下次 GetUser 时触发重新登录。
func (p *LoginUserProvider) Invalidate() {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
	logrus.Infof("🔑 登录凭证缓存已清除 account=%s", p.acc.Name)
}

func sessionMaxAgeDays() int {
	days := vipper.GetInt("login.session_max_age_days")
	if days <= 0 {
		return defaultSessionMaxAgeDays
	}
	return days
}

func BuildUserProvider(acc AccountConfig) (UserProvider, error) {
	sessionStore := DefaultSessionStore()

	switch acc.LoginType {
	case "", WebCredentialModeConfig:
		if !acc.HasStaticWebCredentials() {
			if sessionEntry, ok := sessionStore.Get(acc); ok {
				if _, ok := sessionDataToUser(acc, sessionEntry); ok {
					return NewStaticUserProvider(acc), nil
				}
			}
			return nil, fmt.Errorf("账户 %s 缺少静态 Web 凭证(cookie/token)", acc.Name)
		}
		return NewStaticUserProvider(acc), nil
	case WebCredentialModePassword:
		if !acc.HasLoginCredentials() {
			return nil, fmt.Errorf("账户 %s 缺少登录账号或密码", acc.Name)
		}
		return NewLoginUserProvider(acc, NewDeepCoinLoginService(), sessionStore), nil
	default:
		return nil, fmt.Errorf("账户 %s 的 login_type=%s 不支持", acc.Name, acc.LoginType)
	}
}
