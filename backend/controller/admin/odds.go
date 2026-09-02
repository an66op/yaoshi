package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OddsHandler struct{ odds *services.OddsAdminService }

func NewOddsHandler(db *gorm.DB) *OddsHandler {
	return &OddsHandler{odds: services.NewOddsAdminService(db)}
}

func (h *OddsHandler) Get(c *gin.Context) {
	result, err := h.odds.Get(c.Param("id"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取赔率限额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *OddsHandler) Update(c *gin.Context) {
	var request services.UpdateOddsLimitsInput
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率限额参数不正确", err)
		return
	}
	result, err := h.odds.Update(c.Param("id"), request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "保存赔率限额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "赔率限额已保存", result)
}

func (h *OddsHandler) Catalog(c *gin.Context) {
	if gameID := c.Query("game_id"); gameID != "" {
		constants.SendSuccess(c, http.StatusOK, "ok", services.PlayCatalogForGame(gameID))
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", services.PlayCatalog())
}

func (h *OddsHandler) Reset(c *gin.Context) {
	var request services.OddsMutationGuard
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "赔率配置版本参数不正确", err)
		return
	}
	result, err := h.odds.Reset(c.Param("id"), request)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "清空赔率限额失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "已清空当前彩种赔率，全部玩法暂停受理", result)
}
