package services

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/data/vo"
	"backend/errors"
	"backend/utils"
	"gorm.io/gorm"
	"strings"
	"time"
)

type authService struct {
	db          *gorm.DB
	userService UserService
}

func NewAuthService(db *gorm.DB) AuthService {
	return &authService{
		db:          db,
		userService: NewUserService(db),
	}
}

// Register 注册
func (s *authService) Register(req *vo.RegisterRequest) (*user.User, error) {
	// 优化：合并查询检查用户名和邮箱
	exists, field, err := s.CheckUsernameOrEmailExists(req.Username, req.Email)
	if err != nil {
		// 数据库查询错误，系统错误
		return nil, errors.NewSystemError("DATABASE_ERROR", constants.UserCreateFailed, err)
	}
	if exists {
		// 业务错误：用户名或邮箱已存在
		if field == "username" {
			return nil, errors.NewBusinessError("USERNAME_EXISTS", constants.ErrUsernameExists)
		}
		return nil, errors.NewBusinessError("EMAIL_EXISTS", constants.UserEmailExists)
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		// 系统错误：密码哈希失败
		return nil, errors.NewSystemError("HASH_PASSWORD_ERROR", constants.UserCreateFailed, err)
	}

	newUser := &user.User{
		Username: req.Username,
		Password: hashedPassword,
		Nickname: req.Nickname,
		Email:    req.Email,
		Status:   1,
	}

	if err := s.userService.CreateUser(newUser); err != nil {
		// 检查是否是唯一性约束错误（数据库层面的约束）
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			if strings.Contains(err.Error(), "username") {
				return nil, errors.NewBusinessError("USERNAME_EXISTS", constants.ErrUsernameExists)
			}
			return nil, errors.NewBusinessError("EMAIL_EXISTS", constants.UserEmailExists)
		}
		// 其他数据库错误，系统错误
		return nil, errors.NewSystemError("CREATE_USER_ERROR", constants.UserCreateFailed, err)
	}

	newUser.Password = ""
	return newUser, nil
}

// Login 登录
func (s *authService) Login(username, password string) (*user.User, string, error) {
	u, err := s.userService.GetUserByUsername(username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 业务错误：用户不存在
			return nil, "", errors.NewBusinessError("USER_NOT_FOUND", constants.ErrUserNotFound)
		}
		// 系统错误：数据库查询错误
		return nil, "", errors.NewSystemError("DATABASE_ERROR", constants.ErrUserNotFound, err)
	}

	// 检查用户状态
	if u.Status == 0 {
		// 业务错误：用户被禁用
		return nil, "", errors.NewBusinessError("USER_DISABLED", constants.ErrUserDisabled)
	}

	if !utils.CheckPasswordHash(password, u.Password) {
		// 业务错误：密码错误
		return nil, "", errors.NewBusinessError("INVALID_CREDENTIALS", constants.ErrInvalidCredentials)
	}
	if u.Role != "admin" {
		return nil, "", errors.NewBusinessError("FORBIDDEN", "需要管理员权限")
	}
	return s.issueToken(u)
}

// LoginMember 会员端登录（member / agent，不含 admin）
func (s *authService) LoginMember(username, password string) (*user.User, string, error) {
	u, err := s.userService.GetUserByUsername(username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "", errors.NewBusinessError("USER_NOT_FOUND", constants.ErrUserNotFound)
		}
		return nil, "", errors.NewSystemError("DATABASE_ERROR", constants.ErrUserNotFound, err)
	}
	if u.Status == 0 {
		return nil, "", errors.NewBusinessError("USER_DISABLED", constants.ErrUserDisabled)
	}
	if !utils.CheckPasswordHash(password, u.Password) {
		return nil, "", errors.NewBusinessError("INVALID_CREDENTIALS", constants.ErrInvalidCredentials)
	}
	if u.Role == "admin" {
		return nil, "", errors.NewBusinessError("FORBIDDEN", "请使用管理后台登录")
	}
	return s.issueToken(u)
}

func (s *authService) issueToken(u *user.User) (*user.User, string, error) {
	now := time.Now().UTC()
	if err := s.db.Model(u).Updates(map[string]any{"last_login_at": now, "login_count": gorm.Expr("login_count + 1")}).Error; err == nil {
		u.LastLoginAt = &now
		u.LoginCount++
	}
	token, err := utils.GenerateToken(u.UserID, 24)
	if err != nil {
		return nil, "", errors.NewSystemError("GENERATE_TOKEN_ERROR", constants.ErrGenerateToken, err)
	}
	u.Password = ""
	return u, token, nil
}

func (s *authService) GetByID(id uint64) (*user.User, error) {
	var account user.User
	if err := s.db.First(&account, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.NewBusinessError("USER_NOT_FOUND", constants.ErrUserNotFound)
		}
		return nil, errors.NewSystemError("DATABASE_ERROR", constants.ErrUserNotFound, err)
	}
	if account.Status == 0 {
		return nil, errors.NewBusinessError("USER_DISABLED", constants.ErrUserDisabled)
	}
	account.Password = ""
	return &account, nil
}

// CheckUsernameOrEmailExists 优化：一次查询检查用户名或邮箱是否存在
// 返回: (是否存在, 冲突字段, 错误)
func (s *authService) CheckUsernameOrEmailExists(username, email string) (bool, string, error) {
	var count int64

	// 检查用户名
	result := s.db.Model(&user.User{}).Where("username = ?", username).Count(&count)
	if result.Error != nil {
		return false, "", result.Error
	}
	if count > 0 {
		return true, "username", nil
	}

	// 检查邮箱
	result = s.db.Model(&user.User{}).Where("email = ?", email).Count(&count)
	if result.Error != nil {
		return false, "", result.Error
	}
	if count > 0 {
		return true, "email", nil
	}

	return false, "", nil
}

// CheckEmailExists 检查邮箱是否存在（保留用于其他地方）
func (s *authService) CheckEmailExists(email string) (bool, error) {
	var count int64
	result := s.db.Model(&user.User{}).Where("email = ?", email).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// CheckUsernameExists 检查用户名是否存在（保留用于其他地方）
func (s *authService) CheckUsernameExists(username string) (bool, error) {
	var count int64
	result := s.db.Model(&user.User{}).Where("username = ?", username).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}
