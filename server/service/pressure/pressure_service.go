package pressure

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"encoding/json"
	"fmt"
	pressureDTO "service/pressure/dto"
	pressureRepository "service/pressure/repository"
	"strings"
	"time"
)

type PressureService struct {
	repository *pressureRepository.PressureAnalysisRepository
}

func NewPressureService() *PressureService {
	return &PressureService{
		repository: db.GetRepository[pressureRepository.PressureAnalysisRepository](),
	}
}

func (s *PressureService) EnsureTable() error {
	return s.repository.EnsureTable()
}

// SavePressure 追加一条压力面分析快照（每次分析落一行，形成时间序列）。
func (s *PressureService) SavePressure(dto pressureDTO.PressureAnalysisSaveDTO) error {
	if s.repository.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(dto.CoinCode))
	if coinCode == "" {
		return fmt.Errorf("coinCode 不能为空")
	}
	analyzed := dto.AnalyzedTime
	if analyzed.IsZero() {
		analyzed = time.Now()
	}

	entity := &pressureRepository.PressureAnalysis{
		PlatformCode:        strings.TrimSpace(dto.PlatformCode),
		CoinCode:            coinCode,
		Symbol:              strings.TrimSpace(dto.Symbol),
		Interval:            strings.TrimSpace(dto.Interval),
		RefPrice:            dto.RefPrice,
		Bias:                strings.TrimSpace(dto.Bias),
		ShortPressureLevels: marshalLevels(dto.ShortPressureLevels),
		LongPressureLevels:  marshalLevels(dto.LongPressureLevels),
		KeyResistance:       dto.KeyResistance,
		KeySupport:          dto.KeySupport,
		Summary:             strings.TrimSpace(dto.Summary),
		NewsSummary:         strings.TrimSpace(dto.NewsSummary),
		Model:               strings.TrimSpace(dto.Model),
		Provider:            strings.TrimSpace(dto.Provider),
		RawResponse:         dto.RawResponse,
		AnalyzedTime:        analyzed,
	}
	_, err := s.repository.Create(entity)
	return err
}

// GetLatest 取指定币种最新一条压力面分析；无记录返回 (nil, nil)。
func (s *PressureService) GetLatest(coinCode string) (*pressureDTO.PressureAnalysisDTO, error) {
	entity, err := s.repository.FindLatestByCoin(coinCode)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	return toDTO(entity), nil
}

// ListPressures 按平台、币种与时间范围分页查询历史压力面分析。
func (s *PressureService) ListPressures(query pressureDTO.PressureAnalysisQueryDTO) (*baseDTO.PageDTO[pressureDTO.PressureAnalysisDTO], error) {
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
	list := make([]*pressureDTO.PressureAnalysisDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, toDTO(row))
	}
	return baseDTO.BuildPage(int(total), list), nil
}

func toDTO(entity *pressureRepository.PressureAnalysis) *pressureDTO.PressureAnalysisDTO {
	return &pressureDTO.PressureAnalysisDTO{
		BaseDTO: baseDTO.BaseDTO{
			Id:          entity.Id,
			Active:      entity.Active,
			CreatedTime: entity.CreatedTime,
			CreatedBy:   entity.CreatedBy,
			UpdatedTime: entity.UpdatedTime,
			UpdatedBy:   entity.UpdatedBy,
		},
		PlatformCode:        entity.PlatformCode,
		CoinCode:            entity.CoinCode,
		Symbol:              entity.Symbol,
		Interval:            entity.Interval,
		RefPrice:            entity.RefPrice,
		Bias:                entity.Bias,
		ShortPressureLevels: unmarshalLevels(entity.ShortPressureLevels),
		LongPressureLevels:  unmarshalLevels(entity.LongPressureLevels),
		KeyResistance:       entity.KeyResistance,
		KeySupport:          entity.KeySupport,
		Summary:             entity.Summary,
		NewsSummary:         entity.NewsSummary,
		Model:               entity.Model,
		Provider:            entity.Provider,
		AnalyzedTime:        entity.AnalyzedTime,
	}
}

func marshalLevels(list []pressureDTO.PressureLevel) string {
	if len(list) == 0 {
		return "[]"
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalLevels(raw string) []pressureDTO.PressureLevel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []pressureDTO.PressureLevel{}
	}
	var out []pressureDTO.PressureLevel
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []pressureDTO.PressureLevel{}
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
