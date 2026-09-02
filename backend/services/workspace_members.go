package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"database/sql"
	"strings"
	"time"

	"gorm.io/gorm"
)

// WorkspaceMember is a room roster entry, not an account-management record.
// A previous room may retain the public identity of a member who joined it,
// but must not receive that account's new room, balance or private settings.
type WorkspaceMember struct {
	ID            uint64 `json:"id"`
	PublicID      uint64 `json:"public_id"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar,omitempty"`
	PublicTitle   string `json:"public_title,omitempty"`
	PublicBadge   string `json:"badge,omitempty"`
	Role          string `json:"role"`
	InCurrentRoom bool   `json:"in_current_room"`
	CanManage     bool   `json:"can_manage" gorm:"-"`

	// Null means unavailable to this room, not a zero balance, disabled
	// account or offline member. Only current-room entries populate these.
	Balance     *float64   `json:"balance" gorm:"-"`
	Status      *int       `json:"status"`
	Online      *bool      `json:"online" gorm:"-"`
	Phone       string     `json:"phone,omitempty"`
	Remark      string     `json:"remark,omitempty"`
	RiskLevel   string     `json:"risk_level,omitempty"`
	FlyMode     string     `json:"fly_mode,omitempty"`
	FlyRate     *float64   `json:"fly_rate,omitempty"`
	LoginCount  *int       `json:"login_count,omitempty"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

type WorkspaceMemberList struct {
	Items    []WorkspaceMember `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type WorkspaceMemberService struct{ db *gorm.DB }

func NewWorkspaceMemberService(db *gorm.DB) *WorkspaceMemberService {
	return &WorkspaceMemberService{db: db}
}

// WorkspaceHumanMemberQuery is ONLY a roster/headcount scope. Never use it
// to authorize account, financial or trading operations: membership status 0
// means a member has switched rooms, not that the old room still manages them.
func WorkspaceHumanMemberQuery(db *gorm.DB, workspaceID uint64) *gorm.DB {
	query := db.Model(&user.User{})
	if workspaceID == 0 {
		return query.Where("1 = 0")
	}
	return query.Where(`"user".role = ? AND COALESCE("user".remark, '') NOT LIKE ?`, "member", "测试机器人专用账号%").
		Where(`NOT EXISTS (
			SELECT 1 FROM workspace_robot_profiles AS roster_robot
			WHERE roster_robot.user_id = "user".user_id
		)`).
		Where(`("user".workspace_id = ? OR EXISTS (
			SELECT 1 FROM workspace_memberships AS roster_membership
			WHERE roster_membership.user_id = "user".user_id
			  AND roster_membership.workspace_id = ?
			  AND roster_membership.role = ?
			  AND roster_membership.status IN (0, 1)
		))`, workspaceID, workspaceID, "member")
}

func (s *WorkspaceMemberService) List(workspaceID uint64, filter UserListFilter) (*WorkspaceMemberList, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择当前房间")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := WorkspaceHumanMemberQuery(s.db, workspaceID)
	if filter.UserID > 0 {
		query = query.Where(`"user".user_id = ?`, filter.UserID)
	}
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		// Search only returned public identity. Filtering on a hidden phone,
		// remark, or other room's private settings would leak those values.
		query = query.Where(`LOWER("user".username) LIKE ? OR LOWER("user".nickname) LIKE ? OR CAST("user".public_id AS TEXT) LIKE ?`, like, like, like)
	}
	switch filter.Status {
	case "active":
		query = query.Where(`"user".workspace_id = ? AND "user".status = ?`, workspaceID, 1)
	case "disabled":
		query = query.Where(`"user".workspace_id = ? AND "user".status = ?`, workspaceID, 0)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []struct {
		WorkspaceMember `gorm:"embedded"`
		BalanceCents    *int64
	}
	// Redact at the database projection, before serializing. In particular,
	// do not call AdminUser.Get/enrichOwnership for historical members.
	if err := query.Select(`"user".user_id AS id, "user".public_id, "user".username,
		"user".nickname, "user".avatar, "user".public_title, "user".public_badge, "user".role,
		("user".workspace_id = @room) AS in_current_room,
		CASE WHEN "user".workspace_id = @room THEN "user".balance_cents END AS balance_cents,
		CASE WHEN "user".workspace_id = @room THEN "user".status END AS status,
		CASE WHEN "user".workspace_id = @room THEN "user".phone ELSE '' END AS phone,
		CASE WHEN "user".workspace_id = @room THEN "user".remark ELSE '' END AS remark,
		CASE WHEN "user".workspace_id = @room THEN "user".risk_level ELSE '' END AS risk_level,
		CASE WHEN "user".workspace_id = @room THEN "user".fly_mode ELSE '' END AS fly_mode,
		CASE WHEN "user".workspace_id = @room THEN "user".fly_rate END AS fly_rate,
		CASE WHEN "user".workspace_id = @room THEN "user".login_count END AS login_count,
		CASE WHEN "user".workspace_id = @room THEN "user".last_login_at END AS last_login_at,
		CASE WHEN "user".workspace_id = @room THEN "user".created_at END AS created_at`, sql.Named("room", workspaceID)).
		Order(`"user".created_at DESC, "user".user_id DESC`).Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	currentIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.InCurrentRoom {
			currentIDs = append(currentIDs, row.ID)
		}
	}
	presence := ws.OnlineUsers(currentIDs)
	items := make([]WorkspaceMember, 0, len(rows))
	for _, row := range rows {
		item := row.WorkspaceMember
		item.CanManage = item.InCurrentRoom
		if item.InCurrentRoom {
			if row.BalanceCents != nil {
				balance := centsToAmount(*row.BalanceCents)
				item.Balance = &balance
			}
			online := presence[item.ID]
			item.Online = &online
		}
		items = append(items, item)
	}
	return &WorkspaceMemberList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}
