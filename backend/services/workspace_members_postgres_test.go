package services

import (
	"backend/data/models/application"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestWorkspaceMembersPostgresPreserveHistoryAndIsolation(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "wm_tenant", "76931")
	otherRoom := timingPostgresRoom(t, db, "wm_other_tenant", "76932")
	agentRoom := timingPostgresAgentRoom(t, db, room, "wm_agent", "76933")
	otherAgentRoom := timingPostgresAgentRoom(t, db, otherRoom, "wm_other_agent", "76934")
	service := NewWorkspaceMemberService(db)
	memberSequence := 0
	create := func(target workspacemodel.Workspace, name string, status int, membership bool) user.User {
		t.Helper()
		memberSequence++
		account := user.User{
			Username: name, Password: "fixture-no-login", Nickname: "公开昵称_" + name,
			Role: "member", Status: 1, WorkspaceID: target.ID, LoginScope: target.Scope,
			Avatar: "/images/avatars/member.png", PublicTitle: "公开头衔", PublicBadge: "公开徽章",
			BalanceCents: 123456, Phone: fmt.Sprintf("private-phone-%d", memberSequence), Remark: "private-remark-" + name,
			Email: name + "@private.invalid", RiskLevel: "watch", LoginCount: 23,
		}
		lastLogin := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
		account.LastLoginAt = &lastLogin
		if err := db.Create(&account).Error; err != nil {
			t.Fatal("create workspace member:", err)
		}
		if membership {
			if err := ActivateWorkspaceMembership(db, &account, target); err != nil {
				t.Fatal("activate workspace member:", err)
			}
		}
		if status != 1 {
			if err := db.Model(&account).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.First(&account, account.UserID).Error; err != nil {
			t.Fatal(err)
		}
		return account
	}
	move := func(account *user.User, target workspacemodel.Workspace) {
		t.Helper()
		if err := ActivateWorkspaceMembership(db, account, target); err != nil {
			t.Fatal("move fixture member:", err)
		}
		if err := db.First(account, account.UserID).Error; err != nil {
			t.Fatal(err)
		}
	}
	addMembership := func(account user.User, target workspacemodel.Workspace, role string, status int) {
		t.Helper()
		// A map preserves status=0 instead of applying GORM's default of 1.
		if err := db.Model(&workspacemodel.Membership{}).Create(map[string]any{
			"workspace_id": target.ID, "user_id": account.UserID, "role": role,
			"status": status, "odds_multiplier": 1, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
		}).Error; err != nil {
			t.Fatal("create historical membership:", err)
		}
	}

	current := create(room, "wm_current", 1, true)
	disabled := create(room, "wm_disabled", 0, true)
	legacy := create(room, "wm_legacy_current", 1, false)
	// Older legitimate accounts can have a now-reserved-looking name. A name
	// alone is not robot identity, even though new room activation rejects it.
	prefixHuman := create(room, "room_robot_actual_human", 1, false)
	addMembership(prefixHuman, room, "member", 1)
	historical := create(room, "wm_historical", 1, true)
	move(&historical, otherRoom)
	historicalDisabled := create(room, "wm_historical_disabled", 0, true)
	move(&historicalDisabled, otherAgentRoom)
	unknown := create(otherRoom, "wm_never_joined", 1, true)
	for _, state := range []string{"pending", "rejected", "approved"} {
		account := create(otherRoom, "wm_application_only_"+state, 1, true)
		request := application.Application{
			WorkspaceID: room.ID, UserID: account.UserID, Username: account.Username,
			RequestType: "join", PaymentType: "manual", AccountType: "member", RoomScope: room.Scope,
			TargetRoomCode: room.RoomCode, Status: state, ReviewOddsMultiplier: 1,
		}
		if err := db.Create(&request).Error; err != nil {
			t.Fatal(err)
		}
	}
	badMembershipRole := create(otherRoom, "wm_bad_membership_role", 1, true)
	addMembership(badMembershipRole, room, "admin", 0)
	badMembershipState := create(otherRoom, "wm_bad_membership_state", 1, true)
	addMembership(badMembershipState, room, "member", 2)
	var otherOwner user.User
	if err := db.First(&otherOwner, otherRoom.OwnerUserID).Error; err != nil {
		t.Fatal(err)
	}
	addMembership(otherOwner, room, "member", 0)

	for _, moved := range []bool{false, true} {
		account := create(room, fmt.Sprintf("wm_profile_robot_%t", moved), 1, true)
		profile := workspacemodel.RobotProfile{WorkspaceID: room.ID, UserID: account.UserID, Enabled: true}
		if err := db.Create(&profile).Error; err != nil {
			t.Fatal(err)
		}
		if moved {
			move(&account, otherRoom)
			if err := db.Model(&profile).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	legacyRobot := create(room, "wm_legacy_robot", 1, true)
	move(&legacyRobot, otherRoom)
	if err := db.Model(&legacyRobot).Update("remark", "测试机器人专用账号：历史数据").Error; err != nil {
		t.Fatal(err)
	}
	deleted := create(room, "wm_deleted", 1, true)
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	agentCurrent := create(agentRoom, "wm_agent_current", 1, true)
	agentHistorical := create(agentRoom, "wm_agent_historical", 1, true)
	move(&agentHistorical, otherAgentRoom)
	create(otherAgentRoom, "wm_unrelated_agent_member", 1, true)

	wanted := map[uint64]bool{
		current.UserID: true, disabled.UserID: true, legacy.UserID: true, prefixHuman.UserID: true,
		historical.UserID: false, historicalDisabled.UserID: false,
	}
	list := func(workspaceID uint64, filter UserListFilter) *WorkspaceMemberList {
		t.Helper()
		result, err := service.List(workspaceID, filter)
		if err != nil {
			t.Fatal("list room members:", err)
		}
		return result
	}
	find := func(result *WorkspaceMemberList, userID uint64) WorkspaceMember {
		t.Helper()
		for _, item := range result.Items {
			if item.ID == userID {
				return item
			}
		}
		t.Fatalf("member %d absent from %+v", userID, result)
		return WorkspaceMember{}
	}
	var membershipsBefore []workspacemodel.Membership
	if err := db.Order("id").Find(&membershipsBefore).Error; err != nil {
		t.Fatal(err)
	}

	t.Run("includes joined history and current legacy accounts only", func(t *testing.T) {
		result := list(room.ID, UserListFilter{Page: 1, PageSize: 100})
		if result.Total != int64(len(wanted)) || len(result.Items) != len(wanted) {
			t.Fatalf("room member count=%d/%d, want %d", result.Total, len(result.Items), len(wanted))
		}
		seen := map[uint64]bool{}
		for _, item := range result.Items {
			inRoom, exists := wanted[item.ID]
			if !exists || seen[item.ID] || item.InCurrentRoom != inRoom || item.CanManage != inRoom {
				t.Fatalf("unrelated, duplicate or incorrectly scoped member: %+v", item)
			}
			seen[item.ID] = true
			assertWorkspaceMemberNoDestination(t, workspaceMemberJSON(t, item))
		}
	})

	t.Run("historical members expose public identity but no destination account data", func(t *testing.T) {
		result := list(room.ID, UserListFilter{Page: 1, PageSize: 100})
		for _, account := range []user.User{historical, historicalDisabled} {
			item := find(result, account.UserID)
			if item.Username != account.Username || item.Nickname != account.Nickname || item.PublicID != account.PublicID || item.Avatar != account.Avatar {
				t.Fatalf("historical public identity missing: %+v", item)
			}
			payload := workspaceMemberJSON(t, item)
			for _, field := range []string{"balance", "status", "online", "phone", "remark", "risk_level", "fly_mode", "fly_rate", "login_count", "last_login_at", "created_at"} {
				if value := payload[field]; value != nil {
					t.Fatalf("historical member %d leaks %s=%#v", account.UserID, field, value)
				}
			}
			for _, field := range []string{"balance", "status", "online"} {
				if _, exists := payload[field]; !exists {
					t.Fatalf("historical %s must be explicit null", field)
				}
			}
		}
		item := find(result, current.UserID)
		if item.Balance == nil || *item.Balance != 1234.56 || item.Status == nil || *item.Status != 1 || item.Online == nil {
			t.Fatalf("current member lost existing account details: %+v", item)
		}
		payload := workspaceMemberJSON(t, item)
		if payload["phone"] != current.Phone || payload["remark"] != current.Remark || payload["risk_level"] != "watch" || payload["login_count"] != float64(23) {
			t.Fatalf("current detail data missing: %#v", payload)
		}
	})

	t.Run("search is public identity only and pagination is duplicate free", func(t *testing.T) {
		for _, query := range []string{historical.Username, historical.Nickname, strconv.FormatUint(historical.PublicID, 10)} {
			result := list(room.ID, UserListFilter{Query: query, Page: 1, PageSize: 100})
			// The username also prefixes wm_historical_disabled, so presence, not
			// exact cardinality, is the contract for these substring searches.
			find(result, historical.UserID)
		}
		for _, query := range []string{historical.Phone, historical.Email, historical.Remark, current.Phone, current.Email, current.Remark, unknown.Username} {
			if result := list(room.ID, UserListFilter{Query: query}); result.Total != 0 || len(result.Items) != 0 {
				t.Fatalf("private or unjoined query %q matched %+v", query, result)
			}
		}
		all := list(room.ID, UserListFilter{Page: 1, PageSize: 100})
		var paged []uint64
		for page := 1; page <= 3; page++ {
			result := list(room.ID, UserListFilter{Page: page, PageSize: 2})
			if result.Total != all.Total || len(result.Items) != 2 {
				t.Fatalf("page %d has inconsistent total/size: %+v", page, result)
			}
			for _, item := range result.Items {
				paged = append(paged, item.ID)
			}
		}
		for i, item := range all.Items {
			if paged[i] != item.ID {
				t.Fatalf("pagination is unstable or duplicates members: %v", paged)
			}
		}
		if result := list(room.ID, UserListFilter{Page: 4, PageSize: 2}); result.Total != all.Total || len(result.Items) != 0 {
			t.Fatalf("past-end page is incorrect: %+v", result)
		}
	})

	t.Run("exact member lookup remains inside the authenticated room", func(t *testing.T) {
		for _, fixture := range []struct {
			name    string
			account user.User
			current bool
		}{
			{name: "current", account: current, current: true},
			{name: "historical", account: historical, current: false},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				result := list(room.ID, UserListFilter{UserID: fixture.account.UserID, WorkspaceID: otherRoom.ID, Page: 1, PageSize: 1})
				if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != fixture.account.UserID {
					t.Fatalf("exact lookup did not isolate member %d: %+v", fixture.account.UserID, result)
				}
				item := result.Items[0]
				if item.InCurrentRoom != fixture.current || item.CanManage != fixture.current {
					t.Fatalf("exact lookup changed room permissions: %+v", item)
				}
				if !fixture.current && (item.Balance != nil || item.Status != nil || item.Online != nil) {
					t.Fatalf("exact historical lookup exposed current account data: %+v", item)
				}
				if fixture.current && (item.Balance == nil || item.Status == nil || item.Online == nil) {
					t.Fatalf("exact current lookup omitted manageable account data: %+v", item)
				}
				assertWorkspaceMemberNoDestination(t, workspaceMemberJSON(t, item))
			})
		}
		for _, userID := range []uint64{unknown.UserID, 9_007_199_254_740_991} {
			result := list(room.ID, UserListFilter{UserID: userID, WorkspaceID: otherRoom.ID, Page: 1, PageSize: 1})
			if result.Total != 0 || len(result.Items) != 0 {
				t.Fatalf("forged filter room or unknown member id crossed ownership: %+v", result)
			}
		}
		// Zero is the service's omitted-filter value. HTTP handlers separately
		// reject an explicitly supplied zero instead of interpreting it as an ID.
		if result := list(room.ID, UserListFilter{UserID: 0, Page: 1, PageSize: 100}); result.Total != int64(len(wanted)) {
			t.Fatalf("omitted exact-id filter changed roster size: %+v", result)
		}
	})

	t.Run("account status filters cannot disclose another room state", func(t *testing.T) {
		for _, fixture := range []struct {
			status string
			ids    map[uint64]bool
		}{
			{status: "active", ids: map[uint64]bool{current.UserID: true, legacy.UserID: true, prefixHuman.UserID: true}},
			{status: "disabled", ids: map[uint64]bool{disabled.UserID: true}},
		} {
			result := list(room.ID, UserListFilter{Status: fixture.status, Page: 1, PageSize: 100})
			if result.Total != int64(len(fixture.ids)) || len(result.Items) != len(fixture.ids) {
				t.Fatalf("%s filter leaks historical state: %+v", fixture.status, result)
			}
			for _, item := range result.Items {
				if !fixture.ids[item.ID] || !item.InCurrentRoom {
					t.Fatalf("%s returned other-room member: %+v", fixture.status, item)
				}
			}
		}
	})

	t.Run("agent uses exact workspace rather than tenant tree or supplied filter", func(t *testing.T) {
		result, err := NewAgentWorkspaceService(db).Users(agentRoom.OwnerUserID, UserListFilter{WorkspaceID: otherRoom.ID, Page: 1, PageSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 2 || len(result.Items) != 2 {
			t.Fatalf("agent membership scope crossed rooms: %+v", result)
		}
		if !find(result, agentCurrent.UserID).CanManage || find(result, agentHistorical.UserID).CanManage {
			t.Fatalf("agent current/history management boundaries are incorrect: %+v", result)
		}
		if result := list(otherRoom.ID, UserListFilter{Query: current.Username}); result.Total != 0 {
			t.Fatalf("unrelated tenant can list current member: %+v", result)
		}
	})

	t.Run("room headcounts match the roster without treating history as currently active", func(t *testing.T) {
		counts := map[uint64]int64{}
		for _, target := range []workspacemodel.Workspace{room, otherRoom, agentRoom, otherAgentRoom} {
			roster := list(target.ID, UserListFilter{Page: 1, PageSize: 100})
			active := list(target.ID, UserListFilter{Status: "active", Page: 1, PageSize: 100})
			counts[target.OwnerUserID] = roster.Total
			dashboard, err := NewAgentWorkspaceService(db).DashboardForWorkspace(target.ID)
			if err != nil || dashboard.MemberCount != roster.Total || dashboard.ActiveMemberCount != active.Total {
				t.Fatalf("room %s dashboard differs from roster: dashboard=%+v, total=%d, active=%d, err=%v", target.RoomCode, dashboard, roster.Total, active.Total, err)
			}
			if target.Type == workspacemodel.TypeTenant {
				tenantDashboard, err := NewTenantAdminService(db).Dashboard(target.OwnerUserID)
				if err != nil || tenantDashboard["member_count"] != roster.Total {
					t.Fatalf("tenant %s count differs from roster: %+v, err=%v", target.RoomCode, tenantDashboard, err)
				}
				var owner user.User
				if err := db.First(&owner, target.OwnerUserID).Error; err != nil {
					t.Fatal(err)
				}
				view, err := NewTenantAdminService(db).toView(owner)
				if err != nil || view.MemberCount != roster.Total {
					t.Fatalf("tenant %s view differs from roster: %+v, err=%v", target.RoomCode, view, err)
				}
			} else {
				view, err := NewAgentAdminService(db).view(target.OwnerUserID)
				if err != nil || view.MemberCount != roster.Total {
					t.Fatalf("agent %s view differs from roster: %+v, err=%v", target.RoomCode, view, err)
				}
			}
		}
		agents, err := NewAgentAdminService(db).List("", 1, 100)
		if err != nil || agents.Total != 2 {
			t.Fatalf("agent listing failed: %+v, err=%v", agents, err)
		}
		for _, item := range agents.Items {
			if item.MemberCount != counts[item.ID] {
				t.Fatalf("agent list headcount differs from room roster: %+v", item)
			}
		}
		tenants, err := NewTenantAdminService(db).List("", 1, 100)
		if err != nil || tenants.Total != 2 {
			t.Fatalf("tenant listing failed: %+v, err=%v", tenants, err)
		}
		for _, item := range tenants.Items {
			if item.MemberCount != counts[item.ID] {
				t.Fatalf("tenant list headcount differs from room roster: %+v", item)
			}
		}
	})

	t.Run("listing never reactivates historical memberships", func(t *testing.T) {
		var after []workspacemodel.Membership
		if err := db.Order("id").Find(&after).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(membershipsBefore, after) {
			t.Fatal("member listing changed membership state or timestamps")
		}
		var account user.User
		if err := db.First(&account, historical.UserID).Error; err != nil || account.WorkspaceID != otherRoom.ID {
			t.Fatalf("listing changed current room: %+v, err=%v", account, err)
		}
	})

	t.Run("historical visibility does not authorize cross room mutations", func(t *testing.T) {
		assertDenied := func(label string, err error) {
			t.Helper()
			if err == nil || !apperrors.IsBusinessError(err) {
				t.Fatalf("%s was not rejected as an ownership error: %v", label, err)
			}
		}
		_, err := NewUserAdminService(db).SetStatusInWorkspace(historical.UserID, room.ID, 0)
		assertDenied("tenant historical status", err)
		_, err = NewUserAdminService(db).AdjustBalanceInWorkspace(historical.UserID, room.ID, 1, "must not apply", "fixture")
		assertDenied("tenant historical balance", err)
		_, err = NewTradingAdminService(db).GetForWorkspace(room.ID, historical.UserID, "speed-racing")
		assertDenied("tenant historical trading read", err)
		_, err = NewTradingAdminService(db).UpdateForWorkspace(room.ID, historical.UserID, UpdateUserTradingInput{})
		assertDenied("tenant historical trading update", err)

		// Simulate a legacy stale parent pointer. Immutable current workspace is
		// still authoritative even if the old agent id was not cleared.
		if err := db.Model(&agentHistorical).Update("parent_agent_id", agentRoom.OwnerUserID).Error; err != nil {
			t.Fatal(err)
		}
		_, err = NewAgentWorkspaceService(db).SetUserStatus(agentRoom.OwnerUserID, agentHistorical.UserID, 0)
		assertDenied("agent stale parent status", err)
		_, err = NewAgentWorkspaceService(db).AdjustBalance(agentRoom.OwnerUserID, agentHistorical.UserID, 1, "must not apply", "fixture")
		assertDenied("agent stale parent balance", err)
		for _, before := range []user.User{historical, agentHistorical} {
			var after user.User
			if err := db.First(&after, before.UserID).Error; err != nil {
				t.Fatal(err)
			}
			if after.Status != before.Status || after.BalanceCents != before.BalanceCents || after.WorkspaceID != before.WorkspaceID {
				t.Fatalf("denied operation changed account: before=%+v, after=%+v", before, after)
			}
			var ledgers int64
			if err := db.Model(&user.BalanceTransaction{}).Where("user_id = ?", before.UserID).Count(&ledgers).Error; err != nil || ledgers != 0 {
				t.Fatalf("denied mutation wrote ledger: count=%d, err=%v", ledgers, err)
			}
		}
	})

	t.Run("historical member can return without another approval", func(t *testing.T) {
		if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", room.ID).
			Updates(map[string]any{"room_enabled": true, "require_join_review": true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&workspacemodel.Membership{}).Where("workspace_id = ? AND user_id = ?", room.ID, historical.UserID).
			Update("odds_multiplier", 1.2).Error; err != nil {
			t.Fatal(err)
		}
		joined, err := NewMemberService(db).JoinRoom(historical.UserID, room.RoomCode, "wm-return-existing-member")
		if err != nil || joined.Status != "joined" {
			t.Fatalf("historical reentry requested another approval: result=%+v, err=%v", joined, err)
		}
		var active []workspacemodel.Membership
		if err := db.Where("user_id = ? AND status = 1", historical.UserID).Find(&active).Error; err != nil {
			t.Fatal(err)
		}
		if len(active) != 1 || active[0].WorkspaceID != room.ID || active[0].OddsMultiplier != 1.2 {
			t.Fatalf("reentry changed membership invariant or room odds: %+v", active)
		}
		var newRequests int64
		if err := db.Model(&application.Application{}).Where("user_id = ? AND workspace_id = ?", historical.UserID, room.ID).Count(&newRequests).Error; err != nil || newRequests != 0 {
			t.Fatalf("reentry created new request: %d, err=%v", newRequests, err)
		}
		back := find(list(room.ID, UserListFilter{Query: historical.Username}), historical.UserID)
		if !back.InCurrentRoom || !back.CanManage || back.Balance == nil {
			t.Fatalf("returned member still shown as historical: %+v", back)
		}
		former := find(list(otherRoom.ID, UserListFilter{Query: historical.Username}), historical.UserID)
		if former.InCurrentRoom || former.CanManage || former.Balance != nil || former.Status != nil || former.Online != nil {
			t.Fatalf("previous room retained current account details: %+v", former)
		}
	})
}
