package services

import (
	"backend/data/models/wallet"
	apperrors "backend/errors"
	"backend/utils"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type WalletAdminService struct{ db *gorm.DB }

type PaymentChannelView struct {
	ID              uint64  `json:"id"`
	Provider        string  `json:"provider"`
	Name            string  `json:"name"`
	MerchantNo      string  `json:"merchant_no"`
	CreditType      string  `json:"credit_type"`
	FeeRate         float64 `json:"fee_rate"`
	MinAmount       float64 `json:"min_amount"`
	MaxAmount       float64 `json:"max_amount"`
	Status          string  `json:"status"`
	Remark          string  `json:"remark"`
	SortOrder       int     `json:"sort_order"`
	Mode            string  `json:"mode"`
	APIBase         string  `json:"api_base"`
	CreateOrderPath string  `json:"create_order_path"`
	QueryOrderPath  string  `json:"query_order_path"`
	CallbackPath    string  `json:"callback_path"`
	HasSecret       bool    `json:"has_secret"`
	TimeoutSeconds  int     `json:"timeout_seconds"`
}

type WalletListFilter struct {
	Query  string
	Status string
}

type PaymentChannelPayload struct {
	Provider        string  `json:"provider"`
	Name            string  `json:"name"`
	MerchantNo      string  `json:"merchant_no"`
	CreditType      string  `json:"credit_type"`
	FeeRate         float64 `json:"fee_rate"`
	MinAmount       float64 `json:"min_amount"`
	MaxAmount       float64 `json:"max_amount"`
	Status          string  `json:"status"`
	Remark          string  `json:"remark"`
	SortOrder       int     `json:"sort_order"`
	Mode            string  `json:"mode"`
	APIBase         string  `json:"api_base"`
	CreateOrderPath string  `json:"create_order_path"`
	QueryOrderPath  string  `json:"query_order_path"`
	CallbackPath    string  `json:"callback_path"`
	SecretKey       string  `json:"secret_key"`
	TimeoutSeconds  int     `json:"timeout_seconds"`
}

var allowedCreditTypes = map[string]string{
	"manual": "人工处理",
	"bank":   "银行卡",
	"alipay": "支付宝",
	"wechat": "微信",
	"usdt":   "USDT",
}

func NewWalletAdminService(db *gorm.DB) *WalletAdminService {
	return &WalletAdminService{db: db}
}

func (s *WalletAdminService) List(filter WalletListFilter) ([]PaymentChannelView, error) {
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	query := s.db.Model(&wallet.PaymentChannel{}).Order("sort_order asc, id asc")
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		query = query.Where("name ILIKE ? OR provider ILIKE ? OR merchant_no ILIKE ? OR credit_type ILIKE ? OR remark ILIKE ?", like, like, like, like, like)
	}
	switch strings.TrimSpace(filter.Status) {
	case "", "all":
	case "enabled", "disabled":
		query = query.Where("status = ?", filter.Status)
	default:
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "状态筛选不正确")
	}
	var rows []wallet.PaymentChannel
	if err := query.Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	items := make([]PaymentChannelView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPaymentChannelView(row))
	}
	return items, nil
}

func (s *WalletAdminService) Create(input PaymentChannelPayload) (*PaymentChannelView, error) {
	row, err := validatePaymentChannel(input)
	if err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_CREATE_FAILED", "创建收款方式失败", err)
	}
	view := toPaymentChannelView(*row)
	return &view, nil
}

func (s *WalletAdminService) Update(id uint64, input PaymentChannelPayload) (*PaymentChannelView, error) {
	var row wallet.PaymentChannel
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("CHANNEL_NOT_FOUND", "收款方式不存在")
		}
		return nil, apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	next, err := validatePaymentChannel(input)
	if err != nil {
		return nil, err
	}
	row.Provider = next.Provider
	row.Name = next.Name
	row.MerchantNo = next.MerchantNo
	row.CreditType = next.CreditType
	row.FeeRate = next.FeeRate
	row.MinAmount = next.MinAmount
	row.MaxAmount = next.MaxAmount
	row.Status = next.Status
	row.Remark = next.Remark
	row.SortOrder = next.SortOrder
	row.Mode = next.Mode
	row.APIBase = next.APIBase
	row.CreateOrderPath = next.CreateOrderPath
	row.QueryOrderPath = next.QueryOrderPath
	row.CallbackPath = next.CallbackPath
	row.TimeoutSeconds = next.TimeoutSeconds
	if strings.TrimSpace(input.SecretKey) != "" {
		row.SecretKey = next.SecretKey
	}
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_UPDATE_FAILED", "更新收款方式失败", err)
	}
	view := toPaymentChannelView(row)
	return &view, nil
}

func (s *WalletAdminService) SetStatus(id uint64, status string) (*PaymentChannelView, error) {
	status = strings.TrimSpace(status)
	if status != "enabled" && status != "disabled" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "状态不正确")
	}
	var row wallet.PaymentChannel
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("CHANNEL_NOT_FOUND", "收款方式不存在")
		}
		return nil, apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	row.Status = status
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_UPDATE_FAILED", "更新收款方式失败", err)
	}
	view := toPaymentChannelView(row)
	return &view, nil
}

func (s *WalletAdminService) Delete(id uint64) error {
	result := s.db.Delete(&wallet.PaymentChannel{}, id)
	if result.Error != nil {
		return apperrors.NewSystemError("WALLET_DELETE_FAILED", "删除收款方式失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("CHANNEL_NOT_FOUND", "收款方式不存在")
	}
	return nil
}

func (s *WalletAdminService) ensureDefaults() error {
	var count int64
	if err := s.db.Model(&wallet.PaymentChannel{}).Count(&count).Error; err != nil {
		return apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	if count > 0 {
		return nil
	}
	defaults := []wallet.PaymentChannel{
		{Provider: "manual", Name: "人工处理", MerchantNo: "-", CreditType: "manual", FeeRate: 0, MinAmount: 1, MaxAmount: 100000, Status: "enabled", SortOrder: 0, Remark: "线下人工上下分", Mode: "manual", TimeoutSeconds: 10},
		{Provider: "bank_transfer", Name: "银行卡转账", MerchantNo: "BANK-001", CreditType: "bank", FeeRate: 0, MinAmount: 100, MaxAmount: 50000, Status: "enabled", SortOrder: 1, Remark: "对公银行卡收款"},
		{Provider: "alipay", Name: "支付宝", MerchantNo: "ALI-001", CreditType: "alipay", FeeRate: 0.6, MinAmount: 10, MaxAmount: 20000, Status: "enabled", SortOrder: 2},
		{Provider: "wechat", Name: "微信支付", MerchantNo: "WX-001", CreditType: "wechat", FeeRate: 0.6, MinAmount: 10, MaxAmount: 20000, Status: "enabled", SortOrder: 3},
		{Provider: "usdt", Name: "USDT-TRC20", MerchantNo: "USDT-TRC20", CreditType: "usdt", FeeRate: 0, MinAmount: 20, MaxAmount: 100000, Status: "disabled", SortOrder: 4, Remark: "默认停用，接入后启用"},
	}
	return s.db.Create(&defaults).Error
}

func validatePaymentChannel(input PaymentChannelPayload) (*wallet.PaymentChannel, error) {
	provider := strings.TrimSpace(input.Provider)
	name := strings.TrimSpace(input.Name)
	creditType := strings.TrimSpace(input.CreditType)
	status := strings.TrimSpace(input.Status)
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "gateway" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "接入模式不正确")
	}
	if provider == "" || name == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "支付接口和支付名称不能为空")
	}
	if _, ok := allowedCreditTypes[creditType]; !ok {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("充值种类不正确，可选: %s", strings.Join(creditTypeKeys(), " / ")))
	}
	if status == "" {
		status = "enabled"
	}
	if status != "enabled" && status != "disabled" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "状态不正确")
	}
	if input.FeeRate < 0 || input.FeeRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "手续费率需在 0-100 之间")
	}
	if input.MinAmount < 0 || input.MaxAmount < 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "金额不能为负数")
	}
	if input.MaxAmount > 0 && input.MinAmount > input.MaxAmount {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "最小金额不能高于最大金额")
	}
	apiBase := strings.TrimSpace(input.APIBase)
	createPath := strings.TrimSpace(input.CreateOrderPath)
	if mode == "gateway" && (apiBase == "" || createPath == "") {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "API 网关必须填写接口地址和创建订单路径")
	}
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = 10
	}
	if timeout < 2 || timeout > 60 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "接口超时需在 2–60 秒之间")
	}
	encryptedSecret, err := utils.EncryptSensitive(strings.TrimSpace(input.SecretKey))
	if err != nil {
		return nil, apperrors.NewSystemError("WALLET_SECRET_SAVE_FAILED", "保存支付密钥失败", err)
	}
	return &wallet.PaymentChannel{
		Provider:   provider,
		Name:       name,
		MerchantNo: strings.TrimSpace(input.MerchantNo),
		CreditType: creditType,
		FeeRate:    input.FeeRate,
		MinAmount:  input.MinAmount,
		MaxAmount:  input.MaxAmount,
		Status:     status,
		Remark:     strings.TrimSpace(input.Remark),
		SortOrder:  input.SortOrder,
		Mode:       mode, APIBase: apiBase, CreateOrderPath: createPath, QueryOrderPath: strings.TrimSpace(input.QueryOrderPath), CallbackPath: strings.TrimSpace(input.CallbackPath), SecretKey: encryptedSecret, TimeoutSeconds: timeout,
	}, nil
}

func toPaymentChannelView(row wallet.PaymentChannel) PaymentChannelView {
	return PaymentChannelView{
		ID:         row.ID,
		Provider:   row.Provider,
		Name:       row.Name,
		MerchantNo: row.MerchantNo,
		CreditType: row.CreditType,
		FeeRate:    row.FeeRate,
		MinAmount:  row.MinAmount,
		MaxAmount:  row.MaxAmount,
		Status:     row.Status,
		Remark:     row.Remark,
		SortOrder:  row.SortOrder,
		Mode:       row.Mode, APIBase: row.APIBase, CreateOrderPath: row.CreateOrderPath, QueryOrderPath: row.QueryOrderPath, CallbackPath: row.CallbackPath, HasSecret: strings.TrimSpace(row.SecretKey) != "", TimeoutSeconds: row.TimeoutSeconds,
	}
}

func creditTypeKeys() []string {
	keys := make([]string, 0, len(allowedCreditTypes))
	for key := range allowedCreditTypes {
		keys = append(keys, key)
	}
	return keys
}
