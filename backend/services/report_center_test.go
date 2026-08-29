package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"reflect"
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

func TestReportCSVValueNeutralizesSpreadsheetFormulaPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "equals", value: "=SUM(A1:A2)", want: "'=SUM(A1:A2)"},
		{name: "plus after spaces", value: "   +1+1", want: "'   +1+1"},
		{name: "minus after tab", value: "\t-2+3", want: "'\t-2+3"},
		{name: "at after line breaks", value: "\r\n@SUM(A1:A2)", want: "'\r\n@SUM(A1:A2)"},
		{name: "equals after unicode space", value: "\u00a0=1+1", want: "'\u00a0=1+1"},
		{name: "plus after byte order mark", value: "\ufeff+1+1", want: "'\ufeff+1+1"},
		{name: "at after zero width format", value: "\u200b@SUM(A1:A2)", want: "'\u200b@SUM(A1:A2)"},
		{name: "equals after nul", value: "\x00=1+1", want: "'\x00=1+1"},
		{name: "byte text", value: []byte("\t=cmd"), want: "'\t=cmd"},
		{name: "ordinary text", value: "  member-01", want: "  member-01"},
		{name: "existing apostrophe", value: "'=SUM(A1:A2)", want: "'=SUM(A1:A2)"},
		{name: "only whitespace", value: " \t\r\n", want: " \t\r\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := reportCSVValue(test.value); got != test.want {
				t.Fatalf("reportCSVValue(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestReportCSVValuePreservesNumericCellSemantics(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{value: int(-7), want: "-7"},
		{value: int64(-9007199254740991), want: "-9007199254740991"},
		{value: uint64(42), want: "42"},
		{value: float64(-12.5), want: "-12.50"},
		{value: float32(3.25), want: "3.25"},
	}
	for _, test := range tests {
		if got := reportCSVValue(test.value); got != test.want {
			t.Errorf("numeric %T(%v) = %q, want %q", test.value, test.value, got, test.want)
		}
		if strings.HasPrefix(reportCSVValue(test.value), "'") {
			t.Errorf("numeric %T(%v) was converted to text", test.value, test.value)
		}
	}
}

func TestAdminReportCSVNeutralizesHeadersAndRows(t *testing.T) {
	result := &ReportCenterResult{
		Columns: []ReportColumn{
			{Key: "actor", Label: "=malicious-header"},
			{Key: "amount", Label: "金额"},
			{Key: "request", Label: "请求"},
		},
		Items: []map[string]any{
			{"actor": " \t@cmd", "amount": int64(-1250), "request": "normal"},
			{"actor": "\r\n=HYPERLINK(\"https://evil.invalid\")", "amount": float64(-12.5), "request": "+formula"},
		},
	}
	var output bytes.Buffer
	if err := WriteReportCSV(&output, result); err != nil {
		t.Fatalf("WriteReportCSV() error = %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(output.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV output: %v", err)
	}
	want := [][]string{
		{"'=malicious-header", "金额", "请求"},
		{"' \t@cmd", "-1250", "normal"},
		// encoding/csv canonicalizes CRLF inside a field to LF; the leading
		// apostrophe remains the security boundary after parsing.
		{"'\n=HYPERLINK(\"https://evil.invalid\")", "-12.50", "'+formula"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("CSV records = %#v, want %#v", records, want)
	}
}
