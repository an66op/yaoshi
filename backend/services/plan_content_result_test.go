package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"testing"
	"time"
)

func TestGenericPlanDrawResultUsesVersionedTargetContracts(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	created := now.Add(-2 * time.Minute)
	for _, test := range []struct {
		name, gameID, draw, picks, size, parity, want string
	}{
		{"racing champion", "speed-fly", "6,8,10,1,2,4,9,7,3,5", "1,2,3,4,6", "", "", plan.ResultHit},
		{"five digit first ball", "speed-ssc", "7,0,1,2,3", "1,3,7", "大", "单", plan.ResultHit},
		{"five digit composite miss", "sg-ssc", "7,0,1,2,3", "1,3,7", "小", "单", plan.ResultMiss},
		{"pc sum", "canada-28", "8,7,9", "3,14,24", "大", "双", plan.ResultHit},
		{"mark six special", "hong-kong-mark-six", "1,2,3,4,5,6,49", "7,21,49", "大", "单", plan.ResultHit},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := plan.Recommendation{GameID: test.gameID, Issue: "verified-1", Numbers: test.picks, Size: test.size, Parity: test.parity, Result: oppositePlanResult(test.want), CreatedAt: created}
			game := lottery.Game{ID: test.gameID}
			issue := lottery.Issue{GameID: test.gameID, Issue: row.Issue, SealAt: now.Add(-90 * time.Second)}
			draw := lottery.Draw{GameID: test.gameID, Issue: row.Issue, Numbers: test.draw, DrawAt: now.Add(-time.Minute)}
			if got := genericPlanDrawResult(row, game, issue, draw, now); got != test.want {
				t.Fatalf("derived result=%s want=%s", got, test.want)
			}
		})
	}
}

func TestGenericPlanDrawResultIgnoresStoredClaimsAndFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	valid := plan.Recommendation{GameID: "speed-ssc", Issue: "verified-2", Numbers: "1,3,7", Result: plan.ResultHit, CreatedAt: now.Add(-2 * time.Minute)}
	game := lottery.Game{ID: valid.GameID}
	issue := lottery.Issue{GameID: valid.GameID, Issue: valid.Issue, SealAt: now.Add(-90 * time.Second)}
	draw := lottery.Draw{GameID: valid.GameID, Issue: valid.Issue, Numbers: "8,0,1,2,3", DrawAt: now.Add(-time.Minute)}
	if got := genericPlanDrawResult(valid, game, issue, draw, now); got != plan.ResultMiss {
		t.Fatalf("stored hit claim overrode trusted draw: %s", got)
	}
	for name, change := range map[string]func(*plan.Recommendation, *lottery.Game, *lottery.Issue, *lottery.Draw){
		"published after seal": func(row *plan.Recommendation, _ *lottery.Game, issue *lottery.Issue, _ *lottery.Draw) {
			row.CreatedAt = issue.SealAt
		},
		"edited after seal": func(row *plan.Recommendation, _ *lottery.Game, issue *lottery.Issue, _ *lottery.Draw) {
			row.UpdatedAt = issue.SealAt
		},
		"published after draw": func(row *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, draw *lottery.Draw) {
			row.CreatedAt = draw.DrawAt
		},
		"draw before seal": func(_ *plan.Recommendation, _ *lottery.Game, issue *lottery.Issue, draw *lottery.Draw) {
			draw.DrawAt = issue.SealAt.Add(-time.Second)
		},
		"future draw": func(_ *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, draw *lottery.Draw) {
			draw.DrawAt = now.Add(time.Minute)
		},
		"wrong issue": func(_ *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, draw *lottery.Draw) {
			draw.Issue = "other"
		},
		"wrong lifecycle": func(_ *plan.Recommendation, _ *lottery.Game, issue *lottery.Issue, _ *lottery.Draw) {
			issue.Issue = "other"
		},
		"wrong game": func(_ *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, draw *lottery.Draw) {
			draw.GameID = "sg-ssc"
		},
		"unsupported game": func(row *plan.Recommendation, game *lottery.Game, issue *lottery.Issue, draw *lottery.Draw) {
			row.GameID, game.ID, issue.GameID, draw.GameID = "unknown", "unknown", "unknown", "unknown"
		},
		"malformed draw": func(_ *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, draw *lottery.Draw) {
			draw.Numbers = "8,0,x,1,2,3"
		},
		"invalid pick": func(row *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, _ *lottery.Draw) {
			row.Numbers = "1,1,7"
		},
		"invalid direction": func(row *plan.Recommendation, _ *lottery.Game, _ *lottery.Issue, _ *lottery.Draw) { row.Size = "中" },
	} {
		t.Run(name, func(t *testing.T) {
			row, candidateGame, candidateIssue, candidateDraw := valid, game, issue, draw
			change(&row, &candidateGame, &candidateIssue, &candidateDraw)
			if got := genericPlanDrawResult(row, candidateGame, candidateIssue, candidateDraw, now); got != plan.ResultPending {
				t.Fatalf("untrusted evidence produced %s", got)
			}
		})
	}
}

func TestEffectivePlanSealAtUsesEarlierRoomOrSharedCutoff(t *testing.T) {
	global := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	issue := lottery.Issue{GameID: "speed-ssc", Issue: "room-cutoff-1", SealAt: global}
	earlier := lottery.IssueWindow{WorkspaceID: 7, GameID: issue.GameID, Issue: issue.Issue, SealAt: global.Add(-time.Minute)}
	if got := effectivePlanSealAt(issue, &earlier); !got.Equal(earlier.SealAt) {
		t.Fatalf("earlier room cutoff ignored: %v", got)
	}
	later := earlier
	later.SealAt = global.Add(time.Minute)
	if got := effectivePlanSealAt(issue, &later); !got.Equal(global) {
		t.Fatalf("later room cutoff extended shared cutoff: %v", got)
	}
	wrongRoomIdentity := earlier
	wrongRoomIdentity.GameID = "sg-ssc"
	if got := effectivePlanSealAt(issue, &wrongRoomIdentity); !got.Equal(global) {
		t.Fatalf("unrelated room window changed cutoff: %v", got)
	}
}

func oppositePlanResult(result string) string {
	if result == plan.ResultHit {
		return plan.ResultMiss
	}
	return plan.ResultHit
}
