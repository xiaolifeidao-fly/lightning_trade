package argus_config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"common/middleware/vipper"
	argusDTO "service/argus_config/dto"
	"service/argus_config/repository"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	ErrInstanceKeyRequired  = errors.New("argus instance key is required")
	ErrInstanceNotFound     = errors.New("argus instance is not registered")
	ErrInstanceDisabled     = errors.New("argus instance is disabled")
	ErrInstanceKeyDuplicate = errors.New("argus instance key already exists")
	ErrInstanceKeyInvalid   = errors.New("argus instance key must match [A-Za-z0-9_.-]{1,64}")
)

// instanceKeyPattern 与 Redis 键命名空间共用，禁止冒号和空白避免键被拼歪。
var instanceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// NormalizeInstanceKey 统一裁剪并校验实例键格式，不做存在性校验。
func NormalizeInstanceKey(instanceKey string) (string, error) {
	key := strings.TrimSpace(instanceKey)
	if key == "" {
		return "", ErrInstanceKeyRequired
	}
	if !instanceKeyPattern.MatchString(key) {
		return "", fmt.Errorf("%w: %s", ErrInstanceKeyInvalid, key)
	}
	return key, nil
}

// ResolveInstanceKey 解析本次操作的实例键：显式入参优先，其次读取
// argus.instance.default_key / argus.instance.id，最后在注册表里只有一个启用实例
// 时兜底。多实例且未显式指定时必须报错，避免误发布到别的实例。
func (s *ArgusConfigService) ResolveInstanceKey(instanceKey string) (string, error) {
	if strings.TrimSpace(instanceKey) != "" {
		key, err := NormalizeInstanceKey(instanceKey)
		if err != nil {
			return "", err
		}
		return s.assertInstanceUsable(key)
	}
	for _, configKey := range []string{"argus.instance.default_key", "argus.instance.id"} {
		if candidate := strings.TrimSpace(vipper.GetString(configKey)); candidate != "" {
			key, err := NormalizeInstanceKey(candidate)
			if err != nil {
				return "", err
			}
			return s.assertInstanceUsable(key)
		}
	}
	instances, err := s.repository.ListInstances(true)
	if err != nil {
		return "", err
	}
	if len(instances) == 1 {
		return instances[0].InstanceKey, nil
	}
	return "", fmt.Errorf("%w: %d enabled instances registered, instanceKey must be provided", ErrInstanceKeyRequired, len(instances))
}

// assertInstanceUsable 校验实例已注册且处于启用状态。注册表为空时（首次导入前）
// 放行，避免 argus-config-import 陷入自举死锁。
func (s *ArgusConfigService) assertInstanceUsable(instanceKey string) (string, error) {
	instance, err := s.repository.FindInstanceByKey(instanceKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		instances, listErr := s.repository.ListInstances(false)
		if listErr != nil {
			return "", listErr
		}
		if len(instances) == 0 {
			logrus.Warnf("Argus 实例注册表为空，暂时放行实例键 %s，请尽快执行 argus-config-import 注册实例", instanceKey)
			return instanceKey, nil
		}
		return "", fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceKey)
	}
	if err != nil {
		return "", err
	}
	if instance.Enabled == 0 {
		return "", fmt.Errorf("%w: %s", ErrInstanceDisabled, instanceKey)
	}
	return instance.InstanceKey, nil
}

// ListInstances 返回实例注册表，并在发现同键重复记录时打印告警。
func (s *ArgusConfigService) ListInstances(onlyEnabled bool) ([]argusDTO.InstanceDTO, error) {
	s.warnDuplicateInstanceKeys()
	instances, err := s.repository.ListInstances(onlyEnabled)
	if err != nil {
		return nil, err
	}
	result := make([]argusDTO.InstanceDTO, 0, len(instances))
	for _, instance := range instances {
		result = append(result, instanceDTO(instance))
	}
	return result, nil
}

// RegisterInstance 注册一个新实例，实例键已存在时直接拒绝并告警。
func (s *ArgusConfigService) RegisterInstance(req *argusDTO.SaveInstanceRequest) (*argusDTO.InstanceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	key, err := NormalizeInstanceKey(req.InstanceKey)
	if err != nil {
		return nil, err
	}
	existing, err := s.repository.FindInstanceByKey(key)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil && err == nil {
		logrus.Warnf("Argus 实例键重复注册被拒绝: instanceKey=%s existingId=%d", key, existing.Id)
		return nil, fmt.Errorf("%w: %s", ErrInstanceKeyDuplicate, key)
	}
	instance := instanceEntity(key, req)
	if err := s.repository.CreateInstance(instance); err != nil {
		return nil, err
	}
	result := instanceDTO(instance)
	return &result, nil
}

// EnsureInstance 幂等注册：实例键已存在时补齐展示字段并复用原记录，
// 命中重复时打印告警，供 argus-config-import 反复执行。
func (s *ArgusConfigService) EnsureInstance(req *argusDTO.SaveInstanceRequest) (*argusDTO.InstanceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	key, err := NormalizeInstanceKey(req.InstanceKey)
	if err != nil {
		return nil, err
	}
	s.warnDuplicateInstanceKeys()
	existing, err := s.repository.FindInstanceByKey(key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		instance := instanceEntity(key, req)
		if err := s.repository.CreateInstance(instance); err != nil {
			return nil, err
		}
		result := instanceDTO(instance)
		return &result, nil
	}
	if err != nil {
		return nil, err
	}
	logrus.Warnf("Argus 实例键已注册，复用现有记录: instanceKey=%s id=%d", key, existing.Id)
	if strings.TrimSpace(req.InstanceName) != "" {
		existing.InstanceName = strings.TrimSpace(req.InstanceName)
	}
	if strings.TrimSpace(req.Description) != "" {
		existing.Description = strings.TrimSpace(req.Description)
	}
	if strings.TrimSpace(req.ConfigSource) != "" {
		existing.ConfigSource = strings.TrimSpace(req.ConfigSource)
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := s.repository.SaveInstance(existing); err != nil {
		return nil, err
	}
	result := instanceDTO(existing)
	return &result, nil
}

// warnDuplicateInstanceKeys 兜底检测历史库缺唯一索引导致的实例键重复。
func (s *ArgusConfigService) warnDuplicateInstanceKeys() {
	if s.repository == nil || s.repository.Db == nil {
		return
	}
	keys, err := s.repository.DuplicateInstanceKeys()
	if err != nil {
		logrus.Warnf("Argus 实例键重复检测失败: %v", err)
		return
	}
	if len(keys) > 0 {
		logrus.Errorf("Argus 实例键重复，配置发布可能串实例，请清理 argus_instance: %s", strings.Join(keys, ", "))
	}
}

func instanceEntity(key string, req *argusDTO.SaveInstanceRequest) *repository.ArgusInstance {
	name := strings.TrimSpace(req.InstanceName)
	if name == "" {
		name = key
	}
	enabled := uint8(1)
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return &repository.ArgusInstance{
		InstanceKey:  key,
		InstanceName: name,
		Description:  strings.TrimSpace(req.Description),
		ConfigSource: strings.TrimSpace(req.ConfigSource),
		Enabled:      enabled,
	}
}

func instanceDTO(v *repository.ArgusInstance) argusDTO.InstanceDTO {
	return argusDTO.InstanceDTO{ID: uint64(v.Id), InstanceKey: v.InstanceKey, InstanceName: v.InstanceName, Description: v.Description, ConfigSource: v.ConfigSource, Enabled: v.Enabled}
}
