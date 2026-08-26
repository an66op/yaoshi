package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BetHandler struct {
	bets *services.BetAdminService
}

func NewBetHandler(db *gorm.DB) *BetHandler {
	return &BetHandler{bets: services.NewBetAdminService(db)}
}

func (h *BetHandler) Monitor(c *gin.Context) {
	result, err := h.bets.Monitor(c.Query("game_id"), c.Query("issue"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取现场监控失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *BetHandler) Place(c *gin.Context) {
	var request struct {
		GameID    string   `json:"game_id" binding:"required"`
		Issue     string   `json:"issue"`
		UserID    uint64   `json:"user_id" binding:"required"`
		PlayCode  string   `json:"play_code"`
		PlayName  string   `json:"play_name"`
		Position  int      `json:"position" binding:"required"`
		Selection string   `json:"selection" binding:"required"`
		Amount    float64  `json:"amount" binding:"required"`
		Odds      float64  `json:"odds"`
		FlyAmount *float64 `json:"fly_amount"`
		Remark    string   `json:"remark"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "注单参数不正确", err)
		return
	}
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.bets.Place(services.PlaceBetInput{
		GameID: request.GameID, Issue: request.Issue, UserID: request.UserID,
		PlayCode: request.PlayCode, PlayName: request.PlayName, Position: request.Position,
		Selection: request.Selection, Amount: request.Amount, Odds: request.Odds,
		FlyAmount: request.FlyAmount, Remark: request.Remark, Operator: operatorName,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "创建注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusCreated, "注单已创建", result)
}

func (h *BetHandler) Settle(c *gin.Context) {
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.bets.SettleIssue(c.Param("id"), c.Param("issue"), operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "开奖结算失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "开奖结算完成", result)
}

func (h *BetHandler) PublishDraw(c *gin.Context) {
	var request struct {
		Issue   string `json:"issue"`
		Numbers []int  `json:"numbers"`
	}
	_ = c.ShouldBindJSON(&request)
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.bets.PublishDraw(c.Param("id"), request.Issue, request.Numbers, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "开奖并结算失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "开奖并结算完成", result)
}

func (h *BetHandler) SettlementStatus(c *gin.Context) {
	result, err := h.bets.SettlementStatus(c.Param("id"), c.Param("issue"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取结算状态失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *BetHandler) BoardReport(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.bets.BoardReport(services.BoardReportFilter{
		GameID: c.DefaultQuery("game_id", "all"), Query: c.Query("query"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取打盘报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *BetHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := h.bets.List(services.BetListFilter{
		Query: c.Query("query"), GameID: c.DefaultQuery("game_id", "all"), Issue: c.Query("issue"),
		UserID: userID, Status: c.DefaultQuery("status", "all"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *BetHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "注单 ID 不正确", err)
		return
	}
	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	result, err := h.bets.Cancel(id, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "撤单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "注单已撤销", result)
}
