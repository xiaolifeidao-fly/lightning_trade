package argus_config

import (
	"context"
	"errors"
	"strings"
	"time"

	commonRedis "common/middleware/redis"
	argusDTO "service/argus_config/dto"
	"service/argus_config/repository"

	goRedis "github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// awaitingWindow 发布后多久之内把「心跳还没对上」算作正在等待生效，而不是漂移。
// 心跳 5s 上报 / 15s TTL，加上进程重载快照的时间，60s 足够覆盖一次正常生效。
const awaitingWindow = 60 * time.Second

// 生效状态取值与 r7 参数页前端同一套词表，两边不要各造一套。
const (
	EffectStateEffective = "effective"
	EffectStateAwaiting  = "awaiting"
	EffectStateDrift     = "drift"
	EffectStateOffline   = "offline"
	EffectStateUnknown   = "unknown"
)

// instanceOverviewNotice 是总览页与实例对比页共用的固定提示。三个实例的参数本来
// 就不同（阈值 5bp/3bp、上限 15/26+8/246、下单 1/10 张），版本号也是各自独立自增的。
const instanceOverviewNotice = "参数按实例分域：版本号是实例内自增的，跨实例比大小没有意义；" +
	"胜率、盈亏、信号数这类指标也不能跨实例相加。漂移只看同一实例的「已发布版本」与「心跳回报版本」是否一致。"

// InstanceOverview 汇总实例注册表、各实例的已发布版本与心跳生效状态。
//
// 之所以做成一个接口而不是让前端按实例循环调 /argus-config/published +
// /argus/runtime/status：① 三实例就是 6 次往返，页面每 10 秒轮询一次代价明显；
// ② /published 会带出账户与会话子表，总览页只要一个版本号，没必要把整份快照拉过来；
// ③ 漂移判定散在前端会和参数页各写一份，两处口径迟早分叉。
//
// Redis 不可用时不整体失败：注册表与已发布版本仍然返回，心跳段留空并标记 offline——
// 总览页宁可显示「读不到心跳」，也不能因为 Redis 抖动整页空白。
func (s *ArgusConfigService) InstanceOverview(ctx context.Context, onlyEnabled bool) (*argusDTO.InstanceOverviewDTO, error) {
	if s.repository == nil || s.repository.Db == nil {
		return nil, errors.New("database is not initialized")
	}
	instances, err := s.repository.ListInstances(onlyEnabled)
	if err != nil {
		return nil, err
	}
	duplicates, err := s.repository.DuplicateInstanceKeys()
	if err != nil {
		// 重复检测失败不该拖垮整页，降级成空列表并留日志。
		logrus.Warnf("Argus 实例键重复检测失败: %v", err)
		duplicates = nil
	}

	now := time.Now()
	result := &argusDTO.InstanceOverviewDTO{
		Instances:             make([]argusDTO.InstanceRuntimeDTO, 0, len(instances)),
		DuplicateInstanceKeys: duplicates,
		DriftInstanceKeys:     make([]string, 0),
		OfflineInstanceKeys:   make([]string, 0),
		Notice:                instanceOverviewNotice,
	}
	for _, instance := range instances {
		item := argusDTO.InstanceRuntimeDTO{
			InstanceKey:  instance.InstanceKey,
			InstanceName: instance.InstanceName,
			Description:  instance.Description,
			ConfigSource: instance.ConfigSource,
			Enabled:      instance.Enabled,
		}
		s.fillPublished(ctx, &item)
		s.fillHeartbeat(ctx, &item, now)
		applyEffectState(&item, now)

		if item.EffectState == EffectStateOffline {
			result.OfflineInstanceKeys = append(result.OfflineInstanceKeys, item.InstanceKey)
		}
		if item.EffectState == EffectStateDrift || item.EffectState == EffectStateAwaiting {
			result.DriftInstanceKeys = append(result.DriftInstanceKeys, item.InstanceKey)
		}
		result.Instances = append(result.Instances, item)
	}
	return result, nil
}

// fillPublished 读该实例当前 published 槽位上的版本行。没有已发布版本是正常状态
// （只导入了草稿），此时 PublishedVersion 保持 0，由 applyEffectState 判成 unknown。
func (s *ArgusConfigService) fillPublished(ctx context.Context, item *argusDTO.InstanceRuntimeDTO) {
	version, err := s.repository.FindPublishedContext(ctx, item.InstanceKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if err != nil {
		logrus.Warnf("读取 Argus 已发布版本失败: instanceKey=%s err=%v", item.InstanceKey, err)
		return
	}
	fillPublishedFrom(item, version)
}

func fillPublishedFrom(item *argusDTO.InstanceRuntimeDTO, version *repository.ArgusConfigVersion) {
	if version == nil {
		return
	}
	item.PublishedVersion = version.Version
	item.PublishedChecksum = version.SnapshotChecksum
	item.PublishedAt = version.PublishedAt
	item.PublishedBy = version.PublishedBy
}

// fillHeartbeat 读 argus:heartbeat:<instanceKey>。键不存在（TTL 已过或进程没起）
// 时保持 Online=false，不当成错误——实例本来就允许是停的。
func (s *ArgusConfigService) fillHeartbeat(ctx context.Context, item *argusDTO.InstanceRuntimeDTO, now time.Time) {
	heartbeat, err := commonRedis.ReadHeartbeat(ctx, item.InstanceKey)
	if err != nil {
		if !errors.Is(err, goRedis.Nil) && !errors.Is(err, commonRedis.ErrRedisNotInitialized) {
			logrus.Warnf("读取 Argus 心跳失败: instanceKey=%s err=%v", item.InstanceKey, err)
		}
		return
	}
	fillHeartbeatFrom(item, heartbeat, now)
}

func fillHeartbeatFrom(item *argusDTO.InstanceRuntimeDTO, heartbeat commonRedis.ArgusHeartbeat, now time.Time) {
	item.Online = true
	item.RunningVersion = heartbeat.Version
	item.RunningChecksum = heartbeat.ConfigChecksum
	item.Health = heartbeat.Health
	item.Pid = heartbeat.PID
	item.BuildVersion = heartbeat.BuildVersion
	item.LastReloadAt = heartbeat.LastReloadAt
	item.LastReloadSuccess = heartbeat.LastReloadSuccess
	item.LastReloadError = heartbeat.LastReloadError
	if !heartbeat.StartedAt.IsZero() {
		startedAt := heartbeat.StartedAt
		item.StartedAt = &startedAt
	}
	if !heartbeat.UpdatedAt.IsZero() {
		updatedAt := heartbeat.UpdatedAt
		item.HeartbeatAt = &updatedAt
		age := int(now.Sub(updatedAt).Round(time.Second) / time.Second)
		if age < 0 {
			// 管理端与实例机器的时钟不一定完全同步，负数没有意义，压到 0。
			age = 0
		}
		item.HeartbeatAgeSeconds = &age
	}
}

// applyEffectState 生效判定只认心跳：published 只说明管理端写成功了，程序有没有
// 真读到要看心跳回报的 version + checksum。校验和缺失（老构建）时退化成只比版本号。
//
// 与参数页前端的唯一差别：那边知道「刚刚是不是人点了发布」，这边只能按 publishedAt
// 距今是否在 awaitingWindow 内近似，超过就算漂移。
func applyEffectState(item *argusDTO.InstanceRuntimeDTO, now time.Time) {
	if item.PublishedVersion == 0 {
		item.EffectState = EffectStateUnknown
		return
	}
	if !item.Online {
		item.EffectState = EffectStateOffline
		return
	}
	versionMatched := item.RunningVersion == item.PublishedVersion
	checksumMatched := strings.TrimSpace(item.RunningChecksum) == "" ||
		item.RunningChecksum == item.PublishedChecksum
	item.VersionDrift = !versionMatched
	item.ChecksumDrift = versionMatched && !checksumMatched
	if versionMatched && checksumMatched {
		item.EffectState = EffectStateEffective
		return
	}
	if item.PublishedAt != nil && now.Sub(*item.PublishedAt) <= awaitingWindow {
		item.EffectState = EffectStateAwaiting
		return
	}
	item.EffectState = EffectStateDrift
}
