package services

import (
	"backend/data/models/lifecycle"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultCleanupBatch = 5000
	maxCleanupBatch     = 20000
	DeleteModeSoft      = "soft"
	DeleteModeHard      = "hard"
)

var cleanupRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,95}$`)

type lifecycleSpec struct {
	Action      string
	DefaultDays int
	MinimumDays int
	Description string
}

var lifecycleSpecs = map[string]lifecycleSpec{
	lifecycle.ClassChatMessages: {
		Action: lifecycle.ActionSoftDelete, DefaultDays: 180, MinimumDays: 1,
		Description: "真人及业务聊天到期后软删除；客服欢迎语、机器人普通聊天、红包账务和领取记录不处理。",
	},
	lifecycle.ClassRobotChatMessages: {
		Action: lifecycle.ActionSoftDelete, DefaultDays: 30, MinimumDays: 1,
		Description: "仅软删除独立机器人账号发送的普通聊天；真人消息、红包和账务记录不受影响，可按任务恢复。",
	},
	lifecycle.ClassNotifications: {
		Action: lifecycle.ActionSoftDelete, DefaultDays: 180, MinimumDays: 1,
		Description: "会员与后台通知到期后软删除，不删除其关联的注单和余额流水。",
	},
	lifecycle.ClassAuditLogs: {
		Action: lifecycle.ActionArchiveThenPurgeHot, DefaultDays: 730, MinimumDays: 365,
		Description: "审计日志先完整归档，再从热表移除；归档记录不可编辑。",
	},
	lifecycle.ClassRobotTestData: {
		Action: lifecycle.ActionColdArchive, DefaultDays: 90, MinimumDays: 1,
		Description: "已结算且对账正常的机器人注单和安全账务前缀先校验归档，再移出热表；真人、待开奖和异常数据禁止移动。",
	},
}

var protectedFinancialTables = map[string]struct{}{
	"lottery_bets":              {},
	"user_balance_transactions": {},
}

// Hard deletion is deliberately narrower than the retention registry. Only
// recoverable, non-financial content may be permanently removed, and only
// after it has already been soft deleted. Audit, bet, balance, red-packet and
// application evidence never enters this allow-list.
var hardDeleteDataClasses = map[string]struct{}{
	lifecycle.ClassChatMessages:      {},
	lifecycle.ClassRobotChatMessages: {},
	lifecycle.ClassNotifications:     {},
}

// Generic chat retention excludes durable service greetings and ordinary
// robot chatter. Greetings must remain the first persisted service message;
// robot chatter has its own independently controlled retention policy.
const genericChatLifecyclePredicate = `
	message.message_type = 'text'
	AND message.reference_id = 0
	AND NOT EXISTS (
		SELECT 1 FROM workspace_robot_profiles robot
		WHERE robot.user_id = message.user_id
		  AND robot.workspace_id = message.workspace_id
	)`

// Member notifications can be user-facing financial/business receipts. Only
// plain system/activity notices without frozen settlement fields are eligible
// for retention or permanent purge. Admin notices do not carry those fields.
const disposableMemberNotificationPredicate = `
	notice.event_key = ''
	AND notice.game_id = ''
	AND notice.issue = ''
	AND notice.stake_cents = 0
	AND notice.payout_cents = 0
	AND notice.bet_count = 0
	AND notice.won_count = 0
	AND notice.category IN ('system', 'activity')`

// Admin work-queue reminders point at applications, pending bets or other
// business records. They stay as durable operational evidence even after a
// user hides them. Only unlinked informational notices may enter retention.
const disposableAdminNotificationPredicate = `
	notice.link = ''
	AND notice.title NOT IN ('待审核申请', '待结算注单')`

type LifecycleActor struct {
	UserID      uint64
	Username    string
	WorkspaceID uint64
}

type PolicyView struct {
	lifecycle.RetentionPolicy
	Inherited   bool   `json:"inherited"`
	Description string `json:"description"`
}

type UpdateRetentionPolicyInput struct {
	WorkspaceID   uint64 `json:"workspace_id"`
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days"`
}

type CleanupPreviewInput struct {
	RequestID     string   `json:"request_id"`
	WorkspaceID   *uint64  `json:"workspace_id,omitempty"`
	AllWorkspaces bool     `json:"all_workspaces"`
	DataClasses   []string `json:"data_classes,omitempty"`
	BatchLimit    int      `json:"batch_limit,omitempty"`
	DeleteMode    string   `json:"delete_mode,omitempty"`
}

type CleanupExecuteInput struct {
	RequestID string `json:"request_id"`
}

type CleanupPreviewItem struct {
	DataClass      string    `json:"data_class"`
	Action         string    `json:"action"`
	Description    string    `json:"description"`
	Enabled        bool      `json:"enabled"`
	RetentionDays  int       `json:"retention_days"`
	CutoffAt       time.Time `json:"cutoff_at"`
	EligibleCount  int64     `json:"eligible_count"`
	PlannedCount   int64     `json:"planned_count"`
	ProtectedCount int64     `json:"protected_from_deletion"`
	// CandidateFingerprint freezes the exact first batch selected by preview.
	// Execute refuses a stale or legacy preview instead of processing a
	// different row set that happens to have the same count.
	CandidateFingerprint string `json:"candidate_fingerprint,omitempty"`
}

type CleanupPreview struct {
	RequestID     string               `json:"request_id"`
	WorkspaceID   uint64               `json:"workspace_id"`
	AllWorkspaces bool                 `json:"all_workspaces"`
	BatchLimit    int                  `json:"batch_limit"`
	DeleteMode    string               `json:"delete_mode"`
	Status        string               `json:"status"`
	Items         []CleanupPreviewItem `json:"items"`
	CreatedAt     time.Time            `json:"created_at"`
}

type CleanupResultItem struct {
	DataClass     string `json:"data_class"`
	Action        string `json:"action"`
	AffectedCount int64  `json:"affected_count"`
	Note          string `json:"note,omitempty"`
}

type CleanupExecution struct {
	RequestID     string              `json:"request_id"`
	WorkspaceID   uint64              `json:"workspace_id"`
	AllWorkspaces bool                `json:"all_workspaces"`
	DeleteMode    string              `json:"delete_mode"`
	Status        string              `json:"status"`
	Items         []CleanupResultItem `json:"items"`
	CompletedAt   *time.Time          `json:"completed_at,omitempty"`
}

type CleanupRunView struct {
	ID                        uint64               `json:"id"`
	RequestID                 string               `json:"request_id"`
	WorkspaceID               uint64               `json:"workspace_id"`
	AllWorkspaces             bool                 `json:"all_workspaces"`
	DeleteMode                string               `json:"delete_mode"`
	ActorID                   uint64               `json:"actor_id"`
	ActorName                 string               `json:"actor_name"`
	ExecutedByID              uint64               `json:"executed_by_id,omitempty"`
	ExecutedByName            string               `json:"executed_by_name,omitempty"`
	Status                    string               `json:"status"`
	BatchLimit                int                  `json:"batch_limit"`
	Preview                   []CleanupPreviewItem `json:"preview"`
	Result                    []CleanupResultItem  `json:"result"`
	SoftRestoreResult         []CleanupResultItem  `json:"soft_restore_result"`
	FinancialRestoreResult    []CleanupResultItem  `json:"financial_restore_result"`
	LastError                 string               `json:"last_error,omitempty"`
	StartedAt                 *time.Time           `json:"started_at,omitempty"`
	CompletedAt               *time.Time           `json:"completed_at,omitempty"`
	SoftRestoredAt            *time.Time           `json:"soft_restored_at,omitempty"`
	FinancialRestoredAt       *time.Time           `json:"financial_restored_at,omitempty"`
	SoftRestoredByID          uint64               `json:"soft_restored_by_id,omitempty"`
	SoftRestoredByName        string               `json:"soft_restored_by_name,omitempty"`
	FinancialRestoredByID     uint64               `json:"financial_restored_by_id,omitempty"`
	FinancialRestoredByName   string               `json:"financial_restored_by_name,omitempty"`
	ContentPurgedAt           *time.Time           `json:"content_purged_at,omitempty"`
	ContentPurgeCount         int64                `json:"content_purge_count"`
	LastContentPurgeRequestID string               `json:"last_content_purge_request_id,omitempty"`
	CreatedAt                 time.Time            `json:"created_at"`
}

type CleanupRunPage struct {
	Items        []CleanupRunView `json:"items"`
	HasMore      bool             `json:"has_more"`
	NextBeforeID uint64           `json:"next_before_id,omitempty"`
}

type LifecycleRestoreResult struct {
	RequestID     string              `json:"request_id"`
	WorkspaceID   uint64              `json:"workspace_id"`
	AllWorkspaces bool                `json:"all_workspaces"`
	Kind          string              `json:"kind"`
	Items         []CleanupResultItem `json:"items"`
	RestoredAt    *time.Time          `json:"restored_at,omitempty"`
}

type LifecycleArchiveRecord struct {
	ID          uint64     `json:"id"`
	WorkspaceID uint64     `json:"workspace_id"`
	UserID      uint64     `json:"user_id"`
	Kind        string     `json:"kind"`
	GameID      string     `json:"game_id,omitempty"`
	Issue       string     `json:"issue,omitempty"`
	Status      string     `json:"status,omitempty"`
	Reference   string     `json:"reference,omitempty"`
	Type        string     `json:"type,omitempty"`
	AmountCents int64      `json:"amount_cents"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	ArchivedAt  time.Time  `json:"archived_at"`
	RowHash     string     `json:"row_hash"`
}

type LifecycleArchivePage struct {
	Items        []LifecycleArchiveRecord `json:"items"`
	HasMore      bool                     `json:"has_more"`
	NextBeforeID uint64                   `json:"next_before_id,omitempty"`
}

type normalizedCleanupCriteria struct {
	WorkspaceID   uint64   `json:"workspace_id"`
	AllWorkspaces bool     `json:"all_workspaces"`
	DataClasses   []string `json:"data_classes"`
	BatchLimit    int      `json:"batch_limit"`
	DeleteMode    string   `json:"delete_mode"`
}

type MaintenanceSummary struct {
	SoftDeletedChatCount         int64     `json:"soft_deleted_chat_count"`
	SoftDeletedRobotChatCount    int64     `json:"soft_deleted_robot_chat_count"`
	SoftDeletedNotificationCount int64     `json:"soft_deleted_notification_count"`
	StaleIdempotencyCount        int64     `json:"stale_idempotency_count"`
	DeliveredSessionReceiptCount int64     `json:"delivered_session_receipt_count"`
	OrphanChatCursorCount        int64     `json:"orphan_chat_cursor_count"`
	ProtectedBetCount            int64     `json:"protected_bet_count"`
	ProtectedLedgerCount         int64     `json:"protected_ledger_count"`
	ProtectedAuditCount          int64     `json:"protected_audit_count"`
	GeneratedAt                  time.Time `json:"generated_at"`
}

type DataLifecycleService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewDataLifecycleService(db *gorm.DB) *DataLifecycleService {
	return &DataLifecycleService{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// EnsurePlatformAdmin prevents an admin-shaped account in a tenant workspace
// from invoking lifecycle operations. Authorization comes only from the
// authenticated workspace, never from a target workspace in the request.
func (s *DataLifecycleService) EnsurePlatformAdmin(actor LifecycleActor) error {
	if actor.UserID == 0 || actor.WorkspaceID == 0 {
		return apperrors.NewBusinessError("FORBIDDEN", "仅平台管理员可以管理数据生命周期")
	}
	var count int64
	err := s.db.Model(&workspacemodel.Workspace{}).
		Where("id = ? AND type = ? AND status = 1", actor.WorkspaceID, workspacemodel.TypePlatform).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return apperrors.NewBusinessError("FORBIDDEN", "仅平台管理员可以管理数据生命周期")
	}
	return nil
}

func (s *DataLifecycleService) Policies(workspaceID uint64) ([]PolicyView, error) {
	if workspaceID > 0 {
		if err := s.ensureWorkspace(workspaceID); err != nil {
			return nil, err
		}
	}
	items := make([]PolicyView, 0, len(lifecycleSpecs))
	for _, dataClass := range allLifecycleClasses() {
		policy, inherited, err := s.policyForWorkspace(workspaceID, dataClass)
		if err != nil {
			return nil, err
		}
		items = append(items, PolicyView{RetentionPolicy: policy, Inherited: inherited, Description: lifecycleSpecs[dataClass].Description})
	}
	return items, nil
}

// Summary exposes useful, read-only maintenance signals in one place. It does
// not broaden deletion permissions: financial and audit counts are displayed
// only to make the protected boundary visible to operators.
func (s *DataLifecycleService) Summary(actor LifecycleActor) (*MaintenanceSummary, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	result := &MaintenanceSummary{GeneratedAt: s.now()}
	checks := []struct {
		query  string
		args   []any
		target *int64
	}{
		{query: `SELECT COUNT(*) FROM member_chat_messages message WHERE message.deleted_at IS NOT NULL AND ` + genericChatLifecyclePredicate, target: &result.SoftDeletedChatCount},
		{query: `SELECT COUNT(*) FROM member_chat_messages message JOIN workspace_robot_profiles robot ON robot.user_id = message.user_id AND robot.workspace_id = message.workspace_id WHERE message.deleted_at IS NOT NULL AND message.message_type = 'text' AND message.reference_id = 0`, target: &result.SoftDeletedRobotChatCount},
		{query: `SELECT (SELECT COUNT(*) FROM member_notifications notice WHERE notice.deleted_at IS NOT NULL AND ` + disposableMemberNotificationPredicate + `) + (SELECT COUNT(*) FROM admin_notifications notice WHERE notice.deleted_at IS NOT NULL AND ` + disposableAdminNotificationPredicate + `)`, target: &result.SoftDeletedNotificationCount},
		{query: `SELECT (SELECT COUNT(*) FROM lottery_bet_requests WHERE status = 'processing' AND updated_at <= ?) + (SELECT COUNT(*) FROM lottery_assistant_requests WHERE status = 'processing' AND updated_at <= ?)`, args: []any{result.GeneratedAt.Add(-idempotencyReservationTimeout), result.GeneratedAt.Add(-idempotencyReservationTimeout)}, target: &result.StaleIdempotencyCount},
		{query: `SELECT COUNT(*) FROM ws_session_revocation_outbox WHERE delivered_at IS NOT NULL AND delivered_at < ?`, args: []any{result.GeneratedAt.AddDate(0, 0, -30)}, target: &result.DeliveredSessionReceiptCount},
		{query: `SELECT COUNT(*) FROM member_chat_read_cursors cursor_row LEFT JOIN "user" account ON account.user_id = cursor_row.operator_user_id LEFT JOIN workspaces workspace ON workspace.id = cursor_row.workspace_id WHERE account.user_id IS NULL OR workspace.id IS NULL`, target: &result.OrphanChatCursorCount},
		{query: `SELECT COUNT(*) FROM lottery_bets`, target: &result.ProtectedBetCount},
		{query: `SELECT COUNT(*) FROM user_balance_transactions`, target: &result.ProtectedLedgerCount},
		{query: `SELECT COUNT(*) FROM admin_audit_logs`, target: &result.ProtectedAuditCount},
	}
	for _, check := range checks {
		if err := s.db.Raw(check.query, check.args...).Scan(check.target).Error; err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *DataLifecycleService) UpdatePolicy(dataClass string, input UpdateRetentionPolicyInput, actor LifecycleActor) (*PolicyView, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	spec, ok := lifecycleSpecs[strings.TrimSpace(dataClass)]
	if !ok {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "不支持的数据类型")
	}
	if input.RetentionDays < spec.MinimumDays || input.RetentionDays > 3650 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("保留天数应为 %d–3650 天", spec.MinimumDays))
	}
	if input.WorkspaceID > 0 {
		if err := s.ensureWorkspace(input.WorkspaceID); err != nil {
			return nil, err
		}
	}
	now := s.now()
	policy := lifecycle.RetentionPolicy{
		WorkspaceID: input.WorkspaceID, DataClass: dataClass, Enabled: input.Enabled,
		RetentionDays: input.RetentionDays, Action: spec.Action,
		UpdatedByID: actor.UserID, UpdatedByName: actor.Username, CreatedAt: now, UpdatedAt: now,
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}, {Name: "data_class"}},
		DoUpdates: clause.Assignments(map[string]any{
			"enabled": input.Enabled, "retention_days": input.RetentionDays, "action": spec.Action,
			"updated_by_id": actor.UserID, "updated_by_name": actor.Username, "updated_at": now,
		}),
	}).Create(&policy).Error
	if err != nil {
		return nil, err
	}
	if err := s.db.Where("workspace_id = ? AND data_class = ?", input.WorkspaceID, dataClass).First(&policy).Error; err != nil {
		return nil, err
	}
	return &PolicyView{RetentionPolicy: policy, Inherited: false, Description: spec.Description}, nil
}

func (s *DataLifecycleService) Preview(input CleanupPreviewInput, actor LifecycleActor) (*CleanupPreview, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	criteria, requestID, err := s.normalizePreviewInput(input)
	if err != nil {
		return nil, err
	}
	if !criteria.AllWorkspaces {
		if err := s.ensureWorkspace(criteria.WorkspaceID); err != nil {
			return nil, err
		}
	}
	criteriaJSON, _ := json.Marshal(criteria)

	var existing lifecycle.CleanupRun
	err = s.db.Where("request_id = ?", requestID).First(&existing).Error
	if err == nil {
		if existing.CriteriaJSON != string(criteriaJSON) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 已被其他清理条件使用")
		}
		return previewFromRun(existing)
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	items := make([]CleanupPreviewItem, 0, len(criteria.DataClasses))
	for _, dataClass := range criteria.DataClasses {
		policyWorkspace := criteria.WorkspaceID
		if criteria.AllWorkspaces {
			policyWorkspace = 0
		}
		policy, _, err := s.policyForWorkspace(policyWorkspace, dataClass)
		if err != nil {
			return nil, err
		}
		item := CleanupPreviewItem{
			DataClass: dataClass, Action: policy.Action, Description: lifecycleSpecs[dataClass].Description,
			Enabled: policy.Enabled, RetentionDays: policy.RetentionDays,
			CutoffAt: s.now().AddDate(0, 0, -policy.RetentionDays),
		}
		if criteria.DeleteMode == DeleteModeHard {
			item.Action = lifecycle.ActionHardDelete
			item.Description = "永久清除已软删除且超过保留期的非账务内容；操作不可恢复。"
		}
		if policy.Enabled {
			count, err := s.countCandidates(criteria, item)
			if err != nil {
				return nil, err
			}
			item.EligibleCount = count
			item.PlannedCount = minInt64(count, int64(criteria.BatchLimit))
			if item.PlannedCount > 0 {
				fingerprint, err := s.candidateFingerprint(criteria, item, int(item.PlannedCount))
				if err != nil {
					return nil, err
				}
				item.CandidateFingerprint = fingerprint
			}
			if dataClass == lifecycle.ClassRobotTestData {
				protected, err := s.countProtectedRobotFinancialRows(criteria, item.CutoffAt)
				if err != nil {
					return nil, err
				}
				item.ProtectedCount = protected
			}
		}
		items = append(items, item)
	}
	previewJSON, _ := json.Marshal(items)
	run := lifecycle.CleanupRun{
		RequestID: requestID, WorkspaceID: criteria.WorkspaceID, AllWorkspaces: criteria.AllWorkspaces,
		ActorID: actor.UserID, ActorName: actor.Username, Status: "previewed", BatchLimit: criteria.BatchLimit,
		CriteriaJSON: string(criteriaJSON), PreviewJSON: string(previewJSON), ResultJSON: "[]",
	}
	if err := s.db.Create(&run).Error; err != nil {
		// A concurrent identical preview may have reserved the request first.
		if queryErr := s.db.Where("request_id = ?", requestID).First(&existing).Error; queryErr == nil {
			if existing.CriteriaJSON != string(criteriaJSON) {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 已被其他清理条件使用")
			}
			return previewFromRun(existing)
		}
		return nil, err
	}
	return previewFromRun(run)
}

func (s *DataLifecycleService) Execute(input CleanupExecuteInput, actor LifecycleActor) (*CleanupExecution, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	requestID := strings.TrimSpace(input.RequestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 格式不正确")
	}

	var output *CleanupExecution
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// One global maintenance lock protects the frozen preview across every
		// data class. Without it, two valid previews could consume each other's
		// batches while both still report success.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(729421118)).Error; err != nil {
			return err
		}
		var run lifecycle.CleanupRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestID).First(&run).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "请先预览本次清理")
			}
			return err
		}
		if run.Status == "completed" {
			var items []CleanupResultItem
			_ = json.Unmarshal([]byte(run.ResultJSON), &items)
			var criteria normalizedCleanupCriteria
			_ = json.Unmarshal([]byte(run.CriteriaJSON), &criteria)
			output = &CleanupExecution{RequestID: run.RequestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, DeleteMode: normalizeDeleteMode(criteria.DeleteMode), Status: run.Status, Items: items, CompletedAt: run.CompletedAt}
			return nil
		}
		if run.Status != "previewed" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "本次清理已经执行或失败，请使用新的 request_id 重新预览")
		}

		var criteria normalizedCleanupCriteria
		var preview []CleanupPreviewItem
		if err := json.Unmarshal([]byte(run.CriteriaJSON), &criteria); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(run.PreviewJSON), &preview); err != nil {
			return err
		}
		if err := s.validateFrozenPreview(tx, criteria, preview); err != nil {
			return err
		}
		deleteMode := normalizeDeleteMode(criteria.DeleteMode)
		if deleteMode == DeleteModeHard {
			// The protected-table trigger recognizes this transaction-local flag
			// only for already-soft-deleted non-financial content. It is separate
			// from the cold-archive flag used by financial and audit evidence.
			if err := allowLifecycleContentPurge(tx); err != nil {
				return err
			}
		} else if cleanupIncludesArchiveClass(criteria.DataClasses) {
			// The financial/audit archive guard is granted only to a soft-mode
			// task that actually includes a verified archive class. Hard-mode
			// transactions never receive both deletion capabilities.
			if err := allowLifecycleDeletes(tx); err != nil {
				return err
			}
		}
		started := s.now()
		if err := tx.Model(&run).Updates(map[string]any{
			"status": "running", "started_at": started, "updated_at": started,
			"executed_by_id": actor.UserID, "executed_by_name": actor.Username,
		}).Error; err != nil {
			return err
		}
		results := make([]CleanupResultItem, 0, len(preview))
		for _, item := range preview {
			if !item.Enabled {
				results = append(results, CleanupResultItem{DataClass: item.DataClass, Action: item.Action, Note: "策略未启用，未处理任何数据"})
				continue
			}
			affected, err := s.executeClass(tx, criteria, requestID, actor.Username, item)
			if err != nil {
				return err
			}
			result := CleanupResultItem{DataClass: item.DataClass, Action: item.Action, AffectedCount: affected}
			if item.DataClass == lifecycle.ClassRobotTestData {
				result.Note = "已完整复制到冷归档表并校验哈希；待开奖、异常和真人数据未移动"
			}
			results = append(results, result)
		}
		if deleteMode == DeleteModeHard {
			if err := s.recordContentPurgeSources(tx, requestID); err != nil {
				return err
			}
		}
		completed := s.now()
		encoded, _ := json.Marshal(results)
		if err := tx.Model(&run).Updates(map[string]any{
			"status": "completed", "result_json": string(encoded), "completed_at": completed,
			"last_error": "", "updated_at": completed,
		}).Error; err != nil {
			return err
		}
		output = &CleanupExecution{RequestID: run.RequestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, DeleteMode: normalizeDeleteMode(criteria.DeleteMode), Status: "completed", Items: results, CompletedAt: &completed}
		return nil
	})
	if err != nil {
		// The cleanup transaction has rolled back, so recording the failure here
		// cannot leave partially deleted content marked as completed.
		failedAt := s.now()
		_ = s.db.Model(&lifecycle.CleanupRun{}).Where("request_id = ? AND status = ?", requestID, "previewed").Updates(map[string]any{
			"status": "failed", "last_error": truncateLifecycleError(err.Error()),
			"started_at": failedAt, "updated_at": failedAt,
			"executed_by_id": actor.UserID, "executed_by_name": actor.Username,
		}).Error
		return nil, err
	}
	return output, nil
}

func (s *DataLifecycleService) Runs(beforeID uint64, limit int, workspaceID *uint64) (*CleanupRunPage, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	query := s.db.Model(&lifecycle.CleanupRun{}).Order("id DESC").Limit(limit + 1)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	if workspaceID != nil {
		if *workspaceID == 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "workspace_id 必须大于 0")
		}
		if err := s.ensureWorkspace(*workspaceID); err != nil {
			return nil, err
		}
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	var rows []lifecycle.CleanupRun
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]CleanupRunView, 0, len(rows))
	for _, row := range rows {
		view, err := cleanupRunView(row)
		if err != nil {
			return nil, err
		}
		items = append(items, view)
	}
	next := uint64(0)
	if hasMore && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return &CleanupRunPage{Items: items, HasMore: hasMore, NextBeforeID: next}, nil
}

func (s *DataLifecycleService) Run(requestID string) (*CleanupRunView, error) {
	requestID = strings.TrimSpace(requestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 格式不正确")
	}
	var row lifecycle.CleanupRun
	if err := s.db.Where("request_id = ?", requestID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "数据生命周期任务不存在")
		}
		return nil, err
	}
	view, err := cleanupRunView(row)
	return &view, err
}

func (s *DataLifecycleService) RestoreSoftDeleted(requestID string, actor LifecycleActor) (*LifecycleRestoreResult, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 格式不正确")
	}
	var output *LifecycleRestoreResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(729421118)).Error; err != nil {
			return err
		}
		var run lifecycle.CleanupRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestID).First(&run).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "数据生命周期任务不存在")
			}
			return err
		}
		if run.Status != "completed" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "只有已完成的清理任务可以恢复")
		}
		if run.SoftRestoredAt != nil {
			var items []CleanupResultItem
			_ = json.Unmarshal([]byte(run.SoftRestoreResultJSON), &items)
			output = &LifecycleRestoreResult{RequestID: requestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, Kind: "soft_deleted", Items: items, RestoredAt: run.SoftRestoredAt}
			return nil
		}
		var criteria normalizedCleanupCriteria
		if err := json.Unmarshal([]byte(run.CriteriaJSON), &criteria); err != nil {
			return err
		}
		if normalizeDeleteMode(criteria.DeleteMode) != DeleteModeSoft {
			return apperrors.NewBusinessError("INVALID_REQUEST", "硬删除任务不可恢复")
		}
		if run.ContentPurgeCount > 0 || run.ContentPurgedAt != nil {
			return apperrors.NewBusinessError("PERMANENTLY_PURGED", "该任务已有数据被永久删除，不能再执行恢复")
		}
		expected, err := expectedSoftRestoreCount(run.ResultJSON)
		if err != nil {
			return err
		}
		if expected <= 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该任务没有可恢复的软删除数据")
		}
		scopeSQL, scopeArgs := lifecycleScope(criteria, "workspace_id")
		available, err := countSoftDeletedRowsForRequest(tx, requestID, scopeSQL, scopeArgs)
		if err != nil {
			return err
		}
		if available != expected {
			return apperrors.NewBusinessError(
				"RESTORE_INCOMPLETE",
				fmt.Sprintf("回收站数据不完整：任务记录 %d 条，当前可恢复 %d 条；未写入恢复回执", expected, available),
			)
		}
		items := make([]CleanupResultItem, 0, 2)
		for _, target := range []struct {
			DataClass string
			Table     string
		}{
			{DataClass: lifecycle.ClassChatMessages, Table: "member_chat_messages"},
			{DataClass: lifecycle.ClassNotifications, Table: "member_notifications"},
			{DataClass: lifecycle.ClassNotifications, Table: "admin_notifications"},
		} {
			args := []any{requestID}
			args = append(args, scopeArgs...)
			result := tx.Exec(`UPDATE `+target.Table+` SET deleted_at = NULL, deleted_by = '', cleanup_request_id = '' WHERE cleanup_request_id = ? AND deleted_at IS NOT NULL AND `+scopeSQL, args...)
			if result.Error != nil {
				return result.Error
			}
			index := -1
			for i := range items {
				if items[i].DataClass == target.DataClass {
					index = i
					break
				}
			}
			if index < 0 {
				items = append(items, CleanupResultItem{DataClass: target.DataClass, Action: "restore_soft_delete", AffectedCount: result.RowsAffected})
			} else {
				items[index].AffectedCount += result.RowsAffected
			}
		}
		now := s.now()
		var restored int64
		for _, item := range items {
			restored += item.AffectedCount
		}
		if restored != expected {
			return apperrors.NewBusinessError(
				"RESTORE_INCOMPLETE",
				fmt.Sprintf("恢复数量校验失败：应恢复 %d 条，实际 %d 条；本次操作已回滚", expected, restored),
			)
		}
		encoded, _ := json.Marshal(items)
		if err := tx.Model(&run).Updates(map[string]any{
			"soft_restored_at": now, "soft_restore_result_json": string(encoded),
			"soft_restored_by_id": actor.UserID, "soft_restored_by_name": actor.Username,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		output = &LifecycleRestoreResult{RequestID: requestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, Kind: "soft_deleted", Items: items, RestoredAt: &now}
		return nil
	})
	return output, err
}

func expectedSoftRestoreCount(resultJSON string) (int64, error) {
	var items []CleanupResultItem
	if err := json.Unmarshal([]byte(defaultString(strings.TrimSpace(resultJSON), "[]")), &items); err != nil {
		return 0, err
	}
	var expected int64
	for _, item := range items {
		switch item.DataClass {
		case lifecycle.ClassChatMessages, lifecycle.ClassRobotChatMessages, lifecycle.ClassNotifications:
			expected += item.AffectedCount
		}
	}
	return expected, nil
}

func countSoftDeletedRowsForRequest(tx *gorm.DB, requestID, scopeSQL string, scopeArgs []any) (int64, error) {
	var total int64
	for _, table := range []string{"member_chat_messages", "member_notifications", "admin_notifications"} {
		args := []any{requestID}
		args = append(args, scopeArgs...)
		var count int64
		if err := tx.Raw(`SELECT COUNT(*) FROM `+table+` WHERE cleanup_request_id = ? AND deleted_at IS NOT NULL AND `+scopeSQL, args...).Scan(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *DataLifecycleService) RestoreRobotArchive(requestID string, actor LifecycleActor) (*LifecycleRestoreResult, error) {
	if err := s.EnsurePlatformAdmin(actor); err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 格式不正确")
	}
	var output *LifecycleRestoreResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(729421118)).Error; err != nil {
			return err
		}
		if err := allowLifecycleDeletes(tx); err != nil {
			return err
		}
		var run lifecycle.CleanupRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestID).First(&run).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "数据生命周期任务不存在")
			}
			return err
		}
		if run.Status != "completed" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "只有已完成的清理任务可以恢复")
		}
		if run.FinancialRestoredAt != nil {
			var items []CleanupResultItem
			_ = json.Unmarshal([]byte(run.FinancialRestoreResultJSON), &items)
			output = &LifecycleRestoreResult{RequestID: requestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, Kind: "robot_archive", Items: items, RestoredAt: run.FinancialRestoredAt}
			return nil
		}

		bets, err := restoreRobotBetArchive(tx, requestID)
		if err != nil {
			return err
		}
		ledger, err := restoreRobotLedgerArchive(tx, requestID)
		if err != nil {
			return err
		}
		items := []CleanupResultItem{
			{DataClass: lifecycle.ClassRobotTestData, Action: "restore_lottery_bets", AffectedCount: bets},
			{DataClass: lifecycle.ClassRobotTestData, Action: "restore_balance_transactions", AffectedCount: ledger},
		}
		now := s.now()
		encoded, _ := json.Marshal(items)
		if err := tx.Model(&run).Updates(map[string]any{
			"financial_restored_at": now, "financial_restore_result_json": string(encoded),
			"financial_restored_by_id": actor.UserID, "financial_restored_by_name": actor.Username,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		output = &LifecycleRestoreResult{RequestID: requestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces, Kind: "robot_archive", Items: items, RestoredAt: &now}
		return nil
	})
	return output, err
}

func allowLifecycleDeletes(tx *gorm.DB) error {
	return tx.Exec(`SELECT set_config('wangzhe.lifecycle_delete', 'on', true)`).Error
}

func allowLifecycleContentPurge(tx *gorm.DB) error {
	return tx.Exec(`SELECT set_config('wangzhe.lifecycle_content_purge', 'on', true)`).Error
}

func cleanupIncludesArchiveClass(dataClasses []string) bool {
	for _, dataClass := range dataClasses {
		if dataClass == lifecycle.ClassAuditLogs || dataClass == lifecycle.ClassRobotTestData {
			return true
		}
	}
	return false
}

func (s *DataLifecycleService) Archives(requestID, kind string, beforeID uint64, limit int) (*LifecycleArchivePage, error) {
	requestID = strings.TrimSpace(requestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "request_id 格式不正确")
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "bets"
	}
	var query string
	args := []any{requestID}
	if beforeID > 0 {
		args = append(args, beforeID)
	}
	args = append(args, limit+1)
	before := ""
	if beforeID > 0 {
		before = " AND id < ?"
	}
	switch kind {
	case "bets":
		query = `SELECT id, workspace_id, user_id, 'bet' AS kind, game_id, issue, status, '' AS reference, '' AS type, amount_cents, created_at, archived_at, row_hash FROM lottery_bet_archives WHERE cleanup_request_id = ?` + before + ` ORDER BY id DESC LIMIT ?`
	case "ledger":
		query = `SELECT id, workspace_id, user_id, 'ledger' AS kind, '' AS game_id, '' AS issue, '' AS status, reference, type, amount_cents, created_at, archived_at, row_hash FROM user_balance_transaction_archives WHERE cleanup_request_id = ?` + before + ` ORDER BY id DESC LIMIT ?`
	case "audit":
		auditBefore := ""
		if beforeID > 0 {
			auditBefore = " AND source_id < ?"
		}
		query = `SELECT source_id AS id, workspace_id, actor_id AS user_id, 'audit' AS kind,
			'' AS game_id, '' AS issue, status_code::text AS status,
			method || ' ' || path AS reference, actor_role AS type, 0::bigint AS amount_cents,
			source_created_at AS created_at, archived_at, '' AS row_hash
			FROM admin_audit_log_archives WHERE cleanup_request_id = ?` + auditBefore + ` ORDER BY source_id DESC LIMIT ?`
	default:
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "归档类型只支持 bets、ledger 或 audit")
	}
	var rows []LifecycleArchiveRecord
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	next := uint64(0)
	if hasMore && len(rows) > 0 {
		next = rows[len(rows)-1].ID
	}
	return &LifecycleArchivePage{Items: rows, HasMore: hasMore, NextBeforeID: next}, nil
}

func (s *DataLifecycleService) normalizePreviewInput(input CleanupPreviewInput) (normalizedCleanupCriteria, string, error) {
	requestID := strings.TrimSpace(input.RequestID)
	if !cleanupRequestIDPattern.MatchString(requestID) {
		return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "request_id 需要 8–96 位字母、数字或 . _ : -")
	}
	if input.AllWorkspaces {
		if input.WorkspaceID != nil && *input.WorkspaceID != 0 {
			return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "全部房间清理不能同时指定 workspace_id")
		}
	} else if input.WorkspaceID == nil || *input.WorkspaceID == 0 {
		return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "请选择一个工作区，或明确选择全部工作区")
	}
	batch := input.BatchLimit
	if batch == 0 {
		batch = defaultCleanupBatch
	}
	if batch < 1 || batch > maxCleanupBatch {
		return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "单次处理数量应为 1–20000")
	}
	classes := input.DataClasses
	if len(classes) == 0 {
		classes = allLifecycleClasses()
	}
	unique := make(map[string]struct{}, len(classes))
	normalized := make([]string, 0, len(classes))
	for _, value := range classes {
		value = strings.TrimSpace(value)
		if _, ok := lifecycleSpecs[value]; !ok {
			return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "包含不支持的数据类型")
		}
		if _, exists := unique[value]; exists {
			continue
		}
		unique[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	deleteMode := normalizeDeleteMode(input.DeleteMode)
	if deleteMode != DeleteModeSoft && deleteMode != DeleteModeHard {
		return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "delete_mode 只支持 soft 或 hard")
	}
	if deleteMode == DeleteModeHard {
		for _, dataClass := range normalized {
			if _, allowed := hardDeleteDataClasses[dataClass]; !allowed {
				return normalizedCleanupCriteria{}, "", apperrors.NewBusinessError("INVALID_REQUEST", "硬删除仅允许普通聊天、机器人普通聊天和非账务通知")
			}
		}
	}
	workspaceID := uint64(0)
	if input.WorkspaceID != nil {
		workspaceID = *input.WorkspaceID
	}
	return normalizedCleanupCriteria{WorkspaceID: workspaceID, AllWorkspaces: input.AllWorkspaces, DataClasses: normalized, BatchLimit: batch, DeleteMode: deleteMode}, requestID, nil
}

func normalizeDeleteMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DeleteModeSoft
	}
	return value
}

func (s *DataLifecycleService) policyForWorkspace(workspaceID uint64, dataClass string) (lifecycle.RetentionPolicy, bool, error) {
	var policy lifecycle.RetentionPolicy
	err := s.db.Where("workspace_id = ? AND data_class = ?", workspaceID, dataClass).First(&policy).Error
	if err == nil {
		return policy, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return policy, false, err
	}
	if workspaceID > 0 {
		err = s.db.Where("workspace_id = 0 AND data_class = ?", dataClass).First(&policy).Error
		if err == nil {
			return policy, true, nil
		}
		if err != gorm.ErrRecordNotFound {
			return policy, false, err
		}
	}
	spec := lifecycleSpecs[dataClass]
	return lifecycle.RetentionPolicy{WorkspaceID: 0, DataClass: dataClass, Enabled: false, RetentionDays: spec.DefaultDays, Action: spec.Action}, workspaceID > 0, nil
}

func (s *DataLifecycleService) ensureWorkspace(workspaceID uint64) error {
	var count int64
	if err := s.db.Model(&workspacemodel.Workspace{}).Where("id = ?", workspaceID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.NewBusinessError("NOT_FOUND", "工作区不存在")
	}
	return nil
}

type lifecycleCandidateKey struct {
	Key string `gorm:"column:candidate_key"`
}

func fingerprintCandidateKeys(keys []lifecycleCandidateKey) string {
	hash := sha256.New()
	for _, item := range keys {
		// Length-prefixing prevents two different key sequences from producing
		// the same byte stream before hashing.
		_, _ = fmt.Fprintf(hash, "%d:%s\n", len(item.Key), item.Key)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (s *DataLifecycleService) candidateFingerprint(criteria normalizedCleanupCriteria, item CleanupPreviewItem, limit int) (string, error) {
	return lifecycleCandidateFingerprint(s.db, criteria, item, limit)
}

func lifecycleCandidateFingerprint(db *gorm.DB, criteria normalizedCleanupCriteria, item CleanupPreviewItem, limit int) (string, error) {
	if limit <= 0 {
		return "", nil
	}
	rows := make([]lifecycleCandidateKey, 0, limit)
	switch item.DataClass {
	case lifecycle.ClassChatMessages:
		scopeSQL, scopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := append([]any{item.CutoffAt}, scopeArgs...)
		args = append(args, limit)
		deletedPredicate := "message.deleted_at IS NULL AND message.created_at < ?"
		prefix := "chat:"
		if criteria.DeleteMode == DeleteModeHard {
			deletedPredicate = "message.deleted_at IS NOT NULL AND message.deleted_at < ?"
			prefix = "hard-chat:"
		}
		err := db.Raw(`
			SELECT ? || message.id::text AS candidate_key
			FROM member_chat_messages message
			WHERE `+deletedPredicate+`
			  AND `+genericChatLifecyclePredicate+`
			  AND `+scopeSQL+`
			ORDER BY message.id ASC LIMIT ?
		`, append([]any{prefix}, args...)...).Scan(&rows).Error
		if err != nil {
			return "", err
		}
	case lifecycle.ClassRobotChatMessages:
		scopeSQL, scopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := append([]any{item.CutoffAt}, scopeArgs...)
		args = append(args, limit)
		deletedPredicate := "message.deleted_at IS NULL AND message.created_at < ?"
		prefix := "robot-chat:"
		if criteria.DeleteMode == DeleteModeHard {
			deletedPredicate = "message.deleted_at IS NOT NULL AND message.deleted_at < ?"
			prefix = "hard-robot-chat:"
		}
		err := db.Raw(`
			SELECT ? || message.id::text AS candidate_key
			FROM member_chat_messages message
			JOIN workspace_robot_profiles robot
			  ON robot.user_id = message.user_id
			 AND robot.workspace_id = message.workspace_id
			WHERE `+deletedPredicate+`
			  AND message.message_type = 'text'
			  AND message.reference_id = 0
			  AND `+scopeSQL+`
			ORDER BY message.id ASC LIMIT ?
		`, append([]any{prefix}, args...)...).Scan(&rows).Error
		if err != nil {
			return "", err
		}
	case lifecycle.ClassNotifications:
		memberScope, memberArgs := lifecycleScope(criteria, "notice.workspace_id")
		adminScope, adminArgs := lifecycleScope(criteria, "notice.workspace_id")
		memberDeletedPredicate := "notice.deleted_at IS NULL AND notice.created_at < ?"
		adminDeletedPredicate := "notice.deleted_at IS NULL AND notice.created_at < ?"
		memberPrefix, adminPrefix := "member-notification:", "admin-notification:"
		if criteria.DeleteMode == DeleteModeHard {
			memberDeletedPredicate = "notice.deleted_at IS NOT NULL AND notice.deleted_at < ?"
			adminDeletedPredicate = "notice.deleted_at IS NOT NULL AND notice.deleted_at < ?"
			memberPrefix, adminPrefix = "hard-member-notification:", "hard-admin-notification:"
		}
		args := []any{memberPrefix, item.CutoffAt}
		args = append(args, memberArgs...)
		args = append(args, adminPrefix, item.CutoffAt)
		args = append(args, adminArgs...)
		args = append(args, limit)
		err := db.Raw(`
			SELECT candidate_key FROM (
				SELECT 0 AS source_order, notice.id,
					? || notice.id::text AS candidate_key
				FROM member_notifications notice
				WHERE `+memberDeletedPredicate+` AND `+disposableMemberNotificationPredicate+` AND `+memberScope+`
				UNION ALL
				SELECT 1 AS source_order, notice.id,
					? || notice.id::text AS candidate_key
				FROM admin_notifications notice
				WHERE `+adminDeletedPredicate+` AND `+disposableAdminNotificationPredicate+` AND `+adminScope+`
			) candidates
			ORDER BY source_order ASC, id ASC LIMIT ?
		`, args...).Scan(&rows).Error
		if err != nil {
			return "", err
		}
	case lifecycle.ClassAuditLogs:
		scopeSQL, scopeArgs := lifecycleScope(criteria, "audit_row.workspace_id")
		args := append([]any{item.CutoffAt}, scopeArgs...)
		args = append(args, limit)
		err := db.Raw(`
			SELECT 'audit:' || audit_row.id::text AS candidate_key
			FROM admin_audit_logs audit_row
			WHERE audit_row.created_at < ? AND `+scopeSQL+`
			ORDER BY audit_row.id ASC LIMIT ?
		`, args...).Scan(&rows).Error
		if err != nil {
			return "", err
		}
	case lifecycle.ClassRobotTestData:
		betScope, betArgs := lifecycleScope(criteria, "source.workspace_id")
		ledgerScope, ledgerArgs := lifecycleScope(criteria, "source.workspace_id")
		args := append([]any{item.CutoffAt}, betArgs...)
		args = append(args, item.CutoffAt)
		args = append(args, ledgerArgs...)
		args = append(args, limit)
		err := db.Raw(`
			SELECT candidate_key FROM (
				SELECT 0 AS source_order, source.id,
					'bet:' || source.id::text AS candidate_key
				FROM lottery_bets source
				JOIN workspace_robot_profiles robot
				  ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
				LEFT JOIN lottery_bet_archives archive ON archive.id = source.id
				WHERE archive.id IS NULL
				  AND source.status IN ('won', 'lost')
				  AND source.settled_at IS NOT NULL
				  AND source.reconciliation_status = 'normal'
				  AND source.created_at < ? AND `+betScope+`
				UNION ALL
				SELECT 1 AS source_order, source.id,
					'ledger:' || source.id::text AS candidate_key
				FROM user_balance_transactions source
				JOIN workspace_robot_profiles robot
				  ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
				LEFT JOIN user_balance_transaction_archives archive ON archive.id = source.id
				WHERE archive.id IS NULL
				  AND source.created_at < ?
				  AND NOT EXISTS (
					SELECT 1 FROM lottery_bets unresolved
					WHERE unresolved.workspace_id = source.workspace_id
					  AND unresolved.user_id = source.user_id
					  AND (unresolved.status = 'pending' OR unresolved.reconciliation_status <> 'normal')
				  )
				  AND `+ledgerScope+`
			) candidates
			ORDER BY source_order ASC, id ASC LIMIT ?
		`, args...).Scan(&rows).Error
		if err != nil {
			return "", err
		}
	default:
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "不支持的数据类型")
	}
	if len(rows) != limit {
		return "", apperrors.NewBusinessError("PREVIEW_CHANGED", "候选数据在预览期间发生变化，请重新生成预览")
	}
	return fingerprintCandidateKeys(rows), nil
}

func (s *DataLifecycleService) validateFrozenPreview(tx *gorm.DB, criteria normalizedCleanupCriteria, preview []CleanupPreviewItem) error {
	if criteria.DeleteMode == DeleteModeHard && tx != nil {
		if err := tx.Exec(`
			CREATE TEMP TABLE lifecycle_hard_delete_candidates (
				data_class text NOT NULL,
				source_kind text NOT NULL,
				source_order smallint NOT NULL,
				id bigint NOT NULL,
				cleanup_request_id text NOT NULL,
				candidate_key text NOT NULL,
				PRIMARY KEY (data_class, source_kind, id)
			) ON COMMIT DROP
		`).Error; err != nil {
			return err
		}
	}
	for _, item := range preview {
		if !item.Enabled || item.PlannedCount <= 0 {
			continue
		}
		if item.CandidateFingerprint == "" {
			return apperrors.NewBusinessError("PREVIEW_CHANGED", "该预览来自旧版本，请重新生成后再执行")
		}
		var actual string
		var err error
		if criteria.DeleteMode == DeleteModeHard {
			actual, err = s.materializeHardDeleteCandidates(tx, criteria, item, int(item.PlannedCount))
		} else {
			actual, err = lifecycleCandidateFingerprint(tx, criteria, item, int(item.PlannedCount))
		}
		if err != nil {
			return err
		}
		if actual != item.CandidateFingerprint {
			return apperrors.NewBusinessError("PREVIEW_CHANGED", "候选数据已变化，请重新生成预览后执行")
		}
	}
	return nil
}

// materializeHardDeleteCandidates selects and locks the exact batch whose
// fingerprint is compared with the frozen preview. The later DELETE statements
// consume only this transaction-local table, so a concurrent insert/update can
// never be substituted merely because the resulting row count is unchanged.
func (s *DataLifecycleService) materializeHardDeleteCandidates(tx *gorm.DB, criteria normalizedCleanupCriteria, item CleanupPreviewItem, limit int) (string, error) {
	if tx == nil {
		return "", apperrors.NewBusinessError("PREVIEW_CHANGED", "永久删除需要重新生成预览")
	}
	if limit <= 0 {
		return "", nil
	}
	inserted := int64(0)
	switch item.DataClass {
	case lifecycle.ClassChatMessages:
		scopeSQL, scopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := []any{item.CutoffAt}
		args = append(args, scopeArgs...)
		args = append(args, limit)
		result := tx.Exec(`
			WITH locked AS (
				SELECT message.id, COALESCE(message.cleanup_request_id, '') AS cleanup_request_id
				FROM member_chat_messages message
				WHERE message.deleted_at IS NOT NULL
				  AND message.deleted_at < ?
				  AND `+genericChatLifecyclePredicate+`
				  AND `+scopeSQL+`
				ORDER BY message.id ASC LIMIT ? FOR UPDATE OF message
			)
			INSERT INTO lifecycle_hard_delete_candidates
				(data_class, source_kind, source_order, id, cleanup_request_id, candidate_key)
			SELECT ?, 'chat', 0, id, cleanup_request_id, 'hard-chat:' || id::text FROM locked
		`, append(args, item.DataClass)...)
		if result.Error != nil {
			return "", result.Error
		}
		inserted = result.RowsAffected
	case lifecycle.ClassRobotChatMessages:
		scopeSQL, scopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := []any{item.CutoffAt}
		args = append(args, scopeArgs...)
		args = append(args, limit)
		result := tx.Exec(`
			WITH locked AS (
				SELECT message.id, COALESCE(message.cleanup_request_id, '') AS cleanup_request_id
				FROM member_chat_messages message
				JOIN workspace_robot_profiles robot
				  ON robot.user_id = message.user_id AND robot.workspace_id = message.workspace_id
				WHERE message.deleted_at IS NOT NULL
				  AND message.deleted_at < ?
				  AND message.message_type = 'text'
				  AND message.reference_id = 0
				  AND `+scopeSQL+`
				ORDER BY message.id ASC LIMIT ? FOR UPDATE OF message
			)
			INSERT INTO lifecycle_hard_delete_candidates
				(data_class, source_kind, source_order, id, cleanup_request_id, candidate_key)
			SELECT ?, 'robot_chat', 0, id, cleanup_request_id, 'hard-robot-chat:' || id::text FROM locked
		`, append(args, item.DataClass)...)
		if result.Error != nil {
			return "", result.Error
		}
		inserted = result.RowsAffected
	case lifecycle.ClassNotifications:
		memberScope, memberArgs := lifecycleScope(criteria, "notice.workspace_id")
		memberArgsWithLimit := []any{item.CutoffAt}
		memberArgsWithLimit = append(memberArgsWithLimit, memberArgs...)
		memberArgsWithLimit = append(memberArgsWithLimit, limit, item.DataClass)
		memberResult := tx.Exec(`
			WITH locked AS (
				SELECT notice.id, COALESCE(notice.cleanup_request_id, '') AS cleanup_request_id
				FROM member_notifications notice
				WHERE notice.deleted_at IS NOT NULL AND notice.deleted_at < ?
				  AND `+disposableMemberNotificationPredicate+`
				  AND `+memberScope+`
				ORDER BY notice.id ASC LIMIT ? FOR UPDATE OF notice
			)
			INSERT INTO lifecycle_hard_delete_candidates
				(data_class, source_kind, source_order, id, cleanup_request_id, candidate_key)
			SELECT ?, 'member_notification', 0, id, cleanup_request_id,
				'hard-member-notification:' || id::text FROM locked
		`, memberArgsWithLimit...)
		if memberResult.Error != nil {
			return "", memberResult.Error
		}
		inserted = memberResult.RowsAffected
		remaining := limit - int(inserted)
		if remaining > 0 {
			adminScope, adminArgs := lifecycleScope(criteria, "notice.workspace_id")
			adminArgsWithLimit := []any{item.CutoffAt}
			adminArgsWithLimit = append(adminArgsWithLimit, adminArgs...)
			adminArgsWithLimit = append(adminArgsWithLimit, remaining, item.DataClass)
			adminResult := tx.Exec(`
				WITH locked AS (
					SELECT notice.id, COALESCE(notice.cleanup_request_id, '') AS cleanup_request_id
					FROM admin_notifications notice
					WHERE notice.deleted_at IS NOT NULL AND notice.deleted_at < ?
					  AND `+disposableAdminNotificationPredicate+`
					  AND `+adminScope+`
					ORDER BY notice.id ASC LIMIT ? FOR UPDATE OF notice
				)
				INSERT INTO lifecycle_hard_delete_candidates
					(data_class, source_kind, source_order, id, cleanup_request_id, candidate_key)
				SELECT ?, 'admin_notification', 1, id, cleanup_request_id,
					'hard-admin-notification:' || id::text FROM locked
			`, adminArgsWithLimit...)
			if adminResult.Error != nil {
				return "", adminResult.Error
			}
			inserted += adminResult.RowsAffected
		}
	default:
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "该数据类型禁止硬删除")
	}
	if inserted != int64(limit) {
		return "", apperrors.NewBusinessError("PREVIEW_CHANGED", "候选数据已变化，请重新生成预览后执行")
	}
	var keys []lifecycleCandidateKey
	if err := tx.Raw(`
		SELECT candidate_key
		FROM lifecycle_hard_delete_candidates
		WHERE data_class = ?
		ORDER BY source_order ASC, id ASC
	`, item.DataClass).Scan(&keys).Error; err != nil {
		return "", err
	}
	return fingerprintCandidateKeys(keys), nil
}

func (s *DataLifecycleService) countCandidates(criteria normalizedCleanupCriteria, item CleanupPreviewItem) (int64, error) {
	scopeSQL, scopeArgs := lifecycleScope(criteria, "workspace_id")
	var count int64
	switch item.DataClass {
	case lifecycle.ClassChatMessages:
		chatScope, chatArgs := lifecycleScope(criteria, "message.workspace_id")
		deletedPredicate := "message.deleted_at IS NULL AND message.created_at < ?"
		if criteria.DeleteMode == DeleteModeHard {
			deletedPredicate = "message.deleted_at IS NOT NULL AND message.deleted_at < ?"
		}
		err := s.db.Raw(`
			SELECT COUNT(*) FROM member_chat_messages message
			WHERE `+deletedPredicate+`
			  AND `+genericChatLifecyclePredicate+`
			  AND `+chatScope,
			append([]any{item.CutoffAt}, chatArgs...)...).Scan(&count).Error
		return count, err
	case lifecycle.ClassRobotChatMessages:
		robotScope, robotArgs := lifecycleScope(criteria, "message.workspace_id")
		deletedPredicate := "message.deleted_at IS NULL AND message.created_at < ?"
		if criteria.DeleteMode == DeleteModeHard {
			deletedPredicate = "message.deleted_at IS NOT NULL AND message.deleted_at < ?"
		}
		err := s.db.Raw(`
			SELECT COUNT(*)
			FROM member_chat_messages message
			JOIN workspace_robot_profiles robot
			  ON robot.user_id = message.user_id
			 AND robot.workspace_id = message.workspace_id
			WHERE `+deletedPredicate+`
			  AND message.message_type = 'text'
			  AND message.reference_id = 0
			  AND `+robotScope,
			append([]any{item.CutoffAt}, robotArgs...)...).Scan(&count).Error
		return count, err
	case lifecycle.ClassNotifications:
		var memberCount, adminCount int64
		deletedPredicate := "deleted_at IS NULL AND created_at < ?"
		if criteria.DeleteMode == DeleteModeHard {
			deletedPredicate = "deleted_at IS NOT NULL AND deleted_at < ?"
		}
		args := append([]any{item.CutoffAt}, scopeArgs...)
		if err := s.db.Raw("SELECT COUNT(*) FROM member_notifications notice WHERE "+deletedPredicate+" AND "+disposableMemberNotificationPredicate+" AND "+scopeSQL, args...).Scan(&memberCount).Error; err != nil {
			return 0, err
		}
		if err := s.db.Raw("SELECT COUNT(*) FROM admin_notifications notice WHERE "+deletedPredicate+" AND "+disposableAdminNotificationPredicate+" AND "+scopeSQL, args...).Scan(&adminCount).Error; err != nil {
			return 0, err
		}
		return memberCount + adminCount, nil
	case lifecycle.ClassAuditLogs:
		err := s.db.Raw("SELECT COUNT(*) FROM admin_audit_logs WHERE created_at < ? AND "+scopeSQL, append([]any{item.CutoffAt}, scopeArgs...)...).Scan(&count).Error
		return count, err
	case lifecycle.ClassRobotTestData:
		betScope, betArgs := lifecycleScope(criteria, "b.workspace_id")
		ledgerScope, ledgerArgs := lifecycleScope(criteria, "ledger.workspace_id")
		var bets, ledger int64
		if err := s.db.Raw(`
			SELECT COUNT(*) FROM lottery_bets b
			JOIN workspace_robot_profiles robot ON robot.user_id = b.user_id AND robot.workspace_id = b.workspace_id
			LEFT JOIN lottery_bet_archives archive ON archive.id = b.id
			WHERE archive.id IS NULL
			  AND b.status IN ('won', 'lost')
			  AND b.settled_at IS NOT NULL
			  AND b.reconciliation_status = 'normal'
			  AND b.created_at < ? AND `+betScope, append([]any{item.CutoffAt}, betArgs...)...).Scan(&bets).Error; err != nil {
			return 0, err
		}
		if err := s.db.Raw(`
			SELECT COUNT(*) FROM user_balance_transactions ledger
			JOIN workspace_robot_profiles robot ON robot.user_id = ledger.user_id AND robot.workspace_id = ledger.workspace_id
			LEFT JOIN user_balance_transaction_archives archive ON archive.id = ledger.id
			WHERE archive.id IS NULL
			  AND ledger.created_at < ?
			  AND NOT EXISTS (
				SELECT 1 FROM lottery_bets unresolved
				WHERE unresolved.workspace_id = ledger.workspace_id
				  AND unresolved.user_id = ledger.user_id
				  AND (unresolved.status = 'pending' OR unresolved.reconciliation_status <> 'normal')
			  )
			  AND `+ledgerScope, append([]any{item.CutoffAt}, ledgerArgs...)...).Scan(&ledger).Error; err != nil {
			return 0, err
		}
		return bets + ledger, nil
	default:
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "不支持的数据类型")
	}
}

// countProtectedRobotFinancialRows reports old hot-table rows that are
// deliberately excluded from robot cold archiving. It includes every real
// member row and every robot row whose bet state is pending, unsettled, or
// abnormal. A robot ledger is also protected while that member has any
// unresolved bet in the same workspace. This count is informational only and
// is never used to construct the archive candidate set.
func (s *DataLifecycleService) countProtectedRobotFinancialRows(criteria normalizedCleanupCriteria, cutoff time.Time) (int64, error) {
	betScope, betArgs := lifecycleScope(criteria, "source.workspace_id")
	ledgerScope, ledgerArgs := lifecycleScope(criteria, "source.workspace_id")

	var bets, ledger int64
	betParams := append([]any{cutoff}, betArgs...)
	if err := s.db.Raw(`
		SELECT COUNT(*)
		FROM lottery_bets source
		LEFT JOIN workspace_robot_profiles robot
		  ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
		WHERE source.created_at < ? AND `+betScope+`
		  AND (
			robot.user_id IS NULL
			OR source.status NOT IN ('won', 'lost')
			OR source.settled_at IS NULL
			OR source.reconciliation_status <> 'normal'
		  )
	`, betParams...).Scan(&bets).Error; err != nil {
		return 0, err
	}

	ledgerParams := append([]any{cutoff}, ledgerArgs...)
	if err := s.db.Raw(`
		SELECT COUNT(*)
		FROM user_balance_transactions source
		LEFT JOIN workspace_robot_profiles robot
		  ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
		WHERE source.created_at < ? AND `+ledgerScope+`
		  AND (
			robot.user_id IS NULL
			OR EXISTS (
				SELECT 1 FROM lottery_bets unresolved
				WHERE unresolved.workspace_id = source.workspace_id
				  AND unresolved.user_id = source.user_id
				  AND (unresolved.status = 'pending' OR unresolved.reconciliation_status <> 'normal')
			)
		  )
	`, ledgerParams...).Scan(&ledger).Error; err != nil {
		return 0, err
	}
	return bets + ledger, nil
}

func (s *DataLifecycleService) executeClass(tx *gorm.DB, criteria normalizedCleanupCriteria, requestID, operator string, item CleanupPreviewItem) (int64, error) {
	limit := int(item.PlannedCount)
	if limit <= 0 {
		return 0, nil
	}
	if criteria.DeleteMode == DeleteModeHard {
		return s.hardDeleteClass(tx, item, limit)
	}
	scopeSQL, scopeArgs := lifecycleScope(criteria, "workspace_id")
	switch item.DataClass {
	case lifecycle.ClassChatMessages:
		chatScope, chatScopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := []any{item.CutoffAt}
		args = append(args, chatScopeArgs...)
		args = append(args, limit, s.now(), lifecycleOperator(operator, requestID), requestID)
		result := tx.Exec(`
			WITH candidates AS (
				SELECT message.id FROM member_chat_messages message
				WHERE message.deleted_at IS NULL
				  AND message.created_at < ?
				  AND `+genericChatLifecyclePredicate+`
				  AND `+chatScope+`
				ORDER BY message.id ASC LIMIT ? FOR UPDATE OF message SKIP LOCKED
			)
			UPDATE member_chat_messages AS row
			SET deleted_at = ?, deleted_by = ?, cleanup_request_id = ?
			FROM candidates WHERE row.id = candidates.id
		`, args...)
		return result.RowsAffected, result.Error
	case lifecycle.ClassRobotChatMessages:
		robotScope, robotScopeArgs := lifecycleScope(criteria, "message.workspace_id")
		args := []any{item.CutoffAt}
		args = append(args, robotScopeArgs...)
		args = append(args, limit, s.now(), lifecycleOperator(operator, requestID), requestID)
		result := tx.Exec(`
			WITH candidates AS (
				SELECT message.id
				FROM member_chat_messages message
				JOIN workspace_robot_profiles robot
				  ON robot.user_id = message.user_id
				 AND robot.workspace_id = message.workspace_id
				WHERE message.deleted_at IS NULL
				  AND message.message_type = 'text'
				  AND message.reference_id = 0
				  AND message.created_at < ?
				  AND `+robotScope+`
				ORDER BY message.id ASC LIMIT ? FOR UPDATE OF message SKIP LOCKED
			)
			UPDATE member_chat_messages AS message
			SET deleted_at = ?, deleted_by = ?, cleanup_request_id = ?
			FROM candidates WHERE message.id = candidates.id
		`, args...)
		return result.RowsAffected, result.Error
	case lifecycle.ClassNotifications:
		return s.softDeleteNotifications(tx, criteria, item.CutoffAt, limit, lifecycleOperator(operator, requestID), requestID)
	case lifecycle.ClassAuditLogs:
		args := []any{item.CutoffAt}
		args = append(args, scopeArgs...)
		args = append(args, limit, requestID)
		result := tx.Exec(`
			WITH candidates AS (
				SELECT * FROM admin_audit_logs
				WHERE created_at < ? AND `+scopeSQL+`
				ORDER BY id ASC LIMIT ? FOR UPDATE SKIP LOCKED
			), archived AS (
				INSERT INTO admin_audit_log_archives (
					source_id, workspace_id, actor_id, actor_name, actor_role, room_scope,
					method, path, status_code, request_id, ip, source_created_at, cleanup_request_id
				)
				SELECT id, workspace_id, actor_id, actor_name, actor_role, COALESCE(room_scope, ''),
					method, path, status_code, COALESCE(request_id, ''), COALESCE(ip, ''), created_at, ?
				FROM candidates ON CONFLICT (source_id) DO NOTHING RETURNING source_id
			)
			DELETE FROM admin_audit_logs hot USING candidates, archived
			WHERE hot.id = candidates.id
			  AND archived.source_id = hot.id
		`, args...)
		return result.RowsAffected, result.Error
	case lifecycle.ClassRobotTestData:
		return s.archiveRobotFinancialRows(tx, criteria, requestID, item.CutoffAt, limit)
	default:
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "不支持的数据类型")
	}
}

func (s *DataLifecycleService) hardDeleteClass(tx *gorm.DB, item CleanupPreviewItem, limit int) (int64, error) {
	if _, allowed := hardDeleteDataClasses[item.DataClass]; !allowed {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "该数据类型禁止硬删除")
	}
	switch item.DataClass {
	case lifecycle.ClassChatMessages:
		result := tx.Exec(`
			DELETE FROM member_chat_messages AS message
			USING lifecycle_hard_delete_candidates candidate
			WHERE candidate.data_class = ? AND candidate.source_kind = 'chat'
			  AND message.id = candidate.id
		`, item.DataClass)
		return exactHardDeleteResult(item.DataClass, limit, result)
	case lifecycle.ClassRobotChatMessages:
		result := tx.Exec(`
			DELETE FROM member_chat_messages AS message
			USING lifecycle_hard_delete_candidates candidate
			WHERE candidate.data_class = ? AND candidate.source_kind = 'robot_chat'
			  AND message.id = candidate.id
		`, item.DataClass)
		return exactHardDeleteResult(item.DataClass, limit, result)
	case lifecycle.ClassNotifications:
		return s.hardDeleteNotifications(tx, item.DataClass, limit)
	default:
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "该数据类型禁止硬删除")
	}
}

func (s *DataLifecycleService) hardDeleteNotifications(tx *gorm.DB, dataClass string, limit int) (int64, error) {
	var affected int64
	for _, target := range []struct {
		table      string
		sourceKind string
	}{
		{table: "member_notifications", sourceKind: "member_notification"},
		{table: "admin_notifications", sourceKind: "admin_notification"},
	} {
		result := tx.Exec(`
			DELETE FROM `+target.table+` AS notice
			USING lifecycle_hard_delete_candidates candidate
			WHERE candidate.data_class = ? AND candidate.source_kind = ?
			  AND notice.id = candidate.id
		`, dataClass, target.sourceKind)
		if result.Error != nil {
			return 0, result.Error
		}
		affected += result.RowsAffected
	}
	if affected != int64(limit) {
		return 0, apperrors.NewBusinessError(
			"PREVIEW_CHANGED",
			fmt.Sprintf("硬删除候选已变化：预览 %d 条，实际 %d 条；本次操作已回滚，请重新预览", limit, affected),
		)
	}
	return affected, nil
}

func exactHardDeleteResult(dataClass string, planned int, result *gorm.DB) (int64, error) {
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != int64(planned) {
		return 0, apperrors.NewBusinessError(
			"PREVIEW_CHANGED",
			fmt.Sprintf("%s 硬删除候选已变化：预览 %d 条，实际 %d 条；本次操作已回滚，请重新预览", dataClass, planned, result.RowsAffected),
		)
	}
	return result.RowsAffected, nil
}

func (s *DataLifecycleService) recordContentPurgeSources(tx *gorm.DB, purgeRequestID string) error {
	var invalid int64
	if err := tx.Raw(`
		SELECT COUNT(*)
		FROM lifecycle_hard_delete_candidates candidate
		LEFT JOIN data_cleanup_runs source ON source.request_id = candidate.cleanup_request_id
		WHERE candidate.cleanup_request_id <> ''
		  AND (
			source.id IS NULL
			OR source.status <> 'completed'
			OR COALESCE(source.criteria_json->>'delete_mode', 'soft') <> 'soft'
			OR source.soft_restored_at IS NOT NULL
		  )
	`).Scan(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return apperrors.NewBusinessError("PREVIEW_CHANGED", "回收站来源任务不完整，本次永久删除已回滚，请重新核对")
	}
	now := s.now()
	return tx.Exec(`
		WITH purged AS (
			SELECT cleanup_request_id, COUNT(*)::bigint AS affected_count
			FROM lifecycle_hard_delete_candidates
			WHERE cleanup_request_id <> ''
			GROUP BY cleanup_request_id
		)
		UPDATE data_cleanup_runs AS source
		SET content_purged_at = COALESCE(source.content_purged_at, ?),
			content_purge_count = source.content_purge_count + purged.affected_count,
			last_content_purge_request_id = ?,
			updated_at = ?
		FROM purged
		WHERE source.request_id = purged.cleanup_request_id
	`, now, purgeRequestID, now).Error
}

func (s *DataLifecycleService) softDeleteNotifications(tx *gorm.DB, criteria normalizedCleanupCriteria, cutoff time.Time, limit int, operator, requestID string) (int64, error) {
	scopeSQL, scopeArgs := lifecycleScope(criteria, "workspace_id")
	remaining := limit
	var affected int64
	for _, table := range []string{"member_notifications", "admin_notifications"} {
		if remaining <= 0 {
			break
		}
		args := []any{cutoff}
		args = append(args, scopeArgs...)
		args = append(args, remaining, s.now(), operator, requestID)
		predicate := "TRUE"
		if table == "member_notifications" {
			predicate = disposableMemberNotificationPredicate
		} else {
			predicate = disposableAdminNotificationPredicate
		}
		result := tx.Exec(`
			WITH candidates AS (
				SELECT notice.id FROM `+table+` notice
				WHERE notice.deleted_at IS NULL AND notice.created_at < ?
				  AND `+predicate+`
				  AND `+scopeSQL+`
				ORDER BY id ASC LIMIT ? FOR UPDATE SKIP LOCKED
			)
			UPDATE `+table+` AS row SET deleted_at = ?, deleted_by = ?, cleanup_request_id = ?
			FROM candidates WHERE row.id = candidates.id
		`, args...)
		if result.Error != nil {
			return 0, result.Error
		}
		affected += result.RowsAffected
		remaining -= int(result.RowsAffected)
	}
	return affected, nil
}

func (s *DataLifecycleService) archiveRobotFinancialRows(tx *gorm.DB, criteria normalizedCleanupCriteria, requestID string, cutoff time.Time, limit int) (int64, error) {
	// Serialize cold-archive runs. Settlement may continue concurrently, but
	// only already-final rows selected FOR UPDATE can enter this transaction.
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(729421118)).Error; err != nil {
		return 0, err
	}
	for table := range protectedFinancialTables {
		if table != "lottery_bets" && table != "user_balance_transactions" {
			return 0, fmt.Errorf("unrecognized protected financial table %s", table)
		}
	}

	remaining := limit
	betCount, err := s.archiveRobotBets(tx, criteria, requestID, cutoff, remaining)
	if err != nil {
		return 0, err
	}
	remaining -= int(betCount)
	if remaining <= 0 {
		return betCount, nil
	}
	ledgerCount, err := s.archiveRobotLedger(tx, criteria, requestID, cutoff, remaining)
	if err != nil {
		return 0, err
	}
	return betCount + ledgerCount, nil
}

func (s *DataLifecycleService) archiveRobotBets(tx *gorm.DB, criteria normalizedCleanupCriteria, requestID string, cutoff time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	scopeSQL, scopeArgs := lifecycleScope(criteria, "source.workspace_id")
	baseArgs := []any{cutoff}
	baseArgs = append(baseArgs, scopeArgs...)
	baseArgs = append(baseArgs, limit)
	var expected int64
	if err := tx.Raw(`
		SELECT COUNT(*) FROM (
			SELECT source.id FROM lottery_bets source
			JOIN workspace_robot_profiles robot ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
			LEFT JOIN lottery_bet_archives archive ON archive.id = source.id
			WHERE archive.id IS NULL
			  AND source.status IN ('won', 'lost')
			  AND source.settled_at IS NOT NULL
			  AND source.reconciliation_status = 'normal'
			  AND source.created_at < ? AND `+scopeSQL+`
			ORDER BY source.id ASC LIMIT ?
		) eligible
	`, baseArgs...).Scan(&expected).Error; err != nil {
		return 0, err
	}
	if expected == 0 {
		return 0, nil
	}

	args := []any{cutoff}
	args = append(args, scopeArgs...)
	args = append(args, limit, requestID)
	result := tx.Exec(`
		WITH candidates AS (
			SELECT source.* FROM lottery_bets source
			JOIN workspace_robot_profiles robot ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
			LEFT JOIN lottery_bet_archives archive ON archive.id = source.id
			WHERE archive.id IS NULL
			  AND source.status IN ('won', 'lost')
			  AND source.settled_at IS NOT NULL
			  AND source.reconciliation_status = 'normal'
			  AND source.created_at < ? AND `+scopeSQL+`
			ORDER BY source.id ASC LIMIT ? FOR UPDATE OF source SKIP LOCKED
		), archived AS (
			INSERT INTO lottery_bet_archives (
				id, workspace_id, game_id, issue, room_scope, user_id, username,
				play_code, play_name, position, selection, amount_cents, odds, status,
				payout_cents, fly_cents, rebate_rate_snapshot, rebate_cents,
				agent_share_rate_snapshot, agent_share_cents, settled_at, remark,
				operator, reconciliation_status, reconciliation_note, created_at,
				updated_at, source_json, row_hash, archived_at, cleanup_request_id
			)
			SELECT id, workspace_id, game_id, issue, room_scope, user_id, username,
				play_code, play_name, position, selection, amount_cents, odds, status,
				payout_cents, fly_cents, rebate_rate_snapshot, rebate_cents,
				agent_share_rate_snapshot, agent_share_cents, settled_at, remark,
				operator, reconciliation_status, reconciliation_note, created_at,
				updated_at, to_jsonb(candidates), md5(to_jsonb(candidates)::text), now(), ?
			FROM candidates ON CONFLICT (id) DO NOTHING RETURNING id
		)
		DELETE FROM lottery_bets hot USING candidates, lottery_bet_archives archive
		WHERE hot.id = candidates.id AND archive.id = hot.id
		  AND archive.row_hash = md5(to_jsonb(hot)::text)
	`, args...)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != expected {
		return 0, fmt.Errorf("robot bet archive verification failed: expected=%d verified=%d", expected, result.RowsAffected)
	}
	return result.RowsAffected, nil
}

func (s *DataLifecycleService) archiveRobotLedger(tx *gorm.DB, criteria normalizedCleanupCriteria, requestID string, cutoff time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	scopeSQL, scopeArgs := lifecycleScope(criteria, "source.workspace_id")
	where := `
		archive.id IS NULL
		AND source.created_at < ?
		AND NOT EXISTS (
			SELECT 1 FROM lottery_bets unresolved
			WHERE unresolved.workspace_id = source.workspace_id
			  AND unresolved.user_id = source.user_id
			  AND (unresolved.status = 'pending' OR unresolved.reconciliation_status <> 'normal')
		)
		AND ` + scopeSQL
	baseArgs := []any{cutoff}
	baseArgs = append(baseArgs, scopeArgs...)
	baseArgs = append(baseArgs, limit)
	var expected int64
	if err := tx.Raw(`
		SELECT COUNT(*) FROM (
			SELECT source.id FROM user_balance_transactions source
			JOIN workspace_robot_profiles robot ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
			LEFT JOIN user_balance_transaction_archives archive ON archive.id = source.id
			WHERE `+where+`
			ORDER BY source.id ASC LIMIT ?
		) eligible
	`, baseArgs...).Scan(&expected).Error; err != nil {
		return 0, err
	}
	if expected == 0 {
		return 0, nil
	}

	args := []any{cutoff}
	args = append(args, scopeArgs...)
	args = append(args, limit, requestID)
	result := tx.Exec(`
		WITH candidates AS (
			SELECT source.* FROM user_balance_transactions source
			JOIN workspace_robot_profiles robot ON robot.user_id = source.user_id AND robot.workspace_id = source.workspace_id
			LEFT JOIN user_balance_transaction_archives archive ON archive.id = source.id
			WHERE `+where+`
			ORDER BY source.id ASC LIMIT ? FOR UPDATE OF source SKIP LOCKED
		), archived AS (
			INSERT INTO user_balance_transaction_archives (
				id, workspace_id, user_id, reference, amount_cents, before_cents,
				after_cents, type, remark, operator, created_at, source_json, row_hash,
				archived_at, cleanup_request_id
			)
			SELECT id, workspace_id, user_id, reference, amount_cents, before_cents,
				after_cents, type, remark, operator, created_at,
				to_jsonb(candidates), md5(to_jsonb(candidates)::text), now(), ?
			FROM candidates ON CONFLICT (id) DO NOTHING RETURNING id
		)
		DELETE FROM user_balance_transactions hot
		USING candidates, user_balance_transaction_archives archive
		WHERE hot.id = candidates.id AND archive.id = hot.id
		  AND archive.row_hash = md5(to_jsonb(hot)::text)
	`, args...)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != expected {
		return 0, fmt.Errorf("robot ledger archive verification failed: expected=%d verified=%d", expected, result.RowsAffected)
	}
	return result.RowsAffected, nil
}

func restoreRobotBetArchive(tx *gorm.DB, requestID string) (int64, error) {
	if err := tx.Exec(`CREATE TEMP TABLE lifecycle_restore_bets ON COMMIT DROP AS SELECT * FROM lottery_bet_archives WHERE cleanup_request_id = ?`, requestID).Error; err != nil {
		return 0, err
	}
	var expected, invalid int64
	if err := tx.Raw(`SELECT COUNT(*) FROM lifecycle_restore_bets`).Scan(&expected).Error; err != nil {
		return 0, err
	}
	if expected == 0 {
		return 0, nil
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM lifecycle_restore_bets WHERE row_hash <> md5(source_json::text)`).Scan(&invalid).Error; err != nil {
		return 0, err
	}
	if invalid != 0 {
		return 0, fmt.Errorf("robot bet archive hash verification failed: invalid=%d", invalid)
	}
	if result := tx.Exec(`DELETE FROM lottery_bet_archives archive USING lifecycle_restore_bets source WHERE archive.id = source.id`); result.Error != nil || result.RowsAffected != expected {
		if result.Error != nil {
			return 0, result.Error
		}
		return 0, fmt.Errorf("robot bet archive move failed: expected=%d removed=%d", expected, result.RowsAffected)
	}
	result := tx.Exec(`
		INSERT INTO lottery_bets (
			id, workspace_id, game_id, issue, room_scope, user_id, username,
			play_code, play_name, position, selection, amount_cents, odds, status,
			payout_cents, fly_cents, rebate_rate_snapshot, rebate_cents,
			agent_share_rate_snapshot, agent_share_cents, settled_at, remark,
			operator, reconciliation_status, reconciliation_note, created_at, updated_at
		)
		SELECT id, workspace_id, game_id, issue, room_scope, user_id, username,
			play_code, play_name, position, selection, amount_cents, odds, status,
			payout_cents, fly_cents, rebate_rate_snapshot, rebate_cents,
			agent_share_rate_snapshot, agent_share_cents, settled_at, remark,
			operator, reconciliation_status, reconciliation_note, created_at, updated_at
		FROM lifecycle_restore_bets ORDER BY id ASC
	`)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != expected {
		return 0, fmt.Errorf("robot bet restore count mismatch: expected=%d restored=%d", expected, result.RowsAffected)
	}
	if err := tx.Raw(`
		SELECT COUNT(*) FROM lifecycle_restore_bets archive
		JOIN lottery_bets hot ON hot.id = archive.id
		WHERE archive.row_hash <> md5(to_jsonb(hot)::text)
	`).Scan(&invalid).Error; err != nil {
		return 0, err
	}
	if invalid != 0 {
		return 0, fmt.Errorf("restored robot bet hash verification failed: invalid=%d", invalid)
	}
	return expected, nil
}

func restoreRobotLedgerArchive(tx *gorm.DB, requestID string) (int64, error) {
	if err := tx.Exec(`CREATE TEMP TABLE lifecycle_restore_ledger ON COMMIT DROP AS SELECT * FROM user_balance_transaction_archives WHERE cleanup_request_id = ?`, requestID).Error; err != nil {
		return 0, err
	}
	var expected, invalid int64
	if err := tx.Raw(`SELECT COUNT(*) FROM lifecycle_restore_ledger`).Scan(&expected).Error; err != nil {
		return 0, err
	}
	if expected == 0 {
		return 0, nil
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM lifecycle_restore_ledger WHERE row_hash <> md5(source_json::text)`).Scan(&invalid).Error; err != nil {
		return 0, err
	}
	if invalid != 0 {
		return 0, fmt.Errorf("robot ledger archive hash verification failed: invalid=%d", invalid)
	}
	if result := tx.Exec(`DELETE FROM user_balance_transaction_archives archive USING lifecycle_restore_ledger source WHERE archive.id = source.id`); result.Error != nil || result.RowsAffected != expected {
		if result.Error != nil {
			return 0, result.Error
		}
		return 0, fmt.Errorf("robot ledger archive move failed: expected=%d removed=%d", expected, result.RowsAffected)
	}
	result := tx.Exec(`
		INSERT INTO user_balance_transactions (
			id, workspace_id, user_id, reference, amount_cents, before_cents,
			after_cents, type, remark, operator, created_at
		)
		SELECT id, workspace_id, user_id, reference, amount_cents, before_cents,
			after_cents, type, remark, operator, created_at
		FROM lifecycle_restore_ledger ORDER BY id ASC
	`)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != expected {
		return 0, fmt.Errorf("robot ledger restore count mismatch: expected=%d restored=%d", expected, result.RowsAffected)
	}
	if err := tx.Raw(`
		SELECT COUNT(*) FROM lifecycle_restore_ledger archive
		JOIN user_balance_transactions hot ON hot.id = archive.id
		WHERE archive.row_hash <> md5(to_jsonb(hot)::text)
	`).Scan(&invalid).Error; err != nil {
		return 0, err
	}
	if invalid != 0 {
		return 0, fmt.Errorf("restored robot ledger hash verification failed: invalid=%d", invalid)
	}
	return expected, nil
}

func cleanupRunView(row lifecycle.CleanupRun) (CleanupRunView, error) {
	view := CleanupRunView{
		ID: row.ID, RequestID: row.RequestID, WorkspaceID: row.WorkspaceID,
		AllWorkspaces: row.AllWorkspaces, ActorID: row.ActorID, ActorName: row.ActorName,
		ExecutedByID: row.ExecutedByID, ExecutedByName: row.ExecutedByName,
		Status: row.Status, BatchLimit: row.BatchLimit, LastError: row.LastError,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt,
		SoftRestoredAt: row.SoftRestoredAt, FinancialRestoredAt: row.FinancialRestoredAt,
		SoftRestoredByID: row.SoftRestoredByID, SoftRestoredByName: row.SoftRestoredByName,
		FinancialRestoredByID: row.FinancialRestoredByID, FinancialRestoredByName: row.FinancialRestoredByName,
		ContentPurgedAt: row.ContentPurgedAt, ContentPurgeCount: row.ContentPurgeCount,
		LastContentPurgeRequestID: row.LastContentPurgeRequestID,
		CreatedAt:                 row.CreatedAt,
	}
	var criteria normalizedCleanupCriteria
	if strings.TrimSpace(row.CriteriaJSON) != "" {
		if err := json.Unmarshal([]byte(row.CriteriaJSON), &criteria); err != nil {
			return view, err
		}
	}
	view.DeleteMode = normalizeDeleteMode(criteria.DeleteMode)
	for _, item := range []struct {
		source string
		target *[]CleanupResultItem
	}{
		{source: row.ResultJSON, target: &view.Result},
		{source: row.SoftRestoreResultJSON, target: &view.SoftRestoreResult},
		{source: row.FinancialRestoreResultJSON, target: &view.FinancialRestoreResult},
	} {
		source, target := item.source, item.target
		if strings.TrimSpace(source) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(source), target); err != nil {
			return view, err
		}
	}
	if strings.TrimSpace(row.PreviewJSON) != "" {
		if err := json.Unmarshal([]byte(row.PreviewJSON), &view.Preview); err != nil {
			return view, err
		}
	}
	return view, nil
}

func lifecycleScope(criteria normalizedCleanupCriteria, column string) (string, []any) {
	if criteria.AllWorkspaces {
		// workspace 0 is intentionally excluded: those legacy rows have unknown
		// ownership and may only be classified manually after investigation.
		return column + " > 0", nil
	}
	return column + " = ?", []any{criteria.WorkspaceID}
}

func previewFromRun(run lifecycle.CleanupRun) (*CleanupPreview, error) {
	var items []CleanupPreviewItem
	if err := json.Unmarshal([]byte(run.PreviewJSON), &items); err != nil {
		return nil, err
	}
	var criteria normalizedCleanupCriteria
	if strings.TrimSpace(run.CriteriaJSON) != "" {
		if err := json.Unmarshal([]byte(run.CriteriaJSON), &criteria); err != nil {
			return nil, err
		}
	}
	return &CleanupPreview{
		RequestID: run.RequestID, WorkspaceID: run.WorkspaceID, AllWorkspaces: run.AllWorkspaces,
		BatchLimit: run.BatchLimit, DeleteMode: normalizeDeleteMode(criteria.DeleteMode), Status: run.Status, Items: items, CreatedAt: run.CreatedAt,
	}, nil
}

func allLifecycleClasses() []string {
	items := make([]string, 0, len(lifecycleSpecs))
	for key := range lifecycleSpecs {
		items = append(items, key)
	}
	sort.Strings(items)
	return items
}

func lifecycleOperator(username, requestID string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		username = "平台管理员"
	}
	value := username + " · lifecycle:" + requestID
	for len(value) > 80 {
		_, size := utf8.DecodeLastRuneInString(value)
		if size < 1 {
			break
		}
		value = value[:len(value)-size]
	}
	return value
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func truncateLifecycleError(value string) string {
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
