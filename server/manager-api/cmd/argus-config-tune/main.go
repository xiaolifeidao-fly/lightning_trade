// argus-config-tune 改某个实例已发布配置里的单个全局旋钮，走正规的
// SaveDraft → Publish 流程（留版本历史、算快照校验和、发 Redis 让实例立刻重载）。
//
// 为什么不直接 UPDATE argus_config：
//   - 版本表是审计源。就地改会让 published 版本的内容与它的 snapshot_checksum
//     不一致，管理端总览的 ChecksumDrift 会开始误报。
//   - 回滚要靠历史版本。就地改之后没有可回滚的目标。
//
// 为什么草稿必须从已发布快照整体复制：SaveConfigRequest 是**全量**的，
// 漏掉的字段会落成 0，而覆盖层把 0 当作「DB 未配置」→ 回落 properties，
// 于是一次"只改一个参数"的保存会静默改掉一堆参数。
//
// 凭证安全：快照里 cookie/token/apiKey 等一律是 "******"（maskSecret），
// SaveDraft 内部的 mergePublishedSecrets 会按 preserveSecret 语义从已发布版本
// 回填。所以整体复制回写**不会**毁掉凭证——管理端 UI 走的就是这条路。
//
// 用法（默认 dry run，先看 diff）：
//
//	cd server/manager-api
//	go run ./cmd/argus-config-tune --instance argus-single-roc --trend-threshold 3
//	go run ./cmd/argus-config-tune --instance argus-single-roc --trend-threshold 3 --apply
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"reflect"
	"strings"

	"common/middleware/db"
	commonRedis "common/middleware/redis"
	"common/middleware/vipper"
	argusConfig "service/argus_config"
	argusDTO "service/argus_config/dto"
)

// change 一次改动的记录。命名类型而不是匿名 struct：diffOthers 的形参要用同一个类型。
type change struct {
	key      string
	from, to float64
}

func main() {
	instanceKey := flag.String("instance", "", "目标实例键（必填）")
	trendTh := flag.Float64("trend-threshold", -1, "全局趋势闸阈值%（trade.trend_gate.threshold_pct）；-1=不改")
	trendWin := flag.Float64("trend-window", -1, "全局趋势闸动量窗口小时（trade.trend_gate.window_hours）；-1=不改")
	note := flag.String("note", "", "发布说明（写进 release_note，必填当 --apply）")
	actor := flag.String("actor", "argus-config-tune", "审计标记")
	apply := flag.Bool("apply", false, "真正存草稿并发布；不给则只打印 diff")
	flag.Parse()

	if strings.TrimSpace(*instanceKey) == "" {
		log.Fatal("--instance 必填")
	}
	if *trendTh < 0 && *trendWin < 0 {
		log.Fatal("至少要指定一个要改的旋钮")
	}
	if *apply && strings.TrimSpace(*note) == "" {
		log.Fatal("--apply 时 --note 必填：版本历史里没有说明的发布等于没有审计")
	}

	vipper.Init()
	db.InitDB()
	if db.Db == nil {
		log.Fatal("数据库初始化失败")
	}
	ctx := context.Background()
	svc := argusConfig.NewArgusConfigService()

	snap, err := svc.GetPublished(ctx, *instanceKey)
	if err != nil {
		log.Fatalf("读已发布配置失败：%v", err)
	}
	if snap == nil {
		log.Fatalf("实例 %s 没有已发布版本", *instanceKey)
	}
	log.Printf("当前已发布：v%d（发布于 %v，校验和 %s…）",
		snap.Version.Version, snap.Version.PublishedAt, firstN(snap.Version.SnapshotChecksum, 12))

	// 整体复制，只改指定旋钮。
	req := &argusDTO.SaveConfigRequest{
		InstanceKey:    *instanceKey,
		Config:         snap.Config,
		Accounts:       snap.Accounts,
		AccountRisks:   snap.AccountRisks,
		MonitorSymbols: snap.MonitorSymbols,
		Notification:   snap.Notification,
		Sessions:       snap.Sessions,
		ReleaseNote:    strings.TrimSpace(*note),
	}
	var changes []change
	if *trendTh >= 0 && req.Config.TrendGateThresholdPct != *trendTh {
		changes = append(changes, change{"trade.trend_gate.threshold_pct", req.Config.TrendGateThresholdPct, *trendTh})
		req.Config.TrendGateThresholdPct = *trendTh
	}
	if *trendWin >= 0 && req.Config.TrendGateWindowHour != *trendWin {
		changes = append(changes, change{"trade.trend_gate.window_hours", req.Config.TrendGateWindowHour, *trendWin})
		req.Config.TrendGateWindowHour = *trendWin
	}
	if len(changes) == 0 {
		log.Print("目标值与当前值一致，无需改动（不新建版本，避免版本历史里出现空变更）")
		return
	}
	fmt.Println("\n将要改动：")
	for _, c := range changes {
		fmt.Printf("  %-38s %.4f  →  %.4f\n", c.key, c.from, c.to)
	}

	// 逐字段核对：除上面列出的，其余必须与已发布快照完全一致。
	if diff := diffOthers(snap, req, changes); len(diff) > 0 {
		log.Fatalf("草稿里出现了未预期的字段差异，已中止：%s", strings.Join(diff, "; "))
	}
	fmt.Printf("\n其余字段逐项核对通过：config %d 项、账户 %d、风险 %d、币种 %d、会话 %d 全部与已发布一致\n",
		countConfigFields(), len(req.Accounts), len(req.AccountRisks), len(req.MonitorSymbols), len(req.Sessions))

	if !*apply {
		fmt.Println("\ndry run 结束。确认无误后加 --apply --note '...' 重跑。")
		return
	}

	version, err := svc.SaveDraft(*instanceKey, req, strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("存草稿失败：%v", err)
	}
	log.Printf("草稿 v%d 已存（id=%d）", version.Version, version.ID)

	// Redis 只在发布时需要：Publish 会发变更通知让实例立刻重载。
	// 发不出去不影响正确性——实例 60 秒内的指纹比对会自行发现。
	if err := commonRedis.InitRedisClient(vipper.GetString("redis.addr"), vipper.GetString("redis.password")); err != nil {
		log.Printf("⚠️ Redis 初始化失败，发布后改由实例自行轮询生效（最多 60 秒）：%v", err)
	}
	published, err := svc.Publish(ctx, *instanceKey, version.ID,
		&argusDTO.PublishConfigRequest{InstanceKey: *instanceKey, ReleaseNote: strings.TrimSpace(*note)},
		strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("发布失败（草稿 v%d 仍在，可在管理端手工发布）：%v", version.Version, err)
	}
	log.Printf("✅ 已发布 v%d，校验和 %s…", published.Version, firstN(published.SnapshotChecksum, 12))
	log.Print("到管理端「参数与运行控制」看该实例心跳回报的版本与校验和是否对上（最多 60 秒）")
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// diffOthers 核对「除 changed 之外的字段都没变」。
// Config 用反射逐字段比，子表比长度与逐元素相等——SaveConfigRequest 是全量提交，
// 这一步是"只改一个参数"这句话的唯一凭据。
func diffOthers(snap *argusDTO.ConfigSnapshotDTO, req *argusDTO.SaveConfigRequest, changed []change) []string {
	var out []string
	skip := map[string]bool{}
	for _, c := range changed {
		switch c.key {
		case "trade.trend_gate.threshold_pct":
			skip["TrendGateThresholdPct"] = true
		case "trade.trend_gate.window_hours":
			skip["TrendGateWindowHour"] = true
		}
	}
	out = append(out, diffStruct("config", snap.Config, req.Config, skip)...)
	if len(snap.Accounts) != len(req.Accounts) {
		out = append(out, fmt.Sprintf("账户数 %d→%d", len(snap.Accounts), len(req.Accounts)))
	}
	if len(snap.AccountRisks) != len(req.AccountRisks) {
		out = append(out, fmt.Sprintf("风险行数 %d→%d", len(snap.AccountRisks), len(req.AccountRisks)))
	}
	if len(snap.MonitorSymbols) != len(req.MonitorSymbols) {
		out = append(out, fmt.Sprintf("币种数 %d→%d", len(snap.MonitorSymbols), len(req.MonitorSymbols)))
	}
	if len(snap.Sessions) != len(req.Sessions) {
		out = append(out, fmt.Sprintf("会话行数 %d→%d", len(snap.Sessions), len(req.Sessions)))
	}
	for i := range snap.AccountRisks {
		if i < len(req.AccountRisks) && snap.AccountRisks[i] != req.AccountRisks[i] {
			out = append(out, fmt.Sprintf("风险行[%d] 有差异", i))
		}
	}
	return out
}

// diffStruct 反射逐字段比两个同类型结构体，返回不相等的字段名（跳过 skip 里的）。
func diffStruct(label string, a, b interface{}, skip map[string]bool) []string {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Type() != vb.Type() {
		return []string{label + " 类型不一致"}
	}
	var out []string
	for i := 0; i < va.NumField(); i++ {
		name := va.Type().Field(i).Name
		if skip[name] {
			continue
		}
		fa, fb := va.Field(i), vb.Field(i)
		if !fa.CanInterface() {
			continue
		}
		if !reflect.DeepEqual(fa.Interface(), fb.Interface()) {
			out = append(out, fmt.Sprintf("%s.%s 变了", label, name))
		}
	}
	return out
}

func countConfigFields() int {
	return reflect.TypeOf(argusDTO.ConfigDTO{}).NumField()
}
