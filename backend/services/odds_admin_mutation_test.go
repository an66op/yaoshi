package services

import (
	"backend/data/models/odds"
	apperrors "backend/errors"
	"math"
	"testing"
	"time"
)

func TestPlatformOddsRequireExplicitCurrentRuleAdminSave(t *testing.T) {
	base := odds.PlayLimit{Odds: 1.993, ExplicitlyConfigured: true, RuleVersion: "racing-v2", ConfigurationSource: oddsSourceAdminSave}
	if !isActivePlatformOdds(base, "racing-v2") {
		t.Fatal("explicit current-rule quote is unavailable")
	}
	for name, mutate := range map[string]func(*odds.PlayLimit){
		"missing confirmation": func(row *odds.PlayLimit) { row.ExplicitlyConfigured = false },
		"missing version":      func(row *odds.PlayLimit) { row.RuleVersion = "" },
		"retired version":      func(row *odds.PlayLimit) { row.RuleVersion = "racing-v1" },
		"legacy compatible":    func(row *odds.PlayLimit) { row.ConfigurationSource = "legacy_compatible" },
		"system default":       func(row *odds.PlayLimit) { row.ConfigurationSource = "system_default" },
		"unknown source":       func(row *odds.PlayLimit) { row.ConfigurationSource = "imported" },
		"disabled":             func(row *odds.PlayLimit) { row.Odds = 0 },
		"rounded to one":       func(row *odds.PlayLimit) { row.Odds = 1.00001 },
		"nan":                  func(row *odds.PlayLimit) { row.Odds = math.NaN() },
		"infinite":             func(row *odds.PlayLimit) { row.Odds = math.Inf(1) },
		"overflow":             func(row *odds.PlayLimit) { row.Odds = math.MaxFloat64 },
	} {
		t.Run(name, func(t *testing.T) {
			row := base
			mutate(&row)
			if isActivePlatformOdds(row, "racing-v2") {
				t.Fatalf("unconfirmed/invalid row activated: %+v", row)
			}
		})
	}
}

func TestOddsMutationGuardRequiresCurrentRuleAndRevision(t *testing.T) {
	for _, test := range []struct{ version, revision, code string }{
		{"", "", "INVALID_REQUEST"},
		{"racing-v2", "", "INVALID_REQUEST"},
		{"digits5-v2", "revision", "RULE_VERSION_CONFLICT"},
		{"racing-v2", "revision", ""},
	} {
		if got := apperrors.GetErrorCode(validateOddsMutationGuard("racing-v2", test.version, test.revision)); test.code != "" && got != test.code {
			t.Fatalf("guard %q/%q returned %q, want %q", test.version, test.revision, got, test.code)
		} else if test.code == "" && validateOddsMutationGuard("racing-v2", test.version, test.revision) != nil {
			t.Fatal("current nonempty guard rejected")
		}
	}
}

func TestNormalizeOddsLimitsRequiresCompleteUniqueValidatedCatalogue(t *testing.T) {
	catalog := []PlayCatalogItem{{PlayCode: "first", PlayName: "服务器名称", SortOrder: 4}, {PlayCode: "second", PlayName: "第二项", SortOrder: 7}}
	base := []PlayLimitItem{
		{PlayCode: "first", PlayName: "客户端名称", Odds: 1.98765, MinBet: 1, MaxBet: 20, MaxUserPeriod: 40, MaxPeriodTotal: 80},
		{PlayCode: "second", Odds: 0, MinBet: 1, MaxBet: 20, MaxUserPeriod: 40, MaxPeriodTotal: 80},
	}
	normalized, err := normalizeOddsLimitItems(catalog, base)
	if err != nil || len(normalized) != 2 || normalized[0].Odds != 1.9877 || normalized[0].PlayName != "服务器名称" || normalized[1].Odds != 0 {
		t.Fatalf("valid catalogue normalization = %+v / %v", normalized, err)
	}
	for name, mutate := range map[string]func([]PlayLimitItem) []PlayLimitItem{
		"partial catalogue": func(rows []PlayLimitItem) []PlayLimitItem { return rows[:1] },
		"duplicate code":    func(rows []PlayLimitItem) []PlayLimitItem { rows[1].PlayCode = "first"; return rows },
		"unknown code":      func(rows []PlayLimitItem) []PlayLimitItem { rows[1].PlayCode = "unknown"; return rows },
		"negative":          func(rows []PlayLimitItem) []PlayLimitItem { rows[0].Odds = -2; return rows },
		"one":               func(rows []PlayLimitItem) []PlayLimitItem { rows[0].Odds = 1; return rows },
		"nan":               func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MaxBet = math.NaN(); return rows },
		"infinity":          func(rows []PlayLimitItem) []PlayLimitItem { rows[0].Odds = math.Inf(1); return rows },
		"sub-cent limit":    func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MinBet = 1.001; return rows },
		"active zero min":   func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MinBet = 0; return rows },
		"min above max":     func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MinBet = 21; return rows },
		"max above user":    func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MaxBet = 41; return rows },
		"user above room":   func(rows []PlayLimitItem) []PlayLimitItem { rows[0].MaxUserPeriod = 81; return rows },
	} {
		t.Run(name, func(t *testing.T) {
			rows := mutate(append([]PlayLimitItem(nil), base...))
			if _, err := normalizeOddsLimitItems(catalog, rows); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
				t.Fatalf("invalid batch accepted: %+v / %v", rows, err)
			}
		})
	}
}

func TestOddsRevisionChangesEvenWhenEmptyStateReturns(t *testing.T) {
	items := []PlayLimitItem{{PlayCode: "two_sided", Odds: 0}}
	base := oddsConfigRevision("racing-v2", 0, items)
	if base != oddsConfigRevision("racing-v2", 0, items) {
		t.Fatal("unchanged snapshot has unstable revision")
	}
	if base == oddsConfigRevision("racing-v2", 1, items) || base == oddsConfigRevision("racing-v3", 0, items) {
		t.Fatal("revision ignores mutation counter or rules version")
	}
	stamp := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	items[0].ConfiguredAt = &stamp
	if base == oddsConfigRevision("racing-v2", 0, items) {
		t.Fatal("revision ignores confirmation metadata")
	}
}
