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
	if err := prepareLegacySchema(db); err != nil {
		return err
	}
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
		&wallet.MemberPaymentAccount{},
		&bet.Bet{},
		&bet.AssistantRequest{},
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
	// Draw and settlement results have their own inbox. Reclassify historic
	// result messages so they no longer appear among service/system notices.
	if err := db.Exec(`
		UPDATE member_notifications
		SET category = 'winning'
		WHERE category = 'system'
			AND title IN ('开奖结果', '恭喜中奖', '未中奖')
	`).Error; err != nil {
		return err
	}
	// Betting-feed visibility is room scoped. Historic records predate this
	// field and cannot be attributed safely, so keep them in an unreachable
	// legacy scope instead of exposing them in any active room.
	if err := db.Exec(`
		UPDATE lottery_bets
		SET room_scope = 'legacy'
		WHERE room_scope IS NULL OR room_scope = ''
	`).Error; err != nil {
		return err
	}
	// GORM does not evolve a pre-existing unique index when a new column is
	// added. Rebuild it so a member can place the same selection in two rooms
	// without records being merged across room boundaries.
	if err := db.Exec(`DROP INDEX IF EXISTS idx_bet_dedupe`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_bet_dedupe
		ON lottery_bets (game_id, issue, room_scope, user_id, play_code, position, selection)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_bet_feed_scope
		ON lottery_bets (room_scope, game_id, issue, created_at)
	`).Error; err != nil {
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
	if err := db.Exec(`
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
	`).Error; err != nil {
		return err
	}
	return nil
}

func prepareLegacySchema(db *gorm.DB) error {
	// A public ID is deliberately separate from the internal primary key. It is
	// seven digits and remains stable when a member changes their nickname.
	if err := db.Exec(`CREATE SEQUENCE IF NOT EXISTS member_public_id_seq START WITH 1000000`).Error; err != nil {
		return err
	}
	if err := db.Exec(`ALTER TABLE "user" ADD COLUMN IF NOT EXISTS public_id bigint`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE "user"
		SET public_id = 1000000 + user_id
		WHERE public_id IS NULL OR public_id < 1000000
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		SELECT setval(
			'member_public_id_seq',
			GREATEST((SELECT COALESCE(MAX(public_id), 1000000) FROM "user"), 1000000),
			true
		)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		ALTER TABLE "user"
		ALTER COLUMN public_id SET DEFAULT nextval('member_public_id_seq'),
		ALTER COLUMN public_id SET NOT NULL
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_public_id ON "user" (public_id)`).Error; err != nil {
		return err
	}

	// Add biz_date as nullable first, backfill, then enforce NOT NULL before GORM AutoMigrate.
	if err := db.Exec(`
		ALTER TABLE activity_participations
		ADD COLUMN IF NOT EXISTS biz_date varchar(10)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE activity_participations
		SET biz_date = TO_CHAR(participated_at AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')
		WHERE biz_date IS NULL OR biz_date = ''
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE activity_participations
		SET biz_date = TO_CHAR(created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')
		WHERE biz_date IS NULL OR biz_date = ''
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		ALTER TABLE activity_participations
		ALTER COLUMN biz_date SET DEFAULT '',
		ALTER COLUMN biz_date SET NOT NULL
	`).Error; err != nil {
		return err
	}
	// Pool columns for red packet activities.
	if err := db.Exec(`
		ALTER TABLE ops_activities
		ADD COLUMN IF NOT EXISTS pool_total_cents bigint NOT NULL DEFAULT 0,
		ADD COLUMN IF NOT EXISTS pool_remaining_cents bigint NOT NULL DEFAULT 0
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		UPDATE ops_activities
		SET
			pool_total_cents = COALESCE(NULLIF(pool_total_cents, 0), GREATEST(COALESCE((config_json::jsonb->>'pool')::numeric, 88), 1) * 100),
			pool_remaining_cents = CASE
				WHEN pool_remaining_cents > 0 THEN pool_remaining_cents
				ELSE GREATEST(COALESCE((config_json::jsonb->>'pool')::numeric, 88), 1) * 100
			END
		WHERE type = 'redpacket' AND (pool_total_cents = 0 OR pool_remaining_cents = 0)
	`).Error
}
