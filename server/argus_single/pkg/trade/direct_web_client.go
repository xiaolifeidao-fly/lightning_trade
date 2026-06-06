package trade

import (
	"context"
	"encoding/json"
	"fmt"

	"common/utils"
	"common/utils/pc_trade/user"
	pcweb "common/utils/pc_trade/web"

	"github.com/sirupsen/logrus"
)

// DirectWebClient 直接调用 DeepCoin Web 接口，不经过 auto_trade 中转。
// userProvider 允许当前继续使用配置里的静态 cookie，也为后续账号密码登录获取 cookie 预留入口。
type DirectWebClient struct {
	userProvider UserProvider
}

func NewDirectWebClient(userProvider UserProvider) *DirectWebClient {
	return &DirectWebClient{
		userProvider: userProvider,
	}
}

func (c *DirectWebClient) resolveUser(ctx context.Context) (*user.User, error) {
	if c.userProvider == nil {
		return nil, fmt.Errorf("Web 用户提供器未初始化")
	}
	return c.userProvider.GetUser(ctx)
}

// convertOrderResponse 将 pc_trade/web.OrderResponse 转换为 utils.WebOrderResponse。
func convertOrderResponse(src *pcweb.OrderResponse) (*utils.WebOrderResponse, error) {
	b, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("序列化 OrderResponse 失败: %w", err)
	}
	var dst utils.WebOrderResponse
	if err := json.Unmarshal(b, &dst); err != nil {
		return nil, fmt.Errorf("反序列化 WebOrderResponse 失败: %w", err)
	}
	return &dst, nil
}

// ClosePosition 市价全平指定持仓。
func (c *DirectWebClient) ClosePosition(positionID string) (*pcweb.ClosePosResponse, error) {
	u, err := c.resolveUser(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取 Web 用户凭证失败: %w", err)
	}
	return pcweb.SendClosePos(u, &pcweb.ClosePosRequest{
		PositionID: positionID,
	})
}

// MarketBuyLongWithRisk 直接市价做多开仓并异步发送风控。
func (c *DirectWebClient) MarketBuyLongWithRisk(
	instrumentID string,
	volume, lever, isCrossMargin int,
	loginID string,
	tradePrice float64,
) (*utils.WebOrderResponse, error) {
	u, err := c.resolveUser(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取 Web 用户凭证失败: %w", err)
	}

	orderResp, err := pcweb.SendOrderInsert(u, &pcweb.OrderRequest{
		InstrumentID:   instrumentID,
		Volume:         volume,
		Direction:      "0",
		OrderPriceType: "4",
		Price:          "",
		OffsetFlag:     "0",
		IsCrossMargin:  isCrossMargin,
		Lever:          lever,
	})
	if err != nil {
		return nil, fmt.Errorf("下单失败: %w", err)
	}

	go c.sendRiskAsync(u, loginID, instrumentID, volume, lever, isCrossMargin, tradePrice, "买入", "up")

	return convertOrderResponse(orderResp)
}

// MarketSellShortWithRisk 直接市价做空开仓并异步发送风控。
func (c *DirectWebClient) MarketSellShortWithRisk(
	instrumentID string,
	volume, lever, isCrossMargin int,
	loginID string,
	tradePrice float64,
) (*utils.WebOrderResponse, error) {
	u, err := c.resolveUser(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取 Web 用户凭证失败: %w", err)
	}

	orderResp, err := pcweb.SendOrderInsert(u, &pcweb.OrderRequest{
		InstrumentID:   instrumentID,
		Volume:         volume,
		Direction:      "1",
		OrderPriceType: "4",
		Price:          "",
		OffsetFlag:     "0",
		IsCrossMargin:  isCrossMargin,
		Lever:          lever,
	})
	if err != nil {
		return nil, fmt.Errorf("下单失败: %w", err)
	}

	go c.sendRiskAsync(u, loginID, instrumentID, volume, lever, isCrossMargin, tradePrice, "卖出", "down")

	return convertOrderResponse(orderResp)
}

// sendRiskAsync 异步发送下单风控埋点。
// Behavior/Browser/Viewport 均交给 pc_trade/web 包按 LoginID 自动生成（指纹稳定 + 行为伪造）。
func (c *DirectWebClient) sendRiskAsync(
	u *user.User,
	loginID, instrumentID string,
	volume, lever, isCrossMargin int,
	tradePrice float64,
	tradeType, priceTrend string,
) {
	marginModel := "全仓合仓"
	if isCrossMargin == 0 {
		marginModel = "逐仓"
	}
	if err := pcweb.SendTradeRiskRequest(u, &pcweb.TradeRiskRequest{
		LoginID:               loginID,
		InstrumentIDName:      instrumentID,
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "净仓模式",
		TradeType:             tradeType,
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             lever,
		LeverageK:             lever,
		TradePrice:            tradePrice,
		TradeVolume:           volume,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           marginModel,
		IsReduceOnly:          false,
		CoinType:              "CNY",
		LanguageType:          "简体中文",
		EnvPlatform:           "web_desktop",
		ProductionVersion:     "国际版",
		PriceTrend:            priceTrend,
	}); err != nil {
		logrus.Warnf("风控请求失败（不影响下单）: %v", err)
	}
}
