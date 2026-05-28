package user

import (
	"common/middleware/db"
	"fmt"
	"net/mail"
	userRepository "service/manager_user/repository"
	"strings"

	"gorm.io/gorm"
)

type UserService struct {
	userRepository            *userRepository.UserRepository
	userLoginRecordRepository *userRepository.UserLoginRecordRepository
	userRoleRepository        *userRepository.UserRoleRepository
}

func NewUserService() *UserService {
	return &UserService{
		userRepository:            db.GetRepository[userRepository.UserRepository](),
		userLoginRecordRepository: db.GetRepository[userRepository.UserLoginRecordRepository](),
		userRoleRepository:        db.GetRepository[userRepository.UserRoleRepository](),
	}
}

func (s *UserService) EnsureTable() error {
	if err := s.userRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.userLoginRecordRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.userRoleRepository.EnsureTable(); err != nil {
		return err
	}
	return nil
}

func normalizeUserPage(page, pageIndex, pageSize int) (int, int) {
	if pageIndex <= 0 {
		pageIndex = page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageIndex, pageSize
}

func normalizeUserStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active":
		return "active"
	case "inactive":
		return "inactive"
	case "locked":
		return "locked"
	default:
		return ""
	}
}

func normalizeUserRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "member":
		return "member"
	case "admin":
		return "admin"
	case "manager":
		return "manager"
	case "auditor":
		return "auditor"
	default:
		return ""
	}
}

func validateEmail(email string) error {
	if email == "" {
		return nil
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("email format is invalid")
	}
	return nil
}

func ensureUserExists(repo *userRepository.UserRepository, userID uint64) error {
	if userID == 0 {
		return fmt.Errorf("userId must be positive")
	}
	entity, err := repo.FindById(uint(userID))
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
