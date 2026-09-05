package drawfeed

import (
	"backend/constants"
	"backend/lotteryfeed"
	"backend/services"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler exposes the stable read-only API intended for the future standalone
// results website. Admin-specific actions stay outside this controller.
type Handler struct {
	lottery   *services.LotteryService
	scheduler *lotteryfeed.Scheduler
}

func NewHandler(db *gorm.DB, scheduler *lotteryfeed.Scheduler) *Handler {
	return &Handler{lottery: services.NewLotteryService(db), scheduler: scheduler}
}

func (h *Handler) Clock(c *gin.Context) {
	now := time.Now()
	c.Header("Cache-Control", "no-store")
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{
		"server_time":    now,
		"server_time_ms": now.UnixMilli(),
		"timezone":       "Asia/Shanghai",
	})
}

func (h *Handler) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	health, err := h.lottery.SettlementHealth(time.Now().UTC())
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取开奖结算健康状态失败", err)
		return
	}
	type statusResponse struct {
		lotteryfeed.Status
		Health services.SettlementHealthSummary `json:"health"`
	}
	constants.SendSuccess(c, http.StatusOK, "ok", statusResponse{Status: h.scheduler.Status(), Health: health})
}

func (h *Handler) Games(c *gin.Context) {
	games, err := h.lottery.ListGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取官方游戏失败", err)
		return
	}
	official := make([]services.GameSummary, 0, len(games))
	for _, game := range games {
		if (game.SourceKind == "official" || game.SourceKind == "external") && game.Enabled {
			official = append(official, game)
		}
	}
	c.Header("Cache-Control", "public, max-age=10")
	constants.SendSuccess(c, http.StatusOK, "ok", official)
}

// EnabledGames lists all enabled games for the user lobby (official + simulated).
func (h *Handler) EnabledGames(c *gin.Context) {
	games, err := h.lottery.ListGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取游戏列表失败", err)
		return
	}
	enabled := make([]services.GameSummary, 0, len(games))
	for _, game := range games {
		if game.Enabled && strings.TrimSpace(game.LobbyCategory) != "" {
			enabled = append(enabled, game)
		}
	}
	enabled, err = h.lottery.EnrichForLobby(enabled)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取游戏统计失败", err)
		return
	}
	c.Header("Cache-Control", "public, max-age=5")
	constants.SendSuccess(c, http.StatusOK, "ok", enabled)
}

func (h *Handler) Draws(c *gin.Context) {
	gameID := c.Param("id")
	if !h.isEnabledGame(gameID) {
		constants.SendError(c, http.StatusNotFound, "游戏不存在或未启用", nil)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	draws, err := h.lottery.ListDraws(c.Param("id"), limit)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取开奖记录失败", err)
		return
	}
	c.Header("Cache-Control", "public, max-age=5")
	constants.SendSuccess(c, http.StatusOK, "ok", draws)
}

func (h *Handler) Latest(c *gin.Context) {
	games, err := h.lottery.ListGames()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取官方游戏失败", err)
		return
	}
	type latestGame struct {
		Game services.GameSummary `json:"game"`
		Draw *services.DrawResult `json:"draw"`
	}
	items := make([]latestGame, 0, len(games))
	for _, game := range games {
		if (game.SourceKind != "official" && game.SourceKind != "external") || !game.Enabled {
			continue
		}
		draws, drawErr := h.lottery.ListDraws(game.ID, 1)
		if drawErr != nil {
			constants.SendError(c, http.StatusInternalServerError, "读取最新开奖失败", drawErr)
			return
		}
		item := latestGame{Game: game}
		if len(draws) > 0 {
			item.Draw = &draws[0]
		}
		items = append(items, item)
	}
	c.Header("Cache-Control", "public, max-age=5")
	constants.SendSuccess(c, http.StatusOK, "ok", gin.H{"server_time": time.Now(), "items": items})
}

func (h *Handler) isEnabledGame(gameID string) bool {
	games, err := h.lottery.ListGames()
	if err != nil {
		return false
	}
	for _, game := range games {
		if game.ID == gameID && game.Enabled {
			return true
		}
	}
	return false
}
