package hello

import (
	"argus_single/pkg/trade"
	"common/middleware/routers"
	"common/utils"

	"github.com/gin-gonic/gin"
)

type HelloHandler struct {
	*routers.BaseHandler
}

func NewHelloHandler() *HelloHandler {
	return &HelloHandler{}
}

func (h *HelloHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/hello", h.hello)
	engine.GET("/hello1", h.hello1)
	engine.GET("/helloApi", h.helloApi)
}

func (h *HelloHandler) hello(context *gin.Context) {
	routers.ToJson(context, "hello world", nil)
}

func (h *HelloHandler) hello1(context *gin.Context) {
	err := trade.ExecuteArbitrage("BTC-USDT-SWAP", 63808, 63008)
	if err != nil {
		routers.ToJson(context, "hello world1 error", err)
		return
	}
	routers.ToJson(context, "hello world1", nil)
}

func (h *HelloHandler) helloApi(context *gin.Context) {
	// 使用测试文件中的参数创建客户端
	const (
		testServerURL           = "http://localhost:8899"
		autoTradeTestAPIKey     = "8251da7c-ed24-4aad-bd03-484ffdf0af61"
		autoTradeTestSecretKey  = "956381CBFA3058A9D59E23BE8BB1A8FC"
		autoTradeTestPassphrase = "Aa111111@"
	)

	// 创建 AutoTradeClient
	client := utils.NewAutoTradeClient(
		testServerURL,
		autoTradeTestAPIKey,
		autoTradeTestSecretKey,
		autoTradeTestPassphrase,
	)

	// 构造下单请求，和测试文件中一样
	req := &utils.OrderRequest{
		InstId:      "BTC-USDT-SWAP",
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "market",
		Sz:          "1",
		PosSide:     "long",
		MrgPosition: "merge",
	}

	// 调用下单接口
	resp, err := client.PlaceOrder(req)
	if err != nil {
		routers.ToJson(context, gin.H{
			"error": err.Error(),
		}, err)
		return
	}

	// 返回结果
	routers.ToJson(context, gin.H{
		"message": "下单成功",
		"result":  resp,
	}, nil)
}

