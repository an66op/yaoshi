package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestPlanStreamsPostgresRealDrawResultsAreReadOnly(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	now := time.Now().UTC().Truncate(time.Second)
	stream := plan.Stream{WorkspaceID: roomID, GameID: "speed-racing", Position: 1, PlanKey: DefaultPlanKey}
	if err := db.Create(&stream).Error; err != nil {
		t.Fatal(err)
	}
	option, _ := planOption(DefaultPlanKey)
	picks := planCyclePicks(roomID, 1, option, "941001")
	picks[0].Numbers, picks[1].Numbers = []int{1, 2, 3, 4, 6}, []int{1, 2, 3, 4, 5}
	payload, _ := json.Marshal(picks)
	cycle := plan.StreamCycle{StreamID: stream.ID, Periods: 4, PublishedPeriods: 3, Status: "active", StartIssue: "941001", PayloadJSON: string(payload)}
	if err := db.Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&stream).Update("cycle_id", cycle.ID).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		at := now.Add(time.Duration(index-5) * time.Minute)
		issue := lottery.Issue{GameID: stream.GameID, Issue: strconv.Itoa(941001 + index), Status: lottery.IssueStatusSettled, SourceMode: "external", AcceptAt: at.Add(-2 * time.Minute), SealAt: at.Add(-30 * time.Second), ScheduledDrawAt: &at, DrawAt: &at}
		if err := db.Create(&issue).Error; err != nil {
			t.Fatal(err)
		}
		published := at.Add(-time.Minute)
		if index == 1 {
			published = at
		} // Post-draw content must never be graded as a forecast.
		period := plan.StreamPeriod{StreamID: stream.ID, IssueID: issue.ID, Issue: issue.Issue, CycleID: cycle.ID, PeriodIndex: index + 1, ScheduledDrawAt: at, CreatedAt: published}
		if err := db.Create(&period).Error; err != nil {
			t.Fatal(err)
		}
		if index < 2 {
			if err := db.Create(&lottery.Draw{GameID: stream.GameID, Issue: issue.Issue, Numbers: "6,8,10,1,2,4,9,7,3,5", DrawAt: at}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	var before plan.Stream
	if err := db.First(&before, stream.ID).Error; err != nil {
		t.Fatal(err)
	}
	service := NewPlanContentService(db)
	for attempt := 0; attempt < 2; attempt++ {
		detail, err := service.StreamDetail(roomID, 1, DefaultPlanKey)
		if err != nil || len(detail.History) != 9 {
			t.Fatalf("history=%d err=%v", len(detail.History), err)
		}
		for _, pick := range detail.History {
			if pick.Issue != "941001" {
				if pick.Result != "pending" || len(pick.DrawNumbers) != 0 || pick.DrawAt != nil {
					t.Fatalf("unproven result: %+v", pick)
				}
				continue
			}
			if len(pick.DrawNumbers) != 10 || pick.DrawAt == nil {
				t.Fatal("missing real draw", pick)
			}
			if pick.MasterName == picks[0].MasterName && pick.Result != "hit" {
				t.Fatal("actual hit missing", pick)
			}
			if pick.MasterName == picks[1].MasterName && pick.Result != "miss" {
				t.Fatal("actual miss missing", pick)
			}
		}
	}
	var after plan.Stream
	var afterCycle plan.StreamCycle
	var count int64
	if err := db.First(&after, stream.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&afterCycle, cycle.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Where("stream_id = ?", stream.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || afterCycle.PayloadJSON != string(payload) || count != 3 {
		t.Fatal("reading results changed publications or activity")
	}
}
