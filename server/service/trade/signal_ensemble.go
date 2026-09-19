package trade

import (
	"context"
	"fmt"

	tradeDTO "service/trade/dto"
	"service/trade/strategy/signal"
)

// SignalEnsembleRequest 集合评估：同一窗口、同一基线，每组参数跑 N 条扰动路径。
// 不落库——它是"尺子"，用来判断某格与基线的差是不是路径噪声，不是要复核的回测记录。
type SignalEnsembleRequest struct {
	InstanceKey    string
	AccountLabel   string
	Symbol         string
	PlatformCode   string
	Start, End     string // 与批次同口径（parseSignalWindowTime）
	BaselineParams *tradeDTO.SignalBacktestParamsDTO
	Groups         []tradeDTO.SignalBacktestGroupDTO
	N              int
	Perturb        signal.Perturb // Seed 逐条覆盖，只取两个概率
	// RefLabel 配对参照组：空 = 基线。扫参时基线常是"机制关掉"，而要回答的往往是
	// "候选格比**生产格**好多少"，那就把生产格的 label 填这里。
	RefLabel string
}

// SignalEnsembleGroup 一组的分布 + 与基线的配对差。
type SignalEnsembleGroup struct {
	Label      string
	SinglePath float64 // 不扰动的单路径净盈亏（与批次表里那一格同口径）
	SingleCats int
	// SingleSkips 单路径上各闸的拦截次数（趋势闸/加仓闸/上限），用来确认"这一格机制到底咬住了没有"。
	SingleSkips string
	Stats       signal.EnsembleStats
	VsBaseline  signal.PairedDelta // 相对 Report.RefLabel 那一组的配对差（名字沿用，参照可换）
}

type SignalEnsembleReport struct {
	N        int
	Perturb  signal.Perturb
	Signals  int
	Bars     int
	RefLabel string // 实际用作配对参照的组（"baseline" 或某组 label）
	Baseline SignalEnsembleGroup
	Groups   []SignalEnsembleGroup
	Notes    []string
}

// RunSignalEnsemble 取一份数据集，基线与每组各跑 N 条同 seed 的扰动路径，做配对比较。
func (s *TradeService) RunSignalEnsemble(ctx context.Context, req SignalEnsembleRequest) (*SignalEnsembleReport, error) {
	if req.N <= 0 {
		return nil, fmt.Errorf("N 必须 ≥1")
	}
	if !req.Perturb.On() {
		return nil, fmt.Errorf("至少给一种扰动（SignalDropPct 或 ExitLateProb），否则 N 条路径全同")
	}
	start, err := parseSignalWindowTime(req.Start)
	if err != nil {
		return nil, err
	}
	end, err := parseSignalWindowTime(req.End)
	if err != nil {
		return nil, err
	}
	ds, err := s.loadSignalDataset(signalWindow{
		InstanceKey: req.InstanceKey, AccountLabel: req.AccountLabel,
		Symbol: req.Symbol, PlatformCode: req.PlatformCode, Start: start, End: end,
	}, false)
	if err != nil {
		return nil, err
	}
	base, err := s.resolveBatchBaseline(ctx, tradeDTO.CreateSignalBacktestBatchDTO{BaselineParams: req.BaselineParams},
		req.InstanceKey, req.AccountLabel, req.Symbol)
	if err != nil {
		return nil, err
	}
	rep := &SignalEnsembleReport{N: req.N, Perturb: req.Perturb, Signals: len(ds.Signals), Bars: len(ds.Bars), Notes: base.Notes}
	in := signal.Input{Signals: ds.Signals, Bars: ds.Bars, Seed: ds.Seed}

	rep.Baseline, err = runEnsembleGroup("baseline", in, base.Params, req.N, req.Perturb)
	if err != nil {
		return nil, err
	}
	for _, g := range req.Groups {
		params := applySignalParamKnobs(base.Params, g.SignalBacktestParamsDTO)
		grp, err := runEnsembleGroup(g.Label, in, params, req.N, req.Perturb)
		if err != nil {
			return nil, fmt.Errorf("组 %s: %w", g.Label, err)
		}
		rep.Groups = append(rep.Groups, grp)
	}
	// 配对参照：先跑完全部组再配，参照可以是任意一组。
	ref := rep.Baseline
	rep.RefLabel = "baseline"
	if req.RefLabel != "" {
		found := false
		for _, g := range rep.Groups {
			if g.Label == req.RefLabel {
				ref, found = g, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("配对参照组 %q 不在本维网格里", req.RefLabel)
		}
		rep.RefLabel = req.RefLabel
	}
	for i := range rep.Groups {
		rep.Groups[i].VsBaseline = signal.Paired(ref.Stats, rep.Groups[i].Stats)
	}
	return rep, nil
}

func runEnsembleGroup(label string, in signal.Input, p signal.Params, n int, pert signal.Perturb) (SignalEnsembleGroup, error) {
	in.Params = p
	in.Perturb = signal.Perturb{}
	single, err := signal.Replay(in)
	if err != nil {
		return SignalEnsembleGroup{}, err
	}
	cats := 0
	for _, ep := range single.Episodes {
		if ep.Reason == signal.ExitCatastrophe {
			cats++
		}
	}
	st, err := signal.Ensemble(in, n, pert)
	if err != nil {
		return SignalEnsembleGroup{}, err
	}
	skips := fmt.Sprintf("趋%d/加%d/限%d", single.SkipTrend, single.SkipAddRoi, single.SkipCap)
	return SignalEnsembleGroup{Label: label, SinglePath: single.Net, SingleCats: cats, SingleSkips: skips, Stats: st}, nil
}
