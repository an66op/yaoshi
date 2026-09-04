package services

import (
	"context"
	"os"
	"testing"
)

// Explicit opt-in only. No database/config bootstrap, no source binding or
// history imports. Four named read-only probes, never a full catalog crawl.
func TestSourceDiagnosticsLiveReadOnly(t *testing.T) {
	if os.Getenv("SOURCE_DIAGNOSTICS_LIVE") != "1" {
		t.Skip("set SOURCE_DIAGNOSTICS_LIVE=1 for four read-only public-source probes")
	}
	for _, test := range []struct{ key, status string }{{"163:57", "success"}, {"163:169", "success"}, {"163:37", "stale"}, {"163:60", "empty"}} {
		t.Run(test.key, func(t *testing.T) {
			result := NewLotteryService(nil).ProbeSource(context.Background(), test.key)
			issue := ""
			if result.Issue != nil {
				issue = *result.Issue
			}
			t.Logf("source=%s status=%s issue=%s numbers=%v history_count=%d duration_ms=%d message=%s", test.key, result.Status, issue, result.Numbers, result.HistoryCount, result.DurationMS, result.Message)
			if result.Status != test.status {
				t.Fatalf("expected %s; diagnostic=%s", test.status, result.Message)
			}
		})
	}
}
