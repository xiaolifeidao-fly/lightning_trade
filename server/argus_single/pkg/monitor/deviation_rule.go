package monitor

import "math"

// DefaultSignalThreshold 盘口信号默认阈值（5bp）。配置项
// monitor.symbols.<SYM>.signal_threshold 缺失或非正时回落到它。
const DefaultSignalThreshold = 0.0005

// ResolveSignalThreshold 解析生效阈值：非正值回落到默认 5bp。
// 判定与日志共用它，避免日志显示 0 而实际按 5bp 判定。
func ResolveSignalThreshold(configured float64) float64 {
	if configured <= 0 {
		return DefaultSignalThreshold
	}
	return configured
}

// EvaluateDeviationSignal 由 DeepCoin last-vs-mark 偏离判定是否派发信号，
// 是整个系统信号生成规则的单一来源。
//
// 规则（edge-trigger + in-band re-arm）：
//   - |deviation| < threshold ⇒ 回到带内，清空状态（下次超阈会重新派发）
//   - 超阈且方向与上次派发相同 ⇒ 抑制（同向持续只算一次）
//   - 超阈且方向不同（含由空状态首次派发）⇒ 派发
//
// 反向不需要先回带内即可派发，所以状态必须是方向而非布尔量。in-band re-arm 是
// 6/25 高信号密度（181 次/天 vs 常态 ~40）的机制。
//
// prev 为上次派发的方向（""=未派发过或已回带内）；返回新状态与是否派发。
// 由 handleOrderBookSignal 与 DevSampler 共用——共用而非各自实现，是为了让
// 采样器测出的 λ(θ) 与生产口径不可能漂移。
func EvaluateDeviationSignal(deviation, threshold float64, prev SignalDirection) (SignalDirection, bool) {
	threshold = ResolveSignalThreshold(threshold)
	if math.Abs(deviation) < threshold {
		return "", false
	}
	direction := SignalDirectionUp
	if deviation < 0 {
		direction = SignalDirectionDown
	}
	if prev == direction {
		return prev, false
	}
	return direction, true
}
