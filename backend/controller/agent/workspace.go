package agent

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WorkspaceHandler struct {
	db      *gorm.DB
	work    *services.AgentWorkspaceService
	chat    *services.ChatAdminService
	trading *services.TradingAdminService
}

func NewWorkspaceHandler(db *gorm.DB) *WorkspaceHandler {
	return &WorkspaceHandler{db: db, work: services.NewAgentWorkspaceService(db), chat: services.NewChatAdminService(db), trading: services.NewTradingAdminService(db)}
}

func agentIdentity(c *gin.Context) (user.User, uint64, string, bool) {
	raw, ok := c.Get("agent_user")
	account, accountOK := raw.(user.User)
	if !ok || !accountOK || account.UserID == 0 || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return user.User{}, 0, "", false
	}
	return account, account.UserID, fmt.Sprintf("agent:%d", account.UserID), true
}

func (h *WorkspaceHandler) Me(c *gin.Context) {
	account, _, scope, ok := agentIdentity(c)
	if !ok {
		return
	}
	workspace, err := services.WorkspaceForAccount(h.db, account)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "房间不存在或已停用", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{
		"id": account.UserID, "public_id": account.PublicID, "username": account.Username,
		"nickname": account.Nickname, "avatar": account.Avatar, "public_title": account.PublicTitle,
		"badge": account.PublicBadge, "role": account.Role, "room_code": workspace.RoomCode,
		"room_name": workspace.Name, "room_logo": workspace.Logo, "room_scope": scope,
	})
}

func (h *WorkspaceHandler) UpdateRoomSettings(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request struct {
		RoomName string `json:"room_name" binding:"required,max=30"`
		RoomLogo string `json:"room_logo"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间名称不正确", err)
		return
	}
	result, err := h.work.UpdateRoomProfile(id, request.RoomName, request.RoomLogo)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间名称失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间名称已保存", result)
}

func (h *WorkspaceHandler) SystemSettings(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.NewSettingsAdminService(h.db).GetForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) MenuTemplate(c *gin.Context) {
	if _, _, _, ok := agentIdentity(c); !ok {
		return
	}
	result, err := services.NewSettingsAdminService(h.db).MenuTemplate("agent")
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取代理菜单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateSystemSettings(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request services.UpdateSystemSettingsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间设置参数不正确", err)
		return
	}
	result, err := services.NewSettingsAdminService(h.db).UpdateForWorkspace(account.WorkspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间设置已保存", result)
}

func (h *WorkspaceHandler) Dashboard(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := h.work.DashboardForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间工作台失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) Games(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.NewWorkspaceGameService(h.db).List(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间游戏失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SetGameStatus(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "游戏状态参数不正确", err)
		return
	}
	workspace, err := services.WorkspaceForAccount(h.db, account)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间不存在", err)
		return
	}
	result, err := h.chat.SetLotteryRoomEnabledForWorkspace(workspace, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间游戏状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间游戏状态已保存", result)
}

func (h *WorkspaceHandler) OperatingReport(c *gin.Context) {
	_, id, scope, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	if userID > 0 {
		if err := h.work.EnsureOwnedUser(id, userID); err != nil {
			constants.SendError(c, http.StatusForbidden, "不能查询其他房间用户", err)
			return
		}
	}
	result, err := services.NewOperatingReportService(h.db).Report(services.OperatingReportFilter{
		Query: c.Query("query"), Start: c.Query("start"), End: c.Query("end"), RoomScope: scope,
		GameID: c.Query("game_id"), Dimension: c.DefaultQuery("dimension", "game"), UserID: userID,
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间经营报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ReportCatalog(c *gin.Context) {
	constants.SendSuccess(c, http.StatusOK, "ok", services.ReportCatalog())
}

func (h *WorkspaceHandler) Plans(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.NewPlanContentService(h.db).ListAdmin(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) CreatePlan(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request services.PlanRecommendationInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "推荐内容不正确", err)
		return
	}
	result, err := services.NewPlanContentService(h.db).Create(account.WorkspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "新增计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "计划推荐已新增", result)
}

func (h *WorkspaceHandler) UpdatePlan(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "推荐编号不正确", err)
		return
	}
	var request services.PlanRecommendationInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "推荐内容不正确", err)
		return
	}
	result, err := services.NewPlanContentService(h.db).Update(account.WorkspaceID, id, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "计划推荐已保存", result)
}

func (h *WorkspaceHandler) DeletePlan(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "推荐编号不正确", err)
		return
	}
	if err := services.NewPlanContentService(h.db).Delete(account.WorkspaceID, id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "计划推荐已删除", gin.H{"id": id})
}

func (h *WorkspaceHandler) ReportCenter(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	filter := services.ReportCenterFilter{
		WorkspaceID: account.WorkspaceID, Query: c.Query("query"), Start: c.Query("start"), End: c.Query("end"),
		GameID: c.Query("game_id"), Category: c.Query("category"), Issue: c.Query("issue"), Status: c.Query("status"),
		Page: page, PageSize: pageSize,
	}
	reportService := services.NewReportCenterService(h.db)
	if c.Query("format") == "csv" {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.csv", c.Param("report_key"), time.Now().Format("20060102")))
		_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
		if err := reportService.ExportReportCSV(c.Writer, c.Param("report_key"), filter); err != nil {
			constants.SendError(c, http.StatusInternalServerError, "导出报表失败", err)
		}
		return
	}
	result, err := reportService.Report(c.Param("report_key"), filter)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ProfitShares(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.NewAgentProfitShareService(h.db).Statement(c.Query("date"), id)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间分账失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) Users(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.work.Users(id, services.UserListFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间用户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SetUserStatus(c *gin.Context) {
	_, agentID, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var req struct {
		Status int `json:"status" binding:"oneof=0 1"`
	}
	if err = c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户状态不正确", err)
		return
	}
	result, err := h.work.SetUserStatus(agentID, userID, req.Status)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新用户状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户状态已更新", result)
}

func (h *WorkspaceHandler) AdjustBalance(c *gin.Context) {
	account, agentID, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var req struct {
		Amount float64 `json:"amount" binding:"required"`
		Remark string  `json:"remark" binding:"required,max=300"`
	}
	if err = c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "余额调整资料不正确", err)
		return
	}
	result, err := h.work.AdjustBalance(agentID, userID, req.Amount, req.Remark, "房间 "+account.AgentRoomCode+" 代理")
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "调整余额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "余额调整成功", result)
}

func (h *WorkspaceHandler) UserTrading(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	result, err := h.trading.GetForWorkspace(account.WorkspaceID, userID, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取会员赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateUserTrading(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request services.UpdateUserTradingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "会员赔率参数不正确", err)
		return
	}
	result, err := h.trading.UpdateForWorkspace(account.WorkspaceID, userID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存会员赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "会员飞单与交易配置已保存", result)
}

func (h *WorkspaceHandler) Bets(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := h.work.Bets(id, services.BetListFilter{Query: c.Query("query"), GameID: c.Query("game_id"), Issue: c.Query("issue"), UserID: userID, Status: c.DefaultQuery("status", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) Applications(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.work.Applications(id, services.ApplicationFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Date: c.Query("date"), Start: c.Query("start"), End: c.Query("end"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ApplicationStats(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.NewApplicationAdminService(h.db).StatsForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间申请统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ReviewApplication(c *gin.Context) {
	account, agentID, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	applicationID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	var req struct {
		Decision       string   `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64  `json:"received_amount"`
		OddsMultiplier *float64 `json:"odds_multiplier"`
		Remark         string   `json:"remark" binding:"max=500"`
	}
	if err = c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := h.work.ReviewApplication(agentID, applicationID, services.ReviewApplicationInput{Decision: req.Decision, ReceivedAmount: req.ReceivedAmount, OddsMultiplier: req.OddsMultiplier, Remark: req.Remark, Operator: "房间 " + account.AgentRoomCode + " 代理"})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}

func (h *WorkspaceHandler) Trading(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := h.trading.GetRoomForWorkspace(account.WorkspaceID, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateTrading(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var req services.UpdateRoomTradingInput
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率与返水参数不正确", err)
		return
	}
	result, err := h.trading.UpdateRoomForWorkspace(account.WorkspaceID, req)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间赔率与返水已保存", result)
}

func (h *WorkspaceHandler) Conversations(c *gin.Context) {
	_, _, scope, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	result, err := h.chat.ConversationsForRoom(c.Query("room_type"), c.Query("query"), c.Query("channel"), scope, page, size)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间会话失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) Messages(c *gin.Context) {
	_, _, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	scope := strings.TrimSpace(c.Query("scope"))
	roomType := c.Query("room_type")
	if roomType == "group" {
		scope = roomScope
	} else if strings.HasPrefix(scope, "user:") {
		// Historical customer-service ownership is frozen on each message. The
		// service below combines this user scope with the authenticated room's
		// workspace_id and room_scope, so a member moving rooms neither hides the
		// old conversation nor exposes it to the new room.
	} else {
		constants.SendError(c, http.StatusBadRequest, "客服会话不正确", nil)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	before, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	result, err := h.chat.Messages(scope, roomType, roomScope, c.Query("game_id"), limit, before)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取聊天记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ChatUnread(c *gin.Context) {
	account, _, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	result, err := h.chat.UnreadServiceMessages(account.UserID, roomScope, limit)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取客服未读消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) MarkChatRead(c *gin.Context) {
	account, _, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request struct {
		Scope            string `json:"scope" binding:"required"`
		RoomScope        string `json:"room_scope"`
		GameID           string `json:"game_id"`
		ThroughMessageID uint64 `json:"through_message_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "已读参数不正确", err)
		return
	}
	result, err := h.chat.MarkServiceConversationRead(account.UserID, roomScope, request.Scope, roomScope, request.GameID, request.ThroughMessageID)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新客服已读状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "客服会话已读", result)
}

func (h *WorkspaceHandler) Reply(c *gin.Context) {
	account, _, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	var req struct {
		Scope    string `json:"scope" binding:"required"`
		RoomType string `json:"room_type" binding:"required"`
		GameID   string `json:"game_id"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "回复参数不正确", err)
		return
	}
	if req.RoomType == "group" {
		req.Scope = roomScope
	} else if strings.HasPrefix(req.Scope, "user:") {
		// ChatAdminService requires either an existing conversation in this
		// exact authenticated room or the member's current active membership.
	} else {
		constants.SendError(c, http.StatusBadRequest, "客服会话不正确", nil)
		return
	}
	result, err := h.chat.Reply(req.Scope, req.RoomType, roomScope, req.GameID, req.Content, defaultName(account))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送回复失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "回复已发送", result)
}

func (h *WorkspaceHandler) SendRedPacket(c *gin.Context) {
	account, _, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	var req struct {
		RequestID        string  `json:"request_id" binding:"max=96"`
		GameID           string  `json:"game_id"`
		Count            int     `json:"count" binding:"required"`
		TotalAmount      float64 `json:"total_amount" binding:"required"`
		MinDailyTurnover float64 `json:"min_daily_turnover"`
		Greeting         string  `json:"greeting"`
		Cover            string  `json:"cover"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "红包参数不正确", err)
		return
	}
	result, err := h.chat.SendRedPacket(roomScope, roomScope, req.GameID, services.ChatRedPacketInput{
		RequestID: req.RequestID, Count: req.Count, TotalAmount: req.TotalAmount, MinDailyTurnover: req.MinDailyTurnover, Greeting: req.Greeting, Cover: req.Cover,
	}, defaultName(account))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送红包失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "红包已发送到聊天室", result)
}

func (h *WorkspaceHandler) RobotStatus(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.RobotSettingForWorkspace(h.db, account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取机器人设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) Robots(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	result, err := services.NewUserAdminService(h.db).List(services.UserListFilter{
		WorkspaceID: account.WorkspaceID, Kind: "robot", Query: c.Query("query"),
		Status: c.DefaultQuery("status", "all"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateRobot(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人编号不正确", err)
		return
	}
	var request struct {
		Nickname    string   `json:"nickname" binding:"required,max=50"`
		Status      int      `json:"status" binding:"oneof=0 1"`
		GameIDs     []string `json:"game_ids"`
		ActiveStart string   `json:"active_start"`
		ActiveEnd   string   `json:"active_end"`
		MinBet      float64  `json:"min_bet"`
		MaxBet      float64  `json:"max_bet"`
		Avatar      string   `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人配置不正确", err)
		return
	}
	result, err := services.NewUserAdminService(h.db).UpdateRobotForWorkspace(id, account.WorkspaceID, services.UpdateRobotInput{
		Nickname: request.Nickname, Status: request.Status, GameIDs: request.GameIDs,
		ActiveStart: request.ActiveStart, ActiveEnd: request.ActiveEnd,
		MinBet: request.MinBet, MaxBet: request.MaxBet, Avatar: request.Avatar,
	})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存机器人配置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "机器人配置已保存", result)
}

func (h *WorkspaceHandler) ResetRobots(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request services.ResetWorkspaceRobotsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人批量重置参数不正确", err)
		return
	}
	request.WorkspaceID = account.WorkspaceID
	operator := "房间 " + account.AgentRoomCode + " 代理"
	if strings.TrimSpace(account.AgentRoomCode) == "" {
		operator = "代理 " + account.Username
	}
	result, err := services.NewUserAdminService(h.db).ResetRobotsForWorkspace(account.WorkspaceID, request, operator)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "批量重置机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "本房间机器人昵称和余额已重置", result)
}

func (h *WorkspaceHandler) UpdateRobotSetting(c *gin.Context) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request services.UpdateRobotSettingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人设置不正确", err)
		return
	}
	result, err := services.UpdateRobotSettingForWorkspace(h.db, account.WorkspaceID, request)
	if err != nil {
		if services.IsRoomActivityWorkspaceCapError(err) {
			constants.SendError(c, http.StatusConflict, "启用机器人房间已达到生产上限", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "保存机器人设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "机器人设置已保存", result)
}
func (h *WorkspaceHandler) RunRobot(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := services.RunRoomActivityOnceForAgent(id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行房间自动活跃失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间自动活跃已执行", result)
}

func (h *WorkspaceHandler) SetLotteryRoomStatus(c *gin.Context) {
	_, agentID, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "彩票室参数不正确", err)
		return
	}
	result, err := h.chat.SetLotteryRoomEnabled(agentID, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存彩票室状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "彩票室状态已保存", result)
}

func defaultName(account user.User) string {
	if strings.TrimSpace(account.Nickname) != "" {
		return account.Nickname
	}
	return account.Username
}
