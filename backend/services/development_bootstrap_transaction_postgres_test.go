package services

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/utils"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestDevelopmentAcceptanceOuterTransactionPostgresRollsBackEveryStep(t *testing.T) {
	db := timingPostgresDatabase(t)
	var bootstrapAdmin user.User
	if err := db.Where("username = ?", "timing_platform").First(&bootstrapAdmin).Error; err != nil {
		t.Fatal(err)
	}
	adminPassword, err := utils.HashPassword(constants.DefaultAdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&bootstrapAdmin).Updates(map[string]any{
		"username": constants.DefaultAdminUsername,
		"password": adminPassword,
		"nickname": constants.DefaultAdminNickname,
	}).Error; err != nil {
		t.Fatal("normalize isolated bootstrap administrator:", err)
	}

	initialMarker := DevelopmentDatabaseMarkerNamespace + ":initializing:7679044470357611185:" + strings.Repeat("a", 32)
	if err := setDevelopmentDatabaseMarker(db, initialMarker); err != nil {
		t.Fatal("set isolated initializing marker:", err)
	}
	baseline := developmentBootstrapTestSnapshot(t, db)
	completeMarker := DevelopmentDatabaseMarkerNamespace + ":complete:test"
	options := BootstrapOptions{Mode: "debug", SeedExperienceAccounts: true}

	for _, failAt := range []string{"bootstrap", "profile", "comment"} {
		t.Run(failAt, func(t *testing.T) {
			forced := errors.New("forced " + failAt + " failure")
			bootstrapCalled, profileCalled, finalizeCalled := false, false, false
			steps := developmentBootstrapTransactionSteps{
				bootstrap: func(tx *gorm.DB, input BootstrapOptions) error {
					bootstrapCalled = true
					if err := Bootstrap(tx, input); err != nil {
						return err
					}
					if failAt == "bootstrap" {
						return forced
					}
					return nil
				},
				applyProfile: func(tx *gorm.DB, mode string) (*DevelopmentBootstrapReport, error) {
					profileCalled = true
					report, err := ApplyDevelopmentAcceptanceProfile(tx, mode)
					if err != nil {
						return nil, err
					}
					if failAt == "profile" {
						return nil, forced
					}
					return report, nil
				},
				finalize: func(tx *gorm.DB) error {
					finalizeCalled = true
					if err := setDevelopmentDatabaseMarker(tx, completeMarker); err != nil {
						return err
					}
					if failAt == "comment" {
						return forced
					}
					return nil
				},
			}
			report, runErr := initializeDevelopmentAcceptanceWithSteps(db, options, steps)
			if !errors.Is(runErr, forced) {
				t.Fatalf("failure = %v, want injected %v", runErr, forced)
			}
			if report != nil {
				t.Fatalf("failed initialization returned a report: %+v", report)
			}
			wantProfile := failAt != "bootstrap"
			wantFinalize := failAt == "comment"
			if !bootstrapCalled || profileCalled != wantProfile || finalizeCalled != wantFinalize {
				t.Fatalf("step calls = bootstrap:%t profile:%t finalize:%t", bootstrapCalled, profileCalled, finalizeCalled)
			}
			after := developmentBootstrapTestSnapshot(t, db)
			if !reflect.DeepEqual(after, baseline) {
				t.Fatalf("%s failure left partially committed business data or database COMMENT", failAt)
			}
		})
	}
}

func developmentBootstrapTestSnapshot(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()
	var tables []string
	if err := db.Raw(`SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' ORDER BY tablename`).Scan(&tables).Error; err != nil {
		t.Fatal("list isolated business tables:", err)
	}
	snapshot := make(map[string][]string, len(tables)+1)
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		var rows []string
		query := fmt.Sprintf(`SELECT to_jsonb(item)::text FROM %s AS item ORDER BY to_jsonb(item)::text`, quoted)
		if err := db.Raw(query).Scan(&rows).Error; err != nil {
			t.Fatalf("snapshot isolated table %s: %v", table, err)
		}
		snapshot[table] = rows
	}
	var marker string
	if err := db.Raw(`
		SELECT COALESCE(shobj_description(oid, 'pg_database'), '')
		FROM pg_database WHERE datname = current_database()
	`).Scan(&marker).Error; err != nil {
		t.Fatal("read isolated database COMMENT:", err)
	}
	snapshot["__database_comment__"] = []string{marker}
	return snapshot
}
