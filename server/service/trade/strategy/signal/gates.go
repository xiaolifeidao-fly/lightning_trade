package signal

import (
	"fmt"
	"math"
	"sort"

	argusTrade "argus_single/pkg/trade"
)

// 本文件是自动寻优的**判定层**：预注册三关、无解分支、权衡前沿、支配关系。
//
// 为什么判定要和搜索分开、且阈值必须"发起时锁定"：搜索空间只有几十格，跑完看
// 结果再定阈值，等于用同一份数据既定标准又选参数——那样任何一格都能"通过"。
// 阈值随任务落库并带锁定时刻，跑完不接受修改（改阈值 = 建一个新任务），
// 这是本任务唯一的防过拟合机械保障。
//
// 三关（设计文档 §5 第 2 条 / §10.2）：
//
//	① 符号一致率 ≥ SignMin（缺省 0.80）—— 中位数为正但一致率 0.56 的格子是噪声
//	② 熊市月净 p10 ≥ BearNetP10Min（缺省 −B_month×E）—— 频率预算按熊市月情景验收
//	③ p90 MTM 回撤 ≤ MaxDrawdownMax（缺省 DD_max×E）
//
// 另有一个**参考项**（不计入三关）：λ_bear × 单次损失 ≤ B_month。它与 ② 是同一个
// 预算约束的两种表达（毛频率 vs 净收益）。金标准 study_fine.csv 的 10 格 ok_budget
// 全为 False，而 §10.2 定的三关用一致率替换了它——这里如实保留两者，判定只用三关。

// CapUnbounded 搜索空间里的"∞ 上限"哨兵值，口径同金标准脚本的 cap=999。
// 它只作对照（预期违反回撤关），不是一个可上线的配置。
const CapUnbounded = 999

// Gates 预注册的判定阈值。
type Gates struct {
	// RiskEquity 风险本金 E：两个绝对阈值都由它派生。
	RiskEquity float64 `json:"riskEquity"`
	// BearBudgetPct 月度兜底预算 B_month，占本金百分比，缺省 15。
	BearBudgetPct float64 `json:"bearBudgetPct"`
	// DdMaxPct 含浮亏的 MTM 回撤上限，占本金百分比，缺省 25。
	DdMaxPct float64 `json:"ddMaxPct"`
	// SignMin 符号一致率下限，缺省 0.80。
	SignMin float64 `json:"signMin"`

	// BearNetP10Min / MaxDrawdownMax 绝对阈值（USDT）。为零时由上面三项派生；
	// 显式给出则以显式值为准（Derived=false），用于复现历史标准。
	BearNetP10Min  float64 `json:"bearNetP10Min"`
	MaxDrawdownMax float64 `json:"maxDrawdownMax"`
	// StopBudgetMax 参考项的上限（= B_month × E）。
	StopBudgetMax float64 `json:"stopBudgetMax"`
	Derived       bool    `json:"derived"`
}

// DefaultGates 上一轮研究实际用的标准：E=375.73、B_month=15%、DD_max=25%、
// 一致率 0.80 → p10 ≥ −56.36U、p90DD ≤ 93.93U（设计文档 §10.2 记作 −56 / 94）。
func DefaultGates() Gates {
	return Gates{RiskEquity: 375.73, BearBudgetPct: 15, DdMaxPct: 25, SignMin: 0.80}
}

// Normalize 补齐零值并派生绝对阈值。
func (g Gates) Normalize() Gates {
	d := DefaultGates()
	if g.RiskEquity <= 0 {
		g.RiskEquity = d.RiskEquity
	}
	if g.BearBudgetPct <= 0 {
		g.BearBudgetPct = d.BearBudgetPct
	}
	if g.DdMaxPct <= 0 {
		g.DdMaxPct = d.DdMaxPct
	}
	if g.SignMin <= 0 {
		g.SignMin = d.SignMin
	}
	g.StopBudgetMax = g.RiskEquity * g.BearBudgetPct / 100
	// Derived 只在"两个绝对阈值都还是零"时置位，且**只置不清**。
	// Normalize 必须是幂等的：任务把阈值冻结成 JSON 落库，读回来会再
	// Normalize 一次（OOS 任务继承阈值走的就是这条路）。若在这里按
	// "绝对阈值是否为零"重算 Derived，第二次调用会把派生阈值误判成
	// 显式给定，Describe() 里那句"由 E/B_month/DD_max 派生"就变成假的
	// —— 而它正是预注册阈值的审计说明。
	if g.BearNetP10Min == 0 && g.MaxDrawdownMax == 0 {
		g.Derived = true
	}
	if g.BearNetP10Min == 0 {
		g.BearNetP10Min = -g.StopBudgetMax
	}
	if g.MaxDrawdownMax == 0 {
		g.MaxDrawdownMax = g.RiskEquity * g.DdMaxPct / 100
	}
	return g
}

// Describe 三关的可读表述，随任务落库并显示在结果页上——"预注册"要能被看见
// 才有约束力。
func (g Gates) Describe() string {
	g = g.Normalize()
	src := "显式给定"
	if g.Derived {
		src = fmt.Sprintf("由 E=%.2fU / B_month=%.1f%% / DD_max=%.1f%% 派生", g.RiskEquity, g.BearBudgetPct, g.DdMaxPct)
	}
	return fmt.Sprintf("三关（%s）：① 符号一致率 ≥ %.2f ② 熊市月净 p10 ≥ %.1fU ③ p90 MTM 回撤 ≤ %.1fU；参考项：λ_bear×单次损失 ≤ %.1fU",
		src, g.SignMin, g.BearNetP10Min, g.MaxDrawdownMax, g.StopBudgetMax)
}

// CellVerdict 一格的三关判定。
type CellVerdict struct {
	OkSign  bool `json:"okSign"`
	OkBear  bool `json:"okBear"`
	OkDd    bool `json:"okDd"`
	Passed  bool `json:"passed"` // 三关全过
	PassCnt int  `json:"passCnt"`
	// OkStopBudget 参考项，不计入 Passed。
	OkStopBudget bool `json:"okStopBudget"`
	// BearP10 判定用到的熊市月净 p10；BearAvailable=false 表示熊市情景池为空，
	// 该关按**不通过**处理（不可判定不等于通过）。
	BearP10       float64  `json:"bearP10"`
	BearP50       float64  `json:"bearP50"`
	BearAvailable bool     `json:"bearAvailable"`
	Reasons       []string `json:"reasons"`
}

// Judge 对一格做三关判定。判定只读 CellStats，不重新跑回放——阈值锁定 + 统计量
// 冻结，判定就是可复算的纯函数。
func Judge(st CellStats, g Gates) CellVerdict {
	g = g.Normalize()
	v := CellVerdict{}
	v.OkSign = st.SignRatio >= g.SignMin
	if !v.OkSign {
		v.Reasons = append(v.Reasons, fmt.Sprintf("符号一致率 %.2f < %.2f：中位数再高也可能是噪声", st.SignRatio, g.SignMin))
	}
	if bear := st.Scenario(ScenarioBear); bear != nil && bear.PoolSize > 0 {
		v.BearAvailable = true
		v.BearP10, v.BearP50 = bear.P10, bear.P50
		v.OkBear = bear.P10 >= g.BearNetP10Min
		if !v.OkBear {
			v.Reasons = append(v.Reasons, fmt.Sprintf("熊市月净 p10 %.1fU < %.1fU：尾部超出月度兜底预算", bear.P10, g.BearNetP10Min))
		}
	} else {
		v.Reasons = append(v.Reasons, "熊市月情景不可估计（窗口内没有单边日块）：该关按不通过计，不可判定不等于通过")
	}
	v.OkDd = st.P90MaxDrawdown <= g.MaxDrawdownMax
	if !v.OkDd {
		v.Reasons = append(v.Reasons, fmt.Sprintf("p90 MTM 回撤 %.1fU > %.1fU", st.P90MaxDrawdown, g.MaxDrawdownMax))
	}
	v.OkStopBudget = st.StopBudget <= g.StopBudgetMax
	if !v.OkStopBudget {
		v.Reasons = append(v.Reasons, fmt.Sprintf("参考项：λ_bear×单次损失 = %.0fU 超出月度预算 %.0fU（不计入三关，见 §10.2）",
			st.StopBudget, g.StopBudgetMax))
	}
	for _, ok := range []bool{v.OkSign, v.OkBear, v.OkDd} {
		if ok {
			v.PassCnt++
		}
	}
	v.Passed = v.PassCnt == 3
	return v
}

// ─── 搜索空间 ────────────────────────────────────────────────────────────────

// SearchSpace 搜索空间，缺省即设计文档 §3.3 的那张表。
//
// trail 档位刻意**不在**空间里：它有 8 个旋钮，放进来组合数直接爆炸，且
// §3.3 已把它列为二期。signal_threshold 同样不在：改它会把精度降到频率级
// （见 fidelity.go），与事件级格子不可混排比较，混进同一张榜就是误导。
type SearchSpace struct {
	NetCaps  []int     `json:"netCaps"`  // 净仓上限
	DualCaps []int     `json:"dualCaps"` // 双向每侧上限
	StopPcts []float64 `json:"stopPcts"` // S
	GatePcts []float64 `json:"gatePcts"` // gate 副轴
	// IncludeDual 是否把双向形态纳入搜索。实盘形态是净仓，双向只作研究对照
	// （§10.3 第 4 条已复判"维持搁置"），缺省纳入以便复现该结论。
	IncludeDual bool `json:"includeDual"`
	// CoarseGatePct 粗网格固定用的 gate；副轴留到精算阶段再展开，
	// 否则粗网格从 55 格变 110 格却只为了筛掉一半格子。
	CoarseGatePct float64 `json:"coarseGatePct"`
}

// DefaultSearchSpace §3.3 原表：净仓 7 档 × S 5 档 + 双向 4 档 × S 5 档 = 55 格粗网格。
func DefaultSearchSpace() SearchSpace {
	return SearchSpace{
		NetCaps:       []int{10, 15, 20, 26, 32, 40, CapUnbounded},
		DualCaps:      []int{5, 7, 10, 13},
		StopPcts:      []float64{250, 300, 350, 400, 500},
		GatePcts:      []float64{8, 20},
		IncludeDual:   true,
		CoarseGatePct: 20,
	}
}

// Normalize 补齐零值并去重排序，保证同一份空间每次展开出同一串格子。
func (s SearchSpace) Normalize() SearchSpace {
	d := DefaultSearchSpace()
	if len(s.NetCaps) == 0 {
		s.NetCaps = d.NetCaps
	}
	if len(s.StopPcts) == 0 {
		s.StopPcts = d.StopPcts
	}
	if len(s.GatePcts) == 0 {
		s.GatePcts = d.GatePcts
	}
	if s.IncludeDual && len(s.DualCaps) == 0 {
		s.DualCaps = d.DualCaps
	}
	if s.CoarseGatePct <= 0 {
		s.CoarseGatePct = d.CoarseGatePct
	}
	s.NetCaps = dedupInts(s.NetCaps)
	s.DualCaps = dedupInts(s.DualCaps)
	s.StopPcts = dedupFloats(s.StopPcts)
	s.GatePcts = dedupFloats(s.GatePcts)
	return s
}

// CoarseCells 粗网格：mode × cap × S，gate 固定 CoarseGatePct。
func (s SearchSpace) CoarseCells() []CellSpec {
	s = s.Normalize()
	out := make([]CellSpec, 0, (len(s.NetCaps)+len(s.DualCaps))*len(s.StopPcts))
	for _, cap := range s.NetCaps {
		for _, stop := range s.StopPcts {
			out = append(out, CellSpec{Mode: ModeNet, Cap: cap, StopPct: stop, GatePct: s.CoarseGatePct})
		}
	}
	if s.IncludeDual {
		for _, cap := range s.DualCaps {
			for _, stop := range s.StopPcts {
				out = append(out, CellSpec{Mode: ModeDual, Cap: cap, StopPct: stop, GatePct: s.CoarseGatePct})
			}
		}
	}
	return out
}

// Convergence 粗网格 → 精算格的自动收敛规则。
type Convergence struct {
	// TopK 按粗网格中位 PnL 取前 K 格，缺省 10（金标准精算了 10 格）。
	TopK int `json:"topK"`
	// KeepAllPositive 粗网格里全部路径都为正（sign=1）的格子无条件带上，
	// 即使排不进 TopK。这是对"用 4 路径中位数筛格"这件事的补偿：低幅但极稳的
	// 格子恰恰是三关最可能放行的那一类，纯按中位 PnL 排会把它筛掉。
	KeepAllPositive bool `json:"keepAllPositive"`
	// ExpandGateAxis 精算阶段把 gate 副轴展开（每个入选格 × 每个 gate 值）。
	// §10.2 的副轴发现（gate 20→8 让 (26,400) 从 +142.7 到 +168.7）就是这么来的。
	ExpandGateAxis bool `json:"expandGateAxis"`
	// MaxCells 精算格数上限，防止展开后炸开。超限时按粗网格中位 PnL 截断，
	// 并把丢掉了哪些格子写进任务的收敛说明——静默截断会让"覆盖了整个空间"
	// 这句话变成假的。
	MaxCells int `json:"maxCells"`
}

// DefaultConvergence 缺省收敛规则。
func DefaultConvergence() Convergence {
	return Convergence{TopK: 10, KeepAllPositive: true, ExpandGateAxis: true, MaxCells: 40}
}

// Normalize 补齐零值。
func (c Convergence) Normalize() Convergence {
	d := DefaultConvergence()
	if c.TopK <= 0 {
		c.TopK = d.TopK
	}
	if c.MaxCells <= 0 {
		c.MaxCells = d.MaxCells
	}
	return c
}

// ConvergeResult 收敛结果：选中的精算格 + 为什么这么选。
type ConvergeResult struct {
	Cells   []CellSpec `json:"cells"`
	Note    string     `json:"note"`
	Dropped []string   `json:"dropped"` // 因 MaxCells 截断而丢掉的格子
}

// Converge 按粗网格结果自动选出精算格。
//
// **它不是判定**：粗网格只有 4 条路径，符号一致率只有 5 档，分辨不出 0.69 与
// 0.80 的差别；按中位 PnL 排序筛格纯粹是为了省算力。所有判定都在精算阶段用
// 16 条路径 + 预注册三关做。keep 里额外塞进 incumbent（现行配置格），因为
// "现行配置是否被支配"是本任务必须回答的问题，它不能因为排名靠后就消失。
func Converge(coarse []CellStats, space SearchSpace, conv Convergence, incumbent *CellSpec) ConvergeResult {
	space = space.Normalize()
	conv = conv.Normalize()

	ranked := append([]CellStats(nil), coarse...)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].MedPnl28 > ranked[j].MedPnl28 })

	picked := make([]CellSpec, 0, conv.TopK+4)
	seen := map[string]bool{}
	add := func(c CellSpec) {
		if seen[c.Key()] {
			return
		}
		seen[c.Key()] = true
		picked = append(picked, c)
	}
	for i, st := range ranked {
		if i < conv.TopK {
			add(st.Spec)
		}
	}
	reasons := []string{fmt.Sprintf("粗网格 %d 格按中位 PnL 取前 %d 格", len(coarse), conv.TopK)}
	if conv.KeepAllPositive {
		n := 0
		for _, st := range ranked {
			if st.SignRatio >= 1 && !seen[st.Spec.Key()] {
				add(st.Spec)
				n++
			}
		}
		if n > 0 {
			reasons = append(reasons, fmt.Sprintf("另补 %d 格粗网格全路径为正但未进前 %d 名的格子（低幅极稳格最可能过三关，纯按中位排会漏掉）", n, conv.TopK))
		}
	}
	if incumbent != nil {
		if !seen[incumbent.Key()] {
			add(*incumbent)
			reasons = append(reasons, fmt.Sprintf("强制纳入现行配置 %s：「现行配置是否被支配」必须有精算结论，不能因排名靠后而缺席", incumbent.Key()))
		} else {
			reasons = append(reasons, fmt.Sprintf("现行配置 %s 已在入选格中", incumbent.Key()))
		}
	}

	// gate 副轴展开：保持入选顺序，同一格的多个 gate 相邻，便于逐对读副轴效应。
	expanded := picked
	if conv.ExpandGateAxis && len(space.GatePcts) > 1 {
		expanded = make([]CellSpec, 0, len(picked)*len(space.GatePcts))
		for _, c := range picked {
			for _, gate := range space.GatePcts {
				e := c
				e.GatePct = gate
				expanded = append(expanded, e)
			}
		}
		expanded = dedupCells(expanded)
		reasons = append(reasons, fmt.Sprintf("gate 副轴展开为 %v（§10.2 的副轴发现：gate 20→8 让 net(26,400) 的中位从 142.7 抬到 168.7）", space.GatePcts))
	}

	res := ConvergeResult{}
	if len(expanded) > conv.MaxCells {
		for _, c := range expanded[conv.MaxCells:] {
			res.Dropped = append(res.Dropped, c.Key())
		}
		expanded = expanded[:conv.MaxCells]
		reasons = append(reasons, fmt.Sprintf("精算格上限 %d，超出的 %d 格被截断（见 dropped 清单）：这不代表它们更差，只是本次没算",
			conv.MaxCells, len(res.Dropped)))
	}
	res.Cells = expanded
	res.Note = joinNotes(reasons)
	return res
}

// ─── 支配关系与权衡前沿 ──────────────────────────────────────────────────────

// dominationMetrics 一格用于比较的六个维度，全部"越大越好"。
// 回撤与 λ 取负号统一方向。
func dominationMetrics(st CellStats) []float64 {
	bearP10, bearP50 := 0.0, 0.0
	if b := st.Scenario(ScenarioBear); b != nil {
		bearP10, bearP50 = b.P10, b.P50
	}
	return []float64{st.MedPnl28, st.SignRatio, bearP10, bearP50, -st.P90MaxDrawdown, -st.LambdaBearPerMonth}
}

// Dominates a 是否在**全部六个维度上都不差、且至少一个更好**地支配 b。
// 这是"现行 champion 被全面支配"这句话的可计算定义（§10.3 第 2 条）。
func Dominates(a, b CellStats) bool {
	am, bm := dominationMetrics(a), dominationMetrics(b)
	better := false
	for i := range am {
		if am[i] < bm[i]-1e-9 {
			return false
		}
		if am[i] > bm[i]+1e-9 {
			better = true
		}
	}
	return better
}

// FrontierPoint 权衡前沿上的一点。
type FrontierPoint struct {
	Key      string   `json:"key"`
	Spec     CellSpec `json:"spec"`
	MedPnl28 float64  `json:"medPnl28"`
	P90Dd    float64  `json:"p90Dd"`
	BearP10  float64  `json:"bearP10"`
	SignRat  float64  `json:"signRatio"`
	PassCnt  int      `json:"passCnt"`
	// OnDdFrontier 收益/回撤前沿；OnBearFrontier 收益/熊市尾部前沿。
	// 两条都给：回撤是"过程痛感"，熊市 p10 是"预算约束"，本轮真正卡住的是后者。
	OnDdFrontier   bool `json:"onDdFrontier"`
	OnBearFrontier bool `json:"onBearFrontier"`
}

// Frontier 计算权衡前沿。无解分支的核心产出——既然没有格子过三关，能交付的
// 就是"要多少收益得付多少尾部"这张前沿，让人去拍板，而不是替人挑一个。
func Frontier(cells []CellStats, verdicts map[string]CellVerdict) []FrontierPoint {
	pts := make([]FrontierPoint, 0, len(cells))
	for _, st := range cells {
		p := FrontierPoint{Key: st.Key, Spec: st.Spec, MedPnl28: st.MedPnl28,
			P90Dd: st.P90MaxDrawdown, SignRat: st.SignRatio}
		if b := st.Scenario(ScenarioBear); b != nil {
			p.BearP10 = b.P10
		}
		if v, ok := verdicts[st.Key]; ok {
			p.PassCnt = v.PassCnt
		}
		pts = append(pts, p)
	}
	// 收益/回撤前沿：不存在另一格同时"收益不低且回撤不高"（且严格更好）。
	for i := range pts {
		pts[i].OnDdFrontier = !isDominated2(pts, i, func(p FrontierPoint) (float64, float64) {
			return p.MedPnl28, -p.P90Dd
		})
		pts[i].OnBearFrontier = !isDominated2(pts, i, func(p FrontierPoint) (float64, float64) {
			return p.MedPnl28, p.BearP10
		})
	}
	return pts
}

// isDominated2 二维支配判定（两轴都"越大越好"）。
func isDominated2(pts []FrontierPoint, i int, proj func(FrontierPoint) (float64, float64)) bool {
	x, y := proj(pts[i])
	for j := range pts {
		if j == i {
			continue
		}
		ox, oy := proj(pts[j])
		if ox >= x-1e-9 && oy >= y-1e-9 && (ox > x+1e-9 || oy > y+1e-9) {
			return true
		}
	}
	return false
}

// ─── 规模不变性破缺 ──────────────────────────────────────────────────────────

// ScaleInvarianceCheck 一格在给定本金下"能不能真的实例化出这个 cap"。
//
// 为什么必须给：cap 的摊平动态依赖**绝对张数**（§10.3 第 5 条）。small 账户按
// 公式只能实例化 cap≈8，动态更接近 (10,400) 那个平庸格——照抄 cap=26 的结论去
// 小额账户上跑，验证到的根本不是同一件事。这条提示不是免责声明，它决定
// challenger 能验什么、不能验什么。
type ScaleInvarianceCheck struct {
	Key string `json:"key"`
	// FormulaCap 按 N_max = floor(f·E·L·100 / (face·P·S)) 算出的上限。
	FormulaCap int    `json:"formulaCap"`
	CellCap    int    `json:"cellCap"`
	Feasible   bool   `json:"feasible"` // 公式上限 ≥ 格子上限
	Note       string `json:"note"`
}

// CheckScaleInvariance 用基线的 f/face/杠杆 + 给定本金与参考价，核对每格的 cap
// 在该账户上能否真的达到。price 取窗口内首根 1m 收盘（与实盘 cap 懒初始化同源）。
func CheckScaleInvariance(cells []CellStats, base Params, riskEquity, price float64) []ScaleInvarianceCheck {
	base = base.Normalize()
	out := make([]ScaleInvarianceCheck, 0, len(cells))
	for _, st := range cells {
		c := ScaleInvarianceCheck{Key: st.Key, CellCap: st.Spec.Cap}
		n, ok := argusTrade.ComputeMaxContracts(riskEquity, price, argusTrade.CapParams{
			Leverage:           base.Leverage,
			FaceValue:          base.FaceValue,
			RiskBudgetFraction: base.BudgetPct / 100,
			CatastropheStopPct: st.Spec.StopPct,
			Ceiling:            0, // 不设天花板：这里要的就是公式本身能到几张
		})
		if !ok {
			c.Note = fmt.Sprintf("按 E=%.2fU / P=%.1f / S=%.0f 公式算不出 ≥1 张：该账户装不下这一格", riskEquity, price, st.Spec.StopPct)
			out = append(out, c)
			continue
		}
		c.FormulaCap = n
		c.Feasible = n >= st.Spec.Cap
		if c.Feasible {
			c.Note = fmt.Sprintf("E=%.2fU 下公式上限 %d 张 ≥ 本格 %d 张，可原样实例化", riskEquity, n, st.Spec.Cap)
		} else {
			c.Note = fmt.Sprintf("规模不变性破缺：E=%.2fU 下公式上限只有 %d 张，装不下本格 %d 张。摊平动态依赖绝对张数，"+
				"在该账户上只能验证 S / gate 效应的方向性，拿不到完整参数包的收益", riskEquity, n, st.Spec.Cap)
		}
		out = append(out, c)
	}
	return out
}

// ─── 小工具 ──────────────────────────────────────────────────────────────────

func dedupInts(xs []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(xs))
	for _, v := range xs {
		if v < 1 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func dedupFloats(xs []float64) []float64 {
	seen := map[float64]bool{}
	out := make([]float64, 0, len(xs))
	for _, v := range xs {
		if v < 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func dedupCells(xs []CellSpec) []CellSpec {
	seen := map[string]bool{}
	out := make([]CellSpec, 0, len(xs))
	for _, c := range xs {
		if seen[c.Key()] {
			continue
		}
		seen[c.Key()] = true
		out = append(out, c)
	}
	return out
}

func joinNotes(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += " | "
		}
		out += x
	}
	return out
}

// RoundTo 结果落库前的统一舍入（避免 JSON 里出现 1e-17 这种噪声位）。
func RoundTo(v float64, digits int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	f := math.Pow(10, float64(digits))
	return math.Round(v*f) / f
}
