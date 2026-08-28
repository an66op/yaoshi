package user

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBeforeCreateGeneratesFreshAuthVersion(t *testing.T) {
	first := User{AuthVersion: 1}
	second := User{}
	if err := first.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if err := second.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if first.AuthVersion < 2 || second.AuthVersion < 2 {
		t.Fatalf("generated versions must be usable: %d, %d", first.AuthVersion, second.AuthVersion)
	}
	if first.AuthVersion == second.AuthVersion {
		t.Fatalf("two fresh accounts unexpectedly share auth version %d", first.AuthVersion)
	}
}

func TestBeforeCreatePreservesExplicitCredentialGeneration(t *testing.T) {
	account := User{AuthVersion: 99}
	if err := account.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if account.AuthVersion != 99 {
		t.Fatalf("explicit auth version changed to %d", account.AuthVersion)
	}
}

func TestCreateLeavesZeroPublicIDToRandomDatabaseDefault(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}

	account := User{Username: "new-member", Password: "hashed-password", Role: "member"}
	statement := db.Create(&account).Statement
	if statement.Error != nil {
		t.Fatal(statement.Error)
	}
	insertSQL := strings.Split(statement.SQL.String(), "RETURNING")[0]
	if strings.Contains(insertSQL, `"public_id"`) {
		t.Fatalf("zero public ID was written explicitly instead of using the database allocator: %s", statement.SQL.String())
	}
	field := statement.Schema.LookUpField("PublicID")
	if field == nil || field.DefaultValue != "public.next_member_public_id()" {
		t.Fatalf("public ID default = %#v, want random database allocator", field)
	}
}

func TestCreatePreservesExplicitPublicID(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}

	const existingPublicID = uint64(7654321)
	account := User{
		PublicID: existingPublicID,
		Username: "imported-member",
		Password: "hashed-password",
		Role:     "member",
	}
	statement := db.Create(&account).Statement
	if statement.Error != nil {
		t.Fatal(statement.Error)
	}
	insertSQL := strings.Split(statement.SQL.String(), "RETURNING")[0]
	if !strings.Contains(insertSQL, `"public_id"`) {
		t.Fatalf("explicit public ID was omitted from INSERT: %s", statement.SQL.String())
	}
	if account.PublicID != existingPublicID {
		t.Fatalf("explicit public ID changed from %d to %d", existingPublicID, account.PublicID)
	}
}
