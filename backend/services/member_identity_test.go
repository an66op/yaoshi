package services

import (
	"backend/data/models/chat"
	membernotify "backend/data/models/notify"
	"backend/data/models/settings"
	"backend/data/models/user"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func identityDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNormalizeMemberAvatarAcceptsOnlyLocalOrImageData(t *testing.T) {
	for _, value := range []string{
		"", "/images/avatars/avatar-anime-03.png", "data:image/webp;base64,AAAA",
	} {
		got, err := normalizeMemberAvatar(value)
		if err != nil || got != value {
			t.Fatalf("normalizeMemberAvatar(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{
		"https://example.com/avatar.png", "/images/avatars/../secret.png",
		"/images/avatars/nested/avatar.png", "data:image/svg+xml;base64,PHN2Zz4=",
	} {
		if _, err := normalizeMemberAvatar(value); err == nil {
			t.Fatalf("normalizeMemberAvatar accepted unsafe value %q", value)
		}
	}
}

func TestScopedChatMessageQueryIncludesWorkspaceBoundary(t *testing.T) {
	db := identityDryRunDB(t)
	var rows []chat.Message
	statement := scopedChatMessageQuery(db, 37, "group", "agent:9", "agent:9", "speed-racing").Find(&rows).Statement
	sql := statement.SQL.String()
	if !strings.Contains(sql, "workspace_id =") {
		t.Fatalf("chat query omitted workspace boundary: %s", sql)
	}
	want := []any{uint64(37), "group", "agent:9", "agent:9", "speed-racing"}
	if len(statement.Vars) < len(want) {
		t.Fatalf("chat query vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("chat query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestHistoricalChatIdentityUsesRoomMembershipNotCurrentWorkspace(t *testing.T) {
	db := identityDryRunDB(t)
	var accounts []user.User
	statement := historicalChatMemberIdentityQuery(db, 37, []uint64{12}).Scan(&accounts).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"workspace_memberships", "membership.user_id = account.user_id",
		"membership.workspace_id =", "account.user_id IN",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("historical identity query omitted %q: %s", fragment, sql)
		}
	}
	if strings.Contains(sql, "account.workspace_id =") {
		t.Fatalf("historical identity was tied to the member's current room: %s", sql)
	}
	if len(statement.Vars) < 2 || statement.Vars[0] != uint64(37) {
		t.Fatalf("historical identity query has wrong room membership vars: %#v", statement.Vars)
	}
}

func TestClaimableLobbyRedPacketQueryIsUserRoomScoped(t *testing.T) {
	db := identityDryRunDB(t)
	now := time.Date(2026, 8, 28, 5, 0, 0, 0, time.UTC)
	var row chat.Message
	statement := claimableLobbyRedPacketQuery(db, 37, 12, "agent:9", "agent:9", now).
		Order("message.created_at DESC, message.id DESC").Take(&row).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"message.workspace_id =", "message.room_type =", "message.scope =", "message.room_scope =", "message.game_id =",
		"packet.workspace_id = message.workspace_id", "packet.scope = message.scope", "packet.room_scope = message.room_scope",
		"packet.status =", "packet.remaining_cents > 0", "packet.claimed_count < packet.packet_count",
		"claim.workspace_id =", "claim.packet_id = packet.id", "claim.user_id =",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("claimable packet query omitted %q: %s", fragment, sql)
		}
	}
	if len(statement.Vars) < 10 || statement.Vars[0] != uint64(37) || statement.Vars[2] != "agent:9" || statement.Vars[3] != "agent:9" {
		t.Fatalf("claimable packet query has wrong room boundary: %#v", statement.Vars)
	}
	if statement.Vars[len(statement.Vars)-3] != uint64(37) || statement.Vars[len(statement.Vars)-2] != uint64(12) {
		t.Fatalf("claim exclusion has wrong workspace/user boundary: %#v", statement.Vars)
	}
}

func TestRedPacketStateClaimableRejectsClosedClaimedAndExpired(t *testing.T) {
	now := time.Date(2026, 8, 28, 5, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	row := chat.Message{RedPacketCount: 3}
	active := redPacketViewState{
		Status: "active", FundingStatus: "reserved", RemainingCents: 300,
		ClaimedCount: 1, ExpiresAt: &expires,
	}
	if !redPacketStateClaimable(row, active, now) {
		t.Fatal("open unclaimed packet was rejected")
	}
	cases := []redPacketViewState{
		{Status: "empty", FundingStatus: "released", RemainingCents: 0, ClaimedCount: 3},
		{Status: "active", FundingStatus: "reserved", RemainingCents: 100, ClaimedCount: 3},
		{Status: "active", FundingStatus: "refunded", RemainingCents: 100, ClaimedCount: 1},
		{Status: "active", FundingStatus: "reserved", RemainingCents: 100, ClaimedCount: 1, RefundedCents: 1},
		{Status: "active", FundingStatus: "reserved", RemainingCents: 100, ClaimedCount: 1, ExpiresAt: &now},
	}
	for _, state := range cases {
		if redPacketStateClaimable(row, state, now) {
			t.Fatalf("unclaimable packet state was accepted: %#v", state)
		}
	}
}

func TestUpdatedRedPacketRecipientQueryRequiresWorkspace(t *testing.T) {
	db := identityDryRunDB(t)
	query, err := chatScopeRecipientsForWorkspaceQuery(db, 37, "agent:9")
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint64
	statement := query.Pluck("user_id", &ids).Statement
	if !strings.Contains(statement.SQL.String(), "workspace_id =") {
		t.Fatalf("updated red-packet recipients are not workspace scoped: %s", statement.SQL.String())
	}
	if len(statement.Vars) < 4 || statement.Vars[0] != 1 || statement.Vars[1] != uint64(37) || statement.Vars[2] != uint64(9) || statement.Vars[3] != uint64(9) {
		t.Fatalf("updated red-packet recipient boundary is wrong: %#v", statement.Vars)
	}
}

func TestRedPacketUpdateLookupJoinsFrozenWorkspaceAndRoom(t *testing.T) {
	db := identityDryRunDB(t)
	var row chat.Message
	statement := redPacketMessageByPacketQuery(db, 81).Take(&row).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"packet.message_id = message.id", "packet.workspace_id = message.workspace_id",
		"packet.scope = message.scope", "packet.room_scope = message.room_scope", "packet.game_id = message.game_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("red-packet update lookup omitted %q: %s", fragment, sql)
		}
	}
}

func TestUnreadNotificationQueryIsWorkspaceScopedAndVisibleOnly(t *testing.T) {
	db := identityDryRunDB(t)
	var rows []membernotify.MemberNotification
	statement := unreadMemberNotificationQuery(db, 12, 37).Find(&rows).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{"user_id =", "workspace_id =", "category IN", "title <>"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("unread query omitted %q: %s", fragment, sql)
		}
	}
	if len(statement.Vars) < 5 || statement.Vars[0] != uint64(12) || statement.Vars[1] != uint64(37) {
		t.Fatalf("unread query has wrong scope vars: %#v", statement.Vars)
	}
}

func TestChatViewsExposeResolvedPublicIdentity(t *testing.T) {
	row := chat.Message{ID: 11, UserID: 5, Nickname: "晴天", RoomType: "group", RoomScope: "agent:9", GameID: "speed-racing"}
	identity := chatMemberIdentity{PublicID: 7000005, Avatar: "/images/avatars/avatar-1.jpg", Title: "幸运星", Badge: "会员"}
	view := chatMessageView(row, 5, identity, 0, redPacketViewState{})
	if view.PublicID != identity.PublicID || view.Avatar != identity.Avatar || view.Title != identity.Title || view.Badge != identity.Badge || !view.Mine {
		t.Fatalf("member chat identity not mapped: %#v", view)
	}
	adminView := adminChatMessage(row)
	applyAdminChatIdentity(&adminView, identity)
	if adminView.PublicID != identity.PublicID || adminView.Avatar != identity.Avatar || adminView.Title != identity.Title || adminView.Badge != identity.Badge {
		t.Fatalf("admin chat identity not mapped: %#v", adminView)
	}
}

func TestSettingsAndAdminUserExposeConfiguredIdentity(t *testing.T) {
	cfg := toSettingsView(&settings.SystemConfig{RoomName: "永生", ChatNickname: "开奖助手", ChatAvatar: "/images/avatars/avatar-2.jpg"})
	if cfg.RoomName != "永生" || cfg.ChatNickname != "开奖助手" || cfg.ChatAvatar != "/images/avatars/avatar-2.jpg" {
		t.Fatalf("room operator identity not exposed: %#v", cfg)
	}
	account := adminUser(user.User{UserID: 7, PublicID: 7000007, Avatar: "/images/avatars/avatar-3.jpg", PublicTitle: "房间达人", PublicBadge: "VIP"})
	if account.Avatar == "" || account.PublicTitle != "房间达人" || account.PublicBadge != "VIP" {
		t.Fatalf("admin member identity not exposed: %#v", account)
	}
}
