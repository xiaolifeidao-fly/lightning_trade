package backfill

import (
	"sort"

	"argus_single/pkg/eventlog"
)

// 字段适用性矩阵（设计文档 §6.6）。
//
// 历史 JSONL 里没有任何业务字段出现过真零值（omitempty 把零值全省掉了），
// 所以"NULL 到底是缺失还是不适用"只能由事件类型决定：trailing_close 缺
// roi_pct 是异常，open 缺 roi_pct 是正常。回灌时兼作校验器，把"应有而缺"
// 的条数按天报出来，而不是静默写一堆 NULL。
//
// 这里只报数、不拒绝导入：历史数据的缺口是既成事实（例如 6/29 那次未记录的
// 外部平仓），拒绝导入等于把 44MB 历史全挡在库外。
var requiredFields = map[string][]string{
	eventlog.EvOpen:            {"size", "orderSize", "side"},
	eventlog.EvCapSkip:         {"size", "side"},
	eventlog.EvGateBlock:       {"roiPct", "avgPx", "lastPx", "netSide"},
	eventlog.EvTrendSkip:       {"side"},
	eventlog.EvTrailingClose:   {"roiPct", "pnl", "size"},
	eventlog.EvCatastropheStop: {"roiPct", "pnl", "size"},
	eventlog.EvFixedClose:      {"roiPct", "pnl", "size"},
	eventlog.EvExternalClose:   {"roiPct", "size"},
	eventlog.EvManualClose:     {"roiPct", "size"},
	eventlog.EvLossAlert:       {"roiPct", "pnl", "size"},
	eventlog.EvBalance:         {"balance"},
	eventlog.EvDevSample:       {"devTicks"},
}

// cap_skip 的 orderSize 不进必需集：上限跳过时压根没有下单张数。
// gate_block 的 pnl 同理（门控只看 ROI，不结算盈亏）。

// missingRequired 返回该事件按矩阵"应有而缺"的字段名。
func missingRequired(e eventlog.Event) []string {
	var missing []string
	for _, field := range requiredFields[e.Event] {
		if !hasField(e, field) {
			missing = append(missing, field)
		}
	}
	return missing
}

// hasField 判断字段在 JSONL 里是否出现过。omitempty 让零值与缺失编码相同，
// 因此"非零 ⇔ 出现过"，这正是 §2.5 记录的口径。
func hasField(e eventlog.Event, field string) bool {
	switch field {
	case "size":
		return e.Size != 0
	case "orderSize":
		return e.OrderSize != 0
	case "side":
		return e.Side != ""
	case "netSide":
		return e.NetSide != ""
	case "roiPct":
		return e.RoiPct != 0
	case "pnl":
		return e.Pnl != 0
	case "avgPx":
		return e.AvgPx != 0
	case "lastPx":
		return e.LastPx != 0
	case "balance":
		return e.Balance != 0
	case "devTicks":
		return e.DevTicks != 0
	default:
		return true
	}
}

// FieldAnomaly 某一天某类事件的"应有而缺"计数。
type FieldAnomaly struct {
	Event   string
	Field   string
	Count   int
	Example string // 首例的 ts，便于回原始 JSONL 定位
}

// sortAnomalies 让报告的行序稳定（同一批数据两次跑出同样的报告）。
func sortAnomalies(anomalies []FieldAnomaly) {
	sort.Slice(anomalies, func(i, j int) bool {
		if anomalies[i].Event != anomalies[j].Event {
			return anomalies[i].Event < anomalies[j].Event
		}
		return anomalies[i].Field < anomalies[j].Field
	})
}
