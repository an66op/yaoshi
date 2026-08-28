package agent

import (
	"backend/data/models/user"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentIdentityComesOnlyFromAuthenticatedAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest("GET", "/api/agent/games?agent_id=999&workspace_id=999&room_scope=agent:999", nil)
	context.Set("agent_user", user.User{UserID: 41, WorkspaceID: 7, Username: "room-41"})
	context.Set("agent_id", uint64(999))
	context.Set("workspace_id", uint64(999))
	context.Set("room_scope", "agent:999")

	account, agentID, scope, ok := agentIdentity(context)
	if !ok {
		t.Fatal("authenticated agent identity was rejected")
	}
	if agentID != 41 || scope != "agent:41" || account.WorkspaceID != 7 {
		t.Fatalf("identity = id %d, scope %q, workspace %d; request selectors must not override the authenticated account", agentID, scope, account.WorkspaceID)
	}
}

func TestAgentIdentityRejectsMissingOrWrongContextType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []any{nil, uint64(41), user.User{}, user.User{UserID: 41}} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		if value != nil {
			context.Set("agent_user", value)
		}
		if _, _, _, ok := agentIdentity(context); ok {
			t.Fatalf("invalid agent identity %#v was accepted", value)
		}
		if response.Code != 401 {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	}
}
