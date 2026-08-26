package services

import (
	"backend/data/models/activity"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
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
	password := input.Password
	if len(username) < 3 {
		return nil, 0, apperrors.NewBusinessError("INVALID_USERNAME", "帐号至少 3 位")
	}
	if err := utils.ValidatePassword(password); err != nil {
		return nil, 0, apperrors.NewBusinessError("INVALID_PASSWORD", "密码长度需为 8–72 个字符")
	}
	loginScope := platformLoginScope
	var roomAgentID *uint64
	var roomTenantID *uint64
	if roomCode := strings.TrimSpace(input.RoomCode); roomCode != "" {
		resolved, err := NewSpecialAdminService(s.db).ResolveRoom(roomCode)
		if err != nil {
			return nil, 0, err
		}
		agentID := resolved.AgentID
		scope, tenantID, err := loginScopeForAgent(s.db, agentID)
		if err != nil {
			return nil, 0, err
		}
		loginScope, roomAgentID, roomTenantID = scope, &agentID, tenantID
	}
	if err := ensureUsernameInScope(s.db, loginScope, username, 0); err != nil {
		return nil, 0, err
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return nil, 0, apperrors.NewSystemError("HASH_PASSWORD_ERROR", "创建帐号失败", err)
	}
	nickname := defaultString(strings.TrimSpace(input.Nickname), username)
	inviteCode := strings.TrimSpace(input.InviteCode)
	if inviteCode != "" && parseInviteCode(inviteCode) == 0 {
		return nil, 0, apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码至少 4 位且格式正确")
	}
	account := user.User{
		Username: username, LoginScope: loginScope, Password: hash, Nickname: nickname,
		Role: "member", Status: 1, ParentAgentID: roomAgentID, ParentTenantID: roomTenantID,
	}
	inviterID := parseInviteCode(inviteCode)
	if inviterID > 0 {
		account.Remark = fmt.Sprintf("invited_by:%d", inviterID)
	}
	var inviteReward int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&account).Error; err != nil {
			if isDuplicateParticipation(err) {
				return apperrors.NewBusinessError("USERNAME_EXISTS", "帐号已被使用")
			}
			return apperrors.NewSystemError("CREATE_USER_ERROR", "创建帐号失败", err)
		}
		if inviterID > 0 {
			reward, err := s.bindInviterTx(tx, &account, inviterID)
			if err != nil {
				return err
			}
			inviteReward = reward
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	account.Password = ""
	return &account, inviteReward, nil
}

func (s *MemberService) bindInviterTx(tx *gorm.DB, account *user.User, inviterID uint64) (int64, error) {
	var inviter user.User
	if err := tx.First(&inviter, inviterID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码无效")
		}
		return 0, err
	}
	if inviter.Status != 1 || inviter.Role == "admin" || inviter.Role == "tenant" {
		return 0, apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码无效")
	}
	updates := map[string]any{}
	if account.ParentAgentID == nil && inviter.Role == "agent" && inviter.AgentRoomCode != "" {
		updates["parent_agent_id"] = inviter.UserID
		updates["parent_tenant_id"] = inviter.ParentTenantID
		updates["login_scope"] = agentLoginScope(inviter.UserID)
	} else if account.ParentAgentID == nil && inviter.ParentAgentID != nil {
		updates["parent_agent_id"] = *inviter.ParentAgentID
		scope, tenantID, err := loginScopeForAgent(tx, *inviter.ParentAgentID)
		if err != nil {
			return 0, err
		}
		updates["parent_tenant_id"] = tenantID
		updates["login_scope"] = scope
	}
	if len(updates) > 0 {
		scope, _ := updates["login_scope"].(string)
		if err := ensureUsernameInScope(tx, scope, account.Username, account.UserID); err != nil {
			return 0, err
		}
		if err := tx.Model(account).Updates(updates).Error; err != nil {
			return 0, err
		}
		if parent, ok := updates["parent_agent_id"].(uint64); ok {
			account.ParentAgentID = &parent
		}
	}
	reward, err := s.grantInviteRewardsTx(tx, inviterID, account.UserID, account.Username)
	if err != nil {
		return 0, err
	}
	if err := tx.Create(&membernotify.MemberNotification{
		UserID:  inviterID,
		Title:   "邀请成功",
		Content: fmt.Sprintf("用户 %s 通过您的邀请码完成注册。", account.Username),
		Level:   "success", Category: "account",
	}).Error; err != nil {
		return 0, err
	}
	return reward, nil
}

func (s *MemberService) grantInviteRewardsTx(tx *gorm.DB, inviterID, inviteeID uint64, inviteeUsername string) (int64, error) {
	var row activity.Activity
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("type = ? AND status = ?", "invite", "active").
		Order("sort_order asc").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	rewardCents := row.RewardCents
	if rewardCents <= 0 {
		rewardCents = 500
	}
	now := time.Now().UTC()
	credits := []struct {
		userID    uint64
		action    string
		reference string
		remark    string
		title     string
		body      string
	}{
		{
			userID: inviteeID, action: "invite_bonus", reference: strconv.FormatUint(inviterID, 10),
			remark: "邀请注册奖励 · " + row.Title,
			title:  "注册奖励到账", body: fmt.Sprintf("欢迎加入！邀请活动奖励 %s 元已入账。", formatAmount(rewardCents)),
		},
		{
			userID: inviterID, action: "invite_referral", reference: strconv.FormatUint(inviteeID, 10),
			remark: fmt.Sprintf("邀请奖励 · 好友 %s 注册", inviteeUsername),
			title:  "邀请奖励到账", body: fmt.Sprintf("好友 %s 完成注册，奖励 %s 元已入账。", inviteeUsername, formatAmount(rewardCents)),
		},
	}
	for _, item := range credits {
		participation := activity.Participation{
			UserID: item.userID, ActivityID: row.ID, Action: item.action,
			BizDate: bizDateCST(now), Reference: item.reference,
			RewardCents: rewardCents, ParticipatedAt: now,
		}
		if err := tx.Create(&participation).Error; err != nil {
			if isDuplicateParticipation(err) {
				return 0, apperrors.NewBusinessError("INVITE_ALREADY_REWARDED", "该邀请关系已发放奖励")
			}
			return 0, err
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, item.userID).Error; err != nil {
			return 0, err
		}
		after := account.BalanceCents + rewardCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return 0, err
		}
		if err := tx.Create(&user.BalanceTransaction{
			UserID: item.userID, Reference: "activity_participation:" + strconv.FormatUint(participation.ID, 10),
			AmountCents: rewardCents, BeforeCents: account.BalanceCents, AfterCents: after,
			Type: "invite", Remark: item.remark, Operator: "系统",
		}).Error; err != nil {
			return 0, err
		}
		if err := tx.Create(&membernotify.MemberNotification{
			UserID: item.userID, Title: item.title, Content: item.body,
			Level: "success", Category: "account",
		}).Error; err != nil {
			return 0, err
		}
	}
	if err := tx.Model(&row).Update("participants", gorm.Expr("participants + 2")).Error; err != nil {
		return 0, err
	}
	return rewardCents, nil
}

func parseInviteCode(code string) uint64 {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0
	}
	code = strings.TrimPrefix(strings.ToUpper(code), "U")
	if len(code) < 4 {
		return 0
	}
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
	loggedIn, token, err := auth.LoginMember(account.Username, input.Password, account.LoginScope)
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
