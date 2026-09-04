package admin

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func sourceDiagnosticAdmin(c *gin.Context) bool {
	raw, _ := c.Get("admin_user")
	account, ok := raw.(user.User)
	if !ok || account.Role != "admin" || account.Status != 1 {
		constants.SendError(c, http.StatusForbidden, "仅平台管理员可查看或检测开奖来源", nil)
		return false
	}
	return true
}

func (h *DashboardHandler) SourceDiagnostics(c *gin.Context) {
	if !sourceDiagnosticAdmin(c) {
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		constants.SendError(c, http.StatusBadRequest, "来源目录不接受自定义查询参数", nil)
		return
	}
	result, err := h.lottery.SourceDiagnostics(c.Request.Context())
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取来源诊断目录失败", nil)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *DashboardHandler) ProbeSource(c *gin.Context) {
	if !sourceDiagnosticAdmin(c) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1025))
	var fields map[string]json.RawMessage
	var key string
	valid := err == nil && len(body) <= 1024 && c.Request.URL.RawQuery == "" && !c.Request.URL.ForceQuery && json.Unmarshal(body, &fields) == nil && len(fields) == 1
	if valid {
		valid = json.Unmarshal(fields["source_key"], &key) == nil && services.IsSourceDiagnosticKey(key)
	}
	if !valid {
		constants.SendError(c, http.StatusBadRequest, "只接受固定目录中的 source_key，不接收URL、签名或其他参数", nil)
		return
	}
	// No sync/import call: this service does not access the database. Existing
	// platform-admin HTTP audit records the diagnostic action only.
	result := h.lottery.ProbeSource(c.Request.Context(), key)
	constants.SendSuccess(c, http.StatusOK, "只读来源检测完成", result)
}
