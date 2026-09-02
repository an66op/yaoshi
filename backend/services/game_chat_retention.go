package services

import (
	"backend/data/models/lifecycle"
	apperrors "backend/errors"
	"encoding/json"
)

// A real game conversation has a persisted game id. Lobby, service and
// unmapped legacy history never move into the shorter game-room policy.
const gameChatRoomPredicate = `message.room_type = 'group'
	AND message.game_id NOT IN ('', 'lobby', 'legacy', 'service')
	AND EXISTS (SELECT 1 FROM lottery_games game WHERE game.id = message.game_id)`

// The latest draw is deliberately selected by event time, not insertion id:
// importing an older result must not make the actual latest period disposable.
// Without any valid draw there is no safe game-room boundary, so retain all.
const gameChatLifecyclePredicate = `(` + gameChatRoomPredicate + `)
	AND (` + unlinkedChatLifecyclePredicate + `)
	AND message.created_at < (
		SELECT latest.draw_at FROM lottery_draws latest
		WHERE latest.game_id = message.game_id
		ORDER BY latest.draw_at DESC, latest.issue DESC LIMIT 1
	)
	AND (
		(` + ordinaryChatTextPredicate + `)
		OR (
			message.user_id = 0 AND message.username = 'draw_assistant'
			AND message.message_type IN ('settlement', 'scoreboard')
			AND message.reference_id > 0
			AND EXISTS (
				SELECT 1 FROM lottery_draws draw
				JOIN lottery_issues issue ON issue.game_id = draw.game_id AND issue.issue = draw.issue
				WHERE draw.id = message.reference_id AND draw.game_id = message.game_id
				  AND issue.status = 'settled' AND issue.last_error = '' AND issue.settled_at IS NOT NULL
				  AND EXISTS (
					SELECT 1 FROM lottery_draws newer
					WHERE newer.game_id = draw.game_id
					  AND (newer.draw_at, newer.issue) > (draw.draw_at, draw.issue)
				  )
				  AND NOT EXISTS (
					SELECT 1 FROM lottery_bets unsettled
					WHERE unsettled.workspace_id = message.workspace_id
					  AND unsettled.room_scope = message.room_scope
					  AND unsettled.game_id = draw.game_id AND unsettled.issue = draw.issue
					  AND (unsettled.status = 'pending' OR unsettled.reconciliation_status <> 'normal')
				  )
			)
		)
	)`

func validateLifecyclePurgeDays(dataClass string, value *int) error {
	if value == nil {
		return nil
	}
	if *value < 0 || *value > 3650 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "永久清理等待天数应为 0–3650 天，0 表示不自动永久删除")
	}
	if *value > 0 && dataClass != lifecycle.ClassGameChatMessages {
		return apperrors.NewBusinessError("INVALID_REQUEST", "只有游戏房展示记录支持自动永久清理")
	}
	return nil
}

// All three chat classes share one physical table. Restore the frozen task's
// exact rows once, then report their original classes; never reclassify them
// from mutable robot/game metadata at restore time.
func classifyRestoredChatResults(restored []CleanupResultItem, resultJSON string) ([]CleanupResultItem, error) {
	var original []CleanupResultItem
	if err := json.Unmarshal([]byte(resultJSON), &original); err != nil {
		return nil, err
	}
	result := make([]CleanupResultItem, 0, len(restored)+2)
	for _, item := range restored {
		if item.DataClass != lifecycle.ClassChatMessages {
			result = append(result, item)
			continue
		}
		var classified int64
		for _, source := range original {
			switch source.DataClass {
			case lifecycle.ClassChatMessages, lifecycle.ClassRobotChatMessages, lifecycle.ClassGameChatMessages:
				result = append(result, CleanupResultItem{DataClass: source.DataClass, Action: "restore_soft_delete", AffectedCount: source.AffectedCount})
				classified += source.AffectedCount
			}
		}
		if classified != item.AffectedCount {
			return nil, apperrors.NewBusinessError("RESTORE_INCOMPLETE", "聊天恢复分类数量不一致，本次操作已回滚")
		}
	}
	return result, nil
}
