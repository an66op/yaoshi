package services

import (
	"backend/utils"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sensitiveFieldColumns is the complete inventory of database columns written
// through utils.EncryptSensitive. Keep this deliberately explicit: adding a
// new encrypted column requires adding it here before production readiness can
// succeed for that data domain.
var sensitiveFieldColumns = []sensitiveFieldColumn{
	{
		name:           "member_payment_account_number",
		inventoryQuery: `SELECT "account_no" FROM "member_payment_accounts"`,
		batchQuery:     `SELECT "id", "account_no" FROM "member_payment_accounts" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
		casUpdateQuery: `UPDATE "member_payment_accounts" SET "account_no" = ? WHERE "id" = ? AND "account_no" = ?`,
	},
	{
		name:           "wallet_payment_channel_secret",
		inventoryQuery: `SELECT "secret_key" FROM "wallet_payment_channels"`,
		batchQuery:     `SELECT "id", "secret_key" FROM "wallet_payment_channels" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
		casUpdateQuery: `UPDATE "wallet_payment_channels" SET "secret_key" = ? WHERE "id" = ? AND "secret_key" = ?`,
	},
	{
		name:           "entertainment_platform_secret",
		inventoryQuery: `SELECT "secret_key" FROM "entertainment_platforms"`,
		batchQuery:     `SELECT "id", "secret_key" FROM "entertainment_platforms" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
		casUpdateQuery: `UPDATE "entertainment_platforms" SET "secret_key" = ? WHERE "id" = ? AND "secret_key" = ?`,
	},
}

type sensitiveFieldColumn struct {
	name           string
	inventoryQuery string
	batchQuery     string
	casUpdateQuery string
}

// SensitiveEnvelopeCounts is intentionally aggregate-only. It contains no row
// identifier, plaintext, ciphertext, key identifier or key material.
type SensitiveEnvelopeCounts struct {
	Total                uint64 `json:"total"`
	Empty                uint64 `json:"empty"`
	Plaintext            uint64 `json:"plaintext"`
	V1                   uint64 `json:"v1"`
	V2                   uint64 `json:"v2"`
	PrimaryKey           uint64 `json:"primary_key"`
	PreviousKey          uint64 `json:"previous_key"`
	Invalid              uint64 `json:"invalid"`
	UnsupportedVersion   uint64 `json:"unsupported_version"`
	Malformed            uint64 `json:"malformed"`
	Truncated            uint64 `json:"truncated"`
	AuthenticationFailed uint64 `json:"authentication_failed"`
	KeyUnavailable       uint64 `json:"key_unavailable"`
	OtherFailure         uint64 `json:"other_failure"`
}

type SensitiveFieldColumnReadiness struct {
	Field  string                  `json:"field"`
	Counts SensitiveEnvelopeCounts `json:"counts"`
}

// SensitivePreviousKeyDependency identifies only a one-based previous-key
// slot and aggregate dependency counts. The derived key identifier is never
// exposed.
type SensitivePreviousKeyDependency struct {
	PreviousKeyIndex int    `json:"previous_key_index"`
	Total            uint64 `json:"total"`
	V1               uint64 `json:"v1"`
	V2               uint64 `json:"v2"`
}

// SensitiveFieldReadinessReport proves that every stored sensitive value can
// be authenticated by the current keyring. Plaintext is reported separately
// and fails production readiness even though runtime reads retain migration
// compatibility.
type SensitiveFieldReadinessReport struct {
	Complete                bool                             `json:"complete"`
	AuditedColumns          int                              `json:"audited_columns"`
	Counts                  SensitiveEnvelopeCounts          `json:"counts"`
	Columns                 []SensitiveFieldColumnReadiness  `json:"columns"`
	PreviousKeyDependencies []SensitivePreviousKeyDependency `json:"previous_key_dependencies"`
}

// SensitiveEnvelopeReadCapabilities describes a release's database read
// contract. ReadVersions uses numeric envelope versions (1 and 2).
type SensitiveEnvelopeReadCapabilities struct {
	ReadVersions            []int
	SupportsPreviousKeyring bool
}

// SensitiveFieldCompatibility is also aggregate-only. Reasons are stable
// machine-readable categories and never include a stored value or key ID.
type SensitiveFieldCompatibility struct {
	Compatible bool     `json:"compatible"`
	Reasons    []string `json:"reasons,omitempty"`
}

// AuditSensitiveFieldReadiness examines a repeatable, read-only snapshot of
// every encrypted column, including soft-deleted rows. It authenticates each
// envelope but discards the plaintext immediately.
func AuditSensitiveFieldReadiness(ctx context.Context, db *gorm.DB) (*SensitiveFieldReadinessReport, error) {
	if db == nil {
		return nil, fmt.Errorf("敏感字段审计数据库不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	report := newSensitiveFieldReadinessReport()
	quietDB := db.Session(&gorm.Session{Logger: gormlogger.Discard})
	err := quietDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index, column := range sensitiveFieldColumns {
			rows, err := tx.Raw(column.inventoryQuery).Rows()
			if err != nil {
				return fmt.Errorf("读取敏感字段清单 %d 失败: %w", index+1, err)
			}
			for rows.Next() {
				var stored sql.NullString
				if err := rows.Scan(&stored); err != nil {
					_ = rows.Close()
					return fmt.Errorf("扫描敏感字段清单 %d 失败: %w", index+1, err)
				}
				inspectSensitiveFieldValue(report, index, stored.String)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("遍历敏感字段清单 %d 失败: %w", index+1, err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("关闭敏感字段清单 %d 失败: %w", index+1, err)
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("读取敏感字段加密完整度失败: %w", err)
	}
	finalizeSensitiveFieldReadiness(report)
	return report, nil
}

func newSensitiveFieldReadinessReport() *SensitiveFieldReadinessReport {
	report := &SensitiveFieldReadinessReport{
		AuditedColumns: len(sensitiveFieldColumns),
		Columns:        make([]SensitiveFieldColumnReadiness, len(sensitiveFieldColumns)),
	}
	for index, column := range sensitiveFieldColumns {
		report.Columns[index].Field = column.name
	}
	return report
}

func inspectSensitiveFieldValue(report *SensitiveFieldReadinessReport, columnIndex int, stored string) {
	counts := &report.Columns[columnIndex].Counts
	counts.Total++
	report.Counts.Total++
	inspection, err := utils.InspectSensitiveEnvelope(stored)
	if err != nil {
		incrementSensitiveEnvelopeFailure(counts, err)
		incrementSensitiveEnvelopeFailure(&report.Counts, err)
		return
	}
	switch inspection.Version {
	case "empty":
		counts.Empty++
		report.Counts.Empty++
	case "plaintext":
		counts.Plaintext++
		report.Counts.Plaintext++
	case "v1":
		counts.V1++
		report.Counts.V1++
		incrementSensitiveKeyUsage(counts, inspection.PreviousKeyIndex)
		incrementSensitiveKeyUsage(&report.Counts, inspection.PreviousKeyIndex)
		incrementPreviousKeyDependency(report, inspection.PreviousKeyIndex, 1)
	case "v2":
		counts.V2++
		report.Counts.V2++
		incrementSensitiveKeyUsage(counts, inspection.PreviousKeyIndex)
		incrementSensitiveKeyUsage(&report.Counts, inspection.PreviousKeyIndex)
		incrementPreviousKeyDependency(report, inspection.PreviousKeyIndex, 2)
	default:
		// Treat any future inspector classification as unsafe until this
		// inventory has explicitly learned its semantics.
		counts.Invalid++
		counts.OtherFailure++
		report.Counts.Invalid++
		report.Counts.OtherFailure++
	}
}

func incrementSensitiveEnvelopeFailure(counts *SensitiveEnvelopeCounts, err error) {
	counts.Invalid++
	switch {
	case errors.Is(err, utils.ErrSensitiveEnvelopeUnsupportedVersion):
		counts.UnsupportedVersion++
	case errors.Is(err, utils.ErrSensitiveEnvelopeMalformed):
		counts.Malformed++
	case errors.Is(err, utils.ErrSensitiveEnvelopeTruncated):
		counts.Truncated++
	case errors.Is(err, utils.ErrSensitiveEnvelopeAuthentication):
		counts.AuthenticationFailed++
	case errors.Is(err, utils.ErrSensitiveEnvelopeKeyUnavailable):
		counts.KeyUnavailable++
	default:
		counts.OtherFailure++
	}
}

func incrementSensitiveKeyUsage(counts *SensitiveEnvelopeCounts, previousKeyIndex int) {
	if previousKeyIndex > 0 {
		counts.PreviousKey++
		return
	}
	counts.PrimaryKey++
}

func incrementPreviousKeyDependency(report *SensitiveFieldReadinessReport, previousKeyIndex, version int) {
	if previousKeyIndex <= 0 {
		return
	}
	for index := range report.PreviousKeyDependencies {
		dependency := &report.PreviousKeyDependencies[index]
		if dependency.PreviousKeyIndex != previousKeyIndex {
			continue
		}
		dependency.Total++
		if version == 1 {
			dependency.V1++
		} else {
			dependency.V2++
		}
		return
	}
	dependency := SensitivePreviousKeyDependency{PreviousKeyIndex: previousKeyIndex, Total: 1}
	if version == 1 {
		dependency.V1 = 1
	} else {
		dependency.V2 = 1
	}
	report.PreviousKeyDependencies = append(report.PreviousKeyDependencies, dependency)
}

func finalizeSensitiveFieldReadiness(report *SensitiveFieldReadinessReport) {
	sort.Slice(report.PreviousKeyDependencies, func(left, right int) bool {
		return report.PreviousKeyDependencies[left].PreviousKeyIndex < report.PreviousKeyDependencies[right].PreviousKeyIndex
	})
	report.Complete = report.AuditedColumns == len(sensitiveFieldColumns) &&
		report.Counts.Invalid == 0 && report.Counts.Plaintext == 0
}

// AssessSensitiveFieldCompatibility applies target release capabilities to a
// completed inventory. It covers both legacy v1 and identified v2 envelopes,
// including values still dependent on any configured previous key.
func AssessSensitiveFieldCompatibility(report *SensitiveFieldReadinessReport, capabilities SensitiveEnvelopeReadCapabilities) SensitiveFieldCompatibility {
	var reasons []string
	if report == nil || !report.Complete {
		reasons = append(reasons, "inventory_incomplete")
	} else {
		readVersions := make(map[int]bool, len(capabilities.ReadVersions))
		for _, version := range capabilities.ReadVersions {
			readVersions[version] = true
		}
		if report.Counts.V1 > 0 && !readVersions[1] {
			reasons = append(reasons, "v1_not_readable")
		}
		if report.Counts.V2 > 0 && !readVersions[2] {
			reasons = append(reasons, "v2_not_readable")
		}
		if report.Counts.PreviousKey > 0 && !capabilities.SupportsPreviousKeyring {
			reasons = append(reasons, "previous_key_not_readable")
		}
	}
	return SensitiveFieldCompatibility{Compatible: len(reasons) == 0, Reasons: reasons}
}

// SensitivePreviousKeySlotUnused is the pre-removal assertion. Callers must
// execute it while the candidate key is still configured and while encrypted
// writes are quiesced; an absent dependency entry means zero stored envelopes
// rely on that one-based previous-key slot.
func SensitivePreviousKeySlotUnused(report *SensitiveFieldReadinessReport, previousKeyIndex int) bool {
	if report == nil || !report.Complete || previousKeyIndex <= 0 {
		return false
	}
	for _, dependency := range report.PreviousKeyDependencies {
		if dependency.PreviousKeyIndex == previousKeyIndex {
			return dependency.Total == 0
		}
	}
	return true
}
