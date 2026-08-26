// Package accesscontrol contains authorization rules shared by middleware and
// domain services.  Keeping the hierarchy check here prevents one entry point
// (login, room lookup, API middleware, background work) from silently applying
// a weaker definition of an active agent.
package accesscontrol

import (
	"backend/data/models/user"
	"strings"

	"gorm.io/gorm"
)

// AgentHierarchyActive reports whether an otherwise active agent may operate.
// Standalone agents are valid.  An agent assigned to a tenant is valid only
// while that exact tenant account still exists, has role=tenant, and is active.
func AgentHierarchyActive(db *gorm.DB, account user.User) (bool, error) {
	if account.Role != "agent" || account.Status != 1 {
		return false, nil
	}
	if account.ParentTenantID == nil {
		return true, nil
	}
	var count int64
	if err := db.Model(&user.User{}).
		Where("user_id = ? AND role = ? AND status = ?", *account.ParentTenantID, "tenant", 1).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count == 1, nil
}

// AccountRoomActive validates the room that an account is currently attached
// to. Lobby members are valid without an agent. Room members require an active
// agent with a room number and, when present, an active owning tenant.
func AccountRoomActive(db *gorm.DB, account user.User) (bool, error) {
	if account.Status != 1 {
		return false, nil
	}
	if account.Role == "agent" {
		if strings.TrimSpace(account.AgentRoomCode) == "" {
			return false, nil
		}
		return AgentHierarchyActive(db, account)
	}
	if account.ParentAgentID == nil {
		return true, nil
	}
	var agent user.User
	if err := db.Select("user_id", "role", "status", "agent_room_code", "parent_tenant_id").
		Where("user_id = ? AND role = ? AND status = ?", *account.ParentAgentID, "agent", 1).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(agent.AgentRoomCode) == "" {
		return false, nil
	}
	return AgentHierarchyActive(db, agent)
}
