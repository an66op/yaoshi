package user

import (
	"backend/constants"
	"backend/services"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type memberPlanService interface {
	Catalog(workspaceID uint64) ([]services.PlanGameSummary, error)
	Detail(workspaceID uint64, gameID string, historyLimits ...int) (services.PlanDetail, error)
	StreamDetail(workspaceID uint64, position int, key string, historyLimits ...int) (services.PlanStreamDetail, error)
	ActivateStream(ctx context.Context, workspaceID uint64, position int, key string, historyLimits ...int) (services.PlanStreamDetail, error)
	ActivateGame(ctx context.Context, workspaceID uint64, gameID string, historyLimits ...int) (services.PlanDetail, error)
}

func memberPlanHistoryLimit(c *gin.Context) (int, bool) {
	limit, err := strconv.Atoi(c.DefaultQuery("history_limit", "6"))
	if err != nil || limit < 1 || limit > 10 {
		constants.SendError(c, http.StatusBadRequest, "历史期数必须为1至10", err)
		return 0, false
	}
	return limit, true
}

func (h *memberHandler) ActivatePlanStream(c *gin.Context) {
	if _, ok := memberUserID(c); !ok {
		return
	}
	roomID, ok := c.Get("workspace_id")
	workspaceID, valid := roomID.(uint64)
	if !ok || !valid || workspaceID == 0 {
		constants.SendError(c, http.StatusForbidden, "请先进入房间", nil)
		return
	}
	limit, ok := memberPlanHistoryLimit(c)
	if !ok {
		return
	}
	var input struct {
		Position int    `json:"position"`
		PlanKey  string `json:"plan_key"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		constants.SendError(c, http.StatusBadRequest, "推荐位置或方案不正确", err)
		return
	}
	if c.Param("gameID") != "speed-racing" {
		if input.Position != 0 || input.PlanKey != "" {
			constants.SendError(c, http.StatusBadRequest, "该彩种仅支持默认单期推荐", nil)
			return
		}
		result, err := h.plans.ActivateGame(c.Request.Context(), workspaceID, c.Param("gameID"), limit)
		if err != nil {
			constants.SendError(c, http.StatusBadRequest, "读取本次访问推荐失败", err)
			return
		}
		constants.SendSuccess(c, http.StatusOK, "仅本次访问的彩种尝试生成当前开放期", result)
		return
	}
	if input.Position == 0 {
		input.Position = 1
	}
	if input.PlanKey == "" {
		input.PlanKey = services.DefaultPlanKey
	}
	result, err := h.plans.ActivateStream(c.Request.Context(), workspaceID, input.Position, input.PlanKey, limit)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "启用推荐方案失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "仅本次访问的方案尝试生成当前开放期", result)
}
