package api

import (
	"backend/services"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestOddsReadinessHandlerFailsClosedAndReturnsReadOnlyAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		audit      productionOddsAudit
		wantStatus int
		wantState  string
		wantReason string
	}{
		{
			name: "complete", wantStatus: http.StatusOK, wantState: "ready",
			audit: func(context.Context, *gorm.DB) (*services.ProductionOddsReadinessReport, error) {
				return &services.ProductionOddsReadinessReport{Complete: true, AuditedGames: 2, RequiredQuotes: 8, ValidQuotes: 8}, nil
			},
		},
		{
			name: "incomplete", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "odds_incomplete",
			audit: func(context.Context, *gorm.DB) (*services.ProductionOddsReadinessReport, error) {
				return &services.ProductionOddsReadinessReport{
					Complete: false, AuditedGames: 1, RequiredQuotes: 4, ValidQuotes: 3,
					IncompleteGames: []services.ProductionOddsGameReadiness{{GameID: "speed-racing", Reason: "quotes_incomplete"}},
				}, nil
			},
		},
		{
			name: "database error", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "odds_audit_unavailable",
			audit: func(context.Context, *gorm.DB) (*services.ProductionOddsReadinessReport, error) {
				return nil, errors.New("sensitive database error")
			},
		},
		{name: "missing auditor", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "odds_audit_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/ready/odds", oddsReadinessHandlerWithAudit(nil, test.audit))
			request := httptest.NewRequest(http.MethodGet, "/ready/odds", nil)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v", response.Code, response.Header())
			}
			var payload struct {
				Status string                                  `json:"status"`
				Reason string                                  `json:"reason"`
				Odds   *services.ProductionOddsReadinessReport `json:"odds"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != test.wantState || payload.Reason != test.wantReason {
				t.Fatalf("payload=%s", response.Body.String())
			}
			if test.wantReason == "odds_audit_unavailable" && payload.Odds != nil {
				t.Fatalf("database failure leaked audit details: %s", response.Body.String())
			}
			if (test.name == "complete" || test.name == "incomplete") && payload.Odds == nil {
				t.Fatalf("audit report missing: %s", response.Body.String())
			}
			if test.name == "database error" && strings.Contains(response.Body.String(), "sensitive database error") {
				t.Fatal("raw database error leaked")
			}
		})
	}
}

func TestSensitiveFieldReadinessHandlerFailsClosedAndReturnsStatisticsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		audit      sensitiveFieldAudit
		wantStatus int
		wantState  string
		wantReason string
	}{
		{
			name: "complete", wantStatus: http.StatusOK, wantState: "ready",
			audit: func(context.Context, *gorm.DB) (*services.SensitiveFieldReadinessReport, error) {
				return &services.SensitiveFieldReadinessReport{
					Complete: true, AuditedColumns: 3,
					Counts: services.SensitiveEnvelopeCounts{Total: 4, V1: 1, V2: 3},
				}, nil
			},
		},
		{
			name: "unknown key", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "encryption_incomplete",
			audit: func(context.Context, *gorm.DB) (*services.SensitiveFieldReadinessReport, error) {
				return &services.SensitiveFieldReadinessReport{
					Complete: false, AuditedColumns: 3,
					Counts: services.SensitiveEnvelopeCounts{Total: 4, V2: 3, Invalid: 1, KeyUnavailable: 1},
				}, nil
			},
		},
		{
			name: "audit error", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "encryption_inventory_unavailable",
			audit: func(context.Context, *gorm.DB) (*services.SensitiveFieldReadinessReport, error) {
				return nil, errors.New("database contained secret ciphertext")
			},
		},
		{name: "missing auditor", wantStatus: http.StatusServiceUnavailable, wantState: "not_ready", wantReason: "encryption_inventory_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/ready/encryption", sensitiveFieldReadinessHandlerWithAudit(nil, test.audit))
			request := httptest.NewRequest(http.MethodGet, "/ready/encryption", nil)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v", response.Code, response.Header())
			}
			var payload struct {
				Status     string                                  `json:"status"`
				Reason     string                                  `json:"reason"`
				Encryption *services.SensitiveFieldReadinessReport `json:"encryption"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != test.wantState || payload.Reason != test.wantReason {
				t.Fatalf("payload=%s", response.Body.String())
			}
			if test.wantReason == "encryption_inventory_unavailable" && payload.Encryption != nil {
				t.Fatalf("audit failure leaked details: %s", response.Body.String())
			}
			body := response.Body.String()
			for _, forbidden := range []string{"database contained secret ciphertext", "enc:v", "key_id", "plaintext_value", "ciphertext"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("sensitive audit data leaked (%q): %s", forbidden, body)
				}
			}
		})
	}
}
