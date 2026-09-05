// Command field-encryption-audit is the offline production/rollback gate for
// persisted encrypted fields. Its output contains aggregate statistics only.
package main

import (
	"backend/config"
	"backend/services"
	"backend/utils"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type auditOptions struct {
	targetReadVersions       []int
	targetPreviousKeySupport bool
	requireUnusedPreviousKey int
	rewrapPreviousKey        int
	executeRewrap            bool
	rewrapBatchSize          int
}

type auditOutput struct {
	Status                    string                                  `json:"status"`
	Inventory                 *services.SensitiveFieldReadinessReport `json:"inventory,omitempty"`
	Compatibility             *services.SensitiveFieldCompatibility   `json:"compatibility,omitempty"`
	PreviousKeyRemovalAllowed *bool                                   `json:"previous_key_removal_allowed,omitempty"`
	Rewrap                    *services.SensitiveFieldRewrapReport    `json:"rewrap,omitempty"`
}

func main() {
	options, err := parseAuditOptions(os.Args[1:], os.Stderr)
	if err != nil {
		log.Printf("加密信封审计参数无效: %v", err)
		os.Exit(2)
	}
	config.LoadConfig()
	cfg := config.GetConfig()
	if err := utils.InitFieldEncryptionWithFallbacks(cfg.Security.DataEncryptionKey, cfg.Security.DataEncryptionPreviousKeys); err != nil {
		log.Print("加密信封审计无法初始化当前密钥配置")
		os.Exit(1)
	}
	if options.requireUnusedPreviousKey > len(cfg.Security.DataEncryptionPreviousKeys) {
		log.Print("待移除的历史密钥位置不在当前配置中")
		os.Exit(2)
	}
	if options.rewrapPreviousKey > len(cfg.Security.DataEncryptionPreviousKeys) {
		log.Print("待重加密的历史密钥位置不在当前配置中")
		os.Exit(2)
	}
	db, err := config.OpenDatabase()
	if err != nil {
		log.Print("加密信封审计无法连接数据库")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if options.rewrapPreviousKey > 0 {
		rewrap, err := services.RewrapSensitiveFieldsFromPreviousKey(ctx, db, services.SensitiveFieldRewrapOptions{
			PreviousKeyIndex: options.rewrapPreviousKey,
			BatchSize:        options.rewrapBatchSize,
			Execute:          options.executeRewrap,
			MaintenanceCheck: productionMaintenanceCheck(options.executeRewrap),
		})
		if err != nil {
			log.Print("敏感字段重加密失败；未输出任何存储值或密钥信息")
			os.Exit(1)
		}
		status := "dry_run"
		if options.executeRewrap {
			status = "not_ready"
			if rewrap.ReadyForKeyRemoval {
				status = "ready"
			}
		} else if rewrap.Inventory == nil || !rewrap.Inventory.Complete {
			status = "not_ready"
		}
		writeAuditOutput(auditOutput{Status: status, Rewrap: rewrap})
		if status == "not_ready" {
			os.Exit(1)
		}
		return
	}
	report, err := services.AuditSensitiveFieldReadiness(ctx, db)
	if err != nil {
		log.Print("加密信封清单读取失败")
		os.Exit(1)
	}
	output := evaluateSensitiveAudit(report, options)
	writeAuditOutput(output)
	if output.Status != "ready" {
		os.Exit(1)
	}
}

func writeAuditOutput(output auditOutput) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(output); err != nil {
		log.Print("加密信封审计结果输出失败")
		os.Exit(1)
	}
}

func parseAuditOptions(arguments []string, errorOutput io.Writer) (auditOptions, error) {
	flags := flag.NewFlagSet("field-encryption-audit", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	readVersions := flags.String("target-read-versions", "1,2", "comma-separated envelope versions supported by the target release")
	previousKeySupport := flags.Bool("target-supports-previous-keys", true, "whether the target release reads configured previous keys")
	requireUnusedPreviousKey := flags.Int("require-unused-previous-key", 0, "one-based previous-key slot that must have no stored dependency")
	rewrapPreviousKey := flags.Int("rewrap-previous-key", 0, "one-based previous-key slot to inventory or re-encrypt")
	executeRewrap := flags.Bool("execute-rewrap", false, "perform compare-and-swap re-encryption; requires the production maintenance marker")
	rewrapBatchSize := flags.Int("rewrap-batch-size", 100, "rows read per re-encryption transaction (1-1000)")
	if err := flags.Parse(arguments); err != nil {
		return auditOptions{}, err
	}
	if flags.NArg() != 0 {
		return auditOptions{}, fmt.Errorf("不接受位置参数")
	}
	versions, err := parseEnvelopeVersions(*readVersions)
	if err != nil {
		return auditOptions{}, err
	}
	if *requireUnusedPreviousKey < 0 {
		return auditOptions{}, fmt.Errorf("历史密钥位置必须为正整数")
	}
	if *rewrapPreviousKey < 0 || *rewrapBatchSize < 1 || *rewrapBatchSize > 1000 {
		return auditOptions{}, fmt.Errorf("重加密密钥位置或批量参数无效")
	}
	if *executeRewrap && *rewrapPreviousKey == 0 {
		return auditOptions{}, fmt.Errorf("执行重加密必须提供 --rewrap-previous-key")
	}
	if *rewrapPreviousKey > 0 && *requireUnusedPreviousKey > 0 {
		return auditOptions{}, fmt.Errorf("重加密与独立移除检查不能同时执行")
	}
	return auditOptions{
		targetReadVersions: versions, targetPreviousKeySupport: *previousKeySupport,
		requireUnusedPreviousKey: *requireUnusedPreviousKey,
		rewrapPreviousKey:        *rewrapPreviousKey, executeRewrap: *executeRewrap, rewrapBatchSize: *rewrapBatchSize,
	}, nil
}

func parseEnvelopeVersions(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	versions := make([]int, 0, len(parts))
	seen := make(map[int]bool, len(parts))
	for _, part := range parts {
		version, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || (version != 1 && version != 2) || seen[version] {
			return nil, fmt.Errorf("目标读取版本必须是无重复的 1 或 2")
		}
		seen[version] = true
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("目标读取版本不能为空")
	}
	return versions, nil
}

func evaluateSensitiveAudit(report *services.SensitiveFieldReadinessReport, options auditOptions) auditOutput {
	compatibility := services.AssessSensitiveFieldCompatibility(report, services.SensitiveEnvelopeReadCapabilities{
		ReadVersions: options.targetReadVersions, SupportsPreviousKeyring: options.targetPreviousKeySupport,
	})
	output := auditOutput{Status: "not_ready", Inventory: report, Compatibility: &compatibility}
	removalAllowed := true
	if options.requireUnusedPreviousKey > 0 {
		removalAllowed = services.SensitivePreviousKeySlotUnused(report, options.requireUnusedPreviousKey)
		output.PreviousKeyRemovalAllowed = &removalAllowed
	}
	if compatibility.Compatible && removalAllowed {
		output.Status = "ready"
	}
	return output
}

const productionMaintenanceFlag = "/etc/wangzhe/maintenance"

var rewrapCredentialDirectoryPattern = regexp.MustCompile(`^wangzhe-field-encryption-rewrap-[0-9]+\.service$`)

func productionMaintenanceCheck(required bool) func() error {
	if !required {
		return nil
	}
	return func() error {
		if err := validateMaintenanceFlag(productionMaintenanceFlag, 0); err != nil {
			return err
		}
		credentialDirectory := filepath.Clean(os.Getenv("CREDENTIALS_DIRECTORY"))
		if filepath.Dir(credentialDirectory) != "/run/credentials" ||
			!rewrapCredentialDirectoryPattern.MatchString(filepath.Base(credentialDirectory)) {
			return fmt.Errorf("缺少受控停写证明")
		}
		return validateFreezeProofDirectory(credentialDirectory, "/run/credentials", 0)
	}
}

func validateMaintenanceFlag(path string, trustedOwner uint32) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("维护标记不存在")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("维护标记类型或权限不安全")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != trustedOwner {
		return fmt.Errorf("维护标记所有者不可信")
	}
	return nil
}

func validateFreezeProofDirectory(directory, trustedRoot string, trustedOwner uint32) error {
	trustedRoot = filepath.Clean(trustedRoot)
	directory = filepath.Clean(directory)
	if filepath.Dir(directory) != trustedRoot {
		return fmt.Errorf("停写证明目录不受信任")
	}
	rootInfo, err := os.Lstat(trustedRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("停写证明根目录不安全")
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok || rootStat.Uid != trustedOwner {
		return fmt.Errorf("停写证明根目录所有者不可信")
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 || directoryInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("停写证明目录不安全")
	}
	directoryStat, ok := directoryInfo.Sys().(*syscall.Stat_t)
	if !ok || (directoryStat.Uid != trustedOwner && directoryStat.Uid != uint32(os.Geteuid())) {
		return fmt.Errorf("停写证明目录所有者或权限不可信")
	}
	// systemd credentials are an immutable, per-unit read-only mount. Depending
	// on systemd/kernel support its visible owner/mode may be root:0400 or the
	// service UID with 0600/0700 mode bits. Prove effective immutability instead
	// of rejecting the latter merely because owner-write bits are displayed.
	probe, probeErr := os.CreateTemp(directory, ".freeze-proof-write-probe-")
	if probeErr == nil {
		probeName := probe.Name()
		_ = probe.Close()
		_ = os.Remove(probeName)
		return fmt.Errorf("停写证明目录可写")
	}
	if !errors.Is(probeErr, os.ErrPermission) && !errors.Is(probeErr, syscall.EROFS) {
		return fmt.Errorf("无法证明停写目录只读")
	}
	proofPath := filepath.Join(directory, "freeze-proof")
	proofInfo, err := os.Lstat(proofPath)
	if err != nil || !proofInfo.Mode().IsRegular() || proofInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("停写证明文件不安全")
	}
	proofStat, ok := proofInfo.Sys().(*syscall.Stat_t)
	if !ok || proofStat.Nlink != 1 || (proofStat.Uid != trustedOwner && proofStat.Uid != uint32(os.Geteuid())) {
		return fmt.Errorf("停写证明文件所有者或链接数不可信")
	}
	writeProbe, writeErr := os.OpenFile(proofPath, os.O_WRONLY, 0)
	if writeErr == nil {
		_ = writeProbe.Close()
		return fmt.Errorf("停写证明文件可写")
	}
	if !errors.Is(writeErr, os.ErrPermission) && !errors.Is(writeErr, syscall.EROFS) {
		return fmt.Errorf("无法证明停写文件只读")
	}
	proof, err := os.Open(proofPath)
	if err != nil {
		return fmt.Errorf("停写证明不可读")
	}
	defer proof.Close()
	contents, err := io.ReadAll(io.LimitReader(proof, 65))
	if err != nil || string(contents) != "backend-writes-frozen-v1\n" {
		return fmt.Errorf("停写证明内容无效")
	}
	return nil
}
