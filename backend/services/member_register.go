package services

import (
	"backend/data/models/activity"
	"backend/data/models/user"
	membernotify "backend/data/models/notify"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/utils"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberRegisterInput struct {
	Username   string
	Password   string
	Nickname   string
	InviteCode string
	RoomCode   string
}

func (s *MemberService) Register(input MemberRegisterInput) (*user.User, int64, error) {
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)
	if len(username) < 3 {
		return nil, 0, apperrors.NewBusinessError("INVALID_USERNAME", "帐号至少 3 位")
	}
	if len(password) < 6 {
		return nil, 0, apperrors.NewBusinessError("INVALID_PASSWORD", "密码至少 6 位")
	}
	var exists int64
	if err := s.db.Model(&user.User{}).Where("username = ?", username).Count(&exists).Error; err != nil {
		return nil, 0, err
	}
	if exists > 0 {
		return nil, 0, apperrors.NewBusinessError("USERNAME_EXISTS", "帐号已被使用")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return nil, 0, apperrors.NewSystemError("HASH_PASSWORD_ERROR", "创建帐号失败", err)
	}
	nickname := defaultString(strings.TrimSpace(input.Nickname), username)
	account := user.User{
		Username: username, Password: hash, Nickname: nickname,
		Role: "member", Status: 1,
	}
	inviterID := parseInviteCode(input.InviteCode)
	if inviterID > 0 {
		account.Remark = fmt.Sprintf("invited_by:%d", inviterID)
	}
	if err := s.db.Create(&account).Error; err != nil {
		return nil, 0, apperrors.NewSystemError("CREATE_USER_ERROR", "创建帐号失败", err)
	}
	var inviteReward int64
	if inviterID > 0 && inviterID != account.UserID {
		inviteReward = s.bindInviter(&account, inviterID)
	}
	if roomCode := strings.TrimSpace(input.RoomCode); roomCode != "" {
		if _, err := s.JoinRoom(account.UserID, roomCode); err != nil {
			return nil, 0, err
		}
	}
	account.Password = ""
	return &account, inviteReward, nil
}

func (s *MemberService) bindInviter(account *user.User, inviterID uint64) int64 {
	var inviter user.User
	if err := s.db.First(&inviter, inviterID).Error; err != nil {
		return 0
	}
	updates := map[string]any{}
	if inviter.Role == "agent" && inviter.AgentRoomCode != "" {
		updates["parent_agent_id"] = inviter.UserID
	} else if inviter.ParentAgentID != nil {
		updates["parent_agent_id"] = *inviter.ParentAgentID
	}
	if len(updates) > 0 {
		_ = s.db.Model(account).Updates(updates).Error
	}
	reward := s.grantInviteRewards(inviterID, account.UserID, account.Username)
	_ = s.db.Create(&membernotify.MemberNotification{
		UserID: inviterID,
		Title:  "邀请成功",
		Content: fmt.Sprintf("用户 %s 通过您的邀请码完成注册。", account.Username),
		Level: "success", Category: "activity",
	}).Error
	return reward
}

func (s *MemberService) grantInviteRewards(inviterID, inviteeID uint64, inviteeUsername string) int64 {
	var row activity.Activity
	if err := s.db.Where("type = ? AND status = ?", "invite", "active").Order("sort_order asc").First(&row).Error; err != nil {
		return 0
	}
	rewardCents := row.RewardCents
	if rewardCents <= 0 {
		rewardCents = 500
	}
	now := time.Now().UTC()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		credits := []struct {
			userID uint64
			action string
			remark string
			title  string
			body   string
		}{
			{
				userID: inviteeID, action: "invite_bonus", remark: "邀请注册奖励 · " + row.Title,
				title: "注册奖励到账", body: fmt.Sprintf("欢迎加入！邀请活动奖励 %s 元已入账。", formatAmount(rewardCents)),
			},
			{
				userID: inviterID, action: "invite_referral", remark: fmt.Sprintf("邀请奖励 · 好友 %s 注册", inviteeUsername),
				title: "邀请奖励到账", body: fmt.Sprintf("好友 %s 完成注册，奖励 %s 元已入账。", inviteeUsername, formatAmount(rewardCents)),
			},
		}
		for _, item := range credits {
			if err := tx.Create(&activity.Participation{
				UserID: item.userID, ActivityID: row.ID, Action: item.action,
				RewardCents: rewardCents, ParticipatedAt: now,
			}).Error; err != nil {
				return err
			}
			var account user.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, item.userID).Error; err != nil {
				return err
			}
			after := account.BalanceCents + rewardCents
			if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
				return err
			}
			if err := tx.Create(&user.BalanceTransaction{
				UserID: item.userID, AmountCents: rewardCents, BeforeCents: account.BalanceCents, AfterCents: after,
				Type: "invite", Remark: item.remark, Operator: "系统",
			}).Error; err != nil {
				return err
			}
			_ = tx.Create(&membernotify.MemberNotification{
				UserID: item.userID, Title: item.title, Content: item.body,
				Level: "success", Category: "activity",
			}).Error
		}
		return tx.Model(&row).Update("participants", gorm.Expr("participants + 2")).Error
	})
	if err != nil {
		return 0
	}
	return rewardCents
}

func parseInviteCode(code string) uint64 {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0
	}
	code = strings.TrimPrefix(strings.ToUpper(code), "U")
	id, err := strconv.ParseUint(code, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func (s *MemberService) RegisterWithToken(input MemberRegisterInput) (*vo.LoginResponse, error) {
	account, inviteReward, err := s.Register(input)
	if err != nil {
		return nil, err
	}
	auth := NewAuthService(s.db)
	loggedIn, token, err := auth.LoginMember(account.Username, input.Password)
	if err != nil {
		return nil, err
	}
	resp := &vo.LoginResponse{
		Token: token,
		User: vo.UserResponse{
			ID: loggedIn.UserID, Username: loggedIn.Username, Email: loggedIn.Email,
			Nickname: loggedIn.Nickname, Role: loggedIn.Role, Status: loggedIn.Status,
		},
	}
	if inviteReward > 0 {
		resp.Message = fmt.Sprintf("注册成功，邀请奖励 %.2f 元已到账", centsToAmount(inviteReward))
	}
	return resp, nil
}
