package migrations

import (
	"regexp"
	"strings"
	"testing"
)

func TestSGSSCHistoryBackfillMigrationKeepsDurableQueueAndAttemptEvidence(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609030004_sgssc_history_backfill.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{
		"CREATE TABLE lottery_sgssc_backfill_items (",
		"issue varchar(11) PRIMARY KEY CHECK (issue ~ '^[0-9]{11}$')",
		"CREATE TABLE lottery_sgssc_backfill_attempts (",
		"REFERENCES lottery_sgssc_backfill_items(issue) ON DELETE RESTRICT",
		"UNIQUE (issue, attempt)",
		"idx_sgssc_backfill_due ON lottery_sgssc_backfill_items (next_retry_at, draw_at)",
		"WHERE status IN ('pending', 'retry', 'settlement_retry')",
		"idx_sgssc_backfill_lease ON lottery_sgssc_backfill_items (lease_until)",
		"WHERE status = 'running'",
		"idx_sgssc_backfill_attempt_issue ON lottery_sgssc_backfill_attempts (issue, id DESC)",
		"CHECK ((status = 'running' AND finished_at IS NULL) OR (status <> 'running' AND finished_at IS NOT NULL))",
		"IF TG_OP = 'DELETE' THEN RAISE EXCEPTION",
		"IF OLD.status <> 'running'",
		"NEW.id, NEW.issue, NEW.attempt, NEW.trigger, NEW.operator, NEW.request_id, NEW.started_at, NEW.source_revision, NEW.conversion_revision",
		"OLD.id, OLD.issue, OLD.attempt, OLD.trigger, OLD.operator, OLD.request_id, OLD.started_at, OLD.source_revision, OLD.conversion_revision",
		"OR (OLD.imported AND NOT NEW.imported)",
		"RETURN NEW;",
		"BEFORE UPDATE OR DELETE ON lottery_sgssc_backfill_attempts",
		"FOR EACH ROW EXECUTE FUNCTION guard_sgssc_backfill_attempt()",
		"SELECT public.install_application_truncate_guards();",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("SG backfill migration omitted durable safety contract %q", fragment)
		}
	}
	// Running attempts must be able to save an import receipt and finish once;
	// only already-finished rows and frozen identities reject every update.
	if strings.Contains(sql, "IF OLD.status = 'running'") || strings.Contains(sql, "IF TG_OP = 'UPDATE' THEN RAISE EXCEPTION") {
		t.Fatal("migration prevents running attempts from recording their result")
	}
	if regexp.MustCompile(`(?im)^\s*(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM|TRUNCATE|ALTER\s+TABLE|DROP\s+TABLE)\s`).Match(raw) {
		t.Fatal("SG backfill migration must not rewrite existing business tables or seed recovery work")
	}
}
