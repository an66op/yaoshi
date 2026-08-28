package lifecycle

import "time"

const (
	ClassChatMessages      = "chat_messages"
	ClassRobotChatMessages = "robot_chat_messages"
	ClassNotifications     = "notifications"
	ClassAuditLogs         = "audit_logs"
	ClassRobotTestData     = "robot_test_data"

	ActionSoftDelete          = "soft_delete"
	ActionHardDelete          = "hard_delete"
	ActionArchiveThenPurgeHot = "archive_then_purge_hot"
	ActionColdArchive         = "cold_archive"
)

// RetentionPolicy is intentionally separate from room feature settings. A
// workspace-specific row overrides workspace 0, which is the platform default.
type RetentionPolicy struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint64    `gorm:"not null;default:0;uniqueIndex:uq_retention_policy_workspace_class,priority:1" json:"workspace_id"`
	DataClass     string    `gorm:"size:40;not null;uniqueIndex:uq_retention_policy_workspace_class,priority:2" json:"data_class"`
	Enabled       bool      `gorm:"not null;default:false" json:"enabled"`
	RetentionDays int       `gorm:"not null" json:"retention_days"`
	Action        string    `gorm:"size:32;not null" json:"action"`
	UpdatedByID   uint64    `gorm:"not null;default:0" json:"updated_by_id"`
	UpdatedByName string    `gorm:"size:80;not null;default:''" json:"updated_by_name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (RetentionPolicy) TableName() string { return "data_retention_policies" }

// CleanupRun is both the idempotency reservation and the immutable execution
// receipt. Criteria and preview data are frozen before any content is touched.
type CleanupRun struct {
	ID                         uint64     `gorm:"primaryKey" json:"id"`
	RequestID                  string     `gorm:"size:96;not null;uniqueIndex" json:"request_id"`
	WorkspaceID                uint64     `gorm:"not null;default:0;index" json:"workspace_id"`
	AllWorkspaces              bool       `gorm:"not null;default:false" json:"all_workspaces"`
	ActorID                    uint64     `gorm:"not null" json:"actor_id"`
	ActorName                  string     `gorm:"size:80;not null" json:"actor_name"`
	ExecutedByID               uint64     `gorm:"not null;default:0" json:"executed_by_id,omitempty"`
	ExecutedByName             string     `gorm:"size:80;not null;default:''" json:"executed_by_name,omitempty"`
	Status                     string     `gorm:"size:24;not null;index" json:"status"`
	BatchLimit                 int        `gorm:"not null;default:5000" json:"batch_limit"`
	CriteriaJSON               string     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	PreviewJSON                string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	ResultJSON                 string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	LastError                  string     `gorm:"size:500;not null;default:''" json:"last_error,omitempty"`
	StartedAt                  *time.Time `json:"started_at,omitempty"`
	CompletedAt                *time.Time `json:"completed_at,omitempty"`
	SoftRestoredAt             *time.Time `json:"soft_restored_at,omitempty"`
	FinancialRestoredAt        *time.Time `json:"financial_restored_at,omitempty"`
	RestoredByID               uint64     `gorm:"not null;default:0" json:"restored_by_id,omitempty"`
	SoftRestoredByID           uint64     `gorm:"not null;default:0" json:"soft_restored_by_id,omitempty"`
	SoftRestoredByName         string     `gorm:"size:80;not null;default:''" json:"soft_restored_by_name,omitempty"`
	FinancialRestoredByID      uint64     `gorm:"not null;default:0" json:"financial_restored_by_id,omitempty"`
	FinancialRestoredByName    string     `gorm:"size:80;not null;default:''" json:"financial_restored_by_name,omitempty"`
	ContentPurgedAt            *time.Time `json:"content_purged_at,omitempty"`
	ContentPurgeCount          int64      `gorm:"not null;default:0" json:"content_purge_count"`
	LastContentPurgeRequestID  string     `gorm:"size:96;not null;default:''" json:"last_content_purge_request_id,omitempty"`
	SoftRestoreResultJSON      string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	FinancialRestoreResultJSON string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

func (CleanupRun) TableName() string { return "data_cleanup_runs" }
