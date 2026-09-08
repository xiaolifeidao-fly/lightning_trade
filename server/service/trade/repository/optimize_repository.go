package repository

import (
	"common/middleware/db"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ─── 自动参数寻优 study / cell（r16）──────────────────────────────────────────

type TradeOptimizeStudyRepository struct {
	db.Repository[*TradeOptimizeStudy]
}

func (r *TradeOptimizeStudyRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeOptimizeStudy{})
}

// CreateStudy 写入一条寻优任务（status 初始 pending）。
func (r *TradeOptimizeStudyRepository) CreateStudy(study *TradeOptimizeStudy) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	study.Init()
	return r.Db.Create(study).Error
}

// FindStudyByID 按主键查询。
func (r *TradeOptimizeStudyRepository) FindStudyByID(id int64) (*TradeOptimizeStudy, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeOptimizeStudy
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// FindStudies 分页查询，支持按实例/账户/样本口径过滤，按 id 倒序。
func (r *TradeOptimizeStudyRepository) FindStudies(instanceKey, accountLabel, sampleKind string, page, pageSize int) ([]*TradeOptimizeStudy, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeOptimizeStudy{}).Where("active = 1")
	if instanceKey != "" {
		q = q.Where("instance_key = ?", instanceKey)
	}
	if accountLabel != "" {
		q = q.Where("account_label = ?", accountLabel)
	}
	if sampleKind != "" {
		q = q.Where("sample_kind = ?", sampleKind)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var rows []*TradeOptimizeStudy
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// UpdateStudyStage 推进阶段并回填精算格数。
//
// convergeNote 为空时**不写该列**：阶段会推进两次（coarse→fine→concluded），
// 第二次没有新的收敛说明，如果一并写空串会把"精算格是怎么选出来的"这条唯一
// 记录冲掉——那之后就没人能解释这批格子的来历了。
func (r *TradeOptimizeStudyRepository) UpdateStudyStage(id int64, stage string, fineCellCount int, convergeNote string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	updates := map[string]interface{}{
		"stage":           stage,
		"fine_cell_count": fineCellCount,
		"updated_time":    time.Now().UTC(),
	}
	if strings.TrimSpace(convergeNote) != "" {
		updates["converge_note"] = convergeNote
	}
	return r.Db.Model(&TradeOptimizeStudy{}).Where("id = ? AND active = 1", id).Updates(updates).Error
}

// UpdateStudyDataFacts 回填窗口内的数据侧事实（触发数 / K 线数 / 日标签统计）。
func (r *TradeOptimizeStudyRepository) UpdateStudyDataFacts(id int64, signalCount, klineCount, trendDays, volDays int) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeOptimizeStudy{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"signal_count":    signalCount,
		"kline_count":     klineCount,
		"trend_day_count": trendDays,
		"vol_day_count":   volDays,
		"updated_time":    time.Now().UTC(),
	}).Error
}

// BumpStudyProgress 原子推进进度：done/failed/skip/replay 各自累加。
// 不读改写，避免多格并发完成时互相覆盖计数。
func (r *TradeOptimizeStudyRepository) BumpStudyProgress(id int64, doneDelta, failedDelta, skipDelta, replayDelta int) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	updates := map[string]interface{}{"updated_time": time.Now().UTC()}
	for col, delta := range map[string]int{
		"done_cell_count":   doneDelta,
		"failed_cell_count": failedDelta,
		"skip_cell_count":   skipDelta,
		"replay_count":      replayDelta,
	} {
		if delta != 0 {
			updates[col] = gorm.Expr(col+" + ?", delta)
		}
	}
	return r.Db.Model(&TradeOptimizeStudy{}).Where("id = ? AND active = 1", id).Updates(updates).Error
}

// UpdateStudyStatus 更新状态与失败原因。
func (r *TradeOptimizeStudyRepository) UpdateStudyStatus(id int64, status, errMsg string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeOptimizeStudy{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"status":       status,
		"error_msg":    errMsg,
		"updated_time": time.Now().UTC(),
	}).Error
}

// UpdateStudyConclusion 回填结论。**刻意不写 gate_snapshot / gate_locked_at**：
// 阈值是预注册的，任何执行期路径都不允许改它——那是本任务防过拟合的机械保障。
func (r *TradeOptimizeStudyRepository) UpdateStudyConclusion(id int64,
	verdict string, passedCells int, conclusion, frontier, scale string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeOptimizeStudy{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"verdict":           verdict,
		"passed_cell_count": passedCells,
		"conclusion":        conclusion,
		"frontier_snapshot": frontier,
		"scale_snapshot":    scale,
		"updated_time":      time.Now().UTC(),
	}).Error
}

type TradeOptimizeCellRepository struct {
	db.Repository[*TradeOptimizeCell]
}

func (r *TradeOptimizeCellRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeOptimizeCell{})
}

// BatchCreateCells 批量建格。一次性建完再执行：格子建了一半就失败会留下
// 一个"格数与实际不符"的任务，之后没人能判断它是跑完了还是断了。
func (r *TradeOptimizeCellRepository) BatchCreateCells(cells []*TradeOptimizeCell) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(cells) == 0 {
		return nil
	}
	for _, c := range cells {
		c.Init()
	}
	return r.Db.CreateInBatches(cells, 100).Error
}

// FindCellsByStudy 取一个任务下的格子；stage 为空取全部。
// 按 id 升序（= 提交顺序）返回：排序与前沿判定都在服务层做，仓储不掺和排序口径。
func (r *TradeOptimizeCellRepository) FindCellsByStudy(studyID int64, stage string) ([]*TradeOptimizeCell, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Where("study_id = ? AND active = 1", studyID)
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	var rows []*TradeOptimizeCell
	err := q.Order("id ASC").Find(&rows).Error
	return rows, err
}

// UpdateCellStatus 更新格子状态与原因。
func (r *TradeOptimizeCellRepository) UpdateCellStatus(id int64, status, errMsg string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeOptimizeCell{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"status":       status,
		"error_msg":    errMsg,
		"updated_time": time.Now().UTC(),
	}).Error
}

// SaveCellStats 回填一格的降噪产出（不含三关判定——判定要等全格跑完、
// 用任务冻结的阈值统一算一遍，避免同一任务里出现两套阈值）。
func (r *TradeOptimizeCellRepository) SaveCellStats(id int64, fields map[string]interface{}) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(fields) == 0 {
		return nil
	}
	fields["updated_time"] = time.Now().UTC()
	return r.Db.Model(&TradeOptimizeCell{}).Where("id = ? AND active = 1", id).Updates(fields).Error
}

// SaveCellVerdict 回填三关判定、支配关系与前沿标记。
func (r *TradeOptimizeCellRepository) SaveCellVerdict(id int64, fields map[string]interface{}) error {
	return r.SaveCellStats(id, fields)
}
