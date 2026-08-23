package services

import (
	"backend/data/models/application"
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplicationAdminService struct{ db *gorm.DB }

type AdminApplication struct {
	ID                  uint64     `json:"id"`
	UserID              uint64     `json:"user_id"`
	Username            string     `json:"username"`
	AccountType         string     `json:"account_type"`
	RequestType         string     `json:"request_type"`
	PaymentType         string     `json:"payment_type"`
	PaymentAccountID    uint64     `json:"payment_account_id"`
	PaymentAccountLabel string     `json:"payment_account_label"`
	RequestedAmount     float64    `json:"requested_amount"`
	ReceivedAmount      float64    `json:"received_amount"`
	Remark              string     `json:"remark"`
	Status              string     `json:"status"`
	Operator            string     `json:"operator"`
	ReviewRemark        string     `json:"review_remark"`
	ReviewedAt          *time.Time `json:"reviewed_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type ApplicationFilter struct {
	Query, Status, RequestType, Date string
	UserID                           uint64
	Page, PageSize                   int
}

type ApplicationList struct {
	Items    []AdminApplication `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type ApplicationStats struct {
	Pending       int64   `json:"pending"`
	ApprovedToday int64   `json:"approved_today"`
	RejectedToday int64   `json:"rejected_today"`
	TodayAmount   float64 `json:"today_amount"`
}

type CreateApplicationInput struct {
	UserID           uint64
	RequestType      string
	PaymentType      string
	PaymentAccountID uint64
	Amount           float64
	Remark           string
}

type ReviewApplicationInput struct {
	Decision       string
	ReceivedAmount float64
	Remark         string
	Operator       string
}

func NewApplicationAdminService(db *gorm.DB) *ApplicationAdminService {
	return &ApplicationAdminService{db: db}
}

func (s *ApplicationAdminService) List(filter ApplicationFilter) (*ApplicationList, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&application.Application{})
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("LOWER(username) LIKE ? OR LOWER(remark) LIKE ? OR LOWER(review_remark) LIKE ?", like, like, like)
	}
	if filter.Status != "" && filter.Status != "all" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.RequestType != "" && filter.RequestType != "all" {
		query = query.Where("request_type = ?", filter.RequestType)
	}
	if filter.Date != "" {
		if day, err := time.ParseInLocation("2006-01-02", filter.Date, time.Local); err == nil {
			query = query.Where("created_at >= ? AND created_at < ?", day, day.AddDate(0, 0, 1))
		}
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []application.Application
	if err := query.Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, created_at desc").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]AdminApplication, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminApplication(row))
	}
	return &ApplicationList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *ApplicationAdminService) Stats() (*ApplicationStats, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	stats := &ApplicationStats{}
	if err := s.db.Model(&application.Application{}).Where("status = 'pending'").Count(&stats.Pending).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&application.Application{}).Where("status = 'approved' AND reviewed_at >= ?", start).Count(&stats.ApprovedToday).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&application.Application{}).Where("status = 'rejected' AND reviewed_at >= ?", start).Count(&stats.RejectedToday).Error; err != nil {
		return nil, err
	}
	var cents int64
	if err := s.db.Model(&application.Application{}).Select("COALESCE(SUM(requested_cents), 0)").Where("created_at >= ? AND request_type IN ('credit','debit')", start).Scan(&cents).Error; err != nil {
		return nil, err
	}
	stats.TodayAmount = centsToAmount(cents)
	return stats, nil
}

func (s *ApplicationAdminService) Get(id uint64) (*AdminApplication, error) {
	var row application.Application
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	result := adminApplication(row)
	return &result, nil
}

func (s *ApplicationAdminService) Create(input CreateApplicationInput) (*AdminApplication, error) {
	requestType, err := validRequestType(input.RequestType)
	if err != nil {
		return nil, err
	}
	amountCents := int64(math.Round(input.Amount * 100))
	if (requestType == "credit" || requestType == "debit") && amountCents <= 0 {
		return nil, apperrors.NewBusinessError("INVALID_AMOUNT", "上下分申请金额必须大于 0")
	}
	if math.Abs(input.Amount) > 100000000 {
		return nil, apperrors.NewBusinessError("INVALID_AMOUNT", "申请金额超出限制")
	}
	if requestType == "agent" || requestType == "join" {
		amountCents = 0
	}
	var account user.User
	if err := s.db.First(&account, input.UserID).Error; err != nil {
		return nil, err
	}
	if account.Status != 1 {
		return nil, apperrors.NewBusinessError("USER_DISABLED", "停用用户不能创建申请")
	}
	payment := strings.TrimSpace(input.PaymentType)
	if payment == "" {
		payment = "manual"
	}
	paymentAccountID := uint64(0)
	paymentAccountLabel := ""
	if requestType == "debit" {
		paymentAccount, err := NewMemberPaymentAccountService(s.db).GetOwned(account.UserID, input.PaymentAccountID)
		if err != nil {
			return nil, err
		}
		payment = paymentAccount.AccountType
		paymentAccountID = paymentAccount.ID
		paymentAccountLabel = paymentAccount.Label + " · " + maskPaymentAccountNo(paymentAccount.AccountNo)
	}
	row := application.Application{UserID: account.UserID, Username: account.Username, AccountType: defaultString(account.Role, "member"), RequestType: requestType, PaymentType: payment, PaymentAccountID: paymentAccountID, PaymentAccountLabel: paymentAccountLabel, RequestedCents: amountCents, Remark: strings.TrimSpace(input.Remark), Status: "pending"}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	result := adminApplication(row)
	return &result, nil
}

func (s *ApplicationAdminService) Review(id uint64, input ReviewApplicationInput) (*AdminApplication, error) {
	if input.Decision != "approved" && input.Decision != "rejected" {
		return nil, apperrors.NewBusinessError("INVALID_DECISION", "审核结果不正确")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var item application.Application
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, id).Error; err != nil {
			return err
		}
		if item.Status != "pending" {
			return apperrors.NewBusinessError("ALREADY_REVIEWED", "该申请已经审核，不能重复操作")
		}
		now := time.Now().UTC()
		receivedCents := int64(math.Round(input.ReceivedAmount * 100))
		if input.Decision == "rejected" {
			receivedCents = 0
		}
		if math.Abs(input.ReceivedAmount) > 100000000 {
			return apperrors.NewBusinessError("INVALID_AMOUNT", "到账金额超出限制")
		}
		if input.Decision == "approved" && (item.RequestType == "credit" || item.RequestType == "debit") {
			var account user.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, item.UserID).Error; err != nil {
				return err
			}
			change := item.RequestedCents
			if item.RequestType == "credit" {
				if receivedCents <= 0 {
					receivedCents = item.RequestedCents
				}
				change = receivedCents
			} else {
				change = -item.RequestedCents
				if account.BalanceCents+change < 0 {
					return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "用户余额不足，不能通过下分申请")
				}
				if receivedCents <= 0 {
					receivedCents = item.RequestedCents
				}
			}
			after := account.BalanceCents + change
			if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
				return err
			}
			record := user.BalanceTransaction{UserID: account.UserID, AmountCents: change, BeforeCents: account.BalanceCents, AfterCents: after, Type: "application_" + item.RequestType, Remark: "申请 #" + formatUint(item.ID) + " 审核通过", Operator: defaultString(input.Operator, "后台管理员")}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		if input.Decision == "approved" && item.RequestType == "agent" {
			if err := tx.Model(&user.User{}).Where("user_id = ?", item.UserID).Update("role", "agent").Error; err != nil {
				return err
			}
		}
		return tx.Model(&item).Updates(map[string]any{"status": input.Decision, "received_cents": receivedCents, "operator": defaultString(input.Operator, "后台管理员"), "review_remark": strings.TrimSpace(input.Remark), "reviewed_at": now}).Error
	})
	if err != nil {
		return nil, err
	}
	result, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	notifyMemberApplication(s.db, &applicationReviewNotify{
		UserID:   result.UserID,
		Decision: input.Decision,
		Remark:   applicationReviewMessage(*result),
	})
	return result, nil
}

func applicationReviewMessage(app AdminApplication) string {
	typeLabel := map[string]string{"credit": "上分", "debit": "下分", "agent": "代理", "join": "入房"}
	label := typeLabel[app.RequestType]
	if label == "" {
		label = "申请"
	}
	if app.Status == "approved" {
		if app.RequestType == "credit" || app.RequestType == "debit" {
			return fmt.Sprintf("%s申请已通过，金额 %.2f 元", label, app.ReceivedAmount)
		}
		return fmt.Sprintf("%s申请已通过", label)
	}
	if strings.TrimSpace(app.ReviewRemark) != "" {
		return fmt.Sprintf("%s申请未通过：%s", label, app.ReviewRemark)
	}
	return fmt.Sprintf("%s申请未通过", label)
}

func validRequestType(value string) (string, error) {
	switch value {
	case "credit", "debit", "agent", "join":
		return value, nil
	default:
		return "", apperrors.NewBusinessError("INVALID_REQUEST_TYPE", "申请类型不正确")
	}
}

func adminApplication(row application.Application) AdminApplication {
	return AdminApplication{ID: row.ID, UserID: row.UserID, Username: row.Username, AccountType: row.AccountType, RequestType: row.RequestType, PaymentType: row.PaymentType, PaymentAccountID: row.PaymentAccountID, PaymentAccountLabel: row.PaymentAccountLabel, RequestedAmount: centsToAmount(row.RequestedCents), ReceivedAmount: centsToAmount(row.ReceivedCents), Remark: row.Remark, Status: row.Status, Operator: row.Operator, ReviewRemark: row.ReviewRemark, ReviewedAt: row.ReviewedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func formatUint(value uint64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append(buf, digits[value%10])
		value /= 10
	}
	for left, right := 0, len(buf)-1; left < right; left, right = left+1, right-1 {
		buf[left], buf[right] = buf[right], buf[left]
	}
	return string(buf)
}
