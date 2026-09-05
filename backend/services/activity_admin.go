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
	ID            uint64     `json:"id"`
	WorkspaceID   uint64     `json:"workspace_id"`
	Type          string     `json:"type"`
	Title         string     `json:"title"`
	Subtitle      string     `json:"subtitle"`
	Status        string     `json:"status"`
	Cover         string     `json:"cover"`
	Reward        float64    `json:"reward"`
	PoolTotal     float64    `json:"pool_total,omitempty"`
	PoolRemaining float64    `json:"pool_remaining,omitempty"`
	Config        any        `json:"config"`
	Participants  int64      `json:"participants"`
	SortOrder     int        `json:"sort_order"`
	StartsAt      *time.Time `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	CreatedAt     time.Time  `json:"created_at"`
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

var activityTypes = map[string]string{"checkin": "签到", "banner": "轮播", "promotion": "推广活动", "invite": "邀请", "redpacket": "红包"}

// Early development seeds started the displayed participant counters at 128
// and 56. Real participation subsequently incremented those values, so the
// repair must subtract the seed base rather than requiring an unused activity.
// The full immutable seed signature and the base+COUNT equality are both
// required: an operator-edited row or counter is deliberately left alone.
const legacySeedParticipantsReconcileSQL = `
WITH legacy_candidate AS (
  SELECT
    activity.id,
    CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END AS legacy_base,
    COUNT(participation.id)::bigint AS actual_participants
  FROM ops_activities AS activity
  LEFT JOIN activity_participations AS participation
    ON participation.activity_id = activity.id
  WHERE activity.workspace_id = ?
    AND activity.deleted_at IS NULL
    AND (
      (activity.type = 'checkin'
       AND activity.title = '每日签到'
       AND activity.subtitle = '连续签到领取积分'
       AND activity.status = 'active'
       AND COALESCE(activity.cover, '') = ''
       AND activity.reward_cents = 100
       AND activity.pool_total_cents = 0
       AND activity.pool_remaining_cents = 0
       AND activity.config_json = '{"days":7}'
       AND activity.sort_order = 1
       AND activity.starts_at IS NULL
       AND activity.ends_at IS NULL)
      OR
      (activity.type = 'redpacket'
       AND activity.title = '幸运红包'
       AND activity.subtitle = '开奖聊天室随机红包'
       AND activity.status = 'active'
       AND COALESCE(activity.cover, '') = ''
       AND activity.reward_cents = 888
       AND activity.pool_total_cents = 8800
       AND activity.pool_remaining_cents = 8800
       AND activity.config_json = '{"pool":88,"min_amount":1,"max_amount":8.8}'
       AND activity.sort_order = 4
       AND activity.starts_at IS NULL
       AND activity.ends_at IS NULL)
    )
  GROUP BY activity.id, activity.type, activity.participants
  HAVING activity.participants =
    CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END
    + COUNT(participation.id)
)
UPDATE ops_activities AS activity
SET participants = legacy_candidate.actual_participants,
    updated_at = clock_timestamp()
FROM legacy_candidate
WHERE activity.id = legacy_candidate.id
  AND activity.participants =
    legacy_candidate.legacy_base + legacy_candidate.actual_participants`

func NewActivityAdminService(db *gorm.DB) *ActivityAdminService { return &ActivityAdminService{db: db} }

func (s *ActivityAdminService) List(status string) ([]ActivityView, error) {
	return s.ListForWorkspace(0, status)
}

func (s *ActivityAdminService) ListForWorkspace(workspaceID uint64, status string) ([]ActivityView, error) {
	if workspaceID > 0 {
		if err := s.ensureDefaultsForWorkspace(workspaceID); err != nil {
			return nil, err
		}
	}
	query := s.db.Model(&activity.Activity{}).Order("sort_order asc, id desc")
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
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

func (s *ActivityAdminService) CreateForWorkspace(workspaceID uint64, input ActivityPayload) (*ActivityView, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择活动所属房间")
	}
	row, err := validateActivity(input)
	if err != nil {
		return nil, err
	}
	row.WorkspaceID = workspaceID
	applyActivityPool(row)
	if err := s.db.Create(row).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_CREATE_FAILED", "创建活动失败", err)
	}
	view := toActivityView(*row)
	return &view, nil
}

func (s *ActivityAdminService) UpdateForWorkspace(workspaceID, id uint64, input ActivityPayload) (*ActivityView, error) {
	var row activity.Activity
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
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
	applyActivityPool(&row)
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("ACTIVITY_UPDATE_FAILED", "更新活动失败", err)
	}
	view := toActivityView(row)
	return &view, nil
}

func (s *ActivityAdminService) SetStatusForWorkspace(workspaceID, id uint64, status string) (*ActivityView, error) {
	status = strings.TrimSpace(status)
	if status != "draft" && status != "active" && status != "ended" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动状态不正确")
	}
	var row activity.Activity
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
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

func (s *ActivityAdminService) DeleteForWorkspace(workspaceID, id uint64) error {
	result := s.db.Where("workspace_id = ?", workspaceID).Delete(&activity.Activity{}, id)
	if result.Error != nil {
		return apperrors.NewSystemError("ACTIVITY_DELETE_FAILED", "删除活动失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
	}
	return nil
}

// EnsureDefaultsForWorkspace materializes the room-owned activity catalog.
// It is exported for the centralized bootstrap; API reads no longer need to be
// the first operation which happens to create required base records.
func (s *ActivityAdminService) EnsureDefaultsForWorkspace(workspaceID uint64) error {
	if workspaceID == 0 {
		return apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择活动所属房间")
	}
	return s.ensureDefaultsForWorkspace(workspaceID)
}

func workspaceDefaultActivities() []activity.Activity {
	return []activity.Activity{
		{Type: "checkin", Title: "每日签到", Subtitle: "连续签到领取积分", Status: "active", RewardCents: 100, Participants: 0, SortOrder: 1, ConfigJSON: `{"days":7}`},
		{Type: "banner", Title: "首页轮播", Subtitle: "运营位轮播图", Status: "active", SortOrder: 2, ConfigJSON: `{"slides":[]}`},
		{Type: "invite", Title: "邀请有礼", Subtitle: "邀请好友双方得奖励", Status: "active", RewardCents: 500, SortOrder: 3, ConfigJSON: `{"bonus":5}`},
		{Type: "redpacket", Title: "幸运红包", Subtitle: "开奖聊天室随机红包", Status: "active", RewardCents: 888, PoolTotalCents: 8800, PoolRemainingCents: 8800, Participants: 0, SortOrder: 4, ConfigJSON: `{"pool":88,"min_amount":1,"max_amount":8.8}`},
		{Type: "promotion", Title: "幸运大转盘", Subtitle: "单转最高可获 288.88 积分", Status: "active", Cover: "/images/activities/lucky-wheel.jpg", SortOrder: 101, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/welfare"}`},
		{Type: "promotion", Title: "加拿大28玩法上线", Subtitle: "全新加拿大28玩法现已开放", Status: "active", Cover: "/images/activities/canada-28-launch.jpg", SortOrder: 102, ConfigJSON: `{"action_type":"internal","action_url":"/games/canada-28"}`},
		{Type: "promotion", Title: "连续签到七天送积分", Subtitle: "连续签到，天天领取积分好礼", Status: "active", Cover: "/images/activities/seven-day-checkin.jpg", SortOrder: 103, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/welfare"}`},
		{Type: "promotion", Title: "98Pay首充礼", Subtitle: "积分首充活动", Status: "active", Cover: "/images/activities/98pay-first-credit.jpg", SortOrder: 104, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/credit"}`},
		{Type: "promotion", Title: "爆庄来袭", Subtitle: "全场福利活动", Status: "active", Cover: "/images/activities/bonus-arrival.jpg", SortOrder: 105, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/welfare"}`},
		{Type: "promotion", Title: "每周累计流水送彩金", Subtitle: "每周累计流水领取专属奖励", Status: "active", Cover: "/images/activities/weekly-turnover.jpg", SortOrder: 106, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/rebate"}`},
		{Type: "promotion", Title: "天天返水", Subtitle: "每日返水福利活动", Status: "active", Cover: "/images/activities/daily-rebate.jpg", SortOrder: 107, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/rebate"}`},
		{Type: "promotion", Title: "组合及单点连中奖励", Subtitle: "组合及单点连中享额外奖励", Status: "active", Cover: "/images/activities/streak-reward.jpg", SortOrder: 108, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/welfare"}`},
		{Type: "promotion", Title: "全民代理计划", Subtitle: "邀请好友，查看代理奖励计划", Status: "active", Cover: "/images/activities/agent-plan.jpg", SortOrder: 109, ConfigJSON: `{"action_type":"internal","action_url":"/wallet/invite"}`},
	}
}

func (s *ActivityAdminService) ensureDefaultsForWorkspace(workspaceID uint64) error {
	// Keep this precise idempotent reconciliation in runtime bootstrap as well
	// as the versioned migration. Legacy bases are removed while preserving the
	// actual count; operator-managed counters fail the exact equality guard.
	if err := s.db.Exec(legacySeedParticipantsReconcileSQL, workspaceID).Error; err != nil {
		return err
	}
	// Reconcile the former cash-oriented copy before materializing defaults so
	// existing rooms do not keep advertising check-in points as cash bonuses.
	if err := s.db.Model(&activity.Activity{}).
		Where("workspace_id = ? AND type = ? AND title = ?", workspaceID, "promotion", "连续签到七天送彩金").
		Updates(map[string]any{"title": "连续签到七天送积分", "subtitle": "连续签到，天天领取积分好礼"}).Error; err != nil {
		return err
	}
	defaults := workspaceDefaultActivities()
	for index := range defaults {
		row := defaults[index]
		row.WorkspaceID = workspaceID
		if err := s.db.Where("workspace_id = ? AND type = ? AND title = ?", workspaceID, row.Type, row.Title).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
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

func applyActivityPool(row *activity.Activity) {
	if row.Type != "redpacket" {
		return
	}
	cfg := parseRedPacketConfig(row.ConfigJSON)
	total := int64(cfg.Pool * 100)
	if total <= 0 {
		total = 8800
	}
	if row.PoolTotalCents <= 0 {
		row.PoolTotalCents = total
	}
	if row.PoolRemainingCents <= 0 {
		row.PoolRemainingCents = row.PoolTotalCents
	}
}

func toActivityView(row activity.Activity) ActivityView {
	var cfg any
	_ = json.Unmarshal([]byte(defaultJSON(row.ConfigJSON, "{}")), &cfg)
	view := ActivityView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, Status: row.Status,
		Cover: row.Cover, Reward: centsToAmount(row.RewardCents), Config: cfg, Participants: row.Participants,
		SortOrder: row.SortOrder, StartsAt: row.StartsAt, EndsAt: row.EndsAt, CreatedAt: row.CreatedAt,
	}
	if row.Type == "redpacket" {
		ensureActivityPool(&row)
		view.PoolTotal = centsToAmount(row.PoolTotalCents)
		view.PoolRemaining = centsToAmount(row.PoolRemainingCents)
	}
	return view
}
