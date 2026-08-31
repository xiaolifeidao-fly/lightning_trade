package monitor

import "testing"

// EvaluateDeviationSignal 是信号判定链的单一来源：handleOrderBookSignal 与
// DevSampler 共用它，两者因此不可能漂移——采样器测出的 λ(θ) 与生产在同一口径上，
// 这是阶段② 用采样数据定标阈值的前提。
func TestEvaluateDeviationSignal(t *testing.T) {
	const th = 0.0005 // 5bp

	tests := []struct {
		name      string
		deviation float64
		prev      SignalDirection
		wantNext  SignalDirection
		wantFire  bool
	}{
		{
			name:      "带内：清空状态并 re-arm，即使上次派发过",
			deviation: 0.0001,
			prev:      SignalDirectionUp,
			wantNext:  "",
			wantFire:  false,
		},
		{
			name:      "带内负偏离同样 re-arm",
			deviation: -0.0004,
			prev:      SignalDirectionDown,
			wantNext:  "",
			wantFire:  false,
		},
		{
			name:      "正偏离超阈且此前无状态：派发 UP",
			deviation: 0.0006,
			prev:      "",
			wantNext:  SignalDirectionUp,
			wantFire:  true,
		},
		{
			name:      "负偏离超阈且此前无状态：派发 DOWN",
			deviation: -0.0006,
			prev:      "",
			wantNext:  SignalDirectionDown,
			wantFire:  true,
		},
		{
			name:      "同向持续超阈：抑制，状态保持",
			deviation: 0.0009,
			prev:      SignalDirectionUp,
			wantNext:  SignalDirectionUp,
			wantFire:  false,
		},
		{
			name:      "反向超阈无需先回带内：立即派发",
			deviation: -0.0006,
			prev:      SignalDirectionUp,
			wantNext:  SignalDirectionDown,
			wantFire:  true,
		},
		{
			name:      "恰好等于阈值算超阈（生产用 < 判定带内）",
			deviation: 0.0005,
			prev:      "",
			wantNext:  SignalDirectionUp,
			wantFire:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, fire := EvaluateDeviationSignal(tt.deviation, th, tt.prev)
			if next != tt.wantNext || fire != tt.wantFire {
				t.Errorf("EvaluateDeviationSignal(%v, %v, %q) = (%q, %v), want (%q, %v)",
					tt.deviation, th, tt.prev, next, fire, tt.wantNext, tt.wantFire)
			}
		})
	}
}

// 阈值回落只应有一处实现：判定与日志共用它，否则日志会显示 0 而实际按 5bp 判定。
func TestResolveSignalThreshold(t *testing.T) {
	tests := []struct {
		configured float64
		want       float64
	}{
		{configured: 0.0005, want: 0.0005},
		{configured: 0.0002, want: 0.0002}, // 已下调的阈值必须照用，不得被默认值顶替
		{configured: 0, want: DefaultSignalThreshold},
		{configured: -1, want: DefaultSignalThreshold},
	}
	for _, tt := range tests {
		if got := ResolveSignalThreshold(tt.configured); got != tt.want {
			t.Errorf("ResolveSignalThreshold(%v) = %v, want %v", tt.configured, got, tt.want)
		}
	}
}

// 阈值未配置或非正时回落到 5bp 默认值——这个默认值只应存在一处。
func TestEvaluateDeviationSignalFallsBackToDefaultThreshold(t *testing.T) {
	// 3bp 偏离在默认 5bp 下属带内，不得派发
	if _, fire := EvaluateDeviationSignal(0.0003, 0, ""); fire {
		t.Error("阈值为 0 时应回落到默认 5bp，3bp 偏离不该派发")
	}
	// 6bp 偏离超过默认 5bp，应派发
	if _, fire := EvaluateDeviationSignal(0.0006, -1, ""); !fire {
		t.Error("阈值为负时应回落到默认 5bp，6bp 偏离该派发")
	}
}
