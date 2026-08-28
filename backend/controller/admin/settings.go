package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingsHandler struct {
	settings *services.SettingsAdminService
	db       *gorm.DB
}

func NewSettingsHandler(db *gorm.DB) *SettingsHandler {
	return &SettingsHandler{settings: services.NewSettingsAdminService(db), db: db}
}

func (h *SettingsHandler) RobotSetting(c *gin.Context) {
	id, ok := h.robotWorkspaceID(c)
	if !ok {
		return
	}
	result, err := services.RobotSettingForWorkspace(h.db, id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取机器人调度失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *SettingsHandler) UpdateRobotSetting(c *gin.Context) {
	id, ok := h.robotWorkspaceID(c)
	if !ok {
		return
	}
	var request services.UpdateRobotSettingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人调度参数不正确", err)
		return
	}
	result, err := services.UpdateRobotSettingForWorkspace(h.db, id, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存机器人调度失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "机器人调度已生效", result)
}

func (h *SettingsHandler) robotWorkspaceID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Query("workspace_id"), 10, 64)
	if err != nil || id == 0 {
		workspaceRaw, exists := c.Get("workspace_id")
		current, typeOK := workspaceRaw.(uint64)
		if !exists || !typeOK || current == 0 {
			constants.SendError(c, http.StatusBadRequest, "机器人工作区不正确", err)
			return 0, false
		}
		id = current
	}
	if _, err := services.EnabledRobotWorkspace(h.db, id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "机器人工作区不存在或已停用", err)
		return 0, false
	}
	c.Set("target_workspace_id", id)
	return id, true
}

func (h *SettingsHandler) Get(c *gin.Context) {
	workspaceID, err := h.workspaceID(c)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", err)
		return
	}
	result, err := h.settings.GetForWorkspace(workspaceID)
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
	workspaceID, err := h.workspaceID(c)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "工作区不正确", err)
		return
	}
	result, err := h.settings.UpdateForWorkspace(workspaceID, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存系统设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "系统设置已保存", result)
}

func (h *SettingsHandler) workspaceID(c *gin.Context) (uint64, error) {
	value := c.Query("workspace_id")
	if value == "" {
		raw, _ := c.Get("workspace_id")
		id, _ := raw.(uint64)
		return id, nil
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, strconv.ErrSyntax
	}
	var count int64
	if err := h.db.Table("workspaces").Where("id = ?", id).Count(&count).Error; err != nil || count != 1 {
		return 0, strconv.ErrRange
	}
	return id, nil
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
