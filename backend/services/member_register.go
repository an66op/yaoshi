package services

import (
	"backend/data/models/activity"
	"backend/data/models/application"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/utils"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberRegisterInput struct {
	Username   string
	Password   string
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
	var targetWorkspaceID uint64
	var pendingRoom *RoomResolveResult
	if roomCode := strings.TrimSpace(input.RoomCode); roomCode != "" {
		resolved, err := NewSpecialAdminService(s.db).ResolveRoom(roomCode)
		if err != nil {
			return nil, 0, err
		}
		cfg, err := NewSettingsAdminService(s.db).GetForWorkspace(resolved.WorkspaceID)
		if err != nil {
			return nil, 0, apperrors.NewSystemError("ROOM_SETTINGS_FAILED", "读取入房规则失败", err)
		}
		if !cfg.RoomEnabled {
			return nil, 0, apperrors.NewBusinessError("ROOM_CLOSED", "房间暂未开放")
		}
		if cfg.RequireJoinReview {
			// A registration is not an approval. Keep the new account outside all
			// formal rooms and persist the requested target as an application.
			pendingRoom = resolved
		} else {
			targetWorkspaceID = resolved.WorkspaceID
			loginScope = resolved.RoomScope
		}
		if pendingRoom == nil && resolved.WorkspaceType == "agent" {
			agentID := resolved.AgentID
			roomAgentID = &agentID
			var agent user.User
			if err := s.db.Select("parent_tenant_id").First(&agent, agentID).Error; err == nil {
				roomTenantID = agent.ParentTenantID
			}
		} else if pendingRoom == nil && resolved.WorkspaceType == "tenant" {
			// Tenant direct rooms do not use a synthetic agent parent.
			var workspaceOwner struct{ OwnerUserID uint64 }
			if err := s.db.Table("workspaces").Select("owner_user_id").Where("id = ?", resolved.WorkspaceID).Scan(&workspaceOwner).Error; err == nil && workspaceOwner.OwnerUserID > 0 {
				roomTenantID = &workspaceOwner.OwnerUserID
			}
		}
	}
	if err := ensureUsernameInScope(s.db, loginScope, username, 0); err != nil {
		return nil, 0, err
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return nil, 0, apperrors.NewSystemError("HASH_PASSWORD_ERROR", "创建帐号失败", err)
	}
	// Public registration deliberately does not accept a display nickname.
	// A neutral nickname is generated on the server so removing the field from
	// one client cannot be bypassed by posting a custom value directly.
	nickname := randomMemberNickname()
	inviteCode := strings.TrimSpace(input.InviteCode)
	if inviteCode != "" && parseInviteCode(inviteCode) == 0 {
		return nil, 0, apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码至少 4 位且格式正确")
	}
	account := user.User{
		Username: username, LoginScope: loginScope, Password: hash, Nickname: nickname,
		Role: "member", Status: 1, ParentAgentID: roomAgentID, ParentTenantID: roomTenantID, WorkspaceID: targetWorkspaceID,
	}
	inviterID := parseInviteCode(inviteCode)
	if inviterID > 0 {
		account.Remark = fmt.Sprintf("invited_by:%d", inviterID)
	}
	var inviteReward int64
	var pendingApplicationID uint64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&account).Error; err != nil {
			if isDuplicateParticipation(err) {
				return apperrors.NewBusinessError("USERNAME_EXISTS", "帐号已被使用")
			}
			return apperrors.NewSystemError("CREATE_USER_ERROR", "创建帐号失败", err)
		}
		if inviterID > 0 && pendingRoom == nil {
			reward, err := s.bindInviterTx(tx, &account, inviterID)
			if err != nil {
				return err
			}
			inviteReward = reward
		}
		if inviterID > 0 && pendingRoom != nil {
			var inviter user.User
			if err := tx.First(&inviter, inviterID).Error; err != nil || inviter.Status != 1 || inviter.WorkspaceID != pendingRoom.WorkspaceID {
				return apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码不属于目标房间")
			}
		}
		if targetWorkspaceID > 0 {
			var target workspacemodel.Workspace
			if err := tx.First(&target, targetWorkspaceID).Error; err != nil {
				return err
			}
			if err := ActivateWorkspaceMembership(tx, &account, target); err != nil {
				return err
			}
		}
		if pendingRoom != nil {
			pending := application.Application{
				RequestID:   fmt.Sprintf("join:register:%d:%d:%d", account.UserID, pendingRoom.WorkspaceID, time.Now().UTC().UnixNano()),
				WorkspaceID: pendingRoom.WorkspaceID,
				UserID:      account.UserID, Username: account.Username, AccountType: "member",
				RequestType: "join", PaymentType: "manual", RoomScope: pendingRoom.RoomScope,
				TargetRoomCode: pendingRoom.RoomCode, Remark: "申请进入房间 " + pendingRoom.RoomCode, Status: "pending",
			}
			if err := tx.Create(&pending).Error; err != nil {
				return err
			}
			pendingApplicationID = pending.ID
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if pendingApplicationID > 0 && pendingRoom != nil {
		notifyApplicationEvent(s.db, pendingRoom.WorkspaceID, pendingApplicationID, "pending", "join")
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
	if account.WorkspaceID > 0 && inviter.WorkspaceID > 0 && account.WorkspaceID != inviter.WorkspaceID {
		return 0, apperrors.NewBusinessError("INVALID_INVITE_CODE", "邀请码不属于当前房间")
	}
	updates := map[string]any{}
	if account.WorkspaceID == 0 && inviter.WorkspaceID > 0 {
		updates["workspace_id"] = inviter.WorkspaceID
	}
	if account.ParentAgentID == nil && inviter.Role == "agent" && inviter.AgentRoomCode != "" {
		updates["parent_agent_id"] = inviter.UserID
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
		if workspaceID, ok := updates["workspace_id"].(uint64); ok {
			var target workspacemodel.Workspace
			if err := tx.First(&target, workspaceID).Error; err != nil {
				return 0, err
			}
			if err := ActivateWorkspaceMembership(tx, account, target); err != nil {
				return 0, err
			}
		}
	}
	reward, err := s.grantInviteRewardsTx(tx, account.WorkspaceID, inviterID, account.UserID, account.Username)
	if err != nil {
		return 0, err
	}
	if err := tx.Create(&membernotify.MemberNotification{
		WorkspaceID: account.WorkspaceID,
		UserID:      inviterID,
		Title:       "邀请成功",
		Content:     fmt.Sprintf("用户 %s 通过您的邀请码完成注册。", account.Username),
		Level:       "success", Category: "account",
	}).Error; err != nil {
		return 0, err
	}
	return reward, nil
}

func (s *MemberService) grantInviteRewardsTx(tx *gorm.DB, workspaceID, inviterID, inviteeID uint64, inviteeUsername string) (int64, error) {
	if workspaceID == 0 {
		return 0, nil
	}
	if err := NewActivityAdminService(tx).ensureDefaultsForWorkspace(workspaceID); err != nil {
		return 0, err
	}
	var row activity.Activity
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND type = ? AND status = ?", workspaceID, "invite", "active").
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
			WorkspaceID: workspaceID, UserID: item.userID, ActivityID: row.ID, Action: item.action,
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND workspace_id = ?", item.userID, workspaceID).First(&account).Error; err != nil {
			return 0, err
		}
		before := account.BalanceCents
		after := before + rewardCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return 0, err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: workspaceID, UserID: item.userID, Reference: "activity_participation:" + strconv.FormatUint(participation.ID, 10),
			AmountCents: rewardCents, BeforeCents: before, AfterCents: after,
			Type: "invite", Remark: item.remark, Operator: "系统",
		}).Error; err != nil {
			return 0, err
		}
		if err := tx.Create(&membernotify.MemberNotification{
			WorkspaceID: workspaceID, UserID: item.userID, Title: item.title, Content: item.body,
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

var memberNicknameAdjectives = []string{
	"安静的", "轻快的", "清醒的", "勇敢的", "自由的", "专注的", "温和的", "好奇的",
	"从容的", "乐观的", "耐心的", "灵巧的", "明亮的", "坚定的", "元气的", "幸运的",
}

var memberNicknameObjects = []string{
	"星云", "云朵", "月光", "晚风", "鲸落", "纸船", "青柠", "银杏", "山谷", "萤火",
	"白鹭", "海盐", "青山", "雾岛", "微光", "原野", "星河", "晴空", "风铃", "灯塔",
}

func randomMemberNickname() string {
	return memberNicknameAdjectives[secureRandomIndex(len(memberNicknameAdjectives))] +
		memberNicknameObjects[secureRandomIndex(len(memberNicknameObjects))]
}

func secureRandomIndex(length int) int {
	if length <= 1 {
		return 0
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(length))
	}
	return int(value.Int64())
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
			PublicID: loggedIn.PublicID, Nickname: loggedIn.Nickname, Avatar: loggedIn.Avatar,
			Title: loggedIn.PublicTitle, Badge: loggedIn.PublicBadge, Role: loggedIn.Role, Status: loggedIn.Status,
		},
	}
	if inviteReward > 0 {
		resp.Message = fmt.Sprintf("注册成功，邀请奖励 %.2f 元已到账", centsToAmount(inviteReward))
	} else if strings.TrimSpace(input.RoomCode) != "" && account.WorkspaceID == 0 {
		resp.Message = "注册成功，入房申请已提交，请等待审核"
	}
	return resp, nil
}
