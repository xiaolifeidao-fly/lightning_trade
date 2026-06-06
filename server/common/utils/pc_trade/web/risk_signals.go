package web

import (
	"hash/fnv"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RiskBehaviorSnapshot 风控行为采集摘要（前端键鼠行为 + 采样窗口）。
//
// 前端规范见 docs/前端行为识别数据采集方案.md §3-§4。
// 这里在服务端构造时，所有几何字段必须互相自洽（点数少 -> 直度高，点数多 -> 弯曲度高），
// 否则风控侧会把"两个采样点直度=1"这种特征当作机器人。
type RiskBehaviorSnapshot struct {
	// 行为窗口内点击次数
	ClickCount int
	// 点击间隔标准差（毫秒）。点击数 < 3 时为 0
	ClickIntervalStdMs float64
	// 页面进入到首次点击的耗时（毫秒）。无点击时为 0
	FirstClickLatencyMs float64
	// 键盘输入间隔标准差（毫秒）。下单场景常常没有键盘输入，可为 0
	KeystrokeIntervalStdMs float64
	// 鼠标移动速度标准差。采样点 < 5 时为 0
	MousePathSpeedStd float64
	// 行为窗口内鼠标采样点数量
	MousePathPointCount int
	// 鼠标方向变化次数（夹角 > 30°）
	MousePathDirectionChangeCount int
	// 鼠标路径笔直度，0~1。值越接近 1 越像直线
	MousePathStraightnessScore float64
	// 本次行为统计窗口长度（毫秒）。普通场景 5 分钟，交易场景为 min(5min, 上次下单到本次下单)
	RiskBehaviorWindowMs float64
	// 鼠标采样间隔（毫秒），默认 200
	RiskBehaviorSampleIntervalMs int
}

// BrowserSignals 浏览器自动化指纹（每账户固定，启动后不变）。
//
// 前端规范见 docs/前端行为识别数据采集方案.md §4.7。
// 真人浏览器中所有字段都应该是 "正常" 值；这里我们伪装成正常浏览器。
type BrowserSignals struct {
	// navigator.webdriver === true 才是机器人，正常应为 false
	Webdriver bool
	// navigator.plugins.length，常见 3~6
	PluginsLength int
	// navigator.languages.length，常见 1~3
	LanguagesLength int
	// UA 中是否包含 Headless，正常为 false
	UAHeadless bool
	// permissions.query 行为是否异常，正常为 false
	PermissionsQueryAbnormal bool
	// iframe 基础访问检测是否异常，正常为 false
	IframeAccessAbnormal bool
}

// ViewportSignals 视口/屏幕指纹（每账户固定，启动后不变）。
type ViewportSignals struct {
	ScreenHeight   int
	ScreenWidth    int
	ViewportHeight int
	ViewportWidth  int
	// 时区偏移（分钟），中国大陆 -480，新加坡 -480，欧洲 -60/0
	TimezoneOffset int
}

// 常见的真实分辨率组合。挑选时按 LoginID 哈希取模，保证账户间分布、单账户稳定。
var commonScreens = []struct {
	ScreenW, ScreenH int
}{
	{1920, 1080}, // 最常见的 1080p
	{1728, 1117}, // MacBook Pro 14"
	{1512, 982},  // MacBook Pro 13"
	{1440, 900},  // MacBook Air
	{2560, 1440}, // 2K
	{1366, 768},  // 老款笔记本
	{1600, 900},  // 16:9 笔记本
}

// stableSeed 把任意字符串映射成稳定的 int64 种子，用于派生该账户的固定指纹。
func stableSeed(s string) int64 {
	if s == "" {
		return 1
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// GenerateBrowserSignals 基于 LoginID 派生稳定的浏览器指纹。
// 同一 LoginID 多次调用结果完全一致，保证设备指纹不抖动。
//
// 取值范围参考 prod 抓包 + 真实 Chrome 默认值：
//   - plugins: 真实 Chrome 默认 5；3~6 都常见
//   - languages: 80% 用户只有 1 种语言；偶尔 2 种
func GenerateBrowserSignals(loginID string) BrowserSignals {
	r := rand.New(rand.NewSource(stableSeed("browser:" + loginID)))
	plugins := 3 + r.Intn(4) // 3~6
	langs := 1
	if r.Intn(5) == 0 { // 20% 概率为 2
		langs = 2
	}
	return BrowserSignals{
		Webdriver:                false,
		PluginsLength:            plugins,
		LanguagesLength:          langs,
		UAHeadless:               false,
		PermissionsQueryAbnormal: false,
		IframeAccessAbnormal:     false,
	}
}

// GenerateViewportSignals 基于 LoginID 派生稳定的屏幕/视口指纹。
func GenerateViewportSignals(loginID string) ViewportSignals {
	r := rand.New(rand.NewSource(stableSeed("viewport:" + loginID)))
	scr := commonScreens[r.Intn(len(commonScreens))]

	// 视口比屏幕小：通常浏览器窗口非全屏，宽度 50%~95%，高度 70%~95%
	widthRatio := 0.5 + r.Float64()*0.45
	heightRatio := 0.7 + r.Float64()*0.25
	vpW := int(float64(scr.ScreenW) * widthRatio)
	vpH := int(float64(scr.ScreenH) * heightRatio)

	// 时区从常见东八区/欧美中选；这里简单固定 -480（GMT+8），生产可按账户配置覆盖
	tz := -480

	return ViewportSignals{
		ScreenHeight:   scr.ScreenH,
		ScreenWidth:    scr.ScreenW,
		ViewportHeight: vpH,
		ViewportWidth:  vpW,
		TimezoneOffset: tz,
	}
}

// behaviorTracker 跟踪每个 LoginID 的"页面进入时间"和"上次下单时间"。
//
// 对照 prod 真实算法（chunk 6349.js）：
//
//	windowStart = max(now - 5min, pageEnterTime)
//	if scene == "trade" && lastTradeAt > 0: windowStart = max(windowStart, lastTradeAt)
//	risk_behavior_window_ms = now - windowStart
//
// 服务端模拟：
//   - pageEnterTime：每个 LoginID 首次调用时记录，作为"打开页面"的时刻
//   - lastTradeAt：每次下单后更新
//
// first_click_latency_ms 在 prod 是 firstClickTime - pageEnterTime（页面相对），
// 这里我们也用 pageEnterTime 作为基准。
type behaviorTracker struct {
	mu        sync.Mutex
	pageEnter map[string]time.Time // 页面进入时间
	lastTrade map[string]time.Time // 上次下单时间
}

var globalBehaviorTracker = &behaviorTracker{
	pageEnter: map[string]time.Time{},
	lastTrade: map[string]time.Time{},
}

const (
	maxBehaviorWindowMs       = 5 * 60 * 1000 // 5 分钟（对应 prod 的 3e5）
	defaultSampleIntervalMs   = 200
	mousePathMinPointForCurve = 5
)

// trackerSnapshot 一次调用要返回的所有时间相关量。
type trackerSnapshot struct {
	WindowMs       float64 // risk_behavior_window_ms
	PageElapsedMs  float64 // now - pageEnterTime，用于 first_click_latency 上限
	IsFirstAfterPV bool    // 首次"页面浏览"后的第一笔（无 lastTrade）
}

// consume 模拟 prod 行为：更新 lastTradeAt，返回本次窗口和页面停留时间。
func (t *behaviorTracker) consume(loginID string, now time.Time, r *rand.Rand) trackerSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	pageEnter, hasPageEnter := t.pageEnter[loginID]
	if !hasPageEnter {
		// 首次：模拟"用户刚打开页面 X 秒后下单"。
		// 真人通常打开页面 5s~3min 左右浏览后才下单。
		offsetMs := 5000 + r.Intn(3*60*1000) // 5s~3min
		pageEnter = now.Add(-time.Duration(offsetMs) * time.Millisecond)
		t.pageEnter[loginID] = pageEnter
	}

	lastTrade, hasLastTrade := t.lastTrade[loginID]
	t.lastTrade[loginID] = now

	// 计算 windowStart = max(now - 5min, pageEnterTime, lastTradeAt)
	windowStart := now.Add(-maxBehaviorWindowMs * time.Millisecond)
	if pageEnter.After(windowStart) {
		windowStart = pageEnter
	}
	if hasLastTrade && lastTrade.After(windowStart) {
		windowStart = lastTrade
	}

	windowMs := float64(now.Sub(windowStart).Milliseconds())
	if windowMs < 0 {
		windowMs = 0
	}
	// 加 0~1ms 小数尾巴模拟 performance.now() 的高精度
	windowMs += r.Float64()

	pageElapsedMs := float64(now.Sub(pageEnter).Milliseconds()) + r.Float64()
	if pageElapsedMs < 0 {
		pageElapsedMs = 0
	}

	return trackerSnapshot{
		WindowMs:       windowMs,
		PageElapsedMs:  pageElapsedMs,
		IsFirstAfterPV: !hasLastTrade,
	}
}

// GenerateBehaviorSnapshot 生成一份"看似真人"的行为采集摘要。
//
// 完全按 prod 真实算法（chunk 6349.js: getRiskBehaviorSnapshot）的口径输出：
//   - risk_behavior_window_ms = now - max(pageEnterTime, lastTradeAt, now-5min)
//     首次下单后 prod 会把 lastTradeAt 设为 now，所以连续下单时窗口很小。
//   - first_click_latency_ms = firstClickTime - pageEnterTime（页面相对，不依赖窗口）
//   - 各几何字段按文档 §4.4-§4.6 的边界规则退化（点数<5 时 speed_std/direction_change=0）
//   - 小数位严格对齐 prod：默认 toFixed(2)，speed_std/straightness toFixed(4)
//
// loginID 用于 pageEnter / lastTrade 跟踪；windowMsHint > 0 时调用方显式覆盖窗口。
//
// 鼠标活跃比例参考 prod 抓包样本（窗口/点数对）：
//
//	(15891ms, 9点)  → 9/79 ≈ 11%
//	(1612ms,  2点)  → 2/8  ≈ 25%
//
// 故活跃比取 5%~30%，远低于 30%~70% 那种"全程移动"的假设。
func GenerateBehaviorSnapshot(loginID string, windowMsHint float64) RiskBehaviorSnapshot {
	// 行为快照每次下单都不一样
	r := rand.New(rand.NewSource(time.Now().UnixNano() + stableSeed(loginID)))

	var windowMs, pageElapsedMs float64
	if windowMsHint > 0 {
		windowMs = windowMsHint
		if windowMs > maxBehaviorWindowMs {
			windowMs = maxBehaviorWindowMs
		}
		// hint 模式下 page_elapsed 至少和 window 一样长（页面打开比窗口早）
		pageElapsedMs = windowMs + 1000 + r.Float64()*30000
	} else {
		snap := globalBehaviorTracker.consume(loginID, time.Now(), r)
		windowMs = snap.WindowMs
		if windowMs > maxBehaviorWindowMs {
			windowMs = maxBehaviorWindowMs
		}
		pageElapsedMs = snap.PageElapsedMs
	}

	sampleInterval := defaultSampleIntervalMs

	// ---- 鼠标采样点 ----
	// 理论最大点数 = window / 200ms（节流），真实用户活跃比 5%~30%
	theoreticalMax := int(windowMs / float64(sampleInterval))
	if theoreticalMax > 1500 { // prod ring buffer 上限
		theoreticalMax = 1500
	}
	activeRatio := 0.05 + r.Float64()*0.25 // 5%~30%
	pointCount := int(float64(theoreticalMax) * activeRatio)
	if pointCount < 0 {
		pointCount = 0
	}
	// 极小窗口（< 3s）：1~3 点
	if windowMs < 3000 {
		pointCount = r.Intn(4)
	}

	var speedStd, straightness float64
	var directionChange int

	switch {
	case pointCount == 0:
		// 全部为 0
	case pointCount < mousePathMinPointForCurve:
		// prod: speed_std<5 点→0，direction_change<3 点→0
		// 2 点必然共线 → 1.0；3-4 点稍微弯但仍偏直
		if pointCount == 2 {
			straightness = 1.0
		} else if pointCount >= 3 {
			straightness = roundTo(0.55+r.Float64()*0.4, 4) // 0.55~0.95
		}
	default:
		// 真实曲折轨迹
		speedStd = roundTo(0.25+r.Float64()*1.5, 4) // 0.25 ~ 1.75
		minDir := int(float64(pointCount) * 0.3)
		maxDir := int(float64(pointCount) * 0.85)
		if maxDir <= minDir {
			maxDir = minDir + 1
		}
		directionChange = minDir + r.Intn(maxDir-minDir+1)
		straightness = roundTo(0.03+r.Float64()*0.30, 4) // 0.03 ~ 0.33
	}

	// ---- 点击 ----
	windowSec := windowMs / 1000.0
	// 平均点击数与 log2(windowSec) 成正比，加抖动
	avgClicks := math.Log2(windowSec+1) * (0.8 + r.Float64()*1.0)
	clickCount := int(math.Round(avgClicks))
	if clickCount < 0 {
		clickCount = 0
	}
	if windowMs < 1500 && r.Intn(2) == 0 {
		clickCount = 0
	}

	var clickIntervalStd float64
	if clickCount >= 3 {
		// prod: clicks≥3 时 std(intervals).toFixed(2)。真实分布 80~430ms
		clickIntervalStd = roundTo(80+r.Float64()*350, 2)
	}

	// first_click_latency_ms：页面相对时间。
	// 真实抓包样本：324ms / 7685ms / 14673ms（中位数 ~10s）
	// 范围：500ms ~ min(pageElapsedMs, 60s)
	var firstClickLatency float64
	if clickCount > 0 {
		maxLatency := math.Min(pageElapsedMs, 60000)
		minLatency := 500.0
		if maxLatency <= minLatency {
			maxLatency = minLatency + 100
		}
		firstClickLatency = roundTo(minLatency+r.Float64()*(maxLatency-minLatency), 2)
	}

	// ---- 键盘 ----
	// 下单页面下单时基本不打字，95% 概率为 0
	var keyStd float64
	if r.Intn(20) == 0 {
		keyStd = roundTo(120+r.Float64()*250, 2)
	}

	return RiskBehaviorSnapshot{
		ClickCount:                    clickCount,
		ClickIntervalStdMs:            clickIntervalStd,
		FirstClickLatencyMs:           firstClickLatency,
		KeystrokeIntervalStdMs:        keyStd,
		MousePathSpeedStd:             speedStd,
		MousePathPointCount:           pointCount,
		MousePathDirectionChangeCount: directionChange,
		MousePathStraightnessScore:    straightness,
		RiskBehaviorWindowMs:          roundTo(windowMs, 2), // prod 默认 toFixed(2)
		RiskBehaviorSampleIntervalMs:  sampleInterval,
	}
}

func roundTo(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}
