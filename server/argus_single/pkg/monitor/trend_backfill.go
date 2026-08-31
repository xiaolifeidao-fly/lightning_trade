package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// 趋势闸历史回填（8/21 事故补丁）：重启后趋势闸要等一个完整窗口（默认 24h）
// 才能生效，而重启常发生在故障或剧烈行情之后——恰是最需要闸的时刻。启动时
// 从 DeepCoin 公共 candles API 拉窗口内 1m close 灌进 TrendTracker，闸立即在岗。
// 失败只降级（闸等实时数据满窗后自然生效），绝不阻塞启动、不影响交易。

const trendWarmupBaseURL = "https://api.deepcoin.com"

var trendWarmupHTTP = &http.Client{Timeout: 15 * time.Second}

// fetchTrendWarmupFrom 按 after= 倒序分页拉 [now-window-1m, now] 的 1m K线，
// 返回按时间升序的 close 序列（SeedHistory 的契约）。baseURL 可注入以便测试。
func fetchTrendWarmupFrom(baseURL, instId string, window time.Duration, now time.Time) ([]TrendPoint, error) {
	startMs := now.Add(-window - time.Minute).UnixMilli()
	cursor := now.UnixMilli()
	byMinute := make(map[int64]float64)
	for cursor > startMs {
		url := fmt.Sprintf("%s/deepcoin/market/candles?instId=%s&bar=1m&limit=300&after=%d",
			baseURL, instId, cursor)
		resp, err := trendWarmupHTTP.Get(url)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("candles HTTP %d", resp.StatusCode)
		}
		var body struct {
			Data [][]string `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(body.Data) == 0 {
			break
		}
		prev := cursor
		for _, row := range body.Data {
			if len(row) < 5 {
				continue
			}
			ts, err1 := strconv.ParseInt(row[0], 10, 64)
			px, err2 := strconv.ParseFloat(row[4], 64)
			if err1 != nil || err2 != nil || px <= 0 {
				continue
			}
			byMinute[ts] = px
			if ts < cursor {
				cursor = ts
			}
		}
		if cursor >= prev {
			break // 游标未推进（异常响应），防死循环
		}
	}
	pts := make([]TrendPoint, 0, len(byMinute))
	for ts, px := range byMinute {
		pts = append(pts, TrendPoint{At: time.UnixMilli(ts), Px: px})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].At.Before(pts[j].At) })
	return pts, nil
}

// backfillTrendHistory 启动期异步回填全部 symbol 的趋势窗口。
func (pm *PriceMonitor) backfillTrendHistory() {
	for symbol, cfg := range pm.symbolConfigs {
		pts, err := fetchTrendWarmupFrom(trendWarmupBaseURL, cfg.DeepInst, pm.trendWindow, time.Now())
		if err != nil {
			logrus.Warnf("[趋势闸] %s 历史回填失败（闸将等实时数据满窗后自然生效）: %v", symbol, err)
			continue
		}
		pm.signalMu.Lock()
		tr := pm.trendTrackers[symbol]
		if tr == nil {
			tr = NewTrendTracker(pm.trendWindow)
			pm.trendTrackers[symbol] = tr
		}
		tr.SeedHistory(pts)
		pm.signalMu.Unlock()
		logrus.Infof("[趋势闸] %s 已回填 %d 根 1m close，窗口 %v 立即在岗", symbol, len(pts), pm.trendWindow)
	}
}
