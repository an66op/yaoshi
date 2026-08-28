package ws

// NotifyDraw broadcasts a lottery draw update.
func NotifyDraw(gameID, issue string, numbers []int) {
	Publish(Event{Type: "draw_update", RoomScope: "*", GameID: gameID, Issue: issue, Data: map[string]any{
		"game_id": gameID, "issue": issue, "numbers": numbers,
	}})
}

// NotifyGameCatalogChanged invalidates the member-facing game catalogue.
// A non-zero workspaceID is intentionally delivered only to sockets whose
// authenticated workspace matches that room. Platform catalogue changes use
// workspaceID=0 because they affect every room. roomCode is included so the
// browser can also reject a stale frame received around a room switch.
func NotifyGameCatalogChanged(workspaceID uint64, roomScope, roomCode, gameID string, enabled bool) {
	Publish(Event{
		Type:        "game_catalog_update",
		WorkspaceID: workspaceID,
		RoomScope:   roomScope,
		GameID:      gameID,
		Data: map[string]any{
			"workspace_id": workspaceID,
			"room_scope":   roomScope,
			"room_code":    roomCode,
			"game_id":      gameID,
			"enabled":      enabled,
		},
	})
}

// NotifyBetFeed prompts only members of the originating room to refresh its
// live betting feed. Broadcasting this event globally would leak the presence
// and amount of bets across rooms.
func NotifyBetFeed(userIDs []uint64, workspaceID uint64, gameID, issue, scope string) {
	PublishToUsers(userIDs, Event{Type: "bet_feed", WorkspaceID: workspaceID, RoomScope: scope, GameID: gameID, Issue: issue, Data: map[string]any{
		"workspace_id": workspaceID, "game_id": gameID, "issue": issue, "scope": scope, "room_scope": scope,
	}})
}

// NotifyChat tells only authorized recipients to reload their allowed message
// history. deliveryWorkspaceID binds the frame to each recipient's current
// socket; sourceWorkspaceID identifies the room that owns the message. These
// differ for platform admins viewing a room workspace.
func NotifyChat(userIDs []uint64, deliveryWorkspaceID, sourceWorkspaceID uint64, roomType, roomScope, gameID, scope string, messageID uint64, operation, senderKind, messageType string) {
	PublishToUsers(userIDs, Event{Type: "chat_message", WorkspaceID: deliveryWorkspaceID, RoomScope: roomScope, GameID: gameID, Data: map[string]any{
		"workspace_id": sourceWorkspaceID, "source_workspace_id": sourceWorkspaceID,
		"room_type": roomType, "room_scope": roomScope, "game_id": gameID,
		"scope": scope, "message_id": messageID, "operation": operation,
		"sender_kind": senderKind, "is_staff": senderKind == "staff", "message_type": messageType,
	}})
}

// NotifyUser sends a user-scoped event (notification, balance, etc.).
func NotifyUser(userID uint64, eventType string, data any) {
	PublishToUser(userID, Event{Type: eventType, Data: data})
}
