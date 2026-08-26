package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SystemAuditHandler struct{ service *services.SystemAuditService }

func NewSystemAuditHandler(db *gorm.DB) *SystemAuditHandler {
	return &SystemAuditHandler{service: services.NewSystemAuditService(db)}
}

func (h *SystemAuditHandler) Logs(c *gin.Context) {
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	result, err := h.service.Logs(beforeID, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取审计日志失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) Reconciliation(c *gin.Context) {
	result, err := h.service.Reconciliation()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行账务对账失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}
