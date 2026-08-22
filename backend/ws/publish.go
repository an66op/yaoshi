package ws

// NotifyDraw broadcasts a lottery draw update.
func NotifyDraw(gameID, issue string, numbers []int) {
	Publish(Event{Type: "draw_update", Data: map[string]any{
		"game_id": gameID, "issue": issue, "numbers": numbers,
	}})
}

// NotifyBetFeed prompts clients to refresh in-room bet feed.
func NotifyBetFeed(gameID, issue string) {
	Publish(Event{Type: "bet_feed", Data: map[string]any{
		"game_id": gameID, "issue": issue,
	}})
}

// NotifyChat tells only room members to reload their allowed message history.
// The message body intentionally never travels in the push event.
func NotifyChat(userIDs []uint64, roomType, scope string, messageID uint64) {
	PublishToUsers(userIDs, Event{Type: "chat_message", Data: map[string]any{
		"room_type": roomType, "scope": scope, "message_id": messageID,
	}})
}

// NotifyUser sends a user-scoped event (notification, balance, etc.).
func NotifyUser(userID uint64, eventType string, data any) {
	PublishToUser(userID, Event{Type: eventType, Data: data})
}
