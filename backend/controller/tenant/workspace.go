package tenant

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
	tenants *services.TenantAdminService
	agents  *services.AgentAdminService
	work    *services.AgentWorkspaceService
	chat    *services.ChatAdminService
	trading *services.TradingAdminService
}

func NewWorkspaceHandler(db *gorm.DB) *WorkspaceHandler {
	return &WorkspaceHandler{db: db, tenants: services.NewTenantAdminService(db), agents: services.NewAgentAdminService(db), work: services.NewAgentWorkspaceService(db), chat: services.NewChatAdminService(db), trading: services.NewTradingAdminService(db)}
}

func tenantID(c *gin.Context) (uint64, bool) {
	value, ok := c.Get("tenant_id")
	id, valid := value.(uint64)
	if !ok || !valid || id == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return 0, false
	}
	return id, true
}

// ownedAgentForProfileManagement is deliberately limited to the one operation
// a tenant is allowed to perform on a subordinate agent room: maintaining its
// public name and logo. Agent-room members, bets, applications, chat, reports
// and robots remain private to the agent workspace.
func (h *WorkspaceHandler) ownedAgentForProfileManagement(c *gin.Context) (user.User, user.User, bool) {
	tenantRaw, ok := c.Get("tenant_user")
	tenant, tenantOK := tenantRaw.(user.User)
	if !ok || !tenantOK || tenant.UserID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return user.User{}, user.User{}, false
	}
	agentID, err := services.ParseUserID(c.Param("agentID"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间编号不正确", err)
		return user.User{}, user.User{}, false
	}
	var agent user.User
	if err := h.db.Where("user_id = ? AND role = ? AND parent_tenant_id = ?", agentID, "agent", tenant.UserID).First(&agent).Error; err != nil {
		constants.SendError(c, http.StatusForbidden, "该房间不属于当前租户", err)
		return user.User{}, user.User{}, false
	}
	return tenant, agent, true
}

func (h *WorkspaceHandler) Me(c *gin.Context) {
	account, ok := c.Get("tenant_user")
	if !ok {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", account)
}

func (h *WorkspaceHandler) Dashboard(c *gin.Context) {
	id, ok := tenantID(c)
	if !ok {
		return
	}
	result, err := h.tenants.Dashboard(id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户概览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectRoomDashboard(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := h.work.DashboardForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间概览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectGames(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := services.NewWorkspaceGameService(h.db).List(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间游戏失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SetDirectGameStatus(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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
		constants.SendError(c, http.StatusBadRequest, "直属房间不存在", err)
		return
	}
	result, err := h.chat.SetLotteryRoomEnabledForWorkspace(workspace, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间游戏状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间游戏状态已保存", result)
}

func (h *WorkspaceHandler) DirectTrading(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := h.trading.GetRoomForWorkspace(account.WorkspaceID, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateDirectTrading(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request services.UpdateRoomTradingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率与返水参数不正确", err)
		return
	}
	result, err := h.trading.UpdateRoomForWorkspace(account.WorkspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存直属房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "直属房间赔率与返水已保存", result)
}

func (h *WorkspaceHandler) UpdateDirectRoomProfile(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request struct {
		RoomName string `json:"room_name" binding:"required,max=30"`
		RoomLogo string `json:"room_logo"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间资料不正确", err)
		return
	}
	view, err := services.NewSettingsAdminService(h.db).UpdateRoomProfileForWorkspace(account.WorkspaceID, request.RoomName, request.RoomLogo)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存直属房间资料失败", err)
		return
	}
	dashboard, err := h.work.DashboardForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "刷新房间概览失败", err)
		return
	}
	dashboard.RoomName, dashboard.RoomLogo = view.RoomName, view.RoomLogo
	constants.SendSuccess(c, http.StatusOK, "房间资料已保存", dashboard)
}

func (h *WorkspaceHandler) ReportCatalog(c *gin.Context) {
	constants.SendSuccess(c, http.StatusOK, "ok", services.ReportCatalog())
}

func tenantRoomAccount(c *gin.Context) (user.User, bool) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.UserID == 0 || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return user.User{}, false
	}
	return account, true
}

func (h *WorkspaceHandler) Plans(c *gin.Context) {
	account, ok := tenantRoomAccount(c)
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
	account, ok := tenantRoomAccount(c)
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
	account, ok := tenantRoomAccount(c)
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
	account, ok := tenantRoomAccount(c)
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
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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
		constants.SendError(c, http.StatusBadRequest, "读取直属房间报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SystemSettings(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := services.NewSettingsAdminService(h.db).GetForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) MenuTemplate(c *gin.Context) {
	if _, ok := tenantID(c); !ok {
		return
	}
	result, err := services.NewSettingsAdminService(h.db).MenuTemplate("tenant")
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户菜单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateSystemSettings(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request services.UpdateSystemSettingsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "直属房间设置参数不正确", err)
		return
	}
	result, err := services.NewSettingsAdminService(h.db).UpdateForWorkspace(account.WorkspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存直属房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "直属房间设置已保存", result)
}

func (h *WorkspaceHandler) Agents(c *gin.Context) {
	id, ok := tenantID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.agents.ListForTenant(id, c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取代理列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectUsers(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.UserID == 0 || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var memberID uint64
	if rawID := c.Query("user_id"); rawID != "" {
		var err error
		memberID, err = strconv.ParseUint(rawID, 10, 63)
		if err != nil || memberID == 0 {
			constants.SendError(c, http.StatusBadRequest, "会员编号不正确", nil)
			return
		}
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := services.NewWorkspaceMemberService(h.db).List(account.WorkspaceID, services.UserListFilter{UserID: memberID, Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间会员失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SetDirectUserStatus(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Status int `json:"status" binding:"oneof=0 1"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户状态不正确", err)
		return
	}
	result, err := services.NewUserAdminService(h.db).SetStatusInWorkspace(userID, account.WorkspaceID, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新用户状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户状态已更新", result)
}

func (h *WorkspaceHandler) AdjustDirectUserBalance(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	userID, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Amount float64 `json:"amount" binding:"required"`
		Remark string  `json:"remark" binding:"required,max=300"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "余额调整资料不正确", err)
		return
	}
	result, err := services.NewUserAdminService(h.db).AdjustBalanceInWorkspace(userID, account.WorkspaceID, request.Amount, request.Remark, "租户 "+account.Username)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "调整用户余额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户余额已调整", result)
}

func (h *WorkspaceHandler) DirectUserTrading(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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

func (h *WorkspaceHandler) UpdateDirectUserTrading(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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

func (h *WorkspaceHandler) DirectBets(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := services.NewBetAdminService(h.db).List(services.BetListFilter{WorkspaceID: account.WorkspaceID, UserID: userID, Query: c.Query("query"), GameID: c.Query("game_id"), Issue: c.Query("issue"), Status: c.DefaultQuery("status", "all"), Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectApplications(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := services.NewApplicationAdminService(h.db).List(services.ApplicationFilter{WorkspaceID: account.WorkspaceID, Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Start: c.Query("start"), End: c.Query("end"), Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectConversations(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	result, err := h.chat.ConversationsForRoom(c.Query("room_type"), c.Query("query"), c.Query("channel"), fmt.Sprintf("tenant:%d", account.UserID), page, size)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取直属房间会话失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) DirectMessages(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	roomScope := fmt.Sprintf("tenant:%d", account.UserID)
	scope, roomType := strings.TrimSpace(c.Query("scope")), c.Query("room_type")
	if roomType == "group" {
		scope = roomScope
	} else if strings.HasPrefix(scope, "user:") {
		// The exact room workspace and frozen message scope are authoritative.
		// account.workspace_id is intentionally not consulted because it changes
		// when a member later enters another room.
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
	account, ok := tenantRoomAccount(c)
	if !ok {
		return
	}
	roomScope := fmt.Sprintf("tenant:%d", account.UserID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	result, err := h.chat.UnreadServiceMessages(account.UserID, roomScope, limit)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取客服未读消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) MarkChatRead(c *gin.Context) {
	account, ok := tenantRoomAccount(c)
	if !ok {
		return
	}
	roomScope := fmt.Sprintf("tenant:%d", account.UserID)
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

func (h *WorkspaceHandler) ReplyDirectChat(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	roomScope := fmt.Sprintf("tenant:%d", account.UserID)
	var request struct {
		Scope    string `json:"scope" binding:"required"`
		RoomType string `json:"room_type" binding:"required"`
		GameID   string `json:"game_id"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "回复参数不正确", err)
		return
	}
	if request.RoomType == "group" {
		request.Scope = roomScope
	} else if strings.HasPrefix(request.Scope, "user:") {
		// The service layer accepts a reply only when this exact room already
		// owns a durable service conversation or currently owns the member.
	} else {
		constants.SendError(c, http.StatusBadRequest, "客服会话不正确", nil)
		return
	}
	result, err := h.chat.Reply(request.Scope, request.RoomType, roomScope, request.GameID, request.Content, firstTenantName(account))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送回复失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "回复已发送", result)
}

func (h *WorkspaceHandler) SendDirectRedPacket(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request struct {
		RequestID        string  `json:"request_id" binding:"max=96"`
		GameID           string  `json:"game_id"`
		Count            int     `json:"count" binding:"required"`
		TotalAmount      float64 `json:"total_amount" binding:"required"`
		MinDailyTurnover float64 `json:"min_daily_turnover"`
		Greeting         string  `json:"greeting"`
		Cover            string  `json:"cover"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "红包参数不正确", err)
		return
	}
	roomScope := fmt.Sprintf("tenant:%d", account.UserID)
	result, err := h.chat.SendRedPacket(roomScope, roomScope, request.GameID, services.ChatRedPacketInput{RequestID: request.RequestID, Count: request.Count, TotalAmount: request.TotalAmount, MinDailyTurnover: request.MinDailyTurnover, Greeting: request.Greeting, Cover: request.Cover}, firstTenantName(account))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送红包失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "红包已发送到聊天室", result)
}

func (h *WorkspaceHandler) SetDirectLotteryRoomStatus(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "彩票室参数不正确", err)
		return
	}
	workspace, err := services.WorkspaceForAccount(h.db, account)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "直属房间不存在", err)
		return
	}
	result, err := h.chat.SetLotteryRoomEnabledForWorkspace(workspace, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存彩票室状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "彩票室状态已保存", result)
}

func (h *WorkspaceHandler) DirectApplicationStats(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := services.NewApplicationAdminService(h.db).StatsForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间申请统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ReviewDirectApplication(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	var request struct {
		Decision       string   `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64  `json:"received_amount"`
		OddsMultiplier *float64 `json:"odds_multiplier"`
		Remark         string   `json:"remark" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := services.NewApplicationAdminService(h.db).ReviewForWorkspace(id, account.WorkspaceID, services.ReviewApplicationInput{Decision: request.Decision, ReceivedAmount: request.ReceivedAmount, OddsMultiplier: request.OddsMultiplier, Remark: request.Remark, Operator: "租户 " + account.Username})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}

func (h *WorkspaceHandler) RobotSetting(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	result, err := services.NewUserAdminService(h.db).List(services.UserListFilter{
		WorkspaceID: account.WorkspaceID, Kind: "robot", Query: c.Query("query"),
		Status: c.DefaultQuery("status", "all"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取直属房间机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateRobot(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	var request services.ResetWorkspaceRobotsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人批量重置参数不正确", err)
		return
	}
	request.WorkspaceID = account.WorkspaceID
	result, err := services.NewUserAdminService(h.db).ResetRobotsForWorkspace(account.WorkspaceID, request, "租户 "+account.Username)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "批量重置机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "本房间机器人昵称和余额已重置", result)
}

func (h *WorkspaceHandler) UpdateRobotSetting(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
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

func (h *WorkspaceHandler) RunDirectRobot(c *gin.Context) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	result, err := services.RunRoomActivityOnceForWorkspace(account.WorkspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行直属房间机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "直属房间机器人已执行", result)
}

type agentRequest struct {
	Username        string  `json:"username"`
	Password        string  `json:"password"`
	Email           string  `json:"email"`
	Nickname        string  `json:"nickname"`
	Phone           string  `json:"phone"`
	RoomCode        string  `json:"room_code" binding:"required"`
	RoomName        string  `json:"room_name" binding:"max=30"`
	RoomLogo        string  `json:"room_logo"`
	Remark          string  `json:"remark"`
	Status          int     `json:"status"`
	RebateRate      float64 `json:"rebate_rate"`
	ProfitShareRate float64 `json:"profit_share_rate"`
	RobotQuota      *int    `json:"robot_quota"`
}

func (h *WorkspaceHandler) CreateAgent(c *gin.Context) {
	id, ok := tenantID(c)
	if !ok {
		return
	}
	var request agentRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Username == "" || request.Password == "" {
		constants.SendError(c, http.StatusBadRequest, "房间管理员资料不正确", err)
		return
	}
	result, err := h.agents.CreateForTenant(id, services.CreateAgentInput{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate, RobotQuota: request.RobotQuota})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "开通房间失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "房间已开通", result)
}

func (h *WorkspaceHandler) UpdateAgent(c *gin.Context) {
	tid, ok := tenantID(c)
	if !ok {
		return
	}
	aid, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理编号不正确", err)
		return
	}
	var request agentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间资料不正确", err)
		return
	}
	result, err := h.agents.UpdateForTenant(tid, aid, services.UpdateAgentInput{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate, RobotQuota: request.RobotQuota})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间资料已保存", result)
}

func (h *WorkspaceHandler) ResetAgentPassword(c *gin.Context) {
	tid, ok := tenantID(c)
	if !ok {
		return
	}
	aid, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理编号不正确", err)
		return
	}
	var request struct {
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "新密码长度需为 8–72 个字符", err)
		return
	}
	if err := h.agents.ResetPasswordForTenant(tid, aid, request.Password); err != nil {
		constants.SendError(c, http.StatusBadRequest, "重置密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间管理员登录密码已重置", gin.H{"id": aid})
}

func (h *WorkspaceHandler) UpdateRoomSettings(c *gin.Context) {
	_, agent, ok := h.ownedAgentForProfileManagement(c)
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
	result, err := h.work.UpdateRoomProfile(agent.UserID, request.RoomName, request.RoomLogo)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间名称失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间名称已保存", result)
}

func firstTenantName(account user.User) string {
	if strings.TrimSpace(account.Nickname) != "" {
		return account.Nickname
	}
	return account.Username
}
