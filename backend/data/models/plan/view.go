package plan

import "time"

// PublicationView is the first confirmed member view of one persisted plan
// publication. The unique identity makes refreshes and retries idempotent;
// ViewedAt is never rewritten after the first successful insert.
type PublicationView struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64    `gorm:"not null;uniqueIndex:idx_plan_publication_view,priority:1;index" json:"workspace_id"`
	UserID      uint64    `gorm:"not null;uniqueIndex:idx_plan_publication_view,priority:2;index" json:"user_id"`
	GameID      string    `gorm:"size:40;not null;uniqueIndex:idx_plan_publication_view,priority:3;index" json:"game_id"`
	Issue       string    `gorm:"size:64;not null;uniqueIndex:idx_plan_publication_view,priority:4" json:"issue"`
	Position    int       `gorm:"not null;uniqueIndex:idx_plan_publication_view,priority:5" json:"position"`
	PlanKey     string    `gorm:"size:48;not null;uniqueIndex:idx_plan_publication_view,priority:6" json:"plan_key"`
	ViewedAt    time.Time `gorm:"not null" json:"viewed_at"`
}

func (PublicationView) TableName() string { return "plan_publication_views" }
