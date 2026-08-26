package services

import (
	"backend/data/models/user"
	"backend/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	demoUsername      = "wangzhe88"
	demoPassword      = "Wz888888"
	demoNickname      = "王者玩家"
	demoRoomCode      = "8801"
	demoAgentPassword = "Room8801"
	demoBalanceCents  = int64(1_000_000_000) // 10,000,000.00 元
)

// SeedExperienceMember keeps the public experience account usable after every
// deployment. It never replaces the server database: it only creates or
// repairs this one account and links it to room 8801 when that agent exists.
func SeedExperienceMember(db *gorm.DB) error {
	hash, err := utils.HashPassword(demoPassword)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		agentHash, err := utils.HashPassword(demoAgentPassword)
		if err != nil {
			return err
		}
		var agent user.User
		if err := tx.Where("role = ? AND agent_room_code = ?", "agent", demoRoomCode).First(&agent).Error; err == gorm.ErrRecordNotFound {
			agent = user.User{
				Username: "room8801", Password: agentHash, Nickname: "8801房间",
				Role: "agent", Status: 1, AgentRoomCode: demoRoomCode, AgentRoomName: "王者体验房", LoginScope: platformLoginScope,
			}
			if err := tx.Create(&agent).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := tx.Model(&agent).Updates(map[string]any{"password": agentHash, "status": 1}).Error; err != nil {
			return err
		}
		parentID := &agent.UserID

		var account user.User
		memberScope := agentLoginScope(agent.UserID)
		queryErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("login_scope = ? AND username = ?", memberScope, demoUsername).First(&account).Error
		if queryErr == gorm.ErrRecordNotFound {
			account = user.User{
				Username: demoUsername, LoginScope: memberScope, Password: hash, Nickname: demoNickname,
				Role: "member", Status: 1, BalanceCents: demoBalanceCents,
				ParentAgentID: parentID, ParentTenantID: agent.ParentTenantID,
			}
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			return ensureSeededBalance(tx, &account, demoBalanceCents, demoBalanceCents, "账户初始化")
		}
		if queryErr != nil {
			return queryErr
		}

		updates := map[string]any{
			"password": hash,
			"role":     "member",
			"status":   1,
		}
		// The seed keeps the login account usable, but a member-owned nickname
		// must never be reset when the backend restarts.
		if account.Nickname == "" {
			updates["nickname"] = demoNickname
		}
		updates["parent_agent_id"] = *parentID
		updates["parent_tenant_id"] = agent.ParentTenantID
		updates["login_scope"] = memberScope
		if err := tx.Model(&account).Updates(updates).Error; err != nil {
			return err
		}
		return ensureSeededBalance(tx, &account, demoBalanceCents, demoBalanceCents, "账户初始化")
	})
}
