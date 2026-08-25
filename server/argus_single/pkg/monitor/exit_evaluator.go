package monitor

import "math"

// ExitAction trailing 账户的退出决策。
type ExitAction int

const (
	ActionHold            ExitAction = iota // 继续持有
	ActionTrailingClose                     // 移动止盈线触发，整仓平
	ActionCatastropheStop                   // 兜底止损触发，整仓平
)

// Tier 某档位的激活阈值与回撤比例。
type Tier struct {
	ActivatePct  float64 // A：达到此 ROI% 才启动移动止盈
	GivebackFrac float64 // r：从峰值回撤此比例即平仓（退出线 = peak*(1-r)）
}

// ExitConfig 移动止盈 + 兜底止损的完整参数（分档边界由 N_max 算好后传入）。
type ExitConfig struct {
	SmallMaxContracts  int // 小仓上限张数（含）
	LargeMinContracts  int // 大仓下限张数（含）
	Small              Tier
	Medium             Tier
	Large              Tier
	CatastropheStopPct float64 // S（正数）：pct <= -S 触发兜底止损
}

func (c ExitConfig) tierFor(size int) Tier {
	if size <= c.SmallMaxContracts {
		return c.Small
	}
	if size >= c.LargeMinContracts {
		return c.Large
	}
	return c.Medium
}

// TrailState 单持仓的移动止盈状态（按 账户:instId:posSide 维护）。
type TrailState struct {
	PeakPct  float64 // 激活后的峰值 ROI%
	LastSize int     // 上次见到的张数（检测加仓）
	Active   bool    // 是否已激活移动止盈
}

// EvaluateExit 纯函数：给定当前张数（绝对值）、当前 ROI%、配置与旧状态，
// 返回退出决策与新状态。无任何 I/O，便于表驱动单测。
func EvaluateExit(size int, pnlPct float64, cfg ExitConfig, prev TrailState) (ExitAction, TrailState) {
	tier := cfg.tierFor(size)
	st := prev

	// (1) 加仓 rebase：张数增大 => 均价变、pct 跳变 => 重置峰值；
	//     active 单调（一旦激活保持到平仓，不因加仓稀释而解除已锁保护）。
	if size > st.LastSize {
		st.PeakPct = pnlPct
		st.Active = st.Active || pnlPct >= tier.ActivatePct
	}
	st.LastSize = size

	// (2) 激活
	if !st.Active && pnlPct >= tier.ActivatePct {
		st.Active = true
		st.PeakPct = pnlPct
	}

	// (3) 更新峰值
	if st.Active && pnlPct > st.PeakPct {
		st.PeakPct = pnlPct
	}

	// (4) 兜底止损：独立于分档/激活，优先于移动止盈判定（深度亏损永远标记为 stop）。
	if pnlPct <= -cfg.CatastropheStopPct {
		return ActionCatastropheStop, st
	}

	// (5) 移动止盈：已激活且回撤到退出线 peak*(1-r)。
	if st.Active && pnlPct <= st.PeakPct*(1-tier.GivebackFrac) {
		return ActionTrailingClose, st
	}

	return ActionHold, st
}

// TrailParams 移动止盈的全局可调参数（来自配置：分档比例 + 三档 + 兜底）。
type TrailParams struct {
	TierSmallRatio     float64
	TierLargeRatio     float64
	Small              Tier
	Medium             Tier
	Large              Tier
	CatastropheStopPct float64
}

// BuildExitConfig 由账户的 N_max 与全局参数算出该账户的分档退出配置：
//
//	小仓 ≤ floor(small_ratio*N)，大仓 ≥ ceil(large_ratio*N)，中间为中仓。
func BuildExitConfig(nmax int, p TrailParams) ExitConfig {
	return ExitConfig{
		SmallMaxContracts:  int(math.Floor(p.TierSmallRatio * float64(nmax))),
		LargeMinContracts:  int(math.Ceil(p.TierLargeRatio * float64(nmax))),
		Small:              p.Small,
		Medium:             p.Medium,
		Large:              p.Large,
		CatastropheStopPct: p.CatastropheStopPct,
	}
}

