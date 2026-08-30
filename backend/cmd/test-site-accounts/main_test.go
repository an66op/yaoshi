package main

import (
	"backend/services"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commandFixture() services.TestSiteAccountsConfig {
	return services.TestSiteAccountsConfig{
		Site:     "fixture.example",
		Platform: services.TestSiteCredential{Username: "testadmin", Password: "V7!mQ2#zL9@pR4$x"},
		Tenant:   services.TestSiteRoomAccount{TestSiteCredential: services.TestSiteCredential{Username: "testtenant", Password: "T8!mQ2#zL9@pR4$x"}, RoomCode: "88002", RoomName: "测试租户房"},
		Agent:    services.TestSiteRoomAccount{TestSiteCredential: services.TestSiteCredential{Username: "testagent", Password: "A9!mQ2#zL9@pR4$x"}, RoomCode: "88001", RoomName: "测试代理房"},
		Member:   services.TestSiteMemberAccount{TestSiteCredential: services.TestSiteCredential{Username: "testmember", Password: "M6!mQ2#zL9@pR4$x"}, RoomCode: "88001"},
	}
}

func TestRunRequiresExplicitTestConfirmationBeforeReadingConfiguration(t *testing.T) {
	for _, args := range [][]string{nil, {"--config-file", "missing"}, {"--confirm-test-site"}, {"--confirm-test-site", "--config-file", "missing", "password"}} {
		var output bytes.Buffer
		if err := run(args, &output); err == nil || !strings.Contains(err.Error(), "必须明确提供") {
			t.Fatalf("explicit confirmation was not required: %v", err)
		}
		if output.Len() != 0 {
			t.Fatal("refused command produced output")
		}
	}
}

func TestReadConfigFileRequiresOwnerOnlyRegularValidatedJSON(t *testing.T) {
	directory := t.TempDir()
	contents, err := json.Marshal(commandFixture())
	if err != nil {
		t.Fatal(err)
	}
	secure := filepath.Join(directory, "accounts.json")
	if err := os.WriteFile(secure, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readConfigFile(secure); err != nil || got.Member.RoomCode != "88001" {
		t.Fatalf("valid file rejected: %v", err)
	}
	for _, mode := range []os.FileMode{0o640, 0o604, 0o644} {
		if err := os.Chmod(secure, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := readConfigFile(secure); err == nil {
			t.Fatalf("unsafe mode %o accepted", mode)
		}
	}
	if err := os.Chmod(secure, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.json")
	if err := os.Symlink(secure, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfigFile(link); err == nil {
		t.Fatal("symlink accepted")
	}
	for _, bad := range []string{"", "null", "{}", string(contents) + "{}", strings.Repeat("x", maxConfigBytes+1), strings.Replace(string(contents), `"site":`, `"unknown":true,"site":`, 1)} {
		if err := os.WriteFile(secure, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readConfigFile(secure); err == nil {
			t.Fatal("invalid JSON configuration accepted")
		}
	}
}
