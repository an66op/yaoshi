package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OpsHandler struct {
	activities    *services.ActivityAdminService
	special       *services.SpecialAdminService
	entertainment *services.EntertainmentAdminService
	notify        *services.NotifyAdminService
	rebates       *services.RebateAdminService
	chat          *services.ChatAdminService
}

func NewOpsHandler(db *gorm.DB) *OpsHandler {
	return &OpsHandler{
		activities:    services.NewActivityAdminService(db),
		special:       services.NewSpecialAdminService(db),
		entertainment: services.NewEntertainmentAdminService(db),
		notify:        services.NewNotifyAdminService(db),
		rebates:       services.NewRebateAdminService(db),
		chat:          services.NewChatAdminService(db),
	}
}

func (h *OpsHandler) ListChatConversations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	result, err := h.chat.Conversations(c.Query("room_type"), c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取会话失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) ListChatMessages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	beforeID, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	result, err := h.chat.Messages(c.Query("scope"), c.Query("room_type"), limit, beforeID)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取聊天记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) ReplyChat(c *gin.Context) {
	var request struct {
		Scope    string `json:"scope" binding:"required"`
		RoomType string `json:"room_type" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "回复参数不正确", err)
		return
	}
	operator, _ := c.Get("username")
	result, err := h.chat.Reply(request.Scope, request.RoomType, request.Content, operatorName(operator))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送回复失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "回复已发送", result)
}

func (h *OpsHandler) DeleteChatMessage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "消息 ID 不正确", err)
		return
	}
	operator, _ := c.Get("username")
	if err := h.chat.DeleteMessage(id, operatorName(operator)); err != nil {
		constants.SendError(c, http.StatusBadRequest, "撤回消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "消息已撤回", gin.H{"id": id})
}

func (h *OpsHandler) SetChatMute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("userID"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "用户 ID 不正确", err)
		return
	}
	var request struct {
		Minutes int    `json:"minutes"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "禁言参数不正确", err)
		return
	}
	result, err := h.chat.SetMute(id, request.Minutes, request.Reason)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新禁言失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "禁言状态已更新", gin.H{"user_id": result.UserID, "muted_until": result.MutedUntil, "mute_reason": result.MuteReason})
}

func (h *OpsHandler) SetChatAnnouncement(c *gin.Context) {
	var request struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "公告参数不正确", err)
		return
	}
	content, err := h.chat.SetAnnouncement(request.Content)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "更新公告失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间公告已更新", gin.H{"content": content})
}

func operatorName(value any) string {
	name, _ := value.(string)
	return name
}

func (h *OpsHandler) ListActivities(c *gin.Context) {
	result, err := h.activities.List(c.Query("status"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) CreateActivity(c *gin.Context) {
	var request services.ActivityPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	result, err := h.activities.Create(request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "活动已创建", result)
}

func (h *OpsHandler) UpdateActivity(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	var request services.ActivityPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	result, err := h.activities.Update(id, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动已更新", result)
}

func (h *OpsHandler) SetActivityStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请提供活动状态", err)
		return
	}
	result, err := h.activities.SetStatus(id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新活动状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动状态已更新", result)
}

func (h *OpsHandler) DeleteActivity(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	if err := h.activities.Delete(id); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "删除活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "活动已删除", gin.H{"id": id})
}

func (h *OpsHandler) SpecialOverview(c *gin.Context) {
	result, err := h.special.Overview()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取靓号数据失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) AddSpecialNumbers(c *gin.Context) {
	var request struct {
		Numbers string `json:"numbers"`
		Level   string `json:"level"`
		Remark  string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "参数不正确", err)
		return
	}
	parts := strings.FieldsFunc(request.Numbers, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';' || r == ' ' || r == '\t'
	})
	created, err := h.special.AddResources(parts, request.Level, request.Remark)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "添加靓号失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "靓号已添加", gin.H{"created": created})
}

func (h *OpsHandler) CreateSpecialCampaign(c *gin.Context) {
	var request struct {
		Title  string `json:"title" binding:"required"`
		Rule   string `json:"rule_text"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动参数不正确", err)
		return
	}
	result, err := h.special.CreateCampaign(request.Title, request.Rule, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建靓号活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "靓号活动已创建", result)
}

func (h *OpsHandler) GrantSpecialNumber(c *gin.Context) {
	var request struct {
		CampaignID uint64 `json:"campaign_id" binding:"required"`
		ResourceID uint64 `json:"resource_id" binding:"required"`
		UserID     uint64 `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "发放参数不正确", err)
		return
	}
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	if err := h.special.Grant(request.CampaignID, request.ResourceID, request.UserID, operatorName); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "发放靓号失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "房间号已发放给代理", gin.H{"ok": true})
}

func (h *OpsHandler) AssignSpecialRoom(c *gin.Context) {
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

func (h *OpsHandler) ListEntertainment(c *gin.Context) {
	result, err := h.entertainment.List()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取娱乐平台失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) UpsertEntertainment(c *gin.Context) {
	var request services.PlatformPayload
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "平台参数不正确", err)
		return
	}
	result, err := h.entertainment.Upsert(request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存娱乐平台失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "娱乐平台已保存", result)
}

func (h *OpsHandler) SetEntertainmentStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "平台 ID 不正确", err)
		return
	}
	var request struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请提供平台状态", err)
		return
	}
	result, err := h.entertainment.SetStatus(id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新平台状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "平台状态已更新", result)
}

func (h *OpsHandler) ListNotifications(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.notify.List(limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取通知失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) MarkNotificationRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "通知 ID 不正确", err)
		return
	}
	if err := h.notify.MarkRead(id); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "标记已读失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已标记已读", gin.H{"id": id})
}

func (h *OpsHandler) MarkAllNotificationsRead(c *gin.Context) {
	if err := h.notify.MarkAllRead(); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "标记已读失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "全部已读", gin.H{"ok": true})
}

func (h *OpsHandler) RebatePreview(c *gin.Context) {
	result, err := h.rebates.PreviewToday()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取回水预览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OpsHandler) RunRebate(c *gin.Context) {
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.rebates.RunToday(operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "回水结算失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "回水已结算入账", result)
}
