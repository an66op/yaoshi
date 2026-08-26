package services

import (
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/notify"
	apperrors "backend/errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type NotifyAdminService struct{ db *gorm.DB }

type NotificationView struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Level     string    `json:"level"`
	Link      string    `json:"link"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

func NewNotifyAdminService(db *gorm.DB) *NotifyAdminService { return &NotifyAdminService{db: db} }

func (s *NotifyAdminService) List(limit int) ([]NotificationView, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if err := s.refreshDerived(); err != nil {
		return nil, apperrors.NewSystemError("NOTIFY_REFRESH_FAILED", "刷新待办提醒失败", err)
	}
	var rows []notify.Notification
	if err := s.db.Order("id desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("NOTIFY_READ_FAILED", "读取通知失败", err)
	}
	items := make([]NotificationView, 0, len(rows))
	for _, row := range rows {
		items = append(items, NotificationView{ID: row.ID, Title: row.Title, Content: row.Content, Level: row.Level, Link: row.Link, Read: row.Read, CreatedAt: row.CreatedAt})
	}
	return items, nil
}

func (s *NotifyAdminService) MarkRead(id uint64) error {
	now := time.Now().UTC()
	result := s.db.Model(&notify.Notification{}).Where("id = ?", id).Updates(map[string]any{"read": true, "read_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("NOT_FOUND", "通知不存在")
	}
	return nil
}

func (s *NotifyAdminService) MarkAllRead() error {
	now := time.Now().UTC()
	return s.db.Model(&notify.Notification{}).Where("read = ?", false).Updates(map[string]any{"read": true, "read_at": now}).Error
}

func (s *NotifyAdminService) refreshDerived() error {
	var pendingApps int64
	if err := s.db.Model(&application.Application{}).Where("status = ?", "pending").Count(&pendingApps).Error; err != nil {
		return err
	}
	var pendingBets int64
	if err := s.db.Model(&bet.Bet{}).Where("status = ?", "pending").Count(&pendingBets).Error; err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Older code generated a new row whenever a count changed. Remove those
		// snapshots and keep one stable actionable reminder per work queue.
		if err := tx.Where("title = ?", "系统运行正常").Delete(&notify.Notification{}).Error; err != nil {
			return err
		}
		if err := syncAdminReminder(tx, "待审核申请", "有 % 笔申请待审核", pendingApps,
			fmt.Sprintf("%d 笔申请等待处理", pendingApps), "warning", "/applications"); err != nil {
			return err
		}
		return syncAdminReminder(tx, "待结算注单", "有 % 笔注单待结算", pendingBets,
			fmt.Sprintf("%d 笔注单等待结算", pendingBets), "info", "/monitor")
	})
}

func syncAdminReminder(tx *gorm.DB, stableTitle, legacyPattern string, count int64, content, level, link string) error {
	var rows []notify.Notification
	if err := tx.Where("title = ? OR title LIKE ?", stableTitle, legacyPattern).Order("id desc").Find(&rows).Error; err != nil {
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
		return tx.Create(&notify.Notification{Title: stableTitle, Content: content, Level: level, Link: link}).Error
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
