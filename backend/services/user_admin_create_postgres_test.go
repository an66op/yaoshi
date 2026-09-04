package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"testing"
)

func TestUserAdminCreateMemberPostgresActivatesAgentRoomAtomically(t *testing.T) {
	db := timingPostgresDatabase(t)
	tenantRoom := timingPostgresRoom(t, db, "admin_create_tenant", "76701")
	agentRoom := timingPostgresAgentRoom(t, db, tenantRoom, "admin_create_agent", "76702")

	service := NewUserAdminService(db)
	created, err := service.Create(CreateAdminUserInput{
		Username:      "admin_create_member",
		Password:      "TimingFixture#2026_a9Z",
		Nickname:      "后台开户会员",
		Role:          "member",
		Status:        1,
		ParentAgentID: agentRoom.OwnerUserID,
	})
	if err != nil {
		t.Fatal("create member directly in agent room:", err)
	}
	if created.WorkspaceID != agentRoom.ID {
		t.Fatalf("returned workspace_id = %d, want %d", created.WorkspaceID, agentRoom.ID)
	}
	if created.ParentAgentID == nil || *created.ParentAgentID != agentRoom.OwnerUserID {
		t.Fatalf("returned parent_agent_id = %v, want %d", created.ParentAgentID, agentRoom.OwnerUserID)
	}
	if created.ParentTenantID == nil || *created.ParentTenantID != tenantRoom.OwnerUserID {
		t.Fatalf("returned parent_tenant_id = %v, want %d", created.ParentTenantID, tenantRoom.OwnerUserID)
	}

	var stored user.User
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal("read created member:", err)
	}
	if stored.WorkspaceID != agentRoom.ID || stored.LoginScope != agentRoom.Scope {
		t.Fatalf("stored room identity = workspace %d / scope %q, want %d / %q", stored.WorkspaceID, stored.LoginScope, agentRoom.ID, agentRoom.Scope)
	}
	if stored.ParentAgentID == nil || *stored.ParentAgentID != agentRoom.OwnerUserID {
		t.Fatalf("stored parent_agent_id = %v, want %d", stored.ParentAgentID, agentRoom.OwnerUserID)
	}
	if stored.ParentTenantID == nil || *stored.ParentTenantID != tenantRoom.OwnerUserID {
		t.Fatalf("stored parent_tenant_id = %v, want %d", stored.ParentTenantID, tenantRoom.OwnerUserID)
	}

	var membership workspacemodel.Membership
	if err := db.Where("workspace_id = ? AND user_id = ?", agentRoom.ID, created.ID).First(&membership).Error; err != nil {
		t.Fatal("read created room membership:", err)
	}
	if membership.Role != "member" || membership.Status != 1 {
		t.Fatalf("created room membership = role %q / status %d, want member / 1", membership.Role, membership.Status)
	}
	var membershipCount int64
	if err := db.Model(&workspacemodel.Membership{}).Where("user_id = ?", created.ID).Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 {
		t.Fatalf("created member has %d workspace memberships, want exactly 1", membershipCount)
	}

	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal("read platform workspace:", err)
	}
	lobbyMember, err := service.Create(CreateAdminUserInput{
		Username: "admin_create_lobby_member",
		Password: "TimingFixture#2026_a9Z",
		Nickname: "平台大厅会员",
		Role:     "member",
		Status:   1,
	})
	if err != nil {
		t.Fatal("create platform lobby member:", err)
	}
	if lobbyMember.WorkspaceID != platform.ID || lobbyMember.ParentAgentID != nil || lobbyMember.ParentTenantID != nil {
		t.Fatalf("lobby member hierarchy = workspace %d / agent %v / tenant %v, want platform %d with no parents", lobbyMember.WorkspaceID, lobbyMember.ParentAgentID, lobbyMember.ParentTenantID, platform.ID)
	}
	var lobbyAccount user.User
	if err := db.First(&lobbyAccount, lobbyMember.ID).Error; err != nil {
		t.Fatal("read platform lobby member:", err)
	}
	if lobbyAccount.WorkspaceID != platform.ID || lobbyAccount.LoginScope != platformLoginScope {
		t.Fatalf("lobby member binding = workspace %d / scope %q, want %d / %q", lobbyAccount.WorkspaceID, lobbyAccount.LoginScope, platform.ID, platformLoginScope)
	}
	var lobbyMembership workspacemodel.Membership
	if err := db.Where("workspace_id = ? AND user_id = ?", platform.ID, lobbyMember.ID).First(&lobbyMembership).Error; err != nil {
		t.Fatal("read platform lobby membership:", err)
	}
	if lobbyMembership.Role != "member" || lobbyMembership.Status != 1 {
		t.Fatalf("lobby membership = role %q / status %d, want member / 1", lobbyMembership.Role, lobbyMembership.Status)
	}

	for _, forbiddenRole := range []string{"tenant", "agent", "admin"} {
		username := "admin_create_forbidden_" + forbiddenRole
		if created, createErr := service.Create(CreateAdminUserInput{
			Username: username,
			Password: "TimingFixture#2026_a9Z",
			Role:     forbiddenRole,
			Status:   1,
		}); created != nil || apperrors.GetErrorCode(createErr) != "DEDICATED_ACCOUNT_SERVICE_REQUIRED" {
			t.Fatalf("generic create role %q = result %#v / error %v, want dedicated-service rejection", forbiddenRole, created, createErr)
		}
		var count int64
		if err := db.Unscoped().Model(&user.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejected generic role %q left %d user rows", forbiddenRole, count)
		}
	}

	if updated, updateErr := service.Update(created.ID, UpdateAdminUserInput{
		Email: created.Email, Nickname: created.Nickname, Phone: created.Phone,
		Role: "agent", Remark: created.Remark, RiskLevel: created.RiskLevel, Status: created.Status,
	}); updated != nil || apperrors.GetErrorCode(updateErr) != "ROLE_CHANGE_NOT_ALLOWED" {
		t.Fatalf("generic role mutation = result %#v / error %v, want role-change rejection", updated, updateErr)
	}
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal("reload role-protected member:", err)
	}
	if stored.Role != "member" || stored.WorkspaceID != agentRoom.ID {
		t.Fatalf("rejected role mutation changed hierarchy: role %q / workspace %d", stored.Role, stored.WorkspaceID)
	}

	if _, err := service.SetStatusInWorkspace(created.ID, agentRoom.ID, 0); err != nil {
		t.Fatal("disable member in workspace:", err)
	}
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("workspace_id = ? AND user_id = ?", agentRoom.ID, created.ID).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != 0 || membership.Status != 0 {
		t.Fatalf("workspace disable drifted: user=%d membership=%d", stored.Status, membership.Status)
	}
	if _, err := service.SetStatus(created.ID, 1); err != nil {
		t.Fatal("enable member through platform admin:", err)
	}
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("workspace_id = ? AND user_id = ?", agentRoom.ID, created.ID).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != 1 || membership.Status != 1 {
		t.Fatalf("platform enable drifted: user=%d membership=%d", stored.Status, membership.Status)
	}
	if _, err := service.Update(created.ID, UpdateAdminUserInput{
		Email: created.Email, Nickname: "停用会员", Phone: created.Phone,
		Role: "member", Remark: created.Remark, RiskLevel: created.RiskLevel, Status: 0,
	}); err != nil {
		t.Fatal("update member status:", err)
	}
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("workspace_id = ? AND user_id = ?", agentRoom.ID, created.ID).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != 0 || membership.Status != 0 {
		t.Fatalf("profile update status drifted: user=%d membership=%d", stored.Status, membership.Status)
	}

	var usersBefore, membershipsBefore int64
	if err := db.Unscoped().Model(&user.User{}).Count(&usersBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Membership{}).Count(&membershipsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE FUNCTION reject_admin_member_room_activation() RETURNS trigger AS $$
		BEGIN
			IF NEW.username = 'admin_create_rollback' AND NEW.workspace_id <> OLD.workspace_id THEN
				RAISE EXCEPTION 'forced room activation failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`).Error; err != nil {
		t.Fatal("install rollback fixture function:", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_admin_member_room_activation
		BEFORE UPDATE OF workspace_id ON "user"
		FOR EACH ROW EXECUTE FUNCTION reject_admin_member_room_activation()
	`).Error; err != nil {
		t.Fatal("install rollback fixture trigger:", err)
	}

	if _, err := service.Create(CreateAdminUserInput{
		Username:      "admin_create_rollback",
		Password:      "TimingFixture#2026_a9Z",
		Nickname:      "应整体回滚",
		Role:          "member",
		Status:        1,
		ParentAgentID: agentRoom.OwnerUserID,
	}); err == nil {
		t.Fatal("expected forced room activation failure")
	}

	var rolledBackUsers int64
	if err := db.Unscoped().Model(&user.User{}).Where("username = ?", "admin_create_rollback").Count(&rolledBackUsers).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBackUsers != 0 {
		t.Fatalf("failed create left %d user rows, want 0", rolledBackUsers)
	}
	var usersAfter, membershipsAfter int64
	if err := db.Unscoped().Model(&user.User{}).Count(&usersAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Membership{}).Count(&membershipsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if usersAfter != usersBefore || membershipsAfter != membershipsBefore {
		t.Fatalf("failed create was not atomic: users %d -> %d, memberships %d -> %d", usersBefore, usersAfter, membershipsBefore, membershipsAfter)
	}
}
