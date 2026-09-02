package services

import (
	"backend/data/models/settings"
	"backend/data/models/special"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PlatformWorkspaceCode      = "00000"
	FirstTenantCode            = 1
	defaultWorkspaceRobotCount = 10
	publicRoomCodeLockID       = int64(0x575A524F4F4D)
)

var defaultRobotAvatars = []string{
	"/images/avatars/avatar-anime-00.png", "/images/avatars/avatar-anime-01.png",
	"/images/avatars/avatar-anime-02.png", "/images/avatars/avatar-anime-03.png",
	"/images/avatars/avatar-anime-04.png", "/images/avatars/avatar-anime-05.png",
	"/images/avatars/avatar-anime-06.png", "/images/avatars/avatar-anime-07.png",
	"/images/avatars/avatar-1.jpg", "/images/avatars/avatar-2.jpg",
	"/images/avatars/avatar-3.jpg", "/images/avatars/avatar-4.jpg",
}

// EnsureWorkspaceHierarchy is an additive, idempotent migration from the
// former parent_agent_id based layout. It never guesses ambiguous historic
// ownership: rows that cannot be matched to a known scope remain workspace 0
// and are therefore invisible to workspace-scoped endpoints.
func EnsureWorkspaceHierarchy(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	// The historic settings service owns the default values. Ensure the template
	// exists before assigning it to platform workspace 00000.
	if _, err := NewSettingsAdminService(db).Get(); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// Workspace creation is infrequent. Serializing the allocator keeps public
		// room numbers unique even when two tenant accounts are created together.
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		// Tables and concurrency indexes are installed by the versioned SQL
		// migrations before bootstrap. Startup only materializes business data.
		platform, err := ensurePlatformWorkspace(tx)
		if err != nil {
			return err
		}
		if err := ensureTenantWorkspaces(tx, platform); err != nil {
			return err
		}
		if err := ensureAgentWorkspaces(tx, platform); err != nil {
			return err
		}
		if err := backfillWorkspaceUsers(tx, platform); err != nil {
			return err
		}
		if err := ensureWorkspaceSettings(tx, platform); err != nil {
			return err
		}
		if err := applyWorkspaceDataMigrations(tx); err != nil {
			return err
		}
		if err := migrateRobotProfiles(tx); err != nil {
			return err
		}
		if err := hardenExistingRobotCredentials(tx); err != nil {
			return err
		}
		if err := ensureWorkspaceRobotAccounts(tx); err != nil {
			return err
		}
		return backfillWorkspaceBusinessRows(tx, platform.ID)
	})
}

// lockPublicRoomCodeRegistry serializes every public-room identity write.
// Public room codes are mirrored in workspaces, the legacy agent account and
// system settings, so checking one table without this shared lock can leave an
// account half-created when a concurrent tenant/agent claims the same code.
func lockPublicRoomCodeRegistry(db *gorm.DB) error {
	return db.Exec(`SELECT pg_advisory_xact_lock(?)`, publicRoomCodeLockID).Error
}

// applyWorkspaceDataMigrations contains one-shot data migrations whose result
// must not be re-applied on every boot. Repeatedly forcing join review on would
// otherwise make the room owner's setting impossible to turn off.
func applyWorkspaceDataMigrations(tx *gorm.DB) error {
	const migrationKey = "20260827_enable_join_review_for_formal_rooms"
	var applied int64
	if err := tx.Raw(`SELECT COUNT(*) FROM workspace_migration_markers WHERE key = ?`, migrationKey).Scan(&applied).Error; err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	if err := tx.Exec(`UPDATE system_settings AS cfg
		SET require_join_review = TRUE, updated_at = CURRENT_TIMESTAMP
		FROM workspaces AS ws
		WHERE cfg.workspace_id = ws.id AND ws.type IN ('tenant', 'agent')`).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO workspace_migration_markers (key) VALUES (?)`, migrationKey).Error
}

func ensurePlatformWorkspace(tx *gorm.DB) (workspacemodel.Workspace, error) {
	var admin user.User
	if err := tx.Where("role = ?", "admin").Order("user_id ASC").First(&admin).Error; err != nil {
		return workspacemodel.Workspace{}, fmt.Errorf("创建平台工作区前未找到管理员: %w", err)
	}
	var existingCode workspacemodel.Workspace
	if err := tx.Where("code = ? AND type <> ?", PlatformWorkspaceCode, workspacemodel.TypePlatform).First(&existingCode).Error; err == nil {
		return workspacemodel.Workspace{}, fmt.Errorf("保留工作区编号 %s 已被其他账户占用", PlatformWorkspaceCode)
	} else if err != gorm.ErrRecordNotFound {
		return workspacemodel.Workspace{}, err
	}
	var workspace workspacemodel.Workspace
	err := tx.Where("type = ?", workspacemodel.TypePlatform).First(&workspace).Error
	if err == gorm.ErrRecordNotFound {
		workspace = workspacemodel.Workspace{
			Code: PlatformWorkspaceCode, RoomCode: "", Type: workspacemodel.TypePlatform,
			OwnerUserID: admin.UserID, Scope: "lobby", Name: "王者平台", Status: 1,
		}
		if err = tx.Create(&workspace).Error; err != nil {
			return workspacemodel.Workspace{}, err
		}
	} else if err != nil {
		return workspacemodel.Workspace{}, err
	} else {
		if err = tx.Model(&workspace).Updates(map[string]any{
			"code": PlatformWorkspaceCode, "owner_user_id": admin.UserID,
			"scope": "lobby", "status": 1,
		}).Error; err != nil {
			return workspacemodel.Workspace{}, err
		}
	}
	return workspace, nil
}

func ensureTenantWorkspaces(tx *gorm.DB, platform workspacemodel.Workspace) error {
	var tenants []user.User
	if err := tx.Where("role = ?", "tenant").Order("user_id ASC").Find(&tenants).Error; err != nil {
		return err
	}
	for index, account := range tenants {
		code := fmt.Sprintf("%05d", FirstTenantCode+index)
		if code == PlatformWorkspaceCode {
			return fmt.Errorf("租户工作区编号不能使用平台保留编号")
		}
		var occupied workspacemodel.Workspace
		if err := tx.Where("code = ? AND owner_user_id <> ?", code, account.UserID).First(&occupied).Error; err == nil {
			return fmt.Errorf("租户工作区编号 %s 已被占用", code)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		workspace := workspacemodel.Workspace{}
		err := tx.Where("owner_user_id = ?", account.UserID).First(&workspace).Error
		name := defaultString(strings.TrimSpace(account.Nickname), account.Username)
		if err == gorm.ErrRecordNotFound {
			roomCode, allocationErr := allocatePublicRoomCode(tx)
			if allocationErr != nil {
				return allocationErr
			}
			workspace = workspacemodel.Workspace{
				Code: code, RoomCode: roomCode, Type: workspacemodel.TypeTenant, OwnerUserID: account.UserID,
				ParentID: &platform.ID, Scope: "tenant:" + strconv.FormatUint(account.UserID, 10),
				Name: name, Status: account.Status,
			}
			if err = tx.Create(&workspace).Error; err != nil {
				return err
			}
			if err = EnsureWorkspaceGameDefaults(tx, workspace); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			roomCode := strings.TrimSpace(workspace.RoomCode)
			if roomCode == "" {
				roomCode, err = allocatePublicRoomCode(tx)
				if err != nil {
					return err
				}
			}
			if err = tx.Model(&workspace).Updates(map[string]any{
				"code": code, "room_code": roomCode, "type": workspacemodel.TypeTenant, "parent_id": platform.ID,
				"scope": "tenant:" + strconv.FormatUint(account.UserID, 10), "status": account.Status,
			}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func allocatePublicRoomCode(tx *gorm.DB) (string, error) {
	var workspaceCodes []string
	if err := tx.Model(&workspacemodel.Workspace{}).Where("room_code <> ''").Pluck("room_code", &workspaceCodes).Error; err != nil {
		return "", err
	}
	var agentCodes []string
	if err := tx.Model(&user.User{}).Where("agent_room_code <> ''").Pluck("agent_room_code", &agentCodes).Error; err != nil {
		return "", err
	}
	var reservedCodes []string
	if err := tx.Model(&special.NumberResource{}).Pluck("number", &reservedCodes).Error; err != nil {
		return "", err
	}
	occupied := make(map[string]struct{}, len(workspaceCodes)+len(agentCodes)+len(reservedCodes))
	for _, code := range append(append(workspaceCodes, agentCodes...), reservedCodes...) {
		occupied[strings.TrimSpace(code)] = struct{}{}
	}
	for candidate := 100001; candidate <= 999999; candidate++ {
		code := strconv.Itoa(candidate)
		if _, exists := occupied[code]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("没有可用的公开房间号")
}

func ensureAgentWorkspaces(tx *gorm.DB, platform workspacemodel.Workspace) error {
	var agents []user.User
	if err := tx.Where("role = ?", "agent").Order("user_id ASC").Find(&agents).Error; err != nil {
		return err
	}
	for _, account := range agents {
		code := strings.TrimSpace(account.AgentRoomCode)
		if validateAgentRoomCode(code) != nil {
			// Historic agents without a usable public number are retained, but
			// receive a valid public room number before a workspace is created.
			// This never changes any historic business row: those rows are scoped
			// by workspace_id rather than by this mutable display identity.
			var err error
			code, err = allocatePublicRoomCode(tx)
			if err != nil {
				return err
			}
			if err = tx.Model(&user.User{}).Where("user_id = ? AND role = ?", account.UserID, "agent").Update("agent_room_code", code).Error; err != nil {
				return err
			}
			account.AgentRoomCode = code
		}
		if code == PlatformWorkspaceCode {
			return fmt.Errorf("代理 %s 使用了平台/租户保留编号 %s", account.Username, code)
		}
		parentID := platform.ID
		if account.ParentTenantID != nil {
			var tenantWorkspace workspacemodel.Workspace
			if err := tx.Where("owner_user_id = ? AND type = ?", *account.ParentTenantID, workspacemodel.TypeTenant).First(&tenantWorkspace).Error; err == nil {
				parentID = tenantWorkspace.ID
			}
		}
		name := defaultString(strings.TrimSpace(account.AgentRoomName), defaultString(strings.TrimSpace(account.Nickname), account.Username))
		workspace := workspacemodel.Workspace{}
		err := tx.Where("owner_user_id = ?", account.UserID).First(&workspace).Error
		if err == gorm.ErrRecordNotFound {
			workspace = workspacemodel.Workspace{
				Code: code, RoomCode: code, Type: workspacemodel.TypeAgent, OwnerUserID: account.UserID,
				ParentID: &parentID, Scope: "agent:" + strconv.FormatUint(account.UserID, 10),
				Name: name, Logo: account.AgentRoomLogo, Status: account.Status,
			}
			if err = tx.Create(&workspace).Error; err != nil {
				return err
			}
			if err = EnsureWorkspaceGameDefaults(tx, workspace); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// Once a workspace exists its public identity is authoritative. Legacy
			// agent columns are compatibility shadows and must never overwrite a
			// room name/logo configured through workspace settings on a later boot.
			updates := map[string]any{
				"type": workspacemodel.TypeAgent, "parent_id": parentID,
				"scope": "agent:" + strconv.FormatUint(account.UserID, 10), "status": account.Status,
			}
			if strings.TrimSpace(workspace.Code) == "" {
				updates["code"] = code
				workspace.Code = code
			}
			if strings.TrimSpace(workspace.RoomCode) == "" {
				updates["room_code"] = code
				workspace.RoomCode = code
			}
			if strings.TrimSpace(workspace.Name) == "" {
				updates["name"] = name
				workspace.Name = name
			}
			if err = tx.Model(&workspace).Updates(updates).Error; err != nil {
				return err
			}
			workspace.Type, workspace.ParentID, workspace.Scope, workspace.Status = workspacemodel.TypeAgent, &parentID, "agent:"+strconv.FormatUint(account.UserID, 10), account.Status
			if err = syncLegacyAgentRoomIdentity(tx, workspace); err != nil {
				return err
			}
		}
	}
	return nil
}

func backfillWorkspaceUsers(tx *gorm.DB, platform workspacemodel.Workspace) error {
	var accounts []user.User
	if err := tx.Order("user_id ASC").Find(&accounts).Error; err != nil {
		return err
	}
	ownerWorkspaces := map[uint64]workspacemodel.Workspace{}
	var workspaces []workspacemodel.Workspace
	if err := tx.Find(&workspaces).Error; err != nil {
		return err
	}
	for _, item := range workspaces {
		ownerWorkspaces[item.OwnerUserID] = item
	}
	for _, account := range accounts {
		workspaceID := platform.ID
		loginScope := platformLoginScope
		if own, ok := ownerWorkspaces[account.UserID]; ok && (account.Role == "admin" || account.Role == "tenant" || account.Role == "agent") {
			workspaceID = own.ID
			if account.Role == "agent" && account.ParentTenantID != nil {
				loginScope = "tenant:" + strconv.FormatUint(*account.ParentTenantID, 10)
			}
		} else if account.ParentAgentID != nil {
			if parent, ok := ownerWorkspaces[*account.ParentAgentID]; ok {
				workspaceID = parent.ID
				loginScope = "agent:" + strconv.FormatUint(*account.ParentAgentID, 10)
			}
		} else if account.ParentTenantID != nil {
			if parent, ok := ownerWorkspaces[*account.ParentTenantID]; ok {
				workspaceID = parent.ID
				loginScope = "tenant:" + strconv.FormatUint(*account.ParentTenantID, 10)
			}
		}
		if err := tx.Model(&user.User{}).Where("user_id = ?", account.UserID).Updates(map[string]any{"workspace_id": workspaceID, "login_scope": loginScope}).Error; err != nil {
			return err
		}
		// A user may have accumulated several historic room memberships. Keep
		// those rows for audit, but only the account's current room may be active.
		if err := tx.Model(&workspacemodel.Membership{}).
			Where("user_id = ? AND workspace_id <> ? AND status = ?", account.UserID, workspaceID, 1).
			Updates(map[string]any{"status": 0, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		membership := workspacemodel.Membership{WorkspaceID: workspaceID, UserID: account.UserID, Role: account.Role, Status: account.Status}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{"role": account.Role, "status": account.Status, "updated_at": time.Now()}),
		}).Create(&membership).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureWorkspaceSettings(tx *gorm.DB, platform workspacemodel.Workspace) error {
	var template settings.SystemConfig
	if err := tx.Order("id ASC").First(&template).Error; err != nil {
		return err
	}
	if template.WorkspaceID == 0 {
		if err := tx.Model(&template).Updates(map[string]any{
			"workspace_id": platform.ID, "room_code": PlatformWorkspaceCode,
		}).Error; err != nil {
			return err
		}
		template.WorkspaceID = platform.ID
		template.RoomCode = PlatformWorkspaceCode
	}
	roomGameSettings, err := initialRoomGameSettingsFromPlatform(tx)
	if err != nil {
		return err
	}
	var workspaces []workspacemodel.Workspace
	if err := tx.Order("id ASC").Find(&workspaces).Error; err != nil {
		return err
	}
	var nextSettingsID uint
	if err := tx.Model(&settings.SystemConfig{}).Select("COALESCE(MAX(id), 0) + 1").Scan(&nextSettingsID).Error; err != nil {
		return err
	}
	for _, workspace := range workspaces {
		// The scheduler is independent from the visual/system settings row. Always
		// ensure it exists, including for the historic platform singleton.
		robotSetting := workspacemodel.RobotSetting{
			WorkspaceID: workspace.ID, Enabled: false, IntervalSecs: 60, BetsPerCycle: 1,
			DailyBetLimit: 200, MaxPendingBets: 50,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&robotSetting).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&settings.SystemConfig{}).Where("workspace_id = ?", workspace.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			// Existing settings are authoritative. Reapplying the default on every
			// boot would silently undo an owner's deliberate decision to disable
			// entry review. Room identity is still synchronized from its owner record.
			if err := tx.Model(&settings.SystemConfig{}).Where("workspace_id = ?", workspace.ID).Updates(map[string]any{
				"room_code": workspace.RoomCode, "room_name": workspace.Name, "room_logo": workspace.Logo,
			}).Error; err != nil {
				return err
			}
			continue
		}
		clone := template
		// Historic installations inserted the singleton as ID=1 without advancing
		// PostgreSQL's sequence. Allocate explicitly from MAX(id) so the first
		// workspace clone cannot collide with that row.
		clone.ID = nextSettingsID
		nextSettingsID++
		clone.WorkspaceID = workspace.ID
		clone.RoomCode = workspace.RoomCode
		clone.RoomName = workspace.Name
		clone.RoomLogo = workspace.Logo
		clone.RequireJoinReview = true
		if workspace.Type != workspacemodel.TypePlatform {
			// New rooms start with structural defaults but no inherited content.
			clone.RoomNotice = ""
			clone.AnnouncementsJSON = "[]"
			clone.QuickRepliesJSON = "[]"
			clone.GameSettingsJSON = roomGameSettings
			clone.RebateSettingsJSON = `{"enabled":false,"rate_percent":0,"min_turnover":0,"settle_mode":"daily","auto_credit":false}`
		}
		clone.CreatedAt = time.Time{}
		clone.UpdatedAt = time.Time{}
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateRobotProfiles(tx *gorm.DB) error {
	var accounts []user.User
	// Legacy robots were created with both this reserved prefix and marker.
	// Requiring both prevents an ordinary historic member from being converted
	// into a robot merely because they happened to choose a similar username.
	if err := legacyRobotAccountsQuery(tx).Order("user_id ASC").Find(&accounts).Error; err != nil {
		return err
	}
	for index, account := range accounts {
		if account.WorkspaceID == 0 {
			continue
		}
		if err := ensureSeededBalance(tx, &account, 0, 0, "机器人账户初始化"); err != nil {
			return err
		}
		profile := workspacemodel.RobotProfile{
			WorkspaceID: account.WorkspaceID, UserID: account.UserID,
			Avatar: defaultRobotAvatars[index%len(defaultRobotAvatars)], Enabled: account.Status == 1,
			ActiveStart: account.RobotActiveStart, ActiveEnd: account.RobotActiveEnd,
			MinBetCents: account.RobotMinBetCents, MaxBetCents: account.RobotMaxBetCents,
		}
		if profile.MinBetCents <= 0 {
			profile.MinBetCents = 100
		}
		if profile.MaxBetCents < profile.MinBetCents {
			profile.MaxBetCents = 5000
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"workspace_id", "enabled", "active_start", "active_end", "min_bet_cents", "max_bet_cents", "updated_at"}),
		}).Create(&profile).Error; err != nil {
			return err
		}
		var stored workspacemodel.RobotProfile
		if err := tx.Where("user_id = ?", account.UserID).First(&stored).Error; err != nil {
			return err
		}
		var gameIDs []string
		if json.Unmarshal([]byte(strings.TrimSpace(account.RobotGameIDsJSON)), &gameIDs) != nil {
			continue
		}
		for _, gameID := range gameIDs {
			gameID = strings.TrimSpace(gameID)
			if gameID == "" {
				continue
			}
			assignment := workspacemodel.RobotGame{RobotID: stored.ID, GameID: gameID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&assignment).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyRobotAccountsQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&user.User{}).
		Where("role = ? AND workspace_id > 0 AND remark = ? AND LOWER(username) LIKE ?", "member", roomActivityRemark, "room_activity\\_%")
}

const robotCredentialHardeningMarker = "20260828_harden_robot_credentials"

// hardenExistingRobotCredentials rotates legacy deterministic credentials
// exactly once. The generated plaintext is never persisted or returned, and
// updating password also advances auth_version through the database trigger.
func hardenExistingRobotCredentials(tx *gorm.DB) error {
	var applied int64
	if err := tx.Raw(`SELECT COUNT(*) FROM workspace_migration_markers WHERE key = ?`, robotCredentialHardeningMarker).Scan(&applied).Error; err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	var accountIDs []uint64
	if err := tx.Model(&user.User{}).Select(`"user".user_id`).
		Joins(`JOIN workspace_robot_profiles AS profile ON profile.user_id = "user".user_id`).
		Pluck(`"user".user_id`, &accountIDs).Error; err != nil {
		return err
	}
	for _, accountID := range accountIDs {
		hash, err := newRobotPasswordHash()
		if err != nil {
			return err
		}
		if err := tx.Model(&user.User{}).Where("user_id = ?", accountID).Update("password", hash).Error; err != nil {
			return err
		}
	}
	return tx.Exec(`INSERT INTO workspace_migration_markers (key) VALUES (?)`, robotCredentialHardeningMarker).Error
}

func newRobotPasswordHash() (string, error) {
	secret := make([]byte, 32)
	if _, err := cryptorand.Read(secret); err != nil {
		return "", fmt.Errorf("生成机器人随机凭证失败: %w", err)
	}
	return utils.HashPassword(base64.RawURLEncoding.EncodeToString(secret))
}

// ensureWorkspaceRobotAccounts provisions independent ordinary-member
// identities for every managed workspace. Existing workspaces are topped up
// to ten; an operator's existing nicknames, balances and enabled switches are preserved.
// The scheduler remains disabled by default, and no account is ever borrowed
// from another workspace.
func ensureWorkspaceRobotAccounts(tx *gorm.DB) error {
	var rooms []workspacemodel.Workspace
	if err := managedRobotWorkspacesQuery(tx).Order("id ASC").Find(&rooms).Error; err != nil {
		return err
	}
	for roomIndex, room := range rooms {
		var existing int64
		if err := validWorkspaceRobotProfilesQuery(tx, room.ID).Count(&existing).Error; err != nil {
			return err
		}
		parentTenantID := (*uint64)(nil)
		parentAgentID := (*uint64)(nil)
		if room.Type == workspacemodel.TypeTenant {
			owner := room.OwnerUserID
			parentTenantID = &owner
		} else if room.Type == workspacemodel.TypeAgent {
			owner := room.OwnerUserID
			parentAgentID = &owner
			var agent user.User
			if err := tx.Select("parent_tenant_id").First(&agent, owner).Error; err != nil {
				return err
			}
			parentTenantID = agent.ParentTenantID
		}
		for slot := 0; existing < defaultWorkspaceRobotCount; slot++ {
			username := fmt.Sprintf("room_robot_%d_%02d", room.ID, slot+1)
			aliasIndex := (roomIndex*defaultWorkspaceRobotCount + slot) % len(roomActivityAliases)
			var account user.User
			var profile workspacemodel.RobotProfile
			created := false
			accountErr := tx.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", username).First(&account).Error
			if accountErr == gorm.ErrRecordNotFound {
				hash, err := newRobotPasswordHash()
				if err != nil {
					return err
				}
				account = user.User{
					Username: username, LoginScope: room.Scope, Password: hash,
					Nickname: roomActivityAliases[aliasIndex], Role: "member", Remark: roomActivityRemark,
					Status: 1, BalanceCents: 1_000_000_000, WorkspaceID: room.ID,
					ParentTenantID: parentTenantID, ParentAgentID: parentAgentID,
					RobotMinBetCents: 100, RobotMaxBetCents: 5000,
				}
				if err := tx.Create(&account).Error; err != nil {
					return err
				}
				created = true
			} else if accountErr != nil {
				return accountErr
			} else {
				profileErr := tx.Where("user_id = ?", account.UserID).First(&profile).Error
				if profileErr == gorm.ErrRecordNotFound {
					// A pre-existing human account must never be silently promoted
					// to robot. Skip the occupied reserved slot and allocate the next.
					continue
				}
				if profileErr != nil {
					return profileErr
				}
				if account.WorkspaceID != room.ID || account.Role != "member" || profile.WorkspaceID != room.ID {
					return fmt.Errorf("机器人账号 %s 的房间归属异常", username)
				}
				// Existing status and profile switches are operator-owned. A restart
				// must never silently reactivate a robot that was deliberately paused.
				if err := tx.Model(&account).Updates(map[string]any{
					"login_scope": room.Scope, "parent_tenant_id": parentTenantID, "parent_agent_id": parentAgentID,
					"remark": roomActivityRemark,
				}).Error; err != nil {
					return err
				}
			}
			if err := ensureSeededBalance(tx, &account, 0, 0, "机器人账户初始化"); err != nil {
				return err
			}
			if created {
				profile = workspacemodel.RobotProfile{
					WorkspaceID: room.ID, UserID: account.UserID,
					Avatar: defaultRobotAvatars[(roomIndex*defaultWorkspaceRobotCount+slot)%len(defaultRobotAvatars)], Enabled: account.Status == 1,
					MinBetCents: 100, MaxBetCents: 5000,
				}
				if err := tx.Create(&profile).Error; err != nil {
					return err
				}
				existing++
			}
			membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: account.UserID, Role: "member", Status: account.Status}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{"role": "member", "status": account.Status, "updated_at": time.Now().UTC()}),
			}).Create(&membership).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func managedRobotWorkspacesQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&workspacemodel.Workspace{}).
		Where("status = ? AND type IN ?", 1, []string{workspacemodel.TypePlatform, workspacemodel.TypeTenant, workspacemodel.TypeAgent})
}

func validWorkspaceRobotProfilesQuery(db *gorm.DB, workspaceID uint64) *gorm.DB {
	return db.Table("workspace_robot_profiles AS profile").
		Joins(`JOIN "user" AS account ON account.user_id = profile.user_id`).
		Where("profile.workspace_id = ? AND account.workspace_id = ? AND account.role = ? AND account.deleted_at IS NULL", workspaceID, workspaceID, "member")
}

func backfillWorkspaceBusinessRows(tx *gorm.DB, platformID uint64) error {
	statements := []string{
		`UPDATE lottery_bets AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.room_scope = ws.scope`,
		`UPDATE member_chat_messages AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.room_scope = ws.scope`,
		`UPDATE user_applications AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.room_scope = ws.scope`,
		`UPDATE chat_red_packets AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.room_scope = ws.scope`,
		`UPDATE chat_red_packet_claims AS claim SET workspace_id = packet.workspace_id FROM chat_red_packets AS packet WHERE claim.workspace_id = 0 AND claim.packet_id = packet.id AND packet.workspace_id > 0`,
		`UPDATE member_notifications AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.room_scope = ws.scope`,
		`UPDATE activity_participations AS row SET workspace_id = activity.workspace_id FROM ops_activities AS activity WHERE row.workspace_id = 0 AND row.activity_id = activity.id AND activity.workspace_id > 0`,
		`UPDATE user_balance_transactions AS row SET workspace_id = account.workspace_id FROM "user" AS account WHERE row.workspace_id = 0 AND row.user_id = account.user_id AND account.workspace_id > 0 AND row.type IN ('opening_balance', 'system_topup', 'seed_reconciliation')`,
		`UPDATE room_game_settings AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.agent_id = ws.owner_user_id`,
		`UPDATE room_play_odds AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.agent_id = ws.owner_user_id`,
		`UPDATE agent_profit_share_records AS row SET workspace_id = ws.id FROM workspaces AS ws WHERE row.workspace_id = 0 AND row.agent_id = ws.owner_user_id`,
		fmt.Sprintf(`UPDATE admin_notifications SET workspace_id = %d WHERE workspace_id = 0`, platformID),
		fmt.Sprintf(`UPDATE wallet_payment_channels SET workspace_id = %d WHERE workspace_id = 0`, platformID),
		fmt.Sprintf(`UPDATE ops_activities SET workspace_id = %d WHERE workspace_id = 0`, platformID),
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func WorkspaceForAccount(db *gorm.DB, account user.User) (workspacemodel.Workspace, error) {
	var workspace workspacemodel.Workspace
	if account.WorkspaceID > 0 {
		if err := db.First(&workspace, account.WorkspaceID).Error; err == nil {
			return workspace, nil
		}
	}
	if err := db.Where("owner_user_id = ?", account.UserID).First(&workspace).Error; err == nil {
		return workspace, nil
	}
	return workspacemodel.Workspace{}, gorm.ErrRecordNotFound
}

func WorkspaceByScope(db *gorm.DB, scope string) (workspacemodel.Workspace, error) {
	var workspace workspacemodel.Workspace
	err := db.Where("scope = ?", strings.TrimSpace(scope)).First(&workspace).Error
	return workspace, err
}

func WorkspaceByRoomCode(db *gorm.DB, roomCode string) (workspacemodel.Workspace, error) {
	var workspace workspacemodel.Workspace
	roomCode = strings.TrimSpace(roomCode)
	if err := validateAgentRoomCode(roomCode); err != nil {
		return workspace, err
	}
	err := db.Where("room_code = ? AND type IN ? AND status = ?", roomCode, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1).
		First(&workspace).Error
	return workspace, err
}

// ActivateWorkspaceMembership changes only the member's current workspace.
// Historic business rows retain their immutable workspace_id and therefore
// never move when a member enters another room.
func ActivateWorkspaceMembership(tx *gorm.DB, account *user.User, target workspacemodel.Workspace) error {
	if account == nil || account.UserID == 0 || target.ID == 0 {
		return fmt.Errorf("invalid workspace membership")
	}
	if err := tx.Model(&workspacemodel.Membership{}).
		Where("user_id = ? AND workspace_id <> ? AND status = ?", account.UserID, target.ID, 1).
		Update("status", 0).Error; err != nil {
		return err
	}
	membership := workspacemodel.Membership{WorkspaceID: target.ID, UserID: account.UserID, Role: "member", Status: 1}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"role": "member", "status": 1, "updated_at": time.Now().UTC()}),
	}).Create(&membership).Error; err != nil {
		return err
	}

	updates := map[string]any{"workspace_id": target.ID, "login_scope": target.Scope}
	switch target.Type {
	case workspacemodel.TypeAgent:
		owner := target.OwnerUserID
		updates["parent_agent_id"] = owner
		var agent user.User
		if err := tx.Select("parent_tenant_id").First(&agent, owner).Error; err != nil {
			return err
		}
		updates["parent_tenant_id"] = agent.ParentTenantID
	case workspacemodel.TypeTenant:
		owner := target.OwnerUserID
		updates["parent_agent_id"] = nil
		updates["parent_tenant_id"] = owner
	default:
		updates["parent_agent_id"] = nil
		updates["parent_tenant_id"] = nil
	}
	if err := ensureUsernameInScope(tx, target.Scope, account.Username, account.UserID); err != nil {
		return err
	}
	if err := tx.Model(account).Updates(updates).Error; err != nil {
		return err
	}
	account.WorkspaceID = target.ID
	return nil
}

func TenantWorkspaceCodes(items []workspacemodel.Workspace) []string {
	codes := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type == workspacemodel.TypeTenant {
			codes = append(codes, item.Code)
		}
	}
	sort.Strings(codes)
	return codes
}
