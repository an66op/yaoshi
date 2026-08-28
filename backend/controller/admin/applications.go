package admin

import (
	"backend/constants"
	workspacemodel "backend/data/models/workspace"
	"backend/services"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ApplicationAdminHandler struct {
	db           *gorm.DB
	applications *services.ApplicationAdminService
}

func NewApplicationAdminHandler(db *gorm.DB) *ApplicationAdminHandler {
	return &ApplicationAdminHandler{db: db, applications: services.NewApplicationAdminService(db)}
}

func (h *ApplicationAdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	workspaceID, err := h.optionalRoomWorkspace(c)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请房间不存在", err)
		return
	}
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := h.applications.ListForPlatform(services.ApplicationFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Date: c.Query("date"), Start: c.Query("start"), End: c.Query("end"), WorkspaceID: workspaceID, UserID: userID, Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取申请列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ApplicationAdminHandler) Stats(c *gin.Context) {
	workspaceID, err := h.optionalRoomWorkspace(c)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请房间不存在", err)
		return
	}
	result, err := h.applications.StatsForPlatform(workspaceID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取申请统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ApplicationAdminHandler) Get(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	result, err := h.applications.Get(id)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "申请不存在", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ApplicationAdminHandler) Create(c *gin.Context) {
	var request struct {
		UserID      uint64  `json:"user_id" binding:"required"`
		WorkspaceID uint64  `json:"workspace_id"`
		RequestID   string  `json:"request_id" binding:"max=96"`
		RequestType string  `json:"request_type" binding:"required"`
		PaymentType string  `json:"payment_type" binding:"required"`
		GameID      string  `json:"game_id" binding:"max=40"`
		Amount      float64 `json:"amount"`
		Remark      string  `json:"remark" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请资料不正确", err)
		return
	}
	if request.WorkspaceID > 0 {
		if err := h.validateRoomWorkspace(request.WorkspaceID); err != nil {
			constants.SendError(c, http.StatusBadRequest, "申请房间不存在", err)
			return
		}
		c.Set("target_workspace_id", request.WorkspaceID)
	}
	result, err := h.applications.CreateForPlatform(services.CreateApplicationInput{UserID: request.UserID, WorkspaceID: request.WorkspaceID, RequestID: request.RequestID, RequestType: request.RequestType, PaymentType: request.PaymentType, GameID: request.GameID, Amount: request.Amount, Remark: request.Remark})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "申请已创建", result)
}

func (h *ApplicationAdminHandler) optionalRoomWorkspace(c *gin.Context) (uint64, error) {
	raw := c.Query("workspace_id")
	if raw == "" || raw == "0" {
		return 0, nil
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("房间编号不正确")
	}
	if err := h.validateRoomWorkspace(id); err != nil {
		return 0, err
	}
	c.Set("target_workspace_id", id)
	return id, nil
}

func (h *ApplicationAdminHandler) validateRoomWorkspace(id uint64) error {
	var count int64
	if err := h.db.Model(&workspacemodel.Workspace{}).
		Where("id = ? AND type IN ?", id, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).
		Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("目标房间不存在")
	}
	return nil
}

func (h *ApplicationAdminHandler) Review(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	if current, lookupErr := h.applications.Get(id); lookupErr == nil && current.WorkspaceID > 0 {
		c.Set("target_workspace_id", current.WorkspaceID)
	}
	var request struct {
		Decision       string   `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64  `json:"received_amount"`
		OddsMultiplier *float64 `json:"odds_multiplier"`
		Remark         string   `json:"remark" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := h.applications.Review(id, services.ReviewApplicationInput{Decision: request.Decision, ReceivedAmount: request.ReceivedAmount, OddsMultiplier: request.OddsMultiplier, Remark: request.Remark, Operator: "后台管理员"})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}
