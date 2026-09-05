package main

import (
	"backend/services"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseAuditOptionsFailsClosedForUnknownDuplicateAndEmptyVersions(t *testing.T) {
	for _, arguments := range [][]string{
		{"--target-read-versions=3"},
		{"--target-read-versions=1,1"},
		{"--target-read-versions="},
		{"--require-unused-previous-key=-1"},
		{"--execute-rewrap"},
		{"--rewrap-previous-key=1", "--require-unused-previous-key=1"},
		{"--rewrap-previous-key=1", "--rewrap-batch-size=1001"},
		{"unexpected"},
	} {
		if _, err := parseAuditOptions(arguments, &bytes.Buffer{}); err == nil {
			t.Fatalf("unsafe arguments were accepted: %v", arguments)
		}
	}
	options, err := parseAuditOptions([]string{
		"--target-read-versions=2,1",
		"--target-supports-previous-keys=false",
		"--require-unused-previous-key=2",
	}, &bytes.Buffer{})
	if err != nil || !reflect.DeepEqual(options.targetReadVersions, []int{2, 1}) ||
		options.targetPreviousKeySupport || options.requireUnusedPreviousKey != 2 {
		t.Fatalf("parsed options=%+v err=%v", options, err)
	}
}

func TestValidateMaintenanceFlagRejectsMissingSymlinkAndWritableMarker(t *testing.T) {
	directory := t.TempDir()
	owner := uint32(os.Getuid())
	marker := filepath.Join(directory, "maintenance")
	if err := validateMaintenanceFlag(marker, owner); err == nil {
		t.Fatal("missing maintenance marker was accepted")
	}
	if err := os.WriteFile(marker, []byte("maintenance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMaintenanceFlag(marker, owner); err != nil {
		t.Fatalf("secure maintenance marker rejected: %v", err)
	}
	if err := os.Chmod(marker, 0o622); err != nil {
		t.Fatal(err)
	}
	if err := validateMaintenanceFlag(marker, owner); err == nil {
		t.Fatal("writable maintenance marker was accepted")
	}
	link := filepath.Join(directory, "maintenance-link")
	if err := os.Symlink(marker, link); err != nil {
		t.Fatal(err)
	}
	if err := validateMaintenanceFlag(link, owner); err == nil {
		t.Fatal("maintenance symlink was accepted")
	}
}

func TestValidateFreezeProofDirectoryRejectsWritableAndForgedProofs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "credentials")
	unit := filepath.Join(root, "wangzhe-field-encryption-rewrap-123.service")
	if err := os.MkdirAll(unit, 0o700); err != nil {
		t.Fatal(err)
	}
	proof := filepath.Join(unit, "freeze-proof")
	if err := os.WriteFile(proof, []byte("backend-writes-frozen-v1\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(unit, 0o700)
		_ = os.Chmod(proof, 0o600)
	})
	if err := os.Chmod(unit, 0o500); err != nil {
		t.Fatal(err)
	}
	owner := uint32(os.Getuid())
	if err := validateFreezeProofDirectory(unit, root, owner); err != nil {
		t.Fatalf("valid freeze proof rejected: %v", err)
	}
	if err := os.Chmod(unit, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateFreezeProofDirectory(unit, root, owner); err == nil {
		t.Fatal("writable credential directory was accepted")
	}
	if err := os.Chmod(unit, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(proof, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateFreezeProofDirectory(unit, root, owner); err == nil {
		t.Fatal("writable freeze proof was accepted")
	}
	if err := os.WriteFile(proof, []byte("forged\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(proof, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := validateFreezeProofDirectory(unit, root, owner); err == nil {
		t.Fatal("forged freeze proof was accepted")
	}
}

func TestEvaluateSensitiveAuditRejectsOldKeyV1AndNewV2IncompatibleTarget(t *testing.T) {
	report := &services.SensitiveFieldReadinessReport{
		Complete:                true,
		Counts:                  services.SensitiveEnvelopeCounts{Total: 2, V1: 1, V2: 1, PrimaryKey: 1, PreviousKey: 1},
		PreviousKeyDependencies: []services.SensitivePreviousKeyDependency{{PreviousKeyIndex: 1, Total: 1, V1: 1}},
	}
	oldTarget := evaluateSensitiveAudit(report, auditOptions{targetReadVersions: []int{1}})
	if oldTarget.Status != "not_ready" || oldTarget.Compatibility == nil || oldTarget.Compatibility.Compatible ||
		!reflect.DeepEqual(oldTarget.Compatibility.Reasons, []string{"v2_not_readable", "previous_key_not_readable"}) {
		t.Fatalf("old target accepted current database: %+v", oldTarget)
	}
	currentTarget := evaluateSensitiveAudit(report, auditOptions{
		targetReadVersions: []int{1, 2}, targetPreviousKeySupport: true,
	})
	if currentTarget.Status != "ready" || currentTarget.Compatibility == nil || !currentTarget.Compatibility.Compatible {
		t.Fatalf("compatible target rejected: %+v", currentTarget)
	}
}

func TestEvaluateSensitiveAuditRequiresPreviousKeyDependencyToReachZero(t *testing.T) {
	report := &services.SensitiveFieldReadinessReport{
		Complete:                true,
		Counts:                  services.SensitiveEnvelopeCounts{Total: 1, V2: 1, PreviousKey: 1},
		PreviousKeyDependencies: []services.SensitivePreviousKeyDependency{{PreviousKeyIndex: 1, Total: 1, V2: 1}},
	}
	blocked := evaluateSensitiveAudit(report, auditOptions{
		targetReadVersions: []int{1, 2}, targetPreviousKeySupport: true, requireUnusedPreviousKey: 1,
	})
	if blocked.Status != "not_ready" || blocked.PreviousKeyRemovalAllowed == nil || *blocked.PreviousKeyRemovalAllowed {
		t.Fatalf("in-use previous key was removable: %+v", blocked)
	}
	report.Counts.PreviousKey = 0
	report.Counts.PrimaryKey = 1
	report.PreviousKeyDependencies = nil
	allowed := evaluateSensitiveAudit(report, auditOptions{
		targetReadVersions: []int{1, 2}, targetPreviousKeySupport: true, requireUnusedPreviousKey: 1,
	})
	if allowed.Status != "ready" || allowed.PreviousKeyRemovalAllowed == nil || !*allowed.PreviousKeyRemovalAllowed {
		t.Fatalf("unused previous key was not removable: %+v", allowed)
	}
}
