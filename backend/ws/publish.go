package ws

// NotifyDraw broadcasts a lottery draw update.
func NotifyDraw(gameID, issue string, numbers []int) {
	Publish(Event{Type: "draw_update", RoomScope: "*", GameID: gameID, Issue: issue, Data: map[string]any{
		"game_id": gameID, "issue": issue, "numbers": numbers,
	}})
}

// NotifyBetFeed prompts only members of the originating room to refresh its
// live betting feed. Broadcasting this event globally would leak the presence
// and amount of bets across rooms.
func NotifyBetFeed(userIDs []uint64, gameID, issue, scope string) {
	PublishToUsers(userIDs, Event{Type: "bet_feed", RoomScope: scope, GameID: gameID, Issue: issue, Data: map[string]any{
		"game_id": gameID, "issue": issue, "scope": scope, "room_scope": scope,
	}})
}

// NotifyChat tells only room members to reload their allowed message history.
// The message body intentionally never travels in the push event.
func NotifyChat(userIDs []uint64, roomType, roomScope, gameID, scope string, messageID uint64) {
	PublishToUsers(userIDs, Event{Type: "chat_message", RoomScope: roomScope, GameID: gameID, Data: map[string]any{
		"room_type": roomType, "room_scope": roomScope, "game_id": gameID,
		"scope": scope, "message_id": messageID,
	}})
}

// NotifyUser sends a user-scoped event (notification, balance, etc.).
func NotifyUser(userID uint64, eventType string, data any) {
	PublishToUser(userID, Event{Type: eventType, Data: data})
}
