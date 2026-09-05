package tenant

import (
	"backend/data/models/user"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTenantPlanRoomComesOnlyFromAuthenticatedAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest("GET", "/api/tenant/plans?workspace_id=999", nil)
	context.Set("tenant_user", user.User{UserID: 52, WorkspaceID: 7, Role: "tenant"})
	context.Set("workspace_id", uint64(999))

	account, ok := tenantRoomAccount(context)
	if !ok || account.WorkspaceID != 7 {
		t.Fatalf("tenant plan scope = %#v; want authenticated workspace 7", account)
	}
}

func TestTenantIdentityIgnoresRequestScopeSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest("GET", "/api/tenant/applications?tenant_id=999&workspace_id=999&room_scope=agent:999", nil)
	context.Set("tenant_id", uint64(52))
	context.Set("workspace_id", uint64(999))

	id, ok := tenantID(context)
	if !ok {
		t.Fatal("authenticated tenant identity was rejected")
	}
	if id != 52 {
		t.Fatalf("tenant id = %d, want authenticated id 52", id)
	}
}

func TestTenantHandlerDoesNotExportSubordinateAgentBusiness(t *testing.T) {
	typeOfHandler := reflect.TypeOf(&WorkspaceHandler{})
	for _, method := range []string{
		"RoomDashboard", "RoomUsers", "SetRoomUserStatus", "AdjustRoomUserBalance",
		"RoomBets", "RoomApplications", "ReviewRoomApplication", "RoomOperatingReport",
		"RoomConversations", "RoomMessages", "ReplyRoomChat", "SendRoomRedPacket",
		"RoomRobotStatus", "RunRoomRobot", "SetLotteryRoomStatus",
	} {
		if _, exposed := typeOfHandler.MethodByName(method); exposed {
			t.Fatalf("tenant handler must not export subordinate agent business method %s", method)
		}
	}
	if _, allowed := typeOfHandler.MethodByName("UpdateRoomSettings"); !allowed {
		t.Fatal("tenant must retain agent account room-name/logo management")
	}
}

func TestTenantIdentityRejectsMissingOrWrongContextType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []any{nil, "52", uint64(0)} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		if value != nil {
			context.Set("tenant_id", value)
		}
		if _, ok := tenantID(context); ok {
			t.Fatalf("invalid tenant identity %#v was accepted", value)
		}
		if response.Code != 401 {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	}
}
