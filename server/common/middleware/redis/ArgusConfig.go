package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goRedis "github.com/go-redis/redis"
)

const (
	ArgusConfigChannel   = "argus:config:changed"
	ArgusHeartbeatPrefix = "argus:heartbeat:"
	ArgusControlChannel  = "argus:control"
	// ArgusConfigPrefix 是配置键的实例命名空间前缀，完整键形如
	// argus:config:{instanceKey}:snapshot，不同部署实例互不覆盖。
	ArgusConfigPrefix = "argus:config:"
)

var ErrRedisNotInitialized = errors.New("redis is not initialized")

var ErrInstanceKeyRequired = errors.New("argus instance key is required")

// ArgusConfigVersionKey 返回某个实例当前已发布版本号的键。
func ArgusConfigVersionKey(instanceKey string) string {
	return ArgusConfigPrefix + instanceKey + ":version"
}

// ArgusConfigSnapshotKey 返回某个实例当前生效快照的键。
func ArgusConfigSnapshotKey(instanceKey string) string {
	return ArgusConfigPrefix + instanceKey + ":snapshot"
}

// ArgusConfigSnapshotVersionKey 返回某个实例指定版本的历史快照键，用于回滚查阅。
func ArgusConfigSnapshotVersionKey(instanceKey string, version uint64) string {
	return ArgusConfigPrefix + instanceKey + ":snapshot:" + fmt.Sprintf("%d", version)
}

type ConfigSnapshotEnvelope struct {
	// InstanceID 是快照所属实例键，读取时用于兜底校验，避免错读到别的实例快照。
	InstanceID string          `json:"instanceId,omitempty"`
	Version    uint64          `json:"version"`
	Checksum   string          `json:"checksum"`
	CreatedAt  time.Time       `json:"createdAt"`
	Payload    json.RawMessage `json:"payload"`
}

type ConfigVersionMessage struct {
	// InstanceID 是目标实例键，订阅方必须过滤掉不属于自己的版本消息。
	InstanceID string `json:"instanceId,omitempty"`
	Version    uint64 `json:"version"`
	Checksum   string `json:"checksum"`
}

type ArgusHeartbeat struct {
	InstanceID        string     `json:"instanceId"`
	PID               int        `json:"pid"`
	StartedAt         time.Time  `json:"startedAt"`
	BuildVersion      string     `json:"buildVersion"`
	Version           uint64     `json:"version"`
	LastReloadAt      *time.Time `json:"lastReloadAt,omitempty"`
	LastReloadSuccess *bool      `json:"lastReloadSuccess,omitempty"`
	LastReloadError   string     `json:"lastReloadError,omitempty"`
	Health            string     `json:"health"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type ArgusControlMessage struct {
	Action      string    `json:"action"`
	InstanceID  string    `json:"instanceId,omitempty"`
	RequestedAt time.Time `json:"requestedAt"`
}

func client() (*goRedis.Client, error) {
	if Rdb == nil {
		return nil, ErrRedisNotInitialized
	}
	return Rdb, nil
}

func SetContext(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	return rdb.WithContext(ctx).Set(key, value, expiration).Err()
}

func GetContext(ctx context.Context, key string) (string, error) {
	rdb, err := client()
	if err != nil {
		return "", err
	}
	return rdb.WithContext(ctx).Get(key).Result()
}

func PublishContext(ctx context.Context, channel string, message interface{}) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	return rdb.WithContext(ctx).Publish(channel, message).Err()
}

func SubscribeContext(ctx context.Context, channels ...string) (*goRedis.PubSub, error) {
	rdb, err := client()
	if err != nil {
		return nil, err
	}
	return rdb.WithContext(ctx).Subscribe(channels...), nil
}

func WriteConfigSnapshot(ctx context.Context, instanceKey string, version uint64, payload interface{}, expiration time.Duration) (ConfigSnapshotEnvelope, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return ConfigSnapshotEnvelope{}, ErrInstanceKeyRequired
	}
	if version == 0 {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("config version must be positive")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("marshal config snapshot: %w", err)
	}
	digest := sha256.Sum256(encoded)
	envelope := ConfigSnapshotEnvelope{
		InstanceID: instanceKey,
		Version:    version,
		Checksum:   hex.EncodeToString(digest[:]),
		CreatedAt:  time.Now().UTC(),
		Payload:    encoded,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("marshal config snapshot envelope: %w", err)
	}
	key := ArgusConfigSnapshotVersionKey(instanceKey, version)
	activeKey := ArgusConfigSnapshotKey(instanceKey)
	if err := SetContext(ctx, key, data, expiration); err != nil {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("write config snapshot: %w", err)
	}
	if err := SetContext(ctx, activeKey, data, expiration); err != nil {
		_ = DeleteContext(context.Background(), key)
		return ConfigSnapshotEnvelope{}, fmt.Errorf("activate config snapshot: %w", err)
	}
	if err := SetContext(ctx, ArgusConfigVersionKey(instanceKey), version, expiration); err != nil {
		_ = DeleteContext(context.Background(), key)
		_ = DeleteContext(context.Background(), activeKey)
		return ConfigSnapshotEnvelope{}, fmt.Errorf("write config version: %w", err)
	}
	return envelope, nil
}

func ReadConfigSnapshot(ctx context.Context, instanceKey string) (ConfigSnapshotEnvelope, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return ConfigSnapshotEnvelope{}, ErrInstanceKeyRequired
	}
	data, err := GetContext(ctx, ArgusConfigSnapshotKey(instanceKey))
	if err != nil {
		return ConfigSnapshotEnvelope{}, err
	}
	var envelope ConfigSnapshotEnvelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("decode config snapshot: %w", err)
	}
	digest := sha256.Sum256(envelope.Payload)
	if hex.EncodeToString(digest[:]) != envelope.Checksum {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("config snapshot checksum mismatch")
	}
	// 键已按实例命名空间隔离，快照体内的实例标识只用于兜底校验错读。
	if envelope.InstanceID != "" && envelope.InstanceID != instanceKey {
		return ConfigSnapshotEnvelope{}, fmt.Errorf("config snapshot instance mismatch: snapshot=%s want=%s", envelope.InstanceID, instanceKey)
	}
	return envelope, nil
}

// DeleteConfigSnapshot 清除某个实例当前生效的快照与版本键，只影响该实例。
func DeleteConfigSnapshot(ctx context.Context, instanceKey string) error {
	if strings.TrimSpace(instanceKey) == "" {
		return ErrInstanceKeyRequired
	}
	return DeleteContext(ctx, ArgusConfigSnapshotKey(instanceKey), ArgusConfigVersionKey(instanceKey))
}

func PublishConfigVersion(ctx context.Context, instanceKey string, version uint64, checksum string) error {
	if strings.TrimSpace(instanceKey) == "" {
		return ErrInstanceKeyRequired
	}
	if version == 0 || checksum == "" {
		return fmt.Errorf("version and checksum are required")
	}
	data, err := json.Marshal(ConfigVersionMessage{InstanceID: instanceKey, Version: version, Checksum: checksum})
	if err != nil {
		return fmt.Errorf("marshal config version message: %w", err)
	}
	return PublishContext(ctx, ArgusConfigChannel, data)
}

func SetHeartbeat(ctx context.Context, instanceID string, value interface{}, ttl time.Duration) error {
	if instanceID == "" {
		return fmt.Errorf("heartbeat instance id is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("heartbeat ttl must be positive")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	return SetContext(ctx, ArgusHeartbeatPrefix+instanceID, data, ttl)
}

func ReadHeartbeat(ctx context.Context, instanceID string) (ArgusHeartbeat, error) {
	if instanceID == "" {
		return ArgusHeartbeat{}, fmt.Errorf("heartbeat instance id is required")
	}
	data, err := GetContext(ctx, ArgusHeartbeatPrefix+instanceID)
	if err != nil {
		return ArgusHeartbeat{}, err
	}
	var heartbeat ArgusHeartbeat
	if err := json.Unmarshal([]byte(data), &heartbeat); err != nil {
		return ArgusHeartbeat{}, fmt.Errorf("decode argus heartbeat: %w", err)
	}
	if heartbeat.InstanceID == "" {
		return ArgusHeartbeat{}, fmt.Errorf("argus heartbeat instance id is required")
	}
	return heartbeat, nil
}

func PublishArgusControl(ctx context.Context, action, instanceID string) error {
	if action == "" {
		return fmt.Errorf("argus control action is required")
	}
	data, err := json.Marshal(ArgusControlMessage{
		Action:      action,
		InstanceID:  instanceID,
		RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal argus control message: %w", err)
	}
	return PublishContext(ctx, ArgusControlChannel, data)
}

func DeleteContext(ctx context.Context, keys ...string) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	return rdb.WithContext(ctx).Del(keys...).Err()
}
