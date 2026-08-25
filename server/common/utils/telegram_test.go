package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ==================== 常量测试 ====================

func TestDefaultChatID_Constant(t *testing.T) {
	assert.Equal(t, "-5235555652", DefaultChatID)
}

func TestDefaultBotToken_Constant(t *testing.T) {
	assert.NotEmpty(t, DefaultBotToken)
	assert.Contains(t, DefaultBotToken, ":")
}

func TestTelegramAPIBaseURL_Constant(t *testing.T) {
	assert.Equal(t, "https://api.telegram.org", TelegramAPIBaseURL)
}

func TestDefaultTimeout_Constant(t *testing.T) {
	assert.Equal(t, 10*time.Second, DefaultTimeout)
}

// ==================== SendMessage 成功场景测试 ====================

func TestTelegramClient_SendMessage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/bottest-token/sendMessage", r.URL.Path)

		var request SendMessageRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "test-chat", request.ChatID)
		assert.Equal(t, "测试消息", request.Text)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	client := NewTelegramClientWithBotTokenAndChatID("test-token", "test-chat")
	client.setAPIBaseURL(server.URL)
	success, err := client.SendMessage("测试消息")
	assert.True(t, success)
	assert.NoError(t, err)
}
