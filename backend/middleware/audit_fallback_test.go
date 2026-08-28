package middleware

import (
	"backend/data/models/audit"
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrivilegedAuditFailsClosedWhenNoDurableStoreIsAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	blockingFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKEND_AUDIT_FALLBACK_FILE", filepath.Join(blockingFile, "audit.jsonl"))

	called := false
	router := gin.New()
	router.POST("/admin/users/:id", PrivilegedAudit(nil, "admin"), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/admin/users/42", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if called {
		t.Fatal("privileged handler ran without a durable audit store")
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}

func TestPrivilegedAuditSpoolsIntentAndCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("BACKEND_AUDIT_FALLBACK_FILE", path)

	router := gin.New()
	router.POST("/admin/users/:id", PrivilegedAudit(nil, "admin"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/admin/users/42", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected handler response, got %d", response.Code)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	entries := make([]audit.Log, 0, 2)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry audit.Log
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected intent and completion records, got %d", len(entries))
	}
	if entries[0].EventID == "" || entries[0].EventID != entries[1].EventID {
		t.Fatalf("audit phases must share one event id: %#v", entries)
	}
	if entries[0].StatusCode != http.StatusProcessing || entries[1].StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected audit statuses: %d then %d", entries[0].StatusCode, entries[1].StatusCode)
	}
}

func TestReplaceAuditFallbackDurablyPreservesModeAndReplacesContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := replaceAuditFallbackDurably(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "new\n" {
		t.Fatalf("unexpected replacement contents %q", payload)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("compaction changed spool permissions to %o", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remained after replacement: %v", err)
	}
}
