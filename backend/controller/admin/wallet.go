package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WalletHandler struct{ wallet *services.WalletAdminService }

func NewWalletHandler(db *gorm.DB) *WalletHandler {
	return &WalletHandler{wallet: services.NewWalletAdminService(db)}
}

func (h *WalletHandler) List(c *gin.Context) {
	result, err := h.wallet.List(services.WalletListFilter{
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
	var request services.PaymentChannelPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	result, err := h.wallet.Create(request)
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
	result, err := h.wallet.Update(id, request)
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
	result, err := h.wallet.SetStatus(id, request.Status)
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
	if err := h.wallet.Delete(id); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "删除收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式已删除", gin.H{"id": id})
}

func parseChannelID(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}
