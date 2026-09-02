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
	"strings"
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
		{"member_count", platformMemberCountQuery(h.db, false)},
		{"active_member_count", platformMemberCountQuery(h.db, true)},
		{"agent_count", h.db.Model(&user.User{}).Where("role = ?", "agent")},
		{"active_agent_count", h.db.Model(&user.User{}).Where("role = ? AND status = ?", "agent", 1)},
		{"tenant_count", h.db.Model(&user.User{}).Where("role = ?", "tenant")},
		{"pending_application_count", platformPendingApplicationQuery(h.db)},
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

func platformPendingApplicationQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&application.Application{}).
		Where("status = ? AND request_type <> ?", "pending", "join")
}

func platformMemberCountQuery(db *gorm.DB, activeOnly bool) *gorm.DB {
	query := services.HumanMemberQuery(db)
	if activeOnly {
		query = query.Where(`"user".status = ?`, 1)
	}
	return query
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

// TestOfficialSource runs the same bounded, provider-locked import used by the
// scheduler, but only for one allowlisted source group. The route is mounted
// exclusively on the platform-admin surface.
func (h *DashboardHandler) TestOfficialSource(c *gin.Context) {
	group := strings.TrimSpace(c.Param("group"))
	if !services.IsOfficialSourceGroup(group) {
		constants.SendError(c, http.StatusBadRequest, "未知官方数据源线路", nil)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Second)
	defer cancel()
	results := h.lottery.SyncOfficialGroup(ctx, group)
	failed := 0
	for _, result := range results {
		if result.Status != "ok" {
			failed++
		}
	}
	message := "数据源线路测试完成"
	if failed > 0 {
		message = "数据源线路测试发现异常"
	}
	constants.SendSuccess(c, http.StatusOK, message, gin.H{"group": group, "results": results, "failed": failed})
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

func (h *DashboardHandler) GameCategories(c *gin.Context) {
	categories, err := h.lottery.ListLobbyCategories()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取分类失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", categories)
}

func (h *DashboardHandler) CreateGameCategory(c *gin.Context) {
	h.saveGameCategory(c, 0)
}

func (h *DashboardHandler) UpdateGameCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "分类编号不正确", err)
		return
	}
	h.saveGameCategory(c, id)
}

func (h *DashboardHandler) saveGameCategory(c *gin.Context, id uint64) {
	var request struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请求参数不正确", err)
		return
	}
	category, err := h.lottery.SaveLobbyCategory(id, request.Name, request.SortOrder)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "保存分类失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "分类已保存", category)
}

func (h *DashboardHandler) DeleteGameCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "分类编号不正确", err)
		return
	}
	if err := h.lottery.DeleteLobbyCategory(id); err != nil {
		constants.SendError(c, http.StatusBadRequest, "删除分类失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "分类已删除，原有彩种已转入未分类", gin.H{"id": id})
}

func (h *DashboardHandler) AssignGameCategory(c *gin.Context) {
	var request struct {
		Category  string `json:"category"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请求参数不正确", err)
		return
	}
	game, err := h.lottery.AssignLobbyCategory(c.Param("id"), request.Category, request.SortOrder)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "彩种归类失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "彩种分类已更新", game)
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
		constants.SendError(c, http.StatusBadRequest, "游戏状态更新失败", err)
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
