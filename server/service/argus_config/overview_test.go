package argus_config

import (
	"testing"
	"time"

	commonRedis "common/middleware/redis"
	argusDTO "service/argus_config/dto"
	"service/argus_config/repository"
)

// 生效判定是总览页与实例对比页唯一的告警依据，这里覆盖五种取值与两种漂移的分叉，
// 不依赖 DB / Redis。
func TestApplyEffectState(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	justPublished := now.Add(-10 * time.Second)
	longAgo := now.Add(-2 * time.Hour)

	cases := []struct {
		name         string
		item         argusDTO.InstanceRuntimeDTO
		wantState    string
		wantVersion  bool
		wantChecksum bool
	}{
		{
			name:      "没有已发布版本时判 unknown",
			item:      argusDTO.InstanceRuntimeDTO{Online: true},
			wantState: EffectStateUnknown,
		},
		{
			name:      "有已发布版本但没有心跳时判 offline",
			item:      argusDTO.InstanceRuntimeDTO{PublishedVersion: 37, PublishedChecksum: "aa"},
			wantState: EffectStateOffline,
		},
		{
			name: "版本与校验和都对上判 effective",
			item: argusDTO.InstanceRuntimeDTO{
				PublishedVersion: 37, PublishedChecksum: "aa",
				Online: true, RunningVersion: 37, RunningChecksum: "aa",
			},
			wantState: EffectStateEffective,
		},
		{
			name: "刚发布不久还没对上判 awaiting",
			item: argusDTO.InstanceRuntimeDTO{
				PublishedVersion: 38, PublishedChecksum: "bb", PublishedAt: &justPublished,
				Online: true, RunningVersion: 37, RunningChecksum: "aa",
			},
			wantState:   EffectStateAwaiting,
			wantVersion: true,
		},
		{
			name: "发布很久仍没对上判 drift",
			item: argusDTO.InstanceRuntimeDTO{
				PublishedVersion: 38, PublishedChecksum: "bb", PublishedAt: &longAgo,
				Online: true, RunningVersion: 37, RunningChecksum: "aa",
			},
			wantState:   EffectStateDrift,
			wantVersion: true,
		},
		{
			name: "版本号一致但校验和不同判 checksum 漂移",
			item: argusDTO.InstanceRuntimeDTO{
				PublishedVersion: 37, PublishedChecksum: "bb", PublishedAt: &longAgo,
				Online: true, RunningVersion: 37, RunningChecksum: "aa",
			},
			wantState:    EffectStateDrift,
			wantChecksum: true,
		},
		{
			name: "老构建不上报校验和时退化成只比版本号",
			item: argusDTO.InstanceRuntimeDTO{
				PublishedVersion: 37, PublishedChecksum: "bb",
				Online: true, RunningVersion: 37,
			},
			wantState: EffectStateEffective,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := tc.item
			applyEffectState(&item, now)
			if item.EffectState != tc.wantState {
				t.Fatalf("effectState = %s, want %s", item.EffectState, tc.wantState)
			}
			if item.VersionDrift != tc.wantVersion {
				t.Fatalf("versionDrift = %v, want %v", item.VersionDrift, tc.wantVersion)
			}
			if item.ChecksumDrift != tc.wantChecksum {
				t.Fatalf("checksumDrift = %v, want %v", item.ChecksumDrift, tc.wantChecksum)
			}
		})
	}
}

// 心跳年龄按「本地时钟 − 心跳 updatedAt」算，管理端与实例机器时钟不同步时不得回负数。
func TestFillHeartbeatFromAge(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	item := argusDTO.InstanceRuntimeDTO{InstanceKey: "argus-single-1"}
	fillHeartbeatFrom(&item, commonRedis.ArgusHeartbeat{
		InstanceID: "argus-single-1",
		Version:    37,
		UpdatedAt:  now.Add(-4 * time.Second),
		Health:     "ok",
	}, now)
	if !item.Online || item.HeartbeatAgeSeconds == nil || *item.HeartbeatAgeSeconds != 4 {
		t.Fatalf("heartbeat age = %v, want 4", item.HeartbeatAgeSeconds)
	}

	ahead := argusDTO.InstanceRuntimeDTO{InstanceKey: "argus-single-1"}
	fillHeartbeatFrom(&ahead, commonRedis.ArgusHeartbeat{
		InstanceID: "argus-single-1",
		UpdatedAt:  now.Add(3 * time.Second),
	}, now)
	if ahead.HeartbeatAgeSeconds == nil || *ahead.HeartbeatAgeSeconds != 0 {
		t.Fatalf("clock-skew age = %v, want 0", ahead.HeartbeatAgeSeconds)
	}
}

func TestFillPublishedFrom(t *testing.T) {
	publishedAt := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	item := argusDTO.InstanceRuntimeDTO{}
	fillPublishedFrom(&item, &repository.ArgusConfigVersion{
		Version:          37,
		SnapshotChecksum: "9f3c",
		PublishedAt:      &publishedAt,
		PublishedBy:      "ct",
	})
	if item.PublishedVersion != 37 || item.PublishedChecksum != "9f3c" || item.PublishedBy != "ct" {
		t.Fatalf("published fields not filled: %+v", item)
	}
	// nil 版本行是「该实例还没发布过」的正常状态，不能 panic。
	fillPublishedFrom(&item, nil)
}
