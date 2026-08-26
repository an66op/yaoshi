package admin

import (
	"backend/constants"
	"backend/data/models/application"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/user"
	"backend/services"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db      *gorm.DB
	lottery *services.LotteryService
	bets    *services.BetAdminService
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{
		db:      db,
		lottery: services.NewLotteryService(db),
		bets:    services.NewBetAdminService(db),
	}
}

func (h *DashboardHandler) overview() (gin.H, error) {
	counts := gin.H{}
	queries := []struct {
		key   string
		query *gorm.DB
	}{
		{"member_count", h.db.Model(&user.User{}).Where("role = ?", "member")},
		{"active_member_count", h.db.Model(&user.User{}).Where("role = ? AND status = ?", "member", 1)},
		{"agent_count", h.db.Model(&user.User{}).Where("role = ?", "agent")},
		{"active_agent_count", h.db.Model(&user.User{}).Where("role = ? AND status = ?", "agent", 1)},
		{"tenant_count", h.db.Model(&user.User{}).Where("role = ?", "tenant")},
		{"pending_application_count", h.db.Model(&application.Application{}).Where("status = ?", "pending")},
		{"service_conversation_count", h.db.Model(&chat.Message{}).Where("room_type = ? AND deleted_at IS NULL", "service").Distinct("scope")},
		{"source_error_count", h.db.Model(&lottery.Game{}).Where("enabled = ? AND sync_status = ?", true, "error")},
	}
	for _, item := range queries {
		var count int64
		if err := item.query.Count(&count).Error; err != nil {
			return nil, err
		}
		counts[item.key] = count
	}
	grouped := h.db.Model(&chat.Message{}).
		Select("room_scope, game_id").
		Where("room_type = ? AND deleted_at IS NULL", "group").
		Group("room_scope, game_id")
	var groupCount int64
	if err := h.db.Table("(?) AS grouped_chat", grouped).Count(&groupCount).Error; err != nil {
		return nil, err
	}
	counts["group_conversation_count"] = groupCount
	return counts, nil
}

func (h *DashboardHandler) SyncOfficialSources(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 75*time.Second)
	defer cancel()
	results := h.lottery.SyncOfficialSources(ctx)
	failed := 0
	for _, result := range results {
		if result.Status != "ok" {
			failed++
		}
	}
	message := "官方开奖数据同步完成"
	if failed > 0 {
		message = "部分官方数据源同步失败"
	}
	constants.SendSuccess(c, http.StatusOK, message, gin.H{"results": results, "failed": failed})
}

func (h *DashboardHandler) Dashboard(c *gin.Context) {
	games, err := h.lottery.ListGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取仪表盘失败", err)
		return
	}
	money, err := h.bets.GameMoneyMap()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取经营数据失败", err)
		return
	}
	for i := range games {
		if item, ok := money[games[i].ID]; ok {
			games[i].Turnover = item.Turnover
			games[i].Profit = item.Profit
		}
	}
	stats, err := h.bets.DashboardStats()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取经营统计失败", err)
		return
	}
	overview, err := h.overview()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取综合统计失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{
		"overview": overview,
		"stats": gin.H{
			"user_balance":           stats.UserBalance,
			"today_turnover":         stats.TodayTurnover,
			"today_settled_turnover": stats.TodaySettledTurnover,
			"today_gross_profit":     stats.TodayGrossProfit,
			"today_net_profit":       stats.TodayNetProfit,
			"today_rebate":           stats.TodayRebate,
			"today_welfare":          stats.TodayWelfare,
			"today_agent_share":      stats.TodayAgentShare,
			"total_gross_profit":     stats.TotalGrossProfit,
			"total_net_profit":       stats.TotalNetProfit,
			"total_rebate":           stats.TotalRebate,
			"total_welfare":          stats.TotalWelfare,
			"total_agent_share":      stats.TotalAgentShare,
			"today_profit":           stats.TodayProfit,
			"total_profit":           stats.TotalProfit,
			"pending_settlement":     stats.PendingSettlement,
		},
		"games": games,
	})
}

func (h *DashboardHandler) Games(c *gin.Context) {
	games, err := h.lottery.ListGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取游戏列表失败", err)
		return
	}
	money, _ := h.bets.GameMoneyMap()
	for i := range games {
		if item, ok := money[games[i].ID]; ok {
			games[i].Turnover = item.Turnover
			games[i].Profit = item.Profit
		}
	}
	constants.SendSuccess(c, http.StatusOK, "ok", games)
}

func (h *DashboardHandler) Draws(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	draws, err := h.lottery.ListDraws(c.Param("id"), limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取开奖记录失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", draws)
}

func (h *DashboardHandler) UpdateGameStatus(c *gin.Context) {
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请求参数不正确", err)
		return
	}
	game, err := h.lottery.SetEnabled(c.Param("id"), request.Enabled)
	if err != nil {
		constants.SendError(c, http.StatusNotFound, "游戏不存在", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "游戏状态已更新", game)
}

func (h *DashboardHandler) SyncTargetGames(c *gin.Context) {
	result, err := h.lottery.SyncTargetGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "补全目标彩种失败", err)
		return
	}
	message := "目标彩种已齐全"
	if len(result.Created) > 0 {
		message = fmt.Sprintf("已补全 %d 个目标彩种", len(result.Created))
	}
	constants.SendSuccess(c, http.StatusOK, message, result)
}
