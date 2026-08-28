package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlanHandler struct{ plans *services.PlanContentService }

func NewPlanHandler(db *gorm.DB) *PlanHandler {
	return &PlanHandler{plans: services.NewPlanContentService(db)}
}

func adminPlanWorkspace(c *gin.Context, input uint64) (uint64, bool) {
	value := input
	if value == 0 {
		value, _ = strconv.ParseUint(c.Query("workspace_id"), 10, 64)
	}
	if value == 0 {
		constants.SendError(c, http.StatusBadRequest, "请选择要配置的房间", nil)
		return 0, false
	}
	c.Set("target_workspace_id", value)
	return value, true
}

func (h *PlanHandler) List(c *gin.Context) {
	workspaceID, ok := adminPlanWorkspace(c, 0)
	if !ok {
		return
	}
	result, err := h.plans.ListAdmin(workspaceID)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *PlanHandler) Create(c *gin.Context) {
	var request services.PlanRecommendationInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "推荐内容不正确", err)
		return
	}
	workspaceID, ok := adminPlanWorkspace(c, request.WorkspaceID)
	if !ok {
		return
	}
	result, err := h.plans.Create(workspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "新增计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "计划推荐已新增", result)
}

func (h *PlanHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "推荐编号不正确", err)
		return
	}
	var request services.PlanRecommendationInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "推荐内容不正确", err)
		return
	}
	workspaceID, ok := adminPlanWorkspace(c, request.WorkspaceID)
	if !ok {
		return
	}
	result, err := h.plans.Update(workspaceID, id, request)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "计划推荐已保存", result)
}

func (h *PlanHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "推荐编号不正确", err)
		return
	}
	workspaceID, ok := adminPlanWorkspace(c, 0)
	if !ok {
		return
	}
	if err := h.plans.Delete(workspaceID, id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除计划推荐失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "计划推荐已删除", gin.H{"id": id})
}
