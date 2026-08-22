package user

import (
	"backend/constants"
	"backend/data/models/bet"
	"backend/data/vo"
	"backend/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MemberHandler interface {
	Login(c *gin.Context)
	Register(c *gin.Context)
	Me(c *gin.Context)
	JoinRoom(c *gin.Context)
	ListBets(c *gin.Context)
	PlaceBet(c *gin.Context)
	CancelBet(c *gin.Context)
	ListApplications(c *gin.Context)
	CreateApplication(c *gin.Context)
	BalanceHistory(c *gin.Context)
	WalletChannels(c *gin.Context)
	RoomSettings(c *gin.Context)
	GameOdds(c *gin.Context)
	ListActivities(c *gin.Context)
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
	InviteInfo(c *gin.Context)
	ListEntertainment(c *gin.Context)
	LaunchEntertainment(c *gin.Context)
	ChatPreview(c *gin.Context)
	ListChatMessages(c *gin.Context)
	PostChatMessage(c *gin.Context)
}

type memberHandler struct {
	auth          services.AuthService
	member        *services.MemberService
	bets          *services.BetAdminService
	apps          *services.ApplicationAdminService
	users         *services.UserAdminService
	wallet        *services.WalletAdminService
	portal        *services.MemberPortalService
	entertainment *services.EntertainmentAdminService
	chat          *services.MemberChatService
	db            *gorm.DB
}

func NewMemberHandler(db *gorm.DB) MemberHandler {
	return &memberHandler{
		auth:          services.NewAuthService(db),
		member:        services.NewMemberService(db),
		bets:          services.NewBetAdminService(db),
		apps:          services.NewApplicationAdminService(db),
		users:         services.NewUserAdminService(db),
		wallet:        services.NewWalletAdminService(db),
		portal:        services.NewMemberPortalService(db),
		entertainment: services.NewEntertainmentAdminService(db),
		chat:          services.NewMemberChatService(db),
		db:            db,
	}
}

func (h *memberHandler) Login(c *gin.Context) {
	var req vo.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, constants.ErrInvalidRequestFormat, err)
		return
	}
	account, token, err := h.auth.LoginMember(req.Username, req.Password)
	if err != nil {
		constants.SendError(c, http.StatusUnauthorized, constants.ErrInvalidCredentials, err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, constants.UserLoginSuccess, vo.LoginResponse{
		Token: token,
		User: vo.UserResponse{
			ID: account.UserID, Username: account.Username, Email: account.Email,
			Nickname: account.Nickname, Role: account.Role, Status: account.Status,
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
		Username: req.Username, Password: req.Password, Nickname: req.Nickname,
		InviteCode: req.InviteCode, RoomCode: req.RoomCode,
	})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "注册失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "注册成功", result)
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
	result, err := h.member.JoinRoom(userID, req.RoomCode)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "房间号无效或未开通", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已进入房间", result)
}

func (h *memberHandler) ListBets(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.bets.List(services.BetListFilter{
		UserID: userID, GameID: c.DefaultQuery("game_id", "all"),
		Issue: c.Query("issue"), Status: c.DefaultQuery("status", "all"),
		Page: page, PageSize: pageSize,
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
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "注单参数不正确", err)
		return
	}
	username, _ := c.Get("username")
	operatorName, _ := username.(string)
	result, err := h.bets.Place(services.PlaceBetInput{
		GameID: request.GameID, Issue: request.Issue, UserID: userID,
		PlayCode: request.PlayCode, PlayName: request.PlayName, Position: request.Position,
		Selection: request.Selection, Amount: request.Amount, Odds: request.Odds,
		Operator: operatorName,
	})
	if err != nil {
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
	var row bet.Bet
	if err := h.db.Select("user_id").First(&row, id).Error; err != nil {
		constants.SendError(c, http.StatusNotFound, "注单不存在", err)
		return
	}
	if row.UserID != userID {
		constants.SendError(c, http.StatusForbidden, "无权操作此注单", nil)
		return
	}
	username, _ := c.Get("username")
	operatorName, _ := username.(string)
	result, err := h.bets.Cancel(id, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "撤单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "注单已撤销", result)
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
		RequestType string  `json:"request_type" binding:"required"`
		PaymentType string  `json:"payment_type"`
		Amount      float64 `json:"amount"`
		Remark      string  `json:"remark"`
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
		PaymentType: defaultString(req.PaymentType, "manual"),
		Amount: req.Amount, Remark: req.Remark,
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
	result, err := h.users.BalanceHistory(userID, limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取账变失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) WalletChannels(c *gin.Context) {
	channels, err := h.wallet.List(services.WalletListFilter{Status: "active"})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取收款方式失败", err)
		return
	}
	type memberChannel struct {
		ID        uint64  `json:"id"`
		Provider  string  `json:"provider"`
		Name      string  `json:"name"`
		CreditType string `json:"credit_type"`
		MinAmount float64 `json:"min_amount"`
		MaxAmount float64 `json:"max_amount"`
		Remark    string  `json:"remark"`
	}
	out := make([]memberChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.Status != "active" {
			continue
		}
		out = append(out, memberChannel{
			ID: ch.ID, Provider: ch.Provider, Name: ch.Name,
			CreditType: ch.CreditType, MinAmount: ch.MinAmount, MaxAmount: ch.MaxAmount, Remark: ch.Remark,
		})
	}
	constants.SendSuccess(c, http.StatusOK, "ok", out)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (h *memberHandler) RoomSettings(c *gin.Context) {
	result, err := h.portal.RoomSettings()
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
	result, err := h.portal.ListActivities(c.Query("type"))
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
		constants.SendError(c, http.StatusInternalServerError, "签到失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "签到成功", result)
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

func (h *memberHandler) ListNotifications(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.portal.ListNotifications(userID, limit)
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
	_, ok := memberUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.portal.GameFeed(c.Param("id"), c.Query("issue"), limit)
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
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "密码参数不正确", err)
		return
	}
	if err := h.member.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		constants.SendError(c, http.StatusBadRequest, "修改密码失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "密码已更新", nil)
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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	result, err := h.chat.List(userID, c.DefaultQuery("room_type", "group"), limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取聊天消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *memberHandler) PostChatMessage(c *gin.Context) {
	userID, ok := memberUserID(c)
	if !ok {
		return
	}
	var req struct {
		RoomType string `json:"room_type"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		constants.SendError(c, http.StatusBadRequest, "消息参数不正确", err)
		return
	}
	result, err := h.chat.Post(userID, req.RoomType, req.Content)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "发送消息失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "消息已发送", result)
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
