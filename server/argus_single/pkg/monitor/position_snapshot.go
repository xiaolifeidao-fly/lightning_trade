package monitor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	"common/utils"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/trade"
)

// posSnapshot 上一轮持仓快照（P2-A）：external_close 事件的数据来源 +
// P4 前置探针（PosId/UTime/CTime 身份字段观察）。
type posSnapshot struct {
	InstId  string
	PosSide string
	Size    int
	AvgPx   float64
	LastPx  float64
	Upl     float64 // 符号归一后的未实现盈亏（最后一次轮询，≤5s 旧）
	RoiPct  float64
	PosId   string
	UTime   string
	CTime   string
}

// snapshotFromPosition 从持仓行生成快照；仅 Pos 有效非零（live）才生成，
// 其余字段 best-effort（解析失败→0，不阻断）。纯函数。
func snapshotFromPosition(pos utils.PositionInfo) (posSnapshot, bool) {
	if classifyPosLiveness(pos.Pos) != posLive {
		return posSnapshot{}, false
	}
	snap := posSnapshot{
		InstId:  pos.InstId,
		PosSide: pos.PosSide,
		Size:    absAtoi(pos.Pos),
		PosId:   pos.PosId,
		UTime:   pos.UTime,
		CTime:   pos.CTime,
	}
	snap.AvgPx, _ = strconv.ParseFloat(strings.TrimSpace(pos.AvgPx), 64)
	snap.LastPx, _ = strconv.ParseFloat(strings.TrimSpace(pos.LastPx), 64)
	if pnl, ok := normalizePositionPnl(pos); ok {
		snap.Upl = pnl.InexactFloat64()
		if margin, err := decimal.NewFromString(strings.TrimSpace(pos.UseMargin)); err == nil && margin.IsPositive() {
			snap.RoiPct = pnl.Div(margin).Mul(decimal.NewFromInt(100)).InexactFloat64()
		}
	}
	return snap, true
}

// findExternalCloses 对账（纯函数）：上一轮有、本轮不在 liveKeys、且未被
// bot-close 注册表消费的仓位 = 外部平仓。consume 由调用方注入（可测），
// posId 取消失仓位的最后快照（review fix#2：标记绑定仓位实例）。
// 返回按 key 排序，保证事件顺序确定。
func findExternalCloses(prev map[string]posSnapshot, live map[string]bool, consume func(instId, posSide, posId string) bool) []posSnapshot {
	keys := make([]string, 0, len(prev))
	for k := range prev {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []posSnapshot
	for _, k := range keys {
		if live[k] {
			continue
		}
		snap := prev[k]
		if consume(snap.InstId, snap.PosSide, snap.PosId) {
			continue // 本机器人平仓，正常路径已落事件
		}
		out = append(out, snap)
	}
	return out
}

// buildExternalCloseEvent 外部平仓事件：字段取最后快照，pnl=最后轮询 UPL（≤5s 估算）。
// trail 已激活则附 peakPct（外部平仓时 trailing 走到哪，一目了然）。纯函数。
func buildExternalCloseEvent(acc trade.AccountConfig, snap posSnapshot, st TrailState) eventlog.Event {
	e := eventlog.Event{
		Account: acc.Name,
		Variant: acc.Variant,
		InstId:  snap.InstId,
		Event:   eventlog.EvExternalClose,
		Side:    snap.PosSide,
		Size:    snap.Size,
		AvgPx:   snap.AvgPx,
		LastPx:  snap.LastPx,
		RoiPct:  snap.RoiPct,
		Pnl:     snap.Upl,
		Reason:  "非本机器人平仓;pnl=最后轮询UPL",
	}
	if st.Active {
		e.PeakPct = st.PeakPct
	}
	return e
}

// reconcileExternalCloses 每轮持仓查询成功后调用（须在 gcTrailStates 之前，
// 以便给事件附上尚未被 GC 的 peak）。返回本轮识别的外部平仓数。
func (am *AccountMonitor) reconcileExternalCloses(acc trade.AccountConfig, liveKeys map[string]bool) int {
	prefix := acc.Name + ":"
	prev := make(map[string]posSnapshot)
	am.snapMu.RLock()
	for k, s := range am.snapshots {
		if strings.HasPrefix(k, prefix) {
			prev[k] = s
		}
	}
	am.snapMu.RUnlock()

	externals := findExternalCloses(prev, liveKeys, func(instId, posSide, posId string) bool {
		return trade.ConsumeBotClose(acc.Name, instId, posSide, posId)
	})
	for _, snap := range externals {
		key := fmt.Sprintf("%s:%s:%s", acc.Name, snap.InstId, snap.PosSide)
		am.trailMu.RLock()
		st := am.trailStates[key]
		am.trailMu.RUnlock()
		eventlog.Log(buildExternalCloseEvent(acc, snap, st))
		logrus.Warnf("[持仓监控] 识别到外部平仓: %s %d张 (最后轮询 ROI=%.1f%%, UPL=%.2f)",
			key, snap.Size, snap.RoiPct, snap.Upl)
	}
	return len(externals)
}

// updateSnapshots 用本轮响应刷新该账户快照：live 覆盖（顺带 P4 探针日志）、
// unknown 保留旧值、dead/消失删除。
func (am *AccountMonitor) updateSnapshots(accName string, positions []utils.PositionInfo) {
	prefix := accName + ":"
	next := make(map[string]posSnapshot)
	am.snapMu.RLock()
	old := make(map[string]posSnapshot)
	for k, s := range am.snapshots {
		if strings.HasPrefix(k, prefix) {
			old[k] = s
		}
	}
	am.snapMu.RUnlock()

	for _, pos := range positions {
		if pos.InstId == "" || pos.PosSide == "" {
			continue
		}
		key := fmt.Sprintf("%s:%s:%s", accName, pos.InstId, pos.PosSide)
		switch classifyPosLiveness(pos.Pos) {
		case posLive:
			snap, ok := snapshotFromPosition(pos)
			if !ok {
				continue
			}
			if prevSnap, existed := old[key]; existed && prevSnap.Size != snap.Size {
				// P4 前置探针：加/减仓时打印身份字段，验证 posId 稳定性与 UTime/CTime 语义
				logrus.Infof("[P4探针] %s size %d→%d posId %q→%q uTime %q→%q cTime %q→%q",
					key, prevSnap.Size, snap.Size, prevSnap.PosId, snap.PosId,
					prevSnap.UTime, snap.UTime, prevSnap.CTime, snap.CTime)
			} else if !existedIn(old, key) {
				logrus.Infof("[P4探针] %s 新仓 size=%d posId=%q uTime=%q cTime=%q",
					key, snap.Size, snap.PosId, snap.UTime, snap.CTime)
			}
			next[key] = snap
		case posUnknown:
			if prevSnap, existed := old[key]; existed {
				next[key] = prevSnap // 脏数据轮保留旧快照
			}
		case posDead:
			// 不进 next → 自然删除
		}
	}

	am.snapMu.Lock()
	for k := range am.snapshots {
		if strings.HasPrefix(k, prefix) {
			delete(am.snapshots, k)
		}
	}
	for k, s := range next {
		am.snapshots[k] = s
	}
	am.snapMu.Unlock()
}

func existedIn(m map[string]posSnapshot, key string) bool {
	_, ok := m[key]
	return ok
}

