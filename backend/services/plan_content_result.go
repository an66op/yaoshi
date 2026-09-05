package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type planHitStatistic struct {
	Rate        *float64
	SampleCount int
}

func canonicalPlanSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "manual"
	}
	return source
}

func planMasterStatisticKey(gameID, source, masterName string) string {
	return gameID + "\x00" + canonicalPlanSource(source) + "\x00" + masterName
}

// effectivePlanSealAt is deliberately conservative: a plan publication must
// predate both the shared lifecycle cutoff and the room's frozen cutoff. A
// later room setting can never make a publication trustworthy again.
func effectivePlanSealAt(issue lottery.Issue, window *lottery.IssueWindow) time.Time {
	sealAt := issue.SealAt
	if window != nil && window.WorkspaceID != 0 && window.GameID == issue.GameID && window.Issue == issue.Issue &&
		!window.SealAt.IsZero() && (sealAt.IsZero() || window.SealAt.Before(sealAt)) {
		sealAt = window.SealAt
	}
	return sealAt
}

// genericPlanDrawResult grades the persisted forecast against one immutable
// draw. The stored Result field is intentionally ignored: older administration
// clients could write it directly, so it is not settlement evidence.
//
// The generic plan contract targets one value: the PC28 sum, the Mark Six
// special number, or the first position/ball for the remaining supported
// products. When size/parity are supplied, every supplied dimension must match.
func genericPlanDrawResult(row plan.Recommendation, game lottery.Game, issue lottery.Issue, draw lottery.Draw, now time.Time) string {
	publishedAt := row.CreatedAt
	if row.UpdatedAt.After(publishedAt) {
		publishedAt = row.UpdatedAt
	}
	if row.GameID == "" || row.GameID != game.ID || draw.GameID != row.GameID || draw.Issue != row.Issue ||
		issue.GameID != row.GameID || issue.Issue != row.Issue || issue.SealAt.IsZero() ||
		publishedAt.IsZero() || !publishedAt.Before(issue.SealAt) || draw.DrawAt.IsZero() || draw.DrawAt.Before(issue.SealAt) ||
		!publishedAt.Before(draw.DrawAt) || draw.DrawAt.After(now) {
		return plan.ResultPending
	}
	profile, ok := rulesForGame(&game)
	if !ok {
		return plan.ResultPending
	}
	numbers, ok := strictPlanDrawNumbers(draw.Numbers)
	if !ok || profile.validateDraw(numbers) != nil {
		return plan.ResultPending
	}

	target, minimum, maximum := numbers[0], profile.MinNumber, profile.MaxNumber
	switch {
	case profile.PC28 > 0:
		target, minimum, maximum = 0, 0, 27
		for _, number := range numbers {
			target += number
		}
	case profile.MarkSix:
		target = numbers[len(numbers)-1]
	}

	picks, ok := strictPlanPickNumbers(row.Numbers, minimum, maximum)
	if !ok {
		return plan.ResultPending
	}
	hit := false
	for _, number := range picks {
		hit = hit || number == target
	}
	if row.Size != "" {
		threshold := profile.PositionBigFrom
		if profile.PC28 > 0 {
			threshold = profile.SumBigFrom
		} else if profile.MarkSix {
			threshold = 25
		}
		if threshold == 0 || (row.Size != "大" && row.Size != "小") {
			return plan.ResultPending
		}
		hit = hit && ((row.Size == "大") == (target >= threshold))
	}
	if row.Parity != "" {
		if row.Parity != "单" && row.Parity != "双" {
			return plan.ResultPending
		}
		hit = hit && ((row.Parity == "单") == (target%2 == 1))
	}
	if hit {
		return plan.ResultHit
	}
	return plan.ResultMiss
}

func strictPlanDrawNumbers(raw string) ([]int, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, false
	}
	result := make([]int, len(parts))
	for index, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || strings.TrimSpace(part) == "" {
			return nil, false
		}
		result[index] = value
	}
	return result, true
}

func strictPlanPickNumbers(raw string, minimum, maximum int) ([]int, bool) {
	numbers, ok := strictPlanDrawNumbers(raw)
	if !ok || len(numbers) > 12 {
		return nil, false
	}
	seen := make(map[int]bool, len(numbers))
	for _, number := range numbers {
		if number < minimum || number > maximum || seen[number] {
			return nil, false
		}
		seen[number] = true
	}
	return numbers, true
}

// deriveTrustedPlanResults resets every in-memory outcome before consulting
// validated draws. This makes both historical and newly created rows immune to
// a stale/fabricated persisted Result value without rewriting publication data.
func deriveTrustedPlanResults(db *gorm.DB, rows []plan.Recommendation, now time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	gameIDs, issues, workspaceIDs := make([]string, 0), make([]string, 0), make([]uint64, 0)
	seenGames, seenIssues, seenWorkspaces := map[string]bool{}, map[string]bool{}, map[uint64]bool{}
	for index := range rows {
		rows[index].Result = plan.ResultPending
		if !seenGames[rows[index].GameID] {
			seenGames[rows[index].GameID] = true
			gameIDs = append(gameIDs, rows[index].GameID)
		}
		if !seenIssues[rows[index].Issue] {
			seenIssues[rows[index].Issue] = true
			issues = append(issues, rows[index].Issue)
		}
		if rows[index].WorkspaceID > 0 && !seenWorkspaces[rows[index].WorkspaceID] {
			seenWorkspaces[rows[index].WorkspaceID] = true
			workspaceIDs = append(workspaceIDs, rows[index].WorkspaceID)
		}
	}
	var games []lottery.Game
	if err := db.Where("id IN ?", gameIDs).Find(&games).Error; err != nil {
		return err
	}
	var issueRows []lottery.Issue
	if err := db.Where("game_id IN ? AND issue IN ?", gameIDs, issues).Find(&issueRows).Error; err != nil {
		return err
	}
	var windowRows []lottery.IssueWindow
	if len(workspaceIDs) > 0 {
		if err := db.Where("workspace_id IN ? AND game_id IN ? AND issue IN ?", workspaceIDs, gameIDs, issues).Find(&windowRows).Error; err != nil {
			return err
		}
	}
	var draws []lottery.Draw
	for _, gameID := range gameIDs {
		var gameDraws []lottery.Draw
		if err := trustedDrawsForGame(db, gameID).Where("issue IN ? AND draw_at <= ?", issues, now).Find(&gameDraws).Error; err != nil {
			return err
		}
		draws = append(draws, gameDraws...)
	}
	gameByID := make(map[string]lottery.Game, len(games))
	for _, game := range games {
		gameByID[game.ID] = game
	}
	drawByIssue := make(map[string]lottery.Draw, len(draws))
	for _, draw := range draws {
		drawByIssue[draw.GameID+"\x00"+draw.Issue] = draw
	}
	issueByIdentity := make(map[string]lottery.Issue, len(issueRows))
	for _, issue := range issueRows {
		issueByIdentity[issue.GameID+"\x00"+issue.Issue] = issue
	}
	windowByIdentity := make(map[string]lottery.IssueWindow, len(windowRows))
	for _, window := range windowRows {
		identity := strconv.FormatUint(window.WorkspaceID, 10) + "\x00" + window.GameID + "\x00" + window.Issue
		windowByIdentity[identity] = window
	}
	for index := range rows {
		identity := rows[index].GameID + "\x00" + rows[index].Issue
		game, gameOK := gameByID[rows[index].GameID]
		issue, issueOK := issueByIdentity[identity]
		draw, drawOK := drawByIssue[identity]
		if gameOK && issueOK && drawOK {
			windowIdentity := strconv.FormatUint(rows[index].WorkspaceID, 10) + "\x00" + rows[index].GameID + "\x00" + rows[index].Issue
			if window, ok := windowByIdentity[windowIdentity]; ok {
				issue.SealAt = effectivePlanSealAt(issue, &window)
			}
			rows[index].Result = genericPlanDrawResult(rows[index], game, issue, draw, now)
		}
	}
	return nil
}

func planHitStatistics(rows []plan.Recommendation) map[string]planHitStatistic {
	type score struct{ hits, settled int }
	scores := map[string]score{}
	for _, row := range rows {
		key := planMasterStatisticKey(row.GameID, row.Source, row.MasterName)
		value := scores[key]
		switch row.Result {
		case plan.ResultHit:
			value.hits++
			value.settled++
		case plan.ResultMiss:
			value.settled++
		}
		scores[key] = value
	}
	result := make(map[string]planHitStatistic, len(scores))
	for key, value := range scores {
		statistic := planHitStatistic{SampleCount: value.settled}
		if value.settled > 0 {
			rate := float64(value.hits) * 100 / float64(value.settled)
			statistic.Rate = &rate
		}
		result[key] = statistic
	}
	return result
}
