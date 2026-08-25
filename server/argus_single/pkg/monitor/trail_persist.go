package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// trailSchemaVersion 落盘格式版本；不匹配一律拒载（冷启动），不做跨版本猜测迁移。
const trailSchemaVersion = 1

// persistedTrail 单条 trail 状态的落盘形态。PosId 是恢复时唯一认可的强身份
// （设计 v3）：探针实测 posId 在加/减仓间稳定、平仓重开必换；时间字段禁用
// ——CTime 在净仓 long→short 切换时不重置，用它对账会把旧仓峰值套到新仓上。
type persistedTrail struct {
	PosId    string  `json:"posId"`
	PeakPct  float64 `json:"peakPct"`
	LastSize int     `json:"lastSize"`
	Active   bool    `json:"active"`
}

type trailStateFile struct {
	SchemaVersion int                       `json:"schemaVersion"`
	SavedAt       string                    `json:"savedAt"`
	States        map[string]persistedTrail `json:"states"`
}

// reconcileTrailStates 用实盘持仓的 posId 逐条对账落盘状态（纯函数）。
// livePosIds: key(账户:instId:posSide) → 实盘当前 posId。
// 只有"key 存在于实盘 && 两侧 posId 均非空 && 完全相同"才恢复；其余全部丢弃并返回
// 供日志追述。fail-safe：错误恢复（把旧高峰值套到新仓）会让新仓开出来就被误平，
// 比不恢复危险得多。
func reconcileTrailStates(persisted map[string]persistedTrail, livePosIds map[string]string) (map[string]TrailState, []string) {
	restored := make(map[string]TrailState)
	var dropped []string
	for key, p := range persisted {
		livePosId, exists := livePosIds[key]
		if !exists || p.PosId == "" || livePosId == "" || p.PosId != livePosId {
			dropped = append(dropped, key)
			continue
		}
		restored[key] = TrailState{PeakPct: p.PeakPct, LastSize: p.LastSize, Active: p.Active}
	}
	return restored, dropped
}

// buildPersistedTrails 拼装落盘数据：峰值来自 trailStates，posId 来自 snapshots。
// 无快照或 posId 为空的条目不落盘——落了也必然在对账时被丢弃。
func (am *AccountMonitor) buildPersistedTrails() map[string]persistedTrail {
	am.trailMu.RLock()
	defer am.trailMu.RUnlock()
	am.snapMu.RLock()
	defer am.snapMu.RUnlock()

	out := make(map[string]persistedTrail, len(am.trailStates))
	for k, st := range am.trailStates {
		snap, ok := am.snapshots[k]
		if !ok || snap.PosId == "" {
			continue
		}
		out[k] = persistedTrail{PosId: snap.PosId, PeakPct: st.PeakPct, LastSize: st.LastSize, Active: st.Active}
	}
	return out
}

// restoreAccountTrails 在某账户首轮持仓轮询时用实盘 posId 对账并恢复其 trail 状态。
// 此时实盘 posId 已在手，无需额外 API 调用。恢复后消费掉该账户的待恢复项——
// 否则后续轮次会把已被 GC 的状态反复复活。
func (am *AccountMonitor) restoreAccountTrails(account string, livePosIds map[string]string) {
	am.restoreMu.Lock()
	defer am.restoreMu.Unlock()
	if len(am.pendingRestore) == 0 {
		return
	}
	prefix := account + ":"
	mine := make(map[string]persistedTrail)
	for k, v := range am.pendingRestore {
		if strings.HasPrefix(k, prefix) {
			mine[k] = v
			delete(am.pendingRestore, k) // 消费：只尝试一次
		}
	}
	if len(mine) == 0 {
		return
	}

	restored, dropped := reconcileTrailStates(mine, livePosIds)
	if len(restored) > 0 {
		am.trailMu.Lock()
		for k, st := range restored {
			am.trailStates[k] = st
			logrus.Infof("[trail恢复] %s 峰值=%.2f%% 张数=%d 激活=%v (posId 对账一致)",
				k, st.PeakPct, st.LastSize, st.Active)
		}
		am.trailMu.Unlock()
	}
	for _, k := range dropped {
		logrus.Warnf("[trail恢复] %s 丢弃：posId 不一致/缺失或仓位已不存在（fail-safe，不恢复）", k)
	}
}

// markTrailDirty 标脏，供 30s 异步快照判断是否需要落盘。
func (am *AccountMonitor) markTrailDirty() {
	am.trailDirty.Store(true)
}

// flushTrailStates 落盘一次（脏才写；force=true 时无条件写，用于 shutdown 同步 flush）。
func (am *AccountMonitor) flushTrailStates(force bool) {
	if !force && !am.trailDirty.Swap(false) {
		return
	}
	if force {
		am.trailDirty.Store(false)
	}
	states := am.buildPersistedTrails()
	if err := saveTrailStateFile(am.trailStatePath, states); err != nil {
		// 持久化失败只告警不阻断交易（沿用 eventlog 既有约定）
		logrus.Errorf("[trail持久化] 落盘失败 path=%s: %v", am.trailStatePath, err)
		am.trailDirty.Store(true) // 下轮重试
		return
	}
	logrus.Debugf("[trail持久化] 已落盘 %d 条 → %s", len(states), am.trailStatePath)
}

// startTrailSnapshot 30s 异步快照循环。
func (am *AccountMonitor) startTrailSnapshot() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.flushTrailStates(false)
		}
	}
}

// saveTrailStateFile 原子写（temp + rename）：中途崩溃不会留下半截文件覆盖旧快照。
func saveTrailStateFile(path string, states map[string]persistedTrail) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	blob, err := json.Marshal(trailStateFile{
		SchemaVersion: trailSchemaVersion,
		SavedAt:       time.Now().Format("2006-01-02 15:04:05"),
		States:        states,
	})
	if err != nil {
		return fmt.Errorf("序列化: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".trail_state-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后此调用为 no-op
	if _, err := tmp.Write(blob); err != nil {
		tmp.Close()
		return fmt.Errorf("写临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename 到 %s: %w", path, err)
	}
	return nil
}

// loadTrailStateFile 读取落盘状态。文件缺失/损坏/版本不符均返回 nil
// （= 冷启动 = 现行为），err 仅供日志，调用方不得据此中断启动。
func loadTrailStateFile(path string) (map[string]persistedTrail, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 首次运行，静默冷启动
		}
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	var f trailStateFile
	if err := json.Unmarshal(blob, &f); err != nil {
		return nil, fmt.Errorf("解析 %s 失败(冷启动): %w", path, err)
	}
	if f.SchemaVersion != trailSchemaVersion {
		return nil, fmt.Errorf("schemaVersion=%d 与当前 %d 不匹配(冷启动)", f.SchemaVersion, trailSchemaVersion)
	}
	return f.States, nil
}

