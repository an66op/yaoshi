package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func pc28PostgresFixture(t *testing.T, db *gorm.DB, gameID, issue string, grayPush bool) (*lottery.Game, user.User, workspacemodel.Workspace) {
	t.Helper()
	suffix := strings.NewReplacer("-", "_", ".", "_").Replace(gameID)
	room := timingPostgresRoom(t, db, "pc28_"+suffix, fmt.Sprintf("79%04d", len(gameID)*113))
	member := timingPostgresMember(t, db, room, "pc28_member_"+suffix)
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, gameID, true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, fmt.Sprintf(`{"seal_seconds":30,"pc28_gray_push":%t}`, grayPush))
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.Model(&lottery.Game{}).Where("id = ?", gameID).Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok",
		"last_sync_error": "", "last_sync_at": now, "next_issue": issue,
		// Financial assertions need clock headroom; cadence itself is covered by
		// the timing suite and must not make this fixture's current issue stale.
		"next_draw_at": now.Add(10 * time.Minute), "draw_interval": 3600,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", gameID).Error; err != nil {
		t.Fatal(err)
	}
	limits, err := NewOddsAdminService(db).Get(gameID)
	if err != nil || !limits.RulesReady {
		t.Fatalf("load PC28 catalog: %+v / %v", limits, err)
	}
	for index := range limits.Items {
		limits.Items[index].Odds = 2.8
		limits.Items[index].MinBet = 0.01
		limits.Items[index].MaxBet = 50000
		limits.Items[index].MaxUserPeriod = 50000
		limits.Items[index].MaxPeriodTotal = 100000
	}
	if _, err := NewOddsAdminService(db).Update(gameID, oddsUpdateInput(limits)); err != nil {
		t.Fatal("configure explicit PC28 test odds:", err)
	}
	return &game, member, room
}

func TestPC28WebPostgresEnforcesPointLimitOppositeSidesAndMissingOdds(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member, _ := pc28PostgresFixture(t, db, "pc-canada", "988001", false)
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true

	points := make([]WebBetItem, 0, 10)
	for total := 0; total < 10; total++ {
		points = append(points, WebBetItem{PlayCode: pc28ExactCode(total), Position: 0, Selection: fmt.Sprint(total), Amount: 1})
	}
	before := timingPostgresMoney(t, db, member.UserID)
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, points, member.Username, "pc28-points-first-001")
	if err != nil || receipt.BetCount != 10 || receipt.RuleVersion != pc28RuleV1 || receipt.Total != 10 {
		t.Fatalf("place first ten PC28 points: %+v / %v", receipt, err)
	}
	afterTen := timingPostgresMoney(t, db, member.UserID)
	if afterTen.BalanceCents != before.BalanceCents-1000 || afterTen.Bets != before.Bets+10 || afterTen.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("first PC28 ticket was not atomic: before=%+v after=%+v", before, afterTen)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: pc28ExactCode(10), Position: 0, Selection: "10", Amount: 1,
	}}, member.Username, "pc28-point-eleven-001"); apperrors.GetErrorCode(err) != "POINT_LIMIT_EXCEEDED" {
		t.Fatalf("eleventh distinct point returned %v", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != afterTen {
		t.Fatalf("rejected eleventh point changed funds: before=%+v after=%+v", afterTen, after)
	}
	// Repeating an already used point does not consume another distinct slot.
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: pc28ExactCode(0), Position: 0, Selection: "0", Amount: 1,
	}}, member.Username, "pc28-point-repeat-001"); err != nil {
		t.Fatal("existing point was counted twice:", err)
	}

	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: pc28SumSize, Position: 0, Selection: "大", Amount: 1,
	}}, member.Username, "pc28-side-big-0001"); err != nil {
		t.Fatal(err)
	}
	afterBig := timingPostgresMoney(t, db, member.UserID)
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: pc28SumSize, Position: 0, Selection: "小", Amount: 1,
	}}, member.Username, "pc28-side-small-01"); apperrors.GetErrorCode(err) != "OPPOSITE_BET_NOT_ALLOWED" {
		t.Fatalf("pc28-v1 opposite side returned %v", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != afterBig {
		t.Fatalf("opposite-side rejection changed funds: before=%+v after=%+v", afterBig, after)
	}

	if err := db.Where("game_id = ? AND play_code = ?", game.ID, pc28ColorRed).Delete(&odds.PlayLimit{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: pc28ColorRed, Position: 0, Selection: "红波", Amount: 1,
	}}, member.Username, "pc28-missing-odds-1"); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("missing atomic PC28 odds returned %v", err)
	}
}

func TestPC28V3WebPostgresAllowsOppositeSumSides(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member, _ := pc28PostgresFixture(t, db, "canada-20", "988101", false)
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{
		{PlayCode: pc28SumSize, Position: 0, Selection: "大", Amount: 1},
		{PlayCode: pc28SumSize, Position: 0, Selection: "小", Amount: 1},
	}, member.Username, "pc28-v3-opposites-01")
	if err != nil || receipt.BetCount != 2 || receipt.RuleVersion != pc28RuleV3 {
		t.Fatalf("pc28-v3 inherited v1/v2 reverse restriction: %+v / %v", receipt, err)
	}
}

func TestPC28PointAndReverseLimitsAreScopedToCurrentRoom(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member, _ := pc28PostgresFixture(t, db, "pc-canada", "988151", false)
	oldRoom := timingPostgresRoom(t, db, "pc28_old_room", "799151")
	oldRows := make([]bet.Bet, 0, 11)
	for total := 0; total < 10; total++ {
		amount := int64(100)
		oldRows = append(oldRows, bet.Bet{
			WorkspaceID: oldRoom.ID, RoomScope: oldRoom.Scope, GameID: game.ID, Issue: game.NextIssue,
			UserID: member.UserID, Username: member.Username, PlayCode: pc28ExactCode(total), PlayName: "历史房单点",
			Position: 0, Selection: fmt.Sprint(total), RequestReference: fmt.Sprintf("old-room-point-%d", total),
			RuleVersion: pc28RuleV1, AmountCents: amount, ValidTurnoverCents: &amount, Odds: 2.8, Status: "pending",
		})
	}
	amount := int64(100)
	oldRows = append(oldRows, bet.Bet{
		WorkspaceID: oldRoom.ID, RoomScope: oldRoom.Scope, GameID: game.ID, Issue: game.NextIssue,
		UserID: member.UserID, Username: member.Username, PlayCode: pc28SumSize, PlayName: "历史房大小",
		Position: 0, Selection: "小", RequestReference: "old-room-side-small",
		RuleVersion: pc28RuleV1, AmountCents: amount, ValidTurnoverCents: &amount, Odds: 2.8, Status: "pending",
	})
	if err := db.Create(&oldRows).Error; err != nil {
		t.Fatal(err)
	}
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true
	// Neither ten old-room points nor the old-room opposite side belongs to the
	// current room's constraint set.
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{
		{PlayCode: pc28ExactCode(10), Position: 0, Selection: "10", Amount: 1},
		{PlayCode: pc28SumSize, Position: 0, Selection: "大", Amount: 1},
	}, member.Username, "pc28-room-scope-0001")
	if err != nil || receipt.BetCount != 2 {
		t.Fatalf("old room polluted current PC28 constraints: %+v / %v", receipt, err)
	}
}

func TestPC28V1PostgresSettlementFreezesGrayRuleAndZeroesAll1314Turnover(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member, room := pc28PostgresFixture(t, db, "pc-canada", "988201", true)
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{
		{PlayCode: pc28SumSize, Position: 0, Selection: "大", Amount: 1},
		{PlayCode: pc28ExactCode(13), Position: 0, Selection: "13", Amount: 1},
		{PlayCode: pc28ColorRed, Position: 0, Selection: "红波", Amount: 1},
	}, member.Username, "pc28-v1-settle-0001")
	if err != nil || receipt.BetCount != 3 || receipt.Total != 3 {
		t.Fatalf("place PC28 settlement fixture: %+v / %v", receipt, err)
	}
	// The current room setting may change, but each accepted ticket must retain
	// the setting used at placement.
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30,"pc28_gray_push":false}`)
	if err := db.Create(&lottery.Draw{GameID: game.ID, Issue: game.NextIssue, Numbers: "9,5,0", DrawAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := NewBetAdminService(db).SettleIssue(game.ID, game.NextIssue, "PC28 13/14回归")
	if err != nil || result.PendingBefore != 3 || result.Won != 1 || result.Lost != 1 || result.Push != 1 || result.StakeAmount != 3 || result.PayoutAmount != 2.5 {
		t.Fatalf("PC28 v1 settlement mismatch: %+v / %v", result, err)
	}
	var rows []bet.Bet
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, game.NextIssue).Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("stored %d rows", len(rows))
	}
	byCode := make(map[string]bet.Bet, len(rows))
	for _, row := range rows {
		byCode[row.PlayCode] = row
		if row.ValidTurnoverCents == nil || *row.ValidTurnoverCents != 0 || row.RebateCents != 0 ||
			row.UserIssueStakeCentsSnapshot == nil || *row.UserIssueStakeCentsSnapshot != 300 || !row.PC28GrayPush || row.SettlementOdds == nil {
			t.Fatalf("PC28 immutable settlement evidence missing: %+v", row)
		}
	}
	if row := byCode[pc28SumSize]; row.Status != "won" || *row.SettlementOdds != 1.5 || row.SettlementPolicy != "pc28_v1_13_14_two_sided_gt1" || row.PayoutCents != 150 {
		t.Fatalf("dynamic two-sided settlement was not stored: %+v", row)
	}
	if row := byCode[pc28ExactCode(13)]; row.Status != "lost" || *row.SettlementOdds != 2.8 || row.SettlementPolicy != "pc28_standard" || row.PayoutCents != 0 {
		t.Fatalf("ordinary losing PC28 ticket changed: %+v", row)
	}
	if row := byCode[pc28ColorRed]; row.Status != "cancelled" || *row.SettlementOdds != 1 || row.SettlementPolicy != "pc28_gray_push" || row.PayoutCents != 100 || row.ReconciliationNote != "settlement_push" {
		t.Fatalf("frozen gray push was not auditable: %+v", row)
	}
}
