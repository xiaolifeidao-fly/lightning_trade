package signal

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// 行情路由减仓（第 2 步）：前一日被判为危险状态时，把本日入场适用的仓位上限
// 按系数压低。
//
// 为什么路由的是**入场上限**而不是止损线——这是趋势条件止损那次失败换来的：
// 出场侧的旋钮在最糟的时刻、依据一个在阈值附近抖动的指标做不可逆决策，结果
// 名义 250% 的线实际砍在 −299%，而且把本会回归的浮亏砍成了实亏（80 天回测里
// 两账户兜底次数分别 10→20、4→13）。上限是在**承担风险之前**定的，判断错只
// 少赚。趋势闸的走前验证也印证了入场侧这条路走得通。
//
// 为什么用【前一日】而不是 DayLabels：DayLabels 用当日 OHLC 给整日打标签，是
// 事后情景标注（给 bootstrap 用的），拿它做入场路由等于偷看当天的收盘与振幅。
// 这是本文件最容易犯且最难发现的错，所以单独一个测试钉住。
//
// 阈值复用 RegimeThresholds（金标准 backtest_capsf_study.py 的 1.5% / 2.5%），
// 标签语义也照搬：先看 |日收益|（单边），再看日内振幅（震荡），都不到是安静。
// 代码里那句注释说得对——均值回归累加器怕的是单边行情、不分涨跌。

// PriorDayLabels 给每个自然日一个**决策时可知**的状态标签：取前一个已收盘
// 自然日的标签。缺前一日（窗口首日）或与前一日不相邻（K 线缺整天）时不给标签，
// 调用方据此不减仓——与趋势闸 warmup 同口径：拿不到证据就不动别人的仓位。
func PriorDayLabels(bars []Bar, th RegimeThresholds) map[string]string {
	post := DayLabels(bars, th)
	out := make(map[string]string, len(post))
	for day := range post {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		prev := d.AddDate(0, 0, -1).Format("2006-01-02")
		if l, ok := post[prev]; ok {
			out[day] = l
		}
	}
	return out
}

// regimeScaleLabelSet 解析要触发减仓的标签集。拼错字必须报错而不是静默变成
// "空集=关闭"——扫参时那会表现为"该格与基线完全一样"，是最难发现的失效。
func regimeScaleLabelSet(s string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, raw := range strings.Split(s, ",") {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		switch t {
		case RegimeTrend, RegimeVol, RegimeQuiet:
			out[t] = true
		default:
			return nil, fmt.Errorf("未知行情标签 %q（只接受 %s / %s / %s）",
				raw, RegimeTrend, RegimeVol, RegimeQuiet)
		}
	}
	return out, nil
}

// resolveRegimeScale 本次入场适用的仓位系数。1 = 不减仓。
// 标签集为空即关闭；系数不在 [0,1) 内也按关闭处理（Validate 会先拒掉，
// 这里再兜一层，避免有人绕过校验直接构造 Params）。
func resolveRegimeScale(p Params, labels map[string]string, at time.Time) float64 {
	set, err := regimeScaleLabelSet(p.RegimeScaleLabels)
	if err != nil || len(set) == 0 {
		return 1
	}
	if p.RegimeScaleFactor < 0 || p.RegimeScaleFactor >= 1 {
		return 1
	}
	l, ok := labels[at.Format("2006-01-02")]
	if !ok || !set[l] {
		return 1
	}
	return p.RegimeScaleFactor
}

// scaledCap 系数落到上限上。向下取整：宁可小不可大。
// 系数 0 ⇒ 上限 0 ⇒ 当日完全不开新仓，这是"跳过"档，必须允许。
func scaledCap(cap int, scale float64) int {
	if scale >= 1 {
		return cap
	}
	if scale <= 0 {
		return 0
	}
	return int(math.Floor(float64(cap) * scale))
}

// regimeThresholds 本组参数用的标签阈值；未显式给就用金标准缺省值。
func (p Params) regimeThresholds() RegimeThresholds {
	th := RegimeThresholds{
		TrendAbsRetPct: p.RegimeTrendAbsRetPct,
		VolRangePct:    p.RegimeVolRangePct,
	}
	if th.TrendAbsRetPct <= 0 && th.VolRangePct <= 0 {
		return DefaultRegimeThresholds()
	}
	return th.normalize()
}

// validateRegimeScale 启用时（标签集非空）系数必须真的是减仓。
// 只给系数不给标签视为没启用，放行。
func validateRegimeScale(p Params) error {
	set, err := regimeScaleLabelSet(p.RegimeScaleLabels)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		return nil
	}
	if p.RegimeScaleFactor < 0 || p.RegimeScaleFactor >= 1 {
		return fmt.Errorf("行情路由减仓系数必须落在 [0,1)：0 = 当日不开新仓，1 = 没有减仓（等于没启用），got %.3f",
			p.RegimeScaleFactor)
	}
	return nil
}
