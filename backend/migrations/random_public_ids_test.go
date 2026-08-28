package migrations

import (
	"strings"
	"testing"
)

func TestRandomPublicIDMigrationPreservesExistingIDsAndSerializesAllocation(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280008_random_public_ids.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION public.next_member_public_id()",
		"RETURNS bigint",
		"pg_advisory_xact_lock(24587624048118084)",
		"FOR attempt IN 1..256 LOOP",
		"1000000 + floor(random() * 9000000)::bigint",
		"WHERE account.public_id = candidate",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_user_public_id",
		"ALTER COLUMN public_id SET DEFAULT public.next_member_public_id()",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("random-public-ID migration is missing %q", fragment)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{
		`UPDATE PUBLIC."USER"`,
		`DELETE FROM PUBLIC."USER"`,
		`TRUNCATE PUBLIC."USER"`,
		"NEXTVAL('MEMBER_PUBLIC_ID_SEQ')",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("random-public-ID migration mutates existing IDs or keeps sequential allocation with %q", forbidden)
		}
	}
}
