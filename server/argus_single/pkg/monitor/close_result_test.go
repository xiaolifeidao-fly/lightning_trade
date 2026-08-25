package monitor

import (
	"errors"
	"strings"
	"testing"
	"time"

	"common/utils"

	"argus_single/pkg/trade"
)

// 平仓结果三态文案（设计 §4）：成功/降级成功/双失败三者必须一眼可辨——
// 降级态是新增的"仓位已保住但会话待修"，不能和正常成功混为一谈。
func TestCloseResultMessage(t *testing.T) {
	tests := []struct {
		name    string
		out     trade.CloseOutcome
		wantSub []string
		notWant []string
	}{
		{
			name:    "web 成功",
			out:     trade.CloseOutcome{Channel: "web"},
			wantSub: []string{"🟢", "成功"},
			notWant: []string{"降级", "🟡", "失败"},
		},
		{
			name:    "降级到原生通道成功",
			out:     trade.CloseOutcome{Channel: "native", Degraded: true, PrimaryErr: errors.New("GW: Login Timeout")},
			wantSub: []string{"🟡", "成功", "降级", "原生", "cookie"},
			notWant: []string{"🟢", "🔴"},
		},
		{
			name: "双通道失败",
			out: trade.CloseOutcome{
				PrimaryErr: errors.New("GW: Login Timeout"),
				BackupErr:  errors.New("Invalid Sign"),
			},
			wantSub: []string{"🔴", "失败", "Login Timeout", "Invalid Sign"},
			notWant: []string{"🟢", "🟡"},
		},
		{
			name:    "主通道未配置但备用成功: 属正常路径, 不标降级",
			out:     trade.CloseOutcome{Channel: "native"},
			wantSub: []string{"🟢", "成功"},
			notWant: []string{"🟡", "降级"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := closeResultMessage(tt.out)
			for _, w := range tt.wantSub {
				if !strings.Contains(msg, w) {
					t.Fatalf("文案应含 %q, got: %q", w, msg)
				}
			}
			for _, n := range tt.notWant {
				if strings.Contains(msg, n) {
					t.Fatalf("文案不应含 %q, got: %q", n, msg)
				}
			}
		})
	}
}

// TG 手动平仓的展示行（一键/方向平仓共用）。人工救火通道同样受 GW 故障影响——
// 7/23 roc 只能改用交易所 App 正因为它们也走 web，降级态必须在回执里可见。
func TestManualCloseLine(t *testing.T) {
	long := utils.PositionInfo{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "10"}
	short := utils.PositionInfo{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "6"}

	l := manualCloseLine(long, trade.CloseOutcome{Channel: "web"})
	if !strings.Contains(l, "🔵") || !strings.Contains(l, "🟢") || !strings.Contains(l, "10张") {
		t.Fatalf("多头成功行: %q", l)
	}
	s := manualCloseLine(short, trade.CloseOutcome{Channel: "web"})
	if !strings.Contains(s, "🔴 BTC-USDT-SWAP") {
		t.Fatalf("空头方向 emoji: %q", s)
	}
	d := manualCloseLine(short, trade.CloseOutcome{Channel: "native", Degraded: true,
		PrimaryErr: errors.New("GW: Login Timeout")})
	if !strings.Contains(d, "🟡") || !strings.Contains(d, "降级") {
		t.Fatalf("降级行须可辨: %q", d)
	}
	f := manualCloseLine(short, trade.CloseOutcome{
		PrimaryErr: errors.New("GW: Login Timeout"), BackupErr: errors.New("Invalid Sign")})
	if !strings.Contains(f, "失败") || !strings.Contains(f, "Invalid Sign") {
		t.Fatalf("失败行须含双通道原因: %q", f)
	}
}

// 降级告警节流 30 分钟：不紧急（仓位已平）但必须可见，否则 cookie 过期无限期无人处理；
// 5s 轮询下若不节流会刷屏。需与既有 5min 盈亏冷却分离。
func TestPassCooldownCustomDuration(t *testing.T) {
	am := &AccountMonitor{
		lastPnlAlerts:    make(map[string]time.Time),
		pnlAlertCooldown: 5 * time.Minute,
	}
	const k = "账户A:BTC-USDT-SWAP:long#degraded"
	if !am.passCooldown(k, 30*time.Minute) {
		t.Fatalf("首次应放行")
	}
	if am.passCooldown(k, 30*time.Minute) {
		t.Fatalf("30 分钟内应节流")
	}
	// 既有 5min 冷却语义不得被破坏
	if !am.passAlertCooldown("其他key") {
		t.Fatalf("既有冷却首次应放行")
	}
	// 手动回拨到 20 分钟前：5min 口径应放行，30min 口径仍拦
	am.lastPnlAlerts[k] = time.Now().Add(-20 * time.Minute)
	if am.passCooldown(k, 30*time.Minute) {
		t.Fatalf("20 分钟 < 30 分钟, 应仍节流")
	}
	am.lastPnlAlerts[k] = time.Now().Add(-31 * time.Minute)
	if !am.passCooldown(k, 30*time.Minute) {
		t.Fatalf("超过 30 分钟应放行")
	}
}

