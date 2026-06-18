package web_auth

import (
	"common/middleware/db"
	redisMiddleware "common/middleware/redis"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	permissionRepository "service/manager_permission/repository"
	userRepository "service/manager_user/repository"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	userTokenPrefix     = "KAKROLOT_USER_TOKEN_PRE_"
	userRolePrefix      = "KAKROLOT_USER_ROLE_PRE_KEY_"
	userIPLoginPrefix   = "Kakrolot_user_ip_login_"
	tokenExpireSeconds  = 7 * 24 * 60 * 60
	resourceExpireHours = 24 * 60 * 60
)

var (
	ErrNotLogin           = errors.New("user not login")
	ErrUserNoResource     = errors.New("user not resource")
	ErrInvalidCredential  = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("该用户已被封禁")
	ErrLoginTooManyErrors = errors.New("用户密码错误次数太多,请一小时后再试")
)

type LoginUser struct {
	ID        uint64   `json:"id"`
	Name      string   `json:"name"`
	Username  string   `json:"username"`
	Role      string   `json:"role"`
	Status    string   `json:"status"`
	RoleIDs   []uint64 `json:"roleIds,omitempty"`
}

type AuthService struct {
	userRepository            *userRepository.UserRepository
	userLoginRecordRepository *userRepository.UserLoginRecordRepository
	userRoleRepository        *userRepository.UserRoleRepository
	resourceRepository        *permissionRepository.ResourceRepository
	roleResourceRepository    *permissionRepository.RoleResourceRepository
}

func NewAuthService() *AuthService {
	return &AuthService{
		userRepository:            db.GetRepository[userRepository.UserRepository](),
		userLoginRecordRepository: db.GetRepository[userRepository.UserLoginRecordRepository](),
		userRoleRepository:        db.GetRepository[userRepository.UserRoleRepository](),
		resourceRepository:        db.GetRepository[permissionRepository.ResourceRepository](),
		roleResourceRepository:    db.GetRepository[permissionRepository.RoleResourceRepository](),
	}
}

func (s *AuthService) Login(username, password, ip string, maxLoginErrorNum int64) (string, *LoginUser, error) {
	if err := ensureRedisReady(); err != nil {
		return "", nil, err
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	ip = strings.TrimSpace(ip)
	if username == "" || password == "" {
		return "", nil, ErrInvalidCredential
	}
	if s.isLimit(ip, maxLoginErrorNum) {
		return "", nil, ErrLoginTooManyErrors
	}
	if s.userRepository.Db == nil {
		return "", nil, fmt.Errorf("database is not initialized")
	}

	user, err := s.userRepository.FindByUsername(username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.calLoginError(ip)
			return "", nil, ErrInvalidCredential
		}
		return "", nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(user.Status), "active") {
		return "", nil, ErrUserDisabled
	}
	encryptedPassword := encryptPassword(username, password)
	if !strings.EqualFold(encryptedPassword, strings.TrimSpace(user.Password)) {
		s.calLoginError(ip)
		return "", nil, ErrInvalidCredential
	}

	user.LastLoginTime = time.Now()
	if _, err := s.userRepository.SaveOrUpdate(user); err != nil {
		return "", nil, err
	}
	if _, err := s.userLoginRecordRepository.Create(&userRepository.UserLoginRecord{
		IP:     ip,
		UserID: uint64(user.Id),
	}); err != nil {
		return "", nil, err
	}

	loginUser := toLoginUser(user)
	token, err := s.initAndGetToken(loginUser)
	if err != nil {
		return "", nil, err
	}
	return token, loginUser, nil
}

func (s *AuthService) ValidateToken(token, requestURL string) (*LoginUser, error) {
	if err := ensureRedisReady(); err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNotLogin
	}

	loginUser, err := s.findUserByToken(token)
	if err != nil {
		return nil, err
	}
	if loginUser == nil {
		return nil, ErrNotLogin
	}

	roleIDs, err := s.findRoleIDsByUserID(loginUser.ID)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return nil, ErrUserNoResource
	}

	// 暂时不校验资源权限
	hasResource, err := s.hadResource(roleIDs, requestURL)
	if err != nil {
		return nil, err
	}
	if !hasResource {
		return nil, ErrUserNoResource
	}

	loginUser.RoleIDs = roleIDs
	if err := s.flushExpireTime(token); err != nil {
		return nil, err
	}
	return loginUser, nil
}

func (s *AuthService) initAndGetToken(user *LoginUser) (string, error) {
	payload, err := json.Marshal(user)
	if err != nil {
		return "", err
	}
	token := strings.ReplaceAll(fmt.Sprintf("%d_%d", user.ID, time.Now().UnixNano()), "-", "")
	sessionKey := buildUserTokenKey(token)
	redisMiddleware.SetEx(sessionKey, string(payload), tokenExpireSeconds)
	return token, nil
}

func (s *AuthService) findUserByToken(token string) (*LoginUser, error) {
	value := redisMiddleware.Get(buildUserTokenKey(token))
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var user LoginUser
	if err := json.Unmarshal([]byte(value), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) flushExpireTime(token string) error {
	redisMiddleware.Expire(buildUserTokenKey(token), time.Duration(tokenExpireSeconds)*time.Second)
	return nil
}

func (s *AuthService) findRoleIDsByUserID(userID uint64) ([]uint64, error) {
	key := buildUserRoleKey(userID)
	if value := redisMiddleware.Get(key); strings.TrimSpace(value) != "" {
		return parseUint64List(value)
	}
	if s.userRoleRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entities []*userRepository.UserRole
	if err := s.userRoleRepository.Db.
		Where("user_id = ? AND active = ?", userID, 1).
		Order("id DESC").
		Find(&entities).Error; err != nil {
		return nil, err
	}
	roleIDs := make([]uint64, 0, len(entities))
	for _, entity := range entities {
		roleIDs = append(roleIDs, entity.RoleID)
	}
	if len(roleIDs) == 0 {
		return []uint64{}, nil
	}
	redisMiddleware.SetEx(key, joinUint64List(roleIDs), resourceExpireHours)
	return roleIDs, nil
}

func (s *AuthService) hadResource(roleIDs []uint64, resourceURL string) (bool, error) {
	resourceURL = normalizeResourceURL(resourceURL)
	resource, err := s.findResourceByURL(resourceURL)
	if err != nil {
		return false, err
	}
	if resource == nil {
		return false, nil
	}
	if s.roleResourceRepository.Db == nil {
		return false, fmt.Errorf("database is not initialized")
	}
	var count int64
	if err := s.roleResourceRepository.Db.
		Model(&permissionRepository.RoleResource{}).
		Where("role_id IN ? AND resource_id = ? AND active = ?", roleIDs, resource.Id, 1).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *AuthService) findResourceByURL(resourceURL string) (*permissionRepository.Resource, error) {
	if s.resourceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	candidates := buildResourceURLCandidates(resourceURL)
	for _, candidate := range candidates {
		var entity permissionRepository.Resource
		err := s.resourceRepository.Db.
			Where("resource_url = ? AND active = ?", candidate, 1).
			First(&entity).Error
		if err == nil {
			return &entity, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}
	return nil, nil
}

func (s *AuthService) isLimit(ip string, maxLoginErrorNum int64) bool {
	if strings.TrimSpace(ip) == "" || maxLoginErrorNum <= 0 {
		return false
	}
	value := redisMiddleware.Get(buildIPKey(ip))
	errorNum, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return false
	}
	return errorNum > maxLoginErrorNum
}

func (s *AuthService) calLoginError(ip string) {
	if strings.TrimSpace(ip) == "" {
		return
	}
	key := buildIPKey(ip)
	redisMiddleware.Incr(key)
	redisMiddleware.Expire(key, time.Hour)
}

func ensureRedisReady() error {
	if redisMiddleware.Rdb == nil {
		return fmt.Errorf("redis is not initialized")
	}
	return nil
}

func buildUserTokenKey(token string) string {
	return userTokenPrefix + "_" + strings.TrimSpace(token)
}

func buildUserRoleKey(userID uint64) string {
	return userRolePrefix + strconv.FormatUint(userID, 10)
}

func ClearUserRoleCache(userID uint64) {
	if redisMiddleware.Rdb == nil || userID == 0 {
		return
	}
	redisMiddleware.Del(buildUserRoleKey(userID))
}

func buildIPKey(ip string) string {
	return userIPLoginPrefix + strings.TrimSpace(ip)
}

func joinUint64List(values []uint64) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatUint(value, 10))
	}
	return strings.Join(parts, ",")
}

func parseUint64List(value string) ([]uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []uint64{}, nil
	}
	parts := strings.Split(value, ",")
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func buildResourceURLCandidates(resourceURL string) []string {
	resourceURL = normalizeResourceURL(resourceURL)
	if resourceURL == "" {
		return []string{}
	}
	candidates := []string{resourceURL}
	if strings.HasPrefix(resourceURL, "/api/") {
		candidates = append(candidates, strings.TrimPrefix(resourceURL, "/api"))
	}
	if strings.HasPrefix(resourceURL, "api/") {
		candidates = append(candidates, "/"+strings.TrimPrefix(resourceURL, "api/"))
	}
	return uniqueStrings(candidates)
}

func normalizeResourceURL(resourceURL string) string {
	resourceURL = strings.TrimSpace(resourceURL)
	if resourceURL == "" {
		return ""
	}
	if !strings.HasPrefix(resourceURL, "/") {
		return "/" + resourceURL
	}
	return resourceURL
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func encryptPassword(username, password string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(username) + "_" + strings.TrimSpace(password)))
	return hex.EncodeToString(sum[:])
}

func toLoginUser(user *userRepository.User) *LoginUser {
	return &LoginUser{
		ID:       uint64(user.Id),
		Name:     user.Name,
		Username: user.Username,
		Role:     user.Role,
		Status:   user.Status,
	}
}
