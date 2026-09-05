package services

import (
	"reflect"
	"testing"
)

func TestReleaseBootstrapExcludesLocalFixtures(t *testing.T) {
	steps, err := bootstrapSteps(BootstrapOptions{Mode: "release"})
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryCatalog, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("release bootstrap = %#v, want %#v", steps, want)
	}
}

func TestDebugBootstrapExcludesOptionalFixturesByDefault(t *testing.T) {
	steps, err := bootstrapSteps(BootstrapOptions{Mode: "debug"})
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryCatalog, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("debug bootstrap = %#v, want %#v", steps, want)
	}
}

func TestDebugBootstrapIncludesExperienceAccountsOnlyWhenExplicitlyEnabled(t *testing.T) {
	steps, err := bootstrapSteps(BootstrapOptions{Mode: "debug", SeedExperienceAccounts: true})
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryCatalog, bootstrapExperienceAccount, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("explicit debug bootstrap = %#v, want %#v", steps, want)
	}
}

func TestDebugBootstrapIncludesDeterministicLotteryHistoryOnlyWhenExplicitlyEnabled(t *testing.T) {
	steps, err := bootstrapSteps(BootstrapOptions{Mode: "debug", SeedDeterministicLotteryHistory: true})
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryDebug, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("explicit deterministic-history bootstrap = %#v, want %#v", steps, want)
	}
}

func TestDebugBootstrapFixtureFlagsAreIndependent(t *testing.T) {
	steps, err := bootstrapSteps(BootstrapOptions{
		Mode:                            "debug",
		SeedExperienceAccounts:          true,
		SeedDeterministicLotteryHistory: true,
	})
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	want := []bootstrapStep{bootstrapAdmin, bootstrapLotteryDebug, bootstrapExperienceAccount, bootstrapWorkspaces, bootstrapBaseCatalogs}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("combined explicit debug bootstrap = %#v, want %#v", steps, want)
	}
}

func TestNonDebugBootstrapRejectsDebugOnlyFixtures(t *testing.T) {
	tests := []struct {
		name    string
		options BootstrapOptions
	}{
		{name: "experience accounts", options: BootstrapOptions{SeedExperienceAccounts: true}},
		{name: "deterministic lottery history", options: BootstrapOptions{SeedDeterministicLotteryHistory: true}},
	}
	for _, mode := range []string{"test", "release"} {
		for _, tt := range tests {
			t.Run(mode+"/"+tt.name, func(t *testing.T) {
				tt.options.Mode = mode
				if _, err := bootstrapSteps(tt.options); err == nil {
					t.Fatalf("%s bootstrap accepted debug-only %s", mode, tt.name)
				}
			})
		}
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
	if _, exists := games["canada-28"]; !exists {
		t.Fatal("missing non-racing debug plan template")
	}
	for _, gameID := range racingPlanGameIDs {
		if _, exists := games[gameID]; exists {
			t.Fatalf("racing-v2 debug template %q would create a generic publication hidden from members", gameID)
		}
	}
}
