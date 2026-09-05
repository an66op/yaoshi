package services

import (
	"backend/utils"
	"context"
	"database/sql"
	"fmt"
	"math"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	defaultSensitiveRewrapBatchSize = 100
	maxSensitiveRewrapBatchSize     = 1000
)

type SensitiveFieldRewrapOptions struct {
	PreviousKeyIndex int
	BatchSize        int
	Execute          bool
	MaintenanceCheck func() error

	// beforeCompareAndSwap is a test hook which carries only the logical field
	// name. Production callers leave it nil.
	beforeCompareAndSwap func(field string)
}

// SensitiveFieldRewrapReport never contains a stored value or key identifier.
// Inventory is a fresh full scan after execution (or the dry-run scan).
type SensitiveFieldRewrapReport struct {
	DryRun                bool                           `json:"dry_run"`
	PreviousKeyIndex      int                            `json:"previous_key_index"`
	BatchSize             int                            `json:"batch_size"`
	CandidateEnvelopes    uint64                         `json:"candidate_envelopes"`
	ExaminedRows          uint64                         `json:"examined_rows"`
	UpdatedEnvelopes      uint64                         `json:"updated_envelopes"`
	CompareAndSwapMisses  uint64                         `json:"compare_and_swap_misses"`
	RemainingDependencies uint64                         `json:"remaining_dependencies"`
	ReadyForKeyRemoval    bool                           `json:"ready_for_key_removal"`
	Inventory             *SensitiveFieldReadinessReport `json:"inventory"`
}

type sensitiveRewrapCandidate struct {
	id     uint64
	stored string
}

// RewrapSensitiveFieldsFromPreviousKey performs a full dry-run inventory by
// default. Execute mode requires a maintenance assertion before the run, before
// every batch and immediately before each compare-and-swap update. Every write
// includes the exact authenticated old envelope in its WHERE predicate, so a
// concurrent change is never overwritten. A final full inventory is always
// required before ReadyForKeyRemoval can become true.
func RewrapSensitiveFieldsFromPreviousKey(ctx context.Context, db *gorm.DB, options SensitiveFieldRewrapOptions) (*SensitiveFieldRewrapReport, error) {
	if db == nil {
		return nil, fmt.Errorf("敏感字段重加密数据库不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if options.PreviousKeyIndex <= 0 {
		return nil, fmt.Errorf("历史密钥位置必须为正整数")
	}
	batchSize := options.BatchSize
	if batchSize == 0 {
		batchSize = defaultSensitiveRewrapBatchSize
	}
	if batchSize < 1 || batchSize > maxSensitiveRewrapBatchSize {
		return nil, fmt.Errorf("敏感字段重加密批量必须在 1-%d 之间", maxSensitiveRewrapBatchSize)
	}
	if options.Execute && options.MaintenanceCheck == nil {
		return nil, fmt.Errorf("执行敏感字段重加密必须提供维护状态检查")
	}
	if options.Execute {
		if err := options.MaintenanceCheck(); err != nil {
			return nil, fmt.Errorf("敏感字段重加密要求维护模式: %w", err)
		}
	}

	initial, err := AuditSensitiveFieldReadiness(ctx, db)
	if err != nil {
		return nil, err
	}
	report := &SensitiveFieldRewrapReport{
		DryRun: !options.Execute, PreviousKeyIndex: options.PreviousKeyIndex, BatchSize: batchSize,
		CandidateEnvelopes: sensitivePreviousKeyDependencyCount(initial, options.PreviousKeyIndex),
		Inventory:          initial,
	}
	if !initial.Complete || !options.Execute {
		report.RemainingDependencies = report.CandidateEnvelopes
		// A dry-run is an informative snapshot only. Even a zero count cannot
		// authorize key removal because writers have not been proven frozen.
		report.ReadyForKeyRemoval = sensitiveRewrapRemovalReady(options.Execute, initial, report.RemainingDependencies)
		return report, nil
	}

	quietDB := db.Session(&gorm.Session{Logger: gormlogger.Discard})
	for _, column := range sensitiveFieldColumns {
		var cursor uint64
		for {
			if err := options.MaintenanceCheck(); err != nil {
				return nil, fmt.Errorf("敏感字段重加密期间维护模式失效: %w", err)
			}
			var batch []sensitiveRewrapCandidate
			var batchExamined, batchUpdated, batchMisses uint64
			err := quietDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				rows, err := tx.Raw(column.batchQuery, cursor, batchSize).Rows()
				if err != nil {
					return fmt.Errorf("读取敏感字段重加密批次失败: %w", err)
				}
				for rows.Next() {
					var candidate sensitiveRewrapCandidate
					var stored sql.NullString
					if err := rows.Scan(&candidate.id, &stored); err != nil {
						_ = rows.Close()
						return fmt.Errorf("扫描敏感字段重加密批次失败: %w", err)
					}
					candidate.stored = stored.String
					batch = append(batch, candidate)
				}
				if err := rows.Err(); err != nil {
					_ = rows.Close()
					return fmt.Errorf("遍历敏感字段重加密批次失败: %w", err)
				}
				if err := rows.Close(); err != nil {
					return fmt.Errorf("关闭敏感字段重加密批次失败: %w", err)
				}
				batchExamined = uint64(len(batch))
				for _, candidate := range batch {
					replacement, selected, err := utils.ReencryptSensitiveFromPreviousKey(candidate.stored, options.PreviousKeyIndex)
					if err != nil {
						return fmt.Errorf("认证敏感字段重加密候选失败: %w", err)
					}
					if !selected {
						continue
					}
					if options.beforeCompareAndSwap != nil {
						options.beforeCompareAndSwap(column.name)
					}
					if err := options.MaintenanceCheck(); err != nil {
						return fmt.Errorf("敏感字段写入前维护模式失效: %w", err)
					}
					result := tx.Exec(column.casUpdateQuery, replacement, candidate.id, candidate.stored)
					if result.Error != nil {
						return fmt.Errorf("敏感字段比较更新失败: %w", result.Error)
					}
					switch result.RowsAffected {
					case 0:
						batchMisses++
					case 1:
						batchUpdated++
					default:
						return fmt.Errorf("敏感字段比较更新影响了异常数量的记录")
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			report.ExaminedRows += batchExamined
			report.UpdatedEnvelopes += batchUpdated
			report.CompareAndSwapMisses += batchMisses
			if len(batch) == 0 {
				break
			}
			lastID := batch[len(batch)-1].id
			if lastID == math.MaxUint64 || len(batch) < batchSize {
				break
			}
			cursor = lastID
		}
	}

	if err := options.MaintenanceCheck(); err != nil {
		return nil, fmt.Errorf("敏感字段重加密最终盘点前维护模式失效: %w", err)
	}
	finalInventory, err := AuditSensitiveFieldReadiness(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := options.MaintenanceCheck(); err != nil {
		return nil, fmt.Errorf("敏感字段重加密最终盘点后维护模式失效: %w", err)
	}
	report.Inventory = finalInventory
	report.RemainingDependencies = sensitivePreviousKeyDependencyCount(finalInventory, options.PreviousKeyIndex)
	report.ReadyForKeyRemoval = sensitiveRewrapRemovalReady(options.Execute, finalInventory, report.RemainingDependencies)
	return report, nil
}

func sensitiveRewrapRemovalReady(executed bool, inventory *SensitiveFieldReadinessReport, remaining uint64) bool {
	return executed && inventory != nil && inventory.Complete && remaining == 0
}

func sensitivePreviousKeyDependencyCount(report *SensitiveFieldReadinessReport, previousKeyIndex int) uint64 {
	if report == nil || previousKeyIndex <= 0 {
		return 0
	}
	for _, dependency := range report.PreviousKeyDependencies {
		if dependency.PreviousKeyIndex == previousKeyIndex {
			return dependency.Total
		}
	}
	return 0
}
