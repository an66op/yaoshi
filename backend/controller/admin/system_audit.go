package admin

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SystemAuditHandler struct {
	service   *services.SystemAuditService
	lifecycle *services.DataLifecycleService
}

func NewSystemAuditHandler(db *gorm.DB) *SystemAuditHandler {
	return &SystemAuditHandler{service: services.NewSystemAuditService(db), lifecycle: services.NewDataLifecycleService(db)}
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

func (h *SystemAuditHandler) RetentionPolicies(c *gin.Context) {
	if _, err := h.platformActor(c); err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以管理数据生命周期", err)
		return
	}
	workspaceID, err := strconv.ParseUint(c.DefaultQuery("workspace_id", "0"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区参数不正确", err)
		return
	}
	result, err := h.lifecycle.Policies(workspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取数据保留策略失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) DataMaintenanceSummary(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以查看数据维护概况", err)
		return
	}
	result, err := h.lifecycle.Summary(actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取数据维护概况失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) UpdateRetentionPolicy(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以管理数据生命周期", err)
		return
	}
	var input services.UpdateRetentionPolicyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		constants.SendError(c, http.StatusBadRequest, "数据保留策略参数不正确", err)
		return
	}
	result, err := h.lifecycle.UpdatePolicy(c.Param("dataClass"), input, actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存数据保留策略失败", err)
		return
	}
	if input.WorkspaceID > 0 {
		c.Set("target_workspace_id", input.WorkspaceID)
	}
	constants.SendSuccess(c, http.StatusOK, "数据保留策略已保存", result)
}

func (h *SystemAuditHandler) PreviewCleanup(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以管理数据生命周期", err)
		return
	}
	var input services.CleanupPreviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		constants.SendError(c, http.StatusBadRequest, "清理预览参数不正确", err)
		return
	}
	// The privileged-audit middleware runs after the handler. Copy the domain
	// idempotency key into the request metadata so the immutable audit record
	// can always be correlated with the frozen cleanup run.
	c.Request.Header.Set("X-Request-ID", input.RequestID)
	result, err := h.lifecycle.Preview(input, actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "生成清理预览失败", err)
		return
	}
	if result.WorkspaceID > 0 {
		c.Set("target_workspace_id", result.WorkspaceID)
	}
	c.Params = append(c.Params, gin.Param{Key: "delete_mode", Value: result.DeleteMode})
	constants.SendSuccess(c, http.StatusOK, "清理预览已冻结，请确认后执行", result)
}

func (h *SystemAuditHandler) ExecuteCleanup(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以管理数据生命周期", err)
		return
	}
	var input services.CleanupExecuteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		constants.SendError(c, http.StatusBadRequest, "清理执行参数不正确", err)
		return
	}
	c.Request.Header.Set("X-Request-ID", input.RequestID)
	result, err := h.lifecycle.Execute(input, actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行数据清理失败", err)
		return
	}
	if result.WorkspaceID > 0 {
		c.Set("target_workspace_id", result.WorkspaceID)
	}
	c.Params = append(c.Params, gin.Param{Key: "delete_mode", Value: result.DeleteMode})
	constants.SendSuccess(c, http.StatusOK, "数据生命周期任务已完成", result)
}

func (h *SystemAuditHandler) CleanupRuns(c *gin.Context) {
	if _, err := h.platformActor(c); err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以查看数据生命周期任务", err)
		return
	}
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	var workspaceID *uint64
	if value := c.Query("workspace_id"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			constants.SendError(c, http.StatusBadRequest, "工作区参数不正确", err)
			return
		}
		workspaceID = &parsed
	}
	result, err := h.lifecycle.Runs(beforeID, limit, workspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取数据生命周期任务失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) CleanupRun(c *gin.Context) {
	if _, err := h.platformActor(c); err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以查看数据生命周期任务", err)
		return
	}
	result, err := h.lifecycle.Run(c.Param("requestID"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取数据生命周期任务失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) CleanupArchives(c *gin.Context) {
	if _, err := h.platformActor(c); err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以查看冷归档", err)
		return
	}
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	result, err := h.lifecycle.Archives(c.Param("requestID"), c.DefaultQuery("kind", "bets"), beforeID, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取冷归档失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SystemAuditHandler) RestoreSoftDeleted(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以恢复生命周期数据", err)
		return
	}
	requestID := c.Param("requestID")
	c.Request.Header.Set("X-Request-ID", requestID)
	result, err := h.lifecycle.RestoreSoftDeleted(requestID, actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "恢复软删除数据失败", err)
		return
	}
	if result.WorkspaceID > 0 {
		c.Set("target_workspace_id", result.WorkspaceID)
	}
	constants.SendSuccess(c, http.StatusOK, "聊天和通知已恢复", result)
}

func (h *SystemAuditHandler) RestoreRobotArchive(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以恢复生命周期数据", err)
		return
	}
	requestID := c.Param("requestID")
	c.Request.Header.Set("X-Request-ID", requestID)
	result, err := h.lifecycle.RestoreRobotArchive(requestID, actor)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "恢复机器人冷归档失败", err)
		return
	}
	if result.WorkspaceID > 0 {
		c.Set("target_workspace_id", result.WorkspaceID)
	}
	constants.SendSuccess(c, http.StatusOK, "机器人冷归档已恢复到热表", result)
}

func (h *SystemAuditHandler) platformActor(c *gin.Context) (services.LifecycleActor, error) {
	raw, _ := c.Get("admin_user")
	account, ok := raw.(user.User)
	if !ok {
		return services.LifecycleActor{}, fmt.Errorf("admin identity missing")
	}
	actor := services.LifecycleActor{UserID: account.UserID, Username: account.Username, WorkspaceID: account.WorkspaceID}
	if err := h.lifecycle.EnsurePlatformAdmin(actor); err != nil {
		return services.LifecycleActor{}, err
	}
	return actor, nil
}

func (h *SystemAuditHandler) RecoverSettlement(c *gin.Context) {
	var request struct {
		Limit int `json:"limit"`
	}
	_ = c.ShouldBindJSON(&request)
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.service.RecoverSettlement(c.Request.Context(), request.Limit, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "恢复结算积压失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "结算积压检查完成", result)
}

func (h *SystemAuditHandler) RefundAbnormalBet(c *gin.Context) {
	actor, err := h.platformActor(c)
	if err != nil {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以处理异常注单", err)
		return
	}
	betID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || betID == 0 {
		constants.SendError(c, http.StatusBadRequest, "注单编号不正确", err)
		return
	}
	// The immutable ledger reference is also the privileged-audit correlation
	// key, allowing an idempotent retry to be traced to the same ticket.
	c.Request.Header.Set("X-Request-ID", fmt.Sprintf("reconciliation_refund:%d", betID))
	result, err := h.service.RefundAbnormalPendingBet(c.Request.Context(), betID, actor.Username)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "异常注单退款失败", err)
		return
	}
	c.Set("target_workspace_id", result.WorkspaceID)
	constants.SendSuccess(c, http.StatusOK, "异常注单已退款关闭", result)
}
