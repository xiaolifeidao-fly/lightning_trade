package news

import (
	commonRouter "common/middleware/routers"
	newsService "service/news"
	newsDTO "service/news/dto"

	"github.com/gin-gonic/gin"
)

type NewsHandler struct {
	*commonRouter.BaseHandler
	newsService *newsService.NewsService
}

func NewNewsHandler() *NewsHandler {
	service := newsService.NewNewsService()
	_ = service.EnsureTable()
	return &NewsHandler{
		BaseHandler: &commonRouter.BaseHandler{},
		newsService: service,
	}
}

func (h *NewsHandler) RegisterHandler(engine *gin.RouterGroup) {
	engine.GET("/news-sentiments", h.listSentiments)
}

func (h *NewsHandler) listSentiments(context *gin.Context) {
	var query newsDTO.NewsSentimentQueryDTO
	if err := context.ShouldBindQuery(&query); err != nil {
		commonRouter.ToError(context, "参数错误")
		return
	}
	result, err := h.newsService.ListSentiments(query)
	commonRouter.ToJson(context, result, err)
}
