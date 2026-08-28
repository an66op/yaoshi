package services

import (
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/notify"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type NotifyAdminService struct{ db *gorm.DB }

type NotificationView struct {
	ID          uint64    `json:"id"`
	WorkspaceID uint64    `json:"workspace_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Level       string    `json:"level"`
	Link        string    `json:"link"`
	Read        bool      `json:"read"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewNotifyAdminService(db *gorm.DB) *NotifyAdminService { return &NotifyAdminService{db: db} }

func (s *NotifyAdminService) List(limit int) ([]NotificationView, error) {
	return s.ListForWorkspace(0, limit)
}

func (s *NotifyAdminService) ListForWorkspace(workspaceID uint64, limit int) ([]NotificationView, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if err := s.refreshDerived(workspaceID); err != nil {
		return nil, apperrors.NewSystemError("NOTIFY_REFRESH_FAILED", "刷新待办提醒失败", err)
	}
	var rows []notify.Notification
	query := s.db.Order("id desc").Limit(limit)
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("NOTIFY_READ_FAILED", "读取通知失败", err)
	}
	items := make([]NotificationView, 0, len(rows))
	for _, row := range rows {
		items = append(items, NotificationView{ID: row.ID, WorkspaceID: row.WorkspaceID, Title: row.Title, Content: row.Content, Level: row.Level, Link: row.Link, Read: row.Read, CreatedAt: row.CreatedAt})
	}
	return items, nil
}

func (s *NotifyAdminService) MarkRead(id uint64) error {
	return s.MarkReadForWorkspace(0, id)
}

func (s *NotifyAdminService) MarkReadForWorkspace(workspaceID, id uint64) error {
	now := time.Now().UTC()
	query := s.db.Model(&notify.Notification{}).Where("id = ?", id)
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	result := query.Updates(map[string]any{"read": true, "read_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("NOT_FOUND", "通知不存在")
	}
	return nil
}

func (s *NotifyAdminService) MarkAllRead() error {
	return s.MarkAllReadForWorkspace(0)
}

func (s *NotifyAdminService) MarkAllReadForWorkspace(workspaceID uint64) error {
	now := time.Now().UTC()
	query := s.db.Model(&notify.Notification{}).Where("read = ?", false)
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	return query.Updates(map[string]any{"read": true, "read_at": now}).Error
}

func (s *NotifyAdminService) refreshDerived(workspaceID uint64) error {
	betQuery := s.db.Model(&bet.Bet{}).Where("status = ?", "pending")
	platformScope := workspaceID == 0
	if workspaceID > 0 {
		var workspace workspacemodel.Workspace
		if err := s.db.Select("type").First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		if workspace.Type == workspacemodel.TypePlatform {
			platformScope = true
			// Platform reminders summarize all formal rooms but are stored on the
			// platform workspace itself. Tenant and agent reminders stay isolated.
			betQuery = betQuery.Where("workspace_id > 0")
		} else {
			betQuery = betQuery.Where("workspace_id = ?", workspaceID)
		}
	} else {
		betQuery = betQuery.Where("workspace_id > 0")
	}
	applicationQuery := pendingApplicationReminderQuery(s.db, workspaceID, platformScope)
	var pendingApps int64
	if err := applicationQuery.Count(&pendingApps).Error; err != nil {
		return err
	}
	var pendingBets int64
	if err := betQuery.Count(&pendingBets).Error; err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Older code generated a new row whenever a count changed. Remove those
		// snapshots and keep one stable actionable reminder per work queue.
		if err := tx.Where("workspace_id = ? AND title = ?", workspaceID, "系统运行正常").Delete(&notify.Notification{}).Error; err != nil {
			return err
		}
		if err := syncAdminReminder(tx, workspaceID, "待审核申请", "有 % 笔申请待审核", pendingApps,
			fmt.Sprintf("%d 笔申请等待处理", pendingApps), "warning", "/applications"); err != nil {
			return err
		}
		return syncAdminReminder(tx, workspaceID, "待结算注单", "有 % 笔注单待结算", pendingBets,
			fmt.Sprintf("%d 笔注单等待结算", pendingBets), "info", "/monitor")
	})
}

func pendingApplicationReminderQuery(db *gorm.DB, workspaceID uint64, platformScope bool) *gorm.DB {
	query := db.Model(&application.Application{}).Where("status = ?", "pending")
	if platformScope {
		return query.Where("workspace_id > 0 AND request_type <> ?", "join")
	}
	return query.Where("workspace_id = ?", workspaceID)
}

func syncAdminReminder(tx *gorm.DB, workspaceID uint64, stableTitle, legacyPattern string, count int64, content, level, link string) error {
	var rows []notify.Notification
	if err := tx.Where("workspace_id = ? AND (title = ? OR title LIKE ?)", workspaceID, stableTitle, legacyPattern).Order("id desc").Find(&rows).Error; err != nil {
		return err
	}
	if count == 0 {
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return tx.Where("id IN ?", ids).Delete(&notify.Notification{}).Error
	}
	if len(rows) == 0 {
		return tx.Create(&notify.Notification{WorkspaceID: workspaceID, Title: stableTitle, Content: content, Level: level, Link: link}).Error
	}
	keep := rows[0]
	updates := map[string]any{
		"title": stableTitle, "content": content, "level": level, "link": link,
	}
	if keep.Content != content {
		updates["read"] = false
		updates["read_at"] = nil
	}
	if err := tx.Model(&keep).Updates(updates).Error; err != nil {
		return err
	}
	if len(rows) > 1 {
		ids := make([]uint64, 0, len(rows)-1)
		for _, row := range rows[1:] {
			ids = append(ids, row.ID)
		}
		if err := tx.Where("id IN ?", ids).Delete(&notify.Notification{}).Error; err != nil {
			return err
		}
	}
	return nil
}
