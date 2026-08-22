package services

import (
	"backend/data/models/entertainment"
	apperrors "backend/errors"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type EntertainmentAdminService struct{ db *gorm.DB }

type PlatformView struct {
	ID         uint64 `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	MerchantNo string `json:"merchant_no"`
	APIBase    string `json:"api_base"`
	LaunchPath string `json:"launch_path"`
	SecretKey  string `json:"secret_key,omitempty"`
	Status     string `json:"status"`
	Remark     string `json:"remark"`
	SortOrder  int    `json:"sort_order"`
}

type PlatformPayload struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	MerchantNo string `json:"merchant_no"`
	APIBase    string `json:"api_base"`
	LaunchPath string `json:"launch_path"`
	SecretKey  string `json:"secret_key"`
	Status     string `json:"status"`
	Remark     string `json:"remark"`
	SortOrder  int    `json:"sort_order"`
}

func NewEntertainmentAdminService(db *gorm.DB) *EntertainmentAdminService {
	return &EntertainmentAdminService{db: db}
}

func (s *EntertainmentAdminService) List() ([]PlatformView, error) {
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	var rows []entertainment.Platform
	if err := s.db.Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("ENTERTAINMENT_READ_FAILED", "读取娱乐平台失败", err)
	}
	items := make([]PlatformView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPlatformView(row))
	}
	return items, nil
}

func (s *EntertainmentAdminService) ListForMember() ([]MemberPlatformView, error) {
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	var rows []entertainment.Platform
	if err := s.db.Where("status IN ?", []string{"enabled", "maintenance"}).
		Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("ENTERTAINMENT_READ_FAILED", "读取娱乐平台失败", err)
	}
	items := make([]MemberPlatformView, 0, len(rows))
	for _, row := range rows {
		items = append(items, MemberPlatformView{
			ID: row.ID, Code: row.Code, Name: row.Name,
			Category: row.Category, Status: row.Status, Remark: row.Remark,
		})
	}
	return items, nil
}

type MemberPlatformView struct {
	ID       uint64 `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Remark   string `json:"remark"`
}

type EntertainmentLaunchView struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Ready     bool   `json:"ready"`
	LaunchURL string `json:"launch_url,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

func (s *EntertainmentAdminService) LaunchForMember(code string, userID uint64, username string) (*EntertainmentLaunchView, error) {
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "平台编号不能为空")
	}
	var row entertainment.Platform
	if err := s.db.Where("code = ?", code).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "娱乐平台不存在")
		}
		return nil, err
	}
	view := &EntertainmentLaunchView{Code: row.Code, Name: row.Name, Status: row.Status}
	switch row.Status {
	case "enabled":
		launchURL, expiresAt, err := s.buildLaunchURL(row, username)
		if err != nil {
			return nil, err
		}
		view.Ready = true
		view.LaunchURL = launchURL
		view.ExpiresAt = expiresAt
		view.Message = fmt.Sprintf("%s 已开通，正在为您跳转…", row.Name)
	case "maintenance":
		view.Message = defaultString(row.Remark, fmt.Sprintf("%s 维护中，请稍后再试。", row.Name))
	default:
		view.Message = fmt.Sprintf("%s 暂未开放。", row.Name)
	}
	_ = userID
	return view, nil
}

func (s *EntertainmentAdminService) VerifyLaunchToken(code, username, ts, token string) (*entertainment.Platform, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	var row entertainment.Platform
	if err := s.db.Where("code = ?", code).First(&row).Error; err != nil {
		return nil, apperrors.NewBusinessError("NOT_FOUND", "娱乐平台不存在")
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	if err != nil || time.Now().Unix()-sec > 600 || sec > time.Now().Unix()+120 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "链接已过期")
	}
	if !strings.EqualFold(signLaunch(row.SecretKey, row.MerchantNo, username, row.Code, sec), strings.TrimSpace(token)) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "签名无效")
	}
	return &row, nil
}

func (s *EntertainmentAdminService) buildLaunchURL(row entertainment.Platform, username string) (string, int64, error) {
	ts := time.Now().Unix()
	token := signLaunch(row.SecretKey, row.MerchantNo, username, row.Code, ts)
	base := strings.TrimRight(strings.TrimSpace(row.APIBase), "/")
	if base == "" {
		base = strings.TrimRight(publicEntertainmentBase(), "/")
	}
	path := strings.TrimSpace(row.LaunchPath)
	if path == "" {
		path = "/portal"
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		launchURL := replaceLaunchPlaceholders(path, row, username, token, ts)
		return launchURL, ts + 600, nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	query := url.Values{}
	query.Set("code", row.Code)
	query.Set("user", username)
	query.Set("merchant", row.MerchantNo)
	query.Set("ts", strconv.FormatInt(ts, 10))
	query.Set("token", token)
	return base + path + "?" + query.Encode(), ts + 600, nil
}

func replaceLaunchPlaceholders(raw string, row entertainment.Platform, username, token string, ts int64) string {
	replacer := strings.NewReplacer(
		"{code}", row.Code,
		"{merchant}", row.MerchantNo,
		"{username}", username,
		"{user}", username,
		"{token}", token,
		"{ts}", strconv.FormatInt(ts, 10),
	)
	return replacer.Replace(raw)
}

func signLaunch(secret, merchant, username, code string, ts int64) string {
	secret = defaultString(strings.TrimSpace(secret), "demo")
	payload := fmt.Sprintf("%s|%s|%s|%s|%d", merchant, username, code, secret, ts)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func publicEntertainmentBase() string {
	if base := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")); base != "" {
		return strings.TrimRight(base, "/") + "/api/public/entertainment"
	}
	return "http://127.0.0.1:8080/api/public/entertainment"
}

func (s *EntertainmentAdminService) Upsert(input PlatformPayload) (*PlatformView, error) {
	code := strings.ToLower(strings.TrimSpace(input.Code))
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "平台编号和名称不能为空")
	}
	status := defaultString(strings.TrimSpace(input.Status), "disabled")
	if status != "enabled" && status != "maintenance" && status != "disabled" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "平台状态不正确")
	}
	var row entertainment.Platform
	err := s.db.Where("code = ?", code).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		row = entertainment.Platform{
			Code: code, Name: name, Category: defaultString(strings.TrimSpace(input.Category), "其他"),
			MerchantNo: strings.TrimSpace(input.MerchantNo), APIBase: strings.TrimSpace(input.APIBase),
			LaunchPath: strings.TrimSpace(input.LaunchPath), SecretKey: strings.TrimSpace(input.SecretKey),
			Status: status, Remark: strings.TrimSpace(input.Remark), SortOrder: input.SortOrder,
		}
		if err := s.db.Create(&row).Error; err != nil {
			return nil, apperrors.NewSystemError("ENTERTAINMENT_SAVE_FAILED", "保存娱乐平台失败", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		row.Name = name
		row.Category = defaultString(strings.TrimSpace(input.Category), row.Category)
		row.MerchantNo = strings.TrimSpace(input.MerchantNo)
		row.APIBase = strings.TrimSpace(input.APIBase)
		row.LaunchPath = strings.TrimSpace(input.LaunchPath)
		if strings.TrimSpace(input.SecretKey) != "" {
			row.SecretKey = strings.TrimSpace(input.SecretKey)
		}
		row.Status = status
		row.Remark = strings.TrimSpace(input.Remark)
		row.SortOrder = input.SortOrder
		if err := s.db.Save(&row).Error; err != nil {
			return nil, apperrors.NewSystemError("ENTERTAINMENT_SAVE_FAILED", "保存娱乐平台失败", err)
		}
	}
	view := toPlatformView(row)
	return &view, nil
}

func (s *EntertainmentAdminService) SetStatus(id uint64, status string) (*PlatformView, error) {
	status = strings.TrimSpace(status)
	if status != "enabled" && status != "maintenance" && status != "disabled" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "平台状态不正确")
	}
	var row entertainment.Platform
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "平台不存在")
		}
		return nil, err
	}
	row.Status = status
	if err := s.db.Save(&row).Error; err != nil {
		return nil, err
	}
	view := toPlatformView(row)
	return &view, nil
}

func toPlatformView(row entertainment.Platform) PlatformView {
	return PlatformView{
		ID: row.ID, Code: row.Code, Name: row.Name, Category: row.Category,
		MerchantNo: row.MerchantNo, APIBase: row.APIBase, LaunchPath: row.LaunchPath,
		SecretKey: row.SecretKey, Status: row.Status, Remark: row.Remark, SortOrder: row.SortOrder,
	}
}

func (s *EntertainmentAdminService) ensureDefaults() error {
	var count int64
	if err := s.db.Model(&entertainment.Platform{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []entertainment.Platform{
		{Code: "kaiyuan", Name: "开元棋牌", Category: "棋牌", MerchantNo: "DEMO001", LaunchPath: "/portal", SecretKey: "demo", Status: "enabled", SortOrder: 1, Remark: "演示桥接页，可在管理端配置真实 API 地址"},
		{Code: "pg", Name: "PG电子", Category: "电子", MerchantNo: "DEMO002", LaunchPath: "/portal", SecretKey: "demo", Status: "maintenance", SortOrder: 2, Remark: "维护中"},
		{Code: "ag", Name: "AG真人", Category: "真人", MerchantNo: "DEMO003", LaunchPath: "/portal", SecretKey: "demo", Status: "disabled", SortOrder: 3},
		{Code: "im", Name: "IM电竞", Category: "电竞", MerchantNo: "DEMO004", LaunchPath: "/portal", SecretKey: "demo", Status: "disabled", SortOrder: 4},
	}
	return s.db.Create(&defaults).Error
}
