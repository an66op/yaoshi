package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"context"
	"reflect"
	"testing"
	"time"
)

func sgSSCBackfillPureDraw(issue string) sourceDraw {
	_, _, at, err := parseSGSSCIssue(issue)
	if err != nil {
		panic(err)
	}
	return sourceDraw{Issue: issue, DrawAt: at, Numbers: []int{0, 0, 3, 9, 5}, SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
}

func TestSGSSCBackfillCoverageAcceptsExactlyAccountedPartialProgress(t *testing.T) {
	now := sgSSCHistoryTestNow()
	targets := []string{"20260903003", "20260903001", "20260903002"}
	first, second, third := sgSSCBackfillPureDraw("20260903001"), sgSSCBackfillPureDraw("20260903002"), sgSSCBackfillPureDraw("20260903003")
	for name, result := range map[string]SGSSCHistoryVerification{
		"all verified": {Draws: []sourceDraw{first, second, third}},
		"partial verified": {Draws: []sourceDraw{first, third}, Failures: []SGSSCHistoryFailure{
			{Issue: second.Issue, Error: "115缺少该期"},
		}},
		"all unavailable": {Failures: []SGSSCHistoryFailure{
			{Issue: third.Issue, Error: "双站不一致"}, {Issue: first.Issue, Error: "168缺少该期"}, {Issue: second.Issue, Error: "115缺少该期"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			originalTargets := append([]string(nil), targets...)
			if err := validateSGSSCHistoryCoverage(result, targets, now); err != nil {
				t.Fatalf("explicitly accounted partial result rejected: %v", err)
			}
			if !reflect.DeepEqual(targets, originalTargets) {
				t.Fatal("coverage check mutated original target order")
			}
		})
	}
}

func TestSGSSCBackfillCoverageRejectsMissingDuplicateAndUntrustedEvidence(t *testing.T) {
	now := sgSSCHistoryTestNow()
	targets := []string{"20260903001", "20260903002", "20260903003"}
	base := func() SGSSCHistoryVerification {
		return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPureDraw(targets[0]), sgSSCBackfillPureDraw(targets[2])},
			Failures: []SGSSCHistoryFailure{{Issue: targets[1], Error: "两站不一致"}}}
	}
	for name, mutate := range map[string]func(*SGSSCHistoryVerification){
		"missing all":             func(r *SGSSCHistoryVerification) { *r = SGSSCHistoryVerification{} },
		"missing failure":         func(r *SGSSCHistoryVerification) { r.Failures = nil },
		"missing success":         func(r *SGSSCHistoryVerification) { r.Draws = r.Draws[:1] },
		"empty failure reason":    func(r *SGSSCHistoryVerification) { r.Failures[0].Error = "" },
		"unrequested failure":     func(r *SGSSCHistoryVerification) { r.Failures[0].Issue = "20260903004" },
		"malformed failure issue": func(r *SGSSCHistoryVerification) { r.Failures[0].Issue = "bad" },
		"unrequested success":     func(r *SGSSCHistoryVerification) { r.Draws[1] = sgSSCBackfillPureDraw("20260903004") },
		"duplicate failure":       func(r *SGSSCHistoryVerification) { r.Failures = append(r.Failures, r.Failures[0]) },
		"success also failed": func(r *SGSSCHistoryVerification) {
			r.Failures = append(r.Failures, SGSSCHistoryFailure{Issue: targets[0], Error: "conflicting second outcome"})
		},
		"duplicate success":        func(r *SGSSCHistoryVerification) { r.Draws[1] = r.Draws[0] },
		"reverse success ordering": func(r *SGSSCHistoryVerification) { r.Draws[0], r.Draws[1] = r.Draws[1], r.Draws[0] },
		"wrong source revision":    func(r *SGSSCHistoryVerification) { r.Draws[0].SourceRevision = "platform" },
		"wrong conversion":         func(r *SGSSCHistoryVerification) { r.Draws[0].ConversionRevision = "" },
		"invalid draw time":        func(r *SGSSCHistoryVerification) { r.Draws[0].DrawAt = r.Draws[0].DrawAt.Add(time.Second) },
		"invalid balls":            func(r *SGSSCHistoryVerification) { r.Draws[0].Numbers = []int{1, 2, 3, 4, 10} },
		"live schedule metadata":   func(r *SGSSCHistoryVerification) { r.Draws[0].NextIssue = targets[1] },
	} {
		t.Run(name, func(t *testing.T) {
			result := base()
			mutate(&result)
			if err := validateSGSSCHistoryCoverage(result, targets, now); err == nil {
				t.Fatal("incomplete, contradictory or untrusted result accepted")
			}
		})
	}
	for _, invalidTargets := range [][]string{nil, {targets[0], targets[0]}, {"20260903288"}, {"bad"}} {
		if err := validateSGSSCHistoryCoverage(SGSSCHistoryVerification{}, invalidTargets, now); err == nil {
			t.Fatalf("coverage bypassed invalid target scope: %v", invalidTargets)
		}
	}
}

func TestSGSSCBackfillStoredHistoryRequiresTrustedRawEvidence(t *testing.T) {
	for _, issue := range []string{"20260902288", "20240301001"} {
		draw := sgSSCBackfillPureDraw(issue)
		stored := lottery.Draw{ID: 1, GameID: "sg-ssc", Issue: issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt,
			SourceRevision: draw.SourceRevision, ConversionRevision: draw.ConversionRevision}
		if err := validateSGSSCStoredHistory(stored, issue); err != nil {
			t.Fatalf("valid historical evidence, including older than fetch horizon, rejected: %v", err)
		}
	}
	issue := "20260902288"
	draw := sgSSCBackfillPureDraw(issue)
	base := lottery.Draw{ID: 1, GameID: "sg-ssc", Issue: issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt,
		SourceRevision: draw.SourceRevision, ConversionRevision: draw.ConversionRevision}
	for name, mutate := range map[string]func(*lottery.Draw){
		"wrong game":       func(d *lottery.Draw) { d.GameID = "bingo-ssc-1" },
		"wrong issue":      func(d *lottery.Draw) { d.Issue = "20260903001" },
		"wrong draw time":  func(d *lottery.Draw) { d.DrawAt = d.DrawAt.Add(time.Second) },
		"legacy source":    func(d *lottery.Draw) { d.SourceRevision = "" },
		"legacy convert":   func(d *lottery.Draw) { d.ConversionRevision = "platform" },
		"future revision":  func(d *lottery.Draw) { d.SourceRevision = "sgssc-168-115-v2" },
		"empty numbers":    func(d *lottery.Draw) { d.Numbers = "" },
		"four numbers":     func(d *lottery.Draw) { d.Numbers = "0,0,3,9" },
		"six numbers":      func(d *lottery.Draw) { d.Numbers = "0,0,3,9,5,1" },
		"skipped garbage":  func(d *lottery.Draw) { d.Numbers = "0,0,garbage,3,9,5" },
		"empty extra ball": func(d *lottery.Draw) { d.Numbers = "0,0,,3,9,5" },
		"trailing comma":   func(d *lottery.Draw) { d.Numbers = "0,0,3,9,5," },
		"negative ball":    func(d *lottery.Draw) { d.Numbers = "-1,0,3,9,5" },
		"negative zero":    func(d *lottery.Draw) { d.Numbers = "-0,0,3,9,5" },
		"overflow ball":    func(d *lottery.Draw) { d.Numbers = "10,0,3,9,5" },
		"integer overflow": func(d *lottery.Draw) { d.Numbers = "999999999999999999999,0,3,9,5" },
		"fractional ball":  func(d *lottery.Draw) { d.Numbers = "0.0,0,3,9,5" },
		"signed ball":      func(d *lottery.Draw) { d.Numbers = "+0,0,3,9,5" },
		"exponent ball":    func(d *lottery.Draw) { d.Numbers = "0e0,0,3,9,5" },
		"nonascii digit":   func(d *lottery.Draw) { d.Numbers = "０,0,3,9,5" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := base
			mutate(&copy)
			if err := validateSGSSCStoredHistory(copy, issue); apperrors.GetErrorCode(err) != "SG_HISTORY_CONFLICT" {
				t.Fatalf("invalid stored evidence must remain isolated: %v", err)
			}
		})
	}
	if err := validateSGSSCStoredHistory(base, "20260902000"); apperrors.GetErrorCode(err) != "SG_HISTORY_CONFLICT" {
		t.Fatalf("invalid requested issue accepted: %v", err)
	}
}

func TestSGSSCBackfillPureBoundsAndTargetOrdering(t *testing.T) {
	now := sgSSCHistoryTestNow()
	for _, test := range []struct {
		attempts int
		minutes  int
	}{{0, 5}, {1, 5}, {2, 10}, {3, 20}, {6, 160}, {7, 320}, {8, 320}, {10000, 320}} {
		want := now.Add(time.Duration(test.minutes) * time.Minute)
		if got := sgSSCBackfillRetryAt(now, test.attempts); !got.Equal(want) {
			t.Fatalf("attempts=%d retry=%v want=%v", test.attempts, got, want)
		}
	}
	items := []lottery.SGSSCBackfillItem{{Issue: "20260903001"}, {Issue: "20260902288"}, {Issue: "20260902285"}}
	original := append([]lottery.SGSSCBackfillItem(nil), items...)
	got := sortedSGSSCHistoryTargets(items)
	want := []string{"20260902285", "20260902288", "20260903001"}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(items, original) {
		t.Fatalf("target extraction mutated claims or misordered midnight: got=%v claims=%+v", got, items)
	}
	if len(sortedSGSSCHistoryTargets(nil)) != 0 {
		t.Fatal("empty claims manufactured a target")
	}
}

func TestSGSSCBackfillNilDependenciesFailBeforeDatabaseWork(t *testing.T) {
	service := &LotteryService{} // Deliberately no database: validation must precede I/O.
	clock := func() time.Time { return sgSSCHistoryTestNow() }
	fetch := func(context.Context, []string) (SGSSCHistoryVerification, error) {
		t.Fatal("missing dependency reached upstream fetch")
		return SGSSCHistoryVerification{}, nil
	}
	for _, call := range []func() (sgSSCBackfillRun, error){
		func() (sgSSCBackfillRun, error) { return service.runSGSSCBackfill(nil, clock, fetch) },
		func() (sgSSCBackfillRun, error) { return service.runSGSSCBackfill(context.Background(), nil, fetch) },
		func() (sgSSCBackfillRun, error) { return service.runSGSSCBackfill(context.Background(), clock, nil) },
	} {
		if result, err := call(); err == nil || result != (sgSSCBackfillRun{}) {
			t.Fatalf("nil dependency did not fail closed: %+v err=%v", result, err)
		}
	}
}
