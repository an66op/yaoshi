package admin

import (
	"backend/constants"
	workspacemodel "backend/data/models/workspace"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TenantHandler struct {
	db      *gorm.DB
	tenants *services.TenantAdminService
	trading *services.TradingAdminService
	chat    *services.ChatAdminService
}

func NewTenantHandler(db *gorm.DB) *TenantHandler {
	return &TenantHandler{db: db, tenants: services.NewTenantAdminService(db), trading: services.NewTradingAdminService(db), chat: services.NewChatAdminService(db)}
}

func (h *TenantHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.tenants.List(c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *TenantHandler) Create(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Password string `json:"password" binding:"required,min=8,max=72"`
		Email    string `json:"email" binding:"omitempty,email"`
		Nickname string `json:"nickname" binding:"max=50"`
		Phone    string `json:"phone" binding:"max=30"`
		RoomCode string `json:"room_code" binding:"omitempty,min=5,max=12"`
		RoomName string `json:"room_name" binding:"max=30"`
		RoomLogo string `json:"room_logo"`
		Remark   string `json:"remark" binding:"max=500"`
		Status   int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户资料不正确", err)
		return
	}
	result, err := h.tenants.Create(services.TenantPayload{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "创建租户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "租户账号已创建", result)
}

func (h *TenantHandler) Update(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户编号不正确", err)
		return
	}
	var request struct {
		Email    string `json:"email" binding:"omitempty,email"`
		Nickname string `json:"nickname" binding:"max=50"`
		Phone    string `json:"phone" binding:"max=30"`
		RoomCode string `json:"room_code" binding:"omitempty,min=5,max=12"`
		RoomName string `json:"room_name" binding:"max=30"`
		RoomLogo string `json:"room_logo"`
		Remark   string `json:"remark" binding:"max=500"`
		Status   int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户资料不正确", err)
		return
	}
	result, err := h.tenants.Update(id, services.TenantPayload{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, RoomCode: request.RoomCode, RoomName: request.RoomName, RoomLogo: request.RoomLogo, Remark: request.Remark, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存租户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户资料已保存", result)
}

func (h *TenantHandler) ResetPassword(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户编号不正确", err)
		return
	}
	var request struct {
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "新密码长度需为 8–72 个字符", err)
		return
	}
	if err := h.tenants.ResetPassword(id, request.Password); err != nil {
		constants.SendError(c, http.StatusBadRequest, "重置密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户登录密码已重置", gin.H{"id": id})
}

func (h *TenantHandler) tenantWorkspace(c *gin.Context) (workspacemodel.Workspace, bool) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户编号不正确", err)
		return workspacemodel.Workspace{}, false
	}
	var workspace workspacemodel.Workspace
	if err := h.db.Where("owner_user_id = ? AND type = ?", id, workspacemodel.TypeTenant).First(&workspace).Error; err != nil {
		constants.SendError(c, http.StatusNotFound, "租户直属房间不存在", err)
		return workspacemodel.Workspace{}, false
	}
	return workspace, true
}

func (h *TenantHandler) GetTrading(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
	if !ok {
		return
	}
	result, err := h.trading.GetRoomForWorkspace(workspace.ID, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户直属房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *TenantHandler) UpdateTrading(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
	if !ok {
		return
	}
	var request services.UpdateRoomTradingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率与返水参数不正确", err)
		return
	}
	result, err := h.trading.UpdateRoomForWorkspace(workspace.ID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存租户直属房间赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户直属房间赔率与返水已保存", result)
}

func (h *TenantHandler) GetSettings(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
	if !ok {
		return
	}
	result, err := services.NewSettingsAdminService(h.db).GetForWorkspace(workspace.ID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户直属房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *TenantHandler) UpdateSettings(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
	if !ok {
		return
	}
	var request services.UpdateSystemSettingsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "房间设置参数不正确", err)
		return
	}
	result, err := services.NewSettingsAdminService(h.db).UpdateForWorkspace(workspace.ID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存租户直属房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户直属房间设置已保存", result)
}

func (h *TenantHandler) Games(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
	if !ok {
		return
	}
	result, err := services.NewWorkspaceGameService(h.db).List(workspace.ID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户直属房间游戏失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *TenantHandler) SetGameStatus(c *gin.Context) {
	workspace, ok := h.tenantWorkspace(c)
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
	result, err := h.chat.SetLotteryRoomEnabledForWorkspace(workspace, c.Param("gameID"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存租户直属房间游戏状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户直属房间游戏状态已保存", result)
}
