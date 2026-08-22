package services

import (
	"backend/data/models/activity"
	apperrors "backend/errors"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ActivityAdminService struct{ db *gorm.DB }

type ActivityView struct {
	ID           uint64     `json:"id"`
	Type         string     `json:"type"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle"`
	Status       string     `json:"status"`
	Cover        string     `json:"cover"`
	Reward       float64    `json:"reward"`
	Config       any        `json:"config"`
	Participants int64      `json:"participants"`
	SortOrder    int        `json:"sort_order"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ActivityPayload struct {
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Subtitle  string  `json:"subtitle"`
	Status    string  `json:"status"`
	Cover     string  `json:"cover"`
	Reward    float64 `json:"reward"`
	Config    any     `json:"config"`
	SortOrder int     `json:"sort_order"`
}

var activityTypes = map[string]string{"checkin": "签到", "banner": "轮播", "invite": "邀请", "redpacket": "红包"}

func NewActivityAdminService(db *gorm.DB) *ActivityAdminService { return &ActivityAdminService{db: db} }

func (s *ActivityAdminService) List(status string) ([]ActivityView, error) {
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	query := s.db.Model(&activity.Activity{}).Order("sort_order asc, id desc")
	if st := strings.TrimSpace(status); st != "" && st != "all" {
		query = query.Where("status = ?", st)
	}
	var rows []activity.Activity
	if err := query.Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_READ_FAILED", "读取活动失败", err)
	}
	items := make([]ActivityView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toActivityView(row))
	}
	return items, nil
}

func (s *ActivityAdminService) Create(input ActivityPayload) (*ActivityView, error) {
	row, err := validateActivity(input)
	if err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_CREATE_FAILED", "创建活动失败", err)
	}
	view := toActivityView(*row)
	return &view, nil
}

func (s *ActivityAdminService) Update(id uint64, input ActivityPayload) (*ActivityView, error) {
	var row activity.Activity
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
		}
		return nil, apperrors.NewSystemError("ACTIVITY_READ_FAILED", "读取活动失败", err)
	}
	next, err := validateActivity(input)
	if err != nil {
		return nil, err
	}
	row.Type, row.Title, row.Subtitle, row.Status = next.Type, next.Title, next.Subtitle, next.Status
	row.Cover, row.RewardCents, row.ConfigJSON, row.SortOrder = next.Cover, next.RewardCents, next.ConfigJSON, next.SortOrder
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_UPDATE_FAILED", "更新活动失败", err)
	}
	view := toActivityView(row)
	return &view, nil
}

func (s *ActivityAdminService) SetStatus(id uint64, status string) (*ActivityView, error) {
	status = strings.TrimSpace(status)
	if status != "draft" && status != "active" && status != "ended" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动状态不正确")
	}
	var row activity.Activity
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
		}
		return nil, err
	}
	row.Status = status
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_UPDATE_FAILED", "更新活动失败", err)
	}
	view := toActivityView(row)
	return &view, nil
}

func (s *ActivityAdminService) Delete(id uint64) error {
	result := s.db.Delete(&activity.Activity{}, id)
	if result.Error != nil {
		return apperrors.NewSystemError("ACTIVITY_DELETE_FAILED", "删除活动失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
	}
	return nil
}

func (s *ActivityAdminService) ensureDefaults() error {
	var count int64
	if err := s.db.Model(&activity.Activity{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []activity.Activity{
		{Type: "checkin", Title: "每日签到", Subtitle: "连续签到领取积分", Status: "active", RewardCents: 100, Participants: 128, SortOrder: 1, ConfigJSON: `{"days":7}`},
		{Type: "banner", Title: "首页轮播", Subtitle: "运营位轮播图", Status: "active", SortOrder: 2, ConfigJSON: `{"slides":[]}`},
		{Type: "invite", Title: "邀请有礼", Subtitle: "邀请好友双方得奖励", Status: "active", RewardCents: 500, SortOrder: 3, ConfigJSON: `{"bonus":5}`},
		{Type: "redpacket", Title: "幸运红包", Subtitle: "开奖聊天室随机红包", Status: "active", RewardCents: 888, Participants: 56, SortOrder: 4, ConfigJSON: `{"pool":88}`},
	}
	return s.db.Create(&defaults).Error
}

func validateActivity(input ActivityPayload) (*activity.Activity, error) {
	typ := strings.TrimSpace(input.Type)
	title := strings.TrimSpace(input.Title)
	if _, ok := activityTypes[typ]; !ok {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动类型不正确")
	}
	if title == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动标题不能为空")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "draft"
	}
	if status != "draft" && status != "active" && status != "ended" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动状态不正确")
	}
	cfg, _ := json.Marshal(input.Config)
	if len(cfg) == 0 || string(cfg) == "null" {
		cfg = []byte("{}")
	}
	return &activity.Activity{
		Type: typ, Title: title, Subtitle: strings.TrimSpace(input.Subtitle), Status: status,
		Cover: strings.TrimSpace(input.Cover), RewardCents: int64(input.Reward * 100),
		ConfigJSON: string(cfg), SortOrder: input.SortOrder,
	}, nil
}

func toActivityView(row activity.Activity) ActivityView {
	var cfg any
	_ = json.Unmarshal([]byte(defaultJSON(row.ConfigJSON, "{}")), &cfg)
	return ActivityView{
		ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, Status: row.Status,
		Cover: row.Cover, Reward: centsToAmount(row.RewardCents), Config: cfg, Participants: row.Participants,
		SortOrder: row.SortOrder, StartsAt: row.StartsAt, EndsAt: row.EndsAt, CreatedAt: row.CreatedAt,
	}
}
