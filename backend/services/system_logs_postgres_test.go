package services

import (
	"backend/lotteryfeed"
	"context"
	"testing"
	"time"
)

func TestSystemLogsPostgresRecordsTransitionsAndFiltersWithCursor(t *testing.T) {
	db := timingPostgresDatabase(t)
	service := NewSystemLogService(db)
	now := time.Now().UTC().Truncate(time.Second)
	service.RecordSchedulerEvent(context.Background(), lotteryfeed.Event{Category: "source", Type: "sync_error", Level: "error", Status: "error", SourceGroup: "163-pc28", GameID: "pc-canada", JobID: "163-pc28", Message: "上游开奖过期", ConsecutiveErrors: 1, OccurredAt: now})
	service.RecordSchedulerEvent(context.Background(), lotteryfeed.Event{Category: "source", Type: "sync_recovered", Level: "info", Status: "ok", SourceGroup: "163-pc28", GameID: "pc-canada", JobID: "163-pc28", Message: "开奖源同步已恢复", Imported: 3, LatestIssue: "3477941", OccurredAt: now.Add(time.Second)})
	service.RecordSchedulerEvent(context.Background(), lotteryfeed.Event{Category: "scheduler", Type: "scheduler_recovered", Level: "info", Status: "ok", SourceGroup: "163-pc28", JobID: "163-pc28", Message: "开奖同步任务已恢复", OccurredAt: now.Add(2 * time.Second)})

	page, err := service.Logs(SystemLogFilter{Category: "source", GameID: "pc-canada", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].EventType != "sync_recovered" || !page.HasMore || page.NextBefore == 0 {
		t.Fatalf("unexpected first page: %+v", page)
	}
	next, err := service.Logs(SystemLogFilter{Category: "source", GameID: "pc-canada", BeforeID: page.NextBefore, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].EventType != "sync_error" || next.HasMore {
		t.Fatalf("unexpected cursor page: %+v", next)
	}
	reason, err := service.Logs(SystemLogFilter{Query: "过期", Limit: 10})
	if err != nil || len(reason.Items) != 1 || reason.Items[0].Status != "error" {
		t.Fatalf("reason filter failed: page=%+v err=%v", reason, err)
	}
	cutoff := now.Add(2 * time.Second)
	bounded, err := service.Logs(SystemLogFilter{SourceGroup: "163-pc28", From: &now, To: &cutoff, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bounded.Items) != 2 {
		t.Fatalf("exclusive end boundary leaked the exact-cutoff event: %+v", bounded.Items)
	}
}
