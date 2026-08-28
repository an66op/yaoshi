package services

import (
	"backend/data/models/user"
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
	WorkspaceID     uint64  `json:"workspace_id"`
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
	return s.ListForWorkspace(0, filter)
}

func (s *WalletAdminService) ListForUser(userID uint64, filter WalletListFilter) ([]PaymentChannelView, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return s.ListForWorkspace(account.WorkspaceID, filter)
}

func (s *WalletAdminService) ListForWorkspace(workspaceID uint64, filter WalletListFilter) ([]PaymentChannelView, error) {
	if workspaceID > 0 {
		if err := s.ensureDefaultsForWorkspace(workspaceID); err != nil {
			return nil, err
		}
	}
	query := s.db.Model(&wallet.PaymentChannel{}).Order("sort_order asc, id asc")
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
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

func (s *WalletAdminService) CreateForWorkspace(workspaceID uint64, input PaymentChannelPayload) (*PaymentChannelView, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择收款方式所属房间")
	}
	row, err := validatePaymentChannel(input)
	if err != nil {
		return nil, err
	}
	row.WorkspaceID = workspaceID
	if err := s.db.Create(row).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_CREATE_FAILED", "创建收款方式失败", err)
	}
	view := toPaymentChannelView(*row)
	return &view, nil
}

func (s *WalletAdminService) UpdateForWorkspace(workspaceID, id uint64, input PaymentChannelPayload) (*PaymentChannelView, error) {
	var row wallet.PaymentChannel
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("CHANNEL_NOT_FOUND", "收款方式不存在")
		}
		return nil, apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	next, err := validatePaymentChannel(input)
	if err != nil {
		return nil, err
	}
	row.Provider, row.Name, row.MerchantNo, row.CreditType = next.Provider, next.Name, next.MerchantNo, next.CreditType
	row.FeeRate, row.MinAmount, row.MaxAmount, row.Status = next.FeeRate, next.MinAmount, next.MaxAmount, next.Status
	row.Remark, row.SortOrder, row.Mode, row.APIBase = next.Remark, next.SortOrder, next.Mode, next.APIBase
	row.CreateOrderPath, row.QueryOrderPath, row.CallbackPath, row.TimeoutSeconds = next.CreateOrderPath, next.QueryOrderPath, next.CallbackPath, next.TimeoutSeconds
	if strings.TrimSpace(input.SecretKey) != "" {
		row.SecretKey = next.SecretKey
	}
	if err := s.db.Save(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("WALLET_UPDATE_FAILED", "更新收款方式失败", err)
	}
	view := toPaymentChannelView(row)
	return &view, nil
}

func (s *WalletAdminService) SetStatusForWorkspace(workspaceID, id uint64, status string) (*PaymentChannelView, error) {
	status = strings.TrimSpace(status)
	if status != "enabled" && status != "disabled" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "状态不正确")
	}
	var row wallet.PaymentChannel
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
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

func (s *WalletAdminService) DeleteForWorkspace(workspaceID, id uint64) error {
	result := s.db.Where("workspace_id = ?", workspaceID).Delete(&wallet.PaymentChannel{}, id)
	if result.Error != nil {
		return apperrors.NewSystemError("WALLET_DELETE_FAILED", "删除收款方式失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("CHANNEL_NOT_FOUND", "收款方式不存在")
	}
	return nil
}

func (s *WalletAdminService) platformWorkspaceID() (uint64, error) {
	var row struct{ ID uint64 }
	if err := s.db.Table("workspaces").Select("id").Where("type = ?", "platform").First(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *WalletAdminService) Create(input PaymentChannelPayload) (*PaymentChannelView, error) {
	workspaceID, err := s.platformWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.CreateForWorkspace(workspaceID, input)
}

func (s *WalletAdminService) Update(id uint64, input PaymentChannelPayload) (*PaymentChannelView, error) {
	workspaceID, err := s.platformWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.UpdateForWorkspace(workspaceID, id, input)
}

func (s *WalletAdminService) SetStatus(id uint64, status string) (*PaymentChannelView, error) {
	workspaceID, err := s.platformWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SetStatusForWorkspace(workspaceID, id, status)
}

func (s *WalletAdminService) Delete(id uint64) error {
	workspaceID, err := s.platformWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteForWorkspace(workspaceID, id)
}

// EnsureDefaultsForWorkspace materializes the room-owned payment catalog.
// Fresh installations therefore have a predictable manual application path
// before an operator opens the wallet page for the first time.
func (s *WalletAdminService) EnsureDefaultsForWorkspace(workspaceID uint64) error {
	if workspaceID == 0 {
		return apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择收款方式所属房间")
	}
	return s.ensureDefaultsForWorkspace(workspaceID)
}

func (s *WalletAdminService) ensureDefaultsForWorkspace(workspaceID uint64) error {
	var count int64
	if err := s.db.Model(&wallet.PaymentChannel{}).Where("workspace_id = ?", workspaceID).Count(&count).Error; err != nil {
		return apperrors.NewSystemError("WALLET_READ_FAILED", "读取钱包配置失败", err)
	}
	if count > 0 {
		return nil
	}
	defaults := []wallet.PaymentChannel{
		{Provider: "manual", Name: "人工处理", MerchantNo: "-", CreditType: "manual", FeeRate: 0, MinAmount: 1, MaxAmount: 100000, Status: "enabled", SortOrder: 0, Remark: "线下人工上下分", Mode: "manual", TimeoutSeconds: 10},
		{Provider: "bank_transfer", Name: "银行卡转账", CreditType: "bank", FeeRate: 0, MinAmount: 100, MaxAmount: 50000, Status: "disabled", SortOrder: 1, Remark: "配置真实收款资料后启用"},
		{Provider: "alipay", Name: "支付宝", CreditType: "alipay", FeeRate: 0, MinAmount: 10, MaxAmount: 20000, Status: "disabled", SortOrder: 2, Remark: "配置真实收款资料后启用"},
		{Provider: "wechat", Name: "微信支付", CreditType: "wechat", FeeRate: 0, MinAmount: 10, MaxAmount: 20000, Status: "disabled", SortOrder: 3, Remark: "配置真实收款资料后启用"},
		{Provider: "usdt", Name: "USDT-TRC20", MerchantNo: "USDT-TRC20", CreditType: "usdt", FeeRate: 0, MinAmount: 20, MaxAmount: 100000, Status: "disabled", SortOrder: 4, Remark: "默认停用，接入后启用"},
	}
	for index := range defaults {
		defaults[index].WorkspaceID = workspaceID
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
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Provider:    row.Provider,
		Name:        row.Name,
		MerchantNo:  row.MerchantNo,
		CreditType:  row.CreditType,
		FeeRate:     row.FeeRate,
		MinAmount:   row.MinAmount,
		MaxAmount:   row.MaxAmount,
		Status:      row.Status,
		Remark:      row.Remark,
		SortOrder:   row.SortOrder,
		Mode:        row.Mode, APIBase: row.APIBase, CreateOrderPath: row.CreateOrderPath, QueryOrderPath: row.QueryOrderPath, CallbackPath: row.CallbackPath, HasSecret: strings.TrimSpace(row.SecretKey) != "", TimeoutSeconds: row.TimeoutSeconds,
	}
}

func creditTypeKeys() []string {
	keys := make([]string, 0, len(allowedCreditTypes))
	for key := range allowedCreditTypes {
		keys = append(keys, key)
	}
	return keys
}
