package agent

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
	if !ok || !accountOK || account.UserID == 0 {
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
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{"id": account.UserID, "public_id": account.PublicID, "username": account.Username, "nickname": account.Nickname, "role": account.Role, "room_code": account.AgentRoomCode, "room_name": account.AgentRoomName, "room_logo": account.AgentRoomLogo, "room_scope": scope})
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

func (h *WorkspaceHandler) Dashboard(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := h.work.Dashboard(id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间工作台失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
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
	result, err := h.work.Applications(id, services.ApplicationFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Page: page, PageSize: size})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间申请失败", err)
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
		Decision       string  `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64 `json:"received_amount"`
		Remark         string  `json:"remark" binding:"max=500"`
	}
	if err = c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := h.work.ReviewApplication(agentID, applicationID, services.ReviewApplicationInput{Decision: req.Decision, ReceivedAmount: req.ReceivedAmount, Remark: req.Remark, Operator: "房间 " + account.AgentRoomCode + " 代理"})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}

func (h *WorkspaceHandler) Trading(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	result, err := h.trading.GetRoom(id, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) UpdateTrading(c *gin.Context) {
	_, id, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	var req services.UpdateRoomTradingInput
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率与返水参数不正确", err)
		return
	}
	result, err := h.trading.UpdateRoom(id, req)
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
	_, agentID, roomScope, ok := agentIdentity(c)
	if !ok {
		return
	}
	scope := strings.TrimSpace(c.Query("scope"))
	roomType := c.Query("room_type")
	if roomType == "group" {
		scope = roomScope
	} else if strings.HasPrefix(scope, "user:") {
		userID, _ := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
		if err := h.work.EnsureOwnedUser(agentID, userID); err != nil {
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

func (h *WorkspaceHandler) Reply(c *gin.Context) {
	account, agentID, roomScope, ok := agentIdentity(c)
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
		uid, _ := strconv.ParseUint(strings.TrimPrefix(req.Scope, "user:"), 10, 64)
		if err := h.work.EnsureOwnedUser(agentID, uid); err != nil {
			constants.SendError(c, http.StatusForbidden, "不能回复其他房间客服", err)
			return
		}
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
		Count: req.Count, TotalAmount: req.TotalAmount, MinDailyTurnover: req.MinDailyTurnover, Greeting: req.Greeting, Cover: req.Cover,
	}, defaultName(account))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送红包失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "红包已发送到聊天室", result)
}

func (h *WorkspaceHandler) RobotStatus(c *gin.Context) {
	_, _, _, ok := agentIdentity(c)
	if !ok {
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", services.RoomActivityStatusSnapshot())
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
