package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BetHandler struct {
	bets *services.BetAdminService
	bots *services.TestBotService
}

func NewBetHandler(db *gorm.DB) *BetHandler {
	handler := &BetHandler{bets: services.NewBetAdminService(db), bots: services.NewTestBotService(db)}
	if os.Getenv("BACKEND_TEST_BOTS") == "1" {
		services.StartTestBotsFromEnvironment(handler.bots)
	}
	return handler
}

func (h *BetHandler) TestBotStatus(c *gin.Context) {
	constants.SendSuccess(c, http.StatusOK, "ok", h.bots.Status())
}

func (h *BetHandler) StartTestBots(c *gin.Context) {
	var request struct {
		IntervalSecs int `json:"interval_secs"`
	}
	_ = c.ShouldBindJSON(&request)
	status, err := h.bots.Start(time.Duration(request.IntervalSecs) * time.Second)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "启动测试机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "测试机器人已启动", status)
}

func (h *BetHandler) StopTestBots(c *gin.Context) {
	constants.SendSuccess(c, http.StatusOK, "测试机器人已停止", h.bots.Stop())
}

func (h *BetHandler) RunTestBotsOnce(c *gin.Context) {
	status, err := h.bots.RunOnce()
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "执行测试机器人失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "测试机器人已完成一轮投注", status)
}

func (h *BetHandler) Monitor(c *gin.Context) {
	result, err := h.bets.Monitor(c.Query("game_id"), c.Query("issue"))
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取现场监控失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *BetHandler) SeedMonitor(c *gin.Context) {
	var request struct {
		GameID string `json:"game_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "请选择游戏", err)
		return
	}
	result, err := h.bets.SeedDemo(request.GameID)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "生成演示注单失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "演示注单已生成", result)
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
