package user

import (
	"fmt"
	"strings"

	userDTO "service/manager_user/dto"
	userRepository "service/manager_user/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 账户状态取值。前端只会发这两个（用户管理页的冻结/解冻按钮），
// 服务端按白名单收，别让任意串写进库。
const (
	AccountStatusNormal = "normal"
	AccountStatusFrozen = "frozen"
)

func normalizeAccountStatus(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case AccountStatusNormal:
		return AccountStatusNormal, nil
	case AccountStatusFrozen:
		return AccountStatusFrozen, nil
	case "":
		return "", fmt.Errorf("account status is required")
	default:
		return "", fmt.Errorf("account status must be %s or %s", AccountStatusNormal, AccountStatusFrozen)
	}
}

// parseBalance 解析余额。
//
// 用 decimal 而不是 float64：列是 decimal(38,8)，金额过一遍二进制浮点就会出现
// 0.1+0.2 那类误差，而这里是真钱。负数直接拒绝——前端的充值是
// 「当前余额 + 充值额」算好再发过来的，出现负数说明上游算错了，
// 与其静默存进去不如报错。
func parseBalance(raw string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decimal.Zero, fmt.Errorf("balance amount is required")
	}
	amount, err := decimal.NewFromString(trimmed)
	if err != nil {
		return decimal.Zero, fmt.Errorf("balance amount is not a valid number: %s", trimmed)
	}
	if amount.IsNegative() {
		return decimal.Zero, fmt.Errorf("balance amount must not be negative: %s", trimmed)
	}
	return amount, nil
}

func toAccountDTO(entity *userRepository.Account) *userDTO.AccountDTO {
	if entity == nil {
		return nil
	}
	out := &userDTO.AccountDTO{
		UserID:        entity.UserID,
		AccountStatus: entity.AccountStatus,
		// StringFixed(8) 与列宽一致，避免同一个值在不同请求里回来两种写法。
		BalanceAmount: entity.BalanceAmount.StringFixed(8),
	}
	out.Id = entity.Id
	out.Active = entity.Active
	out.CreatedTime = entity.CreatedTime
	out.UpdatedTime = entity.UpdatedTime
	return out
}

// CreateAccount 给用户建资金账户。
//
// 一个用户只允许一个生效账户：前端是按「record.accountId 为空才新建」写的，
// 真让同一个用户有两条生效账户，用户列表的余额就会取到不确定的那一条。
func (s *UserService) CreateAccount(req *userDTO.CreateAccountDTO) (*userDTO.AccountDTO, error) {
	if s.accountRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	status, err := normalizeAccountStatus(req.AccountStatus)
	if err != nil {
		return nil, err
	}
	balance, err := parseBalance(req.BalanceAmount)
	if err != nil {
		return nil, err
	}

	// 用户必须存在且生效，否则会造出一条挂不到任何人身上的账户。
	owner, err := s.userRepository.FindById(uint(req.UserID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found: %d", req.UserID)
		}
		return nil, err
	}
	if owner == nil || owner.Active == 0 {
		return nil, fmt.Errorf("user not found: %d", req.UserID)
	}

	existing, err := s.accountRepository.FindActiveByUserID(req.UserID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == nil && existing != nil && existing.Id > 0 {
		return nil, fmt.Errorf("account already exists for user %d", req.UserID)
	}

	created, err := s.accountRepository.Create(&userRepository.Account{
		UserID:        req.UserID,
		AccountStatus: status,
		BalanceAmount: balance,
	})
	if err != nil {
		return nil, err
	}
	return toAccountDTO(created), nil
}

// UpdateAccount 局部更新余额或状态。nil 字段表示本次不改。
func (s *UserService) UpdateAccount(id int, req *userDTO.UpdateAccountDTO) (*userDTO.AccountDTO, error) {
	if s.accountRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if id <= 0 {
		return nil, fmt.Errorf("account id is invalid")
	}
	entity, err := s.accountRepository.FindActiveByID(id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	changed := false
	if req.AccountStatus != nil {
		status, err := normalizeAccountStatus(*req.AccountStatus)
		if err != nil {
			return nil, err
		}
		entity.AccountStatus = status
		changed = true
	}
	if req.BalanceAmount != nil {
		balance, err := parseBalance(*req.BalanceAmount)
		if err != nil {
			return nil, err
		}
		entity.BalanceAmount = balance
		changed = true
	}
	if !changed {
		return nil, fmt.Errorf("nothing to update: both accountStatus and balanceAmount are absent")
	}

	saved, err := s.accountRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return toAccountDTO(saved), nil
}
