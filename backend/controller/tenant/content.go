package tenant

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func tenantWorkspaceID(c *gin.Context) (uint64, bool) {
	raw, ok := c.Get("tenant_user")
	account, valid := raw.(user.User)
	if !ok || !valid || account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return 0, false
	}
	return account.WorkspaceID, true
}

func (h *WorkspaceHandler) Activities(c *gin.Context) {
	workspaceID, ok := tenantWorkspaceID(c)
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
	h.saveActivity(c, false)
}

func (h *WorkspaceHandler) UpdateActivity(c *gin.Context) {
	h.saveActivity(c, true)
}

func (h *WorkspaceHandler) saveActivity(c *gin.Context, updating bool) {
	workspaceID, ok := tenantWorkspaceID(c)
	if !ok {
		return
	}
	var request services.ActivityPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	activityService := services.NewActivityAdminService(h.db)
	var result *services.ActivityView
	var err error
	if updating {
		var id uint64
		id, err = strconv.ParseUint(c.Param("id"), 10, 64)
		if err == nil {
			result, err = activityService.UpdateForWorkspace(workspaceID, id, request)
		}
	} else {
		result, err = activityService.CreateForWorkspace(workspaceID, request)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存活动失败", err)
		return
	}
	status := http.StatusOK
	if !updating {
		status = http.StatusCreated
	}
	constants.SendSuccess(c, status, "活动已保存", result)
}

func (h *WorkspaceHandler) SetActivityStatus(c *gin.Context) {
	workspaceID, ok := tenantWorkspaceID(c)
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
	workspaceID, ok := tenantWorkspaceID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err == nil {
		err = services.NewActivityAdminService(h.db).DeleteForWorkspace(workspaceID, id)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动已删除", gin.H{"id": id})
}

func (h *WorkspaceHandler) WalletChannels(c *gin.Context) {
	workspaceID, ok := tenantWorkspaceID(c)
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
	h.saveWalletChannel(c, false)
}

func (h *WorkspaceHandler) UpdateWalletChannel(c *gin.Context) {
	h.saveWalletChannel(c, true)
}

func (h *WorkspaceHandler) saveWalletChannel(c *gin.Context, updating bool) {
	workspaceID, ok := tenantWorkspaceID(c)
	if !ok {
		return
	}
	var request services.PaymentChannelPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	walletService := services.NewWalletAdminService(h.db)
	var result *services.PaymentChannelView
	var err error
	if updating {
		var id uint64
		id, err = strconv.ParseUint(c.Param("id"), 10, 64)
		if err == nil {
			result, err = walletService.UpdateForWorkspace(workspaceID, id, request)
		}
	} else {
		result, err = walletService.CreateForWorkspace(workspaceID, request)
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
	workspaceID, ok := tenantWorkspaceID(c)
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
	workspaceID, ok := tenantWorkspaceID(c)
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
