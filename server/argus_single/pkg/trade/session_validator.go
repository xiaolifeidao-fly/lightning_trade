package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"common/cookies"
	"common/utils"
	pcweb "common/utils/pc_trade/web"

	"github.com/sirupsen/logrus"
)

const (
	sessionValidatorAppID = 547798
	sessionValidatorUA    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

// sessionValidatorParams 账户接口签名字段
type sessionValidatorParams struct {
	AppID       int    `json:"appid"`
	ConvertPOST int    `json:"convertPOST"`
	RandomStr   string `json:"randomstr"`
	Timestamp   int64  `json:"timestamp"`
}

// CheckSessionValidViaWAPI 使用 net-wapi 账户接口（与 TestSessionAccountValid 相同的逻辑）
// 检测 session 是否仍然有效。返回 (valid, err)。
func CheckSessionValidViaWAPI(ctx context.Context, entry SessionAccountData) (bool, error) {
	token := firstNonEmpty(entry.OToken, entry.Token)
	cookie := entry.Cookie
	uid := entry.UID

	if token == "" || cookie == "" {
		return false, fmt.Errorf("token 或 cookie 为空")
	}

	// 解析 cookie 获取 device ID
	cookieData, err := cookies.ParseCookieString(cookie)
	if err != nil {
		return false, fmt.Errorf("解析 cookie 失败: %w", err)
	}
	deviceID := cookieData.GetDeviceID()

	// 确保 WASM 签名器已初始化（cmd.go 启动时已调用，这里做兜底）
	if err := pcweb.InitGlobalSigner(ctx); err != nil {
		return false, fmt.Errorf("初始化 WASM 签名器失败: %w", err)
	}
	signer, err := pcweb.GetGlobalSigner()
	if err != nil {
		return false, fmt.Errorf("获取签名器失败: %w", err)
	}

	// 构造签名参数
	randomStr := sessionValidatorRandomStr(6)
	timestamp := time.Now().UnixMilli()

	params := sessionValidatorParams{
		AppID:       sessionValidatorAppID,
		ConvertPOST: 1,
		RandomStr:   randomStr,
		Timestamp:   timestamp,
	}
	message, err := utils.ConvertToMessage(params)
	if err != nil {
		return false, fmt.Errorf("构造签名消息失败: %w", err)
	}
	sign, err := utils.CalculateSign(message)
	if err != nil {
		return false, fmt.Errorf("计算 sign 失败: %w", err)
	}
	hmacVal, err := signer.SignParams(message)
	if err != nil {
		return false, fmt.Errorf("计算 hmac 失败: %w", err)
	}

	// 构造请求
	apiURL := fmt.Sprintf(
		"https://net-wapi.deepcoin.com/user/public/user/account?appid=%d&randomstr=%s&timestamp=%d&convertPOST=1&sign=%s",
		sessionValidatorAppID, randomStr, timestamp, sign,
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("accept", "application/json, text/plain, */*")
	httpReq.Header.Set("accept-language", "zh-CN,zh;q=0.9")
	httpReq.Header.Set("appid", fmt.Sprintf("%d", sessionValidatorAppID))
	httpReq.Header.Set("device", deviceID)
	httpReq.Header.Set("hmac", hmacVal)
	httpReq.Header.Set("lang", "zh")
	httpReq.Header.Set("origin", "https://www.deepcoin.com")
	httpReq.Header.Set("otoken", token)
	httpReq.Header.Set("platform", "pc")
	httpReq.Header.Set("referer", "https://www.deepcoin.com/")
	httpReq.Header.Set("requestid", sessionValidatorRequestID())
	httpReq.Header.Set("timestamp", fmt.Sprintf("%d", timestamp))
	httpReq.Header.Set("token", token)
	httpReq.Header.Set("uid", uid)
	httpReq.Header.Set("user-agent", sessionValidatorUA)
	httpReq.Header.Set("x-requested-with", "XMLHttpRequest")
	httpReq.Header.Set("cookie", cookie)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("解析响应失败: %w (body=%s)", err, string(body))
	}

	valid := result.Code == 0
	logrus.Infof("session 有效性检查(net-wapi) account=%s code=%d msg=%s valid=%v", entry.AccountName, result.Code, result.Msg, valid)
	return valid, nil
}

func sessionValidatorRandomStr(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func sessionValidatorRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ReplaceAll(fmt.Sprintf("%x", b), "-", "")
}
