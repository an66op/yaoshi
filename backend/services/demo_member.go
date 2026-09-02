package services

import (
	"backend/data/models/user"
	"backend/utils"
	"fmt"

	"gorm.io/gorm"
)

const (
	demoUsername       = "wangzhe88"
	demoPassword       = "Wz888888"
	demoNickname       = "王者玩家"
	demoRoomCode       = "88001"
	demoRoomName       = "王者聊天室"
	demoAgentUsername  = "suyang"
	demoAgentPassword  = "Room8801"
	demoTenantUsername = "wangzhetenant"
	demoTenantPassword = "WzTenant8801"
	demoBalanceCents   = int64(1_000_000_000) // 10,000,000.00 元
)

// SeedExperienceMember creates local acceptance accounts only when they do not
// exist.  A bootstrap must never turn a deliberately disabled account back on,
// reset its password, move it to another room, or replace a member nickname.
// Conflicting fixed usernames/room numbers fail visibly instead of silently
// taking ownership of operator-managed data.
func SeedExperienceMember(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var tenant user.User
		tenantErr := tx.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", demoTenantUsername).First(&tenant).Error
		if tenantErr == gorm.ErrRecordNotFound {
			tenantHash, err := utils.HashPassword(demoTenantPassword)
			if err != nil {
				return err
			}
			tenant = user.User{
				Username: demoTenantUsername, LoginScope: platformLoginScope, Password: tenantHash,
				Nickname: "王者体验租户", Role: "tenant", Status: 1,
			}
			if err := tx.Create(&tenant).Error; err != nil {
				return err
			}
		} else if tenantErr != nil {
			return tenantErr
		} else if tenant.Role != "tenant" {
			return fmt.Errorf("本地体验账号 %s 已被其他角色占用", demoTenantUsername)
		}
		tenantID := tenant.UserID

		var agent user.User
		agentErr := tx.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", demoAgentUsername).First(&agent).Error
		if agentErr == gorm.ErrRecordNotFound {
			var occupied user.User
			roomErr := tx.Where("agent_room_code = ? AND deleted_at IS NULL", demoRoomCode).First(&occupied).Error
			if roomErr == nil {
				return fmt.Errorf("本地体验房间号 %s 已由账号 %s 使用", demoRoomCode, occupied.Username)
			}
			if roomErr != gorm.ErrRecordNotFound {
				return roomErr
			}
			agentHash, err := utils.HashPassword(demoAgentPassword)
			if err != nil {
				return err
			}
			agent = user.User{
				Username: demoAgentUsername, Password: agentHash, Nickname: "苏洋",
				Role: "agent", Status: 1, AgentRoomCode: demoRoomCode, AgentRoomName: demoRoomName,
				LoginScope: "tenant:" + demoTenantUsername, ParentTenantID: &tenantID,
			}
			if err := tx.Create(&agent).Error; err != nil {
				return err
			}
		} else if agentErr != nil {
			return agentErr
		} else if agent.Role != "agent" {
			return fmt.Errorf("本地体验账号 %s 已被其他角色占用", demoAgentUsername)
		}
		parentID := &agent.UserID

		var account user.User
		queryErr := tx.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", demoUsername).First(&account).Error
		if queryErr == gorm.ErrRecordNotFound {
			hash, err := utils.HashPassword(demoPassword)
			if err != nil {
				return err
			}
			memberScope := agentLoginScope(agent.UserID)
			account = user.User{
				Username: demoUsername, LoginScope: memberScope, Password: hash, Nickname: demoNickname,
				Role: "member", Status: 1, BalanceCents: demoBalanceCents,
				ParentAgentID: parentID, ParentTenantID: &tenantID,
			}
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			return ensureSeededBalance(tx, &account, 0, 0, "账户初始化")
		}
		if queryErr != nil {
			return queryErr
		}
		if account.Role != "member" {
			return fmt.Errorf("本地体验账号 %s 已被其他角色占用", demoUsername)
		}
		return nil
	})
}
