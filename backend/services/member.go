package services

import (
	"backend/data/models/application"
	"backend/data/models/chat"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/utils"
	"backend/ws"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberService struct {
	db      *gorm.DB
	special *SpecialAdminService
}

// MemberRoomHistoryItem is intentionally limited to public room metadata.
// The member endpoint builds this list from the authenticated user's own
// membership rows, so workspace IDs and other tenants' rooms are never
// exposed to the browser.
type MemberRoomHistoryItem struct {
	RoomCode      string    `json:"room_code"`
	RoomName      string    `json:"room_name"`
	RoomLogo      string    `json:"room_logo,omitempty"`
	Status        string    `json:"status"`
	Current       bool      `json:"current"`
	LastEnteredAt time.Time `json:"last_entered_at"`
}

func NewMemberService(db *gorm.DB) *MemberService {
	return &MemberService{db: db, special: NewSpecialAdminService(db)}
}

// RoomHistory returns rooms previously entered by one authenticated member.
// Membership rows are retained when a member switches rooms; only their
// active flag changes. This makes them the authoritative, cross-device room
// history without relying on localStorage or accepting a caller-supplied user
// or workspace ID.
func (s *MemberService) RoomHistory(userID uint64, limit int) ([]MemberRoomHistoryItem, error) {
	if limit < 1 || limit > 10 {
		limit = 8
	}
	var account user.User
	if err := s.db.Select("user_id", "workspace_id", "status").First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("DATABASE_ERROR", "读取房间记录失败", err)
	}
	if account.Status != 1 {
		return nil, apperrors.NewBusinessError("USER_DISABLED", "账号已被禁用")
	}

	type historyRow struct {
		WorkspaceID    uint64
		WorkspaceState int
		RoomCode       string
		RoomName       string
		RoomLogo       string
		LastEnteredAt  time.Time
	}
	var rows []historyRow
	err := s.db.Table("workspace_memberships AS membership").
		Select("room.id AS workspace_id, room.status AS workspace_state, room.room_code, room.name AS room_name, room.logo AS room_logo, membership.updated_at AS last_entered_at").
		Joins("JOIN workspaces AS room ON room.id = membership.workspace_id").
		Where("membership.user_id = ?", userID).
		Where("room.type IN ? AND room.room_code <> ''", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).
		Order(gorm.Expr("CASE WHEN room.id = ? THEN 0 ELSE 1 END", account.WorkspaceID)).
		Order("membership.updated_at DESC, membership.id DESC").
		Limit(limit * 2).
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.NewSystemError("ROOM_HISTORY_FAILED", "读取房间记录失败", err)
	}

	type candidate struct {
		workspaceID uint64
		item        MemberRoomHistoryItem
	}
	candidates := make([]candidate, 0, len(rows)+2)
	indexByWorkspace := make(map[uint64]int, len(rows)+2)
	for _, row := range rows {
		name := strings.TrimSpace(row.RoomName)
		if name == "" {
			name = row.RoomCode
		}
		status := "available"
		if row.WorkspaceState != 1 {
			status = "disabled"
		}
		if row.WorkspaceID == account.WorkspaceID {
			status = "current"
		}
		indexByWorkspace[row.WorkspaceID] = len(candidates)
		candidates = append(candidates, candidate{workspaceID: row.WorkspaceID, item: MemberRoomHistoryItem{
			RoomCode: row.RoomCode, RoomName: name, RoomLogo: row.RoomLogo,
			Status: status, Current: status == "current", LastEnteredAt: row.LastEnteredAt,
		}})
	}

	// Pending entry requests are useful context in the switcher, but they are
	// discovered only through this member's own applications. Selecting them
	// still calls JoinRoom and therefore never bypasses room review.
	var pendingRows []historyRow
	if err := s.db.Table("user_applications AS application").
		Select("room.id AS workspace_id, room.status AS workspace_state, room.room_code, room.name AS room_name, room.logo AS room_logo, application.created_at AS last_entered_at").
		Joins("JOIN workspaces AS room ON room.id = application.workspace_id").
		Where("application.user_id = ? AND application.request_type = ? AND application.status = ?", userID, "join", "pending").
		Where("room.type IN ? AND room.room_code <> ''", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).
		Order("application.created_at DESC, application.id DESC").
		Limit(limit).
		Scan(&pendingRows).Error; err != nil {
		return nil, apperrors.NewSystemError("ROOM_HISTORY_FAILED", "读取房间记录失败", err)
	}
	for _, row := range pendingRows {
		status := "pending"
		if row.WorkspaceState != 1 {
			status = "disabled"
		}
		if row.WorkspaceID == account.WorkspaceID {
			status = "current"
		}
		if index, exists := indexByWorkspace[row.WorkspaceID]; exists {
			if status != "current" {
				candidates[index].item.Status = status
				candidates[index].item.Current = false
			}
			if row.LastEnteredAt.After(candidates[index].item.LastEnteredAt) {
				candidates[index].item.LastEnteredAt = row.LastEnteredAt
			}
			continue
		}
		name := strings.TrimSpace(row.RoomName)
		if name == "" {
			name = row.RoomCode
		}
		indexByWorkspace[row.WorkspaceID] = len(candidates)
		candidates = append(candidates, candidate{workspaceID: row.WorkspaceID, item: MemberRoomHistoryItem{
			RoomCode: row.RoomCode, RoomName: name, RoomLogo: row.RoomLogo,
			Status: status, Current: status == "current", LastEnteredAt: row.LastEnteredAt,
		}})
	}

	// Legacy accounts may predate workspace_memberships. Keep the verified
	// current room visible while the migration/backfill catches up.
	if account.WorkspaceID > 0 {
		if _, exists := indexByWorkspace[account.WorkspaceID]; !exists {
			var room workspacemodel.Workspace
			if roomErr := s.db.Select("id", "room_code", "name", "logo", "status", "updated_at").
				Where("id = ? AND type IN ? AND room_code <> ''", account.WorkspaceID, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).
				First(&room).Error; roomErr == nil {
				name := strings.TrimSpace(room.Name)
				if name == "" {
					name = room.RoomCode
				}
				candidates = append(candidates, candidate{workspaceID: room.ID, item: MemberRoomHistoryItem{
					RoomCode: room.RoomCode, RoomName: name, RoomLogo: room.Logo,
					Status: "current", Current: true, LastEnteredAt: room.UpdatedAt,
				}})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].item.Current != candidates[j].item.Current {
			return candidates[i].item.Current
		}
		return candidates[i].item.LastEnteredAt.After(candidates[j].item.LastEnteredAt)
	})
	items := make([]MemberRoomHistoryItem, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		items = append(items, candidate.item)
		if len(items) == limit {
			break
		}
	}
	return items, nil
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
			PublicID: account.PublicID, Nickname: account.Nickname, Avatar: account.Avatar,
			Title: account.PublicTitle, Badge: account.PublicBadge, Role: account.Role, Status: account.Status,
		},
		Balance:       centsToAmount(account.BalanceCents),
		ParentAgentID: account.ParentAgentID,
	}
	if account.WorkspaceID > 0 {
		var workspace workspacemodel.Workspace
		if err := s.db.Select("room_code", "name", "logo", "type").First(&workspace, account.WorkspaceID).Error; err == nil && workspace.Type != workspacemodel.TypePlatform {
			out.RoomCode = workspace.RoomCode
			out.RoomName = workspace.Name
			out.RoomLogo = workspace.Logo
		}
	}
	return out, nil
}

// UpdateAvatar persists the member-facing avatar used by profile and chat
// responses. Only bundled avatar paths and compact image data URLs are
// accepted; arbitrary remote URLs and traversal paths are rejected.
func (s *MemberService) UpdateAvatar(userID uint64, avatar string) (*vo.MemberProfileResponse, error) {
	avatar, err := normalizeMemberAvatar(avatar)
	if err != nil {
		return nil, err
	}
	result := s.db.Model(&user.User{}).
		Where("user_id = ? AND role NOT IN ?", userID, []string{"admin", "tenant"}).
		Update("avatar", avatar)
	if result.Error != nil {
		return nil, apperrors.NewSystemError("AVATAR_UPDATE_FAILED", "头像更新失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return s.Profile(userID)
}

func normalizeMemberAvatar(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "/images/avatars/") {
		name := strings.TrimPrefix(value, "/images/avatars/")
		lower := strings.ToLower(name)
		if name == "" || len(value) > 300 || strings.ContainsAny(name, "/\\?#") || strings.Contains(name, "..") ||
			(!strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") && !strings.HasSuffix(lower, ".webp")) {
			return "", apperrors.NewBusinessError("INVALID_AVATAR", "头像路径不正确")
		}
		return value, nil
	}
	if len(value) > 500000 {
		return "", apperrors.NewBusinessError("INVALID_AVATAR", "头像文件过大")
	}
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/webp;base64,"} {
		if strings.HasPrefix(value, prefix) {
			return value, nil
		}
	}
	return "", apperrors.NewBusinessError("INVALID_AVATAR", "头像仅支持内置图片、PNG、JPG 或 WebP")
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
	if err := s.db.Model(&account).Updates(passwordSessionUpdate(hash)).Error; err != nil {
		return apperrors.NewSystemError("PASSWORD_UPDATE_FAILED", "密码更新失败", err)
	}
	ws.DisconnectUser(userID)
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

func (s *MemberService) JoinRoom(userID uint64, roomCode, requestID string) (*RoomResolveResult, error) {
	resolved, err := s.special.ResolveRoom(roomCode)
	if err != nil {
		return nil, err
	}
	cfg, err := NewSettingsAdminService(s.db).GetForWorkspace(resolved.WorkspaceID)
	if err != nil {
		return nil, apperrors.NewSystemError("ROOM_SETTINGS_FAILED", "读取房间设置失败", err)
	}
	if !cfg.RoomEnabled {
		return nil, apperrors.NewBusinessError("ROOM_CLOSED", "房间暂未开放")
	}
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("DATABASE_ERROR", "读取用户失败", err)
	}
	if !roomEntryApplicantRoleAllowed(account.Role) {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "只有普通会员可以申请进入房间")
	}
	if account.WorkspaceID == resolved.WorkspaceID {
		resolved.Status = "joined"
		return resolved, nil
	}
	targetScope := resolved.RoomScope
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = fmt.Sprintf("join:%d:%d:%d", userID, resolved.WorkspaceID, time.Now().UTC().UnixNano())
	}
	directJoinAttempted := false
	autoClosedApplicationIDs := make([]uint64, 0, 1)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// ResolveRoom validates the public room and its owner before this
		// transaction. Lock the target workspace again so a concurrent room
		// shutdown cannot be bypassed while an old membership is reactivated.
		var target workspacemodel.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ? AND status = ? AND type IN ?", resolved.WorkspaceID, 1, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).
			First(&target).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("ROOM_NOT_FOUND", "目标房间已停用或不存在")
			}
			return err
		}
		var locked user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, userID).Error; err != nil {
			return err
		}
		if locked.WorkspaceID == target.ID {
			resolved.Status = "joined"
			return nil
		}

		// Membership history is the server-side authority for rooms this user
		// has already entered. Status=0 only means another room is currently
		// active; it must not force a second join review. The room-scoped odds
		// multiplier is intentionally preserved by ActivateWorkspaceMembership.
		var membership workspacemodel.Membership
		membershipLookup := historicalWorkspaceMembershipQuery(tx.Clauses(clause.Locking{Strength: "UPDATE"}), target.ID, userID).Take(&membership)
		hasHistory := membershipLookup.Error == nil
		if membershipLookup.Error != nil && membershipLookup.Error != gorm.ErrRecordNotFound {
			return membershipLookup.Error
		}
		if !roomEntryReviewRequired(cfg.RequireJoinReview, hasHistory) {
			directJoinAttempted = true
			if err := ActivateWorkspaceMembership(tx, &locked, target); err != nil {
				return err
			}
			resolved.Status = "joined"
			if hasHistory {
				// Older versions could leave a pending request behind when a former
				// member attempted to return. Close it atomically so a later review
				// cannot unexpectedly move the member back into this room.
				var pendingIDs []uint64
				if err := pendingHistoricalJoinApplicationsQuery(tx, target.ID, userID).Pluck("id", &pendingIDs).Error; err != nil {
					return err
				}
				now := time.Now().UTC()
				if err := pendingHistoricalJoinApplicationsQuery(tx, target.ID, userID).
					Updates(map[string]any{
						"status": "rejected", "operator": "系统", "review_remark": "已是历史成员，本次直接返回房间，无需重复审核",
						"reviewed_at": now,
					}).Error; err != nil {
					return err
				}
				autoClosedApplicationIDs = append(autoClosedApplicationIDs, pendingIDs...)
			}
			return nil
		}

		var pending application.Application
		lookup := pendingHistoricalJoinApplicationsQuery(tx, target.ID, userID).Order("created_at ASC, id ASC").First(&pending)
		if lookup.Error == nil {
			resolved.ApplicationID = pending.ID
			return nil
		}
		if lookup.Error != gorm.ErrRecordNotFound {
			return lookup.Error
		}
		pending = application.Application{
			RequestID:   requestID,
			WorkspaceID: resolved.WorkspaceID,
			UserID:      locked.UserID, Username: locked.Username, AccountType: defaultString(locked.Role, "member"),
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
		if apperrors.IsBusinessError(err) {
			return nil, err
		}
		if directJoinAttempted {
			return nil, apperrors.NewSystemError("ROOM_JOIN_FAILED", "进入房间失败", err)
		}
		if isDuplicateParticipation(err) {
			var pending application.Application
			if lookupErr := s.db.Where("user_id = ? AND workspace_id = ? AND request_type = ? AND status = ?", userID, resolved.WorkspaceID, "join", "pending").Order("created_at ASC, id ASC").First(&pending).Error; lookupErr == nil {
				resolved.ApplicationID = pending.ID
				resolved.Status = "pending"
				return resolved, nil
			}
		}
		return nil, apperrors.NewSystemError("ROOM_REVIEW_CREATE_FAILED", "提交入房申请失败", err)
	}
	if resolved.Status == "joined" {
		for _, applicationID := range autoClosedApplicationIDs {
			notifyApplicationEvent(s.db, resolved.WorkspaceID, applicationID, "rejected", "join")
		}
		ws.DisconnectUser(userID)
		return resolved, nil
	}
	resolved.Status = "pending"
	notifyApplicationEvent(s.db, resolved.WorkspaceID, resolved.ApplicationID, "pending", "join")
	return resolved, nil
}

// Room changes move the account's active workspace. Management accounts must
// therefore never enter this flow: moving an agent or tenant would also move
// the authority boundary used by their workbench.
func roomEntryApplicantRoleAllowed(role string) bool {
	return strings.TrimSpace(role) == "member"
}

// historicalWorkspaceMembershipQuery intentionally has no status predicate:
// inactive rows are the durable proof that the member previously entered this
// exact workspace. Keeping the boundary in one helper also prevents callers
// from accidentally using a membership from another room.
func historicalWorkspaceMembershipQuery(db *gorm.DB, workspaceID, userID uint64) *gorm.DB {
	return db.Model(&workspacemodel.Membership{}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID)
}

func pendingHistoricalJoinApplicationsQuery(db *gorm.DB, workspaceID, userID uint64) *gorm.DB {
	return db.Model(&application.Application{}).
		Where("workspace_id = ? AND user_id = ? AND request_type = ? AND status = ?", workspaceID, userID, "join", "pending")
}

func roomEntryReviewRequired(requireJoinReview, hasHistoricalMembership bool) bool {
	return requireJoinReview && !hasHistoricalMembership
}
