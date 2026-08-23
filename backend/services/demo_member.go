package services

import (
	"backend/data/models/user"
	"backend/utils"

	"gorm.io/gorm"
)

const (
	demoUsername     = "wangzhe88"
	demoPassword     = "Wz888888"
	demoNickname     = "体验玩家"
	demoRoomCode     = "8801"
	demoBalanceCents = int64(1_000_000_000) // 10,000,000.00 元
)

// SeedDemoMember keeps the public experience account usable after every
// deployment. It never replaces the server database: it only creates or
// repairs this one account and links it to room 8801 when that agent exists.
func SeedDemoMember(db *gorm.DB) error {
	hash, err := utils.HashPassword(demoPassword)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var parentID *uint64
		var agent user.User
		if err := tx.Where("role = ? AND agent_room_code = ?", "agent", demoRoomCode).First(&agent).Error; err == nil {
			parentID = &agent.UserID
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		var account user.User
		err := tx.Where("username = ?", demoUsername).First(&account).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&user.User{
				Username: demoUsername, Password: hash, Nickname: demoNickname,
				Role: "member", Status: 1, BalanceCents: demoBalanceCents,
				ParentAgentID: parentID,
			}).Error
		}
		if err != nil {
			return err
		}

		updates := map[string]any{
			"password":      hash,
			"nickname":      demoNickname,
			"role":          "member",
			"status":        1,
			"balance_cents": gorm.Expr("GREATEST(balance_cents, ?)", demoBalanceCents),
		}
		if parentID != nil {
			updates["parent_agent_id"] = *parentID
		}
		return tx.Model(&account).Updates(updates).Error
	})
}
