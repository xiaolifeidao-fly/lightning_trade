package monitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"common/utils"
)

// posLiveness 持仓存活三态（P1/R3，见 docs/2026-07-17-可靠性与风险资本合并设计.md）。
// 仅用于 trail 状态 GC 的保护判定，与"盈亏是否可评估"解耦。
type posLiveness int

const (
	posLive    posLiveness = iota // Pos 有效非零：仓位存活
	posDead                       // Pos 有效为零：仓位确实不存在，可 GC
	posUnknown                    // Pos 空/非法：瞬时脏数据，保守视为存活一轮
)

// classifyPosLiveness 按原始 Pos 字段判定三态。
func classifyPosLiveness(rawPos string) posLiveness {
	s := strings.TrimSpace(rawPos)
	if s == "" {
		return posUnknown
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return posUnknown
	}
	if v.IsZero() {
		return posDead
	}
	return posLive
}

// livePosIds 构建 key → 实盘 posId 映射，供 P4 trail 状态身份对账。
// 口径与 buildLiveKeys 完全一致（dead 不收录、unknown 保守收录）：
// Pos=0 的死仓 posId 若参与对账，会让已平仓位的旧峰值被"身份一致"复活。
func livePosIds(accName string, positions []utils.PositionInfo) map[string]string {
	out := make(map[string]string, len(positions))
	for _, pos := range positions {
		if pos.InstId == "" || pos.PosSide == "" {
			continue
		}
		if classifyPosLiveness(pos.Pos) == posDead {
			continue
		}
		out[fmt.Sprintf("%s:%s:%s", accName, pos.InstId, pos.PosSide)] = strings.TrimSpace(pos.PosId)
	}
	return out
}

// buildLiveKeys 按三态规则构建存活 key 集合：live/unknown 保留，dead 不保留。
// key 组成字段缺失（InstId/PosSide 为空）时无法构成有效 key，跳过。
func buildLiveKeys(accName string, positions []utils.PositionInfo) map[string]bool {
	liveKeys := make(map[string]bool, len(positions))
	for _, pos := range positions {
		if pos.InstId == "" || pos.PosSide == "" {
			continue
		}
		if classifyPosLiveness(pos.Pos) == posDead {
			continue
		}
		liveKeys[fmt.Sprintf("%s:%s:%s", accName, pos.InstId, pos.PosSide)] = true
	}
	return liveKeys
}

// parsePxOrZero 解析价格字符串，失败返回 0（埋点字段用，解析失败不阻断平仓）。
func parsePxOrZero(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

