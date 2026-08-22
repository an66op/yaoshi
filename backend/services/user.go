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
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
