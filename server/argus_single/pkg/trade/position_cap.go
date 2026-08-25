package trade

import (
	"fmt"
	"math"
	"sync"

	"github.com/sirupsen/logrus"
)

// CapParams 仓位上限公式的全局风险参数（来自配置）。
type CapParams struct {
	Leverage           int     // L，例如 125
	FaceValue          float64 // 合约面值（币/张），例如 BTC 0.001
	RiskBudgetFraction float64 // f：单次最坏实亏占本金比例，例如 0.20
	CatastropheStopPct float64 // S：兜底止损 ROI%（正数），例如 300
	Ceiling            int     // 绝对天花板；<=0 表示不限

	// 档位比例（仅供 EnsureInit 动态打印档位边界；上限公式不使用，零值时省略打印）
	TierSmallRatio float64
	TierLargeRatio float64
}

// ComputeMaxContracts 计算单账户单合约的仓位上限：
//
//	N_max = floor( min( f*E*L*100 / (face*P*S), ceiling ) )
//
// 任一输入非法（价格/本金/参数 <= 0）或结果 < 1 张时返回 (0, false)，
// 调用方据此 fail-closed（不开仓 / 不分档）。
func ComputeMaxContracts(initialBalance, price float64, p CapParams) (int, bool) {
	if initialBalance <= 0 || price <= 0 ||
		p.Leverage <= 0 || p.FaceValue <= 0 ||
		p.RiskBudgetFraction <= 0 || p.CatastropheStopPct <= 0 {
		return 0, false
	}
	nFormula := (p.RiskBudgetFraction * initialBalance * float64(p.Leverage) * 100) /
		(p.FaceValue * price * p.CatastropheStopPct)
	n := int(math.Floor(nFormula))
	if p.Ceiling > 0 && n > p.Ceiling {
		n = p.Ceiling
	}
	if n < 1 {
		return 0, false
	}
	return n, true
}

// PositionCapGuard 持仓上限守卫：按 account 懒缓存 N_max。
//
// 仅按账户名缓存（不含 instId）：当前单账户单币（BTC、净仓模式），
// 开仓路径用 trade_inst（BTCUSDT）、持仓查询返回 swap inst（BTC-USDT-SWAP），
// 按账户名键避免两侧 key 不一致；多币种需要的是“跨币种本金分配”（另议），
// 届时再扩展为按 (account, symbol) 键。
type PositionCapGuard struct {
	defaultParams   CapParams
	paramsByAccount map[string]CapParams // 账户级参数覆盖（champion/challenger）
	mu              sync.RWMutex
	cache           map[string]int
}

// NewPositionCapGuard 创建守卫。paramsByAccount 为账户级覆盖（可 nil，缺省用 defaultParams）。
func NewPositionCapGuard(defaultParams CapParams, paramsByAccount map[string]CapParams) *PositionCapGuard {
	if paramsByAccount == nil {
		paramsByAccount = map[string]CapParams{}
	}
	return &PositionCapGuard{defaultParams: defaultParams, paramsByAccount: paramsByAccount, cache: make(map[string]int)}
}

func (g *PositionCapGuard) paramsFor(account string) CapParams {
	if p, ok := g.paramsByAccount[account]; ok {
		return p
	}
	return g.defaultParams
}

// EnsureInit 幂等懒初始化：首个成功算出的 N_max 入缓存；
// 输入非法（如价格暂为 0）时不缓存，下次调用自动重试（可自愈瞬时坏价）。
func (g *PositionCapGuard) EnsureInit(account string, initialBalance, price float64) {
	g.mu.RLock()
	_, ok := g.cache[account]
	g.mu.RUnlock()
	if ok {
		return
	}
	params := g.paramsFor(account)
	n, valid := ComputeMaxContracts(initialBalance, price, params)
	if !valid {
		return
	}
	g.mu.Lock()
	if _, exists := g.cache[account]; !exists {
		g.cache[account] = n
		nFormula := (params.RiskBudgetFraction * initialBalance * float64(params.Leverage) * 100) /
			(params.FaceValue * price * params.CatastropheStopPct)
		tiers := ""
		if params.TierSmallRatio > 0 && params.TierLargeRatio > params.TierSmallRatio {
			tiers = fmt.Sprintf(", 档位[小≤%d/大≥%d]",
				int(math.Floor(params.TierSmallRatio*float64(n))), int(math.Ceil(params.TierLargeRatio*float64(n))))
		}
		// P5 动态参数行（与启动静态行成对）：P/N_formula/N_effective/档位边界
		logrus.Infof("[仓位上限] 账户 %s P=%.2f N_formula=%.1f N_effective=%d (E=%.2f, f=%.2f, S=%.0f%%, 天花板=%d)%s",
			account, price, nFormula, n, initialBalance, params.RiskBudgetFraction, params.CatastropheStopPct, params.Ceiling, tiers)
	}
	g.mu.Unlock()
}

// MaxContracts 读缓存的 N_max；第二返回值表示是否已初始化。
func (g *PositionCapGuard) MaxContracts(account string) (int, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.cache[account]
	return n, ok
}

// WouldExceedCap 先自带初始化，再判定 abs(currentSize)+orderSize > N_max。
// currentSize 取绝对值（short 持仓为负张数）。
// 若无法初始化（输入非法），fail-closed 返回 true（跳过本次开仓）。
func (g *PositionCapGuard) WouldExceedCap(account string, initialBalance float64, currentSize, orderSize int, price float64) bool {
	g.EnsureInit(account, initialBalance, price)
	n, ok := g.MaxContracts(account)
	if !ok {
		return true
	}
	cur := currentSize
	if cur < 0 {
		cur = -cur
	}
	return cur+orderSize > n
}
