package runtimehealth

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	commonRedis "common/middleware/redis"
)

const (
	defaultInterval = 5 * time.Second
	defaultTTL      = 15 * time.Second
)

type Reporter struct {
	mu         sync.Mutex
	instanceID string
	interval   time.Duration
	ttl        time.Duration
	state      commonRedis.ArgusHeartbeat
	cancel     context.CancelFunc
}

var defaultReporter struct {
	sync.Mutex
	reporter *Reporter
}

func SetDefaultReporter(reporter *Reporter) {
	defaultReporter.Lock()
	previous := defaultReporter.reporter
	defaultReporter.reporter = reporter
	defaultReporter.Unlock()
	if previous != nil && previous != reporter {
		previous.Stop()
	}
}

func StopDefaultReporter() {
	defaultReporter.Lock()
	reporter := defaultReporter.reporter
	defaultReporter.reporter = nil
	defaultReporter.Unlock()
	if reporter != nil {
		reporter.Stop()
	}
}

func New(instanceID, buildVersion string, interval, ttl time.Duration) *Reporter {
	if strings.TrimSpace(instanceID) == "" {
		instanceID = "default"
	}
	if strings.TrimSpace(buildVersion) == "" {
		buildVersion = "dev"
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	if ttl <= interval {
		ttl = interval * 3
	}
	startedAt := time.Now().UTC()
	return &Reporter{
		instanceID: instanceID,
		interval:   interval,
		ttl:        ttl,
		state: commonRedis.ArgusHeartbeat{
			InstanceID:   instanceID,
			PID:          os.Getpid(),
			StartedAt:    startedAt,
			BuildVersion: buildVersion,
			Health:       "healthy",
		},
	}
}

func (r *Reporter) Start(ctx context.Context) {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	childCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()
	r.write(childCtx)
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-childCtx.Done():
				return
			case <-ticker.C:
				r.write(childCtx)
			}
		}
	}()
}

func (r *Reporter) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Reporter) SetVersion(version uint64) {
	r.mu.Lock()
	r.state.Version = version
	r.mu.Unlock()
}

func (r *Reporter) RecordReload(version uint64, err error) {
	now := time.Now().UTC()
	success := err == nil
	r.mu.Lock()
	if success && version != 0 {
		r.state.Version = version
	}
	r.state.LastReloadAt = &now
	r.state.LastReloadSuccess = &success
	if err != nil {
		r.state.LastReloadError = err.Error()
		r.state.Health = "degraded"
	} else {
		r.state.LastReloadError = ""
		r.state.Health = "healthy"
	}
	r.mu.Unlock()
}

func (r *Reporter) write(ctx context.Context) {
	r.mu.Lock()
	state := r.state
	state.UpdatedAt = time.Now().UTC()
	r.state.UpdatedAt = state.UpdatedAt
	r.mu.Unlock()
	if err := commonRedis.SetHeartbeat(ctx, r.instanceID, state, r.ttl); err != nil {
		log.Printf("Argus 心跳写入失败: %v", err)
	}
}
