package services

import (
	"backend/data/models/chat"
	"backend/data/models/lifecycle"
	"backend/data/models/notify"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestNormalizeCleanupPreviewRequiresExplicitWorkspaceScope(t *testing.T) {
	service := &DataLifecycleService{}
	_, _, err := service.normalizePreviewInput(CleanupPreviewInput{RequestID: "cleanup-0001"})
	if err == nil {
		t.Fatal("expected implicit/all-zero workspace scope to be rejected")
	}

	workspaceID := uint64(9)
	_, _, err = service.normalizePreviewInput(CleanupPreviewInput{
		RequestID: "cleanup-0002", WorkspaceID: &workspaceID, AllWorkspaces: true,
	})
	if err == nil {
		t.Fatal("expected all_workspaces plus workspace_id to be rejected")
	}
}

func TestLifecycleCandidateFingerprintIsDeterministicAndOrderSensitive(t *testing.T) {
	first := []lifecycleCandidateKey{{Key: "chat:1"}, {Key: "chat:23"}}
	if fingerprintCandidateKeys(first) != fingerprintCandidateKeys(append([]lifecycleCandidateKey(nil), first...)) {
		t.Fatal("same frozen candidate batch produced different fingerprints")
	}
	if fingerprintCandidateKeys(first) == fingerprintCandidateKeys([]lifecycleCandidateKey{{Key: "chat:12"}, {Key: "chat:3"}}) {
		t.Fatal("length-prefixed candidate batches collided")
	}
	if fingerprintCandidateKeys(first) == fingerprintCandidateKeys([]lifecycleCandidateKey{{Key: "chat:23"}, {Key: "chat:1"}}) {
		t.Fatal("candidate order is part of the frozen batch")
	}
}

func TestGenericChatLifecycleExcludesWelcomeAndRobotPolicyRows(t *testing.T) {
	for _, fragment := range []string{"message.message_type = 'text'", "workspace_robot_profiles", "reference_id = 0"} {
		if !strings.Contains(genericChatLifecyclePredicate, fragment) {
			t.Fatalf("generic chat lifecycle predicate is missing %q", fragment)
		}
	}
}

func TestCleanupDeleteModeIsFrozenAndHardDeleteIsAllowListed(t *testing.T) {
	service := &DataLifecycleService{}
	workspaceID := uint64(9)
	soft, _, err := service.normalizePreviewInput(CleanupPreviewInput{
		RequestID: "cleanup-soft-0001", WorkspaceID: &workspaceID,
		DataClasses: []string{lifecycle.ClassChatMessages},
	})
	if err != nil {
		t.Fatalf("normalize soft preview: %v", err)
	}
	if soft.DeleteMode != DeleteModeSoft {
		t.Fatalf("default delete mode = %q, want soft", soft.DeleteMode)
	}

	hard, _, err := service.normalizePreviewInput(CleanupPreviewInput{
		RequestID: "cleanup-hard-0001", WorkspaceID: &workspaceID, DeleteMode: DeleteModeHard,
		DataClasses: []string{lifecycle.ClassNotifications, lifecycle.ClassChatMessages, lifecycle.ClassRobotChatMessages},
	})
	if err != nil {
		t.Fatalf("normalize hard preview: %v", err)
	}
	if hard.DeleteMode != DeleteModeHard {
		t.Fatalf("hard delete mode = %q", hard.DeleteMode)
	}
	wantAllowed := map[string]struct{}{
		lifecycle.ClassChatMessages: {}, lifecycle.ClassRobotChatMessages: {}, lifecycle.ClassNotifications: {},
	}
	if !reflect.DeepEqual(hardDeleteDataClasses, wantAllowed) {
		t.Fatalf("hard-delete allow-list = %#v, want %#v", hardDeleteDataClasses, wantAllowed)
	}

	for _, protectedClass := range []string{lifecycle.ClassAuditLogs, lifecycle.ClassRobotTestData} {
		_, _, err = service.normalizePreviewInput(CleanupPreviewInput{
			RequestID:   "cleanup-hard-protected-" + protectedClass,
			WorkspaceID: &workspaceID, DeleteMode: DeleteModeHard, DataClasses: []string{protectedClass},
		})
		if err == nil {
			t.Fatalf("hard delete unexpectedly accepted protected class %q", protectedClass)
		}
	}
}

func TestHardDeletePredicatesRejectBusinessEvidence(t *testing.T) {
	for _, fragment := range []string{
		"event_key = ''", "game_id = ''", "issue = ''", "stake_cents = 0",
		"payout_cents = 0", "bet_count = 0", "won_count = 0",
		"category IN ('system', 'activity')",
	} {
		if !strings.Contains(disposableMemberNotificationPredicate, fragment) {
			t.Fatalf("member notification purge predicate is missing %q", fragment)
		}
	}
	for _, fragment := range []string{"notice.link = ''", "待审核申请", "待结算注单"} {
		if !strings.Contains(disposableAdminNotificationPredicate, fragment) {
			t.Fatalf("admin notification purge predicate is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"welcome", "application", "redpacket"} {
		if strings.Contains(strings.ToLower(genericChatLifecyclePredicate), forbidden) {
			t.Fatalf("generic chat predicate directly admits protected message type %q", forbidden)
		}
	}
}

func TestHardDeleteScopeNeverExpandsAcrossWorkspace(t *testing.T) {
	scope, args := lifecycleScope(normalizedCleanupCriteria{WorkspaceID: 77}, "message.workspace_id")
	if scope != "message.workspace_id = ?" || len(args) != 1 || args[0] != uint64(77) {
		t.Fatalf("workspace scope = %q %#v", scope, args)
	}
	allScope, allArgs := lifecycleScope(normalizedCleanupCriteria{AllWorkspaces: true}, "message.workspace_id")
	if allScope != "message.workspace_id > 0" || len(allArgs) != 0 {
		t.Fatalf("all-workspace scope = %q %#v", allScope, allArgs)
	}
}

func TestHardDeleteRequiresExactFrozenCount(t *testing.T) {
	if _, err := exactHardDeleteResult(lifecycle.ClassChatMessages, 2, &gorm.DB{RowsAffected: 1}); err == nil {
		t.Fatal("hard delete accepted a partial frozen batch")
	}
	got, err := exactHardDeleteResult(lifecycle.ClassChatMessages, 2, &gorm.DB{RowsAffected: 2})
	if err != nil || got != 2 {
		t.Fatalf("exact hard delete result = %d, %v", got, err)
	}
}

func TestArchiveDeleteCapabilityIsNeverNeededByHardMode(t *testing.T) {
	if cleanupIncludesArchiveClass([]string{lifecycle.ClassChatMessages, lifecycle.ClassNotifications}) {
		t.Fatal("content-only task unexpectedly requested the financial archive delete capability")
	}
	if !cleanupIncludesArchiveClass([]string{lifecycle.ClassAuditLogs}) || !cleanupIncludesArchiveClass([]string{lifecycle.ClassRobotTestData}) {
		t.Fatal("archive class did not request the archive delete capability")
	}
}

func TestHardDeleteSQLLocksFrozenCandidatesWithoutSkipping(t *testing.T) {
	contents, err := os.ReadFile("data_lifecycle.go")
	if err != nil {
		t.Fatalf("read lifecycle service: %v", err)
	}
	source := string(contents)
	materializeStart := strings.Index(source, "func (s *DataLifecycleService) materializeHardDeleteCandidates")
	materializeEnd := strings.Index(source, "func (s *DataLifecycleService) countCandidates")
	hardStart := strings.Index(source, "func (s *DataLifecycleService) hardDeleteClass")
	hardEnd := strings.Index(source, "func (s *DataLifecycleService) softDeleteNotifications")
	if materializeStart < 0 || materializeEnd <= materializeStart || hardStart < 0 || hardEnd <= hardStart {
		t.Fatal("hard-delete implementation boundary not found")
	}
	hardBlock := source[materializeStart:materializeEnd] + source[hardStart:hardEnd]
	if strings.Contains(hardBlock, "SKIP LOCKED") {
		t.Fatal("hard delete may not skip and replace a frozen preview candidate")
	}
	for _, fragment := range []string{
		"deleted_at IS NOT NULL", "FOR UPDATE OF message", "FOR UPDATE OF notice",
		"lifecycle_hard_delete_candidates", "exactHardDeleteResult", "affected != int64(limit)",
	} {
		if !strings.Contains(hardBlock, fragment) {
			t.Fatalf("hard-delete implementation is missing %q", fragment)
		}
	}
}

func TestExpectedSoftRestoreCountExcludesArchiveClasses(t *testing.T) {
	items := []CleanupResultItem{
		{DataClass: lifecycle.ClassChatMessages, AffectedCount: 2},
		{DataClass: lifecycle.ClassRobotChatMessages, AffectedCount: 3},
		{DataClass: lifecycle.ClassNotifications, AffectedCount: 4},
		{DataClass: lifecycle.ClassAuditLogs, AffectedCount: 100},
		{DataClass: lifecycle.ClassRobotTestData, AffectedCount: 200},
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	got, err := expectedSoftRestoreCount(string(encoded))
	if err != nil || got != 9 {
		t.Fatalf("expected soft restore count = %d, %v; want 9", got, err)
	}
}

func TestExecuteUsesMutuallyExclusiveDeleteCapabilities(t *testing.T) {
	contents, err := os.ReadFile("data_lifecycle.go")
	if err != nil {
		t.Fatalf("read lifecycle service: %v", err)
	}
	source := string(contents)
	start := strings.Index(source, "func (s *DataLifecycleService) Execute")
	end := strings.Index(source, "func (s *DataLifecycleService) Runs")
	if start < 0 || end <= start {
		t.Fatal("execute implementation boundary not found")
	}
	executeBlock := source[start:end]
	for _, fragment := range []string{
		"if deleteMode == DeleteModeHard",
		"allowLifecycleContentPurge(tx)",
		"else if cleanupIncludesArchiveClass(criteria.DataClasses)",
		"allowLifecycleDeletes(tx)",
	} {
		if !strings.Contains(executeBlock, fragment) {
			t.Fatalf("execute capability routing is missing %q", fragment)
		}
	}
}

func TestLegacyPreviewWithCandidatesMustBeRegenerated(t *testing.T) {
	service := &DataLifecycleService{}
	err := service.validateFrozenPreview(nil, normalizedCleanupCriteria{}, []CleanupPreviewItem{{
		DataClass: lifecycle.ClassChatMessages, Enabled: true, PlannedCount: 1,
	}})
	if err == nil || !strings.Contains(err.Error(), "旧版本") {
		t.Fatalf("legacy preview error = %v, want regeneration requirement", err)
	}
}

func TestNormalizeCleanupPreviewIsCanonicalAndBounded(t *testing.T) {
	service := &DataLifecycleService{}
	workspaceID := uint64(9)
	criteria, requestID, err := service.normalizePreviewInput(CleanupPreviewInput{
		RequestID: " cleanup-0003 ", WorkspaceID: &workspaceID,
		DataClasses: []string{lifecycle.ClassNotifications, lifecycle.ClassChatMessages, lifecycle.ClassNotifications},
	})
	if err != nil {
		t.Fatalf("normalizePreviewInput() error = %v", err)
	}
	if requestID != "cleanup-0003" || criteria.BatchLimit != defaultCleanupBatch || criteria.WorkspaceID != workspaceID {
		t.Fatalf("unexpected normalized criteria: %#v request=%q", criteria, requestID)
	}
	wantClasses := []string{lifecycle.ClassChatMessages, lifecycle.ClassNotifications}
	if !reflect.DeepEqual(criteria.DataClasses, wantClasses) {
		t.Fatalf("data classes = %#v, want %#v", criteria.DataClasses, wantClasses)
	}

	_, _, err = service.normalizePreviewInput(CleanupPreviewInput{
		RequestID: "cleanup-0004", WorkspaceID: &workspaceID, BatchLimit: maxCleanupBatch + 1,
	})
	if err == nil {
		t.Fatal("expected oversized cleanup batch to be rejected")
	}
}

func TestLifecycleFinancialDataAlwaysUsesVerifiedColdArchive(t *testing.T) {
	spec := lifecycleSpecs[lifecycle.ClassRobotTestData]
	if spec.Action != lifecycle.ActionColdArchive {
		t.Fatalf("robot financial action = %q, want %q", spec.Action, lifecycle.ActionColdArchive)
	}
	want := map[string]struct{}{"lottery_bets": {}, "user_balance_transactions": {}}
	if !reflect.DeepEqual(protectedFinancialTables, want) {
		t.Fatalf("protected financial table registry changed: %#v", protectedFinancialTables)
	}
	if lifecycleSpecs[lifecycle.ClassAuditLogs].MinimumDays < 365 {
		t.Fatal("audit retention must not allow a period shorter than one year")
	}
}

func TestLifecycleOperatorIsTraceableAndFitsColumn(t *testing.T) {
	value := lifecycleOperator("a-very-long-platform-operator-name-that-needs-to-be-shortened", "cleanup-20260827-abcdefghijk")
	if len(value) > 80 {
		t.Fatalf("operator marker length = %d", len(value))
	}
	if value == "" {
		t.Fatal("operator marker must not be empty")
	}
}

func TestLifecycleContentModelsUseAutomaticSoftDeleteScope(t *testing.T) {
	want := reflect.TypeOf(gorm.DeletedAt{})
	for name, model := range map[string]any{
		"chat":                chat.Message{},
		"member_notification": notify.MemberNotification{},
		"admin_notification":  notify.Notification{},
	} {
		field, ok := reflect.TypeOf(model).FieldByName("DeletedAt")
		if !ok || field.Type != want {
			t.Fatalf("%s DeletedAt type = %v, want gorm.DeletedAt", name, field.Type)
		}
		if _, ok := reflect.TypeOf(model).FieldByName("CleanupRequestID"); !ok {
			t.Fatalf("%s is missing cleanup request correlation", name)
		}
	}
}
