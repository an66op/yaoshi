// Command dev-acceptance-funding explicitly credits one existing acceptance
// member after a completed local business-data reset. It is never invoked by
// server bootstrap and never applies migrations.
package main

import (
	"backend/config"
	"backend/migrations"
	"backend/services"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

type commandOptions struct {
	Input     services.DevAcceptanceFundingInput
	Confirmed bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func parseCommandOptions(args []string) (commandOptions, error) {
	var options commandOptions
	flags := flag.NewFlagSet("dev-acceptance-funding", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.Input.ResetRequestID, "reset-request-id", "", "exact request_id from the latest business reset receipt")
	flags.StringVar(&options.Input.LoginScope, "login-scope", "", "exact login_scope of the existing member")
	flags.StringVar(&options.Input.Username, "username", "", "existing active member username")
	flags.Int64Var(&options.Input.AmountCents, "amount-cents", 0, "positive acceptance balance in integer cents")
	flags.BoolVar(&options.Confirmed, "confirm-dev-acceptance-funding", false, "confirm this one-account post-reset credit")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	provided := map[string]bool{}
	flags.Visit(func(item *flag.Flag) { provided[item.Name] = true })
	for _, required := range []string{"reset-request-id", "login-scope", "username", "amount-cents", "confirm-dev-acceptance-funding"} {
		if !provided[required] {
			return options, fmt.Errorf("必须显式提供 --reset-request-id、--login-scope、--username、--amount-cents 和 --confirm-dev-acceptance-funding")
		}
	}
	if !options.Confirmed || flags.NArg() != 0 {
		return options, fmt.Errorf("必须明确启用 --confirm-dev-acceptance-funding，且不能提供位置参数")
	}
	return options, nil
}

func run(args []string, output io.Writer) error {
	options, err := parseCommandOptions(args)
	if err != nil {
		return err
	}
	if err := services.ValidateDevAcceptanceFundingInput(options.Input); err != nil {
		return err
	}

	config.LoadConfig()
	cfg := config.GetConfig()
	safety := services.DevAcceptanceFundingSafety{
		Mode:         cfg.Server.Mode,
		DatabaseHost: cfg.Database.Host,
		DatabaseName: cfg.Database.DBName,
	}
	if err := services.ValidateDevAcceptanceFundingSafety(safety); err != nil {
		return err
	}

	db, err := config.OpenDatabase()
	if err != nil {
		return fmt.Errorf("连接本机开发数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := migrations.VerifyApplied(db); err != nil {
		return fmt.Errorf("数据库迁移未就绪；本工具不会自动迁移: %w", err)
	}

	result, err := services.FundDevAcceptanceAccount(db, safety, options.Input)
	if err != nil {
		return fmt.Errorf("验收账号注资失败: %w", err)
	}
	return json.NewEncoder(output).Encode(result)
}
