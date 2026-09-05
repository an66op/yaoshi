// Command test-site-accounts explicitly provisions isolated credentials for an
// operator-confirmed test site. It never runs as part of server bootstrap.
package main

import (
	"backend/config"
	"backend/migrations"
	"backend/services"
	"backend/utils"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
)

const maxConfigBytes = 16384

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("test-site-accounts", flag.ContinueOnError)
	configFile := flags.String("config-file", "", "owner-only JSON file containing the four test credentials")
	confirmed := flags.Bool("confirm-test-site", false, "confirm this deployment is a test site, not a production launch")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*confirmed || *configFile == "" || flags.NArg() != 0 {
		return fmt.Errorf("必须明确提供 --confirm-test-site 和 --config-file；不接受命令行密码")
	}
	input, err := readConfigFile(*configFile)
	if err != nil {
		return fmt.Errorf("测试账号配置无效: %w", err)
	}
	config.LoadConfig()
	security := config.GetConfig().Security
	if err := utils.InitFieldEncryptionWithFallbacks(security.DataEncryptionKey, security.DataEncryptionPreviousKeys); err != nil {
		return fmt.Errorf("初始化敏感字段加密失败: %w", err)
	}
	// The deployed backend owns schema initialization. This utility checks its
	// migrations without making out-of-transaction schema changes of its own.
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
		return fmt.Errorf("请先启动匹配版本的后端完成数据库初始化: %w", err)
	}
	result, err := services.ProvisionTestSiteAccounts(db, input)
	if err != nil {
		return fmt.Errorf("初始化测试账号失败: %w", err)
	}
	return json.NewEncoder(output).Encode(result)
}

func readConfigFile(path string) (services.TestSiteAccountsConfig, error) {
	var input services.TestSiteAccountsConfig
	info, err := os.Lstat(path)
	if err != nil {
		return input, err
	}
	if err := validateConfigFileInfo(info); err != nil {
		return input, err
	}
	file, err := os.Open(path)
	if err != nil {
		return input, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return input, err
	}
	if !os.SameFile(info, opened) {
		return input, fmt.Errorf("配置文件在安全检查后被替换，拒绝读取")
	}
	if err := validateConfigFileInfo(opened); err != nil {
		return input, err
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return input, err
	}
	if len(contents) == 0 || len(contents) > maxConfigBytes {
		return input, fmt.Errorf("配置文件必须包含 1–16384 字节")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("必须提供字段正确的 JSON 配置")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return input, fmt.Errorf("配置文件只能包含一个 JSON 对象")
	}
	if err := services.ValidateTestSiteAccountsConfig(input); err != nil {
		return input, err
	}
	return input, nil
}

func validateConfigFileInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("配置必须是权限 0600 或更严格的普通文件，不能为软链接")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("配置文件必须属于执行本命令的操作系统用户")
	}
	return nil
}
