package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserAdminHandler struct {
	users    *services.UserAdminService
	trading  *services.TradingAdminService
}

func NewUserAdminHandler(db *gorm.DB) *UserAdminHandler {
	return &UserAdminHandler{
		users:   services.NewUserAdminService(db),
		trading: services.NewTradingAdminService(db),
	}
}

func (h *UserAdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.users.List(services.UserListFilter{Query: c.Query("query"), Status: c.DefaultQuery("status", "all"), Role: c.DefaultQuery("role", "all"), Page: page, PageSize: pageSize})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取用户列表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *UserAdminHandler) Stats(c *gin.Context) {
	result, err := h.users.Stats()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取用户统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *UserAdminHandler) Get(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	result, err := h.users.Get(id)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "用户不存在", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *UserAdminHandler) Create(c *gin.Context) {
	var request struct {
		Username  string `json:"username" binding:"required,min=3,max=50"`
		Password  string `json:"password" binding:"required,min=6,max=72"`
		Email     string `json:"email" binding:"omitempty,email"`
		Nickname  string `json:"nickname" binding:"max=50"`
		Phone     string `json:"phone" binding:"max=30"`
		Role      string `json:"role"`
		Remark    string `json:"remark" binding:"max=500"`
		RiskLevel string `json:"risk_level"`
		Status    int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户资料不正确", err)
		return
	}
	result, err := h.users.Create(services.CreateAdminUserInput{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, Role: request.Role, Remark: request.Remark, RiskLevel: request.RiskLevel, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建用户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "用户创建成功", result)
}

func (h *UserAdminHandler) Update(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Email     string `json:"email" binding:"omitempty,email"`
		Nickname  string `json:"nickname" binding:"max=50"`
		Phone     string `json:"phone" binding:"max=30"`
		Role      string `json:"role"`
		Remark    string `json:"remark" binding:"max=500"`
		RiskLevel string `json:"risk_level"`
		Status    int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户资料不正确", err)
		return
	}
	result, err := h.users.Update(id, services.UpdateAdminUserInput{Email: request.Email, Nickname: request.Nickname, Phone: request.Phone, Role: request.Role, Remark: request.Remark, RiskLevel: request.RiskLevel, Status: request.Status})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "更新用户失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户资料已更新", result)
}

func (h *UserAdminHandler) SetStatus(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Status int `json:"status" binding:"oneof=0 1"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户状态不正确", err)
		return
	}
	result, err := h.users.SetStatus(id, request.Status)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "用户不存在", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "用户状态已更新", result)
}

func (h *UserAdminHandler) ResetPassword(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Password string `json:"password" binding:"required,min=6,max=72"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "新密码至少需要 6 个字符", err)
		return
	}
	if err := h.users.ResetPassword(id, request.Password); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "重置密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "密码已重置", gin.H{"id": id})
}

func (h *UserAdminHandler) AdjustBalance(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request struct {
		Amount float64 `json:"amount" binding:"required"`
		Remark string  `json:"remark" binding:"required,max=300"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "余额调整资料不正确", err)
		return
	}
	result, err := h.users.AdjustBalance(id, request.Amount, request.Remark, "后台管理员")
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "调整余额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "余额调整成功", result)
}

func (h *UserAdminHandler) BalanceHistory(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.users.BalanceHistory(id, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取余额流水失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *UserAdminHandler) GetTrading(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	result, err := h.trading.Get(id, c.Query("game_id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取飞单与赔率配置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *UserAdminHandler) UpdateTrading(c *gin.Context) {
	id, err := services.ParseUserID(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "用户编号不正确", err)
		return
	}
	var request services.UpdateUserTradingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "飞单与赔率参数不正确", err)
		return
	}
	result, err := h.trading.Update(id, request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存飞单与赔率配置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "飞单与单独赔率已保存", result)
}
