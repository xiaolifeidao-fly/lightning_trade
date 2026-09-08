package argus_runtime

import (
	"context"
	"fmt"
	"sync"

	commonRedis "common/middleware/redis"

	goRedis "github.com/go-redis/redis"
)

// ActionReload 是本服务唯一保留的运行控制动作。start / stop / restart 已下线：
// 停进程后持仓仍挂在交易所，但移动止盈、兜底止损与平仓监控全部失效，比不停更
// 危险；且 Control() 走的是本机 control.sh，对部署在别的机器上的实例根本不可达。
// 进程启停留在运维侧（SSH + script/control.sh），紧急刹车走参数热更新
// （order_size = 0，暂停新开仓但继续看管已有持仓）。
const ActionReload = "reload"

type Status struct {
	Online    bool                        `json:"online"`
	Heartbeat *commonRedis.ArgusHeartbeat `json:"heartbeat,omitempty"`
}

type ControlResult struct {
	Action string `json:"action"`
	Output string `json:"output,omitempty"`
}

type ArgusRuntimeService struct {
	controlMu sync.Mutex
}

func NewArgusRuntimeService() *ArgusRuntimeService {
	return &ArgusRuntimeService{}
}

func (s *ArgusRuntimeService) Status(ctx context.Context, instanceID string) (*Status, error) {
	heartbeat, err := commonRedis.ReadHeartbeat(ctx, instanceID)
	if err != nil {
		if err == goRedis.Nil {
			return &Status{Online: false}, nil
		}
		return nil, err
	}
	return &Status{Online: true, Heartbeat: &heartbeat}, nil
}

// Reload 通过 Redis 广播定向到某个实例，是唯一可以跨机器抵达的运行控制通道。
func (s *ArgusRuntimeService) Reload(ctx context.Context, instanceID string) (*ControlResult, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("argus instance id is required")
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if err := commonRedis.PublishArgusControl(ctx, ActionReload, instanceID); err != nil {
		return nil, fmt.Errorf("publish argus reload request: %w", err)
	}
	return &ControlResult{Action: ActionReload, Output: "reload request published"}, nil
}
