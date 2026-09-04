package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"testing"
)

func TestSystemAuditHierarchyCountersPostgres(t *testing.T) {
	db := timingPostgresDatabase(t)
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal("read platform workspace:", err)
	}
	created, err := NewUserAdminService(db).Create(CreateAdminUserInput{
		Username: "hierarchy_audit_member",
		Password: "TimingFixture#2026_a9Z",
		Role:     "member",
		Status:   1,
	})
	if err != nil {
		t.Fatal("create hierarchy audit member:", err)
	}
	audit := NewSystemAuditService(db)
	baseline, err := audit.Reconciliation()
	if err != nil {
		t.Fatal("baseline hierarchy audit:", err)
	}
	if baseline.AccountHierarchyErrorCount != 0 || baseline.WorkspaceHierarchyErrorCount != 0 || baseline.MembershipHierarchyErrorCount != 0 {
		var accounts []struct {
			Username       string
			Role           string
			LoginScope     string
			WorkspaceType  string
			ParentAgentID  *uint64
			ParentTenantID *uint64
		}
		if queryErr := db.Raw(`
			SELECT account.username, account.role, account.login_scope,
				workspace.type AS workspace_type, account.parent_agent_id, account.parent_tenant_id
			FROM "user" account
			LEFT JOIN workspaces workspace ON workspace.id = account.workspace_id
			WHERE account.deleted_at IS NULL ORDER BY account.user_id
		`).Scan(&accounts).Error; queryErr == nil {
			t.Logf("accounts at failed baseline: %+v", accounts)
		}
		t.Fatalf("fresh hierarchy has errors: accounts=%d workspaces=%d memberships=%d", baseline.AccountHierarchyErrorCount, baseline.WorkspaceHierarchyErrorCount, baseline.MembershipHierarchyErrorCount)
	}

	if err := db.Model(&workspacemodel.Membership{}).
		Where("workspace_id = ? AND user_id = ?", platform.ID, created.ID).
		Update("status", 0).Error; err != nil {
		t.Fatal("corrupt current membership fixture:", err)
	}
	membershipDrift, err := audit.Reconciliation()
	if err != nil {
		t.Fatal("audit membership drift:", err)
	}
	if membershipDrift.MembershipHierarchyErrorCount != 1 {
		t.Fatalf("membership drift count = %d, want 1", membershipDrift.MembershipHierarchyErrorCount)
	}
	if err := db.Model(&workspacemodel.Membership{}).
		Where("workspace_id = ? AND user_id = ?", platform.ID, created.ID).
		Update("status", 1).Error; err != nil {
		t.Fatal("repair current membership fixture:", err)
	}

	if err := db.Model(&user.User{}).Where("user_id = ?", created.ID).
		Update("parent_tenant_id", platform.OwnerUserID).Error; err != nil {
		t.Fatal("corrupt account hierarchy fixture:", err)
	}
	accountDrift, err := audit.Reconciliation()
	if err != nil {
		t.Fatal("audit account drift:", err)
	}
	if accountDrift.AccountHierarchyErrorCount != 1 {
		t.Fatalf("account hierarchy drift count = %d, want 1", accountDrift.AccountHierarchyErrorCount)
	}
	if err := db.Model(&user.User{}).Where("user_id = ?", created.ID).
		Update("parent_tenant_id", nil).Error; err != nil {
		t.Fatal("repair account hierarchy fixture:", err)
	}

	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", platform.ID).
		Update("status", 0).Error; err != nil {
		t.Fatal("corrupt workspace hierarchy fixture:", err)
	}
	workspaceDrift, err := audit.Reconciliation()
	if err != nil {
		t.Fatal("audit workspace drift:", err)
	}
	if workspaceDrift.WorkspaceHierarchyErrorCount != 1 {
		t.Fatalf("workspace hierarchy drift count = %d, want 1", workspaceDrift.WorkspaceHierarchyErrorCount)
	}
}
