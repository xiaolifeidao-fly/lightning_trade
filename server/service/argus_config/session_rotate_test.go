package argus_config

import (
	"testing"
	"time"

	"common/middleware/db"
	"service/argus_config/repository"
)

// account / session 造数助手，只填参与匹配与比较的字段。
func account(id int, name, uid, url string) *repository.ArgusAccount {
	return &repository.ArgusAccount{
		BaseEntity:  db.BaseEntity{Id: id},
		AccountName: name,
		UID:         uid,
		URL:         url,
		Enabled:     1,
	}
}

func session(accountID uint64, cookie, token string) *repository.ArgusRuntimeSession {
	return &repository.ArgusRuntimeSession{
		BaseEntity:       db.BaseEntity{Id: int(accountID) + 100},
		AccountID:        accountID,
		Cookie:           cookie,
		Token:            token,
		Valid:            1,
		SessionUpdatedAt: time.Date(2026, 8, 20, 18, 7, 18, 0, time.Local),
	}
}

func incoming(name, uid, url, cookie, token string) importSession {
	return importSession{AccountName: name, UID: uid, URL: url, Cookie: cookie, Token: token,
		UpdatedAt: "2026-09-10T16:00:00+08:00"}
}

func itemFor(t *testing.T, plan SessionRotatePlan, accountID uint64) SessionRotateItem {
	t.Helper()
	for _, item := range plan.Items {
		if item.AccountID == accountID {
			return item
		}
	}
	t.Fatalf("计划里没有账户 %d 的条目：%+v", accountID, plan.Items)
	return SessionRotateItem{}
}

// 最重要的一条：**绝不能按 url 匹配**。
//
// 同一实例下所有账户的 url 都是 http://localhost:8899（那是个只用来过非空校验的
// 占位值，生产上根本没人监听）。importer 的 findSession 里有一条 url 相等就算命中
// 的分支，轮换若沿用它，一份只含账户 B 的 session.json 会命中账户 A —— 两个账户
// 拿同一套 web 凭证下单，是这条链路上最严重的错误。
func TestPlanSessionRotateNeverMatchesByURL(t *testing.T) {
	accounts := []*repository.ArgusAccount{
		account(1, "账户A-a@qq.com", "9558450", "http://localhost:8899"),
		account(2, "账户B-b@gmail.com", "9533715", "http://localhost:8899"),
	}
	sessions := []*repository.ArgusRuntimeSession{session(1, "old-a", "tok-a"), session(2, "old-b", "tok-b")}
	// 只给账户 B 的新会话，且 accountName 用的是库里没有的写法，只有 uid 能对上。
	in := map[string]importSession{
		"b@gmail.com": incoming("b@gmail.com", "9533715", "http://localhost:8899", "new-b", "tok-b2"),
	}

	plan, err := planSessionRotate(accounts, sessions, in)
	if err != nil {
		t.Fatalf("planSessionRotate 不该报错：%v", err)
	}
	if got := itemFor(t, plan, 2).Action; got != RotateActionRotate {
		t.Errorf("账户 B 应按 uid 命中并轮换，实际 action=%s", got)
	}
	if got := itemFor(t, plan, 1).Action; got != RotateActionMissing {
		t.Errorf("账户 A 没有对应会话，必须标 missing 而不是被 B 的凭证覆盖，实际 action=%s", got)
	}
}

// 名字对上但 uid 不一致：宁可整批失败，也不能猜。
func TestPlanSessionRotateRejectsUIDConflict(t *testing.T) {
	accounts := []*repository.ArgusAccount{account(1, "账户A-a@qq.com", "9558450", "")}
	sessions := []*repository.ArgusRuntimeSession{session(1, "old-a", "tok-a")}
	in := map[string]importSession{
		"a": incoming("账户A-a@qq.com", "8888888", "", "new-a", "tok-a2"),
	}

	if _, err := planSessionRotate(accounts, sessions, in); err == nil {
		t.Fatal("uid 冲突必须报错，不能按名字硬匹配")
	}
}

// cookie 或 token 缺一个就整批拒绝：只写一半等于让实例带着半套凭证跑。
func TestPlanSessionRotateRefusesIncompleteCredentials(t *testing.T) {
	accounts := []*repository.ArgusAccount{
		account(1, "a", "1", ""),
		account(2, "b", "2", ""),
	}
	sessions := []*repository.ArgusRuntimeSession{session(1, "old-a", "tok-a"), session(2, "old-b", "tok-b")}

	for _, bad := range []importSession{
		incoming("a", "1", "", "", "tok-a2"),
		incoming("a", "1", "", "new-a", ""),
		incoming("a", "1", "", "   ", "tok-a2"),
	} {
		in := map[string]importSession{"a": bad, "b": incoming("b", "2", "", "new-b", "tok-b2")}
		if _, err := planSessionRotate(accounts, sessions, in); err == nil {
			t.Errorf("cookie=%q token=%q 应整批拒绝", bad.Cookie, bad.Token)
		}
	}
}

// session.json 里出现这个实例没有的账户：说明文件拿错了（或者实例选错了），
// 必须报错。沿用 importer 同一条守卫。
func TestPlanSessionRotateRejectsUnknownSessionEntry(t *testing.T) {
	accounts := []*repository.ArgusAccount{account(1, "a", "1", "")}
	sessions := []*repository.ArgusRuntimeSession{session(1, "old-a", "tok-a")}
	in := map[string]importSession{
		"a":       incoming("a", "1", "", "new-a", "tok-a2"),
		"stray":   incoming("stray", "999", "", "c", "t"),
	}

	if _, err := planSessionRotate(accounts, sessions, in); err == nil {
		t.Fatal("session.json 含本实例以外的账户时必须报错")
	}
}

// 传进来的和库里一模一样：标 unchanged 不写。
// 写了不会有错，但每次写都会让指纹之外的审计列变动、并在版本历史里留噪音。
func TestPlanSessionRotateDetectsUnchanged(t *testing.T) {
	accounts := []*repository.ArgusAccount{account(1, "a", "1", "")}
	sessions := []*repository.ArgusRuntimeSession{session(1, "same-cookie", "same-token")}
	in := map[string]importSession{"a": incoming("a", "1", "", "same-cookie", "same-token")}

	plan, err := planSessionRotate(accounts, sessions, in)
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if got := itemFor(t, plan, 1).Action; got != RotateActionUnchanged {
		t.Errorf("凭证未变应标 unchanged，实际 %s", got)
	}
	if plan.RotateCount() != 0 {
		t.Errorf("RotateCount 应为 0，实际 %d", plan.RotateCount())
	}
}

// 账户还没有 session 行（新导入的实例可能就是这样）：也要能轮换，走插入。
func TestPlanSessionRotateHandlesAccountWithoutExistingSession(t *testing.T) {
	accounts := []*repository.ArgusAccount{account(7, "a", "1", "")}
	in := map[string]importSession{"a": incoming("a", "1", "", "new-a", "tok-a2")}

	plan, err := planSessionRotate(accounts, nil, in)
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	item := itemFor(t, plan, 7)
	if item.Action != RotateActionRotate {
		t.Errorf("没有历史会话行时应轮换（插入），实际 %s", item.Action)
	}
	if !item.PrevSessionUpdatedAt.IsZero() {
		t.Errorf("没有历史行时 PrevSessionUpdatedAt 应为零值，实际 %v", item.PrevSessionUpdatedAt)
	}
}

// 实例没有已发布账户：报错而不是静默成功。轮换的前提是"这个实例正在跑某个版本"。
func TestPlanSessionRotateRequiresAccounts(t *testing.T) {
	in := map[string]importSession{"a": incoming("a", "1", "", "c", "t")}
	if _, err := planSessionRotate(nil, nil, in); err == nil {
		t.Fatal("已发布版本里没有账户时必须报错")
	}
}

// 计划里绝不能带明文。CLI 会把计划打到日志/终端，凭证长度够用来核对。
func TestSessionRotateItemExposesLengthsNotSecrets(t *testing.T) {
	accounts := []*repository.ArgusAccount{account(1, "a", "1", "")}
	in := map[string]importSession{"a": incoming("a", "1", "", "12345", "abc")}

	plan, err := planSessionRotate(accounts, nil, in)
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	item := itemFor(t, plan, 1)
	if item.CookieLen != 5 || item.TokenLen != 3 {
		t.Errorf("长度不对：cookie=%d token=%d", item.CookieLen, item.TokenLen)
	}
	if item.String() == "" {
		t.Fatal("String() 不该为空")
	}
	for _, secret := range []string{"12345", "abc"} {
		if contains(item.String(), secret) {
			t.Errorf("String() 泄出了明文 %q：%s", secret, item.String())
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
