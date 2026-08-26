package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingsHandler struct {
	settings *services.SettingsAdminService
}

func NewSettingsHandler(db *gorm.DB) *SettingsHandler {
	return &SettingsHandler{settings: services.NewSettingsAdminService(db)}
}

func (h *SettingsHandler) Get(c *gin.Context) {
	result, err := h.settings.Get()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取系统设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SettingsHandler) Update(c *gin.Context) {
	var request services.UpdateSystemSettingsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "系统设置参数不正确", err)
		return
	}
	result, err := h.settings.Update(request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存系统设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "系统设置已保存", result)
}

func (h *SettingsHandler) RoomActivityStatus(c *gin.Context) {
	constants.SendSuccess(c, http.StatusOK, "ok", services.RoomActivityStatusSnapshot())
}

func (h *SettingsHandler) RunRoomActivityOnce(c *gin.Context) {
	result, err := services.RunRoomActivityOnce()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行房间自动活跃失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间自动活跃已执行一轮", result)
}
