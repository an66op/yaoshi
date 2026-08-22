package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AgentHandler struct {
	agents  *services.AgentAdminService
	special *services.SpecialAdminService
}

func NewAgentHandler(db *gorm.DB) *AgentHandler {
	return &AgentHandler{
		agents:  services.NewAgentAdminService(db),
		special: services.NewSpecialAdminService(db),
	}
}

func (h *AgentHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.agents.List(c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取代理列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *AgentHandler) Promote(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		RoomCode string `json:"room_code"`
	}
	_ = c.ShouldBindJSON(&request)
	result, err := h.agents.Promote(id, request.RoomCode)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "设置代理失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已设为代理", result)
}

func (h *AgentHandler) AssignRoom(c *gin.Context) {
	var request struct {
		ResourceID uint64 `json:"resource_id" binding:"required"`
		UserID     uint64 `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "分配参数不正确", err)
		return
	}
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	if err := h.special.AssignRoom(request.ResourceID, request.UserID, operatorName); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "分配房间号失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间号已分配给代理", gin.H{"ok": true})
}
