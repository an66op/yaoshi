package services

import (
	"testing"
	"time"
)

func TestNextLifecycleRunUsesBeijingCalendar(t *testing.T) {
	location := beijingLifecycleLocation()
	before := time.Date(2026, time.August, 27, 3, 29, 59, 0, location)
	if got, want := nextLifecycleRun(before), time.Date(2026, time.August, 27, 3, 30, 0, 0, location); !got.Equal(want) {
		t.Fatalf("next run before window = %v, want %v", got, want)
	}

	after := time.Date(2026, time.August, 27, 3, 30, 0, 0, location)
	if got, want := nextLifecycleRun(after), time.Date(2026, time.August, 28, 3, 30, 0, 0, location); !got.Equal(want) {
		t.Fatalf("next run after window = %v, want %v", got, want)
	}
}
