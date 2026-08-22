package services

import (
	"backend/data/models/user"
	"backend/data/vo"
)

// AuthService 认证服务接口
type AuthService interface {
	Register(req *vo.RegisterRequest) (*user.User, error)
	Login(username, password string) (*user.User, string, error)
	LoginMember(username, password string) (*user.User, string, error)
	GetByID(id uint64) (*user.User, error)
}

// UserService 用户服务接口
type UserService interface {
	CreateUser(u *user.User) error
	GetUserByUsername(username string) (*user.User, error)
}
