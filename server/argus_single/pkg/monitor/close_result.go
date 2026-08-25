package monitor

import (
	"fmt"
	"strings"
	"time"

	"common/utils"
	pcweb "common/utils/pc_trade/web"

	"argus_single/pkg/trade"

	"github.com/sirupsen/logrus"
)

// closeResultMessage 平仓结果的 TG 文案（纯函数）。三态一眼可辨：
// 🟢 正常成功 / 🟡 主通道失效但备用兜住（仓位已平，会话待人工修） / 🔴 双通道均失败。
// 耗时等通道细节进 logrus（见 trade.webCloser），告警里通道状态比耗时重要。
func closeResultMessage(out trade.CloseOutcome) string {
	if !out.OK() {
		return fmt.Sprintf("\n\n🔴 自动平仓: 失败\n原因: %v", out.Err())
	}
	if out.Degraded {
		return fmt.Sprintf("\n\n🟡 自动平仓: 成功（Web通道失效，已降级原生API通道）\n"+
			"Web通道错误: %v\n⚠️ 请尽快更新 cookie/token——开仓仍依赖该会话", out.PrimaryErr)
	}
	return "\n\n🟢 自动平仓: 成功"
}

// manualCloseResult TG 手动平仓单仓的结果分类（用于汇总计数）。
type manualCloseResult int

const (
	manualClosed manualCloseResult = iota
	manualFailed
	manualSkipped
)

func posEmoji(posSide string) string {
	if strings.EqualFold(posSide, "short") {
		return "🔴"
	}
	return "🔵"
}

// manualCloseLine TG 手动平仓的单仓展示行（纯函数）。
func manualCloseLine(pos utils.PositionInfo, out trade.CloseOutcome) string {
	e := posEmoji(pos.PosSide)
	switch {
	case !out.OK():
		return fmt.Sprintf("  %s %s %s %s张: 🔴 失败 (%v)\n", e, pos.InstId, pos.PosSide, pos.Pos, out.Err())
	case out.Degraded:
		return fmt.Sprintf("  %s %s %s %s张: 🟡 成功(Web失效已降级原生API)\n", e, pos.InstId, pos.PosSide, pos.Pos)
	default:
		return fmt.Sprintf("  %s %s %s %s张: 🟢 成功\n", e, pos.InstId, pos.PosSide, pos.Pos)
	}
}

// manualClosePosition TG 一键/方向平仓共用的单仓平仓（含双通道 fallback）。
func (am *AccountMonitor) manualClosePosition(tm *trade.TradeManager, acc trade.AccountConfig,
	pos utils.PositionInfo, reason string) (string, manualCloseResult) {
	e := posEmoji(pos.PosSide)
	primary, backup := tm.WebCloser(acc.Name), tm.NativeCloser(acc.Name)
	if primary == nil && backup == nil {
		return fmt.Sprintf("  %s %s %s: ⚠️ 无平仓通道，跳过\n", e, pos.InstId, pos.PosSide), manualSkipped
	}
	if pos.PosId == "" {
		return fmt.Sprintf("  %s %s %s: ⚠️ 无PosId，跳过\n", e, pos.InstId, pos.PosSide), manualSkipped
	}

	logrus.Infof("[%s] 执行市价全平: 账户=%s, %s %s, PosId=%s", reason, acc.Name, pos.InstId, pos.PosSide, pos.PosId)
	out := trade.CloseWithFallback(primary, backup, trade.ClosePosArgs{
		InstId: pos.InstId, PosId: pos.PosId,
		LastPx: parsePxOrZero(pos.LastPx), Size: absAtoi(pos.Pos),
	})
	line := manualCloseLine(pos, out)
	if !out.OK() {
		logrus.Errorf("[%s] 平仓失败: 账户=%s, %s %s, err=%v", reason, acc.Name, pos.InstId, pos.PosSide, out.Err())
		return line, manualFailed
	}
	logrus.Infof("[%s] 平仓成功: 账户=%s, %s %s, 通道=%s 降级=%v",
		reason, acc.Name, pos.InstId, pos.PosSide, out.Channel, out.Degraded)
	am.recordManualClose(acc, pos, reason) // P2-A
	ResetSignalStateAfterClose(pos.InstId, pos.PosSide)
	if out.Degraded {
		am.alertChannelDegraded(acc, pos, out)
	}
	return line, manualClosed
}

// passCooldown 按指定时长节流同一 key 的告警；首次或超时放行并刷新时刻。
func (am *AccountMonitor) passCooldown(key string, d time.Duration) bool {
	am.pnlAlertMu.RLock()
	last, exists := am.lastPnlAlerts[key]
	am.pnlAlertMu.RUnlock()
	if exists && time.Since(last) < d {
		return false
	}
	am.pnlAlertMu.Lock()
	am.lastPnlAlerts[key] = time.Now()
	am.pnlAlertMu.Unlock()
	return true
}

// installTrackFailureAlert 注册埋点失败的 TG 告警。
//
// 埋点失败此前只写 logrus，淹没在日志里——上报域名下线导致静默失败数周
// 就是这么发生的（2026-07-31 才由人工比对神策后台发现）。现在主动推送。
//
// 节流 30 分钟且按 stage 分桶：埋点失败往往连续发生（域名挂了则每次平仓
// 两条全失败），不节流会刷屏。与盈亏告警、降级告警的冷却互不影响。
func (am *AccountMonitor) installTrackFailureAlert() {
	pcweb.SetTrackFailureHandler(func(stage string, err error) {
		if !am.passCooldown("track_failure#"+stage, 30*time.Minute) {
			return
		}
		msg := fmt.Sprintf("⚠️ 埋点上报失败\n⏰ %s\n\n环节: %s\n原因: %v\n\n"+
			"不影响下单与平仓，但风控画像会缺失。\n"+
			"常见原因：上报域名变更（历史上出现过 ubt.deepcoin.pro 整体下线）、"+
			"网络不通、cookie 失效。\n请核对网页抓包确认当前端点。",
			time.Now().Format("2006-01-02 15:04:05"), stage, err)
		logrus.Warn(msg)
		if _, sErr := am.telegramClient.SendMessage(msg); sErr != nil {
			logrus.Errorf("[埋点告警] TG 发送失败: %v", sErr)
		}
	})
}

