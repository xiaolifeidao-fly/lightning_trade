package argus_event

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// 路由能注册出来且不与既有路径冲突（gin 冲突会直接 panic）。
func TestRegisterHandlerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api")
	NewArgusEventHandler().RegisterHandler(group)
	want := map[string]bool{
		"/api/argus-event/filter-options":     false,
		"/api/argus-event/signals":            false,
		"/api/argus-event/signals/:id":        false,
		"/api/argus-event/signals/:id/slice":  false,
		"/api/argus-event/timeline":           false,
		"/api/argus-event/equity-curve":       false,
		"/api/argus-event/gate-stats":         false,
		"/api/argus-event/instance-summary":   false,
		"/api/argus-event/episodes":           false,
		"/api/argus-event/episodes/:id":       false,
		"/api/argus-event/episode-stats":      false,
		"/api/argus-event/episode-exit-kinds": false,
		"/api/argus-event/slice-compare":      false,
	}
	for _, r := range engine.Routes() {
		if _, ok := want[r.Path]; ok && r.Method == "GET" {
			want[r.Path] = true
		}
	}
	for path, ok := range want {
		if !ok {
			t.Errorf("路由未注册: GET %s", path)
		}
	}
}
