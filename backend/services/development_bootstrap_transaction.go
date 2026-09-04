package services

import (
	"fmt"

	"gorm.io/gorm"
)

type developmentBootstrapTransactionSteps struct {
	bootstrap    func(*gorm.DB, BootstrapOptions) error
	applyProfile func(*gorm.DB, string) (*DevelopmentBootstrapReport, error)
	finalize     func(*gorm.DB) error
}

// InitializeDevelopmentAcceptance commits the experience hierarchy, reviewed
// local odds profile and durable completion marker as one unit. Migrations are
// intentionally outside this transaction so an interrupted schema upgrade can
// be resumed before this all-or-nothing business-data initialization begins.
func InitializeDevelopmentAcceptance(db *gorm.DB, options BootstrapOptions, completionMarker string) (*DevelopmentBootstrapReport, error) {
	return initializeDevelopmentAcceptanceWithSteps(db, options, developmentBootstrapTransactionSteps{
		bootstrap:    Bootstrap,
		applyProfile: ApplyDevelopmentAcceptanceProfile,
		finalize: func(tx *gorm.DB) error {
			return setDevelopmentDatabaseMarker(tx, completionMarker)
		},
	})
}

func setDevelopmentDatabaseMarker(db *gorm.DB, marker string) error {
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	var statement string
	if err := db.Raw(`SELECT format('COMMENT ON DATABASE %I IS %L', current_database(), CAST(? AS text))`, marker).Scan(&statement).Error; err != nil {
		return fmt.Errorf("生成本地数据库初始化凭证失败: %w", err)
	}
	if statement == "" {
		return fmt.Errorf("生成的本地数据库初始化凭证为空")
	}
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("保存本地数据库初始化凭证失败: %w", err)
	}
	return nil
}

func initializeDevelopmentAcceptanceWithSteps(db *gorm.DB, options BootstrapOptions, steps developmentBootstrapTransactionSteps) (*DevelopmentBootstrapReport, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	if steps.bootstrap == nil || steps.applyProfile == nil || steps.finalize == nil {
		return nil, fmt.Errorf("本地验收初始化步骤不完整")
	}
	var report *DevelopmentBootstrapReport
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := steps.bootstrap(tx, options); err != nil {
			return err
		}
		configured, err := steps.applyProfile(tx, options.Mode)
		if err != nil {
			return fmt.Errorf("应用本地验收配置失败: %w", err)
		}
		if err := steps.finalize(tx); err != nil {
			return err
		}
		report = configured
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}
