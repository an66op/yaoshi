package services

import "testing"

func TestParseReportPeriod(t *testing.T) {
	period, err := parseReportPeriod("2026-08-01", "2026-08-07")
	if err != nil {
		t.Fatalf("parse period: %v", err)
	}
	if period.StartDate != "2026-08-01" || period.EndDate != "2026-08-07" {
		t.Fatalf("unexpected display range: %s - %s", period.StartDate, period.EndDate)
	}
	if days := period.End.Sub(period.Start).Hours() / 24; days != 7 {
		t.Fatalf("expected 7 days, got %.0f", days)
	}
}

func TestParseReportPeriodRejectsInvalidRange(t *testing.T) {
	if _, err := parseReportPeriod("2026-08-09", "2026-08-08"); err == nil {
		t.Fatal("expected invalid range to be rejected")
	}
	if _, err := parseReportPeriod("2026-01-01", "2026-05-01"); err == nil {
		t.Fatal("expected excessive range to be rejected")
	}
}

func TestValidateLedgerType(t *testing.T) {
	for _, value := range []string{"", "all", "credit", "debit", "manual", "application_credit", "application_debit"} {
		if err := validateLedgerType(value); err != nil {
			t.Fatalf("%q should be valid: %v", value, err)
		}
	}
	if err := validateLedgerType("unknown"); err == nil {
		t.Fatal("unknown type must be rejected")
	}
}
