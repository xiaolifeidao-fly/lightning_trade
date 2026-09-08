package eventstore

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TsLayout JSONL 的时间戳格式（无时区串），与 eventlog 一致。
const TsLayout = "2006-01-02 15:04:05"

// ErrDSNEmpty sqlconn 未配置：不注册 sink，完全退化成只写 JSONL 的行为。
var ErrDSNEmpty = fmt.Errorf("eventstore: sqlconn is empty")

// NormalizeInstrument 把 instId 归一化到 BTCUSDT 口径。
//
// 实测 instId 100% 系统性分裂：信号侧事件（open/cap_skip/gate_block/
// trend_skip/dev_sample）是 BTCUSDT，持仓侧事件（trailing_close/
// catastrophe_stop/external_close/manual_close/loss_alert）是 BTC-USDT-SWAP。
// 入库归一化到 BTCUSDT，同时保留 inst_id_raw 原值——JSONL 真源里是原值，
// 只存归一化值会让对账工具每行都报不匹配。
//
// 不复用 pkg/trade 的 instrumentBase：它归一化到 BTC（剥掉 USDT 后缀），
// 服务于持仓匹配，是另一个用途；两套口径共用一个函数以后必有人改错一边。
func NormalizeInstrument(instId string) string {
	s := strings.ToUpper(strings.TrimSpace(instId))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimSuffix(s, "SWAP")
	s = strings.TrimSuffix(s, "PERP")
	return s
}

// ParseTs 按本地时区解析 JSONL 的时间戳串（秒精度）。
//
// 精度只到秒是刻意的：实测 28.7% 的信号在分钟内触发 ≥2 次、承载 48.6% 的
// 全部触发，所以不能退到分钟；但 JSONL 真源本身就是秒精度串，再补亚秒等于
// 造数据，会让 JSONL ↔ DB 一致性校验永远对不上。
func ParseTs(ts string) (time.Time, error) {
	return time.ParseInLocation(TsLayout, strings.TrimSpace(ts), time.Local)
}

// NormalizeDSN 强制事件写库连接的关键参数，不依赖 properties 里写对。
//
// loc=Local 是必须显式设的（设计文档 §5.3）：go-sql-driver 默认 loc=UTC，
// 不设的话 Go 的 time.Time 会按 UTC 写入，与 JSONL 差 8 小时。这个错误极
// 隐蔽——数据全在、图能画、只是整体偏移，等到有人拿去对账才会发现。
// parseTime=true 让 DATETIME 读回 time.Time；charset 强制 utf8mb4，因为
// account_label 里有中文（现网 properties 写的是 charset=utf8）。
// 三个超时兜住"DB 慢查询不得让写入 goroutine 无限期挂住"。
func NormalizeDSN(raw string) (string, error) {
	dsn := strings.TrimSpace(raw)
	if dsn == "" {
		return "", ErrDSNEmpty
	}
	base, rawQuery := dsn, ""
	// 参数段在库名之后，密码里可能带 ? / @，因此从最后一个 '/' 之后再找 '?'。
	if slash := strings.LastIndex(dsn, "/"); slash >= 0 {
		if q := strings.Index(dsn[slash:], "?"); q >= 0 {
			base, rawQuery = dsn[:slash+q], dsn[slash+q+1:]
		}
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("eventstore: parse sqlconn parameters: %w", err)
	}
	values.Set("parseTime", "true")
	values.Set("loc", "Local")
	values.Set("charset", "utf8mb4")
	for key, fallback := range map[string]string{"timeout": "5s", "readTimeout": "15s", "writeTimeout": "15s"} {
		if values.Get(key) == "" {
			values.Set(key, fallback)
		}
	}
	return base + "?" + values.Encode(), nil
}
