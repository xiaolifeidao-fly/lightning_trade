package signal

import (
	"fmt"
	"sort"
)

// 结论层：把一批精算格 + 预注册三关判定，变成一份**不含"最优参数"**的结论。
//
// 这条约束是需求级的（需求大纲 §2.2 与本任务的「不做」）：自动寻优不输出最优
// 参数、不自动改线上参数。原因不是谨慎，是统计事实——16 路径 ensemble 下，
// 现行 champion 的符号一致率 0.56、脊线格的熊市尾部 p10 达 −113U（≈30% 权益）。
// "最优"要么是在噪声里挑第一名，要么是替人做了尾部风险的取舍。所以结论只有
// 两种形态：
//
//	候选（三关有格子通过）→ 列出候选与各自的 f*、代价、OOS 验证要求
//	无解（一格没过）      → 出权衡前沿 + 被支配的现行配置 + 结构性方案立项建议
const (
	VerdictCandidateFound = "candidate_found"
	VerdictNoSolution     = "no_solution"
)

// Conclusion 一次寻优的结论。
type Conclusion struct {
	Verdict     string   `json:"verdict"`
	PassedCells []string `json:"passedCells"`
	Headline    string   `json:"headline"`
	// Lines 逐条结论。刻意是"一串陈述"而不是一段自由文本：每条都要能对上一个
	// 可核对的数字，方便页面逐条展示、也方便测试逐条断言。
	Lines []string `json:"lines"`
	// Incumbent 现行配置格及其是否被支配（§10.3 第 2 条那句"被全面支配"的可计算版本）。
	IncumbentKey       string   `json:"incumbentKey"`
	IncumbentDominated bool     `json:"incumbentDominated"`
	DominatorKeys      []string `json:"dominatorKeys"`
	// Frontier 权衡前沿（无解分支的主产出，有解时同样给出，用于看"多付多少尾部换多少收益"）。
	Frontier []FrontierPoint `json:"frontier"`
	// DominatedBy 每格被哪些格子支配。
	DominatedBy map[string][]string `json:"dominatedBy"`
}

// Conclude 生成结论。cells 必须是**精算阶段**的格子：粗网格只有 4 条路径，
// 一致率分辨率不够，不允许参与判定。
func Conclude(cells []CellStats, verdicts map[string]CellVerdict, gates Gates, incumbent *CellSpec) Conclusion {
	gates = gates.Normalize()
	c := Conclusion{DominatedBy: map[string][]string{}}

	// 支配关系：全部格子两两比。
	byKey := map[string]CellStats{}
	for _, st := range cells {
		byKey[st.Key] = st
	}
	for _, b := range cells {
		for _, a := range cells {
			if a.Key == b.Key {
				continue
			}
			if Dominates(a, b) {
				c.DominatedBy[b.Key] = append(c.DominatedBy[b.Key], a.Key)
			}
		}
		sort.Strings(c.DominatedBy[b.Key])
	}

	for _, st := range cells {
		if verdicts[st.Key].Passed {
			c.PassedCells = append(c.PassedCells, st.Key)
		}
	}
	c.Frontier = Frontier(cells, verdicts)

	if incumbent != nil {
		c.IncumbentKey = incumbent.Key()
		c.DominatorKeys = c.DominatedBy[c.IncumbentKey]
		c.IncumbentDominated = len(c.DominatorKeys) > 0
	}

	best := bestByMedian(cells)
	if len(c.PassedCells) > 0 {
		c.Verdict = VerdictCandidateFound
		c.Headline = fmt.Sprintf("%d/%d 格通过预注册三关：输出候选参数包，等 OOS 验证后才谈上线",
			len(c.PassedCells), len(cells))
		c.Lines = append(c.Lines,
			fmt.Sprintf("候选：%v。它们只是「没被三关否掉」，不是「最优」——同一批里中位更高但一致率不足的格子被判为噪声，是判定在起作用，不是排序在起作用", c.PassedCells),
			"候选不自动下发：本任务不改任何线上参数。上线路径是先在小额账户跑 challenger 变体，通过后再晋升 champion（这正是三实例配置分域存在的意义）",
		)
	} else {
		c.Verdict = VerdictNoSolution
		c.Headline = fmt.Sprintf("0/%d 格通过预注册三关 → 走无解分支：出权衡前沿，不放宽标准硬凑参数", len(cells))
		c.Lines = append(c.Lines,
			fmt.Sprintf("三关口径：%s。阈值在任务发起时锁定，跑完不接受修改——跑完再定标准等于用同一份数据既定标准又选参数", gates.Describe()),
			"按设计（设计文档 §5 第 4 条），无解时的正确输出是权衡前沿 + 结构性方案立项建议，而不是挑一个看起来最好的格子当结论",
		)
		if best != nil {
			bearP10 := 0.0
			if b := best.Scenario(ScenarioBear); b != nil {
				bearP10 = b.P10
			}
			tailPct := 0.0
			if gates.RiskEquity > 0 {
				tailPct = -bearP10 / gates.RiskEquity * 100
			}
			c.Lines = append(c.Lines, fmt.Sprintf(
				"卡住的是熊市尾部，不是收益：中位最高的 %s 有中位 %+.1fU/%.0f天、一致率 %.2f、p90回撤 %.1fU，但熊市月净 p10 = %.1fU ≈ %.0f%% 权益，超出预注册的 %.1f%%。要过严格尾部预算只能缩敞口，收益同比例缩水——这是要人拍板的取舍，不是算法能替你做的决定",
				best.Key, best.MedPnl28, 28.0, best.SignRatio, best.P90MaxDrawdown, bearP10, tailPct, gates.BearBudgetPct))
		}
	}

	if c.IncumbentKey != "" {
		if c.IncumbentDominated {
			inc := byKey[c.IncumbentKey]
			bearTxt := "熊市月分位不可估计"
			if b := inc.Scenario(ScenarioBear); b != nil {
				bearTxt = fmt.Sprintf("熊市月净 p50 %+.1fU", b.P50)
			}
			c.Lines = append(c.Lines, fmt.Sprintf(
				"现行配置 %s 被 %d 格全面支配（六个维度上都不差、至少一个更好）：中位 %+.1fU、符号一致率 %.2f、λ_bear %.1f 次/月、%s。支配它的是 %v",
				c.IncumbentKey, len(c.DominatorKeys), inc.MedPnl28, inc.SignRatio, inc.LambdaBearPerMonth, bearTxt, c.DominatorKeys))
		} else {
			c.Lines = append(c.Lines, fmt.Sprintf("现行配置 %s 在本批格子里没有被任何一格全面支配", c.IncumbentKey))
		}
	}

	ddFront, bearFront := make([]string, 0, 4), make([]string, 0, 4)
	for _, p := range c.Frontier {
		if p.OnDdFrontier {
			ddFront = append(ddFront, p.Key)
		}
		if p.OnBearFrontier {
			bearFront = append(bearFront, p.Key)
		}
	}
	sort.Strings(ddFront)
	sort.Strings(bearFront)
	c.Lines = append(c.Lines,
		fmt.Sprintf("收益/回撤前沿：%v", ddFront),
		fmt.Sprintf("收益/熊市尾部前沿：%v（本轮真正的约束在这条线上，回撤关反而较松）", bearFront),
		"OOS 纪律：扫描窗口内的收益必然高估（参数是在这段数据上挑的）。参数一旦锁定就不再调，用锁定之后新进的信号流按周复算——本任务支持以某次扫描为基准建 out_of_sample 任务，继承其冻结阈值与精算格，只换数据窗口",
	)
	return c
}

func bestByMedian(cells []CellStats) *CellStats {
	if len(cells) == 0 {
		return nil
	}
	best := 0
	for i := range cells {
		if cells[i].MedPnl28 > cells[best].MedPnl28 {
			best = i
		}
	}
	return &cells[best]
}
