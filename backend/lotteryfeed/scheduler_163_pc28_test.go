package lotteryfeed

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultJobsUseSingle163Canada28WriterForAllRuleVariants(t *testing.T) {
	want := []string{"pc-canada", "canada-28", "canada-20"}
	writers := make(map[string][]string, len(want))
	var selected *JobConfig
	for _, item := range DefaultJobs() {
		job := item
		for _, gameID := range job.GameIDs {
			for _, target := range want {
				if gameID == target {
					writers[target] = append(writers[target], job.ID)
				}
			}
		}
		if job.ID == "163-pc28" {
			selected = &job
		}
	}
	if selected == nil || selected.Group != "163-pc28" || !reflect.DeepEqual(selected.GameIDs, want) ||
		selected.FastInterval != 15*time.Second || selected.NormalInterval != 15*time.Second || selected.Timeout != 20*time.Second {
		t.Fatalf("unexpected Canada28 job: %+v", selected)
	}
	for _, gameID := range want {
		if got := writers[gameID]; !reflect.DeepEqual(got, []string{"163-pc28"}) {
			t.Fatalf("writers(%s)=%v want only 163-pc28", gameID, got)
		}
	}
}
