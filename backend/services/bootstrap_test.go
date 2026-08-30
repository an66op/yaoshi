package services

import "testing"

func containsBootstrapStep(steps []bootstrapStep, wanted bootstrapStep) bool {
	for _, step := range steps {
		if step == wanted {
			return true
		}
	}
	return false
}

func TestReleaseBootstrapExcludesLocalFixtures(t *testing.T) {
	steps, err := bootstrapSteps("release")
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	for _, forbidden := range []bootstrapStep{bootstrapLotteryDebug, bootstrapExperienceAccount, bootstrapDebugPlans} {
		if containsBootstrapStep(steps, forbidden) {
			t.Fatalf("release bootstrap unexpectedly contains %q: %#v", forbidden, steps)
		}
	}
	for _, required := range []bootstrapStep{bootstrapAdmin, bootstrapLotteryCatalog, bootstrapWorkspaces, bootstrapBaseCatalogs} {
		if !containsBootstrapStep(steps, required) {
			t.Fatalf("release bootstrap is missing %q: %#v", required, steps)
		}
	}
}

func TestDebugBootstrapNeverSeedsPlanRecommendations(t *testing.T) {
	steps, err := bootstrapSteps("debug")
	if err != nil {
		t.Fatalf("bootstrapSteps() error = %v", err)
	}
	for _, required := range []bootstrapStep{bootstrapLotteryDebug, bootstrapExperienceAccount, bootstrapWorkspaces, bootstrapBaseCatalogs} {
		if !containsBootstrapStep(steps, required) {
			t.Fatalf("debug bootstrap is missing %q: %#v", required, steps)
		}
	}
	if containsBootstrapStep(steps, bootstrapDebugPlans) {
		t.Fatalf("debug bootstrap must not publish unrequested room recommendations: %#v", steps)
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
