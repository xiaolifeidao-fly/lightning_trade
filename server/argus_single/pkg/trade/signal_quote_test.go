package trade

import (
	"math"
	"testing"

	"argus_single/pkg/eventlog"
)

// 信号报价快照（P7）：信号源就是 DeepCoin last-vs-mark 偏离，
// 但这个偏离幅度此前从未落盘——只在 price_monitor 的日志里以字符串出现过。
// gap_bp 是后续"信号分级"研究的唯一入口数据。
func TestNewSignalQuote(t *testing.T) {
	tests := []struct {
		name       string
		last, mark float64
		wantGapBp  float64
		wantOK     bool
	}{
		// signal_threshold=0.0005 → 触发信号的最小 gap 恰为 5bp
		{"UP 方向恰在阈值上", 65032.5, 65000, 5.0, true},
		{"DOWN 方向为负", 64967.5, 65000, -5.0, true},
		{"大偏离", 65650, 65000, 100.0, true},
		{"零偏离", 65000, 65000, 0, true},
		{"mark 为零不可算", 65000, 0, 0, false},
		{"mark 为负不可算", 65000, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewSignalQuote(tt.last, tt.mark)
			if q.OK() != tt.wantOK {
				t.Fatalf("OK()=%v want %v (%+v)", q.OK(), tt.wantOK, q)
			}
			if !tt.wantOK {
				return
			}
			if math.Abs(q.GapBp-tt.wantGapBp) > 1e-9 {
				t.Fatalf("GapBp=%v want %v", q.GapBp, tt.wantGapBp)
			}
			if q.Last != tt.last || q.Mark != tt.mark {
				t.Fatalf("原始观测须原样保留: %+v", q)
			}
		})
	}
}

// 三类信号事件（open/cap_skip/gate_block）共同构成信号流，
// 分级研究要看"被拦截信号"的 gap 分布，故三者都必须带上。
func TestApplySignalQuote(t *testing.T) {
	q := NewSignalQuote(65032.5, 65000)
	for _, name := range []string{"open", "cap_skip", "gate_block"} {
		t.Run(name, func(t *testing.T) {
			e := applySignalQuote(newTestEvent(name), q)
			if e.SigLast != 65032.5 || e.SigMark != 65000 {
				t.Fatalf("原始 last/mark 须落盘(可自校验 gapBp): %+v", e)
			}
			if math.Abs(e.GapBp-5.0) > 1e-9 {
				t.Fatalf("GapBp: %+v", e)
			}
		})
	}
	// 不可算时三个字段一律省略，不得落 0 冒充真实观测
	bad := applySignalQuote(newTestEvent("open"), NewSignalQuote(65000, 0))
	if bad.SigLast != 0 || bad.SigMark != 0 || bad.GapBp != 0 {
		t.Fatalf("mark 不可用时须整体省略: %+v", bad)
	}
}

func newTestEvent(kind string) eventlog.Event {
	return eventlog.Event{Account: "账户A", InstId: "BTC-USDT-SWAP", Event: kind, Side: "long"}
}

