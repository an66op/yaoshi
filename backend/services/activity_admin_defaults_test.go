package services

import (
	"strings"
	"testing"
)

func TestWorkspaceDefaultActivitiesDoNotFabricateParticipants(t *testing.T) {
	defaults := workspaceDefaultActivities()
	seen := map[string]bool{}
	for _, item := range defaults {
		if item.Type != "checkin" && item.Type != "redpacket" {
			continue
		}
		seen[item.Type] = true
		if item.Participants != 0 {
			t.Fatalf("default %s participants = %d, want actual count starting at zero", item.Type, item.Participants)
		}
	}
	if !seen["checkin"] || !seen["redpacket"] {
		t.Fatalf("required activity defaults missing: %#v", seen)
	}
}

func TestLegacySeedParticipantReconcileUsesActualCountAndPreservesEditedCounters(t *testing.T) {
	for _, required := range []string{
		"COUNT(participation.id)::bigint AS actual_participants",
		"participation.activity_id = activity.id",
		"CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END",
		"+ COUNT(participation.id)",
		"SET participants = legacy_candidate.actual_participants",
		"legacy_candidate.legacy_base + legacy_candidate.actual_participants",
		"config_json =",
		"activity.deleted_at IS NULL",
	} {
		if !strings.Contains(legacySeedParticipantsReconcileSQL, required) {
			t.Fatalf("legacy seed reconcile missing safety constraint %q", required)
		}
	}
	for _, forbidden := range []string{"NOT EXISTS", "participants >", "participants <>", "participants !="} {
		if strings.Contains(legacySeedParticipantsReconcileSQL, forbidden) {
			t.Fatalf("legacy seed predicate is too broad: %q", forbidden)
		}
	}

	for _, tc := range []struct {
		name       string
		legacyBase int64
		stored     int64
		actual     int64
		wantRepair bool
		want       int64
	}{
		{name: "unused legacy seed", legacyBase: 128, stored: 128, actual: 0, wantRepair: true, want: 0},
		{name: "legacy seed with real participation", legacyBase: 56, stored: 59, actual: 3, wantRepair: true, want: 3},
		{name: "operator edited counter", legacyBase: 128, stored: 140, actual: 3, wantRepair: false, want: 140},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, repaired := legacySeedParticipantCorrection(tc.legacyBase, tc.stored, tc.actual)
			if repaired != tc.wantRepair || got != tc.want {
				t.Fatalf("correction(%d, %d, %d) = (%d, %v), want (%d, %v)", tc.legacyBase, tc.stored, tc.actual, got, repaired, tc.want, tc.wantRepair)
			}
		})
	}
}

// legacySeedParticipantCorrection models the equality guard embedded in the
// single-statement SQL above so the zero, non-zero, and edited-counter cases
// remain explicit regression contracts.
func legacySeedParticipantCorrection(legacyBase, stored, actual int64) (int64, bool) {
	if stored != legacyBase+actual {
		return stored, false
	}
	return actual, true
}
