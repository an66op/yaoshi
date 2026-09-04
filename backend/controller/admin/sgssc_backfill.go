package admin

import (
	"backend/constants"
	"backend/data/models/user"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *DashboardHandler) SGSSCBackfillStatus(c *gin.Context) {
	before, err := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limitErr != nil || limit < 1 || limit > 50 {
		constants.SendError(c, http.StatusBadRequest, "恢复记录分页参数无效", nil)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.lottery.SGSSCBackfillStatus(ctx, before, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取SG历史恢复记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

// A fixed, audited queue action. Administrators cannot supply a source URL,
// substitute draw numbers, choose an unbounded date range or run settlement
// synchronously through this endpoint.
func (h *DashboardHandler) QueueSGSSCBackfill(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1025))
	valid := err == nil && len(body) <= 1024 && c.Request.URL.RawQuery == "" && !c.Request.URL.ForceQuery
	if strings.TrimSpace(string(body)) != "" {
		var object map[string]json.RawMessage
		valid = valid && json.Unmarshal(body, &object) == nil && object != nil && len(object) == 0
	}
	if !valid {
		constants.SendError(c, http.StatusBadRequest, "SG历史补采不接收自定义期号、号码、来源或其他业务参数", nil)
		return
	}
	raw, _ := c.Get("admin_user")
	account, ok := raw.(user.User)
	if !ok || account.Role != "admin" {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可以登记历史补采", nil)
		return
	}
	requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.lottery.QueueSGSSCBackfill(ctx, account.Username, requestID)
	if err != nil {
		constants.SendError(c, http.StatusConflict, "无法登记SG历史补采，请检查来源绑定和游戏状态", err)
		return
	}
	constants.SendSuccess(c, http.StatusAccepted, result.Message, result)
}
