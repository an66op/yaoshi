package services

import (
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/notify"
	"backend/data/models/settings"
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
	_ = s.refreshDerived()
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
	var unread int64
	_ = s.db.Model(&notify.Notification{}).Where("read = ?", false).Count(&unread).Error
	var pendingApps int64
	_ = s.db.Model(&application.Application{}).Where("status = ?", "pending").Count(&pendingApps).Error
	if pendingApps > 0 {
		title := fmt.Sprintf("有 %d 笔申请待审核", pendingApps)
		var exists int64
		_ = s.db.Model(&notify.Notification{}).Where("title = ? AND read = ?", title, false).Count(&exists).Error
		if exists == 0 {
			_ = s.db.Create(&notify.Notification{Title: title, Content: "请前往申请管理处理上下分/入群申请。", Level: "warning", Link: "/applications"}).Error
		}
	}
	var pendingBets int64
	_ = s.db.Model(&bet.Bet{}).Where("status = ?", "pending").Count(&pendingBets).Error
	if pendingBets > 0 {
		title := fmt.Sprintf("有 %d 笔注单待结算", pendingBets)
		var exists int64
		_ = s.db.Model(&notify.Notification{}).Where("title = ? AND read = ?", title, false).Count(&exists).Error
		if exists == 0 {
			_ = s.db.Create(&notify.Notification{Title: title, Content: "可在现场监控或开奖结果页执行结算。", Level: "info", Link: "/monitor"}).Error
		}
	}
	var count int64
	_ = s.db.Model(&notify.Notification{}).Count(&count).Error
	if count == 0 {
		room := "曜图"
		var cfg settings.SystemConfig
		if s.db.First(&cfg, 1).Error == nil && cfg.RoomName != "" {
			room = cfg.RoomName
		}
		_ = s.db.Create(&notify.Notification{Title: "系统运行正常", Content: room + " 管理中心已就绪，开奖线路与调度服务可在扩展服务中查看。", Level: "success", Link: "/lottery-network"}).Error
	}
	return nil
}
