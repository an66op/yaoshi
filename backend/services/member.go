package services

import (
	"backend/data/models/application"
	"backend/data/models/chat"
	"backend/data/models/settings"
	"backend/data/models/user"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/utils"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if account.Role == "admin" || account.Role == "tenant" {
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
		if err := s.db.Select("user_id", "username", "nickname", "agent_room_code", "agent_room_name").
			First(&agent, *account.ParentAgentID).Error; err == nil && agent.AgentRoomCode != "" {
			out.RoomCode = agent.AgentRoomCode
			out.RoomName = agentRoomDisplayName(agent)
		}
	}
	return out, nil
}

func (s *MemberService) ChangePassword(userID uint64, oldPassword, newPassword string) error {
	if err := utils.ValidatePassword(newPassword); err != nil {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "新密码长度需为 8–72 个字符")
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

	err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&user.User{}).Where("user_id = ? AND role <> ?", userID, "admin").Update("nickname", nickname)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		// Chat messages retain the sender identity for persistence. Keep that
		// snapshot synchronized so a renamed member is not shown under two
		// different names in old and new messages.
		return tx.Model(&chat.Message{}).Where("user_id = ?", userID).Update("nickname", nickname).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("NICKNAME_UPDATE_FAILED", "昵称更新失败", err)
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
	if account.Role == "admin" || account.Role == "tenant" {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "管理员不能进入代理房间")
	}
	agentID := resolved.AgentID
	if account.ParentAgentID != nil && *account.ParentAgentID == agentID {
		resolved.Status = "joined"
		return resolved, nil
	}
	requireReview := true
	var config settings.SystemConfig
	if err := s.db.Order("id asc").First(&config).Error; err == nil {
		requireReview = config.RequireJoinReview
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("ROOM_SETTINGS_FAILED", "读取入房规则失败", err)
	}
	if !requireReview {
		loginScope := agentLoginScope(agentID)
		if err := ensureUsernameInScope(s.db, loginScope, account.Username, account.UserID); err != nil {
			return nil, err
		}
		if err := s.db.Model(&account).Updates(map[string]any{"parent_agent_id": agentID, "parent_tenant_id": resolvedTenantID(s.db, agentID), "login_scope": loginScope}).Error; err != nil {
			return nil, apperrors.NewSystemError("ROOM_JOIN_FAILED", "进入房间失败", err)
		}
		resolved.Status = "joined"
		return resolved, nil
	}
	targetScope := fmt.Sprintf("agent:%d", agentID)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var locked user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, userID).Error; err != nil {
			return err
		}
		var pending application.Application
		lookup := tx.Where("user_id = ? AND request_type = ? AND room_scope = ? AND status = ?", userID, "join", targetScope, "pending").Order("id desc").First(&pending)
		if lookup.Error == nil {
			resolved.ApplicationID = pending.ID
			return nil
		}
		if lookup.Error != gorm.ErrRecordNotFound {
			return lookup.Error
		}
		pending = application.Application{
			UserID: locked.UserID, Username: locked.Username, AccountType: defaultString(locked.Role, "member"),
			RequestType: "join", PaymentType: "manual", RoomScope: targetScope,
			TargetRoomCode: resolved.RoomCode, Remark: "申请进入房间 " + resolved.RoomCode, Status: "pending",
		}
		if err := tx.Create(&pending).Error; err != nil {
			return err
		}
		resolved.ApplicationID = pending.ID
		return nil
	})
	if err != nil {
		return nil, apperrors.NewSystemError("ROOM_REVIEW_CREATE_FAILED", "提交入房申请失败", err)
	}
	resolved.Status = "pending"
	return resolved, nil
}

func resolvedTenantID(db *gorm.DB, agentID uint64) *uint64 {
	var agent user.User
	if err := db.Select("parent_tenant_id").First(&agent, agentID).Error; err != nil {
		return nil
	}
	return agent.ParentTenantID
}
