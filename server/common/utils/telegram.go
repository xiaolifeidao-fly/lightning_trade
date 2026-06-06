package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// TelegramAPIBaseURL Telegram Bot API 基础URL
	TelegramAPIBaseURL = "https://api.telegram.org"
	// DefaultTimeout 默认请求超时时间
	DefaultTimeout = 10 * time.Second
	// DefaultBotToken 默认Bot Token
	DefaultBotToken = "8485198554:AAEnX9lMZ8XILcNr-r5ltG2DFZKUjfWiz7I"
	// DefaultChatID 默认ChatID
	DefaultChatID = "-5235555652"
)

// TelegramClient Telegram客户端
type TelegramClient struct {
	botToken   string
	chatID     string
	client     *http.Client
	apiBaseURL string // 用于测试时替换API地址
}

// SendMessageRequest 发送消息请求结构
type SendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// SendMessageResponse Telegram API响应结构
type SendMessageResponse struct {
	OK     bool `json:"ok"`
	Result *struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// NewTelegramClient 创建新的Telegram客户端
// botToken: Bot Token，如果为空则使用默认值
// chatID: 聊天ID（可以是用户ID或群组ID），如果为空则使用默认值
func NewTelegramClientWithBotTokenAndChatID(botToken, chatID string) *TelegramClient {
	if botToken == "" {
		botToken = DefaultBotToken
	}
	if chatID == "" {
		chatID = DefaultChatID
	}

	return &TelegramClient{
		botToken:   botToken,
		chatID:     chatID,
		apiBaseURL: TelegramAPIBaseURL,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

func NewTelegramClient() *TelegramClient {
	return NewTelegramClientWithBotTokenAndChatID(DefaultBotToken, DefaultChatID)
}

// setAPIBaseURL 设置API基础URL（主要用于测试）
func (tc *TelegramClient) setAPIBaseURL(url string) {
	tc.apiBaseURL = url
}

// SendMessage 发送文本消息到Telegram
// message: 要发送的消息内容
// 返回是否发送成功和错误信息
func (tc *TelegramClient) SendMessage(message string) (bool, error) {
	if tc.chatID == "" {
		return false, fmt.Errorf("chat_id is required")
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", tc.apiBaseURL, tc.botToken)

	requestBody := SendMessageRequest{
		ChatID: tc.chatID,
		Text:   message,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		logrus.Errorf("Failed to marshal request body: %v", err)
		return false, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logrus.Errorf("Failed to create request: %v", err)
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头，参照Python脚本的实现
	req.Header.Set("User-Agent", "Apifox/1.0.0 (https://apifox.com)")
	req.Header.Set("Content-Type", "application/json")

	resp, err := tc.client.Do(req)
	if err != nil {
		logrus.Errorf("Failed to send Telegram message: %v", err)
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Failed to read response body: %v", err)
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var response SendMessageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		logrus.Errorf("Failed to unmarshal response: %v, body: %s", err, string(body))
		return false, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !response.OK {
		logrus.Errorf("Telegram API error: %s (code: %d)", response.Description, response.ErrorCode)
		return false, fmt.Errorf("telegram API error: %s (code: %d)", response.Description, response.ErrorCode)
	}

	logrus.Infof("Telegram message sent successfully: %s", message)
	return true, nil
}
