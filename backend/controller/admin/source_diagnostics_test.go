package admin

import (
	"backend/data/models/user"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestSourceDiagnosticHandlersRejectNonAdminBeforeDBOrNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"", "member", "agent", "tenant"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(role+method, func(t *testing.T) {
				response := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(response)
				c.Request = httptest.NewRequest(method, "/api/admin/source-diagnostics", strings.NewReader(`{"source_key":"163:169"}`))
				if role != "" {
					c.Set("admin_user", user.User{Role: role, Status: 1})
				}
				handler := NewDashboardHandler(&gorm.DB{})
				if method == http.MethodGet {
					handler.SourceDiagnostics(c)
				} else {
					handler.ProbeSource(c)
				}
				if response.Code != http.StatusForbidden {
					t.Fatalf("role %s status=%d", role, response.Code)
				}
			})
		}
	}
}

func TestSourceDiagnosticProbeRejectsAllCustomTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct{ body, query string }{
		{`{"source_key":"http://127.0.0.1/internal"}`, ""}, {`{"source_key":"163:99999"}`, ""}, {`{"source_key":"163:169","url":"http://localhost"}`, ""},
		{`{"source_key":"163:169","sign":"old"}`, ""}, {`{"source_key":"163:169"}`, "?url=http://localhost"}, {`{"source_key":"163:169"}`, "?"},
		{`null`, ""}, {`[]`, ""}, {`{}`, ""}, {`{"source_key":169}`, ""}, {`{"source_key":" 163:169"}`, ""}, {strings.Repeat(" ", 1025), ""},
	} {
		t.Run(test.body+test.query, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("admin_user", user.User{Role: "admin", Status: 1})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/source-diagnostics/probe"+test.query, strings.NewReader(test.body))
			NewDashboardHandler(&gorm.DB{}).ProbeSource(c)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("custom target allowed status=%d", response.Code)
			}
		})
	}
}
