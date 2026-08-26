package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AgentHandler struct {
	agents  *services.AgentAdminService
	special *services.SpecialAdminService
	trading *services.TradingAdminService
}

func NewAgentHandler(db *gorm.DB) *AgentHandler {
	return &AgentHandler{
		agents:  services.NewAgentAdminService(db),
		special: services.NewSpecialAdminService(db),
		trading: services.NewTradingAdminService(db),
	}
}

func (h *AgentHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.agents.List(c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取代理列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *AgentHandler) Promote(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		RoomCode string `json:"room_code"`
	}
	_ = c.ShouldBindJSON(&request)
	result, err := h.agents.Promote(id, request.RoomCode)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "设置代理失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已设为代理", result)
}

func (h *AgentHandler) Create(c *gin.Context) {
	var request struct {
		Username        string  `json:"username" binding:"required,min=3,max=50"`
		Password        string  `json:"password" binding:"required,min=8,max=72"`
		Email           string  `json:"email" binding:"omitempty,email"`
		Nickname        string  `json:"nickname" binding:"max=50"`
		Phone           string  `json:"phone" binding:"max=30"`
		RoomCode        string  `json:"room_code" binding:"required"`
		RoomName        string  `json:"room_name" binding:"max=30"`
		RoomLogo        string  `json:"room_logo"`
		Remark          string  `json:"remark" binding:"max=500"`
		Status          int     `json:"status"`
		RebateRate      float64 `json:"rebate_rate"`
		ProfitShareRate float64 `json:"profit_share_rate"`
		TenantID        *uint64 `json:"tenant_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理资料不正确", err)
		return
	}
	result, err := h.agents.Create(services.CreateAgentInput{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate, TenantID: request.TenantID})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "创建代理失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "代理账号已创建", result)
}

func (h *AgentHandler) Update(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Email           string  `json:"email" binding:"omitempty,email"`
		Nickname        string  `json:"nickname" binding:"max=50"`
		Phone           string  `json:"phone" binding:"max=30"`
		RoomCode        string  `json:"room_code" binding:"required"`
		RoomName        string  `json:"room_name" binding:"max=30"`
		RoomLogo        string  `json:"room_logo"`
		Remark          string  `json:"remark" binding:"max=500"`
		Status          int     `json:"status"`
		RebateRate      float64 `json:"rebate_rate"`
		ProfitShareRate float64 `json:"profit_share_rate"`
		TenantID        *uint64 `json:"tenant_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理资料不正确", err)
		return
	}
	result, err := h.agents.Update(id, services.UpdateAgentInput{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status, RebateRate: request.RebateRate, ProfitShareRate: request.ProfitShareRate, TenantID: request.TenantID})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存代理失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "代理资料已保存", result)
}

func (h *AgentHandler) GetTrading(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理编号不正确", err)
		return
	}
	result, err := h.trading.GetRoom(id, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取房间赔率与返水失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *AgentHandler) UpdateTrading(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "代理编号不正确", err)
		return
	}
	var request services.UpdateRoomTradingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间赔率与返水参数不正确", err)
		return
	}
	result, err := h.trading.UpdateRoom(id, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存房间赔率与返水失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间赔率与返水已保存", result)
}

func (h *AgentHandler) ResetPassword(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "新密码长度需为 8–72 个字符", err)
		return
	}
	if err := h.agents.ResetPassword(id, request.Password); err != nil {
		constants.SendError(c, http.StatusBadRequest, "重置密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "代理登录密码已重置", gin.H{"id": id})
}

func (h *AgentHandler) AssignRoom(c *gin.Context) {
	var request struct {
		ResourceID uint64 `json:"resource_id" binding:"required"`
		UserID     uint64 `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "分配参数不正确", err)
		return
	}
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	if err := h.special.AssignRoom(request.ResourceID, request.UserID, operatorName); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "分配房间号失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间号已分配给代理", gin.H{"ok": true})
}
