package user

import (
	"backend/captcha"
	"backend/constants"
	"backend/data/vo"
	apperrors "backend/errors"
	"backend/services"
	"backend/sessionauth"
	uploads "backend/uploadsecurity"
	"backend/ws"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MemberHandler interface {
	Login(c *gin.Context)
	Register(c *gin.Context)
	Logout(c *gin.Context)
	Refresh(c *gin.Context)
	Me(c *gin.Context)
	Games(c *gin.Context)
	JoinRoom(c *gin.Context)
	RoomHistory(c *gin.Context)
	ListBets(c *gin.Context)
	PlaceBet(c *gin.Context)
	CancelCurrentIssueBets(c *gin.Context)
	CancelBet(c *gin.Context)
	ListApplications(c *gin.Context)
	CreateApplication(c *gin.Context)
	BalanceHistory(c *gin.Context)
	WalletChannels(c *gin.Context)
	ListPaymentAccounts(c *gin.Context)
	CreatePaymentAccount(c *gin.Context)
	PaymentAccountQRCode(c *gin.Context)
	DeletePaymentAccount(c *gin.Context)
	RoomSettings(c *gin.Context)
	GameOdds(c *gin.Context)
	ListActivities(c *gin.Context)
	AssistantBet(c *gin.Context)
	WebBets(c *gin.Context)
	AssistantBetHistory(c *gin.Context)
	AssistantStatus(c *gin.Context)
	ActivityStatus(c *gin.Context)
	CheckIn(c *gin.Context)
	ClaimRedPacket(c *gin.Context)
	ListNotifications(c *gin.Context)
	NotificationUnread(c *gin.Context)
	MarkNotificationRead(c *gin.Context)
	MarkAllNotificationsRead(c *gin.Context)
	WalletSummary(c *gin.Context)
	RebatePreview(c *gin.Context)
	GameFeed(c *gin.Context)
	ChangePassword(c *gin.Context)
	UpdateNickname(c *gin.Context)
	UpdateAvatar(c *gin.Context)
	InviteInfo(c *gin.Context)
	ListEntertainment(c *gin.Context)
	LaunchEntertainment(c *gin.Context)
	ChatPreview(c *gin.Context)
	ListChatMessages(c *gin.Context)
	PostChatMessage(c *gin.Context)
	PostChatCommand(c *gin.Context)
	LatestClaimableChatRedPacket(c *gin.Context)
	ClaimChatRedPacket(c *gin.Context)
	ListPlans(c *gin.Context)
	PlanDetail(c *gin.Context)
	ActivatePlanStream(c *gin.Context)
}

type memberHandler struct {
	db              *gorm.DB
	auth            services.AuthService
	member          *services.MemberService
	bets            *services.BetAdminService
	apps            *services.ApplicationAdminService
	users           *services.UserAdminService
	wallet          *services.WalletAdminService
	paymentAccounts *services.MemberPaymentAccountService
	portal          *services.MemberPortalService
	assistant       memberBetAssistant
	entertainment   *services.EntertainmentAdminService
	chat            *services.MemberChatService
	games           *services.WorkspaceGameService
	plans           memberPlanService
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	// Length is validated by the service in UTF-8 bytes. Gin's min/max
	// validators count runes, which would reject otherwise valid passwords.
	NewPassword string `json:"new_password" binding:"required"`
}

type memberBetAssistant interface {
	Place(userID uint64, gameID, issue, content, operator, requestID string) (*services.AssistantBetResult, error)
	PlaceWeb(userID uint64, gameID, issue string, items []services.WebBetItem, operator, requestID string) (*services.AssistantBetResult, error)
	History(userID uint64, gameID string, limit int) ([]services.AssistantBetResult, error)
	DirectHistory(userID uint64, gameID string, limit int) ([]services.AssistantBetResult, error)
	StatusForUser(userID uint64, gameID string) (*services.AssistantDrawStatus, error)
}

func NewMemberHandler(db *gorm.DB) MemberHandler {
	return &memberHandler{
		db:              db,
		auth:            services.NewAuthService(db),
		member:          services.NewMemberService(db),
		bets:            services.NewBetAdminService(db),
		apps:            services.NewApplicationAdminService(db),
		users:           services.NewUserAdminService(db),
		wallet:          services.NewWalletAdminService(db),
		paymentAccounts: services.NewMemberPaymentAccountService(db),
		portal:          services.NewMemberPortalService(db),
		assistant:       services.NewBetAssistantService(db),
		entertainment:   services.NewEntertainmentAdminService(db),
		chat:            services.NewMemberChatService(db),
		games:           services.NewWorkspaceGameService(db),
		plans:           services.NewPlanContentService(db),
	}
}

func (h *memberHandler) ListPlans(c *gin.Context) {
	_, ok := memberUserID(c)
	if !ok {
		return
	}
	workspaceID, valid := c.Get("workspace_id")
	roomID, validID := workspaceID.(uint64)
	if !valid || !validID || roomID == 0 {
		constants.SendError(c, http.StatusForbidden, "请先进入房间", nil)
		return
	}
	result, err := h.plans.Catalog(roomID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取计划群失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) PlanDetail(c *gin.Context) {
	_, ok := memberUserID(c)
	if !ok {
		return
	}
	workspaceID, valid := c.Get("workspace_id")
	roomID, validID := workspaceID.(uint64)
	if !valid || !validID || roomID == 0 {
		constants.SendError(c, http.StatusForbidden, "请先进入房间", nil)
		return
	}
	limit, ok := memberPlanHistoryLimit(c)
	if !ok {
		return
	}
	if services.IsRacingPlanGame(c.Param("gameID")) {
		position, err := strconv.Atoi(c.DefaultQuery("position", "1"))
		if err != nil {
			constants.SendError(c, http.StatusBadRequest, "推荐位置不正确", err)
			return
		}
		result, err := h.plans.StreamDetailForGame(roomID, c.Param("gameID"), position, c.DefaultQuery("plan_key", services.DefaultPlanKey), limit)
		if err != nil {
			constants.SendError(c, http.StatusBadRequest, "读取彩票计划失败", err)
			return
		}
		// GET is metadata-only. Recommendation payloads are returned exclusively by
		// the activation POST after the immutable member-view receipt is committed.
		result.Recommendations = []services.PlanRecommendationView{}
		result.LatestRecommendations = []services.PlanRecommendationView{}
		result.History = []services.PlanRecommendationView{}
		result.LegacyHistory = []services.PlanRecommendationView{}
		constants.SendSuccess(c, http.StatusOK, "ok", result)
		return
	}
	result, err := h.plans.Detail(roomID, c.Param("gameID"), limit)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取彩票计划失败", err)
		return
	}
	result.Recommendations = []services.PlanRecommendationView{}
	result.LatestRecommendations = []services.PlanRecommendationView{}
	result.History = []services.PlanRecommendationView{}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) Games(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.games.ListEnabledForMember(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间游戏失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

// AssistantBet accepts the compact room syntax and delegates every financial
// operation to the server-side ticket parser. The browser never decides what
// gets deducted from the member's balance.
func (h *memberHandler) AssistantBet(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		Issue     string `json:"issue"`
		Content   string `json:"content" binding:"required"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "投注内容不正确", err)
		return
	}
	username, _ := c.Get("username")
	operator, _ := username.(string)
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	result, err := h.assistant.Place(userID, c.Param("id"), req.Issue, req.Content, operator, requestID)
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "投注未受理", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "投注助手暂时不可用", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "开奖助手已受理投注", result)
}

// WebBets is the typed detailed-board boundary used by web-only Bingo Mark Six
// and by PC28's web mode. The game comes from the path, the member from the
// session, and odds from server-side trading configuration; ticket items
// cannot override any of those fields.
func (h *memberHandler) WebBets(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		Issue     string                `json:"issue"`
		Items     []services.WebBetItem `json:"items" binding:"required"`
		RequestID string                `json:"request_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		constants.SendError(c, http.StatusBadRequest, "网投参数不正确", err)
		return
	}
	requestID := strings.TrimSpace(req.RequestID)
	username, _ := c.Get("username")
	operator, _ := username.(string)
	result, err := h.assistant.PlaceWeb(userID, c.Param("id"), req.Issue, req.Items, operator, requestID)
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "网投未受理", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "网投暂时不可用", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "网投已受理", result)
}

// AssistantBetHistory rebuilds the member's own room messages after a refresh.
// It never returns another member's requests and is scoped to the requested game.
func (h *memberHandler) AssistantBetHistory(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.assistant.DirectHistory(userID, c.Param("id"), limit)
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "读取投注消息失败", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "读取投注消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

// AssistantStatus supplies the assistant's current acceptance and result
// summary. Publishing a draw intentionally remains an administrator-only
// operation and is never exposed through this member route.
func (h *memberHandler) AssistantStatus(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.assistant.StatusForUser(userID, c.Param("id"))
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "读取开奖助手状态失败", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "读取开奖助手状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) Login(c *gin.Context) {
	noLoginCache(c)
	var req vo.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	if !verifyLoginCaptcha(c, captcha.Member, req.CaptchaID, req.CaptchaCode) {
		return
	}
	account, token, err := h.auth.LoginMember(req.Username, req.Password, req.Workspace)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, constants.ErrInvalidCredentials, err)
		return
	}
	writeSessionCookie(c, sessionauth.ScopeMember, token)
	constants.SendSuccess(c, http.StatusOK, constants.UserLoginSuccess, vo.LoginResponse{
		User: vo.UserResponse{
			ID: account.UserID, Username: account.Username, Email: account.Email,
			PublicID: account.PublicID, Nickname: account.Nickname, Avatar: account.Avatar,
			Title: account.PublicTitle, Badge: account.PublicBadge, Role: account.Role, Status: account.Status,
		},
	})
}

func (h *memberHandler) Register(c *gin.Context) {
	var req vo.MemberRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	result, err := h.member.RegisterWithToken(services.MemberRegisterInput{
		Username: req.Username, Password: req.Password,
		InviteCode: req.InviteCode, RoomCode: req.RoomCode,
	})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "注册失败", err)
		return
	}
	writeSessionCookie(c, sessionauth.ScopeMember, result.Token)
	result.Token = ""
	constants.SendSuccess(c, http.StatusCreated, "注册成功", result)
}

func (h *memberHandler) Logout(c *gin.Context) {
	if err := ws.RevokeRequestSession(h.db, c.Request); err != nil {
		clearSessionCookie(c, sessionauth.ScopeMember)
		constants.SendError(c, http.StatusInternalServerError, "退出登录失败，请稍后重试", err)
		return
	}
	clearSessionCookie(c, sessionauth.ScopeMember)
	constants.SendSuccess(c, http.StatusOK, "已退出登录", nil)
}

func (h *memberHandler) Refresh(c *gin.Context) {
	refreshSessionCookie(c, sessionauth.ScopeMember)
}

func (h *memberHandler) Me(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	profile, err := h.member.Profile(userID)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
		return
	}
	_ = h.portal.EnsureWelcomeNotification(userID)
	constants.SendSuccess(c, http.StatusOK, "ok", profile)
}

func (h *memberHandler) JoinRoom(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req vo.JoinRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	result, err := h.member.JoinRoom(userID, req.RoomCode, requestID)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "房间号无效或未开通", err)
		return
	}
	if result.Status == "pending" {
		constants.SendSuccess(c, http.StatusAccepted, "入房申请已提交，请等待审核", result)
		return
	}
	// Activating a room membership changes the account's authorization scope.
	// The database trigger therefore advances auth_version and invalidates the
	// request token. Reissue from the committed PostgreSQL value so the very
	// next member request does not fail with a misleading 401.
	account, err := h.auth.GetByID(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "刷新入房登录状态失败", err)
		return
	}
	if err := writeVersionedSessionCookie(c, sessionauth.ScopeMember, account.UserID, account.AuthVersion); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "刷新入房登录状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已进入房间", result)
}

func (h *memberHandler) RoomHistory(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "8"))
	result, err := h.member.RoomHistory(userID, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ListBets(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	result, err := h.bets.List(services.BetListFilter{
		UserID: userID, GameID: c.DefaultQuery("game_id", "all"),
		Issue: c.Query("issue"), Status: c.DefaultQuery("status", "all"),
		BeforeID: beforeID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) PlaceBet(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var request struct {
		GameID    string  `json:"game_id" binding:"required"`
		Issue     string  `json:"issue"`
		PlayCode  string  `json:"play_code"`
		PlayName  string  `json:"play_name"`
		Position  int     `json:"position" binding:"required"`
		Selection string  `json:"selection" binding:"required"`
		Amount    float64 `json:"amount" binding:"required"`
		Odds      float64 `json:"odds"`
		RequestID string  `json:"request_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "注单参数不正确", err)
		return
	}
	username, _ := c.Get("username")
	operatorName, _ := username.(string)
	result, err := h.bets.PlaceIdempotent(services.PlaceBetInput{
		GameID: request.GameID, Issue: request.Issue, UserID: userID,
		PlayCode: request.PlayCode, PlayName: request.PlayName, Position: request.Position,
		Selection: request.Selection, Amount: request.Amount, Odds: request.Odds,
		Operator: operatorName,
	}, request.RequestID)
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "创建注单失败", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "创建注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "注单已创建", result)
}

func (h *memberHandler) CancelBet(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "注单 ID 不正确", err)
		return
	}
	username, _ := c.Get("username")
	operatorName, _ := username.(string)
	result, err := h.bets.CancelOwned(id, userID, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "撤单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "注单已撤销", result)
}

// CancelCurrentIssueBets withdraws every still-pending ticket owned by the
// member in one server-authoritative issue. New clients include the confirmed
// issue so a delayed request cannot silently cancel a different period.
func (h *memberHandler) CancelCurrentIssueBets(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var request struct {
		GameID string `json:"game_id" binding:"required"`
		Issue  string `json:"issue" binding:"omitempty,max=64"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请选择要撤回的彩种", err)
		return
	}
	username, _ := c.Get("username")
	operatorName, _ := username.(string)
	result, err := h.bets.CancelCurrentIssue(userID, request.GameID, operatorName, request.Issue)
	if err != nil {
		if apperrors.IsBusinessError(err) {
			constants.SendError(c, http.StatusBadRequest, "本期注单未撤回", err)
			return
		}
		constants.SendError(c, http.StatusInternalServerError, "本期注单未撤回", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "本期注单已全部撤回", result)
}

func (h *memberHandler) ListApplications(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.apps.List(services.ApplicationFilter{
		UserID: userID, Status: c.DefaultQuery("status", "all"),
		RequestType: c.DefaultQuery("request_type", "all"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取申请记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) CreateApplication(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		RequestID        string  `json:"request_id" binding:"max=96"`
		RequestType      string  `json:"request_type" binding:"required"`
		PaymentType      string  `json:"payment_type"`
		PaymentAccountID uint64  `json:"payment_account_id"`
		Amount           float64 `json:"amount"`
		Remark           string  `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "申请参数不正确", err)
		return
	}
	if req.RequestType != "credit" && req.RequestType != "debit" {
		constants.SendError(c, http.StatusBadRequest, "仅支持上分或下分申请", nil)
		return
	}
	result, err := h.apps.Create(services.CreateApplicationInput{
		UserID: userID, RequestType: req.RequestType,
		RequestID:        req.RequestID,
		PaymentType:      defaultString(req.PaymentType, "manual"),
		PaymentAccountID: req.PaymentAccountID,
		Amount:           req.Amount, Remark: req.Remark,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "提交申请失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "申请已提交", result)
}

func (h *memberHandler) BalanceHistory(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	beforeID, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	result, err := h.users.BalanceHistoryPage(userID, limit, beforeID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取账变失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) WalletChannels(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	channels, err := h.wallet.ListForUser(userID, services.WalletListFilter{Status: "enabled"})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取收款方式失败", err)
		return
	}
	type memberChannel struct {
		ID         uint64  `json:"id"`
		Provider   string  `json:"provider"`
		Name       string  `json:"name"`
		CreditType string  `json:"credit_type"`
		MinAmount  float64 `json:"min_amount"`
		MaxAmount  float64 `json:"max_amount"`
		Remark     string  `json:"remark"`
	}
	out := make([]memberChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.Status != "enabled" {
			continue
		}
		out = append(out, memberChannel{
			ID: ch.ID, Provider: ch.Provider, Name: ch.Name,
			CreditType: ch.CreditType, MinAmount: ch.MinAmount, MaxAmount: ch.MaxAmount, Remark: ch.Remark,
		})
	}
	constants.SendSuccess(c, http.StatusOK, "ok", out)
}

func (h *memberHandler) ListPaymentAccounts(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	items, err := h.paymentAccounts.List(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", items)
}

func (h *memberHandler) CreatePaymentAccount(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	req, qrCode, err := bindMemberPaymentAccount(c)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "收款方式参数不正确", err)
		return
	}
	item, err := h.paymentAccounts.CreateWithQRCode(userID, req, qrCode)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "新增收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "收款方式已保存", item)
}

func bindMemberPaymentAccount(c *gin.Context) (services.CreateMemberPaymentAccountInput, *uploads.PaymentQRCode, error) {
	var request services.CreateMemberPaymentAccountInput
	mediaType, _, mediaTypeErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaTypeErr != nil || mediaType != "multipart/form-data" {
		if err := c.ShouldBindJSON(&request); err != nil {
			return request, nil, err
		}
		return request, nil, nil
	}
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		return request, nil, err
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
		for field, files := range c.Request.MultipartForm.File {
			if field != "qr_code" || len(files) > 1 {
				return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_QR_CODE", "二维码上传内容不正确")
			}
		}
	}
	isDefault := false
	if raw := strings.TrimSpace(c.PostForm("is_default")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "默认收款方式参数不正确")
		}
		isDefault = value
	}
	request = services.CreateMemberPaymentAccountInput{
		AccountType: c.PostForm("account_type"),
		Label:       c.PostForm("label"),
		AccountName: c.PostForm("account_name"),
		AccountNo:   c.PostForm("account_no"),
		HolderName:  c.PostForm("holder_name"),
		IsDefault:   isDefault,
	}
	file, err := c.FormFile("qr_code")
	if errors.Is(err, http.ErrMissingFile) {
		return request, nil, nil
	}
	if err != nil {
		return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_QR_CODE", "二维码图片读取失败")
	}
	if file.Size <= 0 || file.Size > uploads.MaxPaymentQRCodeBytes {
		return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_QR_CODE", "二维码图片需在 4MB 以内")
	}
	source, err := file.Open()
	if err != nil {
		return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_QR_CODE", "二维码图片读取失败")
	}
	defer source.Close()
	cleaned, err := uploads.SanitizePaymentQRCode(source)
	if err != nil {
		return request, nil, apperrors.NewBusinessError("INVALID_PAYMENT_QR_CODE", err.Error())
	}
	return request, cleaned, nil
}

func (h *memberHandler) PaymentAccountQRCode(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusNotFound, "收款二维码不存在", nil)
		return
	}
	qrCode, err := h.paymentAccounts.QRCode(userID, id)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "收款二维码不存在", err)
		return
	}
	defer qrCode.File.Close()
	streamPaymentQRCode(c, qrCode)
}

func streamPaymentQRCode(c *gin.Context, qrCode *services.MemberPaymentQRCode) {
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, qrCode.Size, "image/png", qrCode.File, nil)
}

func (h *memberHandler) DeletePaymentAccount(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		constants.SendError(c, http.StatusBadRequest, "收款方式 ID 不正确", err)
		return
	}
	if err := h.paymentAccounts.Delete(userID, id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除收款方式失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "收款方式已删除", nil)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (h *memberHandler) RoomSettings(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.RoomSettings(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间设置失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) GameOdds(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.GameOdds(userID, c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取赔率失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ListActivities(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.ListActivities(userID, c.Query("type"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取活动失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ActivityStatus(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	result, err := h.portal.ActivityStatus(userID, id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取活动状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) CheckIn(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	result, err := h.portal.CheckIn(userID, id)
	if err != nil {
		sendCheckInError(c, err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "签到成功", result)
}

func sendCheckInError(c *gin.Context, err error) {
	constants.SendError(c, http.StatusBadRequest, "签到失败", err)
}

func (h *memberHandler) ClaimRedPacket(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "活动 ID 不正确", err)
		return
	}
	result, err := h.portal.ClaimRedPacket(userID, id)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "领取失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "领取成功", result)
}

func (h *memberHandler) ClaimChatRedPacket(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	messageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || messageID == 0 {
		constants.SendError(c, http.StatusBadRequest, "红包消息不正确", err)
		return
	}
	result, err := h.chat.ClaimRedPacket(userID, messageID)
	if err != nil {
		status := http.StatusBadRequest
		if !apperrors.IsBusinessError(err) {
			status = http.StatusInternalServerError
		}
		constants.SendError(c, status, "领取失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "领取成功", result)
}

func (h *memberHandler) LatestClaimableChatRedPacket(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.chat.LatestClaimableRedPacket(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取房间红包失败", err)
		return
	}
	// A typed nil pointer keeps the standard success envelope stable while
	// encoding data as JSON null when the room has no claimable envelope.
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ListNotifications(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	result, err := h.portal.ListNotifications(
		userID, limit, beforeID, c.DefaultQuery("category", "all"),
		c.Query("game_id"), c.Query("issue"),
	)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取通知失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) NotificationUnread(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	count, err := h.portal.UnreadCount(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取未读数失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{"unread": count})
}

func (h *memberHandler) MarkNotificationRead(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "通知 ID 不正确", err)
		return
	}
	if err := h.portal.MarkNotificationRead(userID, id); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "标记已读失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", nil)
}

func (h *memberHandler) MarkAllNotificationsRead(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	if err := h.portal.MarkAllNotificationsRead(userID); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "标记已读失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", nil)
}

func (h *memberHandler) WalletSummary(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.WalletSummary(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取钱包统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) RebatePreview(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.RebatePreview(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取回水预览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) GameFeed(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.portal.GameFeed(userID, c.Param("id"), c.Query("issue"), limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取投注动态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ChangePassword(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "密码参数不正确", err)
		return
	}
	if err := h.member.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		sendChangePasswordError(c, err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "密码已更新", nil)
}

func sendChangePasswordError(c *gin.Context, err error) {
	constants.SendError(c, http.StatusBadRequest, "修改密码失败", err)
}

func (h *memberHandler) UpdateNickname(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "昵称参数不正确", err)
		return
	}
	profile, err := h.member.UpdateNickname(userID, req.Nickname)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "昵称修改失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "昵称已更新", profile)
}

func (h *memberHandler) UpdateAvatar(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		Avatar string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "头像参数不正确", err)
		return
	}
	profile, err := h.member.UpdateAvatar(userID, req.Avatar)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "头像修改失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "头像已更新", profile)
}

func (h *memberHandler) InviteInfo(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.portal.InviteInfo(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取邀请信息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ListEntertainment(c *gin.Context) {
	_, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.entertainment.ListForMember()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取娱乐平台失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) LaunchEntertainment(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	profile, err := h.member.Profile(userID)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
		return
	}
	result, err := h.entertainment.LaunchForMember(c.Param("code"), userID, profile.Username)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "无法进入娱乐平台", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ChatPreview(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	result, err := h.chat.Preview(userID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取聊天预览失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) ListChatMessages(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	beforeID, _ := strconv.ParseUint(c.Query("before_id"), 10, 64)
	afterID, _ := strconv.ParseUint(c.Query("after_id"), 10, 64)
	var since time.Time
	if raw := c.Query("since"); raw != "" {
		var err error
		since, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			constants.SendError(c, http.StatusBadRequest, "消息起始时间格式不正确", nil)
			return
		}
	}
	result, err := h.chat.List(userID, c.DefaultQuery("room_type", "group"), c.DefaultQuery("game_id", "lobby"), limit, beforeID, afterID, since)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取聊天消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) PostChatMessage(c *gin.Context) {
	h.postChatMessage(c, false)
}

// PostChatCommand is the rate-limited room-command boundary used by the text
// betting keyboard. Keeping it separate from ordinary chat prevents compact
// bets from bypassing the betting limiter and prevents chat-only mute/minimum
// balance rules from changing the outcome of the same bet placed by the
// structured panel.
func (h *memberHandler) PostChatCommand(c *gin.Context) {
	h.postChatMessage(c, true)
}

func (h *memberHandler) postChatMessage(c *gin.Context, commandRoute bool) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		RoomType  string `json:"room_type"`
		GameID    string `json:"game_id"`
		Content   string `json:"content" binding:"required"`
		Issue     string `json:"issue"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "消息参数不正确", err)
		return
	}
	if bingoMarkSixChatBetBlocked(req.GameID, req.Content) {
		constants.SendError(c, http.StatusBadRequest, "宾果六合彩仅支持网投，请使用投注面板", apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "宾果六合彩不支持聊天投注"))
		return
	}
	isCommand := isRoomCommandRequest(req.RoomType, req.GameID, req.Content)
	if commandRoute != isCommand {
		if commandRoute {
			constants.SendError(c, http.StatusBadRequest, "请输入有效的房间指令", nil)
		} else {
			constants.SendError(c, http.StatusBadRequest, "该内容请通过投注指令发送", nil)
		}
		return
	}
	var result *services.ChatMessageView
	var err error
	if commandRoute {
		requestID := strings.TrimSpace(req.RequestID)
		if len(requestID) < 8 || len(requestID) > 96 {
			constants.SendError(c, http.StatusBadRequest, "请求标识不正确", nil)
			return
		}
		result, err = h.chat.PostCommand(userID, req.RoomType, req.GameID, req.Content, requestID)
	} else {
		result, err = h.chat.Post(userID, req.RoomType, req.GameID, req.Content)
	}
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送消息失败", err)
		return
	}
	if commandRoute {
		requestID := strings.TrimSpace(req.RequestID)
		if command, matched := parseRoomApplicationCommand(req.Content); matched {
			applicationResult, createErr := h.apps.Create(services.CreateApplicationInput{
				RequestID: requestID + ":" + command.RequestType,
				UserID:    userID, RequestType: command.RequestType, PaymentType: "manual", AllowManualDebit: true,
				RoomScope: result.RoomScope, GameID: result.GameID, ChatMessageID: result.ID,
				Amount: command.Amount, Remark: "群聊申请 · " + strings.TrimSpace(req.Content),
			})
			assistantContent := "@" + result.Nickname + "\n[" + command.Label + " " + command.AmountText + "]申请没有提交成功，请稍后再试。"
			if createErr == nil {
				assistantContent = "@" + result.Nickname + "\n已收到[" + command.Label + " " + command.AmountText + "]申请，请稍等！"
				_, _ = h.chat.PostAssistant(result.RoomScope, result.GameID, assistantContent, applicationResult.ID)
			} else {
				_, _ = h.chat.PostAssistant(result.RoomScope, result.GameID, assistantContent, 0)
			}
		} else {
			h.handleRoomBetCommand(userID, result, strings.TrimSpace(req.Content), strings.TrimSpace(req.Issue), requestID)
		}
	}
	constants.SendSuccess(c, http.StatusCreated, "消息已发送", result)
}

// handleRoomBetCommand keeps the compact keyboard commands in the same
// persistent room timeline as ordinary messages. Members always see the text
// they sent first, followed by one durable assistant reply.
func (h *memberHandler) handleRoomBetCommand(userID uint64, message *services.ChatMessageView, content, requestedIssue, requestID string) {
	if message == nil || strings.TrimSpace(message.GameID) == "" || message.GameID == "lobby" {
		return
	}
	mention := "@" + message.Nickname + "\n"
	status, _ := h.assistant.StatusForUser(userID, message.GameID)
	gameName := message.GameID
	if status != nil && strings.TrimSpace(status.GameName) != "" {
		gameName = status.GameName
	}
	bettingIssue := roomCommandBettingIssue(status)

	post := func(body string) {
		_, _ = h.chat.PostAssistant(message.RoomScope, message.GameID, mention+strings.TrimSpace(body), message.ID)
	}
	switch content {
	case "取消":
		if requestedIssue != "" && bettingIssue != requestedIssue {
			post("撤单失败，期号已经切换，请核对最新一期后再操作。")
			return
		}
		expectedIssue := requestedIssue
		if expectedIssue == "" {
			expectedIssue = bettingIssue
		}
		if expectedIssue == "" {
			post("撤单失败，期号尚未同步，请稍后重试。")
			return
		}
		cancelled, err := h.bets.CancelCurrentIssue(userID, message.GameID, message.Nickname, expectedIssue)
		if err != nil {
			post("撤单失败，" + roomCommandError(err))
			return
		}
		post(fmt.Sprintf("【%s - %s】撤单成功\n已撤回 %d 注，退回 %.2f\n剩余：%.2f", gameName, cancelled.Issue, cancelled.Count, cancelled.Refund, cancelled.Balance))
		return
	case "查":
		issue := requestedIssue
		if issue == "" {
			issue = bettingIssue
		}
		if issue == "" {
			var err error
			issue, err = h.bets.BettingIssue(message.GameID)
			if err != nil {
				post("查询失败，请稍后再试。")
				return
			}
		}
		bets, err := h.bets.List(services.BetListFilter{UserID: userID, GameID: message.GameID, Issue: issue, Status: "pending", Page: 1, PageSize: 100})
		if err != nil {
			post("查询失败，请稍后再试。")
			return
		}
		profile, err := h.member.Profile(userID)
		if err != nil {
			post("查询失败，请稍后再试。")
			return
		}
		var body strings.Builder
		fmt.Fprintf(&body, "【%s - %s】\n", gameName, issue)
		var total float64
		for _, item := range bets.Items {
			label := services.BetDisplayLabel(item)
			fmt.Fprintf(&body, "%s [%s/%s]\n", label, item.Selection, services.FormatBetAmount(item.Amount))
			total += item.Amount
		}
		if len(bets.Items) == 0 {
			body.WriteString("本期暂无注单\n")
		}
		fmt.Fprintf(&body, "当期使用积分：%s\n剩余积分：%.2f", services.FormatBetAmount(total), profile.Balance)
		post(body.String())
		return
	case "重复":
		history, err := h.assistant.History(userID, message.GameID, 1)
		if err != nil || len(history) == 0 {
			post("暂无可以重复的上一笔投注。")
			return
		}
		repeatContent, err := services.AssistantRepeatContent(message.GameID, history[len(history)-1].Lines)
		if err != nil {
			post("重复投注失败，" + roomCommandError(err))
			return
		}
		accepted, err := h.assistant.Place(userID, message.GameID, requestedIssue, repeatContent, message.Nickname, requestID)
		if err != nil {
			if apperrors.GetErrorCode(err) == "REQUEST_IN_PROGRESS" {
				return
			}
			post("重复投注失败，" + roomCommandError(err))
			return
		}
		post(formatAssistantAccepted(accepted))
		return
	}

	if !isRoomBetContent(content) {
		return
	}
	accepted, err := h.assistant.Place(userID, message.GameID, requestedIssue, content, message.Nickname, requestID)
	if err != nil {
		// The original request is still committing. Do not persist a misleading
		// failure reply for a concurrent retry; the winning request will append
		// the one authoritative assistant receipt.
		if apperrors.GetErrorCode(err) == "REQUEST_IN_PROGRESS" {
			return
		}
		prefix := "投注失败，"
		if apperrors.GetErrorCode(err) == "INVALID_REQUEST" {
			prefix = "解析失败，"
		}
		post(prefix + roomCommandError(err))
		return
	}
	post(formatAssistantAccepted(accepted))
}

func roomCommandBettingIssue(status *services.AssistantDrawStatus) string {
	if status == nil {
		return ""
	}
	if status.BettingWindow != nil {
		return status.BettingWindow.Issue
	}
	return status.Issue
}

func formatAssistantAccepted(result *services.AssistantBetResult) string {
	if result == nil {
		return "投注没有受理，请稍后再试。"
	}
	var body strings.Builder
	fmt.Fprintf(&body, "【%s - %s】下单成功\n", result.GameName, result.Issue)
	for _, line := range services.AssistantReceiptLines(result.Lines) {
		body.WriteString(line)
		body.WriteByte('\n')
	}
	fmt.Fprintf(&body, "\n使用：%s\n剩余：%.2f", services.FormatBetAmount(result.Total), result.Balance)
	return body.String()
}

func roomCommandError(err error) string {
	if app, ok := err.(*apperrors.AppError); ok && strings.TrimSpace(app.Message) != "" {
		return strings.TrimSpace(app.Message)
	}
	return "暂时没有处理成功，请稍后再试。"
}

type roomApplicationCommand struct {
	RequestType string
	Label       string
	Amount      float64
	AmountText  string
}

var roomApplicationCommandPattern = regexp.MustCompile(`^(申请)?[[:space:]]*(上分|下分)[[:space:]]*[/：:]?[[:space:]]*([0-9]+(\.[0-9]{1,2})?)([[:space:]]+.*)?$`)
var roomIncompleteBetPattern = regexp.MustCompile(`^(买)?(冠军|亚军|第[三四五六七八九十]名|前三|中三|后三|前五|后五|冠亚(和)?)?[0-9大小单双龙虎和豹子顺对半杂六#[:space:],，.]*$`)
var roomBetSemanticPattern = regexp.MustCompile(`[0-9大小单双龙虎和冠亚军第名豹子顺对半杂六]`)

// Bare numbers/play fragments from the betting keyboard are commands too.
// They must reach the authoritative parser and receive a failure receipt when
// the amount is missing. Ordinary conversation remains on the chat boundary.
func isRoomBetContent(content string) bool {
	content = strings.TrimSpace(content)
	return content != "" && (strings.Contains(content, "/") || strings.Contains(content, "梭哈") || (roomBetSemanticPattern.MatchString(content) && roomIncompleteBetPattern.MatchString(content)))
}

func isRoomCommandRequest(roomType, gameID, content string) bool {
	if defaultString(strings.TrimSpace(roomType), "group") != "group" || strings.TrimSpace(gameID) == "" || strings.TrimSpace(gameID) == "lobby" {
		return false
	}
	content = strings.TrimSpace(content)
	if content == "取消" || content == "查" || content == "重复" {
		return true
	}
	if _, matched := parseRoomApplicationCommand(content); matched {
		return true
	}
	return isRoomBetContent(content)
}

func bingoMarkSixChatBetBlocked(gameID, content string) bool {
	if strings.TrimSpace(gameID) != "bingo-mark-six" {
		return false
	}
	content = strings.TrimSpace(content)
	return content == "重复" || isRoomBetContent(content)
}

func parseRoomApplicationCommand(content string) (roomApplicationCommand, bool) {
	matches := roomApplicationCommandPattern.FindStringSubmatch(strings.TrimSpace(content))
	if len(matches) < 4 {
		return roomApplicationCommand{}, false
	}
	amount, err := strconv.ParseFloat(matches[3], 64)
	if err != nil || amount <= 0 {
		return roomApplicationCommand{}, false
	}
	requestType := "credit"
	if matches[2] == "下分" {
		requestType = "debit"
	}
	return roomApplicationCommand{RequestType: requestType, Label: matches[2], Amount: amount, AmountText: matches[3]}, true
}

func memberUserID(c *gin.Context) (uint64, bool) {
	rawID, ok := c.Get("user_id")
	if !ok {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return 0, false
	}
	userID, ok := rawID.(uint64)
	if !ok || userID == 0 {
		constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
		return 0, false
	}
	return userID, true
}
