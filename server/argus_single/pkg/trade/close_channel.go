package trade

import (
	"errors"
	"fmt"

	pcweb "common/utils/pc_trade/web"

	"github.com/sirupsen/logrus"
)

// PositionCloser 一条平仓通道。
// 现有两条鉴权互相独立的通道：web（cookie+token，会过期）与 native（apiKey HMAC 签名，长期有效）。
// 2026-07-21→23 实测 GW: Login Timeout 让 web 通道瘫痪 50 小时，期间 native 鉴权全程正常
// （持仓/余额查询走的就是它）——安全网因此不应只挂在 web 一条通道上。
type PositionCloser interface {
	Close(a ClosePosArgs) error
	Channel() string // "web" / "native"，用于日志与告警文案
}

// ClosePosArgs 平仓入参。LastPx/Size 仅 web 通道用于补发网页同款埋点
// （原生 API 通道不发——那条路径不经过网页接口，发了反而自相矛盾）。
type ClosePosArgs struct {
	InstId string
	PosId  string
	LastPx float64 // 内存快照价（≤5s 旧，非成交价）
	Size   int     // 张数
}

// ErrNoCloseChannel 两条通道均未配置——属配置故障，调用方应立即告警而非计入连败。
var ErrNoCloseChannel = errors.New("无可用平仓通道(web/native 均未配置)")

// nativeProductGroup USDT 本位永续的产品组（与 autotrade_client.go CloseAllPositions 既有约定一致）。
const nativeProductGroup = "SwapU"

// webCloser 主通道适配器（cookie+token 会话）。
type webCloser struct {
	closePos func(posId, instId string, lastPx float64, size int) (*pcweb.ClosePosResponse, error)
}

func (w *webCloser) Close(a ClosePosArgs) error {
	resp, err := w.closePos(a.PosId, a.InstId, a.LastPx, a.Size)
	if err != nil {
		return err
	}
	logrus.Infof("[平仓通道:web] posId=%s spend=%dms", a.PosId, resp.Data.Spend)
	return nil
}

func (w *webCloser) Channel() string { return "web" }

// nativeCloser 备用通道适配器（apiKey HMAC 签名，不依赖会话）。
type nativeCloser struct {
	closeByIds func(productGroup, instId string, posIds []string) (map[string]interface{}, error)
}

func (n *nativeCloser) Close(a ClosePosArgs) error {
	instId, posId := a.InstId, a.PosId
	resp, err := n.closeByIds(nativeProductGroup, instId, []string{posId})
	if err != nil {
		// 完整响应/错误进日志：首次真实故障要靠它区分鉴权/参数/权限三类问题
		logrus.Errorf("[平仓通道:native] 失败 productGroup=%s instId=%s posId=%s err=%v resp=%+v",
			nativeProductGroup, instId, posId, err, resp)
		return err
	}
	logrus.Infof("[平仓通道:native] 成功 instId=%s posId=%s resp=%+v", instId, posId, resp)
	return nil
}

func (n *nativeCloser) Channel() string { return "native" }

// WebCloser 主通道；未配置 cookie/token 时返回真 nil（不可返回带 nil 指针的接口值）。
func (tm *TradeManager) WebCloser(account string) PositionCloser {
	tm.mu.RLock()
	c := tm.webClients[account]
	tm.mu.RUnlock()
	if c == nil {
		return nil
	}
	return &webCloser{closePos: c.ClosePosition}
}

// NativeCloser 备用通道；无 API 客户端时返回真 nil。
func (tm *TradeManager) NativeCloser(account string) PositionCloser {
	tm.mu.RLock()
	c := tm.clients[account]
	tm.mu.RUnlock()
	if c == nil {
		return nil
	}
	return &nativeCloser{closeByIds: c.ClosePositionsByIds}
}

// CloseOutcome 平仓结果。Channel 非空即成功。
type CloseOutcome struct {
	Channel    string // 实际生效的通道；两条都失败时为空
	Degraded   bool   // true = 主通道"配置了但失败"、由备用通道兜住（主通道未配置不算降级）
	PrimaryErr error  // 主通道错误（降级时保留，供诊断）
	BackupErr  error  // 备用通道错误
}

// OK 是否成功平仓。
func (o CloseOutcome) OK() bool { return o.Channel != "" }

// Err 失败原因；成功时为 nil。两条都试过时聚合两条原因——首次真实故障要靠它
// 判定 native 通道是鉴权、参数还是权限问题，三类修法完全不同。
func (o CloseOutcome) Err() error {
	if o.OK() {
		return nil
	}
	switch {
	case o.PrimaryErr != nil && o.BackupErr != nil:
		return fmt.Errorf("双通道均失败: web=%v; native=%v", o.PrimaryErr, o.BackupErr)
	case o.PrimaryErr != nil:
		return o.PrimaryErr
	case o.BackupErr != nil:
		return o.BackupErr
	default:
		return ErrNoCloseChannel
	}
}

// CloseWithFallback 主通道失败时自动降级到备用通道。纯逻辑，无 I/O。
// 主通道成功时备用通道零调用——绝不盲目双发平仓请求。
func CloseWithFallback(primary, backup PositionCloser, a ClosePosArgs) CloseOutcome {
	var out CloseOutcome

	if primary != nil {
		if err := primary.Close(a); err == nil {
			out.Channel = primary.Channel()
			return out
		} else {
			out.PrimaryErr = err
		}
	}

	if backup == nil {
		return out
	}

	if err := backup.Close(a); err != nil {
		out.BackupErr = err
		return out
	}
	out.Channel = backup.Channel()
	// 主通道未配置时走备用属正常路径；只有"配置了却失败"才是降级
	out.Degraded = out.PrimaryErr != nil
	return out
}

