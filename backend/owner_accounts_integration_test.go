package main

import (
	"backend/captcha"
	"backend/cluster"
	"backend/config"
	usercontroller "backend/controller/user"
	"backend/data/models/chat"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/middleware"
	"backend/services"
	"backend/sessionauth"
	"backend/utils"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// These tests reuse the guarded, empty wangzhe_catalog_test database and its
// rollback-only fixture. They never create accounts in the development DB.
func TestOwnerAccountsFreshPostgresCreateAndLogin(t *testing.T) {
	db := catalogTestDatabase(t)
	redisServer := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: redisServer.Addr(), Prefix: "owner-login-fixture"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Init(context.Background(), cluster.Options{}) })
	const password = "OwnerFixture#2026_a9Z"
	tenant, err := services.NewTenantAdminService(db).Create(services.TenantPayload{
		Username: "owner_tenant", Password: password, Nickname: "租户管理员", RoomCode: "77601", Status: 1,
	})
	if err != nil {
		t.Fatal("create tenant and administrator account:", err)
	}
	directAgent, err := services.NewAgentAdminService(db).Create(services.CreateAgentInput{
		Username: "owner_direct_agent", Password: password, Nickname: "直属代理管理员", RoomCode: "77602", Status: 1,
	})
	if err != nil {
		t.Fatal("create direct agent and administrator account:", err)
	}
	tenantAgent, err := services.NewAgentAdminService(db).CreateForTenant(tenant.ID, services.CreateAgentInput{
		Username: "owner_tenant_agent", Password: password, Nickname: "租户下代理管理员", RoomCode: "77603", Status: 1,
	})
	if err != nil {
		t.Fatal("create tenant agent and administrator account:", err)
	}

	utils.InitJWT("isolated-owner-account-test-signing-key", 3600)
	t.Cleanup(func() { utils.InitJWT("", 0) })
	config.Config.JWT.Expire = 3600
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	auth := usercontroller.NewAuthHandler(db)
	engine.POST("/api/login", auth.Login)
	engine.GET("/api/session", middleware.AuthMiddleware(), auth.Me)
	probe := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"workspace_id": c.GetUint64("workspace_id")})
	}
	engine.GET("/api/admin/owner-probe", middleware.AuthMiddleware(), middleware.AdminMiddleware(db), probe)
	engine.GET("/api/tenant/owner-probe", middleware.AuthMiddleware(), middleware.TenantMiddleware(db), probe)
	engine.GET("/api/agent/owner-probe", middleware.AuthMiddleware(), middleware.AgentMiddleware(db), probe)

	for _, tc := range []struct {
		id, workspaceID uint64
		username, role  string
	}{
		{tenant.ID, tenant.WorkspaceID, tenant.Username, "tenant"},
		{directAgent.ID, directAgent.WorkspaceID, directAgent.Username, "agent"},
		{tenantAgent.ID, tenantAgent.WorkspaceID, tenantAgent.Username, "agent"},
	} {
		t.Run(tc.username, func(t *testing.T) {
			var stored user.User
			if err := db.First(&stored, tc.id).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Role != tc.role || stored.Status != 1 || stored.WorkspaceID == 0 || stored.WorkspaceID != tc.workspaceID {
				t.Fatalf("owner account not bound to its active role/workspace: role=%s status=%d workspace=%d", stored.Role, stored.Status, stored.WorkspaceID)
			}
			if stored.Password == password || !utils.CheckPasswordHash(password, stored.Password) {
				t.Fatal("creation did not persist the chosen password as a valid hash")
			}
			room := catalogTestRoom(t, db, tc.id)
			if room.ID != stored.WorkspaceID || room.Type != tc.role || room.OwnerUserID != tc.id {
				t.Fatal("created account and room ownership do not match")
			}
			// Being able to manage the new room does not enable its games.
			assertCatalogRoom(t, db, room)

			// A test-only Redis fixture exercises the real production verifier;
			// there is no test-mode bypass or answer-disclosing endpoint.
			const captchaID, captchaCode = "0123456789abcdef0123456789abcdef", "0123"
			sum := sha256.Sum256([]byte(captchaID + "\x00" + captcha.Management + "\x00192.0.2.1\x00" + captchaCode))
			redisServer.Set(cluster.Key("captcha", captchaID), hex.EncodeToString(sum[:]))
			redisServer.SetTTL(cluster.Key("captcha", captchaID), captcha.Lifetime)
			payload, _ := json.Marshal(map[string]string{"username": strings.ToUpper(tc.username), "password": password, "captcha_id": captchaID, "captcha_code": captchaCode})
			request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(string(payload)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("new owner cannot log in using only its username/password: status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), password) || strings.Contains(response.Body.String(), stored.Password) {
				t.Fatal("login response exposed credentials")
			}
			var login struct {
				Data struct {
					User struct {
						Role string `json:"role"`
					} `json:"user"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil || login.Data.User.Role != tc.role {
				t.Fatalf("login returned wrong role: %q, %v", login.Data.User.Role, err)
			}
			var session *http.Cookie
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == sessionauth.ManagementCookieName {
					session = cookie
				}
			}
			if session == nil || session.Value == "" || !session.HttpOnly || session.Path != "/api" {
				t.Fatal("login did not issue an HttpOnly management session")
			}
			for _, path := range []string{"/api/session", "/api/tenant/owner-probe", "/api/agent/owner-probe", "/api/admin/owner-probe"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.AddCookie(session)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				want := http.StatusForbidden
				if path == "/api/session" || path == "/api/"+tc.role+"/owner-probe" {
					want = http.StatusOK
				}
				if response.Code != want {
					t.Fatalf("%s with role %s: status=%d want=%d", path, tc.role, response.Code, want)
				}
				if path == "/api/"+tc.role+"/owner-probe" {
					var body struct {
						WorkspaceID uint64 `json:"workspace_id"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.WorkspaceID != room.ID {
						t.Fatalf("management request escaped the new owner's room: workspace=%d, %v", body.WorkspaceID, err)
					}
				}
			}
		})
	}
}

func TestOwnerAccountsFreshPostgresRollbackRoomInitializationFailure(t *testing.T) {
	db := catalogTestDatabase(t)
	const callbackName = "owner_account_test:fail_room_settings"
	failure := errors.New("fixture room settings initialization failure")
	// Fail after the owner, workspace, game defaults and membership have been
	// written. None may survive a failure to initialize the room's settings.
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*settings.SystemConfig); ok && row.RoomName == "rollback_owner_fixture" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	models := []any{&user.User{}, &workspacemodel.Workspace{}, &workspacemodel.Membership{}, &settings.SystemConfig{}, &chat.RoomGameSetting{}, &workspacemodel.RobotSetting{}}
	for _, role := range []string{"tenant", "agent"} {
		t.Run(role, func(t *testing.T) {
			before := make([]int64, len(models))
			for index, model := range models {
				if err := db.Model(model).Count(&before[index]).Error; err != nil {
					t.Fatal(err)
				}
			}
			var err error
			if role == "tenant" {
				_, err = services.NewTenantAdminService(db).Create(services.TenantPayload{
					Username: "rollback_tenant", Password: "RollbackFixture#2026_a9", Nickname: "rollback_owner_fixture", RoomCode: "77611", Status: 1,
				})
			} else {
				_, err = services.NewAgentAdminService(db).Create(services.CreateAgentInput{
					Username: "rollback_agent", Password: "RollbackFixture#2026_a9", Nickname: "rollback_owner_fixture", RoomCode: "77612", Status: 1,
				})
			}
			if !errors.Is(err, failure) {
				t.Fatalf("expected injected room initialization failure, got %v", err)
			}
			for index, model := range models {
				var after int64
				if err := db.Model(model).Count(&after).Error; err != nil {
					t.Fatal(err)
				}
				if after != before[index] {
					t.Fatalf("partially created %s survived room failure: %T count %d -> %d", role, model, before[index], after)
				}
			}
		})
	}
}
