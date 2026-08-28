package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDataLifecycleHandlersRejectMissingPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &SystemAuditHandler{}
	tests := []struct {
		name string
		call func(*gin.Context)
	}{
		{name: "policies", call: handler.RetentionPolicies},
		{name: "update policy", call: handler.UpdateRetentionPolicy},
		{name: "preview", call: handler.PreviewCleanup},
		{name: "execute", call: handler.ExecuteCleanup},
		{name: "runs", call: handler.CleanupRuns},
		{name: "run", call: handler.CleanupRun},
		{name: "archives", call: handler.CleanupArchives},
		{name: "restore soft deleted", call: handler.RestoreSoftDeleted},
		{name: "restore robot archive", call: handler.RestoreRobotArchive},
		{name: "refund abnormal bet", call: handler.RefundAbnormalBet},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest("GET", "/api/admin/data-lifecycle", nil)
			test.call(context)
			if response.Code != 403 {
				t.Fatalf("status = %d, want 403", response.Code)
			}
		})
	}
}
