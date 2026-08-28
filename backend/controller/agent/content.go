package agent

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func agentWorkspaceID(c *gin.Context) (uint64, bool) {
	account, _, _, ok := agentIdentity(c)
	if !ok {
		return 0, false
	}
	if account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusForbidden, "当前房间未配置", nil)
		return 0, false
	}
	return account.WorkspaceID, true
}

func (h *WorkspaceHandler) Activities(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	result, err := services.NewActivityAdminService(h.db).ListForWorkspace(workspaceID, c.DefaultQuery("status", "all"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) CreateActivity(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	var request services.ActivityPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	result, err := services.NewActivityAdminService(h.db).CreateForWorkspace(workspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "创建活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "活动已创建", result)
}

func (h *WorkspaceHandler) UpdateActivity(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动编号不正确", err)
		return
	}
	var request services.ActivityPayload
	if err = c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	result, err := services.NewActivityAdminService(h.db).UpdateForWorkspace(workspaceID, id, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动已更新", result)
}

func (h *WorkspaceHandler) SetActivityStatus(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动编号不正确", err)
		return
	}
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err = c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动状态不正确", err)
		return
	}
	result, err := services.NewActivityAdminService(h.db).SetStatusForWorkspace(workspaceID, id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新活动状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动状态已更新", result)
}

func (h *WorkspaceHandler) DeleteActivity(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动编号不正确", err)
		return
	}
	if err = services.NewActivityAdminService(h.db).DeleteForWorkspace(workspaceID, id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动已删除", gin.H{"id": id})
}

func (h *WorkspaceHandler) WalletChannels(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	result, err := services.NewWalletAdminService(h.db).ListForWorkspace(workspaceID, services.WalletListFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all")})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WorkspaceHandler) CreateWalletChannel(c *gin.Context) {
	h.upsertWalletChannel(c, false)
}

func (h *WorkspaceHandler) UpdateWalletChannel(c *gin.Context) {
	h.upsertWalletChannel(c, true)
}

func (h *WorkspaceHandler) upsertWalletChannel(c *gin.Context, updating bool) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	var request services.PaymentChannelPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	wallet := services.NewWalletAdminService(h.db)
	var result *services.PaymentChannelView
	var err error
	if updating {
		var id uint64
		id, err = strconv.ParseUint(c.Param("id"), 10, 64)
		if err == nil {
			result, err = wallet.UpdateForWorkspace(workspaceID, id, request)
		}
	} else {
		result, err = wallet.CreateForWorkspace(workspaceID, request)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存收款方式失败", err)
		return
	}
	status := http.StatusOK
	if !updating {
		status = http.StatusCreated
	}
	constants.SendSuccess(c, status, "收款方式已保存", result)
}

func (h *WorkspaceHandler) SetWalletChannelStatus(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err == nil {
		err = c.ShouldBindJSON(&request)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "状态参数不正确", err)
		return
	}
	result, err := services.NewWalletAdminService(h.db).SetStatusForWorkspace(workspaceID, id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式状态已更新", result)
}

func (h *WorkspaceHandler) DeleteWalletChannel(c *gin.Context) {
	workspaceID, ok := agentWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err == nil {
		err = services.NewWalletAdminService(h.db).DeleteForWorkspace(workspaceID, id)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式已删除", gin.H{"id": id})
}
