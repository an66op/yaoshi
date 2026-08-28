package services

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportCatalogContainsSixteenUniqueReports(t *testing.T) {
	catalog := ReportCatalog()
	if len(catalog) != 16 {
		t.Fatalf("expected 16 reports, got %d", len(catalog))
	}
	seen := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		if item.Key == "" || item.Title == "" || item.Group == "" {
			t.Fatalf("incomplete report definition: %#v", item)
		}
		if _, exists := seen[item.Key]; exists {
			t.Fatalf("duplicate report key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
	}
	for _, required := range []string{"summary", "users", "entertainment", "28", "categories", "unsettled", "financial", "commission", "redpackets", "rebates", "entertainment-rebates", "28-rebates", "alerts", "new-members", "daily-members", "logs"} {
		if _, exists := seen[required]; !exists {
			t.Fatalf("missing report %q", required)
		}
	}
}

func TestPC28GameCatalogIsRestricted(t *testing.T) {
	want := map[string]bool{"pc-canada": true, "canada-28": true, "canada-20": true}
	if len(pc28GameIDs) != len(want) {
		t.Fatalf("expected exactly three 28 games, got %v", pc28GameIDs)
	}
	for _, gameID := range pc28GameIDs {
		if !want[gameID] {
			t.Fatalf("unexpected 28 game %q", gameID)
		}
	}
}

func TestReportCenterResultCollectionsEncodeAsArrays(t *testing.T) {
	definition, ok := reportDefinition("28")
	if !ok {
		t.Fatal("28 report definition is missing")
	}
	result := newReportCenterResult(definition, reportPeriod{
		StartDate: "2026-08-21", EndDate: "2026-08-27",
	}, ReportCenterFilter{Page: 1, PageSize: 20})
	if result == nil {
		t.Fatal("empty report result must be an object, got nil")
	}
	if result.Metrics == nil || result.Columns == nil || result.Items == nil {
		t.Fatalf("empty report collections must be allocated: %#v", result)
	}
	if result.Total != 0 || result.Page != 1 || result.PageSize != 20 || result.Key != "28" {
		t.Fatalf("empty report zero values are unstable: %#v", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal report result: %v", err)
	}
	for _, field := range []string{`"metrics":[]`, `"columns":[]`, `"items":[]`} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("expected %s in %s", field, payload)
		}
	}
	if string(payload) == "null" {
		t.Fatal("empty report encoded as null")
	}
}
