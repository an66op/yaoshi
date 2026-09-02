package api

import (
	"backend/captcha"
	"backend/constants"
	"backend/controller/drawfeed"
	usercontroller "backend/controller/user"
	"backend/lotteryfeed"
	"backend/middleware"
	"backend/services"
	"backend/ws"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LoadRoutes 动态加载所有路由
func LoadRoutes(r *gin.Engine, db *gorm.DB, scheduler *lotteryfeed.Scheduler) {
	LoadRoutesForMode(r, db, scheduler, gin.Mode())
}

func adminRoomActivityRunOnceRoute(handler gin.HandlerFunc) Route {
	return Route{
		Method: "POST", Pattern: "/admin/room-activity/run-once", Handler: handler,
		Middlewares: []gin.HandlerFunc{middleware.RobotRunRateLimit()},
	}
}

// LoadRoutesForMode keeps security-sensitive route availability tied to the
// configured server mode. LoadRoutes remains as a compatibility wrapper for
// tests and local tools that already set Gin's mode explicitly.
func LoadRoutesForMode(r *gin.Engine, db *gorm.DB, scheduler *lotteryfeed.Scheduler, serverMode string) {
	ws.ConfigureSessionDatabase(db)
	h := InitHandlers(db, scheduler)

	methodMap := map[string]func(*gin.RouterGroup, string, ...gin.HandlerFunc) gin.IRoutes{
		"POST":   (*gin.RouterGroup).POST,
		"GET":    (*gin.RouterGroup).GET,
		"PATCH":  (*gin.RouterGroup).PATCH,
		"PUT":    (*gin.RouterGroup).PUT,
		"DELETE": (*gin.RouterGroup).DELETE,
	}

	register := func(groups []RouteGroup) {
		for _, group := range groups {
			g := r.Group(group.Prefix)
			for _, mw := range group.Middlewares {
				g.Use(mw)
			}
			for _, route := range group.Routes {
				handlers := append([]gin.HandlerFunc{}, route.Middlewares...)
				handlers = append(handlers, route.Handler)
				if f, ok := methodMap[route.Method]; ok {
					f(g, route.Pattern, handlers...)
				}
			}
		}
	}

	publicAuthRoutes := []Route{
		{Method: "GET", Pattern: "/login/captcha", Handler: usercontroller.LoginCaptcha(captcha.Management), Middlewares: []gin.HandlerFunc{middleware.RequireSharedLoginCaptcha(serverMode), middleware.LoginCaptchaRateLimit()}},
		{Method: "POST", Pattern: "/login", Handler: h.AuthHandler.Login, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit(), middleware.RequireSharedLoginCaptcha(serverMode)}},
		{Method: "POST", Pattern: "/logout", Handler: h.AuthHandler.Logout},
		{Method: "GET", Pattern: "/session", Handler: h.AuthHandler.Me, Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware()}},
		{Method: "POST", Pattern: "/session/refresh", Handler: h.AuthHandler.Refresh, Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware()}},
		{Method: "POST", Pattern: "/member/login", Handler: h.MemberHandler.Login, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit(), middleware.RequireSharedLoginCaptcha(serverMode)}},
		{Method: "GET", Pattern: "/member/login/captcha", Handler: usercontroller.LoginCaptcha(captcha.Member), Middlewares: []gin.HandlerFunc{middleware.RequireSharedLoginCaptcha(serverMode), middleware.LoginCaptchaRateLimit()}},
		{Method: "POST", Pattern: "/member/register", Handler: h.MemberHandler.Register, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()}},
		{Method: "POST", Pattern: "/member/logout", Handler: h.MemberHandler.Logout},
		{Method: "GET", Pattern: "/ws", Handler: ws.HandleConnect, Middlewares: []gin.HandlerFunc{middleware.WSConnectRateLimit()}},
	}
	if serverMode != gin.ReleaseMode {
		// /api/register is the legacy platform-account registration endpoint.
		// It remains available to debug/test environments only; member signup
		// continues to use /api/member/register in every mode.
		publicAuthRoutes = append([]Route{{
			Method: "POST", Pattern: "/register", Handler: h.AuthHandler.Register,
			Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()},
		}}, publicAuthRoutes...)
	}

	register([]RouteGroup{
		{
			Prefix: "/health",
			Routes: []Route{{Method: "GET", Pattern: "", Handler: func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) }}},
		},
		{
			Prefix: "/ready",
			Routes: []Route{{Method: "GET", Pattern: "", Handler: readinessHandler(db)}},
		},
		{
			Prefix: "/api",
			Routes: publicAuthRoutes,
		},
		{
			Prefix:      "/api/member",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.MemberMiddleware(db)},
			Routes: []Route{
				{Method: "POST", Pattern: "/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "POST", Pattern: "/session/refresh", Handler: h.MemberHandler.Refresh},
				{Method: "GET", Pattern: "/me", Handler: h.MemberHandler.Me},
				{Method: "GET", Pattern: "/games", Handler: h.MemberHandler.Games},
				{Method: "GET", Pattern: "/plans", Handler: h.MemberHandler.ListPlans},
				{Method: "GET", Pattern: "/plans/:gameID", Handler: h.MemberHandler.PlanDetail},
				{Method: "POST", Pattern: "/plans/:gameID/activate", Handler: h.MemberHandler.ActivatePlanStream, Middlewares: []gin.HandlerFunc{middleware.PlanActivationRateLimit()}},
				{Method: "POST", Pattern: "/room/join", Handler: h.MemberHandler.JoinRoom},
				{Method: "GET", Pattern: "/room/history", Handler: h.MemberHandler.RoomHistory},
				{Method: "GET", Pattern: "/bets", Handler: h.MemberHandler.ListBets},
				{Method: "POST", Pattern: "/bets", Handler: h.MemberHandler.PlaceBet, Middlewares: []gin.HandlerFunc{middleware.MemberBetRateLimit()}},
				{Method: "POST", Pattern: "/bets/cancel-current", Handler: h.MemberHandler.CancelCurrentIssueBets},
				{Method: "POST", Pattern: "/bets/:id/cancel", Handler: h.MemberHandler.CancelBet},
				{Method: "GET", Pattern: "/applications", Handler: h.MemberHandler.ListApplications},
				{Method: "POST", Pattern: "/applications", Handler: h.MemberHandler.CreateApplication},
				{Method: "GET", Pattern: "/balance-history", Handler: h.MemberHandler.BalanceHistory},
				{Method: "GET", Pattern: "/wallet/channels", Handler: h.MemberHandler.WalletChannels},
				{Method: "GET", Pattern: "/payment-accounts", Handler: h.MemberHandler.ListPaymentAccounts},
				{Method: "POST", Pattern: "/payment-accounts", Handler: h.MemberHandler.CreatePaymentAccount},
				{Method: "DELETE", Pattern: "/payment-accounts/:id", Handler: h.MemberHandler.DeletePaymentAccount},
				{Method: "GET", Pattern: "/wallet/summary", Handler: h.MemberHandler.WalletSummary},
				{Method: "GET", Pattern: "/wallet/rebate", Handler: h.MemberHandler.RebatePreview},
				{Method: "GET", Pattern: "/invite", Handler: h.MemberHandler.InviteInfo},
				{Method: "GET", Pattern: "/entertainment", Handler: h.MemberHandler.ListEntertainment},
				{Method: "POST", Pattern: "/entertainment/:code/launch", Handler: h.MemberHandler.LaunchEntertainment},
				{Method: "POST", Pattern: "/password", Handler: h.MemberHandler.ChangePassword},
				{Method: "PATCH", Pattern: "/nickname", Handler: h.MemberHandler.UpdateNickname},
				{Method: "PATCH", Pattern: "/avatar", Handler: h.MemberHandler.UpdateAvatar},
				{Method: "GET", Pattern: "/chat/preview", Handler: h.MemberHandler.ChatPreview},
				{Method: "GET", Pattern: "/chat/messages", Handler: h.MemberHandler.ListChatMessages},
				{Method: "POST", Pattern: "/chat/messages", Handler: h.MemberHandler.PostChatMessage},
				{Method: "POST", Pattern: "/chat/commands", Handler: h.MemberHandler.PostChatCommand, Middlewares: []gin.HandlerFunc{middleware.MemberBetRateLimit()}},
				{Method: "GET", Pattern: "/chat/redpackets/available", Handler: h.MemberHandler.LatestClaimableChatRedPacket},
				{Method: "POST", Pattern: "/chat/redpackets/:id/claim", Handler: h.MemberHandler.ClaimChatRedPacket},
				{Method: "GET", Pattern: "/room/settings", Handler: h.MemberHandler.RoomSettings},
				{Method: "GET", Pattern: "/games/:id/assistant", Handler: h.MemberHandler.AssistantStatus},
				{Method: "GET", Pattern: "/games/:id/assistant/history", Handler: h.MemberHandler.AssistantBetHistory},
				{Method: "POST", Pattern: "/games/:id/assistant/bets", Handler: h.MemberHandler.AssistantBet, Middlewares: []gin.HandlerFunc{middleware.MemberBetRateLimit()}},
				{Method: "POST", Pattern: "/games/:id/web-bets", Handler: h.MemberHandler.WebBets, Middlewares: []gin.HandlerFunc{middleware.MemberBetRateLimit()}},
				{Method: "GET", Pattern: "/games/:id/odds", Handler: h.MemberHandler.GameOdds},
				{Method: "GET", Pattern: "/games/:id/feed", Handler: h.MemberHandler.GameFeed},
				{Method: "GET", Pattern: "/activities", Handler: h.MemberHandler.ListActivities},
				{Method: "GET", Pattern: "/activities/:id/status", Handler: h.MemberHandler.ActivityStatus},
				{Method: "POST", Pattern: "/activities/:id/checkin", Handler: h.MemberHandler.CheckIn},
				{Method: "POST", Pattern: "/activities/:id/redpacket", Handler: h.MemberHandler.ClaimRedPacket},
				{Method: "GET", Pattern: "/notifications", Handler: h.MemberHandler.ListNotifications},
				{Method: "GET", Pattern: "/notifications/unread", Handler: h.MemberHandler.NotificationUnread},
				{Method: "POST", Pattern: "/notifications/:id/read", Handler: h.MemberHandler.MarkNotificationRead},
				{Method: "POST", Pattern: "/notifications/read-all", Handler: h.MemberHandler.MarkAllNotificationsRead},
			},
		},
		{
			Prefix:      "/api/tenant",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.TenantMiddleware(db), middleware.PrivilegedAudit(db, "tenant")},
			Routes: []Route{
				{Method: "POST", Pattern: "/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "GET", Pattern: "/me", Handler: h.TenantWorkspaceHandler.Me},
				{Method: "GET", Pattern: "/dashboard", Handler: h.TenantWorkspaceHandler.Dashboard},
				{Method: "GET", Pattern: "/room/dashboard", Handler: h.TenantWorkspaceHandler.DirectRoomDashboard},
				{Method: "GET", Pattern: "/games", Handler: h.TenantWorkspaceHandler.DirectGames},
				{Method: "PATCH", Pattern: "/games/:gameID/status", Handler: h.TenantWorkspaceHandler.SetDirectGameStatus},
				{Method: "GET", Pattern: "/trading", Handler: h.TenantWorkspaceHandler.DirectTrading},
				{Method: "PUT", Pattern: "/trading", Handler: h.TenantWorkspaceHandler.UpdateDirectTrading},
				{Method: "PATCH", Pattern: "/room/settings", Handler: h.TenantWorkspaceHandler.UpdateDirectRoomProfile},
				{Method: "GET", Pattern: "/reports/catalog", Handler: h.TenantWorkspaceHandler.ReportCatalog},
				{Method: "GET", Pattern: "/reports/:report_key", Handler: h.TenantWorkspaceHandler.ReportCenter},
				{Method: "GET", Pattern: "/settings", Handler: h.TenantWorkspaceHandler.SystemSettings},
				{Method: "GET", Pattern: "/menu-template", Handler: h.TenantWorkspaceHandler.MenuTemplate},
				{Method: "PUT", Pattern: "/settings", Handler: h.TenantWorkspaceHandler.UpdateSystemSettings},
				{Method: "GET", Pattern: "/agents", Handler: h.TenantWorkspaceHandler.Agents},
				{Method: "POST", Pattern: "/agents", Handler: h.TenantWorkspaceHandler.CreateAgent},
				{Method: "PATCH", Pattern: "/agents/:id", Handler: h.TenantWorkspaceHandler.UpdateAgent},
				{Method: "POST", Pattern: "/agents/:id/reset-password", Handler: h.TenantWorkspaceHandler.ResetAgentPassword},
				{Method: "PATCH", Pattern: "/rooms/:agentID/settings", Handler: h.TenantWorkspaceHandler.UpdateRoomSettings},
				{Method: "GET", Pattern: "/users", Handler: h.TenantWorkspaceHandler.DirectUsers},
				{Method: "PATCH", Pattern: "/users/:id/status", Handler: h.TenantWorkspaceHandler.SetDirectUserStatus},
				{Method: "POST", Pattern: "/users/:id/balance", Handler: h.TenantWorkspaceHandler.AdjustDirectUserBalance},
				{Method: "GET", Pattern: "/users/:id/trading", Handler: h.TenantWorkspaceHandler.DirectUserTrading},
				{Method: "PUT", Pattern: "/users/:id/trading", Handler: h.TenantWorkspaceHandler.UpdateDirectUserTrading},
				{Method: "GET", Pattern: "/bets", Handler: h.TenantWorkspaceHandler.DirectBets},
				{Method: "GET", Pattern: "/applications", Handler: h.TenantWorkspaceHandler.DirectApplications},
				{Method: "GET", Pattern: "/applications/stats", Handler: h.TenantWorkspaceHandler.DirectApplicationStats},
				{Method: "POST", Pattern: "/applications/:id/review", Handler: h.TenantWorkspaceHandler.ReviewDirectApplication},
				{Method: "GET", Pattern: "/chat/conversations", Handler: h.TenantWorkspaceHandler.DirectConversations},
				{Method: "GET", Pattern: "/chat/messages", Handler: h.TenantWorkspaceHandler.DirectMessages},
				{Method: "GET", Pattern: "/chat/unread", Handler: h.TenantWorkspaceHandler.ChatUnread},
				{Method: "POST", Pattern: "/chat/read", Handler: h.TenantWorkspaceHandler.MarkChatRead},
				{Method: "POST", Pattern: "/chat/messages", Handler: h.TenantWorkspaceHandler.ReplyDirectChat},
				{Method: "POST", Pattern: "/chat/redpackets", Handler: h.TenantWorkspaceHandler.SendDirectRedPacket},
				{Method: "PATCH", Pattern: "/chat/lottery-rooms/:gameID/status", Handler: h.TenantWorkspaceHandler.SetDirectLotteryRoomStatus},
				{Method: "GET", Pattern: "/robots/settings", Handler: h.TenantWorkspaceHandler.RobotSetting},
				{Method: "GET", Pattern: "/robots", Handler: h.TenantWorkspaceHandler.Robots},
				{Method: "POST", Pattern: "/robots/reset", Handler: h.TenantWorkspaceHandler.ResetRobots},
				{Method: "PATCH", Pattern: "/robots/:id", Handler: h.TenantWorkspaceHandler.UpdateRobot},
				{Method: "PATCH", Pattern: "/robots/settings", Handler: h.TenantWorkspaceHandler.UpdateRobotSetting},
				{Method: "POST", Pattern: "/robots/run-once", Handler: h.TenantWorkspaceHandler.RunDirectRobot, Middlewares: []gin.HandlerFunc{middleware.RobotRunRateLimit()}},
				{Method: "GET", Pattern: "/activities", Handler: h.TenantWorkspaceHandler.Activities},
				{Method: "POST", Pattern: "/activities", Handler: h.TenantWorkspaceHandler.CreateActivity},
				{Method: "PUT", Pattern: "/activities/:id", Handler: h.TenantWorkspaceHandler.UpdateActivity},
				{Method: "PATCH", Pattern: "/activities/:id/status", Handler: h.TenantWorkspaceHandler.SetActivityStatus},
				{Method: "DELETE", Pattern: "/activities/:id", Handler: h.TenantWorkspaceHandler.DeleteActivity},
				{Method: "GET", Pattern: "/plans", Handler: h.TenantWorkspaceHandler.Plans},
				{Method: "POST", Pattern: "/plans", Handler: h.TenantWorkspaceHandler.CreatePlan},
				{Method: "PUT", Pattern: "/plans/:id", Handler: h.TenantWorkspaceHandler.UpdatePlan},
				{Method: "DELETE", Pattern: "/plans/:id", Handler: h.TenantWorkspaceHandler.DeletePlan},
				{Method: "GET", Pattern: "/wallet/channels", Handler: h.TenantWorkspaceHandler.WalletChannels},
				{Method: "POST", Pattern: "/wallet/channels", Handler: h.TenantWorkspaceHandler.CreateWalletChannel},
				{Method: "PATCH", Pattern: "/wallet/channels/:id", Handler: h.TenantWorkspaceHandler.UpdateWalletChannel},
				{Method: "PATCH", Pattern: "/wallet/channels/:id/status", Handler: h.TenantWorkspaceHandler.SetWalletChannelStatus},
				{Method: "DELETE", Pattern: "/wallet/channels/:id", Handler: h.TenantWorkspaceHandler.DeleteWalletChannel},
			},
		},
		{
			Prefix:      "/api/agent",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.AgentMiddleware(db), middleware.PrivilegedAudit(db, "agent")},
			Routes: []Route{
				{Method: "POST", Pattern: "/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "GET", Pattern: "/me", Handler: h.AgentWorkspaceHandler.Me},
				{Method: "GET", Pattern: "/dashboard", Handler: h.AgentWorkspaceHandler.Dashboard},
				{Method: "GET", Pattern: "/games", Handler: h.AgentWorkspaceHandler.Games},
				{Method: "PATCH", Pattern: "/games/:gameID/status", Handler: h.AgentWorkspaceHandler.SetGameStatus},
				{Method: "PATCH", Pattern: "/room/settings", Handler: h.AgentWorkspaceHandler.UpdateRoomSettings},
				{Method: "GET", Pattern: "/settings", Handler: h.AgentWorkspaceHandler.SystemSettings},
				{Method: "GET", Pattern: "/menu-template", Handler: h.AgentWorkspaceHandler.MenuTemplate},
				{Method: "PUT", Pattern: "/settings", Handler: h.AgentWorkspaceHandler.UpdateSystemSettings},
				{Method: "GET", Pattern: "/reports/operating", Handler: h.AgentWorkspaceHandler.OperatingReport},
				{Method: "GET", Pattern: "/reports/profit-shares", Handler: h.AgentWorkspaceHandler.ProfitShares},
				{Method: "GET", Pattern: "/reports/catalog", Handler: h.AgentWorkspaceHandler.ReportCatalog},
				{Method: "GET", Pattern: "/reports/:report_key", Handler: h.AgentWorkspaceHandler.ReportCenter},
				{Method: "GET", Pattern: "/users", Handler: h.AgentWorkspaceHandler.Users},
				{Method: "PATCH", Pattern: "/users/:id/status", Handler: h.AgentWorkspaceHandler.SetUserStatus},
				{Method: "POST", Pattern: "/users/:id/balance", Handler: h.AgentWorkspaceHandler.AdjustBalance},
				{Method: "GET", Pattern: "/users/:id/trading", Handler: h.AgentWorkspaceHandler.UserTrading},
				{Method: "PUT", Pattern: "/users/:id/trading", Handler: h.AgentWorkspaceHandler.UpdateUserTrading},
				{Method: "GET", Pattern: "/bets", Handler: h.AgentWorkspaceHandler.Bets},
				{Method: "GET", Pattern: "/applications", Handler: h.AgentWorkspaceHandler.Applications},
				{Method: "GET", Pattern: "/applications/stats", Handler: h.AgentWorkspaceHandler.ApplicationStats},
				{Method: "POST", Pattern: "/applications/:id/review", Handler: h.AgentWorkspaceHandler.ReviewApplication},
				{Method: "GET", Pattern: "/trading", Handler: h.AgentWorkspaceHandler.Trading},
				{Method: "PUT", Pattern: "/trading", Handler: h.AgentWorkspaceHandler.UpdateTrading},
				{Method: "GET", Pattern: "/chat/conversations", Handler: h.AgentWorkspaceHandler.Conversations},
				{Method: "GET", Pattern: "/chat/messages", Handler: h.AgentWorkspaceHandler.Messages},
				{Method: "GET", Pattern: "/chat/unread", Handler: h.AgentWorkspaceHandler.ChatUnread},
				{Method: "POST", Pattern: "/chat/read", Handler: h.AgentWorkspaceHandler.MarkChatRead},
				{Method: "POST", Pattern: "/chat/messages", Handler: h.AgentWorkspaceHandler.Reply},
				{Method: "POST", Pattern: "/chat/redpackets", Handler: h.AgentWorkspaceHandler.SendRedPacket},
				{Method: "PATCH", Pattern: "/chat/lottery-rooms/:gameID/status", Handler: h.AgentWorkspaceHandler.SetLotteryRoomStatus},
				{Method: "GET", Pattern: "/robots/status", Handler: h.AgentWorkspaceHandler.RobotStatus},
				{Method: "GET", Pattern: "/robots", Handler: h.AgentWorkspaceHandler.Robots},
				{Method: "POST", Pattern: "/robots/reset", Handler: h.AgentWorkspaceHandler.ResetRobots},
				{Method: "PATCH", Pattern: "/robots/:id", Handler: h.AgentWorkspaceHandler.UpdateRobot},
				{Method: "GET", Pattern: "/robots/settings", Handler: h.AgentWorkspaceHandler.RobotStatus},
				{Method: "PATCH", Pattern: "/robots/settings", Handler: h.AgentWorkspaceHandler.UpdateRobotSetting},
				{Method: "POST", Pattern: "/robots/run-once", Handler: h.AgentWorkspaceHandler.RunRobot, Middlewares: []gin.HandlerFunc{middleware.RobotRunRateLimit()}},
				{Method: "GET", Pattern: "/activities", Handler: h.AgentWorkspaceHandler.Activities},
				{Method: "POST", Pattern: "/activities", Handler: h.AgentWorkspaceHandler.CreateActivity},
				{Method: "PUT", Pattern: "/activities/:id", Handler: h.AgentWorkspaceHandler.UpdateActivity},
				{Method: "PATCH", Pattern: "/activities/:id/status", Handler: h.AgentWorkspaceHandler.SetActivityStatus},
				{Method: "DELETE", Pattern: "/activities/:id", Handler: h.AgentWorkspaceHandler.DeleteActivity},
				{Method: "GET", Pattern: "/plans", Handler: h.AgentWorkspaceHandler.Plans},
				{Method: "POST", Pattern: "/plans", Handler: h.AgentWorkspaceHandler.CreatePlan},
				{Method: "PUT", Pattern: "/plans/:id", Handler: h.AgentWorkspaceHandler.UpdatePlan},
				{Method: "DELETE", Pattern: "/plans/:id", Handler: h.AgentWorkspaceHandler.DeletePlan},
				{Method: "GET", Pattern: "/wallet/channels", Handler: h.AgentWorkspaceHandler.WalletChannels},
				{Method: "POST", Pattern: "/wallet/channels", Handler: h.AgentWorkspaceHandler.CreateWalletChannel},
				{Method: "PATCH", Pattern: "/wallet/channels/:id", Handler: h.AgentWorkspaceHandler.UpdateWalletChannel},
				{Method: "PATCH", Pattern: "/wallet/channels/:id/status", Handler: h.AgentWorkspaceHandler.SetWalletChannelStatus},
				{Method: "DELETE", Pattern: "/wallet/channels/:id", Handler: h.AgentWorkspaceHandler.DeleteWalletChannel},
			},
		},
		{
			Prefix:      "/api",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.AdminMiddleware(db), middleware.PrivilegedAudit(db, "admin")},
			Routes: []Route{
				{Method: "POST", Pattern: "/admin/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "GET", Pattern: "/admin/me", Handler: h.AuthHandler.Me},
				{Method: "GET", Pattern: "/admin/dashboard", Handler: h.DashboardHandler.Dashboard},
				{Method: "GET", Pattern: "/admin/audit-logs", Handler: h.SystemAuditHandler.Logs},
				{Method: "GET", Pattern: "/admin/reconciliation", Handler: h.SystemAuditHandler.Reconciliation},
				{Method: "GET", Pattern: "/admin/data-lifecycle/policies", Handler: h.SystemAuditHandler.RetentionPolicies},
				{Method: "GET", Pattern: "/admin/data-lifecycle/summary", Handler: h.SystemAuditHandler.DataMaintenanceSummary},
				{Method: "PUT", Pattern: "/admin/data-lifecycle/policies/:dataClass", Handler: h.SystemAuditHandler.UpdateRetentionPolicy},
				{Method: "POST", Pattern: "/admin/data-lifecycle/preview", Handler: h.SystemAuditHandler.PreviewCleanup},
				{Method: "POST", Pattern: "/admin/data-lifecycle/execute", Handler: h.SystemAuditHandler.ExecuteCleanup},
				{Method: "GET", Pattern: "/admin/data-lifecycle/runs", Handler: h.SystemAuditHandler.CleanupRuns},
				{Method: "GET", Pattern: "/admin/data-lifecycle/runs/:requestID", Handler: h.SystemAuditHandler.CleanupRun},
				{Method: "GET", Pattern: "/admin/data-lifecycle/runs/:requestID/archives", Handler: h.SystemAuditHandler.CleanupArchives},
				{Method: "POST", Pattern: "/admin/data-lifecycle/runs/:requestID/restore-soft-deleted", Handler: h.SystemAuditHandler.RestoreSoftDeleted},
				{Method: "POST", Pattern: "/admin/data-lifecycle/runs/:requestID/restore-robot-archive", Handler: h.SystemAuditHandler.RestoreRobotArchive},
				{Method: "POST", Pattern: "/admin/reconciliation/recover", Handler: h.SystemAuditHandler.RecoverSettlement},
				{Method: "POST", Pattern: "/admin/reconciliation/bets/:id/refund", Handler: h.SystemAuditHandler.RefundAbnormalBet},
				{Method: "GET", Pattern: "/admin/games", Handler: h.DashboardHandler.Games},
				{Method: "GET", Pattern: "/admin/game-categories", Handler: h.DashboardHandler.GameCategories},
				{Method: "POST", Pattern: "/admin/game-categories", Handler: h.DashboardHandler.CreateGameCategory},
				{Method: "PUT", Pattern: "/admin/game-categories/:id", Handler: h.DashboardHandler.UpdateGameCategory},
				{Method: "DELETE", Pattern: "/admin/game-categories/:id", Handler: h.DashboardHandler.DeleteGameCategory},
				{Method: "POST", Pattern: "/admin/games/sync-target", Handler: h.DashboardHandler.SyncTargetGames},
				{Method: "GET", Pattern: "/admin/games/:id/draws", Handler: h.DashboardHandler.Draws},
				{Method: "POST", Pattern: "/admin/sources/sync", Handler: h.DashboardHandler.SyncOfficialSources},
				{Method: "POST", Pattern: "/admin/sources/:group/test", Handler: h.DashboardHandler.TestOfficialSource},
				{Method: "PATCH", Pattern: "/admin/games/:id/category", Handler: h.DashboardHandler.AssignGameCategory},
				{Method: "PATCH", Pattern: "/admin/games/:id/status", Handler: h.DashboardHandler.UpdateGameStatus},
				{Method: "GET", Pattern: "/admin/users", Handler: h.UserAdminHandler.List},
				{Method: "GET", Pattern: "/admin/users/stats", Handler: h.UserAdminHandler.Stats},
				{Method: "GET", Pattern: "/admin/robot-workspaces", Handler: h.UserAdminHandler.RobotWorkspaces},
				{Method: "GET", Pattern: "/admin/robot-workspaces/:id/games", Handler: h.UserAdminHandler.RobotWorkspaceGames},
				{Method: "GET", Pattern: "/admin/users/:id", Handler: h.UserAdminHandler.Get},
				{Method: "POST", Pattern: "/admin/users", Handler: h.UserAdminHandler.Create},
				{Method: "PATCH", Pattern: "/admin/users/:id", Handler: h.UserAdminHandler.Update},
				{Method: "POST", Pattern: "/admin/robots/reset", Handler: h.UserAdminHandler.ResetRobots},
				{Method: "PATCH", Pattern: "/admin/robots/:id", Handler: h.UserAdminHandler.UpdateRobot},
				{Method: "PATCH", Pattern: "/admin/users/:id/status", Handler: h.UserAdminHandler.SetStatus},
				{Method: "POST", Pattern: "/admin/users/:id/reset-password", Handler: h.UserAdminHandler.ResetPassword},
				{Method: "POST", Pattern: "/admin/users/:id/balance", Handler: h.UserAdminHandler.AdjustBalance},
				{Method: "GET", Pattern: "/admin/users/:id/balance-history", Handler: h.UserAdminHandler.BalanceHistory},
				{Method: "GET", Pattern: "/admin/users/:id/trading", Handler: h.UserAdminHandler.GetTrading},
				{Method: "PUT", Pattern: "/admin/users/:id/trading", Handler: h.UserAdminHandler.UpdateTrading},
				{Method: "GET", Pattern: "/admin/agents", Handler: h.AgentHandler.List},
				{Method: "GET", Pattern: "/admin/tenants", Handler: h.TenantHandler.List},
				{Method: "POST", Pattern: "/admin/tenants", Handler: h.TenantHandler.Create},
				{Method: "PATCH", Pattern: "/admin/tenants/:id", Handler: h.TenantHandler.Update},
				{Method: "GET", Pattern: "/admin/tenants/:id/trading", Handler: h.TenantHandler.GetTrading},
				{Method: "PUT", Pattern: "/admin/tenants/:id/trading", Handler: h.TenantHandler.UpdateTrading},
				{Method: "GET", Pattern: "/admin/tenants/:id/settings", Handler: h.TenantHandler.GetSettings},
				{Method: "PUT", Pattern: "/admin/tenants/:id/settings", Handler: h.TenantHandler.UpdateSettings},
				{Method: "GET", Pattern: "/admin/tenants/:id/games", Handler: h.TenantHandler.Games},
				{Method: "PATCH", Pattern: "/admin/tenants/:id/games/:gameID/status", Handler: h.TenantHandler.SetGameStatus},
				{Method: "POST", Pattern: "/admin/tenants/:id/reset-password", Handler: h.TenantHandler.ResetPassword},
				{Method: "POST", Pattern: "/admin/agents", Handler: h.AgentHandler.Create},
				{Method: "PATCH", Pattern: "/admin/agents/:id", Handler: h.AgentHandler.Update},
				{Method: "GET", Pattern: "/admin/agents/:id/trading", Handler: h.AgentHandler.GetTrading},
				{Method: "PUT", Pattern: "/admin/agents/:id/trading", Handler: h.AgentHandler.UpdateTrading},
				{Method: "GET", Pattern: "/admin/agents/:id/settings", Handler: h.AgentHandler.GetSettings},
				{Method: "PUT", Pattern: "/admin/agents/:id/settings", Handler: h.AgentHandler.UpdateSettings},
				{Method: "GET", Pattern: "/admin/agents/:id/games", Handler: h.AgentHandler.Games},
				{Method: "PATCH", Pattern: "/admin/agents/:id/games/:gameID/status", Handler: h.AgentHandler.SetGameStatus},
				{Method: "POST", Pattern: "/admin/agents/:id/reset-password", Handler: h.AgentHandler.ResetPassword},
				{Method: "POST", Pattern: "/admin/agents/:id/promote", Handler: h.AgentHandler.Promote},
				{Method: "POST", Pattern: "/admin/agents/assign-room", Handler: h.AgentHandler.AssignRoom},
				{Method: "GET", Pattern: "/admin/applications", Handler: h.ApplicationAdminHandler.List},
				{Method: "GET", Pattern: "/admin/applications/stats", Handler: h.ApplicationAdminHandler.Stats},
				{Method: "GET", Pattern: "/admin/applications/:id", Handler: h.ApplicationAdminHandler.Get},
				{Method: "POST", Pattern: "/admin/applications", Handler: h.ApplicationAdminHandler.Create},
				{Method: "POST", Pattern: "/admin/applications/:id/review", Handler: h.ApplicationAdminHandler.Review},
				{Method: "GET", Pattern: "/admin/reports/financial", Handler: h.ReportHandler.Financial},
				{Method: "GET", Pattern: "/admin/reports/operating", Handler: h.ReportHandler.Operating},
				{Method: "GET", Pattern: "/admin/reports/profit-shares", Handler: h.ReportHandler.ProfitShares},
				{Method: "POST", Pattern: "/admin/reports/profit-shares/run", Handler: h.ReportHandler.RunProfitShares},
				{Method: "GET", Pattern: "/admin/reports/catalog", Handler: h.ReportHandler.Catalog},
				{Method: "GET", Pattern: "/admin/reports/:report_key", Handler: h.ReportHandler.Center},
				{Method: "GET", Pattern: "/admin/settings", Handler: h.SettingsHandler.Get},
				{Method: "PUT", Pattern: "/admin/settings", Handler: h.SettingsHandler.Update},
				{Method: "GET", Pattern: "/admin/room-activity/status", Handler: h.SettingsHandler.RoomActivityStatus},
				adminRoomActivityRunOnceRoute(h.SettingsHandler.RunRoomActivityOnce),
				{Method: "GET", Pattern: "/admin/robot-settings", Handler: h.SettingsHandler.RobotSetting},
				{Method: "PATCH", Pattern: "/admin/robot-settings", Handler: h.SettingsHandler.UpdateRobotSetting},
				{Method: "GET", Pattern: "/admin/plays/catalog", Handler: h.OddsHandler.Catalog},
				{Method: "GET", Pattern: "/admin/games/:id/odds-limits", Handler: h.OddsHandler.Get},
				{Method: "PUT", Pattern: "/admin/games/:id/odds-limits", Handler: h.OddsHandler.Update},
				{Method: "POST", Pattern: "/admin/games/:id/odds-limits/reset", Handler: h.OddsHandler.Reset},
				{Method: "GET", Pattern: "/admin/wallet/channels", Handler: h.WalletHandler.List},
				{Method: "POST", Pattern: "/admin/wallet/channels", Handler: h.WalletHandler.Create},
				{Method: "PATCH", Pattern: "/admin/wallet/channels/:id", Handler: h.WalletHandler.Update},
				{Method: "PATCH", Pattern: "/admin/wallet/channels/:id/status", Handler: h.WalletHandler.SetStatus},
				{Method: "DELETE", Pattern: "/admin/wallet/channels/:id", Handler: h.WalletHandler.Delete},
				{Method: "GET", Pattern: "/admin/monitor", Handler: h.BetHandler.Monitor},
				{Method: "GET", Pattern: "/admin/bets", Handler: h.BetHandler.List},
				{Method: "POST", Pattern: "/admin/bets", Handler: h.BetHandler.Place},
				{Method: "POST", Pattern: "/admin/bets/:id/cancel", Handler: h.BetHandler.Cancel},
				{Method: "POST", Pattern: "/admin/games/:id/publish-draw", Handler: h.BetHandler.PublishDraw},
				{Method: "POST", Pattern: "/admin/games/:id/issues/:issue/settle", Handler: h.BetHandler.Settle},
				{Method: "GET", Pattern: "/admin/games/:id/issues/:issue/settlement", Handler: h.BetHandler.SettlementStatus},
				{Method: "GET", Pattern: "/admin/reports/board", Handler: h.BetHandler.BoardReport},
				{Method: "GET", Pattern: "/admin/activities", Handler: h.OpsHandler.ListActivities},
				{Method: "POST", Pattern: "/admin/activities/upload", Handler: h.OpsHandler.UploadActivityImage},
				{Method: "POST", Pattern: "/admin/activities", Handler: h.OpsHandler.CreateActivity},
				{Method: "PUT", Pattern: "/admin/activities/:id", Handler: h.OpsHandler.UpdateActivity},
				{Method: "PATCH", Pattern: "/admin/activities/:id/status", Handler: h.OpsHandler.SetActivityStatus},
				{Method: "DELETE", Pattern: "/admin/activities/:id", Handler: h.OpsHandler.DeleteActivity},
				{Method: "GET", Pattern: "/admin/plans", Handler: h.PlanHandler.List},
				{Method: "POST", Pattern: "/admin/plans", Handler: h.PlanHandler.Create},
				{Method: "PUT", Pattern: "/admin/plans/:id", Handler: h.PlanHandler.Update},
				{Method: "DELETE", Pattern: "/admin/plans/:id", Handler: h.PlanHandler.Delete},
				{Method: "GET", Pattern: "/admin/plan-automation", Handler: h.PlanHandler.Automation},
				{Method: "PUT", Pattern: "/admin/plan-automation", Handler: h.PlanHandler.SaveAutomation},
				{Method: "POST", Pattern: "/admin/plan-automation/preview", Handler: h.PlanHandler.PreviewAutomation},
				{Method: "GET", Pattern: "/admin/special-numbers", Handler: h.OpsHandler.SpecialOverview},
				{Method: "POST", Pattern: "/admin/special-numbers/resources", Handler: h.OpsHandler.AddSpecialNumbers},
				{Method: "POST", Pattern: "/admin/special-numbers/campaigns", Handler: h.OpsHandler.CreateSpecialCampaign},
				{Method: "POST", Pattern: "/admin/special-numbers/grant", Handler: h.OpsHandler.GrantSpecialNumber},
				{Method: "POST", Pattern: "/admin/special-numbers/assign", Handler: h.OpsHandler.AssignSpecialRoom},
				{Method: "GET", Pattern: "/admin/entertainment", Handler: h.OpsHandler.ListEntertainment},
				{Method: "POST", Pattern: "/admin/entertainment", Handler: h.OpsHandler.UpsertEntertainment},
				{Method: "PATCH", Pattern: "/admin/entertainment/:id/status", Handler: h.OpsHandler.SetEntertainmentStatus},
				{Method: "GET", Pattern: "/admin/chat/conversations", Handler: h.OpsHandler.ListChatConversations},
				{Method: "GET", Pattern: "/admin/chat/messages", Handler: h.OpsHandler.ListChatMessages},
				{Method: "GET", Pattern: "/admin/chat/unread", Handler: h.OpsHandler.ChatUnread},
				{Method: "POST", Pattern: "/admin/chat/read", Handler: h.OpsHandler.MarkChatRead},
				{Method: "POST", Pattern: "/admin/chat/messages", Handler: h.OpsHandler.ReplyChat},
				{Method: "POST", Pattern: "/admin/chat/redpackets", Handler: h.OpsHandler.SendChatRedPacket},
				{Method: "PATCH", Pattern: "/admin/chat/lottery-rooms/status", Handler: h.OpsHandler.SetLotteryRoomStatus},
				{Method: "PATCH", Pattern: "/admin/chat/rooms/:agentID/group-chat", Handler: h.OpsHandler.SetRoomGroupChat},
				{Method: "DELETE", Pattern: "/admin/chat/messages/:id", Handler: h.OpsHandler.DeleteChatMessage},
				{Method: "PATCH", Pattern: "/admin/chat/users/:userID/mute", Handler: h.OpsHandler.SetChatMute},
				{Method: "PUT", Pattern: "/admin/chat/announcement", Handler: h.OpsHandler.SetChatAnnouncement},
				{Method: "GET", Pattern: "/admin/notifications", Handler: h.OpsHandler.ListNotifications},
				{Method: "POST", Pattern: "/admin/notifications/:id/read", Handler: h.OpsHandler.MarkNotificationRead},
				{Method: "POST", Pattern: "/admin/notifications/read-all", Handler: h.OpsHandler.MarkAllNotificationsRead},
				{Method: "GET", Pattern: "/admin/rebates/preview", Handler: h.OpsHandler.RebatePreview},
				{Method: "POST", Pattern: "/admin/rebates/run", Handler: h.OpsHandler.RunRebate},
			},
		},
	})
	LoadDrawFeedRoutes(r, db, scheduler)
}

func LoadDrawFeedRoutes(r *gin.Engine, db *gorm.DB, scheduler *lotteryfeed.Scheduler) {
	h := drawfeed.NewHandler(db, scheduler)
	g := r.Group("/api/public")
	g.GET("/clock", h.Clock)
	g.GET("/lottery/status", h.Status)
	g.GET("/lottery/games", h.Games)
	g.GET("/lottery/games/enabled", h.EnabledGames)
	g.GET("/lottery/games/:id/draws", h.Draws)
	g.GET("/lottery/latest", h.Latest)
	g.GET("/rooms/:code", func(c *gin.Context) {
		result, err := services.NewSpecialAdminService(db).ResolveRoom(c.Param("code"))
		if err != nil {
			constants.SendError(c, http.StatusNotFound, "房间号无效或未开通", err)
			return
		}
		// Room ownership is an internal authorization detail.  The public
		// resolver is used only to confirm that a room exists, so never expose
		// the owning agent's database id or login name to anonymous callers.
		constants.SendSuccess(c, http.StatusOK, "ok", gin.H{
			"room_code": result.RoomCode,
			"room_name": result.RoomName,
			"room_logo": result.RoomLogo,
		})
	})
}
