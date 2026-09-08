package argus_event

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"common/middleware/db"
	argusConfigRepository "service/argus_config/repository"
	argusDTO "service/argus_event/dto"

	"argus_single/pkg/eventstore"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 真实 MySQL 集成校验。默认跳过（CI 与本地开发不一定有库），需要时显式给 DSN：
//
//	ARGUS_EVENT_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/argus_event_check' \
//	  go test ./argus_event/ -run TestIntegration -v
//
// 库会被反复建表并写入，请指向一个**可丢弃的 schema**，不要指向生产库。
//
// 实例键一律用 it- 前缀的测试专用值，绝不用真实部署的 argus.instance.id：
// 这些用例会 DELETE 自己那个实例键下的全部行，用真实键等于给"DSN 指错库"
// 配上一把删除生产历史的枪（r6 回灌的 16 万行历史事件挂在真实实例键下）。
const (
	itInstanceA  = "it-argus-event-a"
	itInstanceB  = "it-argus-event-b"
	itInstrument = "BTCUSDT"
)

// integrationService 建两条连接：
//   - 写连接走 eventstore.OpenDB，强制 loc=Local（与 argus_single 生产口径一致）；
//   - 读连接**故意用原始 DSN**（不补 loc），模拟 manager-api 那条由部署方给、
//     很可能是 go-sql-driver 默认 UTC 的连接。
//
// 两条连接 loc 不一致时读出来的墙钟仍必须与写入时逐字相同——这正是本包
// "时间全程按串走"这个设计要挡住的 8 小时偏移。
func integrationService(t *testing.T) (*ArgusEventService, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("ARGUS_EVENT_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 ARGUS_EVENT_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	writeDB, err := eventstore.OpenDB(dsn)
	if err != nil {
		t.Fatalf("打开写连接失败: %v", err)
	}
	readDB, err := gorm.Open(mysql.Open(readDSN(dsn)), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		t.Fatalf("打开读连接失败: %v", err)
	}
	if err := writeDB.AutoMigrate(eventstore.Models()...); err != nil {
		t.Fatalf("建事件表失败: %v", err)
	}
	db.Db = readDB
	svc := NewArgusEventService()
	// 每个仓储都是全局单例，前面的用例可能已经拿到过旧连接，这里强制指向本次的读连接。
	svc.strategyEventRepository.SetDb(readDB)
	svc.balanceRepository.SetDb(readDB)
	svc.devSampleRepository.SetDb(readDB)
	svc.signalSliceRepository.SetDb(readDB)
	svc.klineRepository.SetDb(readDB)
	if err := svc.klineRepository.EnsureTable(); err != nil {
		t.Fatalf("建 K 线表失败: %v", err)
	}
	// 实例注册表只用来补展示名：itInstanceA 注册、itInstanceB 故意不注册，
	// 好验证"事件里出现但没注册的实例仍然可见且标 registered=false"。
	if err := writeDB.AutoMigrate(&argusConfigRepository.ArgusInstance{}); err != nil {
		t.Fatalf("建实例注册表失败: %v", err)
	}
	if err := writeDB.Exec("DELETE FROM argus_instance WHERE instance_key IN ?", []string{itInstanceA, itInstanceB}).Error; err != nil {
		t.Fatalf("清理实例注册表失败: %v", err)
	}
	registered := &argusConfigRepository.ArgusInstance{InstanceKey: itInstanceA, InstanceName: "集成测试实例A", Enabled: 1}
	registered.Init()
	if err := writeDB.Create(registered).Error; err != nil {
		t.Fatalf("注册测试实例失败: %v", err)
	}
	seedIntegrationData(t, writeDB)
	return svc, writeDB
}

// readDSN 只补 parseTime=true（其余模块把 DATETIME 读成 time.Time 必须要它），
// **刻意不补 loc**：这样连接就退回 go-sql-driver 的默认 UTC，与写侧的
// loc=Local 恰好错开 8 小时——本包的读路径必须在这种情况下仍给出正确墙钟。
func readDSN(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "parseTime=true"
}

func itHash(seed string) []byte {
	h := make([]byte, 16)
	copy(h, seed)
	return h
}

func itTime(hhmmss string) time.Time {
	ts, err := time.ParseInLocation(eventTimeLayout, "2026-08-18 "+hhmmss, time.Local)
	if err != nil {
		panic(err)
	}
	return ts
}

// seedIntegrationData 清掉测试实例键下的旧数据后写入一组贴近真实形态的样本。
func seedIntegrationData(t *testing.T, conn *gorm.DB) {
	t.Helper()
	keys := []string{itInstanceA, itInstanceB}
	for _, table := range []string{"strategy_event", "balance_sample", "dev_sample", "signal_slice"} {
		if err := conn.Exec("DELETE FROM "+table+" WHERE instance_key IN ?", keys).Error; err != nil {
			t.Fatalf("清理 %s 失败: %v", table, err)
		}
	}
	ingested := time.Now()
	quote1Last, quote1Mark, quote1Gap := 64080.6, 64058.0, 3.5280527
	quote2Last, quote2Mark, quote2Gap := 64091.2, 64060.0, 4.87
	quote3Last, quote3Mark, quote3Gap := 64100.0, 64050.0, 7.8
	long := "long"
	events := []*eventstore.StrategyEvent{
		// 一次触发，两个账户：拦截先落地，成交因下单往返晚 4 秒
		{EventHash: itHash("it-ev-1"), Ts: itTime("10:15:03"), InstanceKey: itInstanceA, ConfigVersion: 37,
			UID: "111", AccountLabel: "acctA", Variant: ptr("champion/default"), Event: "gate_block",
			Instrument: itInstrument, Side: &long, Size: ptr(12), OrderSize: ptr(1),
			SigLast: &quote1Last, SigMark: &quote1Mark, GapBp: &quote1Gap,
			GateKind: ptr("reverse_gate_profit"), GateThreshold: ptr(8.0), GateActual: ptr(-108.7),
			Reason: ptr("盈利不足 ROI=-108.7% < 8%"), Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-ev-2"), Ts: itTime("10:15:07"), InstanceKey: itInstanceA, ConfigVersion: 37,
			UID: "222", AccountLabel: "acctB", Variant: ptr("challenger/S400_cap8"), Event: "open",
			Instrument: itInstrument, Side: &long, Size: ptr(4), OrderSize: ptr(1),
			SigLast: &quote1Last, SigMark: &quote1Mark, GapBp: &quote1Gap,
			Source: eventstore.SourceLive, IngestedAt: ingested},
		// 同一分钟内的第二次触发（实测单分钟最多 5 次）
		{EventHash: itHash("it-ev-3"), Ts: itTime("10:15:41"), InstanceKey: itInstanceA, ConfigVersion: 37,
			UID: "111", AccountLabel: "acctA", Variant: ptr("champion/default"), Event: "open",
			Instrument: itInstrument, Side: &long, Size: ptr(13), OrderSize: ptr(1),
			SigLast: &quote2Last, SigMark: &quote2Mark, GapBp: &quote2Gap,
			Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-ev-4"), Ts: itTime("10:16:00"), InstanceKey: itInstanceA, ConfigVersion: 37,
			UID: "111", AccountLabel: "acctA", Event: "cap_skip", Instrument: itInstrument, Side: &long,
			Size: ptr(15), OrderSize: ptr(1), SigLast: &quote3Last, SigMark: &quote3Mark, GapBp: &quote3Gap,
			GateKind: ptr("cap"), GateThreshold: ptr(15.0), GateActual: ptr(16.0),
			Reason: ptr("当前15+1>上限15"), Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-ev-5"), Ts: itTime("10:20:00"), InstanceKey: itInstanceA, ConfigVersion: 37,
			UID: "111", AccountLabel: "acctA", Event: "trailing_close", Instrument: itInstrument,
			Size: ptr(13), Pnl: ptr(42.5), RoiPct: ptr(128.7), PeakPct: ptr(186.4),
			Source: eventstore.SourceLive, IngestedAt: ingested},
		// 另一个实例、同名账户、同一秒、同一份报价——绝不能串进实例A 的结果
		{EventHash: itHash("it-ev-6"), Ts: itTime("10:15:03"), InstanceKey: itInstanceB, ConfigVersion: 12,
			UID: "999", AccountLabel: "acctA", Variant: ptr("champion/S400_cap246_gate8"), Event: "open",
			Instrument: itInstrument, Side: &long, Size: ptr(120), OrderSize: ptr(10),
			SigLast: &quote1Last, SigMark: &quote1Mark, GapBp: &quote1Gap,
			Source: eventstore.SourceBackfill, IngestedAt: ingested},
	}
	if err := conn.Create(&events).Error; err != nil {
		t.Fatalf("写入事件样本失败: %v", err)
	}

	balances := []*eventstore.BalanceSample{
		{EventHash: itHash("it-bal-1"), Ts: itTime("10:00:00"), InstanceKey: itInstanceA, UID: "111",
			AccountLabel: "acctA", Variant: ptr("champion/default"), Balance: 1000, Equity: ptr(1000.0),
			Upl: ptr(0.0), EquityKnown: 1, NetSize: ptr(0), NetSizeKnown: 1,
			Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-bal-2"), Ts: itTime("10:10:00"), InstanceKey: itInstanceA, UID: "111",
			AccountLabel: "acctA", Variant: ptr("champion/default"), Balance: 1000, Equity: ptr(900.0),
			Upl: ptr(-100.0), EquityKnown: 1, NetSize: ptr(12), NetSizeKnown: 1,
			Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-bal-3"), Ts: itTime("10:10:00"), InstanceKey: itInstanceA, UID: "222",
			AccountLabel: "acctB", Balance: 500, Equity: ptr(500.0), EquityKnown: 1,
			NetSize: ptr(4), NetSizeKnown: 1, Source: eventstore.SourceLive, IngestedAt: ingested},
		{EventHash: itHash("it-bal-4"), Ts: itTime("10:10:00"), InstanceKey: itInstanceB, UID: "999",
			AccountLabel: "acctA", Balance: 9000, Equity: ptr(8800.0), EquityKnown: 1,
			NetSize: ptr(120), NetSizeKnown: 1, Source: eventstore.SourceLive, IngestedAt: ingested},
	}
	if err := conn.Create(&balances).Error; err != nil {
		t.Fatalf("写入心跳样本失败: %v", err)
	}

	devs := []*eventstore.DevSample{
		{EventHash: itHash("it-dev-1"), Ts: itTime("10:16:00"), InstanceKey: itInstanceA,
			Instrument: itInstrument, DevTicks: 320, DevMaxBp: ptr(9.4), DevMeanBp: ptr(2.1),
			Source: eventstore.SourceLive, IngestedAt: ingested},
	}
	if err := conn.Create(&devs).Error; err != nil {
		t.Fatalf("写入偏离采样样本失败: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// 时间不偏移：写连接 loc=Local、读连接不带 loc，读回来的墙钟仍与写入时逐字相同。
func TestIntegrationReadsWallClockWithoutTimezoneDrift(t *testing.T) {
	svc, _ := integrationService(t)
	page, err := svc.ListSignals(argusDTO.SignalQueryDTO{
		InstanceKey: itInstanceA, Category: CategoryAll, Order: "ts_asc", PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListSignals: %v", err)
	}
	if len(page.Data) != 5 {
		t.Fatalf("实例A 应有 5 条事件, got %d", len(page.Data))
	}
	if page.Data[0].Ts != "2026-08-18 10:15:03" {
		t.Errorf("首条 ts = %q，期望 2026-08-18 10:15:03（两条连接 loc 不同也不得偏移）", page.Data[0].Ts)
	}
	if page.Data[len(page.Data)-1].Ts != "2026-08-18 10:20:00" {
		t.Errorf("末条 ts = %q", page.Data[len(page.Data)-1].Ts)
	}
}

// 按实例筛选不得串数据：实例B 的同名账户 acctA 不能出现在实例A 的结果里。
func TestIntegrationInstanceScopeNeverLeaks(t *testing.T) {
	svc, _ := integrationService(t)
	page, err := svc.ListSignals(argusDTO.SignalQueryDTO{
		InstanceKey: itInstanceA, AccountLabel: "acctA", Category: CategoryAll, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListSignals: %v", err)
	}
	for _, row := range page.Data {
		if row.InstanceKey != itInstanceA {
			t.Fatalf("实例A 的结果里混入了 %s: %+v", row.InstanceKey, row)
		}
		if row.Uid == "999" {
			t.Fatalf("实例B 的 acctA（uid 999）串进了实例A")
		}
	}
	if page.Total != 4 {
		t.Errorf("实例A 的 acctA 应有 4 条事件, got %d", page.Total)
	}
}

// 同一分钟内的多次触发能分别定位到秒。
func TestIntegrationSameMinuteTriggersStaySeparate(t *testing.T) {
	svc, _ := integrationService(t)
	page, err := svc.ListSignals(argusDTO.SignalQueryDTO{
		InstanceKey: itInstanceA, Start: "2026-08-18 10:15:00", End: "2026-08-18 10:15:59",
		Order: "ts_asc", PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListSignals: %v", err)
	}
	if len(page.Data) != 3 {
		t.Fatalf("10:15 这一分钟应有 3 条触发事件, got %d", len(page.Data))
	}
	seen := map[string]bool{}
	for _, row := range page.Data {
		seen[row.Ts] = true
	}
	if !seen["2026-08-18 10:15:03"] || !seen["2026-08-18 10:15:07"] || !seen["2026-08-18 10:15:41"] {
		t.Errorf("同分钟内的秒级时刻丢失: %v", seen)
	}

	// 详情：10:15:03 的那次触发含两个账户；10:15:41 是独立的另一次
	first := page.Data[0]
	detail, err := svc.GetSignalDetail(first.EventID)
	if err != nil {
		t.Fatalf("GetSignalDetail: %v", err)
	}
	if detail.AccountCount != 2 || detail.OpenedCount != 1 || detail.BlockedCount != 1 {
		t.Fatalf("首次触发的逐账户判定有误: %+v", detail)
	}
	for _, acc := range detail.Accounts {
		if acc.AccountLabel != "acctA" && acc.AccountLabel != "acctB" {
			t.Errorf("详情里混入了别的账户: %+v", acc)
		}
	}
	third, err := svc.GetSignalDetail(page.Data[2].EventID)
	if err != nil {
		t.Fatalf("GetSignalDetail(第二次触发): %v", err)
	}
	if third.AccountCount != 1 || third.Ts != "2026-08-18 10:15:41" {
		t.Errorf("同分钟第二次触发未独立: %+v", third)
	}
}

func TestIntegrationSliceAndTimelineAndEquity(t *testing.T) {
	svc, _ := integrationService(t)

	page, err := svc.ListSignals(argusDTO.SignalQueryDTO{
		InstanceKey: itInstanceA, Start: "2026-08-18 10:15:03", End: "2026-08-18 10:15:03", PageSize: 5,
	})
	if err != nil || len(page.Data) == 0 {
		t.Fatalf("取锚点事件失败: %v / %d", err, len(page.Data))
	}
	slice, err := svc.GetSignalSlice(page.Data[0].EventID, argusDTO.SliceQueryDTO{WindowSeconds: 60})
	if err != nil {
		t.Fatalf("GetSignalSlice: %v", err)
	}
	if slice.TickSource != TickSourceStrategyEvent || slice.TickComplete {
		t.Errorf("没有逐秒切片（超保留期 / 采集器未上线）时必须自曝降级: %+v", slice)
	}
	if slice.SliceAnchorTs != "" || slice.DcPoints != 0 {
		t.Errorf("降级路径不该给出切片锚点与覆盖率: %+v", slice)
	}
	// ±60 秒内有 10:15:03 / 10:15:07 / 10:15:41 / 10:16:00 四个秒级点位
	if len(slice.Points) != 4 {
		t.Errorf("切片点位数有误: %+v", slice.Points)
	}
	if len(slice.DevSamples) != 1 {
		t.Errorf("窗口内应带出 1 条无条件偏离采样: %+v", slice.DevSamples)
	}

	timeline, err := svc.GetTimeline(argusDTO.TimelineQueryDTO{
		InstanceKey: itInstanceA, Instrument: itInstrument, Interval: "1m",
		Start: "2026-08-18 10:15:00", End: "2026-08-18 10:20:00",
	})
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	if len(timeline.Buckets) != 6 {
		t.Fatalf("10:15–10:20 的 1m 桶应有 6 个, got %d", len(timeline.Buckets))
	}
	if timeline.Buckets[0].Total != 3 || timeline.Buckets[0].Open != 2 || timeline.Buckets[0].GateBlock != 1 {
		t.Errorf("首桶聚合有误: %+v", timeline.Buckets[0])
	}
	if timeline.Buckets[5].Exit != 1 {
		t.Errorf("10:20 桶应有一次出场: %+v", timeline.Buckets[5])
	}
	if timeline.EventTotal != 5 {
		t.Errorf("时间轴只应覆盖实例A 的 5 条事件, got %d", timeline.EventTotal)
	}

	// 缺 instanceKey 的净仓/权益类接口必须直接拒绝，而不是跨实例合并
	if _, err := svc.GetTimeline(argusDTO.TimelineQueryDTO{Instrument: itInstrument}); err == nil {
		t.Error("timeline 缺 instanceKey 应报错")
	}
	if _, err := svc.GetEquityCurve(argusDTO.EquityQueryDTO{}); err == nil {
		t.Error("equity-curve 缺 instanceKey 应报错")
	}

	equity, err := svc.GetEquityCurve(argusDTO.EquityQueryDTO{
		InstanceKey: itInstanceA, Start: "2026-08-18 09:00:00", End: "2026-08-18 11:00:00", BucketSeconds: 600,
	})
	if err != nil {
		t.Fatalf("GetEquityCurve: %v", err)
	}
	if len(equity.Series) != 2 {
		t.Fatalf("实例A 应有 2 个账户序列, got %d", len(equity.Series))
	}
	acctA := equity.Series[0]
	if acctA.AccountLabel != "acctA" || len(acctA.Points) != 2 {
		t.Fatalf("acctA 序列有误: %+v", acctA)
	}
	if acctA.Points[0].Time != "2026-08-18 10:00:00" {
		t.Errorf("降采样桶键应落在墙钟网格上: %q", acctA.Points[0].Time)
	}
	if acctA.ChangePct == nil || *acctA.ChangePct != -10 {
		t.Errorf("acctA 权益变动应为 -10%%: %v", acctA.ChangePct)
	}
}

func TestIntegrationGateStatsAndInstanceSummaryAndOptions(t *testing.T) {
	svc, _ := integrationService(t)

	stats, err := svc.GetGateStats(argusDTO.GateStatsQueryDTO{
		InstanceKey: itInstanceA, Start: "2026-08-18 00:00:00", End: "2026-08-18 23:59:59",
	})
	if err != nil {
		t.Fatalf("GetGateStats: %v", err)
	}
	if stats.TotalTriggers != 4 || stats.Opened != 2 || stats.Blocked != 2 {
		t.Fatalf("实例A 触发统计有误: %+v", stats)
	}
	if stats.CrossInstance {
		t.Error("单实例查询不应标 crossInstance")
	}
	kinds := map[string]int64{}
	for _, g := range stats.ByGate {
		kinds[g.GateKind] = g.Count
	}
	if kinds["reverse_gate_profit"] != 1 || kinds["cap"] != 1 {
		t.Errorf("门控聚合有误: %+v", stats.ByGate)
	}

	summary, err := svc.GetInstanceSummary(argusDTO.InstanceSummaryQueryDTO{
		Start: "2026-08-18 00:00:00", End: "2026-08-18 23:59:59",
	})
	if err != nil {
		t.Fatalf("GetInstanceSummary: %v", err)
	}
	byKey := map[string]argusDTO.InstanceSummaryDTO{}
	for _, item := range summary.Instances {
		byKey[item.InstanceKey] = item
	}
	a, okA := byKey[itInstanceA]
	b, okB := byKey[itInstanceB]
	if !okA || !okB {
		t.Fatalf("两个测试实例都应出现: %+v", summary.Instances)
	}
	if !a.Registered || a.InstanceName != "集成测试实例A" {
		t.Errorf("已注册实例应带上展示名: %+v", a)
	}
	if b.Registered {
		t.Errorf("未注册实例必须标 registered=false 让人看见: %+v", b)
	}
	if a.Signals != 4 || a.Opened != 2 || a.Exits != 1 {
		t.Errorf("实例A 汇总有误: %+v", a)
	}
	if b.Signals != 1 || b.Exits != 0 {
		t.Errorf("实例B 汇总有误: %+v", b)
	}
	// 同名账户 acctA 在两个实例下的权益必须各归各的
	if a.EquityTotal == nil || *a.EquityTotal != 1400 {
		t.Errorf("实例A 权益合计应为 900+500=1400: %v", a.EquityTotal)
	}
	if b.EquityTotal == nil || *b.EquityTotal != 8800 {
		t.Errorf("实例B 权益合计应为 8800: %v", b.EquityTotal)
	}
	if summary.Notice == "" {
		t.Error("跨实例汇总必须带不可比提示")
	}

	options, err := svc.GetFilterOptions("", "")
	if err != nil {
		t.Fatalf("GetFilterOptions: %v", err)
	}
	seen := map[string]bool{}
	for _, item := range options.Instances {
		seen[item.InstanceKey] = true
	}
	if !seen[itInstanceA] || !seen[itInstanceB] {
		t.Errorf("筛选项应枚举出两个测试实例: %+v", options.Instances)
	}
	accounts := 0
	for _, acc := range options.Accounts {
		if acc.InstanceKey == itInstanceA || acc.InstanceKey == itInstanceB {
			accounts++
			if acc.InstanceKey == "" {
				t.Error("账户筛选项必须带 instanceKey，账户名会跨实例重号")
			}
		}
	}
	if accounts != 3 {
		t.Errorf("应枚举出 3 个 (实例, 账户) 组合, got %d", accounts)
	}
	if len(options.EventKinds) != 10 || len(options.Strengths) != 3 {
		t.Errorf("枚举字典有误: %d 类事件 / %d 档强度", len(options.EventKinds), len(options.Strengths))
	}
}

// 未指定窗口时，默认窗口锚在已入库数据的最新时刻，而不是服务器当前时间。
func TestIntegrationDefaultWindowFollowsData(t *testing.T) {
	svc, _ := integrationService(t)
	stats, err := svc.GetGateStats(argusDTO.GateStatsQueryDTO{InstanceKey: itInstanceA})
	if err != nil {
		t.Fatalf("GetGateStats: %v", err)
	}
	if stats.Window.Resolved != "latest-data" {
		t.Fatalf("默认窗口来源标记有误: %+v", stats.Window)
	}
	// 拦截原因聚合只看四类触发事件，所以右界是最后一次触发（10:16:00），
	// 而不是 10:20:00 那条平仓事件——窗口跟随的是本查询自己的事件集合。
	if stats.Window.End != "2026-08-18 10:16:00" {
		t.Errorf("默认窗口右界应是最新一次触发时刻: %q", stats.Window.End)
	}
	if stats.TotalTriggers != 4 {
		t.Errorf("默认窗口应覆盖到全部样本: %d", stats.TotalTriggers)
	}
}

// 有逐秒切片时走 signal_slice：121 点逐秒完整、含币安 last、以切片锚点为原点。
//
// 样本刻意让锚点比事件早 5 秒（trade.signal.delay_seconds 的默认值），
// 这是生产的真实关系——按事件 ts 精确等值去取切片永远取不到。
func TestIntegrationSliceUsesSignalSliceWhenPresent(t *testing.T) {
	svc, conn := integrationService(t)

	page, err := svc.ListSignals(argusDTO.SignalQueryDTO{
		InstanceKey: itInstanceA, Start: "2026-08-18 10:15:41", End: "2026-08-18 10:15:41", PageSize: 5,
	})
	if err != nil || len(page.Data) == 0 {
		t.Fatalf("取锚点事件失败: %v / %d", err, len(page.Data))
	}
	anchor := itTime("10:15:36") // 事件 10:15:41 − 5 秒信号延迟
	row := &eventstore.SignalSlice{
		Ts: anchor, InstanceKey: itInstanceA, ConfigVersion: 37, Instrument: itInstrument,
		InstIdRaw: ptr(itInstrument),
		StartAt:   anchor.Add(-60 * time.Second), EndAt: anchor.Add(60 * time.Second),
		HalfWindowSec: 60, SeriesPoints: 121, DcPoints: 121, BinPoints: 121,
		DcLastJson:  itSeriesJSON(64000),
		DcMarkJson:  itSeriesJSON(64000),
		BinLastJson: itSeriesJSON(64010),
		Source:      eventstore.SourceLive, IngestedAt: time.Now(),
	}
	if err := conn.Create(row).Error; err != nil {
		t.Fatalf("写入切片样本失败: %v", err)
	}

	slice, err := svc.GetSignalSlice(page.Data[0].EventID, argusDTO.SliceQueryDTO{WindowSeconds: 60})
	if err != nil {
		t.Fatalf("GetSignalSlice: %v", err)
	}
	if slice.TickSource != TickSourceSignalSlice || !slice.TickComplete {
		t.Fatalf("应走 signal_slice 且逐秒完整: %+v", slice)
	}
	if slice.SliceAnchorTs != "2026-08-18 10:15:36" || slice.AnchorLagSec != 5 {
		t.Errorf("锚点/滞后不对: anchorTs=%q lag=%d", slice.SliceAnchorTs, slice.AnchorLagSec)
	}
	if slice.Window.Resolved != "signal-slice" {
		t.Errorf("窗口原点应标成 signal-slice: %+v", slice.Window)
	}
	if len(slice.Points) != 121 {
		t.Fatalf("应有 121 点, got %d", len(slice.Points))
	}
	if slice.DcPoints != 121 || slice.BinPoints != 121 {
		t.Errorf("覆盖率应满: %d/%d", slice.DcPoints, slice.BinPoints)
	}
	if slice.Points[0].BinLast == nil {
		t.Error("币安 last 必须带出来——降级来源里根本没有这一路")
	}
	// 触发行落在 offset=+5，不是 offset==0。
	trigger := -1
	for i, p := range slice.Points {
		if p.IsTriggerTs {
			trigger = i
		}
	}
	if trigger != 65 || slice.Points[trigger].Ts != "2026-08-18 10:15:41" {
		t.Errorf("触发行应是事件那一秒: idx=%d", trigger)
	}
	// 请求超过切片半窗时收窄并说明，不能假装有 ±300 秒逐秒数据。
	wide, err := svc.GetSignalSlice(page.Data[0].EventID, argusDTO.SliceQueryDTO{WindowSeconds: 300})
	if err != nil {
		t.Fatalf("GetSignalSlice(300): %v", err)
	}
	if wide.WindowSeconds != 60 || wide.DegradedReason == "" {
		t.Errorf("超宽窗口应收窄到 ±60 并给出说明: windowSeconds=%d reason=%q", wide.WindowSeconds, wide.DegradedReason)
	}
	if len(wide.Points) != 121 {
		t.Errorf("收窄后仍应是 121 点, got %d", len(wide.Points))
	}
}

// itSeriesJSON 121 点递增序列，逐秒无空洞（覆盖率与 dc_points/bin_points 自洽）。
// null 元素的处理由单测覆盖（slice_test.go）。
func itSeriesJSON(base float64) string {
	parts := make([]string, 121)
	for i := range parts {
		parts[i] = strconv.FormatFloat(base+float64(i), 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
