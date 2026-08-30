package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestSiteAccountsConfig is accepted only by the explicit operator command.
// It is never read by server bootstrap or exposed through an HTTP endpoint.
type TestSiteAccountsConfig struct {
	Site     string                `json:"site"`
	Platform TestSiteCredential    `json:"platform"`
	Tenant   TestSiteRoomAccount   `json:"tenant"`
	Agent    TestSiteRoomAccount   `json:"agent"`
	Member   TestSiteMemberAccount `json:"member"`
}

type TestSiteCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TestSiteRoomAccount struct {
	TestSiteCredential
	RoomCode string `json:"room_code"`
	RoomName string `json:"room_name"`
}

type TestSiteMemberAccount struct {
	TestSiteCredential
	RoomCode string `json:"room_code"`
}

type TestSiteAccountResult struct {
	UserID      uint64 `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	RoomCode    string `json:"room_code,omitempty"`
}

type TestSiteAccountsResult struct {
	Platform TestSiteAccountResult `json:"platform"`
	Tenant   TestSiteAccountResult `json:"tenant"`
	Agent    TestSiteAccountResult `json:"agent"`
	Member   TestSiteAccountResult `json:"member"`
}

const testSiteAccountLock = int64(0x575A54455354)

func testSiteAccountMarker(site, role string) string {
	return "test-site-accounts:v1:" + site + ":" + role
}

// ValidateTestSiteAccountsConfig validates the entire input before opening a
// transaction. All roles use distinct strong credentials, even on test sites.
func ValidateTestSiteAccountsConfig(input TestSiteAccountsConfig) error {
	if input.Site != strings.ToLower(strings.TrimSpace(input.Site)) || len(input.Site) > 253 || !strings.Contains(input.Site, ".") {
		return fmt.Errorf("site 必须是小写站点域名，不含协议、路径或端口")
	}
	for _, label := range strings.Split(input.Site, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("site 域名格式不正确")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return fmt.Errorf("site 域名格式不正确")
			}
		}
	}
	credentials := []TestSiteCredential{input.Platform, input.Tenant.TestSiteCredential, input.Agent.TestSiteCredential, input.Member.TestSiteCredential}
	roles := []string{"platform", "tenant", "agent", "member"}
	usernames, passwords := map[string]bool{}, map[string]bool{}
	for i, credential := range credentials {
		if credential.Username != strings.TrimSpace(credential.Username) {
			return fmt.Errorf("%s 账号不能包含首尾空白", roles[i])
		}
		if err := validateHumanUsername(credential.Username); err != nil {
			return fmt.Errorf("%s 账号无效: %w", roles[i], err)
		}
		for _, char := range credential.Username {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' && char != '-' && char != '.' {
				return fmt.Errorf("%s 账号只能包含字母、数字、下划线、中划线和点", roles[i])
			}
		}
		key := strings.ToLower(credential.Username)
		if usernames[key] || passwords[credential.Password] {
			return fmt.Errorf("四个测试账号和密码必须分别独立，不能重复")
		}
		usernames[key], passwords[credential.Password] = true, true
		if err := utils.ValidatePassword(credential.Password); err != nil || strings.ContainsAny(credential.Password, "\r\n\x00") {
			return fmt.Errorf("%s 密码长度或格式无效", roles[i])
		}
		if err := ValidateBootstrapAdminPassword(credential.Username, credential.Password); err != nil {
			return fmt.Errorf("%s 密码不满足测试站安全要求: %w", roles[i], err)
		}
	}
	for _, room := range []TestSiteRoomAccount{input.Tenant, input.Agent} {
		if err := validateAgentRoomCode(room.RoomCode); err != nil {
			return err
		}
		if room.RoomName == "" || room.RoomName != normalizeAgentRoomName(room.RoomName) {
			return fmt.Errorf("必须填写规范的房间名称")
		}
		if err := validateAgentRoomName(room.RoomName); err != nil {
			return err
		}
	}
	if input.Tenant.RoomCode == input.Agent.RoomCode || input.Member.RoomCode != input.Agent.RoomCode {
		return fmt.Errorf("租户与代理房间号不能相同，会员必须归属配置的代理房间")
	}
	return nil
}

// ProvisionTestSiteAccounts is an explicit, additive operator action. It never
// updates a pre-existing password, balance, role or room choice. A complete
// repeat is read-only; conflicting or deleted identities fail atomically.
func ProvisionTestSiteAccounts(db *gorm.DB, input TestSiteAccountsConfig) (*TestSiteAccountsResult, error) {
	if err := ValidateTestSiteAccountsConfig(input); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	var result TestSiteAccountsResult
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", testSiteAccountLock).Error; err != nil {
			return err
		}
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		var platform workspacemodel.Workspace
		if err := tx.Where("type = ? AND status = ?", workspacemodel.TypePlatform, 1).First(&platform).Error; err != nil {
			return fmt.Errorf("请先完成正式管理员及默认目录初始化: %w", err)
		}
		var primaryAdmin user.User
		if err := tx.Where("user_id = ? AND role = ? AND status = ?", platform.OwnerUserID, "admin", 1).First(&primaryAdmin).Error; err != nil {
			return fmt.Errorf("平台原管理员不可用，拒绝创建测试账号")
		}
		var hierarchyAdmin user.User
		if err := tx.Select("user_id").Where("role = ?", "admin").Order("user_id ASC").First(&hierarchyAdmin).Error; err != nil {
			return err
		}
		// Normal tenant/agent creation reconciles the hierarchy to the earliest
		// administrator. Refuse unexpected ownership drift before invoking it.
		if hierarchyAdmin.UserID != platform.OwnerUserID {
			return fmt.Errorf("平台工作区所有者与默认层级不一致，请先由管理员核对；不会借测试初始化改写")
		}
		credentials := []TestSiteCredential{input.Platform, input.Tenant.TestSiteCredential, input.Agent.TestSiteCredential, input.Member.TestSiteCredential}
		roles := []string{"admin", "tenant", "agent", "member"}
		accounts := make([]user.User, len(roles))
		for i, credential := range credentials {
			var matches []user.User
			if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(username) = LOWER(?)", credential.Username).Find(&matches).Error; err != nil {
				return err
			}
			if len(matches) == 0 {
				continue
			}
			if len(matches) != 1 {
				return fmt.Errorf("%s 测试账号存在重复或历史身份冲突", roles[i])
			}
			account := matches[0]
			if account.DeletedAt.Valid || account.Role != roles[i] || account.Status != 1 || account.Remark != testSiteAccountMarker(input.Site, roles[i]) || !utils.CheckPasswordHash(credential.Password, account.Password) {
				return fmt.Errorf("%s 账号已存在但并非相同的受控测试账号；不会覆盖或重置", roles[i])
			}
			accounts[i] = account
		}
		rooms := make([]workspacemodel.Workspace, 2)
		for i, roomInput := range []TestSiteRoomAccount{input.Tenant, input.Agent} {
			account := accounts[i+1]
			if err := ensureRoomCodeAvailable(tx, roomInput.RoomCode, account.UserID); err != nil {
				return fmt.Errorf("%s 测试房间冲突: %w", roles[i+1], err)
			}
			if account.UserID != 0 {
				if err := tx.Where("owner_user_id = ?", account.UserID).First(&rooms[i]).Error; err != nil {
					return fmt.Errorf("既有测试账号的房间不存在，拒绝修复或覆盖")
				}
				room := rooms[i]
				if room.Type != roles[i+1] || room.Status != 1 || room.RoomCode != roomInput.RoomCode || room.Name != roomInput.RoomName || account.WorkspaceID != room.ID {
					return fmt.Errorf("%s 房间配置已变化，拒绝覆盖", roles[i+1])
				}
			}
		}
		if err := validateTestSiteExistingOwnership(accounts, rooms, platform); err != nil {
			return err
		}
		if accounts[3].UserID != 0 {
			var memberships int64
			if err := tx.Model(&workspacemodel.Membership{}).Where("user_id = ? AND workspace_id = ? AND role = ? AND status = ?", accounts[3].UserID, rooms[1].ID, "member", 1).Count(&memberships).Error; err != nil {
				return err
			}
			if memberships != 1 {
				return fmt.Errorf("既有测试会员的入房关系已变化，拒绝覆盖")
			}
			if err := tx.Model(&workspacemodel.Membership{}).Where("user_id = ? AND status = ?", accounts[3].UserID, 1).Count(&memberships).Error; err != nil {
				return err
			}
			if memberships != 1 {
				return fmt.Errorf("既有测试会员存在其他有效房间关系，拒绝覆盖")
			}
		}
		if accounts[0].UserID == 0 {
			hash, err := utils.HashPassword(input.Platform.Password)
			if err != nil {
				return err
			}
			accounts[0] = user.User{Username: input.Platform.Username, Password: hash, Role: "admin", Nickname: "测试管理员", LoginScope: platformLoginScope, WorkspaceID: platform.ID, Status: 1, Remark: testSiteAccountMarker(input.Site, "admin")}
			if err := tx.Create(&accounts[0]).Error; err != nil {
				return err
			}
		}
		if accounts[1].UserID == 0 {
			view, err := NewTenantAdminService(tx).Create(TenantPayload{Username: input.Tenant.Username, Password: input.Tenant.Password, Nickname: "测试租户", RoomCode: input.Tenant.RoomCode, RoomName: input.Tenant.RoomName, Status: 1, Remark: testSiteAccountMarker(input.Site, "tenant")})
			if err != nil {
				return err
			}
			if err := tx.First(&accounts[1], view.ID).Error; err != nil {
				return err
			}
		}
		if accounts[2].UserID == 0 {
			view, err := NewAgentAdminService(tx).CreateForTenant(accounts[1].UserID, CreateAgentInput{Username: input.Agent.Username, Password: input.Agent.Password, Nickname: "测试代理", RoomCode: input.Agent.RoomCode, RoomName: input.Agent.RoomName, Status: 1, Remark: testSiteAccountMarker(input.Site, "agent")})
			if err != nil {
				return err
			}
			if err := tx.First(&accounts[2], view.ID).Error; err != nil {
				return err
			}
		}
		for i := range rooms {
			if err := tx.Where("owner_user_id = ?", accounts[i+1].UserID).First(&rooms[i]).Error; err != nil {
				return err
			}
		}
		if accounts[3].UserID == 0 {
			hash, err := utils.HashPassword(input.Member.Password)
			if err != nil {
				return err
			}
			accounts[3] = user.User{Username: input.Member.Username, Password: hash, Role: "member", Nickname: "王者玩家", LoginScope: rooms[1].Scope, WorkspaceID: rooms[1].ID, ParentAgentID: &accounts[2].UserID, ParentTenantID: &accounts[1].UserID, Status: 1, Remark: testSiteAccountMarker(input.Site, "member")}
			if err := tx.Create(&accounts[3]).Error; err != nil {
				return err
			}
			if err := ActivateWorkspaceMembership(tx, &accounts[3], rooms[1]); err != nil {
				return err
			}
		}
		result = TestSiteAccountsResult{
			Platform: TestSiteAccountResult{UserID: accounts[0].UserID, WorkspaceID: platform.ID},
			Tenant:   TestSiteAccountResult{UserID: accounts[1].UserID, WorkspaceID: rooms[0].ID, RoomCode: rooms[0].RoomCode},
			Agent:    TestSiteAccountResult{UserID: accounts[2].UserID, WorkspaceID: rooms[1].ID, RoomCode: rooms[1].RoomCode},
			Member:   TestSiteAccountResult{UserID: accounts[3].UserID, WorkspaceID: rooms[1].ID, RoomCode: rooms[1].RoomCode},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func validateTestSiteExistingOwnership(accounts []user.User, rooms []workspacemodel.Workspace, platform workspacemodel.Workspace) error {
	for _, index := range []int{0, 1} {
		account := accounts[index]
		if account.UserID != 0 && (account.LoginScope != platformLoginScope || account.ParentAgentID != nil || account.ParentTenantID != nil || (index == 0 && account.WorkspaceID != platform.ID)) {
			return fmt.Errorf("平台或租户测试账号归属不匹配，拒绝覆盖")
		}
	}
	if rooms[0].ID != 0 && (rooms[0].ParentID == nil || *rooms[0].ParentID != platform.ID || rooms[0].Scope != tenantLoginScope(accounts[1].UserID)) {
		return fmt.Errorf("测试租户房间上级关系不匹配")
	}
	agent := accounts[2]
	if agent.UserID != 0 && (accounts[1].UserID == 0 || agent.ParentTenantID == nil || *agent.ParentTenantID != accounts[1].UserID || agent.ParentAgentID != nil || agent.LoginScope != tenantLoginScope(accounts[1].UserID) || agent.AgentRoomCode != rooms[1].RoomCode || agent.AgentRoomName != rooms[1].Name || rooms[1].ParentID == nil || *rooms[1].ParentID != rooms[0].ID || rooms[1].Scope != agentLoginScope(agent.UserID)) {
		return fmt.Errorf("测试代理上级或房间归属不匹配，拒绝覆盖")
	}
	member := accounts[3]
	if member.UserID != 0 && (agent.UserID == 0 || member.ParentTenantID == nil || *member.ParentTenantID != accounts[1].UserID || member.ParentAgentID == nil || *member.ParentAgentID != agent.UserID || member.WorkspaceID != rooms[1].ID || member.LoginScope != rooms[1].Scope) {
		return fmt.Errorf("测试会员房间归属不匹配，拒绝覆盖")
	}
	return nil
}
