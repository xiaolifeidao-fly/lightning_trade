package monitor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"argus_single/pkg/trade"
	"common/utils"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

const defaultAICloseApprovalTimeout = 30 * time.Minute

type PendingAICloseApproval struct {
	ID          string
	AlertKey    string
	Account     trade.AccountConfig
	Position    utils.PositionInfo
	Source      string
	Reason      string
	TriggerType string
	PnLPercent  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type aiCloseApprovalStore struct {
	mu         sync.Mutex
	byID       map[string]*PendingAICloseApproval
	byAlertKey map[string]string
	timeout    time.Duration
}

func newAICloseApprovalStore(timeout time.Duration) *aiCloseApprovalStore {
	if timeout <= 0 {
		timeout = defaultAICloseApprovalTimeout
	}
	return &aiCloseApprovalStore{
		byID:       make(map[string]*PendingAICloseApproval),
		byAlertKey: make(map[string]string),
		timeout:    timeout,
	}
}

func (s *aiCloseApprovalStore) createOrGetExisting(acc trade.AccountConfig, pos utils.PositionInfo, alertKey, source, reason, triggerType string, pct decimal.Decimal) (*PendingAICloseApproval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked()
	if existingID, ok := s.byAlertKey[alertKey]; ok {
		if existing := s.byID[existingID]; existing != nil {
			return existing, false
		}
		delete(s.byAlertKey, alertKey)
	}

	now := time.Now()
	req := &PendingAICloseApproval{
		ID:          fmt.Sprintf("AICLOSE-%d", now.UnixNano()),
		AlertKey:    alertKey,
		Account:     acc,
		Position:    pos,
		Source:      source,
		Reason:      reason,
		TriggerType: triggerType,
		PnLPercent:  pct.StringFixed(2),
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.timeout),
	}
	s.byID[req.ID] = req
	s.byAlertKey[alertKey] = req.ID
	return req, true
}

func (s *aiCloseApprovalStore) take(id string) (*PendingAICloseApproval, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked()
	req := s.byID[id]
	if req == nil {
		return nil, "⚠️ 未找到待审批的 AI 平仓请求，请确认请求ID是否正确"
	}
	if time.Now().After(req.ExpiresAt) {
		s.deleteLocked(req)
		return nil, "⚠️ 这条 AI 平仓审批请求已过期，请等待新的 AI 信号"
	}
	s.deleteLocked(req)
	return req, ""
}

func (s *aiCloseApprovalStore) reject(id string) (*PendingAICloseApproval, string) {
	return s.take(id)
}

func (s *aiCloseApprovalStore) list() []*PendingAICloseApproval {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked()
	items := make([]*PendingAICloseApproval, 0, len(s.byID))
	for _, req := range s.byID {
		items = append(items, req)
	}
	return items
}

func (s *aiCloseApprovalStore) deleteLocked(req *PendingAICloseApproval) {
	delete(s.byID, req.ID)
	delete(s.byAlertKey, req.AlertKey)
}

func (s *aiCloseApprovalStore) pruneLocked() {
	now := time.Now()
	for id, req := range s.byID {
		if now.After(req.ExpiresAt) {
			delete(s.byID, id)
			delete(s.byAlertKey, req.AlertKey)
		}
	}
}

func (am *AccountMonitor) requestManualAICloseApproval(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, alertKey, source, reason, triggerType string, pct decimal.Decimal) string {
	webClient := tm.GetWebClient(acc.Name)
	if webClient == nil {
		return fmt.Sprintf("\n\n⚠️ AI平仓信号: 已产生但跳过\n来源: %s\n原因: %s\n详情: 无Web客户端配置", source, reason)
	}
	if pos.PosId == "" {
		return fmt.Sprintf("\n\n⚠️ AI平仓信号: 已产生但跳过\n来源: %s\n原因: %s\n详情: 无PositionID", source, reason)
	}

	req, created := am.aiApprovalStore.createOrGetExisting(acc, pos, alertKey, source, reason, triggerType, pct)
	if !created {
		return fmt.Sprintf("\n\n⏳ AI平仓: 已有待审批请求\n请求ID: %s\n过期时间: %s", req.ID, req.ExpiresAt.Format("2006-01-02 15:04:05"))
	}

	if am.telegramClient != nil {
		mention := GetTelegramBotMention()
		if mention == "" {
			mention = "@你的Bot"
		}
		msg := fmt.Sprintf(
			"🤖 AI平仓待人工确认\n"+
				"请求ID: %s\n"+
				"账户: %s\n"+
				"仓位: %s %s\n"+
				"持仓ID: %s\n"+
				"开仓均价: %s  最新价: %s\n"+
				"当前盈亏: %s%%\n"+
				"信号来源: %s\n"+
				"原因: %s\n"+
				"过期时间: %s\n\n"+
				"确认命令: %s 确认平仓 %s\n"+
				"拒绝命令: %s 拒绝平仓 %s",
			req.ID,
			acc.Name,
			pos.InstId, pos.PosSide,
			pos.PosId,
			pos.AvgPx, pos.LastPx,
			req.PnLPercent,
			source,
			reason,
			req.ExpiresAt.Format("2006-01-02 15:04:05"),
			mention, req.ID,
			mention, req.ID,
		)
		if success, err := am.telegramClient.SendMessage(msg); err != nil || !success {
			_, _ = am.aiApprovalStore.reject(req.ID)
			if err != nil {
				logrus.Errorf("[AI平仓] 发送人工审批消息失败: %v", err)
				return fmt.Sprintf("\n\n⚠️ AI平仓: 发送人工审批消息失败\n原因: %v", err)
			}
			return "\n\n⚠️ AI平仓: 发送人工审批消息失败"
		}
	}

	logrus.Infof("[AI平仓] 已创建待审批请求: %s, alertKey=%s", req.ID, alertKey)
	return fmt.Sprintf("\n\n⏳ AI平仓: 已发送人工审批\n请求ID: %s\n过期时间: %s", req.ID, req.ExpiresAt.Format("2006-01-02 15:04:05"))
}

func (am *AccountMonitor) ApprovePendingAIClose(id string) string {
	if am == nil {
		return "⚠️ 账户监控器未初始化"
	}
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法执行审批平仓"
	}

	req, msg := am.aiApprovalStore.take(strings.TrimSpace(id))
	if req == nil {
		return msg
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	alertKey := fmt.Sprintf("%s:%s:%s", req.Account.Name, req.Position.InstId, req.Position.PosSide)
	result := am.closePositionBySignal(tm, req.Account, req.Position, alertKey, "ai-manual-approved", req.Reason)
	return fmt.Sprintf("✅ 已确认执行 AI 平仓\n请求ID: %s%s", req.ID, result)
}

func (am *AccountMonitor) RejectPendingAIClose(id string) string {
	if am == nil {
		return "⚠️ 账户监控器未初始化"
	}

	req, msg := am.aiApprovalStore.reject(strings.TrimSpace(id))
	if req == nil {
		return msg
	}

	logrus.Infof("[AI平仓] 已拒绝待审批请求: %s, alertKey=%s", req.ID, req.AlertKey)
	return fmt.Sprintf("🛑 已拒绝 AI 平仓请求\n请求ID: %s\n仓位: %s %s", req.ID, req.Position.InstId, req.Position.PosSide)
}

func (am *AccountMonitor) ListPendingAICloseRequests() string {
	if am == nil {
		return "⚠️ 账户监控器未初始化"
	}

	items := am.aiApprovalStore.list()
	if len(items) == 0 {
		return "✅ 当前没有待审批的 AI 平仓请求"
	}

	var b strings.Builder
	b.WriteString("📝 待审批 AI 平仓请求列表\n")
	for _, req := range items {
		fmt.Fprintf(&b,
			"\n%s\n账户: %s\n仓位: %s %s\n盈亏: %s%%\n过期: %s\n",
			req.ID,
			req.Account.Name,
			req.Position.InstId,
			req.Position.PosSide,
			req.PnLPercent,
			req.ExpiresAt.Format("2006-01-02 15:04:05"),
		)
	}
	return strings.TrimSpace(b.String())
}
