package api

import (
	"backend/config"
	"backend/data/models/audit"
	"backend/data/models/user"
	"backend/sessionauth"
	"backend/utils"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const sgSSCBackfillTestPath = "/api/admin/sources/sg-ssc/backfill"

func TestSGSSCBackfillRoutesRequireAuthenticationAndTrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test", AllowedOrigins: []string{"https://operator.example"}}}
	t.Cleanup(func() { config.Config = previous })
	utils.InitJWT("sg-backfill-origin-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(Cors())
	// This handle cannot execute any SQL: every request must stop at Origin or
	// JWT validation before the account, handler, audit, or queue is consulted.
	LoadRoutes(engine, &gorm.DB{}, nil)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, test := range []struct {
			name, origin, cookie string
			bearer               bool
			want                 int
		}{
			{name: "anonymous trusted origin", origin: "https://operator.example", want: http.StatusUnauthorized},
			{name: "anonymous non-browser", want: http.StatusUnauthorized},
			{name: "foreign origin with valid cookie", origin: "https://foreign.example", cookie: token, want: http.StatusForbidden},
			{name: "opaque origin with valid cookie", origin: "null", cookie: token, want: http.StatusForbidden},
			{name: "invalid cookie cannot use bearer to bypass", origin: "https://operator.example", cookie: "invalid-session", bearer: true, want: http.StatusUnauthorized},
		} {
			t.Run(method+"/"+test.name, func(t *testing.T) {
				request := httptest.NewRequest(method, sgSSCBackfillTestPath, strings.NewReader(`{}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Origin", test.origin)
				if test.cookie != "" {
					request.AddCookie(&http.Cookie{Name: sessionauth.ManagementCookieName, Value: test.cookie})
				}
				if test.bearer {
					request.Header.Set("Authorization", "Bearer "+token)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if response.Code != test.want {
					t.Fatalf("%s status=%d want=%d body=%s", method, response.Code, test.want, response.Body.String())
				}
			})
		}
	}
}

type sgSSCBackfillSecurityEvidence struct {
	accountReads, businessReads, businessWrites int
	auditIntents                                []audit.Log
	auditCompletions                            []map[string]any
}

// Instrument only the account and generic HTTP audit boundaries. Disable ping
// and immediately close the unused pool: DryRun alone does NOT stop an explicit
// Transaction from opening a real connection. Unexpected business queries or
// writes are errors and are counted even if a handler ignores them.
func sgSSCBackfillSecurityDatabase(t *testing.T, account user.User, allowAudit bool) (*gorm.DB, *sgSSCBackfillSecurityEvidence) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	evidence := &sgSSCBackfillSecurityEvidence{}
	rejectRead := func(tx *gorm.DB) {
		evidence.businessReads++
		tx.AddError(fmt.Errorf("rejected request reached a business read"))
	}
	rejectWrite := func(tx *gorm.DB) {
		evidence.businessWrites++
		tx.AddError(fmt.Errorf("rejected request reached a business write"))
	}
	registrations := []error{
		db.Callback().Query().Before("gorm:query").Register("test:sg_backfill_account", func(tx *gorm.DB) {
			if target, ok := tx.Statement.Dest.(*user.User); ok {
				evidence.accountReads++
				*target = account
				return
			}
			rejectRead(tx)
		}),
		db.Callback().Raw().Before("gorm:raw").Register("test:sg_backfill_raw", rejectRead),
		db.Callback().Row().Before("gorm:row").Register("test:sg_backfill_row", rejectRead),
		db.Callback().Create().Before("gorm:create").Register("test:sg_backfill_create", func(tx *gorm.DB) {
			if entry, ok := tx.Statement.Dest.(*audit.Log); ok {
				evidence.auditIntents = append(evidence.auditIntents, *entry)
				if !allowAudit {
					tx.AddError(fmt.Errorf("fixture HTTP audit unavailable"))
				}
				return
			}
			rejectWrite(tx)
		}),
		db.Callback().Update().Before("gorm:update").Register("test:sg_backfill_update", func(tx *gorm.DB) {
			if tx.Statement.Table == (audit.Log{}).TableName() && allowAudit {
				if updates, ok := tx.Statement.Dest.(map[string]any); ok {
					copy := make(map[string]any, len(updates))
					for key, value := range updates {
						copy[key] = value
					}
					evidence.auditCompletions = append(evidence.auditCompletions, copy)
					return
				}
			}
			rejectWrite(tx)
		}),
		db.Callback().Delete().Before("gorm:delete").Register("test:sg_backfill_delete", rejectWrite),
	}
	for _, err := range registrations {
		if err != nil {
			t.Fatal(err)
		}
	}
	return db, evidence
}

func TestSGSSCBackfillSecurityFixtureRejectsExplicitTransactions(t *testing.T) {
	db, _ := sgSSCBackfillSecurityDatabase(t, user.User{}, false)
	if err := db.Begin().Error; err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("offline fixture must reject explicit transactions using its closed pool: %v", err)
	}
}

func TestSGSSCBackfillRoutesRejectNonAdminDisabledAndRevokedSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("sg-backfill-role-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, role string
		status     int
		version    uint64
		want       int
	}{
		{"tenant", "tenant", 1, 7, http.StatusForbidden},
		{"agent", "agent", 1, 7, http.StatusForbidden},
		{"member", "member", 1, 7, http.StatusForbidden},
		{"disabled admin", "admin", 0, 7, http.StatusUnauthorized},
		{"revoked admin", "admin", 1, 8, http.StatusUnauthorized},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(test.name+"/"+method, func(t *testing.T) {
				account := user.User{Role: test.role, Status: test.status, AuthVersion: test.version, WorkspaceID: 17}
				account.UserID = 42
				db, evidence := sgSSCBackfillSecurityDatabase(t, account, false)
				engine := gin.New()
				LoadRoutes(engine, db, nil)
				request := httptest.NewRequest(method, sgSSCBackfillTestPath+"?workspace_id=0&role=admin", strings.NewReader(`{"operator":"admin","user_id":1}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if response.Code != test.want || evidence.accountReads != 1 || evidence.businessReads != 0 || evidence.businessWrites != 0 || len(evidence.auditIntents) != 0 {
					t.Fatalf("role/session boundary bypassed: status=%d want=%d evidence=%+v body=%s", response.Code, test.want, evidence, response.Body.String())
				}
			})
		}
	}
}

func TestSGSSCBackfillPostRejectsBusinessPayloadBeforeQueueAndKeepsHTTPAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("sg-backfill-payload-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, payload, query string }{
		{name: "malformed JSON", payload: `{`},
		{name: "custom issues", payload: `{"issues":["20260901001"]}`},
		{name: "custom source", payload: `{"force":true,"source_url":"https://foreign.example"}`},
		{name: "forged actor and game", payload: `{"operator":"other-admin","request_id":"override","game_id":"speed-ssc"}`},
		{name: "null", payload: `null`},
		{name: "array", payload: `[]`},
		{name: "oversized whitespace", payload: strings.Repeat(" ", 1025)},
		{name: "query cannot replace body", payload: `{}`, query: "?issues=20260901001"},
		{name: "invalid query cannot disappear", payload: `{}`, query: "?issues=%ZZ"},
		{name: "bare query marker", payload: `{}`, query: "?"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := user.User{Role: "admin", Status: 1, AuthVersion: 7, Username: "platform-fixture", WorkspaceID: 1}
			account.UserID = 42
			db, evidence := sgSSCBackfillSecurityDatabase(t, account, true)
			engine := gin.New()
			LoadRoutes(engine, db, nil)
			request := httptest.NewRequest(http.MethodPost, sgSSCBackfillTestPath+test.query, strings.NewReader(test.payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Idempotency-Key", "sg-backfill-http-security")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || evidence.businessReads != 0 || evidence.businessWrites != 0 || len(evidence.auditIntents) != 1 || len(evidence.auditCompletions) != 1 {
				t.Fatalf("invalid payload reached queue or lost HTTP audit: status=%d evidence=%+v body=%s", response.Code, evidence, response.Body.String())
			}
			intent, completion := evidence.auditIntents[0], evidence.auditCompletions[0]
			if intent.ActorID != 42 || intent.ActorName != account.Username || intent.ActorRole != "admin" || intent.EventID == "" ||
				intent.RequestID != "sg-backfill-http-security" || intent.Path != sgSSCBackfillTestPath || intent.StatusCode != http.StatusProcessing ||
				completion["status_code"] != http.StatusBadRequest || completion["request_id"] != intent.RequestID || completion["actor_id"] != intent.ActorID {
				t.Fatalf("HTTP audit failed to retain authenticated identity/result: intent=%+v completion=%+v", intent, completion)
			}
		})
	}
}

func TestSGSSCBackfillGetRejectsInvalidPaginationWithoutAuditOrBusinessAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("sg-backfill-pagination-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"?limit=0", "?limit=51", "?limit=invalid", "?before_id=-1", "?before_id=18446744073709551616"} {
		t.Run(query, func(t *testing.T) {
			account := user.User{Role: "admin", Status: 1, AuthVersion: 7, Username: "platform-fixture", WorkspaceID: 1}
			account.UserID = 42
			db, evidence := sgSSCBackfillSecurityDatabase(t, account, false)
			engine := gin.New()
			LoadRoutes(engine, db, nil)
			request := httptest.NewRequest(http.MethodGet, sgSSCBackfillTestPath+query, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || evidence.accountReads != 1 || evidence.businessReads != 0 || evidence.businessWrites != 0 ||
				len(evidence.auditIntents) != 0 || len(evidence.auditCompletions) != 0 {
				t.Fatalf("invalid GET pagination reached business work: status=%d evidence=%+v body=%s", response.Code, evidence, response.Body.String())
			}
		})
	}
}

func TestSGSSCBackfillPostFailsClosedWithoutDurableHTTPAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("sg-backfill-audit-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	blockingFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKEND_AUDIT_FALLBACK_FILE", filepath.Join(blockingFile, "audit.jsonl"))
	account := user.User{Role: "admin", Status: 1, AuthVersion: 7, Username: "platform-fixture", WorkspaceID: 1}
	account.UserID = 42
	db, evidence := sgSSCBackfillSecurityDatabase(t, account, false)
	engine := gin.New()
	LoadRoutes(engine, db, nil)
	request := httptest.NewRequest(http.MethodPost, sgSSCBackfillTestPath, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "AUDIT_UNAVAILABLE") ||
		evidence.businessReads != 0 || evidence.businessWrites != 0 || len(evidence.auditIntents) != 1 || len(evidence.auditCompletions) != 0 {
		t.Fatalf("unaudited enqueue was not blocked: status=%d evidence=%+v body=%s", response.Code, evidence, response.Body.String())
	}
}
