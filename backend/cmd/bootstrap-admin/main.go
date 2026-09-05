// Command bootstrap-admin creates the first production administrator without
// putting its password in shell history, process arguments or environment
// variables. The password file must be a regular owner-only secret (0600).
package main

import (
	"backend/config"
	"backend/services"
	"backend/utils"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	username := flag.String("username", "", "first administrator username")
	passwordFile := flag.String("password-file", "", "owner-only file containing the password")
	nickname := flag.String("nickname", "", "administrator display name")
	email := flag.String("email", "", "administrator email (optional)")
	flag.Parse()
	if strings.TrimSpace(*username) == "" || strings.TrimSpace(*passwordFile) == "" {
		log.Fatal("必须提供 --username 和 --password-file；不接受命令行密码")
	}
	password, err := readPasswordFile(*passwordFile)
	if err != nil {
		log.Fatalf("读取管理员密码失败: %v", err)
	}

	config.LoadConfig()
	cfg := config.GetConfig()
	if err := utils.InitFieldEncryptionWithFallbacks(cfg.Security.DataEncryptionKey, cfg.Security.DataEncryptionPreviousKeys); err != nil {
		log.Fatalf("初始化敏感字段加密失败: %v", err)
	}
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	account, err := services.CreateBootstrapAdmin(db, services.BootstrapAdminInput{
		Username: *username, Password: password, Nickname: *nickname, Email: *email,
	})
	if err != nil {
		log.Fatalf("创建初始管理员失败: %v", err)
	}
	fmt.Printf("初始管理员已创建: %s (ID %d)\n", account.Username, account.UserID)
}

func readPasswordFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("密码路径必须是普通文件，不能是软链接或设备")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("密码文件权限必须为 0600 或更严格，当前为 %04o", info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("密码文件在安全检查后已变更，拒绝读取")
	}
	contents, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return "", err
	}
	if len(contents) == 0 || len(contents) > 4096 {
		return "", fmt.Errorf("密码文件必须包含 1-4096 字节")
	}
	password := strings.TrimSuffix(string(contents), "\n")
	password = strings.TrimSuffix(password, "\r")
	if password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return "", fmt.Errorf("密码文件只能包含一行密码")
	}
	return password, nil
}
