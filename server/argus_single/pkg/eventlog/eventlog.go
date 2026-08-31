// Package eventlog 提供结构化交易事件的 JSONL 落盘与聚合，作为对比报告的单一数据源。
// 它是 leaf 包（不依赖 trade/monitor），可被两者共同引用，无循环依赖。
package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Event 一条结构化交易事件（每行一条 JSON）。
type Event struct {
	Ts        string  `json:"ts"`
	Account   string  `json:"account"`
	Variant   string  `json:"variant,omitempty"`
	InstId    string  `json:"instId,omitempty"`
	Event     string  `json:"event"` // open/cap_skip/gate_block/trailing_close/catastrophe_stop/loss_alert/fixed_close/balance
	Side      string  `json:"side,omitempty"`
	NetSide   string  `json:"netSide,omitempty"`
	Size      int     `json:"size,omitempty"`
	AvgPx     float64 `json:"avgPx,omitempty"`
	LastPx    float64 `json:"lastPx,omitempty"`
	RoiPct    float64 `json:"roiPct,omitempty"`
	OrderSize int     `json:"orderSize,omitempty"`
	Pnl       float64 `json:"pnl,omitempty"`
	Reason    string  `json:"reason,omitempty"`
	Balance   float64 `json:"balance,omitempty"`
	PeakPct   float64 `json:"peakPct,omitempty"` // 平仓时 trail 峰值（激活过才有，P2-B 审计字段）
	Equity    float64 `json:"equity,omitempty"`  // balance+未实现盈亏（UPL 缓存≤5s 滞后，P2-C）
	Upl       float64 `json:"upl,omitempty"`     // 未实现盈亏合计（可自校验 equity−balance）
	// EquityKnown equity 有效标记：0/负权益也是已知样本（恰是最极端回撤，omitempty
	// 会省略 0 值，正数判断则丢负值）；老日志无此字段，聚合回退 Equity>0 判断。
	EquityKnown bool `json:"equityKnown,omitempty"`
	// 信号报价快照（P7）：信号源 = DeepCoin last-vs-mark 偏离，落在 open/cap_skip/
	// gate_block 三类信号事件上。GapBp 带符号（>0=UP），是"信号分级"研究的入口数据。
	SigLast float64 `json:"sigLast,omitempty"`
	SigMark float64 `json:"sigMark,omitempty"`
	GapBp   float64 `json:"gapBp,omitempty"`
	// 无条件偏离采样（dev_sample 事件，阶段①）：不经 signal_threshold 过滤的
	// last-vs-mark 偏离分布。P7 的 GapBp 只在信号触发时落盘，被 5bp 阈值结构性
	// 截断（实测 min=5.01bp），看不到分布主体，无法回答"阈值降到 θ 后 λ 是多少"。
	//
	// DevTicks 同时是已知性标记：它非零即表示采样器在工作，此时 DevCross/DevOver
	// 的缺失就是真零而非字段未上线（omitempty 会省略零值 map，与 EquityKnown 同类处理）。
	DevTicks  int            `json:"devTicks,omitempty"`
	DevCross  map[string]int `json:"devCross,omitempty"`  // 各候选阈值(bp)的穿越次数 → λ(θ)
	DevOver   map[string]int `json:"devOver,omitempty"`   // 各候选阈值(bp)的超阈 tick 数 → P(|dev|>θ)
	DevMaxBp  float64        `json:"devMaxBp,omitempty"`  // 窗口内 |dev| 极值
	DevMeanBp float64        `json:"devMeanBp,omitempty"` // 窗口内 |dev| 均值
	// 趋势闸（trend_skip 事件，8/21 事故补丁）：拦截时刻的窗口动量（%，带符号）。
	TrendMomPct float64 `json:"trendMomPct,omitempty"`
}

// 事件类型常量
const (
	EvOpen            = "open"
	EvCapSkip         = "cap_skip"
	EvGateBlock       = "gate_block"
	EvTrailingClose   = "trailing_close"
	EvCatastropheStop = "catastrophe_stop"
	EvLossAlert       = "loss_alert"
	EvFixedClose      = "fixed_close"
	EvBalance         = "balance"
	EvExternalClose   = "external_close" // 非本机器人平仓（快照对账识别，P2-A）
	EvManualClose     = "manual_close"   // TG 一键/方向平仓（P2-A）
	EvDevSample       = "dev_sample"     // 无条件偏离采样（市场侧，与账户无关，阶段①）
	EvTrendSkip       = "trend_skip"     // 趋势闸拦截逆势开/加仓（8/21 事故补丁）
)

// Marshal 把事件序列化为一行 JSONL（含换行）。纯函数，可测。
func Marshal(e Event) string {
	b, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(b) + "\n"
}

// Logger 线程安全地把事件追加到按日期分文件的 JSONL。写盘失败只记 error，不影响主流程。
type Logger struct {
	mu  sync.Mutex
	dir string
}

func New(dir string) *Logger { return &Logger{dir: dir} }

func (l *Logger) filePath(now time.Time) string {
	return filepath.Join(l.dir, fmt.Sprintf("events-%s.jsonl", now.Format("2006-01-02")))
}

// Log 追加一条事件；Ts 为空时填当前时间。写盘失败仅告警。
func (l *Logger) Log(e Event) {
	now := time.Now()
	if e.Ts == "" {
		e.Ts = now.Format("2006-01-02 15:04:05")
	}
	line := Marshal(e)
	if line == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		logrus.Errorf("[eventlog] mkdir 失败（不影响交易）: %v", err)
		return
	}
	f, err := os.OpenFile(l.filePath(now), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logrus.Errorf("[eventlog] 打开文件失败（不影响交易）: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		logrus.Errorf("[eventlog] 写入失败（不影响交易）: %v", err)
	}
}

// ParseFile 读取一个 JSONL 文件为事件切片（跳过坏行）。
func ParseFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

const tsLayout = "2006-01-02 15:04:05"

// inWindow 判断事件时间戳是否落在 [since, until]（含端点）；解析失败返回 false。
func inWindow(ts string, since, until time.Time) bool {
	t, err := time.ParseInLocation(tsLayout, ts, time.Local)
	if err != nil {
		return false
	}
	return !t.Before(since) && !t.After(until)
}

// LoadWindow 读取 [since, until] 覆盖到的按日期分文件（缺失文件忽略），
// 返回时间戳落在窗口内的事件（按文件内时间顺序）。
func LoadWindow(dir string, since, until time.Time) []Event {
	var out []Event
	day := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	for !day.After(until) {
		path := filepath.Join(dir, fmt.Sprintf("events-%s.jsonl", day.Format("2006-01-02")))
		if ev, err := ParseFile(path); err == nil {
			for _, e := range ev {
				if inWindow(e.Ts, since, until) {
					out = append(out, e)
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// 包级默认 logger（供 trade/monitor 直接埋点，无需层层传递）。
var (
	defaultLogger *Logger
	defaultMu     sync.RWMutex
)

func Init(dir string) {
	defaultMu.Lock()
	defaultLogger = New(dir)
	defaultMu.Unlock()
}

// Log 用默认 logger 记录事件；未 Init 时静默忽略（不影响交易）。
func Log(e Event) {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	if l != nil {
		l.Log(e)
	}
}
