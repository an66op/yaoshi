package admin

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSystemLogFilterAcceptsCursorAndOperationalFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/admin/system-logs?before_id=88&limit=25&category=source&type=sync_error&status=error&game_id=pc-canada&source_group=163-pc28&q=%E8%BF%87%E6%9C%9F&from=2026-09-04T00:00:00Z&to=2026-09-04T23:59:59Z", nil)
	filter, err := systemLogFilter(c)
	if err != nil {
		t.Fatal(err)
	}
	if filter.BeforeID != 88 || filter.Limit != 25 || filter.Category != "source" || filter.EventType != "sync_error" || filter.Status != "error" || filter.GameID != "pc-canada" || filter.SourceGroup != "163-pc28" || filter.Query != "过期" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if filter.From == nil || filter.To == nil || !filter.From.Equal(time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("time range was not parsed: %+v", filter)
	}
}

func TestSystemLogFilterRejectsUnknownOrUnboundedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{
		"?limit=101",
		"?type=anything",
		"?status=unknown",
		"?from=2026-09-05T00:00:00Z&to=2026-09-04T00:00:00Z",
		"?source_url=https://example.invalid",
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/admin/system-logs"+query, nil)
		if _, err := systemLogFilter(c); err == nil {
			t.Fatalf("unsafe query was accepted: %s", query)
		}
	}
}
