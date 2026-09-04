package lotteryfeed

import (
	"reflect"
	"testing"
)

func TestDefaultJobsUseOne163WriterForSevenMigratedGamesAndKeepSGVerified(t *testing.T) {
	jobs := DefaultJobs()
	groups := map[string]JobConfig{}
	for _, job := range jobs {
		groups[job.Group] = job
	}
	if _, found := groups["168-highfreq"]; found {
		t.Fatal("legacy 168 writer remains scheduled alongside 163")
	}
	want := []string{"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "speed-ssc", "au-lucky-5"}
	if got := groups["163-highfreq"].GameIDs; !reflect.DeepEqual(got, want) {
		t.Fatalf("163 games=%v want=%v", got, want)
	}
	if got := groups["sg-ssc-verified"].GameIDs; !reflect.DeepEqual(got, []string{"sg-ssc"}) {
		t.Fatalf("SG must retain its existing verified writer, got=%v", got)
	}
}
