package monitor

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"common/utils"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
)

// buildCloseEvent 构造平仓类事件（trailing_close/catastrophe_stop/fixed_close），
// 补齐审计字段（P2-B）：AvgPx/LastPx 取自持仓快照（脏数据→0 由 omitempty 省略），
// PeakPct 仅在 trail 已激活时填（未激活的兜底止损可能从未有过峰值）。纯函数，可测。
func buildCloseEvent(acc trade.AccountConfig, pos utils.PositionInfo, ev string,
	pct, pnl decimal.Decimal, reason string, st TrailState) eventlog.Event {
	avgPx, _ := strconv.ParseFloat(strings.TrimSpace(pos.AvgPx), 64)
	lastPx, _ := strconv.ParseFloat(strings.TrimSpace(pos.LastPx), 64)
	e := eventlog.Event{
		Account: acc.Name,
		Variant: acc.Variant,
		InstId:  pos.InstId,
		Event:   ev,
		Side:    pos.PosSide,
		Size:    absAtoi(pos.Pos),
		AvgPx:   avgPx,
		LastPx:  lastPx,
		RoiPct:  pct.InexactFloat64(),
		Pnl:     pnl.InexactFloat64(),
		Reason:  reason,
	}
	if st.Active {
		e.PeakPct = st.PeakPct
	}
	return e
}

