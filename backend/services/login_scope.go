package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const platformLoginScope = "platform"

func tenantLoginScope(tenantID uint64) string { return fmt.Sprintf("tenant:%d", tenantID) }
func agentLoginScope(agentID uint64) string   { return fmt.Sprintf("agent:%d", agentID) }

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
	var count int64
	query := db.Model(&user.User{}).Where("login_scope = ? AND LOWER(username) = LOWER(?)", scope, strings.TrimSpace(username))
	if excludeID > 0 {
		query = query.Where("user_id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return apperrors.NewBusinessError("USERNAME_EXISTS", "该租户或代理下已存在相同登录帐号")
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
			var agents []user.User
			like := strings.ToLower(workspace)
			if err := db.Select("user_id").Where("role = ? AND status = ? AND (LOWER(agent_room_code) = ? OR LOWER(username) = ? OR LOWER(nickname) = ?)", "agent", 1, like, like, like).Limit(2).Find(&agents).Error; err != nil {
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
