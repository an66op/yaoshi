package api

import (
	"backend/constants"
	"backend/controller/drawfeed"
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

	register([]RouteGroup{
		{
			Prefix: "/health",
			Routes: []Route{{Method: "GET", Pattern: "", Handler: func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) }}},
		},
		{
			Prefix: "/api",
			Routes: []Route{
				{Method: "POST", Pattern: "/register", Handler: h.AuthHandler.Register, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()}},
				{Method: "POST", Pattern: "/login", Handler: h.AuthHandler.Login, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()}},
				{Method: "POST", Pattern: "/member/login", Handler: h.MemberHandler.Login, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()}},
				{Method: "POST", Pattern: "/member/register", Handler: h.MemberHandler.Register, Middlewares: []gin.HandlerFunc{middleware.AuthRateLimit()}},
				{Method: "GET", Pattern: "/ws", Handler: ws.HandleConnect, Middlewares: []gin.HandlerFunc{middleware.WSConnectRateLimit()}},
			},
		},
		{
			Prefix:      "/api/member",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.MemberMiddleware(db)},
			Routes: []Route{
				{Method: "POST", Pattern: "/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "GET", Pattern: "/me", Handler: h.MemberHandler.Me},
				{Method: "POST", Pattern: "/room/join", Handler: h.MemberHandler.JoinRoom},
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
				{Method: "GET", Pattern: "/chat/preview", Handler: h.MemberHandler.ChatPreview},
				{Method: "GET", Pattern: "/chat/messages", Handler: h.MemberHandler.ListChatMessages},
				{Method: "POST", Pattern: "/chat/messages", Handler: h.MemberHandler.PostChatMessage},
				{Method: "POST", Pattern: "/chat/redpackets/:id/claim", Handler: h.MemberHandler.ClaimChatRedPacket},
				{Method: "GET", Pattern: "/room/settings", Handler: h.MemberHandler.RoomSettings},
				{Method: "GET", Pattern: "/games/:id/assistant", Handler: h.MemberHandler.AssistantStatus},
				{Method: "GET", Pattern: "/games/:id/assistant/history", Handler: h.MemberHandler.AssistantBetHistory},
				{Method: "POST", Pattern: "/games/:id/assistant/bets", Handler: h.MemberHandler.AssistantBet, Middlewares: []gin.HandlerFunc{middleware.MemberBetRateLimit()}},
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
				{Method: "GET", Pattern: "/agents", Handler: h.TenantWorkspaceHandler.Agents},
				{Method: "POST", Pattern: "/agents", Handler: h.TenantWorkspaceHandler.CreateAgent},
				{Method: "PATCH", Pattern: "/agents/:id", Handler: h.TenantWorkspaceHandler.UpdateAgent},
				{Method: "POST", Pattern: "/agents/:id/reset-password", Handler: h.TenantWorkspaceHandler.ResetAgentPassword},
				{Method: "GET", Pattern: "/rooms/:agentID/dashboard", Handler: h.TenantWorkspaceHandler.RoomDashboard},
				{Method: "PATCH", Pattern: "/rooms/:agentID/settings", Handler: h.TenantWorkspaceHandler.UpdateRoomSettings},
				{Method: "GET", Pattern: "/rooms/:agentID/users", Handler: h.TenantWorkspaceHandler.RoomUsers},
				{Method: "PATCH", Pattern: "/rooms/:agentID/users/:userID/status", Handler: h.TenantWorkspaceHandler.SetRoomUserStatus},
				{Method: "POST", Pattern: "/rooms/:agentID/users/:userID/balance", Handler: h.TenantWorkspaceHandler.AdjustRoomUserBalance},
				{Method: "GET", Pattern: "/rooms/:agentID/bets", Handler: h.TenantWorkspaceHandler.RoomBets},
				{Method: "GET", Pattern: "/rooms/:agentID/applications", Handler: h.TenantWorkspaceHandler.RoomApplications},
				{Method: "POST", Pattern: "/rooms/:agentID/applications/:applicationID/review", Handler: h.TenantWorkspaceHandler.ReviewRoomApplication},
				{Method: "GET", Pattern: "/rooms/:agentID/reports/operating", Handler: h.TenantWorkspaceHandler.RoomOperatingReport},
				{Method: "GET", Pattern: "/rooms/:agentID/chat/conversations", Handler: h.TenantWorkspaceHandler.RoomConversations},
				{Method: "GET", Pattern: "/rooms/:agentID/chat/messages", Handler: h.TenantWorkspaceHandler.RoomMessages},
				{Method: "POST", Pattern: "/rooms/:agentID/chat/messages", Handler: h.TenantWorkspaceHandler.ReplyRoomChat},
				{Method: "POST", Pattern: "/rooms/:agentID/chat/redpackets", Handler: h.TenantWorkspaceHandler.SendRoomRedPacket},
				{Method: "PATCH", Pattern: "/rooms/:agentID/chat/lottery-rooms/:gameID/status", Handler: h.TenantWorkspaceHandler.SetLotteryRoomStatus},
				{Method: "GET", Pattern: "/rooms/:agentID/robots/status", Handler: h.TenantWorkspaceHandler.RoomRobotStatus},
				{Method: "POST", Pattern: "/rooms/:agentID/robots/run-once", Handler: h.TenantWorkspaceHandler.RunRoomRobot},
			},
		},
		{
			Prefix:      "/api/agent",
			Middlewares: []gin.HandlerFunc{middleware.AuthMiddleware(), middleware.AgentMiddleware(db), middleware.PrivilegedAudit(db, "agent")},
			Routes: []Route{
				{Method: "POST", Pattern: "/ws-ticket", Handler: ws.HandleTicket, Middlewares: []gin.HandlerFunc{middleware.WSTicketRateLimit()}},
				{Method: "GET", Pattern: "/me", Handler: h.AgentWorkspaceHandler.Me},
				{Method: "GET", Pattern: "/dashboard", Handler: h.AgentWorkspaceHandler.Dashboard},
				{Method: "PATCH", Pattern: "/room/settings", Handler: h.AgentWorkspaceHandler.UpdateRoomSettings},
				{Method: "GET", Pattern: "/reports/operating", Handler: h.AgentWorkspaceHandler.OperatingReport},
				{Method: "GET", Pattern: "/reports/profit-shares", Handler: h.AgentWorkspaceHandler.ProfitShares},
				{Method: "GET", Pattern: "/users", Handler: h.AgentWorkspaceHandler.Users},
				{Method: "PATCH", Pattern: "/users/:id/status", Handler: h.AgentWorkspaceHandler.SetUserStatus},
				{Method: "POST", Pattern: "/users/:id/balance", Handler: h.AgentWorkspaceHandler.AdjustBalance},
				{Method: "GET", Pattern: "/bets", Handler: h.AgentWorkspaceHandler.Bets},
				{Method: "GET", Pattern: "/applications", Handler: h.AgentWorkspaceHandler.Applications},
				{Method: "POST", Pattern: "/applications/:id/review", Handler: h.AgentWorkspaceHandler.ReviewApplication},
				{Method: "GET", Pattern: "/trading", Handler: h.AgentWorkspaceHandler.Trading},
				{Method: "PUT", Pattern: "/trading", Handler: h.AgentWorkspaceHandler.UpdateTrading},
				{Method: "GET", Pattern: "/chat/conversations", Handler: h.AgentWorkspaceHandler.Conversations},
				{Method: "GET", Pattern: "/chat/messages", Handler: h.AgentWorkspaceHandler.Messages},
				{Method: "POST", Pattern: "/chat/messages", Handler: h.AgentWorkspaceHandler.Reply},
				{Method: "POST", Pattern: "/chat/redpackets", Handler: h.AgentWorkspaceHandler.SendRedPacket},
				{Method: "PATCH", Pattern: "/chat/lottery-rooms/:gameID/status", Handler: h.AgentWorkspaceHandler.SetLotteryRoomStatus},
				{Method: "GET", Pattern: "/robots/status", Handler: h.AgentWorkspaceHandler.RobotStatus},
				{Method: "POST", Pattern: "/robots/run-once", Handler: h.AgentWorkspaceHandler.RunRobot},
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
				{Method: "GET", Pattern: "/admin/games", Handler: h.DashboardHandler.Games},
				{Method: "POST", Pattern: "/admin/games/sync-target", Handler: h.DashboardHandler.SyncTargetGames},
				{Method: "GET", Pattern: "/admin/games/:id/draws", Handler: h.DashboardHandler.Draws},
				{Method: "POST", Pattern: "/admin/sources/sync", Handler: h.DashboardHandler.SyncOfficialSources},
				{Method: "PATCH", Pattern: "/admin/games/:id/status", Handler: h.DashboardHandler.UpdateGameStatus},
				{Method: "GET", Pattern: "/admin/users", Handler: h.UserAdminHandler.List},
				{Method: "GET", Pattern: "/admin/users/stats", Handler: h.UserAdminHandler.Stats},
				{Method: "GET", Pattern: "/admin/users/:id", Handler: h.UserAdminHandler.Get},
				{Method: "POST", Pattern: "/admin/users", Handler: h.UserAdminHandler.Create},
				{Method: "PATCH", Pattern: "/admin/users/:id", Handler: h.UserAdminHandler.Update},
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
				{Method: "POST", Pattern: "/admin/tenants/:id/reset-password", Handler: h.TenantHandler.ResetPassword},
				{Method: "POST", Pattern: "/admin/agents", Handler: h.AgentHandler.Create},
				{Method: "PATCH", Pattern: "/admin/agents/:id", Handler: h.AgentHandler.Update},
				{Method: "GET", Pattern: "/admin/agents/:id/trading", Handler: h.AgentHandler.GetTrading},
				{Method: "PUT", Pattern: "/admin/agents/:id/trading", Handler: h.AgentHandler.UpdateTrading},
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
				{Method: "GET", Pattern: "/admin/settings", Handler: h.SettingsHandler.Get},
				{Method: "PUT", Pattern: "/admin/settings", Handler: h.SettingsHandler.Update},
				{Method: "GET", Pattern: "/admin/room-activity/status", Handler: h.SettingsHandler.RoomActivityStatus},
				{Method: "POST", Pattern: "/admin/room-activity/run-once", Handler: h.SettingsHandler.RunRoomActivityOnce},
				{Method: "GET", Pattern: "/admin/plays/catalog", Handler: h.OddsHandler.Catalog},
				{Method: "GET", Pattern: "/admin/games/:id/odds-limits", Handler: h.OddsHandler.Get},
				{Method: "PUT", Pattern: "/admin/games/:id/odds-limits", Handler: h.OddsHandler.Update},
				{Method: "POST", Pattern: "/admin/games/:id/odds-limits/reset", Handler: h.OddsHandler.Reset},
				{Method: "POST", Pattern: "/admin/games/sync-odds-limits", Handler: h.OddsHandler.SyncAll},
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
				{Method: "POST", Pattern: "/admin/chat/messages", Handler: h.OpsHandler.ReplyChat},
				{Method: "POST", Pattern: "/admin/chat/redpackets", Handler: h.OpsHandler.SendChatRedPacket},
				{Method: "PATCH", Pattern: "/admin/chat/lottery-rooms/status", Handler: h.OpsHandler.SetLotteryRoomStatus},
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
