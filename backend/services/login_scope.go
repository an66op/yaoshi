package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const platformLoginScope = "platform"

var reservedRobotUsernamePrefixes = []string{"room_robot_", "room_activity_"}

func tenantLoginScope(tenantID uint64) string { return fmt.Sprintf("tenant:%d", tenantID) }
func agentLoginScope(agentID uint64) string   { return fmt.Sprintf("agent:%d", agentID) }

// validateHumanUsername keeps the internal robot identity namespace out of
// every public/admin account-creation path. Robot provisioning deliberately
// bypasses this helper and is the only code allowed to allocate these names.
func validateHumanUsername(username string) error {
	value := strings.ToLower(strings.TrimSpace(username))
	for _, prefix := range reservedRobotUsernamePrefixes {
		if strings.HasPrefix(value, prefix) {
			return apperrors.NewBusinessError("RESERVED_USERNAME", "该登录帐号前缀为系统机器人保留")
		}
	}
	return nil
}

func loginScopeForAgent(db *gorm.DB, agentID uint64) (string, *uint64, error) {
	var agent user.User
	if err := db.Select("user_id", "parent_tenant_id", "role", "status").Where("user_id = ? AND role = ? AND status = ?", agentID, "agent", 1).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在或已停用")
		}
		return "", nil, err
	}
	return agentLoginScope(agent.UserID), agent.ParentTenantID, nil
}

func ensureUsernameInScope(db *gorm.DB, scope, username string, excludeID uint64) error {
	if err := validateHumanUsername(username); err != nil {
		return err
	}
	var count int64
	// Login is account + password only. Account names are therefore globally
	// unique; workspace ownership is resolved after authentication and is never
	// accepted from a public login form.
	query := db.Model(&user.User{}).Where("LOWER(username) = LOWER(?)", strings.TrimSpace(username))
	if excludeID > 0 {
		query = query.Where("user_id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return apperrors.NewBusinessError("USERNAME_EXISTS", "登录帐号已存在")
	}
	return nil
}

func loginScopeForWorkspace(db *gorm.DB, username, workspace string, memberPortal bool) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if strings.HasPrefix(workspace, "tenant:") || strings.HasPrefix(workspace, "agent:") || workspace == platformLoginScope {
		return workspace, nil
	}
	if workspace == "平台" || strings.EqualFold(workspace, "platform") {
		return platformLoginScope, nil
	}
	if workspace != "" {
		if memberPortal {
			value := strings.ToLower(workspace)
			var room workspacemodel.Workspace
			if err := db.Where("type IN ? AND status = ? AND LOWER(room_code) = ?", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1, value).First(&room).Error; err == nil {
				return room.Scope, nil
			} else if err != gorm.ErrRecordNotFound {
				return "", err
			}
			var rooms []workspacemodel.Workspace
			if err := db.Select("scope").Where("type IN ? AND status = ? AND LOWER(name) = ?", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1, value).Limit(2).Find(&rooms).Error; err != nil {
				return "", err
			}
			if len(rooms) == 1 {
				return rooms[0].Scope, nil
			}
			if len(rooms) > 1 {
				return "", apperrors.NewBusinessError("AMBIGUOUS_WORKSPACE", "房间名称重复，请使用房间号")
			}

			// Compatibility for historic bookmarks that used the agent account or
			// nickname instead of the public room number. Room identity itself is
			// always resolved from workspaces above and never from the legacy shadow.
			var agents []user.User
			if err := db.Select("user_id").Where("role = ? AND status = ? AND (LOWER(username) = ? OR LOWER(nickname) = ?)", "agent", 1, value, value).Limit(2).Find(&agents).Error; err != nil {
				return "", err
			}
			if len(agents) == 1 {
				return agentLoginScope(agents[0].UserID), nil
			}
			if len(agents) > 1 {
				return "", apperrors.NewBusinessError("AMBIGUOUS_WORKSPACE", "代理名称重复，请使用房间号")
			}
			return "", apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间号或代理不存在")
		}
		var tenants []user.User
		value := strings.ToLower(workspace)
		if err := db.Select("user_id").Where("role = ? AND status = ? AND (LOWER(username) = ? OR LOWER(nickname) = ?)", "tenant", 1, value, value).Limit(2).Find(&tenants).Error; err != nil {
			return "", err
		}
		if len(tenants) == 1 {
			return tenantLoginScope(tenants[0].UserID), nil
		}
		if len(tenants) > 1 {
			return "", apperrors.NewBusinessError("AMBIGUOUS_WORKSPACE", "租户名称重复，请使用租户登录帐号")
		}
		return "", apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "租户不存在")
	}

	// Compatibility for existing bookmarked login forms: an omitted owner is
	// accepted only when the username identifies exactly one account. Once the
	// same account name exists under multiple owners, the owner becomes required.
	query := db.Model(&user.User{}).Where("LOWER(username) = LOWER(?)", strings.TrimSpace(username))
	if memberPortal {
		query = query.Where("role IN ?", []string{"member", "agent"})
	} else {
		query = query.Where("role IN ?", []string{"admin", "tenant", "agent"})
	}
	var rows []user.User
	if err := query.Select("login_scope").Limit(2).Find(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 1 {
		return rows[0].LoginScope, nil
	}
	if len(rows) > 1 {
		if memberPortal {
			return "", apperrors.NewBusinessError("LOGIN_SCOPE_REQUIRED", "请选择所属房间或代理")
		}
		return "", apperrors.NewBusinessError("LOGIN_SCOPE_REQUIRED", "请选择所属租户")
	}
	return platformLoginScope, nil
}
