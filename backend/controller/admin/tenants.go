package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TenantHandler struct{ tenants *services.TenantAdminService }

func NewTenantHandler(db *gorm.DB) *TenantHandler {
	return &TenantHandler{tenants: services.NewTenantAdminService(db)}
}

func (h *TenantHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.tenants.List(c.Query("query"), page, pageSize)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取租户列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *TenantHandler) Create(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Password string `json:"password" binding:"required,min=8,max=72"`
		Email    string `json:"email" binding:"omitempty,email"`
		Nickname string `json:"nickname" binding:"max=50"`
		Phone    string `json:"phone" binding:"max=30"`
		Remark   string `json:"remark" binding:"max=500"`
		Status   int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户资料不正确", err)
		return
	}
	result, err := h.tenants.Create(services.TenantPayload{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, Remark: request.Remark, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "创建租户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "租户账号已创建", result)
}

func (h *TenantHandler) Update(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户编号不正确", err)
		return
	}
	var request struct {
		Email    string `json:"email" binding:"omitempty,email"`
		Nickname string `json:"nickname" binding:"max=50"`
		Phone    string `json:"phone" binding:"max=30"`
		Remark   string `json:"remark" binding:"max=500"`
		Status   int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户资料不正确", err)
		return
	}
	result, err := h.tenants.Update(id, services.TenantPayload{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, Remark: request.Remark, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存租户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户资料已保存", result)
}

func (h *TenantHandler) ResetPassword(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "租户编号不正确", err)
		return
	}
	var request struct {
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "新密码长度需为 8–72 个字符", err)
		return
	}
	if err := h.tenants.ResetPassword(id, request.Password); err != nil {
		constants.SendError(c, http.StatusBadRequest, "重置密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "租户登录密码已重置", gin.H{"id": id})
}
