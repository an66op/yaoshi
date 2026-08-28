package services

import (
	"backend/data/models/user"
	"gorm.io/gorm"
)

type userService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) UserService {
	return &userService{db: db}
}

// CreateUser 创建用户
func (s *userService) CreateUser(u *user.User) error {
	return s.db.Create(u).Error
}

// GetUserByUsername 根据用户名获取用户
func (s *userService) GetUserByUsername(username string) (*user.User, error) {
	var u user.User
	err := s.db.Where("login_scope = ? AND LOWER(username) = LOWER(?)", platformLoginScope, username).First(&u).Error
	if err == gorm.ErrRecordNotFound {
		// Recover databases written by an earlier workspace migration that used
		// the platform business scope ("lobby") as the administrator login scope.
		if recoveryErr := s.db.Where("role = ? AND LOWER(username) = LOWER(?)", "admin", username).First(&u).Error; recoveryErr != nil {
			return nil, recoveryErr
		}
		if updateErr := s.db.Model(&u).Update("login_scope", platformLoginScope).Error; updateErr != nil {
			return nil, updateErr
		}
		return &u, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
