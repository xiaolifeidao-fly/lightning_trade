package trade

import (
	"errors"
	"strings"
	"testing"

	"common/utils"
)

// fakeCloser 可控平仓通道，记录调用次数与入参（验证"主成功时备用零调用"）。
type fakeCloser struct {
	name   string
	err    error
	calls  int
	gotIID string
	gotPID string
}

func (f *fakeCloser) Close(a ClosePosArgs) error {
	f.calls++
	f.gotIID, f.gotPID = a.InstId, a.PosId
	return f.err
}

func (f *fakeCloser) Channel() string { return f.name }

// 原生通道参数约定（设计 §3）：productGroup 固定 SwapU、instId 原样透传
// （原生 API 的 GetPositions 返回的就是 BTC-USDT-SWAP，不做 instrumentBase 归一
// ——那是 bot-close 注册表的问题，两者不可混用）、posIds 单元素数组。
// 这条通道零实盘调用历史，参数错会在首次故障时才暴露，故必须钉死。
func TestNativeCloserParams(t *testing.T) {
	var gotPG, gotIID string
	var gotIDs []string
	n := &nativeCloser{closeByIds: func(pg, instId string, posIds []string) (map[string]interface{}, error) {
		gotPG, gotIID, gotIDs = pg, instId, posIds
		return nil, nil
	}}
	if err := n.Close(ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "1001124331810473"}); err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if gotPG != "SwapU" {
		t.Fatalf("productGroup 必须为 SwapU, got %q", gotPG)
	}
	if gotIID != "BTC-USDT-SWAP" {
		t.Fatalf("instId 须原样透传, got %q", gotIID)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "1001124331810473" {
		t.Fatalf("posIds 应为单元素数组, got %v", gotIDs)
	}
	if n.Channel() != "native" {
		t.Fatalf("通道名: %q", n.Channel())
	}
}

// Go nil-interface 陷阱：返回 (*nativeCloser)(nil) 装箱后 != nil，
// CloseWithFallback 的 nil 判断会失效并 panic。必须返回真 nil。
func TestManagerClosersReturnTrueNilWhenUnconfigured(t *testing.T) {
	tm := &TradeManager{
		clients:    map[string]*utils.DeepCoinClient{},
		webClients: map[string]*DirectWebClient{},
	}
	if c := tm.NativeCloser("不存在的账户"); c != nil {
		t.Fatalf("无 API 客户端须返回真 nil, got %#v", c)
	}
	if c := tm.WebCloser("不存在的账户"); c != nil {
		t.Fatalf("无 Web 客户端须返回真 nil, got %#v", c)
	}
	// 有客户端时须返回可用实例
	tm.clients["A"] = utils.NewDeepCoinClient("k", "s", "p")
	if c := tm.NativeCloser("A"); c == nil || c.Channel() != "native" {
		t.Fatalf("有 API 客户端应返回 native closer, got %#v", c)
	}
}

func TestCloseWithFallback(t *testing.T) {
	gwErr := errors.New("GW: Login Timeout")
	apiErr := errors.New("HTTP 401 body={\"code\":\"50111\",\"msg\":\"Invalid Sign\"}")

	t.Run("主通道成功: 备用零调用", func(t *testing.T) {
		web := &fakeCloser{name: "web"}
		native := &fakeCloser{name: "native"}
		out := CloseWithFallback(web, native, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "1001124331810473"})
		if !out.OK() || out.Channel != "web" {
			t.Fatalf("应走主通道: %+v", out)
		}
		if out.Degraded {
			t.Fatalf("主通道成功不算降级: %+v", out)
		}
		if native.calls != 0 {
			t.Fatalf("主通道成功时备用通道必须零调用, got %d 次（不得盲目双发平仓）", native.calls)
		}
		if web.gotIID != "BTC-USDT-SWAP" || web.gotPID != "1001124331810473" {
			t.Fatalf("入参未透传: instId=%q posId=%q", web.gotIID, web.gotPID)
		}
	})

	t.Run("主失败备成功: 降级且保留主错误", func(t *testing.T) {
		web := &fakeCloser{name: "web", err: gwErr}
		native := &fakeCloser{name: "native"}
		out := CloseWithFallback(web, native, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "pos1"})
		if !out.OK() || out.Channel != "native" {
			t.Fatalf("应降级到备用通道: %+v", out)
		}
		if !out.Degraded {
			t.Fatalf("主通道配置了却失败=降级: %+v", out)
		}
		if !errors.Is(out.PrimaryErr, gwErr) {
			t.Fatalf("主通道错误须保留(诊断用): %+v", out)
		}
		if native.calls != 1 || native.gotPID != "pos1" {
			t.Fatalf("备用通道调用异常: calls=%d posId=%q", native.calls, native.gotPID)
		}
	})

	t.Run("双通道失败: 聚合两条错误", func(t *testing.T) {
		web := &fakeCloser{name: "web", err: gwErr}
		native := &fakeCloser{name: "native", err: apiErr}
		out := CloseWithFallback(web, native, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "pos1"})
		if out.OK() || out.Channel != "" {
			t.Fatalf("双失败不得报成功: %+v", out)
		}
		err := out.Err()
		if err == nil {
			t.Fatalf("双失败须返回错误")
		}
		// 首次真实故障要靠这条信息判定原生通道是哪类问题，两条原因缺一不可
		for _, want := range []string{"web", "native", "Login Timeout", "Invalid Sign"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("错误须含 %q, got: %v", want, err)
			}
		}
	})

	t.Run("无备用通道: 退化为现状", func(t *testing.T) {
		web := &fakeCloser{name: "web", err: gwErr}
		out := CloseWithFallback(web, nil, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "pos1"})
		if out.OK() {
			t.Fatalf("主失败且无备用应失败: %+v", out)
		}
		if out.Degraded {
			t.Fatalf("无备用通道谈不上降级: %+v", out)
		}
		if !strings.Contains(out.Err().Error(), "Login Timeout") {
			t.Fatalf("须保留主通道原因: %v", out.Err())
		}
	})

	t.Run("主通道未配置: 正常走备用, 不算降级", func(t *testing.T) {
		native := &fakeCloser{name: "native"}
		out := CloseWithFallback(nil, native, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "pos1"})
		if !out.OK() || out.Channel != "native" {
			t.Fatalf("应直接用备用通道: %+v", out)
		}
		// 纯 API 账户（未配 cookie）每轮都走备用是正常路径，标降级会永久刷告警
		if out.Degraded {
			t.Fatalf("主通道未配置属正常路径, 不得标降级: %+v", out)
		}
	})

	t.Run("两条通道都没有: 配置故障", func(t *testing.T) {
		out := CloseWithFallback(nil, nil, ClosePosArgs{InstId: "BTC-USDT-SWAP", PosId: "pos1"})
		if out.OK() {
			t.Fatalf("无通道不得报成功: %+v", out)
		}
		if !errors.Is(out.Err(), ErrNoCloseChannel) {
			t.Fatalf("应为可识别的配置故障错误(P3/R4 立即告警), got: %v", out.Err())
		}
	})
}

