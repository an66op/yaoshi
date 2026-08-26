package user

import (
	"backend/constants"
	"backend/data/vo"
	"backend/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthHandler 定义接口（专门处理登录/注册相关）
type AuthHandler interface {
	Register(c *gin.Context)
	Login(c *gin.Context)
	Me(c *gin.Context)
}

type authHandler struct {
	authService services.AuthService
}

func NewAuthHandler(db *gorm.DB) AuthHandler {
	return &authHandler{authService: services.NewAuthService(db)}
}

func (h *authHandler) Register(c *gin.Context) {
	var req vo.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	account, err := h.authService.Register(&req)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, constants.UserCreateFailed, err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, constants.UserCreateSuccess, vo.RegisterResponse{
		ID: account.UserID, PublicID: account.PublicID, Username: account.Username, Email: account.Email, Nickname: account.Nickname, Status: account.Status,
	})
}

func (h *authHandler) Login(c *gin.Context) {
	var req vo.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	account, token, err := h.authService.Login(req.Username, req.Password, req.Workspace)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, constants.ErrInvalidCredentials, err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, constants.UserLoginSuccess, vo.LoginResponse{
		Token: token,
		User: vo.UserResponse{
			ID: account.UserID, Username: account.Username, Email: account.Email,
			PublicID: account.PublicID, Nickname: account.Nickname, Role: account.Role, Status: account.Status,
		},
	})
}

func (h *authHandler) Me(c *gin.Context) {
	rawID, ok := c.Get("user_id")
	if !ok {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	userID, ok := rawID.(uint64)
	if !ok {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return
	}
	account, err := h.authService.GetByID(userID)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
		return
	}
	if (account.Role != "admin" && account.Role != "tenant" && account.Role != "agent") || account.Status != 1 {
		constants.SendError(c, http.StatusForbidden, "需要管理员、租户或房间代理权限", nil)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", vo.UserResponse{
		ID: account.UserID, Username: account.Username, Email: account.Email,
		PublicID: account.PublicID, Nickname: account.Nickname, Role: account.Role, Status: account.Status,
	})
}
