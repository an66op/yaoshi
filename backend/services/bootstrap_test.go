package services

import (
	"reflect"
	"testing"
)

func TestReleaseBootstrapExcludesLocalFixtures(t *testing.T) {
	steps, err := bootstrapSteps("release")
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryCatalog, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("release bootstrap = %#v, want %#v", steps, want)
	}
}

func TestDebugBootstrapNeverSeedsPlanRecommendations(t *testing.T) {
	steps, err := bootstrapSteps("debug")
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryDebug, bootstrapExperienceAccount, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("debug bootstrap = %#v, want %#v", steps, want)
	}
}

func TestValidateBootstrapAdminPassword(t *testing.T) {
	for name, password := range map[string]string{
		"default":   "123456",
		"demo":      demoPassword,
		"short":     "Aa1!too-short",
		"no-symbol": "LongPassword12345",
		"username":  "RootAdmin.Safe1!",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateBootstrapAdminPassword("rootadmin", password); err == nil {
				t.Fatalf("expected %q to be rejected", password)
			}
		})
	}
	if err := ValidateBootstrapAdminPassword("rootadmin", "V7!mQ2#zL9@pR4$x"); err != nil {
		t.Fatalf("strong password rejected: %v", err)
	}
}

func TestDebugPlanTemplatesHaveUniqueRoomIdentity(t *testing.T) {
	seen := map[string]struct{}{}
	games := map[string]struct{}{}
	for _, template := range debugPlanTemplates {
		key := template.GameID + "\x00" + template.MasterName
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate debug plan template %q", key)
		}
		seen[key] = struct{}{}
		games[template.GameID] = struct{}{}
	}
	for _, gameID := range []string{"speed-racing", "canada-28", "au-lucky-10"} {
		if _, exists := games[gameID]; !exists {
			t.Fatalf("missing plan template game %q", gameID)
		}
	}
}
