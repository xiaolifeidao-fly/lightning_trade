package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultLoginTimeout   = 120 * time.Second
	defaultSwapTargetPath = "/swap/BTCUSDT"
)

// PlInstanceURL is the base URL of the pl-instance Node.js service.
// Override via the PL_INSTANCE_URL environment variable or set directly.
var PlInstanceURL = "http://localhost:8765"

type DeepCoinLoginService struct {
	baseURL    string
	httpClient *http.Client
}

type DeepCoinLoginResult struct {
	ResourceID      string
	LoginURL        string
	FinalURL        string
	Cookie          string
	Token           string
	OToken          string
	SentryRelease   string
	SentryPublicKey string
	Baggage         string
	Storage         map[string]string
	SessionStorage  map[string]string
}

func NewDeepCoinLoginService() *DeepCoinLoginService {
	return &DeepCoinLoginService{
		baseURL: PlInstanceURL,
		httpClient: &http.Client{
			Timeout: defaultLoginTimeout + 10*time.Second,
		},
	}
}

func (s *DeepCoinLoginService) Login(ctx context.Context, acc AccountConfig) (*DeepCoinLoginResult, error) {
	if acc.Username == "" || acc.Password == "" {
		return nil, fmt.Errorf("DeepCoin 登录缺少账号或密码")
	}

	loginURL := buildDeepCoinLoginURL(acc.LoginURL, time.Now())
	logrus.Infof("DeepCoin 通过 pl-instance 登录 account=%s loginURL=%s", acc.Name, loginURL)

	// 每次登录前重新读取 session.json，确保拿到最新的 secretKey（服务启动后用户可能手动写入）
	freshStore := NewSessionStore(defaultSessionFilePath)
	if err := freshStore.Load(); err == nil {
		if entry, ok := freshStore.Get(acc); ok {
			acc.GoogleAuthKey = firstNonEmpty(acc.GoogleAuthKey, entry.GoogleAuthKey, entry.SecretKey)
			if acc.SecretKey == "" {
				acc.SecretKey = entry.SecretKey
			}
		}
	}

	reqBody := map[string]interface{}{
		"username":         acc.Username,
		"password":         acc.Password,
		"loginURL":         acc.LoginURL,
		"headless":         acc.LoginHeadless,
		"skipSessionCheck": true,
	}
	googleAuthKey := firstNonEmpty(acc.GoogleAuthKey, acc.SecretKey)
	if googleAuthKey != "" {
		reqBody["googleAuthKey"] = googleAuthKey
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化登录请求失败: %w", err)
	}

	endpoint := strings.TrimRight(s.baseURL, "/") + "/api/deepcoin/login"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("构建 pl-instance 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 pl-instance /api/deepcoin/login 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 pl-instance 响应失败: %w", err)
	}

	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Data  struct {
			ResourceID      string            `json:"resourceId"`
			LoginURL        string            `json:"loginURL"`
			FinalURL        string            `json:"finalURL"`
			Cookie          string            `json:"cookie"`
			Token           string            `json:"token"`
			OToken          string            `json:"oToken"`
			SentryRelease   string            `json:"sentryRelease"`
			SentryPublicKey string            `json:"sentryPublicKey"`
			Baggage         string            `json:"baggage"`
			Storage         map[string]string `json:"storage"`
			SessionStorage  map[string]string `json:"sessionStorage"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 pl-instance 响应失败: %w (body=%s)", err, string(body))
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("pl-instance 登录失败: %s", apiResp.Error)
	}

	d := apiResp.Data
	logrus.Infof("pl-instance 登录结果 cookieLen=%d token=%q oToken=%q", len(d.Cookie), d.Token, d.OToken)

	result := &DeepCoinLoginResult{
		ResourceID:      d.ResourceID,
		LoginURL:        firstNonEmpty(d.LoginURL, loginURL),
		FinalURL:        d.FinalURL,
		Cookie:          d.Cookie,
		Token:           d.Token,
		OToken:          d.OToken,
		SentryRelease:   d.SentryRelease,
		SentryPublicKey: d.SentryPublicKey,
		Baggage:         d.Baggage,
		Storage:         d.Storage,
		SessionStorage:  d.SessionStorage,
	}

	logrus.Infof("✅ DeepCoin pl-instance 登录成功 account=%s finalURL=%s", acc.Name, result.FinalURL)
	return result, nil
}

// CheckCookieValid 调用 DeepCoin user-status 接口验证 cookie 是否仍然有效。
// 返回 true 表示当前 cookie 对应的登录态仍然有效，无需重新登录。
func (s *DeepCoinLoginService) CheckCookieValid(ctx context.Context, cookie string) (bool, error) {
	if strings.TrimSpace(cookie) == "" {
		return false, nil
	}

	apiURL := "https://www.deepcoin.com/wealth/myb/user-status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("构建 user-status 请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", "https://www.deepcoin.com/turbo/zh/my/dashboard")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("x-requested-with", "XMLHttpRequest")

	checkClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := checkClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("调用 user-status 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取 user-status 响应失败: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		logrus.Warnf("cookie 有效性检查响应解析失败 body=%s: %v", string(body), err)
		return false, nil
	}

	valid := result.Code == 0 && strings.ToLower(strings.TrimSpace(result.Msg)) == "ok"
	logrus.Infof("cookie 有效性检查 code=%d msg=%s valid=%v", result.Code, result.Msg, valid)
	return valid, nil
}

func buildDeepCoinLoginURL(raw string, now time.Time) string {
	base := raw
	if strings.TrimSpace(base) == "" {
		base = DefaultDeepCoinLoginURL
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}

	query := parsed.Query()
	query.Set("status", "login")
	query.Set("timeStamp", strconv.FormatInt(now.UnixMilli(), 10))
	if query.Get("target") == "" {
		query.Set("target", defaultSwapTargetPath)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mapKeys(m map[string]string) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
