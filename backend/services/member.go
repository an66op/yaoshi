package services

import (
	"backend/data/models/user"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/utils"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

type MemberService struct {
	db      *gorm.DB
	special *SpecialAdminService
}

func NewMemberService(db *gorm.DB) *MemberService {
	return &MemberService{db: db, special: NewSpecialAdminService(db)}
}

func (s *MemberService) Profile(userID uint64) (*vo.MemberProfileResponse, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("DATABASE_ERROR", "读取用户失败", err)
	}
	if account.Status != 1 {
		return nil, apperrors.NewBusinessError("USER_DISABLED", "账号已被禁用")
	}
	if account.Role == "admin" {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "请使用管理后台")
	}
	out := &vo.MemberProfileResponse{
		UserResponse: vo.UserResponse{
			ID: account.UserID, Username: account.Username, Email: account.Email,
			PublicID: account.PublicID, Nickname: account.Nickname, Role: account.Role, Status: account.Status,
		},
		Balance:       centsToAmount(account.BalanceCents),
		ParentAgentID: account.ParentAgentID,
	}
	if account.ParentAgentID != nil {
		var agent user.User
		if err := s.db.Select("user_id", "username", "nickname", "agent_room_code").
			First(&agent, *account.ParentAgentID).Error; err == nil && agent.AgentRoomCode != "" {
			out.RoomCode = agent.AgentRoomCode
			out.RoomName = defaultString(agent.Nickname, agent.Username) + "的房间"
		}
	}
	return out, nil
}

func (s *MemberService) ChangePassword(userID uint64, oldPassword, newPassword string) error {
	oldPassword = strings.TrimSpace(oldPassword)
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 6 {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "新密码至少 6 位")
	}
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return apperrors.NewSystemError("DATABASE_ERROR", "读取用户失败", err)
	}
	if !utils.CheckPasswordHash(oldPassword, account.Password) {
		return apperrors.NewBusinessError("INVALID_CREDENTIALS", "原密码不正确")
	}
	hash, err := utils.HashPassword(newPassword)
	if err != nil {
		return apperrors.NewSystemError("HASH_PASSWORD_ERROR", "密码更新失败", err)
	}
	if err := s.db.Model(&account).Update("password", hash).Error; err != nil {
		return apperrors.NewSystemError("PASSWORD_UPDATE_FAILED", "密码更新失败", err)
	}
	return nil
}

// UpdateNickname persists the member's public in-room name. Keeping this in
// the member service ensures reconnecting or refreshing never restores an old
// browser-only nickname.
func (s *MemberService) UpdateNickname(userID uint64, nickname string) (*vo.MemberProfileResponse, error) {
	nickname = strings.Join(strings.Fields(nickname), " ")
	if length := utf8.RuneCountInString(nickname); length < 2 || length > 16 {
		return nil, apperrors.NewBusinessError("INVALID_NICKNAME", "昵称需为 2–16 个字符")
	}
	if strings.Contains(nickname, "*") {
		return nil, apperrors.NewBusinessError("INVALID_NICKNAME", "昵称不能使用遮挡字符")
	}

	result := s.db.Model(&user.User{}).Where("user_id = ? AND role <> ?", userID, "admin").Update("nickname", nickname)
	if result.Error != nil {
		return nil, apperrors.NewSystemError("NICKNAME_UPDATE_FAILED", "昵称更新失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return s.Profile(userID)
}

func (s *MemberService) JoinRoom(userID uint64, roomCode string) (*RoomResolveResult, error) {
	resolved, err := s.special.ResolveRoom(roomCode)
	if err != nil {
		return nil, err
	}
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("DATABASE_ERROR", "读取用户失败", err)
	}
	if account.Role == "admin" {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "管理员不能进入代理房间")
	}
	agentID := resolved.AgentID
	if err := s.db.Model(&account).Update("parent_agent_id", agentID).Error; err != nil {
		return nil, apperrors.NewSystemError("ROOM_JOIN_FAILED", "进入房间失败", err)
	}
	return resolved, nil
}
