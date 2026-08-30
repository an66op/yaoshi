package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func testSiteFixture() TestSiteAccountsConfig {
	return TestSiteAccountsConfig{
		Site:     "fixture.example",
		Platform: TestSiteCredential{Username: "testadmin", Password: "V7!mQ2#zL9@pR4$x"},
		Tenant:   TestSiteRoomAccount{TestSiteCredential: TestSiteCredential{Username: "testtenant", Password: "T8!mQ2#zL9@pR4$x"}, RoomCode: "88002", RoomName: "测试租户房"},
		Agent:    TestSiteRoomAccount{TestSiteCredential: TestSiteCredential{Username: "testagent", Password: "A9!mQ2#zL9@pR4$x"}, RoomCode: "88001", RoomName: "测试代理房"},
		Member:   TestSiteMemberAccount{TestSiteCredential: TestSiteCredential{Username: "testmember", Password: "M6!mQ2#zL9@pR4$x"}, RoomCode: "88001"},
	}
}

func TestTestSiteAccountsValidateAllConfigurationBeforeDatabaseAccess(t *testing.T) {
	if err := ValidateTestSiteAccountsConfig(testSiteFixture()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*TestSiteAccountsConfig){
		"site-url":            func(c *TestSiteAccountsConfig) { c.Site = "https://fixture.example" },
		"site-label":          func(c *TestSiteAccountsConfig) { c.Site = "-fixture.example" },
		"same-user":           func(c *TestSiteAccountsConfig) { c.Member.Username = "TESTADMIN" },
		"same-password":       func(c *TestSiteAccountsConfig) { c.Member.Password = c.Platform.Password },
		"weak-password":       func(c *TestSiteAccountsConfig) { c.Member.Password = "12345678" },
		"oversize-password":   func(c *TestSiteAccountsConfig) { c.Member.Password = strings.Repeat("M!9a", 25) },
		"whitespace-username": func(c *TestSiteAccountsConfig) { c.Agent.Username = " agent " },
		"reserved-username":   func(c *TestSiteAccountsConfig) { c.Agent.Username = "room_robot_11" },
		"control-character":   func(c *TestSiteAccountsConfig) { c.Platform.Username = "test\nadmin" },
		"room-conflict":       func(c *TestSiteAccountsConfig) { c.Tenant.RoomCode = c.Agent.RoomCode },
		"member-foreign-room": func(c *TestSiteAccountsConfig) { c.Member.RoomCode = "99999" },
		"room-name-required":  func(c *TestSiteAccountsConfig) { c.Agent.RoomName = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := testSiteFixture()
			mutate(&cfg)
			if err := ValidateTestSiteAccountsConfig(cfg); err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if _, err := ProvisionTestSiteAccounts(nil, cfg); err == nil || err.Error() == "数据库连接不可用" {
				t.Fatal("database was accessed before validating the full configuration")
			}
		})
	}
}

func TestTestSiteAccountsPostgresProvisionRepeatAndOwnership(t *testing.T) {
	db := timingPostgresDatabase(t)
	cfg := testSiteFixture()
	var original user.User
	if err := db.Where("username = ?", "timing_platform").First(&original).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProvisionTestSiteAccounts(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Platform.UserID == original.UserID || result.Tenant.RoomCode != "88002" || result.Agent.RoomCode != "88001" || result.Member.WorkspaceID != result.Agent.WorkspaceID {
		t.Fatal("incorrect hierarchy", result)
	}
	ids := []uint64{result.Platform.UserID, result.Tenant.UserID, result.Agent.UserID, result.Member.UserID}
	var before, after []user.User
	if err := db.Where("user_id IN ?", ids).Order("user_id").Find(&before).Error; err != nil || len(before) != 4 {
		t.Fatal("missing test identities", err)
	}
	for index, account := range before {
		credential := []TestSiteCredential{cfg.Platform, cfg.Tenant.TestSiteCredential, cfg.Agent.TestSiteCredential, cfg.Member.TestSiteCredential}[index]
		if account.BalanceCents != 0 || !utils.CheckPasswordHash(credential.Password, account.Password) || account.AuthVersion <= 1 {
			t.Fatal("initial account credentials or balance are incorrect")
		}
	}
	for _, table := range []string{"lottery_draws", "lottery_bets", "plan_recommendations", "plan_streams"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("initializer wrote %s: %d, %v", table, count, err)
		}
	}
	var count int64
	if err := db.Table("room_game_settings").Where("workspace_id IN ? AND enabled = true", []uint64{result.Tenant.WorkspaceID, result.Agent.WorkspaceID}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("test account creation changed closed game defaults", count, err)
	}
	if err := db.Table("workspace_robot_settings").Where("workspace_id IN ? AND enabled = true", []uint64{result.Tenant.WorkspaceID, result.Agent.WorkspaceID}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("test account creation enabled robots", count, err)
	}
	if err := db.Model(&user.BalanceTransaction{}).Where("user_id IN ?", ids).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("test identities have unexpected financial records", count, err)
	}
	var membership workspacemodel.Membership
	if err := db.Where("user_id = ? AND workspace_id = ? AND status = 1", result.Member.UserID, result.Agent.WorkspaceID).First(&membership).Error; err != nil {
		t.Fatal("member not enrolled in its test room", err)
	}
	repeated, err := ProvisionTestSiteAccounts(db, cfg)
	if err != nil || !reflect.DeepEqual(result, repeated) {
		t.Fatal("repeat not idempotent", err)
	}
	if err := db.Where("user_id IN ?", ids).Order("user_id").Find(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("repeat modified existing accounts", err)
	}
	var primaryAfter user.User
	if err := db.First(&primaryAfter, original.UserID).Error; err != nil || primaryAfter.Password != original.Password || primaryAfter.AuthVersion != original.AuthVersion || primaryAfter.Role != original.Role || primaryAfter.Status != original.Status {
		t.Fatal("original administrator was changed", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), cfg.Platform.Password) || strings.Contains(string(encoded), cfg.Member.Username) {
		t.Fatal("result exposed credentials", err)
	}
	t.Run("modified password is not reset", func(t *testing.T) {
		changed := cfg
		changed.Member.Password = "Changed#Fixture78zQ"
		if _, err := ProvisionTestSiteAccounts(db, changed); err == nil {
			t.Fatal("credential replacement accepted")
		}
	})
	t.Run("different site cannot claim identities", func(t *testing.T) {
		changed := cfg
		changed.Site = "other.example"
		if _, err := ProvisionTestSiteAccounts(db, changed); err == nil {
			t.Fatal("foreign marker accepted")
		}
	})
	t.Run("database rejects additional active room", func(t *testing.T) {
		extra := workspacemodel.Membership{UserID: result.Member.UserID, WorkspaceID: result.Tenant.WorkspaceID, Role: "member", Status: 1}
		if err := db.Transaction(func(tx *gorm.DB) error { return tx.Create(&extra).Error }); err == nil {
			t.Fatal("database accepted ambiguous active memberships")
		}
		if _, err := ProvisionTestSiteAccounts(db, cfg); err != nil {
			t.Fatal("failed extra membership changed the original room", err)
		}
	})
	t.Run("disabled user is not resurrected", func(t *testing.T) {
		if err := db.Model(&user.User{}).Where("user_id = ?", result.Member.UserID).Update("status", 0).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := ProvisionTestSiteAccounts(db, cfg); err == nil {
			t.Fatal("disabled identity resurrected")
		}
	})
}

func TestTestSiteAccountsPostgresConflictsAndAtomicRollback(t *testing.T) {
	db := timingPostgresDatabase(t)
	cfg := testSiteFixture()
	// A failure at the final member insert must roll back the previously
	// created admin, tenant, agent and their room defaults as a single action.
	if err := db.Exec(`CREATE FUNCTION fail_test_site_member() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.username = 'testmember' THEN RAISE EXCEPTION 'fixture insertion refused'; END IF; RETURN NEW; END $$;
		CREATE TRIGGER fail_test_site_member BEFORE INSERT ON "user" FOR EACH ROW EXECUTE FUNCTION fail_test_site_member()`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ProvisionTestSiteAccounts(db, cfg); err == nil {
		t.Fatal("injected failure was not propagated")
	}
	var count int64
	if err := db.Model(&user.User{}).Where("remark LIKE ?", "test-site-accounts:v1:%").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed operation partially created identities", count, err)
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("room_code IN ?", []string{"88001", "88002"}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed operation partially created rooms", count, err)
	}
	if err := db.Exec(`DROP TRIGGER fail_test_site_member ON "user"; DROP FUNCTION fail_test_site_member()`).Error; err != nil {
		t.Fatal(err)
	}
	var primary user.User
	if err := db.Where("username = ?", "timing_platform").First(&primary).Error; err != nil {
		t.Fatal(err)
	}
	human := user.User{Username: cfg.Platform.Username, Password: primary.Password, Role: "admin", LoginScope: platformLoginScope, WorkspaceID: primary.WorkspaceID, Status: 1}
	if err := db.Create(&human).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ProvisionTestSiteAccounts(db, cfg); err == nil {
		t.Fatal("unmarked human account was claimed")
	}
	cfg.Platform.Username = "another_testadmin"
	timingPostgresRoom(t, db, "existing_room_owner", "88001")
	if _, err := ProvisionTestSiteAccounts(db, cfg); err == nil {
		t.Fatal("unrelated room was claimed")
	}
	if err := db.Model(&user.User{}).Where("remark LIKE ?", "test-site-accounts:v1:%").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("preflight conflict created identities", count, err)
	}
}
