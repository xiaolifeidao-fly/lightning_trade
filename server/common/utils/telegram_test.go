package utils

import (
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
	client := NewTelegramClient()
	success, err := client.SendMessage("测试消息")
	assert.True(t, success)
	assert.NoError(t, err)
}
