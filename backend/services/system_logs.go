package services

import (
	"backend/data/models/audit"
	"backend/lotteryfeed"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

type SystemLogService struct{ db *gorm.DB }

func NewSystemLogService(db *gorm.DB) *SystemLogService { return &SystemLogService{db: db} }

type SystemLogFilter struct {
	BeforeID    uint64
	Limit       int
	Category    string
	EventType   string
	Status      string
	GameID      string
	SourceGroup string
	Query       string
	From        *time.Time
	To          *time.Time
}

type SystemLogPage struct {
	Items      []audit.SystemEvent `json:"items"`
	NextBefore uint64              `json:"next_before_id,omitempty"`
	HasMore    bool                `json:"has_more"`
}

func (s *SystemLogService) Logs(filter SystemLogFilter) (SystemLogPage, error) {
	if s == nil || s.db == nil {
		return SystemLogPage{}, fmt.Errorf("database is nil")
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	query := s.db.Model(&audit.SystemEvent{}).Order("id DESC").Limit(filter.Limit + 1)
	if filter.BeforeID > 0 {
		query = query.Where("id < ?", filter.BeforeID)
	}
	if value := strings.TrimSpace(filter.Category); value != "" {
		query = query.Where("category = ?", value)
	}
	if value := strings.TrimSpace(filter.EventType); value != "" {
		query = query.Where("event_type = ?", value)
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("status = ?", value)
	}
	if value := strings.TrimSpace(filter.GameID); value != "" {
		query = query.Where("game_id = ?", value)
	}
	if value := strings.TrimSpace(filter.SourceGroup); value != "" {
		query = query.Where("source_group = ?", value)
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		query = query.Where("STRPOS(LOWER(message), LOWER(?)) > 0", value)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		// The UI sends the next local midnight as an exclusive day boundary.
		// Excluding it prevents an exact 00:00 event from leaking into the
		// preceding day's filter.
		query = query.Where("created_at < ?", filter.To.UTC())
	}
	var rows []audit.SystemEvent
	if err := query.Find(&rows).Error; err != nil {
		return SystemLogPage{}, err
	}
	hasMore := len(rows) > filter.Limit
	if hasMore {
		rows = rows[:filter.Limit]
	}
	next := uint64(0)
	if hasMore && len(rows) > 0 {
		next = rows[len(rows)-1].ID
	}
	return SystemLogPage{Items: rows, NextBefore: next, HasMore: hasMore}, nil
}

// RecordSchedulerEvent is deliberately best-effort: failure to write an
// observability event must never block draw import or settlement. The source
// health fields on lottery_games remain the fail-closed operational control.
func (s *SystemLogService) RecordSchedulerEvent(ctx context.Context, event lotteryfeed.Event) {
	if s == nil || s.db == nil {
		return
	}
	row := audit.SystemEvent{
		Category: event.Category, EventType: event.Type, Level: event.Level, Status: event.Status,
		SourceGroup: event.SourceGroup, GameID: event.GameID, JobID: event.JobID,
		Message: event.Message, Imported: event.Imported, LatestIssue: event.LatestIssue,
		ConsecutiveErrors: event.ConsecutiveErrors, CreatedAt: event.OccurredAt.UTC(),
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	row.Message = limitDBText(strings.TrimSpace(row.Message), 500)
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		log.Printf("系统运行日志写入失败: type=%s game=%s error=%v", row.EventType, row.GameID, err)
	}
}
