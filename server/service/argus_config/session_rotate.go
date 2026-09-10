package argus_config

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	commonRedis "common/middleware/redis"
	"service/argus_config/repository"

	"gorm.io/gorm"
)

// 轮换动作。放在这里而不是用裸字符串，是因为 CLI 要按动作分组打印。
const (
	RotateActionRotate    = "rotate"    // 要写：凭证有变化
	RotateActionUnchanged = "unchanged" // 传进来的和库里一样，不写
	RotateActionMissing   = "missing"   // session.json 没给这个账户，保持原样
)

// SessionRotateItem 一个账户的轮换结论。
//
// 明文凭证只放在未导出字段里，包外拿不到；导出的只有长度与时间，够运维核对
// “这次换的是哪一份、比原来新多久”。
type SessionRotateItem struct {
	AccountID   uint64
	AccountName string
	UID         string
	Action      string
	CookieLen   int
	TokenLen    int
	// PrevSessionUpdatedAt 零值表示这个账户此前没有会话行。
	PrevSessionUpdatedAt time.Time
	NextSessionUpdatedAt time.Time

	// sessionRowID 为 0 表示要插入新行而不是更新。
	sessionRowID int
	next         importSession
}

// String 是给终端和日志用的一行摘要，**只输出长度，绝不输出明文**。
func (i SessionRotateItem) String() string {
	prev := "无"
	if !i.PrevSessionUpdatedAt.IsZero() {
		prev = i.PrevSessionUpdatedAt.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("account=%d(%s) uid=%s %s cookie=%d字 token=%d字 原会话=%s 新会话=%s",
		i.AccountID, i.AccountName, i.UID, i.Action, i.CookieLen, i.TokenLen,
		prev, i.NextSessionUpdatedAt.Format("2006-01-02 15:04:05"))
}

// SessionRotatePlan 一次轮换的完整计划。先算清楚再落库，dry run 与 apply 共用同一份。
type SessionRotatePlan struct {
	InstanceKey string
	Version     uint64
	Items       []SessionRotateItem
}

// RotateCount 真正要写的行数。为 0 时 apply 是空操作。
func (p SessionRotatePlan) RotateCount() int {
	count := 0
	for _, item := range p.Items {
		if item.Action == RotateActionRotate {
			count++
		}
	}
	return count
}

// LoadRotateSessionFile 读一份 session.json。文件名校验与导入工具同一套，
// 免得把随便一个 json 当会话文件灌进去。
func LoadRotateSessionFile(path string) (map[string]importSession, error) {
	if err := validateImportFile(path, mainSessionFilename); err != nil {
		return nil, err
	}
	sessions, err := readSessionFile(path)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("session.json 里没有任何账户")
	}
	return sessions, nil
}

// planSessionRotate 纯函数：给定已发布账户、现有会话行与新会话，算出要改哪些行。
//
// 匹配规则与 importer 的 findSession **故意不同**：这里绝不按 url 匹配。
// 同一实例下所有账户的 url 都是同一个占位值（http://localhost:8899，生产上根本
// 没有进程监听它，只用来过 trade/config.go 的非空校验），按它匹配会把一个账户的
// cookie 写到另一个账户上 —— 两个账户拿同一套 web 凭证下单，是这条链路上后果
// 最严重的错误，而且事后极难从现象反推。
//
// 所以只认两条：uid 双方都有且相等（最强），或 accountName 完全相同。
// 名字对上而 uid 冲突时整批报错，绝不猜。
func planSessionRotate(accounts []*repository.ArgusAccount, sessions []*repository.ArgusRuntimeSession,
	incoming map[string]importSession) (SessionRotatePlan, error) {
	if len(accounts) == 0 {
		return SessionRotatePlan{}, fmt.Errorf("该实例的已发布版本里没有账户，先确认实例键与发布状态")
	}

	sessionByAccount := make(map[uint64]*repository.ArgusRuntimeSession, len(sessions))
	for _, row := range sessions {
		sessionByAccount[row.AccountID] = row
	}

	// map 遍历顺序随机，排序后再匹配，保证同样的输入得到同样的计划。
	keys := make([]string, 0, len(incoming))
	for key := range incoming {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	consumed := make(map[string]bool, len(incoming))
	plan := SessionRotatePlan{Items: make([]SessionRotateItem, 0, len(accounts))}

	for _, acc := range accounts {
		accountID := uint64(acc.Id)
		item := SessionRotateItem{
			AccountID:   accountID,
			AccountName: acc.AccountName,
			UID:         acc.UID,
			Action:      RotateActionMissing,
		}
		if existing := sessionByAccount[accountID]; existing != nil {
			item.sessionRowID = existing.Id
			item.PrevSessionUpdatedAt = existing.SessionUpdatedAt
		}

		matchedKey, matched, err := matchIncoming(acc, incoming, keys, consumed)
		if err != nil {
			return SessionRotatePlan{}, err
		}
		if matchedKey == "" {
			plan.Items = append(plan.Items, item)
			continue
		}
		consumed[matchedKey] = true

		cookie := strings.TrimSpace(matched.Cookie)
		token := strings.TrimSpace(matched.Token)
		// 缺一半就整批拒绝。只写 cookie 不写 token（或反之）会让实例带着半套
		// 凭证跑，表现是下单一直失败而配置看上去"已更新"。
		if cookie == "" || token == "" {
			return SessionRotatePlan{}, fmt.Errorf("账户 %s 的新会话缺 cookie 或 token（cookie=%d字 token=%d字），整批不写",
				acc.AccountName, len(cookie), len(token))
		}
		matched.Cookie, matched.Token = cookie, token
		matched.OToken = strings.TrimSpace(matched.OToken)

		item.next = matched
		item.CookieLen = len(cookie)
		item.TokenLen = len(token)
		item.NextSessionUpdatedAt = parseSessionUpdatedAt(matched.UpdatedAt)

		existing := sessionByAccount[accountID]
		if existing != nil && existing.Cookie == cookie && existing.Token == token && existing.OToken == matched.OToken {
			item.Action = RotateActionUnchanged
		} else {
			item.Action = RotateActionRotate
		}
		plan.Items = append(plan.Items, item)
	}

	// 有没被认领的条目，说明文件拿错了或实例选错了。与 importer 同一条守卫：
	// 静默忽略等于让人以为换了、其实没换。
	var stray []string
	for _, key := range keys {
		if !consumed[key] {
			stray = append(stray, key)
		}
	}
	if len(stray) > 0 {
		return SessionRotatePlan{}, fmt.Errorf("session.json 里这些账户不属于本实例：%s（核对 --instance 与文件是否配套）",
			strings.Join(stray, "、"))
	}
	return plan, nil
}

// matchIncoming 找这个账户对应的新会话。uid 命中优先于名字命中。
func matchIncoming(acc *repository.ArgusAccount, incoming map[string]importSession, keys []string,
	consumed map[string]bool) (string, importSession, error) {
	accUID := strings.TrimSpace(acc.UID)
	accName := strings.TrimSpace(acc.AccountName)

	var nameKey string
	var nameHit importSession
	for _, key := range keys {
		if consumed[key] {
			continue
		}
		candidate := incoming[key]
		candUID := strings.TrimSpace(candidate.UID)
		candName := strings.TrimSpace(candidate.AccountName)

		if accUID != "" && candUID != "" && accUID == candUID {
			return key, candidate, nil
		}
		if candName != "" && candName == accName {
			// 名字一样但 uid 对不上：这两个"账户 A"不是同一个平台账户。
			if accUID != "" && candUID != "" && accUID != candUID {
				return "", importSession{}, fmt.Errorf("账户 %s 名字对得上但 uid 冲突（库里 %s，session.json 里 %s），拒绝轮换",
					acc.AccountName, accUID, candUID)
			}
			nameKey, nameHit = key, candidate
		}
	}
	return nameKey, nameHit, nil
}

// parseSessionUpdatedAt 解析 session.json 的 updatedAt；给不出就用当前时间。
func parseSessionUpdatedAt(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed
	}
	return time.Now().UTC()
}

// PlanSessionRotate 读实例当前**已发布**版本的账户与会话，算出轮换计划。
//
// 为什么钉在已发布版本：进程读的就是它（loadPublishedSnapshot），改草稿版本的
// 会话行对正在跑的实例毫无影响。
func (s *ArgusConfigService) PlanSessionRotate(ctx context.Context, instanceKey string,
	incoming map[string]importSession) (*SessionRotatePlan, error) {
	resolvedKey, err := NormalizeInstanceKey(instanceKey)
	if err != nil {
		return nil, err
	}
	version, err := s.repository.FindPublishedContext(ctx, resolvedKey)
	// 没有已发布版本时仓储返回的是 gorm.ErrRecordNotFound，不是 (nil, nil)——
	// 不显式认它，运维只会看到一句裸的 "record not found"，看不出该去导配置。
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && version == nil) {
		return nil, fmt.Errorf("实例 %s 没有已发布配置（先用 argus-config-import 导入并发布）", resolvedKey)
	}
	if err != nil {
		return nil, fmt.Errorf("查该实例的已发布版本失败：%w", err)
	}
	// 往下一律用**库里那一行自己的**实例键，而不是命令行传进来的写法。
	// MySQL 的默认排序规则不区分大小写，`--instance ARGUS-SINGLE-FLY` 照样能查到行；
	// 但 Redis 控制消息在进程侧是 Go 字符串精确比较（runtime.go: message.InstanceID
	// != m.instanceID），拿用户的大写形式去发通知会被静默忽略，表现为"轮换成功但
	// 一直不生效"。
	resolvedKey = strings.TrimSpace(version.InstanceKey)
	accounts, sessions, err := s.repository.LoadAccountsWithSessions(ctx, uint64(version.Id))
	if err != nil {
		return nil, fmt.Errorf("读该版本的账户与会话失败：%w", err)
	}
	plan, err := planSessionRotate(accounts, sessions, incoming)
	if err != nil {
		return nil, err
	}
	plan.InstanceKey = resolvedKey
	plan.Version = version.Version
	return &plan, nil
}

// ApplySessionRotate 落库，返回实际写了几行。
//
// 只动会话列，**不新建配置版本**：版本号代表参数集，轮换凭证不是改参数。
// 进程侧靠内容指纹发现变更（fingerprint 覆盖 trade 配置里的 cookie/token），
// 所以改完这些行，最迟一个轮询周期（60 秒）就会热加载；要立刻生效用 NotifyReload。
func (s *ArgusConfigService) ApplySessionRotate(ctx context.Context, plan *SessionRotatePlan, actor string) (int, error) {
	if plan == nil {
		return 0, fmt.Errorf("轮换计划为空")
	}
	written := 0
	for _, item := range plan.Items {
		if item.Action != RotateActionRotate {
			continue
		}
		row := repository.ArgusRuntimeSession{
			AccountID:        item.AccountID,
			Cookie:           item.next.Cookie,
			Token:            item.next.Token,
			OToken:           item.next.OToken,
			SentryRelease:    strings.TrimSpace(item.next.SentryRelease),
			SentryPublicKey:  strings.TrimSpace(item.next.SentryPublicKey),
			Baggage:          strings.TrimSpace(item.next.Baggage),
			LoginURL:         strings.TrimSpace(item.next.LoginURL),
			FinalURL:         strings.TrimSpace(item.next.FinalURL),
			Valid:            1,
			SessionUpdatedAt: item.NextSessionUpdatedAt,
		}
		row.Id = item.sessionRowID
		if err := s.repository.UpsertRuntimeSession(ctx, &row, actor); err != nil {
			// 中途失败就地返回：已写的行是好的（每行都是完整的一套凭证），
			// 没写的保持原样，不存在半套凭证的账户。
			return written, fmt.Errorf("写账户 %d 的会话失败：%w", item.AccountID, err)
		}
		written++
	}
	return written, nil
}

// NotifyReload 让目标实例立刻重读配置，而不是等下一个轮询周期。
//
// 它不会**额外**触发一次热加载：指纹已经变了，进程本来就会在 60 秒内重载一次，
// 这里只是把那一次提前。失败不算致命——轮询兜底仍然会生效。
func (s *ArgusConfigService) NotifyReload(ctx context.Context, instanceKey string) error {
	resolvedKey, err := NormalizeInstanceKey(instanceKey)
	if err != nil {
		return err
	}
	return commonRedis.PublishArgusControl(ctx, "reload", resolvedKey)
}
