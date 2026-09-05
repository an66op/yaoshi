// Command plan-history-backfill explicitly adds recent real issue identities as
// ungraded plan display history. It does not run migrations or synthesize draws.
package main

import (
	"backend/config"
	"backend/migrations"
	"backend/services"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("plan-history-backfill", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	roomCode := flags.String("room-code", "", "exact room code")
	periods := flags.Int("periods", services.MaxPlanHistoryBackfillPeriods, "real historical periods per configured game")
	confirmed := flags.Bool("confirm-history-display-backfill", false, "confirm ungraded display-history insertion")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*confirmed || strings.TrimSpace(*roomCode) == "" || flags.NArg() != 0 {
		return fmt.Errorf("必须提供 --room-code、--periods 和 --confirm-history-display-backfill")
	}
	config.LoadConfig()
	db, err := config.OpenDatabase()
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := migrations.VerifyApplied(db); err != nil {
		return fmt.Errorf("数据库迁移未就绪；本工具不会自动迁移: %w", err)
	}
	var workspaceID uint64
	if err := db.Raw("SELECT id FROM workspaces WHERE room_code = ? AND status = 1", strings.TrimSpace(*roomCode)).Scan(&workspaceID).Error; err != nil {
		return err
	}
	if workspaceID == 0 {
		return fmt.Errorf("房间 %s 不存在或未启用", strings.TrimSpace(*roomCode))
	}
	report, err := services.NewPlanAutomationService(db).BackfillHistory(context.Background(), workspaceID, *periods)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}
