package lotteryfeed

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultJobsUseOnly163ForAllMarkSixProducts(t *testing.T) {
	var current, legacy []JobConfig
	for _, job := range DefaultJobs() {
		switch job.Group {
		case "163-marksix":
			current = append(current, job)
		case "168-marksix":
			legacy = append(legacy, job)
		}
	}
	if len(current) != 1 || !reflect.DeepEqual(current[0].GameIDs, []string{"hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six"}) || current[0].Timeout != 20*time.Second {
		t.Fatalf("163 Mark Six job=%+v", current)
	}
	if len(legacy) != 0 {
		t.Fatalf("legacy writer remained scheduled: %+v", legacy)
	}
}
