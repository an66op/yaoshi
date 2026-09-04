package services

import (
	"context"
	"os"
	"testing"
	"time"
)

// Explicit opt-in, read-only public-source probe. No database, wager or source
// binding is read or written. Upstream availability is not an offline CI gate.
func TestSGSSCBackfillLiveHistoricalTargets(t *testing.T) {
	if os.Getenv("SGSSC_HISTORY_LIVE_TEST") != "1" {
		t.Skip("set SGSSC_HISTORY_LIVE_TEST=1 for the read-only 168/115 historical probe")
	}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	date := time.Now().In(zone).AddDate(0, 0, -1).Format("20060102")
	issues := []string{date + "072", date + "073", date + "085"}
	result, err := fetchSGSSCVerifiedHistory(context.Background(), issues)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSGSSCHistoryCoverage(result, issues, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(result.Draws) == 0 {
		t.Fatalf("no target has dual-station evidence: %+v", result.Failures)
	}
	for _, draw := range result.Draws {
		t.Logf("verified issue=%s time=%s numbers=%s", draw.Issue, draw.DrawAt.In(zone).Format(time.RFC3339), joinNumbers(draw.Numbers))
	}
	for _, failure := range result.Failures {
		t.Logf("unresolved issue=%s reason=%s", failure.Issue, failure.Error)
	}
}
