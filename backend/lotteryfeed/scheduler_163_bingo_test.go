package lotteryfeed

import (
	"reflect"
	"testing"
)

func TestDefaultJobsUseSingle163BingoWriter(t *testing.T) {
	want := []string{"bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "bingo-racing-a", "bingo-racing-b", "bingo-mark-six"}
	wantSet := make(map[string]bool, len(want))
	for _, gameID := range want {
		wantSet[gameID] = true
	}

	writers := make(map[string][]string, len(want))
	var bingoJobs []JobConfig
	for _, job := range DefaultJobs() {
		if job.ID == "168-bingo" || job.Group == "168-bingo" {
			t.Fatalf("legacy 168 Bingo writer remains scheduled: %+v", job)
		}
		containsBingo := false
		for _, gameID := range job.GameIDs {
			if wantSet[gameID] {
				containsBingo = true
				writers[gameID] = append(writers[gameID], job.ID)
			}
		}
		if containsBingo {
			bingoJobs = append(bingoJobs, job)
		}
	}

	if len(bingoJobs) != 1 {
		t.Fatalf("Bingo-derived games must have one scheduler writer, got=%+v", bingoJobs)
	}
	job := bingoJobs[0]
	if job.ID != "163-bingo" || job.Group != "163-bingo" || !reflect.DeepEqual(job.GameIDs, want) {
		t.Fatalf("unexpected 163 Bingo job: %+v", job)
	}
	for _, gameID := range want {
		if got := writers[gameID]; !reflect.DeepEqual(got, []string{"163-bingo"}) {
			t.Fatalf("writers(%s)=%v want only 163-bingo", gameID, got)
		}
	}
}
