// Package argus_event 是 Argus 事件读服务的 HTTP 入口。
//
// 全部接口只读，不提供任何写入或运行控制入口——与 r7 下线 start/stop/restart
// 的口径一致：页面上不存在能影响实盘进程的动作。
package argus_event

import (
	commonRouter "common/middleware/routers"
	"errors"
	eventService "service/argus_event"
	eventDTO "service/argus_event/dto"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ArgusEventHandler struct {
	*commonRouter.BaseHandler
	service *eventService.ArgusEventService
}

func NewArgusEventHandler() *ArgusEventHandler {
	svc := eventService.NewArgusEventService()
	// 事件表由 argus_single（r4）拥有；管理端先起来时这里也建一次，
	// 好让四个页面在事件还没进来之前查出空集而不是报未知表。
	_ = svc.EnsureTable()
	return &ArgusEventHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		service:     svc,
	}
}

func (h *ArgusEventHandler) RegisterHandler(engine *gin.RouterGroup) {
	// 筛选项：实例 / 账户 / 合约 / 变体 / 参数版本 / 事件类型 / 门控类型 / 强度分档
	engine.GET("/argus-event/filter-options", h.filterOptions)

	// 信号列表与详情。/signals/:id/slice 必须在 :id 之前注册，
	// 否则 gin 会把 "slice" 当成 id 段（与 strategy 里 summary 先注册同因）。
	engine.GET("/argus-event/signals", h.listSignals)
	engine.GET("/argus-event/signals/:id/slice", h.getSignalSlice)
	engine.GET("/argus-event/signals/:id", h.getSignalDetail)

	// 聚合读：K 线对齐时间轴 / 权益曲线 / 拦截原因 / 跨实例汇总对比
	engine.GET("/argus-event/timeline", h.getTimeline)
	engine.GET("/argus-event/equity-curve", h.getEquityCurve)
	engine.GET("/argus-event/gate-stats", h.getGateStats)
	engine.GET("/argus-event/instance-summary", h.getInstanceSummary)

	// 持仓 episode（r10 派生表的读路径）。出场方式筛选项走独立路径，
	// 不挂在 /episodes 下面，省得再跟 :id 抢段。
	engine.GET("/argus-event/episodes", h.listEpisodes)
	engine.GET("/argus-event/episodes/:id", h.getEpisodeDetail)
	engine.GET("/argus-event/episode-stats", h.getEpisodeStats)
	engine.GET("/argus-event/episode-exit-kinds", h.listExitKinds)

	// 按 (实例, 参数版本) 切片的横向对比。
	engine.GET("/argus-event/slice-compare", h.getSliceCompare)
}

func (h *ArgusEventHandler) filterOptions(c *gin.Context) {
	result, err := h.service.GetFilterOptions(c.Query("instanceKey"), c.Query("instanceKeys"))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) listSignals(c *gin.Context) {
	var query eventDTO.SignalQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.ListSignals(query)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getSignalDetail(c *gin.Context) {
	id, err := parseEventID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.service.GetSignalDetail(uint64(id))
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, eventService.ErrEventNotFound) {
		commonRouter.ToError(c, "事件不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getSignalSlice(c *gin.Context) {
	id, err := parseEventID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	var query eventDTO.SliceQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetSignalSlice(uint64(id), query)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, eventService.ErrEventNotFound) {
		commonRouter.ToError(c, "事件不存在")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getTimeline(c *gin.Context) {
	var query eventDTO.TimelineQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetTimeline(query)
	if errors.Is(err, eventService.ErrInstanceKeyRequired) {
		commonRouter.ToError(c, "instanceKey 必填：净持仓与触发率是实例内的量，跨实例合并会串数据")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getEquityCurve(c *gin.Context) {
	var query eventDTO.EquityQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetEquityCurve(query)
	if errors.Is(err, eventService.ErrInstanceKeyRequired) {
		commonRouter.ToError(c, "instanceKey 必填：账户唯一性是 (实例, 账户)，跨实例合并会串数据")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getGateStats(c *gin.Context) {
	var query eventDTO.GateStatsQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetGateStats(query)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getInstanceSummary(c *gin.Context) {
	var query eventDTO.InstanceSummaryQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetInstanceSummary(query)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) listEpisodes(c *gin.Context) {
	var query eventDTO.EpisodeQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.ListEpisodes(query)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getEpisodeDetail(c *gin.Context) {
	id, err := parseEventID(c)
	if err != nil {
		commonRouter.ToError(c, "id 格式错误")
		return
	}
	result, err := h.service.GetEpisodeDetail(uint64(id))
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, eventService.ErrEpisodeNotFound) {
		commonRouter.ToError(c, "持仓不存在：可能是还没跑过 argus-episode-rebuild")
		return
	}
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getEpisodeStats(c *gin.Context) {
	var query eventDTO.EpisodeStatsQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetEpisodeStats(query)
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) listExitKinds(c *gin.Context) {
	keys := c.Query("instanceKeys")
	if keys == "" {
		keys = c.Query("instanceKey")
	}
	result, err := h.service.ListExitKindOptions(splitCSV(keys))
	commonRouter.ToJson(c, result, err)
}

func (h *ArgusEventHandler) getSliceCompare(c *gin.Context) {
	var query eventDTO.SliceCompareQueryDTO
	if err := c.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.service.GetSliceCompare(query)
	commonRouter.ToJson(c, result, err)
}

// splitCSV 逗号分隔串 → 去空白去空项的切片，与 service 侧同口径。
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseEventID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}
