package argus_runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	commonRedis "common/middleware/redis"

	goRedis "github.com/go-redis/redis"
)

const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionReload  = "reload"
)

type Status struct {
	Online    bool                        `json:"online"`
	Heartbeat *commonRedis.ArgusHeartbeat `json:"heartbeat,omitempty"`
}

type ControlResult struct {
	Action string `json:"action"`
	Output string `json:"output,omitempty"`
}

type ArgusRuntimeService struct {
	controlScript string
	timeout       time.Duration
	controlMu     sync.Mutex
}

func NewArgusRuntimeService(controlScript string) (*ArgusRuntimeService, error) {
	resolved, err := filepath.Abs(controlScript)
	if err != nil {
		return nil, fmt.Errorf("resolve argus control script: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, fmt.Errorf("resolve argus control script symlink: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat argus control script: %w", err)
	}
	if info.IsDir() || info.Mode()&0111 == 0 || filepath.Base(resolved) != "control.sh" {
		return nil, fmt.Errorf("argus control script must be an executable control.sh file")
	}
	return &ArgusRuntimeService{controlScript: resolved, timeout: 30 * time.Second}, nil
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

func (s *ArgusRuntimeService) Control(ctx context.Context, action string) (*ControlResult, error) {
	if action != ActionStart && action != ActionStop && action != ActionRestart {
		return nil, fmt.Errorf("unsupported argus control action: %s", action)
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()

	commandCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, s.controlScript, action).CombinedOutput()
	result := &ControlResult{Action: action, Output: strings.TrimSpace(string(output))}
	if err != nil {
		return result, fmt.Errorf("execute argus %s: %w", action, err)
	}
	return result, nil
}

func (s *ArgusRuntimeService) Reload(ctx context.Context, instanceID string) (*ControlResult, error) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if err := commonRedis.PublishArgusControl(ctx, ActionReload, instanceID); err != nil {
		return nil, fmt.Errorf("publish argus reload request: %w", err)
	}
	return &ControlResult{Action: ActionReload, Output: "reload request published"}, nil
}
