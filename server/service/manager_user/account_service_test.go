package user

import (
	"strings"
	"testing"
)

func TestNormalizeAccountStatusAcceptsOnlyWhitelist(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"normal", AccountStatusNormal, false},
		{"frozen", AccountStatusFrozen, false},
		{"  Normal  ", AccountStatusNormal, false}, // 前后空格 + 大小写都要收
		{"FROZEN", AccountStatusFrozen, false},
		{"", "", true},
		{"deleted", "", true}, // 白名单之外一律拒绝，别让任意串写进库
		{"normal;DROP", "", true},
	}
	for _, c := range cases {
		got, err := normalizeAccountStatus(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("normalizeAccountStatus(%q) 应报错，实际得到 %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeAccountStatus(%q) 不该报错：%v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeAccountStatus(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 余额必须走 decimal：列是 decimal(38,8)，用 float64 中转会引入二进制浮点误差，
// 而这里是真钱。这个用例钉住"不丢精度"。
func TestParseBalanceKeepsPrecision(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0", "0.00000000"},
		{"100.00", "100.00000000"},
		{"0.1", "0.10000000"},
		// float64 下 0.1+0.2 = 0.30000000000000004；decimal 必须原样保留
		{"0.30000000", "0.30000000"},
		{"12345678901234567890.12345678", "12345678901234567890.12345678"},
		{"  88.5  ", "88.50000000"},
	}
	for _, c := range cases {
		got, err := parseBalance(c.in)
		if err != nil {
			t.Errorf("parseBalance(%q) 不该报错：%v", c.in, err)
			continue
		}
		if s := got.StringFixed(8); s != c.want {
			t.Errorf("parseBalance(%q) = %s，期望 %s", c.in, s, c.want)
		}
	}
}

func TestParseBalanceRejectsBadInput(t *testing.T) {
	// 负数：前端的充值是「当前余额 + 充值额」算好再发的，出现负数说明上游算错，
	// 静默存进去比报错更糟。
	for _, in := range []string{"", "   ", "abc", "1.2.3", "-1", "-0.00000001", "1e5x"} {
		if got, err := parseBalance(in); err == nil {
			t.Errorf("parseBalance(%q) 应报错，实际得到 %s", in, got.String())
		}
	}
}

// 报错信息要能定位到是哪个字段、收到的是什么，否则前端只看到"参数错误"。
func TestParseBalanceErrorMentionsInput(t *testing.T) {
	_, err := parseBalance("abc")
	if err == nil || !strings.Contains(err.Error(), "abc") {
		t.Fatalf("报错应带上原始输入，实际：%v", err)
	}
	_, err = parseBalance("-5")
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("负数报错应说明原因，实际：%v", err)
	}
}
