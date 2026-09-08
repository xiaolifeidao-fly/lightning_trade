package eventstore

import (
	"crypto/md5"
	"strings"

	"argus_single/pkg/eventlog"
)

// EventHash 幂等哈希：md5(instanceKey + "\n" + 事件的规范 JSONL 行)。
//
// 为什么在 Go 侧算、不用 MySQL 生成列（设计文档 §5.1）：
// 不能用 UNIQUE(ts, uid, event, size, roi_pct)——MySQL 里 NULL 互不相等，
// 可空列进唯一键等于不去重；交给 MySQL 拼字符串又会让 Go 的浮点格式化与
// DECIMAL 渲染不一致，同一条事件在直写与回灌两条路径上算出两个哈希。
//
// 为什么直接哈希整行 JSON、不逐字段枚举：
//  1. 直写路径拿到的是 eventlog.Event，回灌路径拿到的是 eventlog.ParseFile
//     反序列化出的同一个结构体，两边都走 eventlog.Marshal ⇒ 字节级同源；
//  2. 逐字段枚举需要随 Event 加字段同步维护，漏一个就静默插重复行；
//  3. encoding/json 对 map 按 key 排序，DevCross/DevOver 天然确定。
//
// instanceKey 必须进哈希：实例1 与实例3 各有一个「account1」，是两个不同账户，
// 同一秒的同内容事件是两条真事件而不是重复行。
//
// 已知取舍：内容完全相同且落在同一秒的两条真事件会被判成重复。实测这不可达
// ——loss_alert 有 5 分钟冷却、balance 每分钟一条、dev_sample 带 devTicks、
// open 的 size 是递增净仓、信号类事件带 sigLast/gapBp 逐 tick 变化。
func EventHash(instanceKey string, e eventlog.Event) ([]byte, bool) {
	line := eventlog.Marshal(e)
	if line == "" {
		return nil, false
	}
	sum := md5.Sum([]byte(strings.TrimSpace(instanceKey) + "\n" + line))
	return sum[:], true
}
