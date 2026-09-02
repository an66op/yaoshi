package services

import (
	"backend/data/models/user"

	"gorm.io/gorm"
)

// HumanMemberQuery keeps member counts consistent with the member list. A
// disabled robot is still a robot; do not filter profiles by enabled/status.
// The remark exclusion preserves the list's legacy test-account handling.
// Usernames alone never establish robot identity.
func HumanMemberQuery(db *gorm.DB) *gorm.DB {
	return excludeRobotProfileUsers(db.Model(&user.User{})).
		Where(`"user".role = ? AND COALESCE("user".remark, '') NOT LIKE ?`, "member", "测试机器人专用账号%")
}
