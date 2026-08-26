package tenant

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WorkspaceHandler struct {
	db      *gorm.DB
	tenants *services.TenantAdminService
	agents  *services.AgentAdminService
	work    *services.AgentWorkspaceService
	chat    *services.ChatAdminService
}

func NewWorkspaceHandler(db *gorm.DB) *WorkspaceHandler {
	return &WorkspaceHandler{db: db, tenants: services.NewTenantAdminService(db), agents: services.NewAgentAdminService(db), work: services.NewAgentWorkspaceService(db), chat: services.NewChatAdminService(db)}
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

func (h *WorkspaceHandler) ownedAgent(c *gin.Context) (user.User, user.User, bool) {
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
	result, err := h.agents.CreateForTenant(id, services.CreateAgentInput{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate})
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
	result, err := h.agents.UpdateForTenant(tid, aid, services.UpdateAgentInput{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate})
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

func (h *WorkspaceHandler) RoomDashboard(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	result, err := h.work.Dashboard(agent.UserID)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间概览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateRoomSettings(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
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

func (h *WorkspaceHandler) RoomUsers(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.work.Users(agent.UserID, services.UserListFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间用户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) SetRoomUserStatus(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("userID"))
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
	result, err := h.work.SetUserStatus(agent.UserID, userID, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新用户状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户状态已更新", result)
}

func (h *WorkspaceHandler) AdjustRoomUserBalance(c *gin.Context) {
	tenant, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	userID, err := services.ParseUserID(c.Param("userID"))
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
	result, err := h.work.AdjustBalance(agent.UserID, userID, request.Amount, request.Remark, "租户 "+tenant.Username)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "调整余额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "余额调整成功", result)
}

func (h *WorkspaceHandler) RoomBets(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := h.work.Bets(agent.UserID, services.BetListFilter{Query: c.Query("query"), GameID: c.Query("game_id"), Issue: c.Query("issue"), UserID: userID, Status: c.DefaultQuery("status", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) RoomApplications(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.work.Applications(agent.UserID, services.ApplicationFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) ReviewRoomApplication(c *gin.Context) {
	tenant, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	applicationID, err := services.ParseUserID(c.Param("applicationID"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	var request struct {
		Decision       string  `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64 `json:"received_amount"`
		Remark         string  `json:"remark" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := h.work.ReviewApplication(agent.UserID, applicationID, services.ReviewApplicationInput{Decision: request.Decision, ReceivedAmount: request.ReceivedAmount, Remark: request.Remark, Operator: "租户 " + tenant.Username})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}

func (h *WorkspaceHandler) RoomOperatingReport(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	if userID > 0 {
		if err := h.work.EnsureOwnedUser(agent.UserID, userID); err != nil {
			constants.SendError(c, http.StatusForbidden, "不能查询其他房间用户", err)
			return
		}
	}
	result, err := services.NewOperatingReportService(h.db).Report(services.OperatingReportFilter{Query: c.Query("query"), Start: c.Query("start"), End: c.Query("end"), RoomScope: fmt.Sprintf("agent:%d", agent.UserID), GameID: c.Query("game_id"), Dimension: c.DefaultQuery("dimension", "game"), UserID: userID, Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间经营报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) RoomConversations(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	roomScope := fmt.Sprintf("agent:%d", agent.UserID)
	result, err := h.chat.ConversationsForRoom(c.Query("room_type"), c.Query("query"), c.Query("channel"), roomScope, page, size)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间会话失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) RoomMessages(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	roomScope := fmt.Sprintf("agent:%d", agent.UserID)
	scope := strings.TrimSpace(c.Query("scope"))
	roomType := c.Query("room_type")
	if roomType == "group" {
		scope = roomScope
	} else if strings.HasPrefix(scope, "user:") {
		userID, _ := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
		if err := h.work.EnsureOwnedUser(agent.UserID, userID); err != nil {
			constants.SendError(c, http.StatusForbidden, "不能读取其他房间客服", err)
			return
		}
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

func (h *WorkspaceHandler) ReplyRoomChat(c *gin.Context) {
	tenant, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	roomScope := fmt.Sprintf("agent:%d", agent.UserID)
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
		userID, _ := strconv.ParseUint(strings.TrimPrefix(request.Scope, "user:"), 10, 64)
		if err := h.work.EnsureOwnedUser(agent.UserID, userID); err != nil {
			constants.SendError(c, http.StatusForbidden, "不能回复其他房间客服", err)
			return
		}
	} else {
		constants.SendError(c, http.StatusBadRequest, "客服会话不正确", nil)
		return
	}
	result, err := h.chat.Reply(request.Scope, request.RoomType, roomScope, request.GameID, request.Content, firstTenantName(tenant))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送回复失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "回复已发送", result)
}

func (h *WorkspaceHandler) SendRoomRedPacket(c *gin.Context) {
	tenant, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	roomScope := fmt.Sprintf("agent:%d", agent.UserID)
	var request struct {
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
	result, err := h.chat.SendRedPacket(roomScope, roomScope, request.GameID, services.ChatRedPacketInput{Count: request.Count, TotalAmount: request.TotalAmount, MinDailyTurnover: request.MinDailyTurnover, Greeting: request.Greeting, Cover: request.Cover}, firstTenantName(tenant))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送红包失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "红包已发送到聊天室", result)
}

func (h *WorkspaceHandler) RoomRobotStatus(c *gin.Context) {
	if _, _, ok := h.ownedAgent(c); !ok {
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", services.RoomActivityStatusSnapshot())
}
func (h *WorkspaceHandler) RunRoomRobot(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
	if !ok {
		return
	}
	result, err := services.RunRoomActivityOnceForAgent(agent.UserID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行房间自动活跃失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间自动活跃已执行", result)
}

func (h *WorkspaceHandler) SetLotteryRoomStatus(c *gin.Context) {
	_, agent, ok := h.ownedAgent(c)
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
	result, err := h.chat.SetLotteryRoomEnabled(agent.UserID, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存彩票室状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "彩票室状态已保存", result)
}

func firstTenantName(account user.User) string {
	if strings.TrimSpace(account.Nickname) != "" {
		return account.Nickname
	}
	return account.Username
}
