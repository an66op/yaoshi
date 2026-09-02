package services

import (
	"backend/data/models/bet"
	workspacemodel "backend/data/models/workspace"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// These tests use the shared opt-in helper, which requires a completely empty,
// explicitly named loopback test database and rolls back its entire schema.
// They never read the application's configured database or place a real bet.
func TestRobotBetArchivePostgresPreservesVersionsAndRequests(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "archive_version_room", "782021")
	member := timingPostgresMember(t, db, room, "archive_version_member")
	if err := db.Create(&workspacemodel.RobotProfile{WorkspaceID: room.ID, UserID: member.UserID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewDataLifecycleService(db)
	criteria := normalizedCleanupCriteria{WorkspaceID: room.ID}
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	requestID := "archive-rules-and-requests"
	snapshots := map[uint64]string{}
	var originals []bet.Bet
	for _, version := range []string{"", "racing-v2"} {
		// Archive the legacy version first. A later insertion with the same
		// request and selection but a different version must remain distinct.
		for _, reference := range []string{"assistant:archive-request-a", "assistant:archive-request-b"} {
			row := robotBetArchiveFixture(room.ID, member.UserID, member.Username, room.Scope)
			row.RuleVersion, row.RequestReference = version, reference
			row.AmountCents += int64(len(originals))
			if err := db.Create(&row).Error; err != nil {
				t.Fatalf("insert distinct version/request: %v", err)
			}
			originals = append(originals, row)
			snapshots[row.ID] = robotBetHotJSON(t, db, row.ID)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := allowLifecycleDeletes(tx); err != nil {
				return err
			}
			count, err := service.archiveRobotBets(tx, criteria, requestID, cutoff, 100)
			if err == nil && count != 2 {
				return fmt.Errorf("archived %d rows, want 2", count)
			}
			if err != nil {
				return err
			}
			repeated, err := service.archiveRobotBets(tx, criteria, requestID, cutoff, 100)
			if err == nil && repeated != 0 {
				return fmt.Errorf("archive retry moved %d rows twice", repeated)
			}
			return err
		}); err != nil {
			t.Fatal("archive complete immutable snapshots:", err)
		}
	}
	assertRobotBetArchiveCounts(t, db, 0, 4)
	var badEvidence int64
	if err := db.Raw(`SELECT COUNT(*) FROM lottery_bet_archives
		WHERE row_hash <> md5(source_json::text)
		   OR rule_version <> source_json ->> 'rule_version'
		   OR COALESCE(source_json ->> 'request_reference', '') = ''`).Scan(&badEvidence).Error; err != nil || badEvidence != 0 {
		t.Fatalf("archive lost version/request evidence: count=%d error=%v", badEvidence, err)
	}
	for _, original := range originals {
		duplicate := original
		duplicate.ID = 0
		err := db.Transaction(func(tx *gorm.DB) error { return tx.Create(&duplicate).Error })
		if err == nil || !strings.Contains(err.Error(), "cold archive") {
			t.Fatalf("cold archive allowed an identical request/version: %+v / %v", original, err)
		}
	}
	// A genuinely different request in the same version must not be blocked
	// just because this selection was already archived for another request.
	later := originals[len(originals)-1]
	later.ID, later.RequestReference = 0, "assistant:archive-request-c"
	if err := db.Create(&later).Error; err != nil {
		t.Fatal("cold archive blocked a different request:", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := allowLifecycleDeletes(tx); err != nil {
			return err
		}
		count, err := restoreRobotBetArchive(tx, requestID)
		if err == nil && count != 4 {
			return fmt.Errorf("restored %d rows, want 4", count)
		}
		return err
	}); err != nil {
		t.Fatal("restore complete immutable snapshots:", err)
	}
	assertRobotBetArchiveCounts(t, db, 5, 0)
	for id, expected := range snapshots {
		if actual := robotBetHotJSON(t, db, id); actual != expected {
			t.Fatalf("restored row %d changed financial/request/rule evidence:\n%s\n%s", id, expected, actual)
		}
	}
}

func TestRobotBetArchivePostgresRollsBackIncompleteCopy(t *testing.T) {
	db := timingPostgresDatabase(t)
	first := robotBetArchiveOwnedFixture(t, db)
	first.RuleVersion, first.RequestReference = "racing-v2", "assistant:atomic-a"
	second := first
	second.RequestReference = "assistant:atomic-b"
	rows := []bet.Bet{first, second}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&workspacemodel.RobotProfile{WorkspaceID: first.WorkspaceID, UserID: first.UserID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	snapshots := map[uint64]string{}
	for _, row := range rows {
		snapshots[row.ID] = robotBetHotJSON(t, db, row.ID)
	}
	// Let the first cold insert succeed, but suppress the second one. The
	// verified row count must roll back both the first insert and its DELETE.
	if err := db.Exec(`CREATE FUNCTION fixture_skip_archive_copy() RETURNS trigger AS $$
		BEGIN
			IF NEW.source_json ->> 'request_reference' = 'assistant:atomic-b' THEN RETURN NULL; END IF;
			RETURN NEW;
		END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER fixture_skip_archive_copy BEFORE INSERT ON lottery_bet_archives
		FOR EACH ROW EXECUTE FUNCTION fixture_skip_archive_copy()`).Error; err != nil {
		t.Fatal(err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := allowLifecycleDeletes(tx); err != nil {
			return err
		}
		_, err := NewDataLifecycleService(db).archiveRobotBets(tx, normalizedCleanupCriteria{WorkspaceID: first.WorkspaceID}, "archive-atomic", first.CreatedAt.Add(time.Hour), 100)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "expected=2 verified=1") {
		t.Fatalf("incomplete copy was not rejected atomically: %v", err)
	}
	assertRobotBetArchiveCounts(t, db, 2, 0)
	for id, expected := range snapshots {
		if actual := robotBetHotJSON(t, db, id); actual != expected {
			t.Fatalf("failed archive changed hot evidence %d", id)
		}
	}
}

func TestRobotBetArchivePostgresRestoresLegacySnapshotSchemas(t *testing.T) {
	for _, test := range []struct {
		name, missingKeys, version, reference string
	}{
		{"before-request-and-rule-columns", "request_reference,rule_version", "", ""},
		{"request-before-rule-column", "rule_version", "", "assistant:legacy-request"},
		{"rule-without-request-column", "request_reference", "racing-v2", ""},
		{"before-pc28-financial-columns", "valid_turnover_cents,settlement_odds,user_issue_stake_cents_snapshot,settlement_policy,pc28_gray_push", "racing-v2", "assistant:legacy-finance"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := timingPostgresDatabase(t)
			row := robotBetArchiveOwnedFixture(t, db)
			row.RuleVersion, row.RequestReference = test.version, test.reference
			if strings.Contains(test.missingKeys, "valid_turnover_cents") {
				row.ValidTurnoverCents = nil
				row.SettlementOdds = nil
				row.UserIssueStakeCentsSnapshot = nil
				row.SettlementPolicy = ""
				row.PC28GrayPush = false
			}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			expected := robotBetHotJSON(t, db, row.ID)
			seedRobotBetColdFixture(t, db, row.ID, "archive-old-schema", test.missingKeys, "{}")
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := allowLifecycleDeletes(tx); err != nil {
					return err
				}
				count, err := restoreRobotBetArchive(tx, "archive-old-schema")
				if err == nil && count != 1 {
					return fmt.Errorf("restored %d legacy rows, want 1", count)
				}
				return err
			}); err != nil {
				t.Fatal("restore old snapshot without rewriting its evidence:", err)
			}
			assertRobotBetArchiveCounts(t, db, 1, 0)
			if actual := robotBetHotJSON(t, db, row.ID); actual != expected {
				t.Fatalf("legacy restore changed financial evidence:\n%s\n%s", expected, actual)
			}
		})
	}
}

func TestRobotBetArchivePostgresRejectsInconsistentEvidenceAtomically(t *testing.T) {
	for _, test := range []struct {
		name, missingKeys, version, overrides, rewrittenColumn string
	}{
		{name: "financial-column", version: "racing-v2", overrides: `{"payout_cents":99999}`},
		{name: "version-column", version: "racing-v2", overrides: `{"rule_version":"digits5-v2"}`},
		{name: "source-hash", version: "racing-v2", overrides: `{"row_hash":"00000000000000000000000000000000"}`},
		{name: "legacy-version-cannot-be-invented", missingKeys: "rule_version", overrides: `{"rule_version":"racing-v2"}`},
		{name: "present-request-must-still-match", version: "racing-v2", rewrittenColumn: "request_reference"},
		{name: "present-version-must-still-match", version: "racing-v2", rewrittenColumn: "rule_version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := timingPostgresDatabase(t)
			row := robotBetArchiveOwnedFixture(t, db)
			row.RuleVersion, row.RequestReference = test.version, "assistant:immutable-archive-request"
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			overrides := test.overrides
			if overrides == "" {
				overrides = "{}"
			}
			seedRobotBetColdFixture(t, db, row.ID, "archive-inconsistent", test.missingKeys, overrides)
			if test.rewrittenColumn != "" {
				// Test-only insert hooks simulate accidental future restore code
				// losing an existing field. The final hash must catch both fields.
				if err := db.Exec(fmt.Sprintf(`CREATE FUNCTION fixture_rewrite_restored_bet() RETURNS trigger AS $$
					BEGIN NEW.%s := ''; RETURN NEW; END; $$ LANGUAGE plpgsql;
					CREATE TRIGGER fixture_rewrite_restored_bet BEFORE INSERT ON lottery_bets
					FOR EACH ROW EXECUTE FUNCTION fixture_rewrite_restored_bet()`, test.rewrittenColumn)).Error; err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err := db.Raw(`SELECT to_jsonb(archive)::text FROM lottery_bet_archives archive WHERE id = ?`, row.ID).Scan(&before).Error; err != nil {
				t.Fatal(err)
			}
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := allowLifecycleDeletes(tx); err != nil {
					return err
				}
				_, err := restoreRobotBetArchive(tx, "archive-inconsistent")
				return err
			})
			if err == nil || !strings.Contains(err.Error(), "hash verification failed") {
				t.Fatalf("inconsistent archived evidence restored: %v", err)
			}
			assertRobotBetArchiveCounts(t, db, 0, 1)
			var after string
			if err := db.Raw(`SELECT to_jsonb(archive)::text FROM lottery_bet_archives archive WHERE id = ?`, row.ID).Scan(&after).Error; err != nil || after != before {
				t.Fatalf("failed restore changed immutable archive: %v\n%s\n%s", err, before, after)
			}
		})
	}
}

func robotBetArchiveOwnedFixture(t *testing.T, db *gorm.DB) bet.Bet {
	t.Helper()
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}
	return robotBetArchiveFixture(platform.ID, platform.OwnerUserID, "timing_platform", platform.Scope)
}

func robotBetArchiveFixture(workspaceID, userID uint64, username, roomScope string) bet.Bet {
	old := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	validTurnover := int64(1200)
	settlementOdds := 1.5
	userIssueStake := int64(1000000)
	return bet.Bet{
		WorkspaceID: workspaceID, UserID: userID, Username: username, RoomScope: roomScope,
		GameID: "speed-racing", Issue: "archive-rule-1001", PlayCode: "ball_1_5", PlayName: "冠军", Position: 1, Selection: "2",
		AmountCents: 1234, Odds: 9.96, ValidTurnoverCents: &validTurnover, SettlementOdds: &settlementOdds,
		UserIssueStakeCentsSnapshot: &userIssueStake, SettlementPolicy: "pc28_archive_fixture", PC28GrayPush: true,
		Status: "won", PayoutCents: 12291, FlyCents: 123,
		RebateRateSnapshot: 1.23, RebateCents: 15, AgentShareRateSnapshot: 5.67, AgentShareCents: 89,
		SettledAt: &old, Remark: "immutable financial fixture", Operator: "archive-fixture",
		ReconciliationStatus: "normal", CreatedAt: old, UpdatedAt: old,
	}
}

func robotBetHotJSON(t *testing.T, db *gorm.DB, id uint64) string {
	t.Helper()
	var snapshot string
	if err := db.Raw(`SELECT to_jsonb(hot)::text FROM lottery_bets hot WHERE id = ?`, id).Scan(&snapshot).Error; err != nil || snapshot == "" {
		t.Fatalf("read isolated hot bet snapshot: %v", err)
	}
	return snapshot
}

func assertRobotBetArchiveCounts(t *testing.T, db *gorm.DB, hotWant, coldWant int64) {
	t.Helper()
	for table, expected := range map[string]int64{"lottery_bets": hotWant, "lottery_bet_archives": coldWant} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != expected {
			t.Fatalf("%s count=%d, want=%d, error=%v", table, count, expected, err)
		}
	}
}

func seedRobotBetColdFixture(t *testing.T, db *gorm.DB, betID uint64, requestID, missingKeys, overrides string) {
	t.Helper()
	// Construct historical/corrupt fixtures directly at INSERT time. Never
	// disable archive immutability guards or update a saved source_json/hash.
	result := db.Exec(`WITH source AS (
		SELECT to_jsonb(hot) AS original,
		       to_jsonb(hot) - string_to_array(?, ',') AS snapshot
		FROM lottery_bets hot WHERE id = ?
	)
	INSERT INTO lottery_bet_archives
	SELECT (jsonb_populate_record(NULL::lottery_bet_archives,
		original || jsonb_build_object('source_json', snapshot, 'row_hash', md5(snapshot::text),
		'archived_at', now(), 'cleanup_request_id', ?::text) || ?::jsonb)).*
	FROM source`, missingKeys, betID, requestID, overrides)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("seed isolated cold snapshot: count=%d error=%v", result.RowsAffected, result.Error)
	}
	if err := allowLifecycleDeletes(db); err != nil {
		t.Fatal(err)
	}
	if result := db.Delete(&bet.Bet{}, betID); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("move isolated fixture out of hot table: count=%d error=%v", result.RowsAffected, result.Error)
	}
}
