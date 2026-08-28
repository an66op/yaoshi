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

type WalletHandler struct {
	wallet *services.WalletAdminService
	db     *gorm.DB
}

func NewWalletHandler(db *gorm.DB) *WalletHandler {
	return &WalletHandler{wallet: services.NewWalletAdminService(db), db: db}
}

func (h *WalletHandler) List(c *gin.Context) {
	workspaceID, err := resolveAdminWorkspaceID(c, h.db)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", err)
		return
	}
	result, err := h.wallet.ListForWorkspace(workspaceID, services.WalletListFilter{
		Query:  c.Query("query"),
		Status: c.DefaultQuery("status", "all"),
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取钱包配置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *WalletHandler) Create(c *gin.Context) {
	workspaceID, err := resolveAdminWorkspaceID(c, h.db)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", err)
		return
	}
	var request services.PaymentChannelPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	result, err := h.wallet.CreateForWorkspace(workspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "收款方式已创建", result)
}

func (h *WalletHandler) Update(c *gin.Context) {
	id, err := parseChannelID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式编号不正确", err)
		return
	}
	var request services.PaymentChannelPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	workspaceID, workspaceErr := resolveAdminWorkspaceID(c, h.db)
	if workspaceErr != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", workspaceErr)
		return
	}
	result, err := h.wallet.UpdateForWorkspace(workspaceID, id, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式已更新", result)
}

func (h *WalletHandler) SetStatus(c *gin.Context) {
	id, err := parseChannelID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式编号不正确", err)
		return
	}
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "状态参数不正确", err)
		return
	}
	workspaceID, workspaceErr := resolveAdminWorkspaceID(c, h.db)
	if workspaceErr != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", workspaceErr)
		return
	}
	result, err := h.wallet.SetStatusForWorkspace(workspaceID, id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新收款方式状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式状态已更新", result)
}

func (h *WalletHandler) Delete(c *gin.Context) {
	id, err := parseChannelID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式编号不正确", err)
		return
	}
	workspaceID, workspaceErr := resolveAdminWorkspaceID(c, h.db)
	if workspaceErr != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", workspaceErr)
		return
	}
	if err := h.wallet.DeleteForWorkspace(workspaceID, id); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "删除收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式已删除", gin.H{"id": id})
}

func parseChannelID(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}

func resolveAdminWorkspaceID(c *gin.Context, db *gorm.DB) (uint64, error) {
	value := c.Query("workspace_id")
	if value == "" {
		raw, _ := c.Get("workspace_id")
		id, _ := raw.(uint64)
		if id == 0 {
			return 0, strconv.ErrSyntax
		}
		return id, nil
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, strconv.ErrSyntax
	}
	var count int64
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ? AND type IN ?", id, []string{workspacemodel.TypePlatform, workspacemodel.TypeTenant, workspacemodel.TypeAgent}).Count(&count).Error; err != nil || count != 1 {
		return 0, strconv.ErrRange
	}
	c.Set("target_workspace_id", id)
	return id, nil
}
