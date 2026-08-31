package monitor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 重启回填：从 DeepCoin 公共 candles API 拉窗口内 1m close 灌进 TrendTracker，
// 让趋势闸在重启后立即在岗（否则要裸奔一个窗口——重启常发生在剧烈行情后，
// 恰是最需要闸的时刻）。失败只降级放行，绝不阻塞启动。

func TestFetchTrendWarmupPaginatesAndParses(t *testing.T) {
	base := time.Date(2026, 8, 21, 10, 0, 0, 0, time.Local)
	toMs := func(dt time.Time) int64 { return dt.UnixMilli() }
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
			t.Errorf("instId 参数错误: %s", r.URL.RawQuery)
		}
		page++
		var rows string
		if page == 1 { // 倒序分页：先给最近 2 根
			rows = fmt.Sprintf(`["%d","63500","63600","63400","63550"],["%d","63400","63500","63300","63450"]`,
				toMs(base.Add(-1*time.Minute)), toMs(base.Add(-2*time.Minute)))
		} else { // 第二页给更早 1 根，之后空页停止
			rows = fmt.Sprintf(`["%d","63300","63400","63200","63350"]`, toMs(base.Add(-3*time.Minute)))
			if page > 2 {
				rows = ""
			}
		}
		fmt.Fprintf(w, `{"code":"0","data":[%s]}`, rows)
	}))
	defer srv.Close()

	pts, err := fetchTrendWarmupFrom(srv.URL, "BTC-USDT-SWAP", 3*time.Minute, base)
	if err != nil {
		t.Fatalf("拉取失败: %v", err)
	}
	if len(pts) != 3 {
		t.Fatalf("应拿到 3 根, got %d: %+v", len(pts), pts)
	}
	for i := 1; i < len(pts); i++ {
		if !pts[i-1].At.Before(pts[i].At) {
			t.Fatalf("必须按时间升序返回（SeedHistory 契约）: %+v", pts)
		}
	}
	if pts[2].Px != 63550 {
		t.Errorf("close 取第 5 列, got %v", pts[2].Px)
	}
}

func TestFetchTrendWarmupErrorIsNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	if _, err := fetchTrendWarmupFrom(srv.URL, "BTC-USDT-SWAP", time.Hour, time.Now()); err == nil {
		t.Fatal("HTTP 500 应返回 error（调用方 warn 后放行）")
	}
}
