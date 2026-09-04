package services

import (
	"errors"
	"testing"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Test163MarkSixPostgresPromotesOnlyExactBlankLegacyRows(t *testing.T) {
	db := timingPostgresDatabase(t)
	binding, _ := source163MarkSixBindingForGame("hong-kong-mark-six")
	drawAt := time.Date(2026, 8, 27, 13, 30, 0, 0, time.UTC)
	numbers := []int{12, 45, 44, 25, 17, 31, 35}
	rows := []lottery.Draw{
		{GameID: binding.GameID, Issue: "2026094", Numbers: joinNumbers(numbers), DrawAt: drawAt},
		{GameID: binding.GameID, Issue: "2026095", Numbers: joinNumbers(numbers), DrawAt: drawAt.Add(-24 * time.Hour)},
		{GameID: binding.GameID, Issue: "2026096", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: "other-source-v1", ConversionRevision: "other-conversion-v1"},
		{GameID: binding.GameID, Issue: "2026097", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	draws := []sourceDraw{
		{Issue: "2026094", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026095", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026096", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026097", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026098", Numbers: numbers, DrawAt: drawAt},
	}
	for index := range draws {
		draws[index].SourceRevision = binding.SourceRevision
		draws[index].ConversionRevision = binding.ConversionRevision
	}
	imported, err := insert163MarkSixDraws(db, binding, draws)
	if err != nil || imported != 2 { // one exact promotion plus one new row
		t.Fatalf("safe promotion/import failed: imported=%d err=%v", imported, err)
	}
	var exact, wrongTime, otherSource, current, inserted lottery.Draw
	for issue, target := range map[string]*lottery.Draw{
		"2026094": &exact, "2026095": &wrongTime, "2026096": &otherSource, "2026097": &current, "2026098": &inserted,
	} {
		if err := db.First(target, "game_id = ? AND issue = ?", binding.GameID, issue).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []lottery.Draw{exact, current, inserted} {
		if row.SourceRevision != binding.SourceRevision || row.ConversionRevision != binding.ConversionRevision {
			t.Fatalf("verified row lacks exact contract: %+v", row)
		}
	}
	if wrongTime.SourceRevision != "" || wrongTime.ConversionRevision != "" || !wrongTime.DrawAt.Equal(drawAt.Add(-24*time.Hour)) {
		t.Fatalf("time-conflicting blank history was modified: %+v", wrongTime)
	}
	if otherSource.SourceRevision != "other-source-v1" || otherSource.ConversionRevision != "other-conversion-v1" {
		t.Fatalf("non-empty source history was overwritten: %+v", otherSource)
	}

	currentConflict := lottery.Draw{GameID: binding.GameID, Issue: "2026099", Numbers: joinNumbers(numbers), DrawAt: drawAt.Add(-time.Hour), SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision}
	if err := db.Create(&currentConflict).Error; err != nil {
		t.Fatal(err)
	}
	conflicting := sourceDraw{Issue: currentConflict.Issue, Numbers: numbers, DrawAt: drawAt, SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision}
	if imported, err := insert163MarkSixDraws(db, binding, []sourceDraw{conflicting}); imported != 0 || !errors.Is(err, err163MirrorDrawConflict) {
		t.Fatalf("current revision time conflict was not blocked: imported=%d err=%v", imported, err)
	}
}

func Test163NewMacauPostgresUpgradesOnlyExactLegacyConversion(t *testing.T) {
	db := timingPostgresDatabase(t)
	binding, _ := source163MarkSixBindingForGame("new-macau-mark-six")
	drawAt := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	numbers := []int{12, 45, 44, 25, 17, 31, 35}
	rows := []lottery.Draw{
		{GameID: binding.GameID, Issue: "2026243", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: binding.SourceRevision, ConversionRevision: source163MirrorConversionVersion},
		{GameID: binding.GameID, Issue: "2026244", Numbers: joinNumbers(numbers), DrawAt: drawAt.Add(-24 * time.Hour), SourceRevision: binding.SourceRevision, ConversionRevision: source163MirrorConversionVersion},
		{GameID: binding.GameID, Issue: "2026245", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: "other-source-v1", ConversionRevision: source163MirrorConversionVersion},
		{GameID: binding.GameID, Issue: "2026246", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: binding.SourceRevision, ConversionRevision: "other-conversion-v1"},
		{GameID: binding.GameID, Issue: "2026247", Numbers: joinNumbers(numbers), DrawAt: drawAt, SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	draws := []sourceDraw{
		{Issue: "2026243", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026244", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026245", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026246", Numbers: numbers, DrawAt: drawAt},
		{Issue: "2026247", Numbers: numbers, DrawAt: drawAt},
	}
	for index := range draws {
		draws[index].SourceRevision = binding.SourceRevision
		draws[index].ConversionRevision = binding.ConversionRevision
	}
	imported, err := insert163MarkSixDraws(db, binding, draws)
	if err != nil || imported != 1 {
		t.Fatalf("new-macau legacy conversion promotion failed: imported=%d err=%v", imported, err)
	}
	var exact, wrongTime, otherSource, otherConversion, current lottery.Draw
	for issue, target := range map[string]*lottery.Draw{
		"2026243": &exact, "2026244": &wrongTime, "2026245": &otherSource, "2026246": &otherConversion, "2026247": &current,
	} {
		if err := db.First(target, "game_id = ? AND issue = ?", binding.GameID, issue).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []lottery.Draw{exact, current} {
		if row.SourceRevision != binding.SourceRevision || row.ConversionRevision != binding.ConversionRevision {
			t.Fatalf("verified new-macau row lacks current contract: %+v", row)
		}
	}
	if wrongTime.SourceRevision != binding.SourceRevision || wrongTime.ConversionRevision != source163MirrorConversionVersion || !wrongTime.DrawAt.Equal(drawAt.Add(-24*time.Hour)) {
		t.Fatalf("time-conflicting legacy conversion was modified: %+v", wrongTime)
	}
	if otherSource.SourceRevision != "other-source-v1" || otherSource.ConversionRevision != source163MirrorConversionVersion {
		t.Fatalf("other source was overwritten: %+v", otherSource)
	}
	if otherConversion.SourceRevision != binding.SourceRevision || otherConversion.ConversionRevision != "other-conversion-v1" {
		t.Fatalf("other conversion was overwritten: %+v", otherConversion)
	}
}

func Test163HongKongPostgresRepairsEmptyOverdueIssueAndEveryWorkspaceWindow(t *testing.T) {
	db := timingPostgresDatabase(t)
	binding, _ := source163MarkSixBindingForGame(source163HongKongGameID)
	now := time.Now().UTC().Truncate(time.Second)
	oldAt, newAt := now.Add(-time.Hour), now.Add(2*time.Hour)
	if err := db.Model(&lottery.Game{}).Where("id = ?", binding.GameID).Updates(map[string]any{
		"source_kind": "external", "source_name": source163MirrorName, "source_url": source163MirrorURL,
		"sync_status": "ok", "next_issue": "2026096", "next_draw_at": newAt,
		"draw_interval": int((4 * time.Hour) / time.Second), "timing_source": "upstream",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", binding.GameID).Error; err != nil {
		t.Fatal(err)
	}
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	firstRoom := timingPostgresRoom(t, db, "hk_repair_room_one", "791621")
	secondRoom := timingPostgresRoom(t, db, "hk_repair_room_two", "791622")
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":31}`)
	timingPostgresSettings(t, db, firstRoom.ID, `{"seal_seconds":11}`)
	timingPostgresSettings(t, db, secondRoom.ID, `{"seal_seconds":99,"game_timing_overrides":{"hong-kong-mark-six":{"seal_seconds":17}}}`)

	issue := lottery.Issue{
		GameID: binding.GameID, Issue: "2026096", Status: lottery.IssueStatusError, SourceMode: "external",
		AcceptAt: oldAt.Add(-4 * time.Hour), SealAt: oldAt.Add(-30 * time.Second), ScheduledDrawAt: &oldAt,
		LastError: "对账异常：旧开奖生命周期超时",
	}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	for _, workspaceID := range []uint64{platformID, firstRoom.ID, secondRoom.ID} {
		window := newIssueWindow(workspaceID, &game, issue.Issue, oldAt, 30)
		if err := db.Create(&window).Error; err != nil {
			t.Fatal(err)
		}
	}
	schedule := sourceSchedule{Issue: issue.Issue, DrawAt: newAt, Interval: int(newAt.Sub(now) / time.Second), Source: "upstream"}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		repaired, err := repair163HongKongIssueLifecycle(tx, &game, binding, schedule, now)
		if err != nil {
			return err
		}
		if !repaired {
			t.Fatal("eligible isolated issue was not repaired")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored lottery.Issue
	if err := db.First(&stored, issue.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != lottery.IssueStatusAccepting || stored.LastError != "" || stored.ScheduledDrawAt == nil || !stored.ScheduledDrawAt.Equal(newAt) ||
		!stored.SealAt.Equal(newAt.Add(-31*time.Second)) || stored.DrawAt != nil || stored.SettledAt != nil {
		t.Fatalf("shared lifecycle was not safely rebased: %+v", stored)
	}
	wantSeal := map[uint64]int{platformID: 31, firstRoom.ID: 11, secondRoom.ID: 17}
	var windows []lottery.IssueWindow
	if err := db.Where("game_id = ? AND issue = ?", binding.GameID, issue.Issue).Find(&windows).Error; err != nil {
		t.Fatal(err)
	}
	if len(windows) != len(wantSeal) {
		t.Fatalf("workspace windows=%+v", windows)
	}
	for _, window := range windows {
		seal, ok := wantSeal[window.WorkspaceID]
		if !ok || !window.ScheduledDrawAt.Equal(newAt) || !window.AcceptAt.Equal(newAt.Add(-4*time.Hour)) ||
			!window.SealAt.Equal(newAt.Add(-time.Duration(seal)*time.Second)) || window.SealSeconds != seal {
			t.Fatalf("workspace %d did not use its current seal config: %+v", window.WorkspaceID, window)
		}
	}
}
