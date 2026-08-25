package monitor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"argus_single/pkg/trade"
)

type SignalDirection string

const (
	SignalDirectionUp   SignalDirection = "UP"
	SignalDirectionDown SignalDirection = "DOWN"
)

type SignalState string

const (
	SignalStateIdle          SignalState = "IDLE"
	SignalStateWaitLong      SignalState = "WAIT_LONG"
	SignalStateWaitShort     SignalState = "WAIT_SHORT"
	SignalStateLongPosition  SignalState = "LONG_POSITION"
	SignalStateShortPosition SignalState = "SHORT_POSITION"
)

type SignalSnapshot struct {
	Symbol       string
	TradeInst    string
	DeepInst     string
	Direction    SignalDirection
	State        SignalState
	DueAt        time.Time
	LastSignalAt time.Time
	LastExecAt   time.Time
	SignalCount  int
	Price        float64
	Source       string
	// Quote P7：信号时刻的 last/mark/gap_bp。与 Price 同生命周期、按 symbol 隔离
	// （同向叠加时一并刷新为最新信号的报价，与"fire 用最后一个信号价"的既有语义一致）。
	// 存在快照里而非全局缓存：延迟 5s 期间跨 symbol 的信号不得相互串扰。
	Quote trade.SignalQuote
}

type signalScheduleState struct {
	SignalSnapshot
	timer      *time.Timer
	generation int64
}

type SignalScheduler struct {
	mu     sync.Mutex
	delay  time.Duration
	states map[string]*signalScheduleState
}

func NewSignalScheduler(delay time.Duration) *SignalScheduler {
	if delay <= 0 {
		delay = 5 * time.Second
	}
	return &SignalScheduler{
		delay:  delay,
		states: make(map[string]*signalScheduleState),
	}
}

func NormalizeSignalDirection(direction string) (SignalDirection, bool) {
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case string(SignalDirectionUp):
		return SignalDirectionUp, true
	case string(SignalDirectionDown):
		return SignalDirectionDown, true
	default:
		return "", false
	}
}

func waitStateForDirection(direction SignalDirection) SignalState {
	if direction == SignalDirectionUp {
		return SignalStateWaitLong
	}
	return SignalStateWaitShort
}

func positionStateForDirection(direction SignalDirection) SignalState {
	if direction == SignalDirectionUp {
		return SignalStateLongPosition
	}
	return SignalStateShortPosition
}

func isWaitState(state SignalState) bool {
	return state == SignalStateWaitLong || state == SignalStateWaitShort
}

func (s *SignalScheduler) OnSignal(symbol string, config SymbolConfig, direction SignalDirection, price float64, source string, quote trade.SignalQuote) {
	now := time.Now()

	s.mu.Lock()
	state := s.states[symbol]
	if state == nil {
		state = &signalScheduleState{
			SignalSnapshot: SignalSnapshot{
				Symbol:    symbol,
				TradeInst: config.TradeInst,
				DeepInst:  config.DeepInst,
				State:     SignalStateIdle,
			},
		}
		s.states[symbol] = state
	}

	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}

	if isWaitState(state.State) && state.Direction == direction {
		state.DueAt = state.DueAt.Add(s.delay)
		state.SignalCount++
		logrus.Infof("[%s] 盘口%s信号叠加: 第%d次，执行时间延后至 %s",
			symbol, direction, state.SignalCount, state.DueAt.Format("15:04:05"))
	} else {
		if isWaitState(state.State) && state.Direction != direction {
			logrus.Infof("[%s] 盘口反向信号: 取消等待中的%s任务，改为%s", symbol, state.Direction, direction)
		}
		state.Direction = direction
		state.DueAt = now.Add(s.delay)
		state.SignalCount = 1
	}

	state.Symbol = symbol
	state.TradeInst = config.TradeInst
	state.DeepInst = config.DeepInst
	state.State = waitStateForDirection(direction)
	state.Price = price
	state.Source = source
	state.Quote = quote
	state.LastSignalAt = now
	state.generation++
	generation := state.generation
	waitDuration := time.Until(state.DueAt)
	if waitDuration < 0 {
		waitDuration = 0
	}
	state.timer = time.AfterFunc(waitDuration, func() {
		s.fire(symbol, generation)
	})

	snapshot := state.SignalSnapshot
	s.mu.Unlock()

	logrus.Infof("[%s] 盘口信号入队: signal=%s, state=%s, price=%.8f, source=%s, remaining=%s",
		snapshot.Symbol, snapshot.Direction, snapshot.State, snapshot.Price, snapshot.Source, time.Until(snapshot.DueAt).Round(time.Second))
}

func (s *SignalScheduler) fire(symbol string, generation int64) {
	s.mu.Lock()
	state := s.states[symbol]
	if state == nil || state.generation != generation || !isWaitState(state.State) {
		s.mu.Unlock()
		return
	}
	snapshot := state.SignalSnapshot
	state.timer = nil
	s.mu.Unlock()

	logrus.Infof("[%s] 盘口信号倒计时结束，执行下单: signal=%s, tradeInst=%s, price=%.8f",
		snapshot.Symbol, snapshot.Direction, snapshot.TradeInst, snapshot.Price)

	opened := executeSignalTradeInternal(snapshot.TradeInst, snapshot.Price, string(snapshot.Direction), snapshot.Quote)

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.states[symbol]
	if current == nil || current.generation != generation {
		return
	}
	current.LastExecAt = time.Now()
	current.timer = nil
	// A1: 只有确有账户开仓(success/opened)才进入 POSITION；全跳过/全失败 → IDLE，避免状态漂移
	current.State = nextStateAfterFire(opened, snapshot.Direction)
}

// nextStateAfterFire 根据"是否确有账户开仓"决定开仓后的调度器状态。
func nextStateAfterFire(opened bool, direction SignalDirection) SignalState {
	if opened {
		return positionStateForDirection(direction)
	}
	return SignalStateIdle
}

func (s *SignalScheduler) ResetAfterClose(instId, posSide string) {
	direction := SignalDirectionUp
	if strings.EqualFold(posSide, "short") {
		direction = SignalDirectionDown
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, state := range s.states {
		if !state.matchesInstID(instId) || state.Direction != direction {
			continue
		}
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		state.State = SignalStateIdle
		state.SignalCount = 0
		state.DueAt = time.Time{}
		logrus.Infof("[%s] 盘口信号状态已因平仓清空: instId=%s, posSide=%s", state.Symbol, instId, posSide)
	}
}

func (state *signalScheduleState) matchesInstID(instId string) bool {
	return instId == state.Symbol || instId == state.TradeInst || instId == state.DeepInst
}

func (s *SignalScheduler) Snapshots() []SignalSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshots := make([]SignalSnapshot, 0, len(s.states))
	for _, state := range s.states {
		snapshots = append(snapshots, state.SignalSnapshot)
	}
	return snapshots
}

func (s *SignalScheduler) StatusReport(listening bool) string {
	now := time.Now()
	status := "停止"
	if listening {
		status = "监听中"
	}

	message := fmt.Sprintf("📡 盘口信号状态\n监听状态: %s\n延迟: %s\n\n", status, s.delay)
	snapshots := s.Snapshots()
	if len(snapshots) == 0 {
		message += "最近信号: 暂无\n等待剩余时间: 无\n当前状态: IDLE"
		return message
	}

	for i, snapshot := range snapshots {
		if i > 0 {
			message += "\n"
		}
		remaining := "无"
		if isWaitState(snapshot.State) && snapshot.DueAt.After(now) {
			remaining = time.Until(snapshot.DueAt).Round(time.Second).String()
		}
		lastSignal := "暂无"
		if !snapshot.LastSignalAt.IsZero() {
			lastSignal = fmt.Sprintf("%s %s %.2f (%s)",
				snapshot.Direction, snapshot.LastSignalAt.Format("15:04:05"), snapshot.Price, snapshot.Source)
		}
		lastExec := "暂无"
		if !snapshot.LastExecAt.IsZero() {
			lastExec = snapshot.LastExecAt.Format("15:04:05")
		}

		message += fmt.Sprintf(
			"交易对: %s\n"+
				"当前状态: %s\n"+
				"最近信号: %s\n"+
				"等待剩余时间: %s\n"+
				"累计同向信号: %d\n"+
				"最近执行: %s\n",
			snapshot.Symbol,
			snapshot.State,
			lastSignal,
			remaining,
			snapshot.SignalCount,
			lastExec,
		)
	}
	return message
}

