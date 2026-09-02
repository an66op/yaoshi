package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type oddsRiskQueryRecorder struct {
	logger.Interface
	queries    []string
	afterQuery func(string)
}

func configureDigits5V3RiskFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	view, err := NewOddsAdminService(db).Get("speed-ssc")
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]float64{
		"ball_1_5": 9.9, "two_sided": 1.993, "dragon_tiger": 1.993, "dragon_tiger_tie": 8.7,
		"leopard": 50, "straight": 15, "pair": 8, "half_straight": 6, "mixed": 4,
	}
	for index := range view.Items {
		view.Items[index].Odds = prices[view.Items[index].PlayCode]
	}
	if _, err := NewOddsAdminService(db).Update("speed-ssc", UpdateOddsLimitsInput{Items: view.Items}); err != nil {
		t.Fatal("configure explicit risk fixture:", err)
	}
}

func (r *oddsRiskQueryRecorder) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	r.queries = append(r.queries, sql)
	if r.afterQuery != nil {
		r.afterQuery(sql)
	}
}

func TestOddsRiskPostgresReadsOneSnapshot(t *testing.T) {
	db := timingPostgresDatabase(t)
	configureDigits5V3RiskFixture(t, db)
	room := timingPostgresRoom(t, db, "odds_risk_snapshot", "782054")
	member := timingPostgresMember(t, db, room, "odds_risk_snapshot_member")
	queries := &oddsRiskQueryRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	trade := NewTradingAdminService(db.Session(&gorm.Session{Logger: queries}))
	if err := trade.checkFrontThreeOddsRisk(member, "speed-ssc", "leopard", 50); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("unsafe shape prices escaped snapshot check: %v", err)
	}
	if len(queries.queries) != 1 {
		t.Fatalf("shape risk prices must use one statement snapshot, got %d queries", len(queries.queries))
	}
	for _, table := range []string{"lottery_play_limits", "room_play_odds", "user_play_odds", "workspace_memberships"} {
		if !strings.Contains(queries.queries[0], table) {
			t.Fatalf("shape price snapshot omits %s", table)
		}
	}
	queries.queries = nil
	if err := trade.checkFrontThreeOddsRisk(member, "speed-ssc", "ball_1_5", 9.9); err != nil {
		t.Fatal(err)
	}
	if len(queries.queries) != 0 {
		t.Fatalf("ordinary number bets added %d risk queries", len(queries.queries))
	}
}

// Uses only the dedicated loopback test database; the helper wraps schema and
// fixtures in a rollback and refuses a nonempty or wrongly named database.
func TestOddsRiskPostgresPlacementGuardAndSnapshots(t *testing.T) {
	db := timingPostgresDatabase(t)
	configureDigits5V3RiskFixture(t, db)
	room := timingPostgresRoom(t, db, "odds_risk_room", "782051")
	member := timingPostgresMember(t, db, room, "odds_risk_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	var game lottery.Game
	if err := db.First(&game, "id = ?", "speed-ssc").Error; err != nil {
		t.Fatal(err)
	}
	game.NextIssue = "981105"
	game.NextDrawAt = time.Now().UTC().Truncate(time.Second).Add(10 * time.Minute)
	if err := db.Model(&game).Updates(map[string]any{
		"next_issue": game.NextIssue, "next_draw_at": game.NextDrawAt,
		"draw_interval": 3600, "timing_source": "upstream", "enabled": true, "sync_status": "ok", "last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var originalOdds []odds.PlayLimit
	if err := db.Where("game_id = ?", game.ID).Order("id").Find(&originalOdds).Error; err != nil {
		t.Fatal(err)
	}
	view, err := NewOddsAdminService(db).Get(game.ID)
	if err != nil || len(view.RiskWarnings) != 1 || view.RiskWarnings[0].Code != "SHAPE_COVERAGE_RISK" {
		t.Fatalf("platform risk not exposed: %+v err=%v", view, err)
	}
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	shape := PlaceBetInput{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "leopard", Position: 1, Selection: "yes", Amount: 20}
	number := shape
	number.PlayCode, number.Selection = "ball_1_5", "2"
	before := timingPostgresMoney(t, db, member.UserID)
	if _, err := service.Place(shape); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("unsafe single shape accepted: %v", err)
	}
	if _, err := service.PlaceBatch([]PlaceBetInput{number, shape}); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("unsafe shape in later batch line accepted: %v", err)
	}
	if got := timingPostgresMoney(t, db, member.UserID); got != before {
		t.Fatalf("rejected single/batch altered funds: before=%+v after=%+v", before, got)
	}
	if _, err := service.Place(number); err != nil {
		t.Fatal("shape risk must not block ordinary number bets:", err)
	}
	var afterOdds []odds.PlayLimit
	if err := db.Where("game_id = ?", game.ID).Order("id").Find(&afterOdds).Error; err != nil || !reflect.DeepEqual(originalOdds, afterOdds) {
		t.Fatalf("risk inspection rewrote configured odds: %v", err)
	}

	// Old accepted tickets retain their own prices, even while the live market
	// is blocked. This deliberately seeded isolated fixture is not a new bet.
	legacy := bet.Bet{
		WorkspaceID: room.ID, RoomScope: betRoomScope(member), UserID: member.UserID, Username: member.Username,
		GameID: game.ID, Issue: "981104", PlayCode: "leopard", Position: 1, Selection: "yes",
		AmountCents: 200, Odds: 50, Status: "pending", RuleVersion: "digits5-v2", RequestReference: "risk-existing-ticket",
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lottery.Draw{GameID: game.ID, Issue: legacy.Issue, Numbers: "1,1,1,2,3", DrawAt: time.Now().UTC().Add(-time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	settled, err := service.SettleIssue(game.ID, legacy.Issue, "risk-regression")
	if err != nil || settled.Won != 1 || settled.PayoutAmount != 100 {
		t.Fatalf("risk guard changed historical settlement: %+v err=%v", settled, err)
	}
	var saved bet.Bet
	if err := db.First(&saved, legacy.ID).Error; err != nil || saved.Odds != 50 || saved.AmountCents != 200 || saved.PayoutCents != 10000 || saved.RuleVersion != legacy.RuleVersion {
		t.Fatalf("old financial snapshot was repriced: %+v err=%v", saved, err)
	}
}

func TestOddsRiskPostgresUsesEffectiveRoomMemberPrices(t *testing.T) {
	db := timingPostgresDatabase(t)
	configureDigits5V3RiskFixture(t, db)
	room := timingPostgresRoom(t, db, "odds_risk_prices", "782052")
	otherRoom := timingPostgresRoom(t, db, "odds_risk_other", "782053")
	member := timingPostgresMember(t, db, room, "odds_risk_prices_member")
	other := timingPostgresMember(t, db, otherRoom, "odds_risk_other_member")
	trade := NewTradingAdminService(db)
	resolve := func(accountID uint64) error {
		_, err := trade.Resolve(accountID, "speed-ssc", "leopard", 20, 999, 0)
		return err
	}
	if err := resolve(member.UserID); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("unsafe platform fallback accepted: %v", err)
	}
	// Test fixture only: never overwrite a developer's actual prices.
	safePrices := []float64{50, 15, 2.8, 2.4, 3}
	for index, code := range frontThreeShapeCodes {
		row := odds.RoomPlayOdds{WorkspaceID: room.ID, GameID: "speed-ssc", PlayCode: code, Odds: safePrices[index]}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := resolve(member.UserID); err != nil {
		t.Fatal("valid room override was blocked by unsafe base:", err)
	}
	if err := resolve(other.UserID); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("another room inherited a different room's safe prices: %v", err)
	}
	membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: member.UserID, Status: 1, Role: "member", OddsMultiplier: 1.5}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if err := resolve(member.UserID); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("membership multiplier escaped coverage check: %v", err)
	}
	for index, code := range frontThreeShapeCodes {
		row := odds.UserPlayOdds{WorkspaceID: room.ID, UserID: member.UserID, GameID: "speed-ssc", PlayCode: code, Odds: safePrices[index]}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := resolve(member.UserID); err != nil {
		t.Fatal("exact user odds were wrongly multiplied or ignored:", err)
	}
	if err := db.Model(&odds.UserPlayOdds{}).Where("workspace_id = ? AND user_id = ? AND game_id = ?", room.ID, member.UserID, "speed-ssc").Update("odds", 50).Error; err != nil {
		t.Fatal(err)
	}
	if err := resolve(member.UserID); apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("unsafe member prices escaped coverage check: %v", err)
	}
	if _, err := trade.Resolve(member.UserID, "speed-ssc", "ball_1_5", 20, 999, 0); err != nil {
		t.Fatal("ordinary number odds were blocked:", err)
	}
}

func TestOddsRiskPostgresRejectsBatchMixedPriceSnapshots(t *testing.T) {
	db := timingPostgresDatabase(t)
	configureDigits5V3RiskFixture(t, db)
	room := timingPostgresRoom(t, db, "odds_risk_batch", "782055")
	member := timingPostgresMember(t, db, room, "odds_risk_batch_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-ssc").Updates(map[string]any{
		"next_issue": "981107", "next_draw_at": time.Now().UTC().Truncate(time.Second).Add(10 * time.Minute),
		"draw_interval": 3600, "timing_source": "upstream", "enabled": true, "sync_status": "ok", "last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	pricesA := []float64{50, 2, 2, 2, 2}
	pricesB := []float64{1.2, 20, 20, 20, 20}
	for _, prices := range [][]float64{pricesA, pricesB} {
		offers := make(map[string]float64)
		for index, code := range frontThreeShapeCodes {
			offers[code] = prices[index]
		}
		if len(frontThreeOddsRisks("speed-ssc", offers)) != 0 {
			t.Fatal("fixture must consist of two individually safe markets")
		}
	}
	setPrices := func(prices []float64) error {
		for index, code := range frontThreeShapeCodes {
			if err := db.Model(&odds.PlayLimit{}).Where("game_id = ? AND play_code = ?", "speed-ssc", code).Update("odds", prices[index]).Error; err != nil {
				return err
			}
		}
		return nil
	}
	if err := setPrices(pricesA); err != nil {
		t.Fatal(err)
	}
	// Deterministically interleave a completed price change between the first
	// resolved quote and the next one. GORM Scan closes its result rows before
	// Trace, so the fixture can update the same disposable transaction safely.
	// This exercises the boundary without timing-dependent goroutines or a
	// second connection that cannot see the uncommitted fixture schema.
	var shiftErr error
	marketReads := 0
	queries := &oddsRiskQueryRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	queries.afterQuery = func(sql string) {
		if strings.Contains(sql, "FROM (VALUES") && strings.Contains(sql, "room_member.odds_multiplier") {
			marketReads++
			if marketReads == 1 {
				shiftErr = setPrices(pricesB)
			}
		}
	}
	service := NewBetAdminService(db.Session(&gorm.Session{Logger: queries}))
	service.suppressNotifications = true
	inputs := make([]PlaceBetInput, 0, len(frontThreeShapeCodes))
	for _, code := range frontThreeShapeCodes {
		inputs = append(inputs, PlaceBetInput{GameID: "speed-ssc", Issue: "981107", UserID: member.UserID, PlayCode: code, Position: 1, Selection: "yes", Amount: 20})
	}
	before := timingPostgresMoney(t, db, member.UserID)
	_, err := service.PlaceBatch(inputs)
	if shiftErr != nil || marketReads != 5 {
		t.Fatalf("did not exercise all five individually safe reads: reads=%d update=%v placement=%v", marketReads, shiftErr, err)
	}
	if apperrors.GetErrorCode(err) != "ODDS_RISK_UNSAFE" {
		t.Fatalf("mixed actual batch quotes 50/20/20/20/20 escaped check: %v", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("rejected mixed-price batch changed money: before=%+v after=%+v", before, after)
	}
}
