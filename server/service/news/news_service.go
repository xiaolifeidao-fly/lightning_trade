package news

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"encoding/json"
	"fmt"
	newsDTO "service/news/dto"
	newsRepository "service/news/repository"
	"strings"
	"time"
)

type NewsService struct {
	repository *newsRepository.NewsSentimentRepository
}

func NewNewsService() *NewsService {
	return &NewsService{
		repository: db.GetRepository[newsRepository.NewsSentimentRepository](),
	}
}

func (s *NewsService) EnsureTable() error {
	return s.repository.EnsureTable()
}

// SaveSentiment 追加一条消息面快照（每次刷新落一行，形成时间序列）。
func (s *NewsService) SaveSentiment(dto newsDTO.NewsSentimentSaveDTO) error {
	if s.repository.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(dto.CoinCode))
	if coinCode == "" {
		return fmt.Errorf("coinCode 不能为空")
	}
	fetched := dto.FetchedTime
	if fetched.IsZero() {
		fetched = time.Now()
	}

	entity := &newsRepository.NewsSentiment{
		CoinCode:    coinCode,
		Sentiment:   strings.TrimSpace(dto.Sentiment),
		Score:       dto.Score,
		KeyEvents:   marshalList(dto.KeyEvents),
		RiskFlags:   marshalList(dto.RiskFlags),
		AsOf:        strings.TrimSpace(dto.AsOf),
		Freshness:   strings.TrimSpace(dto.Freshness),
		Summary:     strings.TrimSpace(dto.Summary),
		Model:       strings.TrimSpace(dto.Model),
		Provider:    strings.TrimSpace(dto.Provider),
		RawResponse: dto.RawResponse,
		FetchedTime: fetched,
	}
	_, err := s.repository.Create(entity)
	return err
}

// GetLatest 取指定币种最新一条消息面；无记录返回 (nil, nil)。
func (s *NewsService) GetLatest(coinCode string) (*newsDTO.NewsSentimentDTO, error) {
	entity, err := s.repository.FindLatestByCoin(coinCode)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	return toDTO(entity), nil
}

// ListSentiments 按币种与时间范围分页查询历史消息面。
func (s *NewsService) ListSentiments(query newsDTO.NewsSentimentQueryDTO) (*baseDTO.PageDTO[newsDTO.NewsSentimentDTO], error) {
	if s.repository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.repository.CountByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.repository.ListByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]*newsDTO.NewsSentimentDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, toDTO(row))
	}
	return baseDTO.BuildPage(int(total), list), nil
}

func toDTO(entity *newsRepository.NewsSentiment) *newsDTO.NewsSentimentDTO {
	return &newsDTO.NewsSentimentDTO{
		BaseDTO: baseDTO.BaseDTO{
			Id:          entity.Id,
			Active:      entity.Active,
			CreatedTime: entity.CreatedTime,
			CreatedBy:   entity.CreatedBy,
			UpdatedTime: entity.UpdatedTime,
			UpdatedBy:   entity.UpdatedBy,
		},
		CoinCode:    entity.CoinCode,
		Sentiment:   entity.Sentiment,
		Score:       entity.Score,
		KeyEvents:   unmarshalList(entity.KeyEvents),
		RiskFlags:   unmarshalList(entity.RiskFlags),
		AsOf:        entity.AsOf,
		Freshness:   entity.Freshness,
		Summary:     entity.Summary,
		Model:       entity.Model,
		Provider:    entity.Provider,
		FetchedTime: entity.FetchedTime,
	}
}

func marshalList(list []string) string {
	if len(list) == 0 {
		return "[]"
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func normalizePage(page, pageIndex, pageSize int) (int, int) {
	if pageIndex <= 0 {
		pageIndex = page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}
	return pageIndex, pageSize
}
