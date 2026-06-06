package trade

import (
	"strings"
	"testing"
	"time"
)

func TestSessionNeedsRefreshByUpdatedAt(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		updatedAt  string
		maxAgeDays int
		wantStale  bool
		wantReason string
	}{
		{
			name:       "未超过默认5天不刷新",
			updatedAt:  now.Add(-4*24*time.Hour - 23*time.Hour).Format(time.RFC3339),
			maxAgeDays: 5,
			wantStale:  false,
			wantReason: "<= maxAge",
		},
		{
			name:       "超过默认5天强制刷新",
			updatedAt:  now.Add(-5*24*time.Hour - time.Second).Format(time.RFC3339),
			maxAgeDays: 5,
			wantStale:  true,
			wantReason: "> maxAge",
		},
		{
			name:       "可配置3天",
			updatedAt:  now.Add(-4 * 24 * time.Hour).Format(time.RFC3339),
			maxAgeDays: 3,
			wantStale:  true,
			wantReason: "> maxAge",
		},
		{
			name:       "缺失updatedAt强制刷新",
			updatedAt:  "",
			maxAgeDays: 5,
			wantStale:  true,
			wantReason: "updatedAt 为空",
		},
		{
			name:       "updatedAt解析失败强制刷新",
			updatedAt:  "not-a-time",
			maxAgeDays: 5,
			wantStale:  true,
			wantReason: "updatedAt 解析失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStale, gotReason := sessionNeedsRefreshByUpdatedAt(
				SessionAccountData{UpdatedAt: tt.updatedAt},
				now,
				tt.maxAgeDays,
			)
			if gotStale != tt.wantStale {
				t.Fatalf("stale = %v, want %v (reason=%s)", gotStale, tt.wantStale, gotReason)
			}
			if !strings.Contains(gotReason, tt.wantReason) {
				t.Fatalf("reason = %q, want contains %q", gotReason, tt.wantReason)
			}
		})
	}
}
