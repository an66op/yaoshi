package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"fmt"
	"testing"
)

func TestHumanMemberCountsPostgresPreserveOwnershipScopes(t *testing.T) {
	db := timingPostgresDatabase(t)
	tenant := timingPostgresRoom(t, db, "count_tenant", "76921")
	otherTenant := timingPostgresRoom(t, db, "count_other_tenant", "76922")
	agent := timingPostgresAgentRoom(t, db, tenant, "count_agent", "76923")
	siblingAgent := timingPostgresAgentRoom(t, db, tenant, "count_sibling_agent", "76924")
	otherAgent := timingPostgresAgentRoom(t, db, otherTenant, "count_other_agent", "76925")
	standalone, err := NewAgentAdminService(db).Create(CreateAgentInput{
		Username: "count_standalone_agent", Password: "TimingFixture#2026_a9Z", RoomCode: "76926", Status: 1,
	})
	if err != nil {
		t.Fatal("create standalone agent:", err)
	}
	var standaloneRoom, platform workspacemodel.Workspace
	if err := db.First(&standaloneRoom, standalone.WorkspaceID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}

	type roomFixture struct {
		name     string
		room     workspacemodel.Workspace
		statuses []int
		humans   map[uint64]int
	}
	rooms := []roomFixture{
		{name: "platform", room: platform, statuses: []int{1}},
		{name: "tenant direct", room: tenant, statuses: []int{1, 0, 1}},
		{name: "other tenant direct", room: otherTenant, statuses: []int{1, 1}},
		{name: "tenant agent", room: agent, statuses: []int{1, 0, 1}},
		{name: "sibling agent", room: siblingAgent, statuses: []int{1}},
		{name: "other tenant agent", room: otherAgent, statuses: []int{1, 0}},
		{name: "standalone agent", room: standaloneRoom, statuses: []int{1}},
	}
	allHumans := make(map[uint64]int)
	for index := range rooms {
		fixture := &rooms[index]
		fixture.humans = make(map[uint64]int)
		var parentTenantID, parentAgentID *uint64
		switch fixture.room.Type {
		case workspacemodel.TypeTenant:
			parentTenantID = &fixture.room.OwnerUserID
		case workspacemodel.TypeAgent:
			parentAgentID = &fixture.room.OwnerUserID
			var owner user.User
			if err := db.First(&owner, fixture.room.OwnerUserID).Error; err != nil {
				t.Fatal(err)
			}
			parentTenantID = owner.ParentTenantID
		}
		insert := func(name, remark string, status int, robot bool) user.User {
			t.Helper()
			account := user.User{
				Username: name, Password: "fixture-no-login", Nickname: name,
				LoginScope: fixture.room.Scope, WorkspaceID: fixture.room.ID,
				ParentTenantID: parentTenantID, ParentAgentID: parentAgentID,
				Role: "member", Status: 1, Remark: remark,
			}
			if err := db.Create(&account).Error; err != nil {
				t.Fatal("create member count fixture:", err)
			}
			// GORM applies the model's default to zero values on insertion.
			if err := db.Model(&account).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
			if robot {
				profile := workspacemodel.RobotProfile{WorkspaceID: fixture.room.ID, UserID: account.UserID, Enabled: true}
				if err := db.Create(&profile).Error; err != nil {
					t.Fatal("create explicit robot identity:", err)
				}
				if err := db.Model(&profile).Update("enabled", status == 1).Error; err != nil {
					t.Fatal(err)
				}
			}
			return account
		}
		for humanIndex, status := range fixture.statuses {
			name := fmt.Sprintf("count_human_%d_%d", fixture.room.ID, humanIndex)
			if humanIndex == 2 {
				// A reserved-looking historic name is not robot identity.
				name = fmt.Sprintf("room_robot_human_%d", fixture.room.ID)
			}
			account := insert(name, "", status, false)
			fixture.humans[account.UserID] = status
			allHumans[account.UserID] = status
		}
		for _, status := range []int{0, 1} {
			// Profile-backed accounts remain robots after their names/remarks change,
			// and a disabled robot/profile must not become a counted human.
			insert(fmt.Sprintf("count_profile_%d_%d", fixture.room.ID, status), "已修改的机器人备注", status, true)
			insert(fmt.Sprintf("count_legacy_%d_%d", fixture.room.ID, status), "测试机器人专用账号：历史兼容", status, false)
		}
	}

	assertMembers := func(t *testing.T, list *UserList, expected map[uint64]int, status string) {
		t.Helper()
		wanted := make(map[uint64]int)
		for id, active := range expected {
			if status == "active" && active != 1 || status == "disabled" && active != 0 {
				continue
			}
			wanted[id] = active
		}
		if list.Total != int64(len(wanted)) || len(list.Items) != len(wanted) {
			t.Fatalf("member list count = %d/%d, want %d", list.Total, len(list.Items), len(wanted))
		}
		for _, item := range list.Items {
			active, ok := wanted[item.ID]
			if !ok || item.Status != active || item.IsRobot {
				t.Fatalf("unexpected or misclassified member: %+v", item)
			}
			delete(wanted, item.ID)
		}
		if len(wanted) != 0 {
			t.Fatalf("missing human members: %v", wanted)
		}
	}

	t.Run("member list and stats", func(t *testing.T) {
		service := NewUserAdminService(db)
		for _, status := range []string{"", "active", "disabled"} {
			list, err := service.List(UserListFilter{Kind: "member", Status: status, Page: 1, PageSize: 100})
			if err != nil {
				t.Fatal(err)
			}
			assertMembers(t, list, allHumans, status)
		}
		stats, err := service.Stats("member")
		if err != nil {
			t.Fatal(err)
		}
		if stats.Total != 13 || stats.Active != 10 || stats.Disabled != 3 || stats.NewToday != 13 {
			t.Fatalf("human member statistics = %+v, want total/new 13, active 10, disabled 3", stats)
		}
	})

	t.Run("workspace membership scope", func(t *testing.T) {
		for _, fixture := range rooms {
			t.Run(fixture.name, func(t *testing.T) {
				list, err := NewUserAdminService(db).List(UserListFilter{Kind: "member", WorkspaceID: fixture.room.ID, Page: 1, PageSize: 100})
				if err != nil {
					t.Fatal(err)
				}
				assertMembers(t, list, fixture.humans, "")
				dashboard, err := NewAgentWorkspaceService(db).DashboardForWorkspace(fixture.room.ID)
				if err != nil {
					t.Fatal(err)
				}
				var active int64
				for _, status := range fixture.statuses {
					active += int64(status)
				}
				if dashboard.MemberCount != int64(len(fixture.humans)) || dashboard.ActiveMemberCount != active {
					t.Fatalf("workspace counts = %d/%d, want %d/%d", dashboard.MemberCount, dashboard.ActiveMemberCount, len(fixture.humans), active)
				}
			})
		}
	})

	t.Run("tenant direct counts and descendant summary", func(t *testing.T) {
		service := NewTenantAdminService(db)
		wanted := map[uint64]int64{tenant.OwnerUserID: 3, otherTenant.OwnerUserID: 2}
		for id, members := range wanted {
			dashboard, err := service.Dashboard(id)
			if err != nil {
				t.Fatal(err)
			}
			if dashboard["member_count"] != members {
				t.Fatalf("tenant %d dashboard member count = %v, want direct-only %d", id, dashboard["member_count"], members)
			}
			var owner user.User
			if err := db.First(&owner, id).Error; err != nil {
				t.Fatal(err)
			}
			view, err := service.toView(owner)
			if err != nil || view.MemberCount != members {
				t.Fatalf("tenant %d view = %+v, err=%v, want %d members", id, view, err, members)
			}
		}
		list, err := service.List("", 1, 100)
		if err != nil {
			t.Fatal(err)
		}
		// The existing summary includes tenant-owned agent rooms, not tenant
		// direct rooms, the platform lobby, or independently owned agent rooms.
		if list.Total != 2 || len(list.Items) != 2 || list.Members != 6 {
			t.Fatalf("tenant summary = %+v, want 2 tenants and 6 descendant-agent members", list)
		}
		for _, item := range list.Items {
			if members, ok := wanted[item.ID]; !ok || item.MemberCount != members {
				t.Fatalf("tenant list crossed direct-room ownership: %+v", item)
			}
		}
	})

	t.Run("agent rows and ownership summaries", func(t *testing.T) {
		service := NewAgentAdminService(db)
		wanted := map[uint64]int64{agent.OwnerUserID: 3, siblingAgent.OwnerUserID: 1, otherAgent.OwnerUserID: 2, standalone.ID: 1}
		for id, members := range wanted {
			view, err := service.view(id)
			if err != nil || view.MemberCount != members {
				t.Fatalf("agent %d view = %+v, err=%v, want %d members", id, view, err, members)
			}
		}
		for _, fixture := range []struct {
			name     string
			tenantID uint64
			members  int64
			agents   map[uint64]int64
		}{
			{name: "all agents", members: 7, agents: wanted},
			{name: "first tenant", tenantID: tenant.OwnerUserID, members: 4, agents: map[uint64]int64{agent.OwnerUserID: 3, siblingAgent.OwnerUserID: 1}},
			{name: "other tenant", tenantID: otherTenant.OwnerUserID, members: 2, agents: map[uint64]int64{otherAgent.OwnerUserID: 2}},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				var list *AgentListResult
				var err error
				if fixture.tenantID == 0 {
					list, err = service.List("", 1, 100)
				} else {
					list, err = service.ListForTenant(fixture.tenantID, "", 1, 100)
				}
				if err != nil {
					t.Fatal(err)
				}
				if list.Total != int64(len(fixture.agents)) || len(list.Items) != len(fixture.agents) || list.Summary.Members != fixture.members {
					t.Fatalf("agent summary = %+v, want %d agents and %d members", list, len(fixture.agents), fixture.members)
				}
				for _, item := range list.Items {
					if members, ok := fixture.agents[item.ID]; !ok || item.MemberCount != members {
						t.Fatalf("agent list crossed ownership: %+v", item)
					}
				}
			})
		}
	})
}
