package config

import (
	"backend/constants"
	"backend/data/models/activity"
	"backend/data/models/application"
	"backend/data/models/audit"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/entertainment"
	"backend/data/models/lottery"
	"backend/data/models/notify"
	"backend/data/models/odds"
	"backend/data/models/plan"
	"backend/data/models/profitshare"
	"backend/data/models/rebate"
	"backend/data/models/settings"
	"backend/data/models/special"
	"backend/data/models/user"
	"backend/data/models/wallet"
	workspacemodel "backend/data/models/workspace"
	"backend/migrations"
	"backend/utils"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnectDB() (*gorm.DB, error) {
	db, err := OpenDatabase()
	if err != nil {
		return nil, err
	}

	// Application startup is migration-only. In particular, production must
	// never infer schema changes from the current Go models: every change is an
	// immutable SQL file with a checksum in schema_migrations.
	if err := migrations.Run(db); err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseMigrationFailed, err)
	}

	log.Println(constants.DatabaseConnectionSuccess)
	return db, nil
}

// OpenDatabase opens and configures the PostgreSQL connection without making
// any schema changes. Normal application code should use ConnectDB. It is
// exported for the explicit legacy-bootstrap command only.
func OpenDatabase() (*gorm.DB, error) {
	config := GetConfig()
	dsn, err := BuildPostgresDSN(config.Database)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

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

	return db, nil
}

// BuildPostgresDSN returns a pgx-compatible URL. Building a keyword DSN with
// fmt.Sprintf is unsafe because spaces, quotes, backslashes and equal signs in
// a generated password can change how pgx parses the following fields.
// url.UserPassword and url.Values preserve every credential byte without
// logging the resulting secret-bearing URL.
func BuildPostgresDSN(cfg DatabaseConfig) (string, error) {
	host := strings.TrimSpace(cfg.Host)
	username := cfg.User
	database := cfg.DBName
	sslMode := strings.TrimSpace(cfg.SSLMode)
	if host == "" || strings.TrimSpace(username) == "" || strings.TrimSpace(database) == "" {
		return "", fmt.Errorf("数据库主机、用户和库名不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("数据库端口必须在1-65535之间")
	}

	connectionURL := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, cfg.Password),
		Path:   "/" + database,
	}
	// Keep every application connection pinned to the versioned application
	// schema. This prevents an account-level or role-level search_path from
	// redirecting the migration runner or an unqualified legacy query into a stale copy.
	query := url.Values{
		"sslmode":     []string{sslMode},
		"search_path": []string{"public"},
	}
	if strings.HasPrefix(host, "/") {
		// PostgreSQL Unix sockets are represented as an escaped query value;
		// putting an absolute path in URL.Host would produce an invalid URL.
		query.Set("host", host)
		query.Set("port", strconv.Itoa(cfg.Port))
	} else {
		connectionURL.Host = net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	}
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String(), nil
}

// BootstrapLegacySchema upgrades an installation created before the versioned
// core-schema baseline. It intentionally remains separate from ConnectDB and
// is only invoked by cmd/db-bootstrap after an explicit confirmation token.
// Do not call this from a server startup path and do not add new schema changes
// here; all future changes belong in migrations/*.sql.
func BootstrapLegacySchema(db *gorm.DB) error {
	// A fresh database does not have the legacy user table yet, but the
	// PublicID column default still needs its allocator before AutoMigrate emits
	// CREATE TABLE. Existing databases continue through the full backfill path.
	if db.Migrator().HasTable(&user.User{}) {
		if err := prepareLegacySchema(db); err != nil {
			return err
		}
	} else if err := installLegacyMemberPublicIDAllocator(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&user.User{},
		&user.BalanceTransaction{},
		&application.Application{},
		&lottery.Game{},
		&lottery.LobbyCategory{},
		&lottery.Draw{},
		&lottery.Issue{},
		&settings.SystemConfig{},
		&workspacemodel.Workspace{},
		&workspacemodel.Membership{},
		&workspacemodel.RobotProfile{},
		&workspacemodel.RobotGame{},
		&workspacemodel.RobotSetting{},
		&workspacemodel.RobotResetReceipt{},
		&odds.PlayLimit{},
		&odds.UserPlayOdds{},
		&odds.RoomPlayOdds{},
		&plan.Recommendation{},
		&wallet.PaymentChannel{},
		&wallet.MemberPaymentAccount{},
		&bet.Bet{},
		&bet.AssistantRequest{},
		&bet.BetRequest{},
		&activity.Activity{},
		&activity.Participation{},
		&special.NumberResource{},
		&special.Campaign{},
		&special.GrantRecord{},
		&entertainment.Platform{},
		&notify.Notification{},
		&notify.MemberNotification{},
		&chat.Message{},
		&chat.ReadCursor{},
		&chat.RedPacket{},
		&chat.RedPacketClaim{},
		&chat.RoomGameSetting{},
		&rebate.DailyRecord{},
		&profitshare.DailyRecord{},
		&audit.Log{},
	); err != nil {
		return err
	}
	if err := hardenLoginIdentity(db); err != nil {
		return err
	}
	// Normalize envelopes created by older admin builds that copied the
	// operator login name into the public chat preview (for example “admin”).
	// Internal operator identity remains available through the admin action
	// audit; member chat consistently shows the room-benefit identity.
	if err := db.Exec(`
		UPDATE member_chat_messages
		SET username = 'support', nickname = '房间福利'
		WHERE message_type = 'redpacket'
			AND (username <> 'support' OR nickname <> '房间福利')
	`).Error; err != nil {
		return err
	}
	if err := encryptSensitiveRecords(db); err != nil {
		return err
	}
	if err := hardenFinancialLedger(db); err != nil {
		return err
	}
	if err := hardenFinancialRecords(db); err != nil {
		return err
	}
	if err := hardenRemainingFinancialRecords(db); err != nil {
		return err
	}
	// The original participation key ended at biz_date. That made the first
	// invitation reward permanent for an inviter and caused every later friend
	// registration to collide with it. Rebuild the additive index with a stable
	// business reference while preserving daily check-in/red-packet semantics
	// (their reference remains the empty string).
	if err := db.Exec(`DROP INDEX IF EXISTS idx_participation_daily_unique`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_participation_daily_unique
		ON activity_participations (workspace_id, user_id, activity_id, action, biz_date, reference)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP INDEX IF EXISTS idx_rebate_user_day`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_rebate_user_day
		ON rebate_daily_records (workspace_id, biz_date, user_id)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP INDEX IF EXISTS idx_profit_share_agent_day`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_profit_share_agent_day
		ON agent_profit_share_records (workspace_id, biz_date, agent_id)
	`).Error; err != nil {
		return err
	}
	// Empty room codes are normal for members and administrators. PostgreSQL
	// otherwise treats the empty string as a real unique value, which prevents
	// the second ordinary account from being created.
	if err := db.Exec(`DROP INDEX IF EXISTS idx_user_agent_room_code`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_agent_room_code
		ON "user" (agent_room_code)
		WHERE agent_room_code <> '' AND deleted_at IS NULL
	`).Error; err != nil {
		return err
	}
	// Retire the original public demo labels without overwriting nicknames that
	// members have already customized. Future restarts preserve the stored name.
	if err := db.Exec(`
		UPDATE "user"
		SET nickname = '王者玩家'
		WHERE username = 'wangzhe88' AND nickname = '体验玩家'
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE user_balance_transactions
		SET remark = '账户初始化'
		WHERE remark = '体验账户'
	`).Error; err != nil {
		return err
	}
	// A nickname is mutable user identity, not immutable message content.
	// Synchronize historic sender snapshots so every surface immediately shows
	// the member's current name after an edit.
	if err := db.Exec(`
		UPDATE member_chat_messages AS message
		SET username = account.username,
			nickname = COALESCE(NULLIF(account.nickname, ''), account.username)
		FROM "user" AS account
		WHERE message.user_id = account.user_id
			AND (
				message.username IS DISTINCT FROM account.username
				OR message.nickname IS DISTINCT FROM COALESCE(NULLIF(account.nickname, ''), account.username)
			)
	`).Error; err != nil {
		return err
	}
	// Draw and settlement results have their own inbox. Reclassify historic
	// result messages so they no longer appear among service/system notices.
	if err := db.Exec(`
		UPDATE member_notifications
		SET category = 'winning'
		WHERE category = 'system'
			AND title IN ('开奖结果', '恭喜中奖', '未中奖', '开奖通知')
	`).Error; err != nil {
		return err
	}
	// System notices contain only platform/service announcements.  Historic
	// rewards and account-review results are moved into the account inbox
	// inboxes without changing the recipient or deleting the audit trail.
	if err := db.Exec(`
		UPDATE member_notifications
		SET category = 'account'
		WHERE category = 'system'
			AND title IN ('签到成功', '红包领取成功', '邀请成功', '注册奖励到账', '邀请奖励到账')
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE member_notifications
		SET category = 'account'
		WHERE category = 'system'
			AND title IN ('申请审核结果', '申请已通过', '申请未通过')
	`).Error; err != nil {
		return err
	}
	// Settlement notifications created before game/room snapshots were added
	// can be attributed safely only when a matching historic bet exists.  The
	// game name is part of the match because some providers reuse date-based
	// issue numbers across multiple games.
	if err := db.Exec(`
		UPDATE member_notifications AS notice
		SET game_id = source.game_id,
			room_scope = source.room_scope,
			event_key = CASE
				WHEN notice.id = (
					SELECT MAX(peer.id) FROM member_notifications AS peer
					WHERE peer.category = 'winning'
						AND peer.user_id = notice.user_id
						AND peer.issue = notice.issue
						AND peer.game_name = notice.game_name
				) THEN 'settlement:' || source.game_id || ':' || notice.issue || ':' || notice.user_id::text || ':' || source.room_scope
				ELSE ''
			END
		FROM (
			SELECT DISTINCT ON (placed.user_id, placed.issue, game.name)
				placed.user_id, placed.issue, placed.game_id, placed.room_scope, game.name AS game_name
			FROM lottery_bets AS placed
			JOIN lottery_games AS game ON game.id = placed.game_id
			WHERE placed.room_scope IS NOT NULL AND placed.room_scope <> '' AND placed.room_scope <> 'legacy'
			ORDER BY placed.user_id, placed.issue, game.name, placed.created_at DESC
		) AS source
		WHERE notice.category = 'winning'
			AND notice.user_id = source.user_id
			AND notice.issue = source.issue
			AND notice.game_name = source.game_name
			AND (notice.game_id IS NULL OR notice.game_id = '')
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_member_notification_event_key
		ON member_notifications (event_key)
		WHERE event_key <> ''
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
	// Historic bets without a durable issue cannot be settled safely. Keep the
	// original rows and balances untouched while exposing them for reconciliation.
	if err := db.Exec(`
		UPDATE lottery_bets AS bet
		SET reconciliation_status = 'abnormal',
			reconciliation_note = '无法匹配期号，等待人工核对'
		WHERE bet.reconciliation_status = 'normal'
			AND NOT EXISTS (
				SELECT 1 FROM lottery_issues AS issue
				WHERE issue.game_id = bet.game_id AND issue.issue = bet.issue
			)
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
	// Additive chat isolation. Historic group messages did not record a game,
	// so they remain available only in the explicit legacy conversation instead
	// of being guessed into a real lottery room. Service history is associated
	// with the member's current owning room and remains private to that member.
	if err := db.Exec(`
		UPDATE member_chat_messages
		SET room_scope = scope,
			game_id = 'legacy'
		WHERE room_type = 'group'
			AND (room_scope IS NULL OR room_scope = '' OR room_scope = 'legacy')
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE member_chat_messages AS message
		SET room_scope = CASE
				WHEN account.parent_agent_id IS NOT NULL THEN 'agent:' || account.parent_agent_id::text
				ELSE 'lobby'
			END,
			game_id = 'service'
		FROM "user" AS account
		WHERE message.user_id = account.user_id
			AND message.room_type = 'service'
			AND (message.room_scope IS NULL OR message.room_scope = '' OR message.room_scope = 'legacy')
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE member_chat_messages AS message
		SET room_scope = CASE
				WHEN account.parent_agent_id IS NOT NULL THEN 'agent:' || account.parent_agent_id::text
				ELSE 'lobby'
			END,
			game_id = 'service'
		FROM "user" AS account
		WHERE message.scope = 'user:' || account.user_id::text
			AND message.room_type = 'service'
			AND (message.room_scope IS NULL OR message.room_scope = '' OR message.room_scope = 'legacy')
	`).Error; err != nil {
		return err
	}
	return nil
}

func encryptSensitiveRecords(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var accounts []wallet.MemberPaymentAccount
		if err := tx.Where("account_no <> '' AND account_no NOT LIKE ?", "enc:v1:%").Find(&accounts).Error; err != nil {
			return err
		}
		for _, account := range accounts {
			encrypted, err := utils.EncryptSensitive(account.AccountNo)
			if err != nil {
				return fmt.Errorf("encrypt payment account %d: %w", account.ID, err)
			}
			if err := tx.Model(&wallet.MemberPaymentAccount{}).Where("id = ?", account.ID).UpdateColumn("account_no", encrypted).Error; err != nil {
				return err
			}
		}

		var platforms []entertainment.Platform
		if err := tx.Where("secret_key <> '' AND secret_key NOT LIKE ?", "enc:v1:%").Find(&platforms).Error; err != nil {
			return err
		}
		for _, platform := range platforms {
			encrypted, err := utils.EncryptSensitive(platform.SecretKey)
			if err != nil {
				return fmt.Errorf("encrypt entertainment platform %d: %w", platform.ID, err)
			}
			if err := tx.Model(&entertainment.Platform{}).Where("id = ?", platform.ID).UpdateColumn("secret_key", encrypted).Error; err != nil {
				return err
			}
		}

		var paymentChannels []wallet.PaymentChannel
		if err := tx.Where("secret_key <> '' AND secret_key NOT LIKE ?", "enc:v1:%").Find(&paymentChannels).Error; err != nil {
			return err
		}
		for _, channel := range paymentChannels {
			encrypted, err := utils.EncryptSensitive(channel.SecretKey)
			if err != nil {
				return fmt.Errorf("encrypt payment channel %d: %w", channel.ID, err)
			}
			if err := tx.Model(&wallet.PaymentChannel{}).Where("id = ?", channel.ID).UpdateColumn("secret_key", encrypted).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// hardenFinancialLedger repairs the small, mechanically provable legacy
// inconsistencies left by older seed/check-in code, then moves the two core
// money invariants into PostgreSQL. Application transactions remain the first
// line of defence; these constraints make an accidental direct write fail
// closed instead of silently corrupting the ledger.
func hardenFinancialLedger(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			WITH ordered AS (
				SELECT id, amount_cents, before_cents, after_cents,
					LAG(after_cents) OVER (PARTITION BY user_id ORDER BY id) AS prior_after
				FROM user_balance_transactions
			), repairable AS (
				SELECT id,
					CASE WHEN prior_after IS NULL
						THEN after_cents - amount_cents
						ELSE prior_after
					END AS corrected_before
				FROM ordered
				WHERE after_cents <> before_cents + amount_cents
					AND (
						(prior_after IS NULL AND before_cents = after_cents)
						OR (prior_after IS NOT NULL AND after_cents = prior_after + amount_cents)
					)
			)
			UPDATE user_balance_transactions AS ledger
			SET before_cents = repairable.corrected_before
			FROM repairable
			WHERE ledger.id = repairable.id
		`).Error; err != nil {
			return fmt.Errorf("修复历史资金流水失败: %w", err)
		}

		var invalidArithmetic int64
		if err := tx.Raw(`
			SELECT COUNT(*)
			FROM user_balance_transactions
			WHERE after_cents <> before_cents + amount_cents
				OR before_cents < 0 OR after_cents < 0
		`).Scan(&invalidArithmetic).Error; err != nil {
			return err
		}
		if invalidArithmetic != 0 {
			return fmt.Errorf("存在 %d 条无法自动证明的异常资金流水，拒绝启用资金约束", invalidArithmetic)
		}

		var negativeBalances int64
		if err := tx.Raw(`SELECT COUNT(*) FROM "user" WHERE balance_cents < 0`).Scan(&negativeBalances).Error; err != nil {
			return err
		}
		if negativeBalances != 0 {
			return fmt.Errorf("存在 %d 个负余额账户，拒绝启用余额约束", negativeBalances)
		}

		statements := []string{
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_user_balance_nonnegative') THEN
					ALTER TABLE "user" ADD CONSTRAINT chk_user_balance_nonnegative CHECK (balance_cents >= 0) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE "user" VALIDATE CONSTRAINT chk_user_balance_nonnegative`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_balance_ledger_arithmetic') THEN
					ALTER TABLE user_balance_transactions ADD CONSTRAINT chk_balance_ledger_arithmetic
					CHECK (after_cents = before_cents + amount_cents AND before_cents >= 0 AND after_cents >= 0) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE user_balance_transactions VALIDATE CONSTRAINT chk_balance_ledger_arithmetic`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_balance_ledger_user') THEN
					ALTER TABLE user_balance_transactions ADD CONSTRAINT fk_balance_ledger_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE user_balance_transactions VALIDATE CONSTRAINT fk_balance_ledger_user`,
			`CREATE INDEX IF NOT EXISTS idx_balance_ledger_user_id_id ON user_balance_transactions (user_id, id)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_balance_ledger_user_reference
				ON user_balance_transactions (user_id, reference) WHERE reference <> ''`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("启用资金数据库约束失败: %w", err)
			}
		}
		return nil
	})
}

// hardenRemainingFinancialRecords protects the secondary money and notification
// tables that are not part of the bet/application transaction itself.  It also
// normalizes the legacy default-payment-account state before enforcing one
// default per member at the database level.
func hardenRemainingFinancialRecords(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			WITH ranked AS (
				SELECT id,
					ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY is_default DESC, id DESC) AS position
				FROM member_payment_accounts
				WHERE deleted_at IS NULL
			)
			UPDATE member_payment_accounts AS account
			SET is_default = (ranked.position = 1)
			FROM ranked
			WHERE account.id = ranked.id
				AND account.deleted_at IS NULL
				AND account.is_default IS DISTINCT FROM (ranked.position = 1)
		`).Error; err != nil {
			return fmt.Errorf("修复默认收款账户失败: %w", err)
		}

		checks := []struct {
			name  string
			query string
		}{
			{
				name:  "会员收款账户",
				query: `SELECT COUNT(*) FROM member_payment_accounts WHERE account_type NOT IN ('bank', 'alipay', 'wechat', 'usdt')`,
			},
			{
				name: "平台收款渠道",
				query: `SELECT COUNT(*) FROM wallet_payment_channels
					WHERE fee_rate NOT BETWEEN 0 AND 100 OR min_amount < 0 OR max_amount < 0
						OR (max_amount <> 0 AND max_amount < min_amount)
						OR credit_type NOT IN ('manual', 'bank', 'alipay', 'wechat', 'usdt')
						OR status NOT IN ('enabled', 'disabled')`,
			},
			{
				name: "每日回水",
				query: `SELECT COUNT(*) FROM rebate_daily_records
					WHERE turnover_cents < 0 OR rate_percent NOT BETWEEN 0 AND 100
						OR amount_cents < 0 OR status <> 'credited'`,
			},
			{
				name: "代理分成",
				query: `SELECT COUNT(*) FROM agent_profit_share_records
					WHERE bet_count < 0 OR turnover_cents < 0 OR payout_cents < 0 OR rebate_cents < 0
						OR accrued_share_cents < 0 OR paid_share_cents < 0
						OR paid_share_cents > accrued_share_cents OR run_count < 0
						OR BTRIM(room_scope) = '' OR status NOT IN ('pending', 'credited')`,
			},
			{
				name: "会员通知资金摘要",
				query: `SELECT COUNT(*) FROM member_notifications
					WHERE bet_count < 0 OR won_count < 0 OR won_count > bet_count
						OR stake_cents < 0 OR payout_cents < 0
						OR category NOT IN ('system', 'account', 'activity', 'winning')
						OR level NOT IN ('info', 'success', 'warning', 'error')`,
			},
		}
		for _, check := range checks {
			var invalid int64
			if err := tx.Raw(check.query).Scan(&invalid).Error; err != nil {
				return fmt.Errorf("核对%s失败: %w", check.name, err)
			}
			if invalid != 0 {
				return fmt.Errorf("存在 %d 条非法%s记录，拒绝启用资金约束", invalid, check.name)
			}
		}

		statements := []string{
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_member_payment_account_type' AND conrelid = 'member_payment_accounts'::regclass) THEN
					ALTER TABLE member_payment_accounts ADD CONSTRAINT chk_member_payment_account_type
					CHECK (account_type IN ('bank', 'alipay', 'wechat', 'usdt')) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE member_payment_accounts VALIDATE CONSTRAINT chk_member_payment_account_type`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_member_payment_account_user' AND conrelid = 'member_payment_accounts'::regclass) THEN
					ALTER TABLE member_payment_accounts ADD CONSTRAINT fk_member_payment_account_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE member_payment_accounts VALIDATE CONSTRAINT fk_member_payment_account_user`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_member_payment_account_one_default
				ON member_payment_accounts (user_id) WHERE is_default AND deleted_at IS NULL`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_wallet_payment_channel_financials' AND conrelid = 'wallet_payment_channels'::regclass) THEN
					ALTER TABLE wallet_payment_channels ADD CONSTRAINT chk_wallet_payment_channel_financials CHECK (
						fee_rate BETWEEN 0 AND 100 AND min_amount >= 0 AND max_amount >= 0
						AND (max_amount = 0 OR max_amount >= min_amount)
						AND credit_type IN ('manual', 'bank', 'alipay', 'wechat', 'usdt')
						AND status IN ('enabled', 'disabled')
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE wallet_payment_channels VALIDATE CONSTRAINT chk_wallet_payment_channel_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_rebate_daily_financials' AND conrelid = 'rebate_daily_records'::regclass) THEN
					ALTER TABLE rebate_daily_records ADD CONSTRAINT chk_rebate_daily_financials CHECK (
						turnover_cents >= 0 AND rate_percent BETWEEN 0 AND 100
						AND amount_cents >= 0 AND status = 'credited'
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE rebate_daily_records VALIDATE CONSTRAINT chk_rebate_daily_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rebate_daily_user' AND conrelid = 'rebate_daily_records'::regclass) THEN
					ALTER TABLE rebate_daily_records ADD CONSTRAINT fk_rebate_daily_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE rebate_daily_records VALIDATE CONSTRAINT fk_rebate_daily_user`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_agent_profit_share_financials' AND conrelid = 'agent_profit_share_records'::regclass) THEN
					ALTER TABLE agent_profit_share_records ADD CONSTRAINT chk_agent_profit_share_financials CHECK (
						bet_count >= 0 AND turnover_cents >= 0 AND payout_cents >= 0 AND rebate_cents >= 0
						AND accrued_share_cents >= 0 AND paid_share_cents >= 0
						AND paid_share_cents <= accrued_share_cents AND run_count >= 0
						AND BTRIM(room_scope) <> '' AND status IN ('pending', 'credited')
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE agent_profit_share_records VALIDATE CONSTRAINT chk_agent_profit_share_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_agent_profit_share_agent' AND conrelid = 'agent_profit_share_records'::regclass) THEN
					ALTER TABLE agent_profit_share_records ADD CONSTRAINT fk_agent_profit_share_agent
					FOREIGN KEY (agent_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE agent_profit_share_records VALIDATE CONSTRAINT fk_agent_profit_share_agent`,
			// Earlier builds forgot the account inbox category even though the
			// application model and member APIs already supported it.  Replace
			// that exact legacy constraint in-place so chat red-packet claims can
			// create their balance notification in the same transaction.
			`DO $$
			DECLARE constraint_definition text;
			BEGIN
				SELECT pg_get_constraintdef(oid) INTO constraint_definition
				FROM pg_constraint
				WHERE conname = 'chk_member_notification_financials'
					AND conrelid = 'member_notifications'::regclass;
				IF constraint_definition IS NOT NULL
					AND POSITION('account' IN constraint_definition) = 0 THEN
					ALTER TABLE member_notifications DROP CONSTRAINT chk_member_notification_financials;
				END IF;
			END $$`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_member_notification_financials' AND conrelid = 'member_notifications'::regclass) THEN
					ALTER TABLE member_notifications ADD CONSTRAINT chk_member_notification_financials CHECK (
						bet_count >= 0 AND won_count >= 0 AND won_count <= bet_count
						AND stake_cents >= 0 AND payout_cents >= 0
						AND category IN ('system', 'account', 'activity', 'winning')
						AND level IN ('info', 'success', 'warning', 'error')
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE member_notifications VALIDATE CONSTRAINT chk_member_notification_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_member_notification_user' AND conrelid = 'member_notifications'::regclass) THEN
					ALTER TABLE member_notifications ADD CONSTRAINT fk_member_notification_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE member_notifications VALIDATE CONSTRAINT fk_member_notification_user`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("启用扩展资金数据库约束失败: %w", err)
			}
		}
		return nil
	})
}

// hardenFinancialRecords gives the remaining money-bearing tables the same
// fail-closed protection as the balance ledger.  Services still validate
// requests first so members receive friendly errors; these constraints are the
// last line of defence against a future direct SQL write, missed code path or
// partial migration creating an impossible financial record.
func hardenFinancialRecords(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		checks := []struct {
			name  string
			query string
		}{
			{
				name: "注单",
				query: `SELECT COUNT(*) FROM lottery_bets
					WHERE amount_cents <= 0 OR odds <= 0
						OR payout_cents < 0 OR fly_cents < 0 OR rebate_cents < 0 OR agent_share_cents < 0
						OR rebate_rate_snapshot < 0 OR rebate_rate_snapshot > 100
						OR agent_share_rate_snapshot < 0 OR agent_share_rate_snapshot > 100
						OR status NOT IN ('pending', 'won', 'lost', 'cancelled')
						OR reconciliation_status NOT IN ('normal', 'abnormal', 'resolved')`,
			},
			{
				name: "上下分申请",
				query: `SELECT COUNT(*) FROM user_applications
					WHERE ((request_type IN ('credit', 'debit') AND requested_cents <= 0)
						OR (request_type IN ('agent', 'join') AND requested_cents <> 0))
						OR received_cents < 0
						OR request_type NOT IN ('credit', 'debit', 'agent', 'join')
						OR status NOT IN ('pending', 'approved', 'rejected')`,
			},
			{
				name:  "活动参与",
				query: `SELECT COUNT(*) FROM activity_participations WHERE reward_cents < 0 OR streak < 0`,
			},
			{
				name: "活动资金池",
				query: `SELECT COUNT(*) FROM ops_activities
					WHERE reward_cents < 0 OR pool_total_cents < 0 OR pool_remaining_cents < 0
						OR pool_remaining_cents > pool_total_cents OR participants < 0`,
			},
		}
		for _, check := range checks {
			var invalid int64
			if err := tx.Raw(check.query).Scan(&invalid).Error; err != nil {
				return fmt.Errorf("核对%s失败: %w", check.name, err)
			}
			if invalid != 0 {
				return fmt.Errorf("存在 %d 条非法%s记录，拒绝启用资金约束", invalid, check.name)
			}
		}

		statements := []string{
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_lottery_bet_financials' AND conrelid = 'lottery_bets'::regclass) THEN
					ALTER TABLE lottery_bets ADD CONSTRAINT chk_lottery_bet_financials CHECK (
						amount_cents > 0 AND odds > 0
						AND payout_cents >= 0 AND fly_cents >= 0 AND rebate_cents >= 0 AND agent_share_cents >= 0
						AND rebate_rate_snapshot BETWEEN 0 AND 100
						AND agent_share_rate_snapshot BETWEEN 0 AND 100
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE lottery_bets VALIDATE CONSTRAINT chk_lottery_bet_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_lottery_bet_status' AND conrelid = 'lottery_bets'::regclass) THEN
					ALTER TABLE lottery_bets ADD CONSTRAINT chk_lottery_bet_status CHECK (
						status IN ('pending', 'won', 'lost', 'cancelled')
						AND reconciliation_status IN ('normal', 'abnormal', 'resolved')
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE lottery_bets VALIDATE CONSTRAINT chk_lottery_bet_status`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_lottery_bet_user' AND conrelid = 'lottery_bets'::regclass) THEN
					ALTER TABLE lottery_bets ADD CONSTRAINT fk_lottery_bet_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE lottery_bets VALIDATE CONSTRAINT fk_lottery_bet_user`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_user_application_financials' AND conrelid = 'user_applications'::regclass) THEN
					ALTER TABLE user_applications ADD CONSTRAINT chk_user_application_financials CHECK (
						((request_type IN ('credit', 'debit') AND requested_cents > 0)
						 OR (request_type IN ('agent', 'join') AND requested_cents = 0))
						AND received_cents >= 0
						AND status IN ('pending', 'approved', 'rejected')
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE user_applications VALIDATE CONSTRAINT chk_user_application_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_user_application_user' AND conrelid = 'user_applications'::regclass) THEN
					ALTER TABLE user_applications ADD CONSTRAINT fk_user_application_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE user_applications VALIDATE CONSTRAINT fk_user_application_user`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_activity_participation_financials' AND conrelid = 'activity_participations'::regclass) THEN
					ALTER TABLE activity_participations ADD CONSTRAINT chk_activity_participation_financials
					CHECK (reward_cents >= 0 AND streak >= 0) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE activity_participations VALIDATE CONSTRAINT chk_activity_participation_financials`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_activity_participation_user' AND conrelid = 'activity_participations'::regclass) THEN
					ALTER TABLE activity_participations ADD CONSTRAINT fk_activity_participation_user
					FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE activity_participations VALIDATE CONSTRAINT fk_activity_participation_user`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_activity_participation_activity' AND conrelid = 'activity_participations'::regclass) THEN
					ALTER TABLE activity_participations ADD CONSTRAINT fk_activity_participation_activity
					FOREIGN KEY (activity_id) REFERENCES ops_activities (id) ON DELETE RESTRICT NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE activity_participations VALIDATE CONSTRAINT fk_activity_participation_activity`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_ops_activity_financials' AND conrelid = 'ops_activities'::regclass) THEN
					ALTER TABLE ops_activities ADD CONSTRAINT chk_ops_activity_financials CHECK (
						reward_cents >= 0 AND pool_total_cents >= 0 AND pool_remaining_cents >= 0
						AND pool_remaining_cents <= pool_total_cents AND participants >= 0
					) NOT VALID;
				END IF;
			END $$`,
			`ALTER TABLE ops_activities VALIDATE CONSTRAINT chk_ops_activity_financials`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("启用业务资金数据库约束失败: %w", err)
			}
		}
		return nil
	})
}

func prepareLegacySchema(db *gorm.DB) error {
	if err := db.Exec(`ALTER TABLE "user" ADD COLUMN IF NOT EXISTS login_scope varchar(80) NOT NULL DEFAULT 'platform'`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE "user"
		SET login_scope = CASE
			WHEN role = 'agent' AND parent_tenant_id IS NOT NULL THEN 'tenant:' || parent_tenant_id::text
			WHEN role = 'member' AND parent_agent_id IS NOT NULL THEN 'agent:' || parent_agent_id::text
			WHEN role = 'member' AND parent_tenant_id IS NOT NULL THEN 'tenant:' || parent_tenant_id::text
			ELSE 'platform'
		END
		WHERE login_scope IS NULL OR login_scope = '' OR login_scope = 'platform'
	`).Error; err != nil {
		return err
	}
	// Older schemas enforced one username across the entire platform. Account
	// identity is now scoped by tenant/agent, so remove only that legacy key.
	if err := db.Exec(`ALTER TABLE "user" DROP CONSTRAINT IF EXISTS uni_user_username`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP INDEX IF EXISTS idx_user_username`).Error; err != nil {
		return err
	}
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
	if err := installLegacyMemberPublicIDAllocator(db); err != nil {
		return err
	}
	if err := db.Exec(`
		ALTER TABLE "user"
		ALTER COLUMN public_id SET DEFAULT public.next_member_public_id(),
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

// installLegacyMemberPublicIDAllocator only keeps the explicit, development-
// only AutoMigrate bridge compatible with the versioned schema. Production
// installs the same allocator through migrations/202608280008_random_public_ids.sql.
func installLegacyMemberPublicIDAllocator(db *gorm.DB) error {
	return db.Exec(`
		CREATE OR REPLACE FUNCTION public.next_member_public_id()
		RETURNS bigint
		LANGUAGE plpgsql
		VOLATILE
		SET search_path = pg_catalog, public
		AS $$
		DECLARE
			candidate bigint;
		BEGIN
			PERFORM pg_advisory_xact_lock(24587624048118084);
			FOR attempt IN 1..256 LOOP
				candidate := 1000000 + floor(random() * 9000000)::bigint;
				IF NOT EXISTS (
					SELECT 1 FROM public."user" AS account
					WHERE account.public_id = candidate
				) THEN
					RETURN candidate;
				END IF;
			END LOOP;
			RAISE EXCEPTION 'unable to allocate a unique seven-digit member public ID after 256 attempts'
				USING ERRCODE = '54000';
		END;
		$$
	`).Error
}

func hardenLoginIdentity(db *gorm.DB) error {
	if err := db.Exec(`
		UPDATE "user"
		SET login_scope = CASE
			WHEN role = 'agent' AND parent_tenant_id IS NOT NULL THEN 'tenant:' || parent_tenant_id::text
			WHEN role = 'member' AND parent_agent_id IS NOT NULL THEN 'agent:' || parent_agent_id::text
			ELSE 'platform'
		END
		WHERE login_scope IS NULL OR login_scope = ''
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_login_identity
		ON "user" (login_scope, LOWER(username))
		WHERE deleted_at IS NULL
	`).Error
}
