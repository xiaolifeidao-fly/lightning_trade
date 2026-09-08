package eventstore

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// SliceRetention 秒级切片的保留期：90 天滚动（需求大纲 §3.1 已按此算过容量，
// 0.44GB/年 是"90 天在库"的稳态占用，不是无限累积）。
//
// 只有 signal_slice 参与滚动清理。三张事件表是 append-only 真源，任何"顺手也清一下"
// 都会把历史归因直接删掉——这个边界不能模糊。
const SliceRetention = 90 * 24 * time.Hour

const (
	// slicePurgeInterval 清理节奏。切片的过期是按天推进的，6 小时一轮已经远快于
	// 数据增长，不需要更频繁；进程刚起来时先清一次，好让长期停机后重启能收敛。
	slicePurgeInterval = 6 * time.Hour
	// slicePurgeBatch 单次 DELETE 的行数上限。一次删光会长时间持锁并撑爆 binlog，
	// 分批循环让每条语句都是短事务——这条连接与写入侧共用只有 4 个连接的池。
	slicePurgeBatch = 500
	// slicePurgeMaxBatches 单轮最多删几批，防止异常数据量把这一轮拖成长任务；
	// 没删完的下一轮继续。
	slicePurgeMaxBatches = 200
)

// StartSlicePurge 启动切片滚动清理（幂等）。调用方须先 Start：清理的退出条件
// 之一是 writer 收尾（那时连接会被关掉）。
//
// 与 writer 分开成独立 goroutine 而不是塞进 run()：清理是分钟级的慢 DELETE，
// 混进 flush 循环会让批量写入被它阻住，等于把"写库慢不许影响交易"这条前提
// 悄悄削弱。多实例同时清理无害——DELETE 是幂等的。
func (s *Store) StartSlicePurge(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	s.purgeOnce.Do(func() {
		go s.runSlicePurge(ctx)
	})
}

func (s *Store) runSlicePurge(ctx context.Context) {
	ticker := time.NewTicker(slicePurgeInterval)
	defer ticker.Stop()
	s.purgeOnceNow()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			// writer 收尾时连接就会被 Close 掉，清理必须跟着退出，
			// 否则下一轮 tick 会在已关闭的连接上报错刷屏。
			return
		case <-ticker.C:
			s.purgeOnceNow()
		}
	}
}

func (s *Store) purgeOnceNow() {
	deleted, err := s.PurgeExpiredSlices(time.Now())
	switch {
	case err != nil:
		// 清理失败只是磁盘多占一点，绝不升级成任何形式的中断。
		logrus.Errorf("[eventstore] signal_slice 滚动清理失败（不影响交易与写入）: %v", err)
	case deleted > 0:
		logrus.Infof("[eventstore] signal_slice 已清理 %d 条（保留期 %.0f 天）", deleted, SliceRetention.Hours()/24)
	}
}

// PurgeExpiredSlices 删掉 ts 早于 now-SliceRetention 的切片，分批执行。
// 返回本轮删除的行数。导出它是为了能在集成测试里直接验保留边界。
func (s *Store) PurgeExpiredSlices(now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	cutoff := now.Add(-SliceRetention)
	var total int64
	for i := 0; i < slicePurgeMaxBatches; i++ {
		res := s.db.Where("ts < ?", cutoff).Limit(slicePurgeBatch).Delete(&SignalSlice{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected < slicePurgeBatch {
			break
		}
	}
	return total, nil
}
