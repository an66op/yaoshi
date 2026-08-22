package admin

import (
	"backend/constants"
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	reports *services.FinancialReportService
}

func NewReportHandler(db *gorm.DB) *ReportHandler {
	return &ReportHandler{reports: services.NewFinancialReportService(db)}
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
