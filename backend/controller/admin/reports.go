package admin

import (
	"backend/constants"
	"backend/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	reports   *services.FinancialReportService
	operating *services.OperatingReportService
	shares    *services.AgentProfitShareService
}

func NewReportHandler(db *gorm.DB) *ReportHandler {
	return &ReportHandler{
		reports:   services.NewFinancialReportService(db),
		operating: services.NewOperatingReportService(db),
		shares:    services.NewAgentProfitShareService(db),
	}
}

func (h *ReportHandler) Operating(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	result, err := h.operating.Report(services.OperatingReportFilter{
		Query: c.Query("query"), Start: c.Query("start"), End: c.Query("end"),
		RoomScope: c.Query("room_scope"), GameID: c.Query("game_id"), Dimension: c.DefaultQuery("dimension", "room"),
		UserID: userID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取经营报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ReportHandler) ProfitShares(c *gin.Context) {
	result, err := h.shares.Statement(c.Query("date"), 0)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "读取代理分账失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}

func (h *ReportHandler) RunProfitShares(c *gin.Context) {
	var request struct {
		Date string `json:"date"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		constants.SendError(c, http.StatusBadRequest, "分账日期不正确", err)
		return
	}
	operatorName := "系统"
	if operator, exists := c.Get("username"); exists {
		if value := strings.TrimSpace(fmt.Sprint(operator)); value != "" && value != "<nil>" {
			operatorName = value
		}
	}
	result, err := h.shares.Run(request.Date, operatorName)
	if err != nil {
		constants.SendError(c, http.StatusBadRequest, "执行代理分账失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "代理分账已完成", result)
}

func (h *ReportHandler) Financial(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.reports.Financial(services.FinancialReportFilter{
		Query: c.Query("query"), Type: c.DefaultQuery("type", "all"), Start: c.Query("start"), End: c.Query("end"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "读取财务报表失败", err)
		return
	}
	constants.SendSuccess(c, http.StatusOK, "ok", result)
}
