package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ApplicationAdminHandler struct {
	applications *services.ApplicationAdminService
}

func NewApplicationAdminHandler(db *gorm.DB) *ApplicationAdminHandler {
	return &ApplicationAdminHandler{applications: services.NewApplicationAdminService(db)}
}

func (h *ApplicationAdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.applications.List(services.ApplicationFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), RequestType: c.DefaultQuery("type", "all"), Date: c.Query("date"), Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取申请列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ApplicationAdminHandler) Stats(c *gin.Context) {
	result, err := h.applications.Stats()
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
	result, err := h.applications.Create(services.CreateApplicationInput{UserID: request.UserID, RequestType: request.RequestType, PaymentType: request.PaymentType, GameID: request.GameID, Amount: request.Amount, Remark: request.Remark})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "申请已创建", result)
}

func (h *ApplicationAdminHandler) Review(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请编号不正确", err)
		return
	}
	var request struct {
		Decision       string  `json:"decision" binding:"required,oneof=approved rejected"`
		ReceivedAmount float64 `json:"received_amount"`
		Remark         string  `json:"remark" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "审核资料不正确", err)
		return
	}
	result, err := h.applications.Review(id, services.ReviewApplicationInput{Decision: request.Decision, ReceivedAmount: request.ReceivedAmount, Remark: request.Remark, Operator: "后台管理员"})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "审核申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "申请审核完成", result)
}
