package config

import (
	"backend/constants"
	"backend/data/models/activity"
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/entertainment"
	"backend/data/models/lottery"
	"backend/data/models/notify"
	"backend/data/models/odds"
	"backend/data/models/rebate"
	"backend/data/models/settings"
	"backend/data/models/special"
	"backend/data/models/user"
	"backend/data/models/wallet"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnectDB() (*gorm.DB, error) {
	config := GetConfig()
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		config.Database.Host,
		config.Database.User,
		config.Database.Password,
		config.Database.DBName,
		config.Database.Port,
		config.Database.SSLMode,
	)

	// 连接数据库
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

	// 获取底层sql.DB设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(10)           // 最大空闲连接数
	sqlDB.SetMaxOpenConns(100)          // 最大打开连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大生存时间

	if err := autoMigrate(db); err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseMigrationFailed, err)
	}

	log.Println(constants.DatabaseConnectionSuccess)
	return db, nil
}

func autoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&user.User{},
		&user.BalanceTransaction{},
		&application.Application{},
		&lottery.Game{},
		&lottery.Draw{},
		&settings.SystemConfig{},
		&odds.PlayLimit{},
		&odds.UserPlayOdds{},
		&wallet.PaymentChannel{},
		&bet.Bet{},
		&activity.Activity{},
		&activity.Participation{},
		&special.NumberResource{},
		&special.Campaign{},
		&special.GrantRecord{},
		&entertainment.Platform{},
		&notify.Notification{},
		&notify.MemberNotification{},
		&chat.Message{},
		&rebate.DailyRecord{},
	); err != nil {
		return err
	}
	// Normalize values used by older versions so the current admin UI has a
	// single stable role/risk vocabulary.
	if err := db.Model(&user.User{}).Where("role IS NULL OR role = '' OR role = 'user'").Update("role", "member").Error; err != nil {
		return err
	}
	if err := db.Model(&user.User{}).Where("risk_level IS NULL OR risk_level = ''").Update("risk_level", "normal").Error; err != nil {
		return err
	}
	// Backfill the audience field introduced for chat privacy. It preserves the
	// former agent-room behaviour while preventing historic messages from being
	// exposed to a newly assigned, unrelated room.
	if err := db.Exec(`
		UPDATE member_chat_messages
		SET scope = 'user:' || user_id::text
		WHERE room_type = 'service' AND (scope IS NULL OR scope = '' OR scope = 'lobby')
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		UPDATE member_chat_messages AS message
		SET scope = CASE
			WHEN account.role = 'agent' THEN 'agent:' || account.user_id::text
			WHEN account.parent_agent_id IS NOT NULL THEN 'agent:' || account.parent_agent_id::text
			ELSE 'lobby'
		END
		FROM "user" AS account
		WHERE message.user_id = account.user_id
			AND message.room_type = 'group'
			AND (message.scope IS NULL OR message.scope = '' OR message.scope = 'lobby')
	`).Error
}
