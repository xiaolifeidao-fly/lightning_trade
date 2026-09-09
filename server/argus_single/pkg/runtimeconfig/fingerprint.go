package runtimeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"argus_single/pkg/monitor"
	"argus_single/pkg/trade"
)

// configFingerprintView 是 RuntimeConfig 的显式镜像，专门用于求内容指纹。
//
// 为什么不直接 json.Marshal(RuntimeConfig)：它的 Notification 字段类型
// notificationConfig 的三个字段（enabled/token/chatID）都是**未导出**的，
// encoding/json 会静默跳过——那样轮换 Telegram token 就检测不到变更，
// 而且这种漏检不会有任何报错。显式镜像把"哪些字段参与指纹"变成一份可读、
// 可测的清单；新增字段忘记纳入时，fingerprint_test.go 的字段数守卫会失败。
//
// 同时故意**排除** Version 与 Checksum：
//   - Checksum 是"发布那一刻的快照校验和"，运维直接改库不会改动它；
//   - Version 同理，回滚场景下同一版本号会被重新发布。
//
// 指纹只对"影响进程行为的内容"求取，所以它能覆盖上面两种版本号覆盖不到的场景。
type configFingerprintView struct {
	Trade       *trade.TradingSystemConfig      `json:"trade"`
	Symbols     map[string]monitor.SymbolConfig `json:"symbols"`
	ServerPort  uint16                          `json:"serverPort"`
	RequestPath string                          `json:"requestPath"`
	LogDir      string                          `json:"logDir"`
	// Notification 摊平成三个导出字段，绕开 notificationConfig 的未导出字段。
	NotifyEnabled bool                 `json:"notifyEnabled"`
	NotifyToken   string               `json:"notifyToken"`
	NotifyChatID  string               `json:"notifyChatId"`
	Tuning        RuntimeTuning        `json:"tuning"`
	Overrides     trade.ParamOverrides `json:"overrides"`
}

// configFingerprint 求派生运行配置的内容指纹。
//
// 用 encoding/json 而不是 fmt：它对 map 的键做排序，所以 Symbols 与
// Overrides 这两个 map 字段的序列化是确定性的，同样内容不会算出两个指纹。
func configFingerprint(c RuntimeConfig) (string, error) {
	view := configFingerprintView{
		Trade:         c.Trade,
		Symbols:       c.Symbols,
		ServerPort:    c.ServerPort,
		RequestPath:   c.RequestPath,
		LogDir:        c.LogDir,
		NotifyEnabled: c.Notification.enabled,
		NotifyToken:   c.Notification.token,
		NotifyChatID:  c.Notification.chatID,
		Tuning:        c.Tuning,
		Overrides:     c.Overrides,
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("marshal config fingerprint view: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
