package services

import (
	"backend/constants"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
)

// BootstrapOptions contains only process-mode policy. Credentials and other
// secrets are never accepted by the normal server bootstrap path.
type BootstrapOptions struct {
	Mode string
}

type bootstrapStep string

const (
	bootstrapAdmin             bootstrapStep = "admin"
	bootstrapLotteryCatalog    bootstrapStep = "lottery_catalog"
	bootstrapLotteryDebug      bootstrapStep = "lottery_debug_history"
	bootstrapExperienceAccount bootstrapStep = "experience_accounts"
	bootstrapWorkspaces        bootstrapStep = "workspaces"
	bootstrapBaseCatalogs      bootstrapStep = "base_catalogs"
	bootstrapDebugPlans        bootstrapStep = "debug_plans"
)

func bootstrapSteps(mode string) ([]bootstrapStep, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "debug":
		return []bootstrapStep{
			bootstrapAdmin, bootstrapLotteryDebug, bootstrapExperienceAccount,
			bootstrapWorkspaces, bootstrapBaseCatalogs,
		}, nil
	case "release", "test":
		return []bootstrapStep{
			bootstrapAdmin, bootstrapLotteryCatalog, bootstrapWorkspaces, bootstrapBaseCatalogs,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的启动模式 %q", mode)
	}
}

// Bootstrap is the single authoritative initialization entry point. Every
// step is additive and idempotent; release mode deliberately excludes local
// accounts, deterministic draw history and editorial plan fixtures.
func Bootstrap(db *gorm.DB, options BootstrapOptions) error {
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	steps, err := bootstrapSteps(mode)
	if err != nil {
		return err
	}
	for _, step := range steps {
		var stepErr error
		switch step {
		case bootstrapAdmin:
			stepErr = ensureBootstrapAdmin(db, mode)
		case bootstrapLotteryCatalog:
			stepErr = SeedLotteryCatalog(db, LotterySeedOptions{})
		case bootstrapLotteryDebug:
			stepErr = SeedLotteryCatalog(db, LotterySeedOptions{IncludeDeterministicHistory: true})
		case bootstrapExperienceAccount:
			stepErr = SeedExperienceMember(db)
		case bootstrapWorkspaces:
			stepErr = EnsureWorkspaceHierarchy(db)
		case bootstrapBaseCatalogs:
			stepErr = EnsureBaseCatalogs(db)
		}
		if stepErr != nil {
			return fmt.Errorf("启动初始化步骤 %s 失败: %w", step, stepErr)
		}
	}
	return nil
}

func ensureBootstrapAdmin(db *gorm.DB, mode string) error {
	if mode == "debug" {
		var existing user.User
		err := db.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", constants.DefaultAdminUsername).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hash, hashErr := utils.HashPassword(constants.DefaultAdminPassword)
			if hashErr != nil {
				return fmt.Errorf("%s: %w", constants.ErrCreateAdminPasswordFailed, hashErr)
			}
			account := user.User{
				Username: constants.DefaultAdminUsername, LoginScope: platformLoginScope,
				Password: hash, Nickname: constants.DefaultAdminNickname,
				Email: constants.DefaultAdminEmail, Role: "admin", Status: 1,
			}
			if createErr := db.Create(&account).Error; createErr != nil {
				return fmt.Errorf("%s: %w", constants.ErrCreateAdminUserFailed, createErr)
			}
		} else if err != nil {
			return err
		}
	}

	var admins []user.User
	if err := db.Where("role = ?", "admin").Order("user_id ASC").Find(&admins).Error; err != nil {
		return err
	}
	activeAdmins := 0
	for _, account := range admins {
		if account.Status == 1 {
			activeAdmins++
		}
	}
	if activeAdmins == 0 {
		if mode == "release" {
			return fmt.Errorf("release 模式未配置可用管理员，请先运行 go run ./cmd/bootstrap-admin --username <账号> --password-file <密码文件>")
		}
		return fmt.Errorf("未找到可用的平台管理员；若同名账号已存在，初始化不会强行改写其角色或状态")
	}
	if mode == "release" {
		for _, account := range admins {
			if utils.CheckPasswordHash(constants.DefaultAdminPassword, account.Password) {
				return fmt.Errorf("release 模式检测到默认管理员密码，请修改后再启动")
			}
		}
	}
	return nil
}

// EnsureBaseCatalogs eagerly materializes records that were historically
// created as a side effect of the first page read. That makes readiness checks
// and fresh-database behavior deterministic.
func EnsureBaseCatalogs(db *gorm.DB) error {
	if _, err := NewOddsAdminService(db).SyncAllGames(); err != nil {
		return fmt.Errorf("初始化赔率目录: %w", err)
	}
	if err := NewEntertainmentAdminService(db).EnsureDefaults(); err != nil {
		return fmt.Errorf("初始化娱乐目录: %w", err)
	}
	var workspaces []workspacemodel.Workspace
	if err := db.Where("status = ?", 1).Order("id ASC").Find(&workspaces).Error; err != nil {
		return err
	}
	activities := NewActivityAdminService(db)
	wallet := NewWalletAdminService(db)
	for _, workspace := range workspaces {
		if err := activities.EnsureDefaultsForWorkspace(workspace.ID); err != nil {
			return fmt.Errorf("初始化工作区 %d 活动目录: %w", workspace.ID, err)
		}
		if err := wallet.EnsureDefaultsForWorkspace(workspace.ID); err != nil {
			return fmt.Errorf("初始化工作区 %d 收款目录: %w", workspace.ID, err)
		}
	}
	return nil
}

type BootstrapAdminInput struct {
	Username string
	Password string
	Nickname string
	Email    string
}

// ValidateBootstrapAdminPassword enforces a production bootstrap policy which
// is intentionally stricter than ordinary member registration.
func ValidateBootstrapAdminPassword(username, password string) error {
	if utf8.RuneCountInString(password) < 14 {
		return fmt.Errorf("初始管理员密码至少 14 个字符")
	}
	lowerPassword := strings.ToLower(password)
	for _, forbidden := range []string{
		strings.ToLower(constants.DefaultAdminPassword), strings.ToLower(demoPassword),
		strings.ToLower(demoAgentPassword), strings.ToLower(demoTenantPassword),
		"password", "changeme", "change_me", "admin123",
	} {
		if lowerPassword == forbidden {
			return fmt.Errorf("不能使用默认、演示或常见弱密码")
		}
	}
	if normalizedUsername := strings.ToLower(strings.TrimSpace(username)); len(normalizedUsername) >= 3 && strings.Contains(lowerPassword, normalizedUsername) {
		return fmt.Errorf("密码不能包含管理员账号")
	}
	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, value := range password {
		switch {
		case unicode.IsUpper(value):
			hasUpper = true
		case unicode.IsLower(value):
			hasLower = true
		case unicode.IsDigit(value):
			hasDigit = true
		case unicode.IsPunct(value) || unicode.IsSymbol(value):
			hasSymbol = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSymbol {
		return fmt.Errorf("密码必须同时包含大写字母、小写字母、数字和符号")
	}
	return nil
}

// CreateBootstrapAdmin provisions the first administrator under a database
// advisory lock. It refuses to act after any live administrator exists, so the
// command cannot become a second unaudited account-creation backdoor.
func CreateBootstrapAdmin(db *gorm.DB, input BootstrapAdminInput) (*user.User, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	username := strings.TrimSpace(input.Username)
	if utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 50 {
		return nil, fmt.Errorf("管理员账号长度需在 3-50 个字符之间")
	}
	for _, value := range username {
		if !unicode.IsLetter(value) && !unicode.IsDigit(value) && value != '_' && value != '-' && value != '.' {
			return nil, fmt.Errorf("管理员账号只能包含字母、数字、下划线、中划线和点")
		}
	}
	if err := ValidateBootstrapAdminPassword(username, input.Password); err != nil {
		return nil, err
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	var created user.User
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(0x575A41444D494E)).Error; err != nil {
			return err
		}
		var adminCount int64
		if err := tx.Model(&user.User{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
			return err
		}
		if adminCount > 0 {
			return fmt.Errorf("已存在管理员，请使用后台的账号管理功能")
		}
		var usernameCount int64
		if err := tx.Model(&user.User{}).Where("LOWER(username) = LOWER(?)", username).Count(&usernameCount).Error; err != nil {
			return err
		}
		if usernameCount > 0 {
			return fmt.Errorf("账号 %s 已存在", username)
		}
		nickname := strings.TrimSpace(input.Nickname)
		if nickname == "" {
			nickname = "平台管理员"
		}
		created = user.User{
			Username: username, LoginScope: platformLoginScope, Password: hash,
			Nickname: nickname, Email: strings.TrimSpace(input.Email), Role: "admin", Status: 1,
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}
