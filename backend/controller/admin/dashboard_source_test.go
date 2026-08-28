package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestOfficialSourceRejectsUnknownGroupBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/sources/not-a-provider/test", nil)
	context.Params = gin.Params{{Key: "group", Value: "not-a-provider"}}

	NewDashboardHandler(&gorm.DB{}).TestOfficialSource(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "未知官方数据源线路") {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}
