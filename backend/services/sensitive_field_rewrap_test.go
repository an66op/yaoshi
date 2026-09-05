package services

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestSensitiveRewrapRemovalReadyRequiresExecutedCompleteZeroInventory(t *testing.T) {
	complete := &SensitiveFieldReadinessReport{Complete: true}
	for _, test := range []struct {
		name      string
		executed  bool
		inventory *SensitiveFieldReadinessReport
		remaining uint64
		want      bool
	}{
		{name: "executed complete zero", executed: true, inventory: complete, want: true},
		{name: "dry run never authorizes removal", inventory: complete},
		{name: "missing final inventory", executed: true},
		{name: "incomplete final inventory", executed: true, inventory: &SensitiveFieldReadinessReport{}},
		{name: "remaining dependency", executed: true, inventory: complete, remaining: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sensitiveRewrapRemovalReady(test.executed, test.inventory, test.remaining); got != test.want {
				t.Fatalf("ready=%v want=%v", got, test.want)
			}
		})
	}
}

func TestSensitiveRewrapExecuteFailsBeforeDatabaseAccessWithoutLiveMaintenanceProof(t *testing.T) {
	db := &gorm.DB{}
	if _, err := RewrapSensitiveFieldsFromPreviousKey(nil, db, SensitiveFieldRewrapOptions{
		PreviousKeyIndex: 1, Execute: true,
	}); err == nil {
		t.Fatal("execute accepted a missing maintenance proof")
	}
	maintenanceErr := errors.New("writer freeze lost")
	if _, err := RewrapSensitiveFieldsFromPreviousKey(nil, db, SensitiveFieldRewrapOptions{
		PreviousKeyIndex: 1, Execute: true, MaintenanceCheck: func() error { return maintenanceErr },
	}); !errors.Is(err, maintenanceErr) {
		t.Fatalf("execute did not fail closed on maintenance proof: %v", err)
	}
}
