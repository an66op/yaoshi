package services

import (
	"backend/data/models/special"
	"backend/data/models/user"
	apperrors "backend/errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SpecialAdminService struct{ db *gorm.DB }

type NumberResourceView struct {
	ID            uint64    `json:"id"`
	Number        string    `json:"number"`
	Level         string    `json:"level"`
	Status        string    `json:"status"`
	OwnerUserID   *uint64   `json:"owner_user_id"`
	OwnerUsername string    `json:"owner_username"`
	Price         float64   `json:"price"`
	Remark        string    `json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}

type SpecialCampaignView struct {
	ID           uint64     `json:"id"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	RuleText     string     `json:"rule_text"`
	GrantedCount int64      `json:"granted_count"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type SpecialOverview struct {
	Available int64                 `json:"available"`
	Reserved  int64                 `json:"reserved"`
	Granted   int64                 `json:"granted"`
	Campaigns []SpecialCampaignView `json:"campaigns"`
	Resources []NumberResourceView  `json:"resources"`
}

type RoomResolveResult struct {
	RoomCode      string `json:"room_code"`
	RoomName      string `json:"room_name"`
	AgentID       uint64 `json:"agent_id"`
	AgentUsername string `json:"agent_username"`
	AgentNickname string `json:"agent_nickname"`
}

func NewSpecialAdminService(db *gorm.DB) *SpecialAdminService { return &SpecialAdminService{db: db} }

func (s *SpecialAdminService) Overview() (*SpecialOverview, error) {
	var available, reserved, granted int64
	_ = s.db.Model(&special.NumberResource{}).Where("status = ?", "available").Count(&available).Error
	_ = s.db.Model(&special.NumberResource{}).Where("status = ?", "reserved").Count(&reserved).Error
	_ = s.db.Model(&special.NumberResource{}).Where("status = ?", "granted").Count(&granted).Error
	var campaigns []special.Campaign
	_ = s.db.Order("id desc").Limit(50).Find(&campaigns).Error
	var resources []special.NumberResource
	_ = s.db.Order("id desc").Limit(100).Find(&resources).Error
	ownerIDs := make([]uint64, 0)
	for _, row := range resources {
		if row.OwnerUserID != nil {
			ownerIDs = append(ownerIDs, *row.OwnerUserID)
		}
	}
	names := map[uint64]string{}
	if len(ownerIDs) > 0 {
		var owners []user.User
		_ = s.db.Select("user_id, username, nickname").Where("user_id IN ?", ownerIDs).Find(&owners).Error
		for _, owner := range owners {
			names[owner.UserID] = defaultString(owner.Nickname, owner.Username)
		}
	}
	out := &SpecialOverview{Available: available, Reserved: reserved, Granted: granted}
	for _, row := range campaigns {
		out.Campaigns = append(out.Campaigns, SpecialCampaignView{ID: row.ID, Title: row.Title, Status: row.Status, RuleText: row.RuleText, GrantedCount: row.GrantedCount, StartsAt: row.StartsAt, EndsAt: row.EndsAt, CreatedAt: row.CreatedAt})
	}
	for _, row := range resources {
		item := NumberResourceView{ID: row.ID, Number: row.Number, Level: row.Level, Status: row.Status, OwnerUserID: row.OwnerUserID, Price: centsToAmount(row.PriceCents), Remark: row.Remark, CreatedAt: row.CreatedAt}
		if row.OwnerUserID != nil {
			item.OwnerUsername = names[*row.OwnerUserID]
		}
		out.Resources = append(out.Resources, item)
	}
	return out, nil
}

func (s *SpecialAdminService) AddResources(numbers []string, level, remark string) (int, error) {
	level = defaultString(strings.TrimSpace(level), "normal")
	if level != "normal" && level != "rare" && level != "epic" {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "房间号等级不正确")
	}
	created := 0
	for _, raw := range numbers {
		number := strings.TrimSpace(raw)
		if number == "" {
			continue
		}
		row := special.NumberResource{Number: number, Level: level, Status: "available", Remark: strings.TrimSpace(remark)}
		result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return created, apperrors.NewSystemError("SPECIAL_CREATE_FAILED", "添加房间号失败", result.Error)
		}
		created += int(result.RowsAffected)
	}
	return created, nil
}

func (s *SpecialAdminService) CreateCampaign(title, rule, status string) (*SpecialCampaignView, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动标题不能为空")
	}
	status = defaultString(strings.TrimSpace(status), "draft")
	row := special.Campaign{Title: title, RuleText: strings.TrimSpace(rule), Status: status}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("SPECIAL_CREATE_FAILED", "创建房间号活动失败", err)
	}
	view := SpecialCampaignView{ID: row.ID, Title: row.Title, Status: row.Status, RuleText: row.RuleText, GrantedCount: row.GrantedCount, CreatedAt: row.CreatedAt}
	return &view, nil
}

// Grant assigns a vanity room number to a user and promotes them to agent.
func (s *SpecialAdminService) Grant(campaignID, resourceID, userID uint64, operator string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var campaign special.Campaign
		if err := tx.First(&campaign, campaignID).Error; err != nil {
			return apperrors.NewBusinessError("NOT_FOUND", "房间号活动不存在")
		}
		var resource special.NumberResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&resource, resourceID).Error; err != nil {
			return apperrors.NewBusinessError("NOT_FOUND", "房间号资源不存在")
		}
		if resource.Status != "available" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该房间号不可发放")
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		if account.Role == "admin" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "不能给管理员发放代理房间号")
		}
		if strings.TrimSpace(account.AgentRoomCode) != "" && account.AgentRoomCode != resource.Number {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该用户已绑定其他房间号: "+account.AgentRoomCode)
		}
		var occupied int64
		if err := tx.Model(&user.User{}).Where("agent_room_code = ? AND user_id <> ?", resource.Number, account.UserID).Count(&occupied).Error; err != nil {
			return err
		}
		if occupied > 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该房间号已被其他代理占用")
		}
		ownerID := account.UserID
		resource.Status = "granted"
		resource.OwnerUserID = &ownerID
		if err := tx.Save(&resource).Error; err != nil {
			return err
		}
		updates := map[string]any{"role": "agent", "agent_room_code": resource.Number}
		if err := tx.Model(&account).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Create(&special.GrantRecord{CampaignID: campaignID, ResourceID: resourceID, Number: resource.Number, UserID: account.UserID, Username: account.Username, Operator: defaultString(operator, "后台管理员")}).Error; err != nil {
			return err
		}
		return tx.Model(&campaign).Update("granted_count", gorm.Expr("granted_count + 1")).Error
	})
}

// AssignRoom binds an available room number to an agent without requiring a campaign.
func (s *SpecialAdminService) AssignRoom(resourceID, userID uint64, operator string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var campaign special.Campaign
		err := tx.Where("status = ?", "active").Order("id desc").First(&campaign).Error
		if err == gorm.ErrRecordNotFound {
			campaign = special.Campaign{Title: "代理房间号发放", Status: "active", RuleText: "后台直接分配"}
			if err := tx.Create(&campaign).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		return NewSpecialAdminService(tx).Grant(campaign.ID, resourceID, userID, operator)
	})
}

func (s *SpecialAdminService) ResolveRoom(code string) (*RoomResolveResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请输入房间号")
	}
	var account user.User
	err := s.db.Where("agent_room_code = ? AND role = ? AND status = ?", code, "agent", 1).First(&account).Error
	if err == gorm.ErrRecordNotFound {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间号无效或未开通")
	}
	if err != nil {
		return nil, err
	}
	return &RoomResolveResult{
		RoomCode: code,
		RoomName: defaultString(account.Nickname, account.Username) + "的房间",
		AgentID: account.UserID,
		AgentUsername: account.Username,
		AgentNickname: defaultString(account.Nickname, account.Username),
	}, nil
}
