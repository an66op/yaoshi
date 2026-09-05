package user

import (
	"backend/accesscontrol"
	"backend/captcha"
	"backend/constants"
	"backend/data/vo"
	"backend/services"
	"backend/sessionauth"
	"backend/ws"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthHandler 定义接口（专门处理登录/注册相关）
type AuthHandler interface {
	Register(c *gin.Context)
	Login(c *gin.Context)
	Me(c *gin.Context)
	Logout(c *gin.Context)
	Refresh(c *gin.Context)
}

type authHandler struct {
	authService services.AuthService
	db          *gorm.DB
}

func NewAuthHandler(db *gorm.DB) AuthHandler {
	return &authHandler{authService: services.NewAuthService(db), db: db}
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
	noLoginCache(c)
	var req vo.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	if !verifyLoginCaptcha(c, captcha.Management, req.CaptchaID, req.CaptchaCode) {
		return
	}
	account, token, err := h.authService.Login(req.Username, req.Password, req.Workspace, req.Role)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, constants.ErrInvalidCredentials, err)
		return
	}
	writeSessionCookie(c, sessionauth.ScopeManagement, token)
	constants.SendSuccess(c, http.StatusOK, constants.UserLoginSuccess, vo.LoginResponse{
		User: vo.UserResponse{
			ID: account.UserID, Username: account.Username, Email: account.Email,
			PublicID: account.PublicID, Nickname: account.Nickname, Avatar: account.Avatar,
			Title: account.PublicTitle, Badge: account.PublicBadge, Role: account.Role, Status: account.Status,
		},
	})
}

func (h *authHandler) Logout(c *gin.Context) {
	if err := ws.RevokeRequestSession(h.db, c.Request); err != nil {
		clearSessionCookie(c, sessionauth.ScopeManagement)
		constants.SendError(c, http.StatusInternalServerError, "退出登录失败，请稍后重试", err)
		return
	}
	clearSessionCookie(c, sessionauth.ScopeManagement)
	constants.SendSuccess(c, http.StatusOK, "已退出登录", nil)
}

func (h *authHandler) Refresh(c *gin.Context) {
	if _, ok := h.managementAccount(c); !ok {
		return
	}
	refreshSessionCookie(c, sessionauth.ScopeManagement)
}

func (h *authHandler) managementAccount(c *gin.Context) (*vo.UserResponse, bool) {
	rawID, idOK := c.Get("user_id")
	rawVersion, versionOK := c.Get("auth_version")
	userID, validID := rawID.(uint64)
	claimVersion, validVersion := rawVersion.(uint64)
	if !idOK || !versionOK || !validID || !validVersion || userID == 0 || claimVersion == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return nil, false
	}
	account, err := h.authService.GetByID(userID)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
		return nil, false
	}
	if account.AuthVersion == 0 || account.AuthVersion != claimVersion {
		constants.SendError(c, http.StatusUnauthorized, "登录已失效，请重新登录", nil)
		return nil, false
	}
	if account.Role != "admin" && account.Role != "tenant" && account.Role != "agent" {
		constants.SendError(c, http.StatusForbidden, "需要管理员、租户或房间代理权限", nil)
		return nil, false
	}
	if account.Role != "admin" && account.WorkspaceID == 0 {
		constants.SendError(c, http.StatusForbidden, "账号尚未绑定房间工作区", nil)
		return nil, false
	}
	if account.Role == "agent" {
		active, hierarchyErr := accesscontrol.AgentHierarchyActive(h.db, *account)
		if hierarchyErr != nil {
			constants.SendError(c, http.StatusInternalServerError, "读取代理权限失败", hierarchyErr)
			return nil, false
		}
		if !active {
			constants.SendError(c, http.StatusForbidden, "所属租户已停用，代理工作台不可用", nil)
			return nil, false
		}
	}
	return &vo.UserResponse{
		ID: account.UserID, Username: account.Username, Email: account.Email,
		PublicID: account.PublicID, Nickname: account.Nickname, Avatar: account.Avatar,
		Title: account.PublicTitle, Badge: account.PublicBadge, Role: account.Role, Status: account.Status,
	}, true
}

func (h *authHandler) Me(c *gin.Context) {
	account, ok := h.managementAccount(c)
	if !ok {
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", account)
}
