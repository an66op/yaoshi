package main

import (
	"backend/config"
	"backend/migrations"
	"fmt"
	"log"
	"os"

	"backend/data/models/user"
)

// db-bootstrap is a one-time bridge for a pre-versioned development database.
// Fresh databases and every normal server startup use migrations.Run only.
func main() {
	config.LoadConfig()
	cfg := config.GetConfig()
	if cfg.Server.Mode == "release" {
		log.Fatal("legacy AutoMigrate bootstrap is disabled in release mode")
	}
	want := "legacy-bootstrap:" + cfg.Database.DBName
	if os.Getenv("BACKEND_DATABASE_LEGACY_BOOTSTRAP_CONFIRM") != want {
		log.Fatalf("refusing legacy bootstrap; set BACKEND_DATABASE_LEGACY_BOOTSTRAP_CONFIRM=%q", want)
	}

	db, err := config.OpenDatabase()
	if err != nil {
		log.Fatal(err)
	}
	if !db.Migrator().HasTable(&user.User{}) {
		log.Fatal("legacy bootstrap only accepts an existing pre-versioned database; start the application to migrate a fresh database")
	}
	if err := config.BootstrapLegacySchema(db); err != nil {
		log.Fatalf("legacy schema bootstrap failed: %v", err)
	}
	if err := migrations.Run(db); err != nil {
		log.Fatalf("versioned migration failed: %v", err)
	}
	fmt.Println("legacy database upgraded and registered in schema_migrations")
}
