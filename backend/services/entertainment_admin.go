package services

import (
	"backend/data/models/entertainment"
	apperrors "backend/errors"
	"backend/utils"
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
	HasSecret  bool   `json:"has_secret"`
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
		configured, err := entertainmentProviderConfigured(row)
		if err != nil {
			return nil, apperrors.NewSystemError("ENTERTAINMENT_SECRET_READ_FAILED", "读取娱乐平台配置失败", err)
		}
		if !configured {
			view.Status = "maintenance"
			view.Message = fmt.Sprintf("%s 暂未开放。", row.Name)
			return view, nil
		}
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

func entertainmentProviderConfigured(row entertainment.Platform) (bool, error) {
	launchPath := strings.TrimSpace(row.LaunchPath)
	secret, err := utils.DecryptSensitive(row.SecretKey)
	if err != nil {
		return false, err
	}
	return (strings.HasPrefix(launchPath, "https://") || strings.HasPrefix(launchPath, "http://") || strings.TrimSpace(row.APIBase) != "") &&
		strings.TrimSpace(secret) != "" && !strings.EqualFold(strings.TrimSpace(secret), "demo") &&
		!strings.HasPrefix(strings.ToUpper(strings.TrimSpace(row.MerchantNo)), "DEMO"), nil
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
	secret, err := utils.DecryptSensitive(row.SecretKey)
	if err != nil {
		return nil, apperrors.NewSystemError("ENTERTAINMENT_SECRET_READ_FAILED", "读取娱乐平台配置失败", err)
	}
	if !strings.EqualFold(signLaunch(secret, row.MerchantNo, username, row.Code, sec), strings.TrimSpace(token)) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "签名无效")
	}
	return &row, nil
}

func (s *EntertainmentAdminService) buildLaunchURL(row entertainment.Platform, username string) (string, int64, error) {
	ts := time.Now().Unix()
	secret, err := utils.DecryptSensitive(row.SecretKey)
	if err != nil {
		return "", 0, apperrors.NewSystemError("ENTERTAINMENT_SECRET_READ_FAILED", "读取娱乐平台配置失败", err)
	}
	token := signLaunch(secret, row.MerchantNo, username, row.Code, ts)
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
		encryptedSecret, encryptErr := utils.EncryptSensitive(strings.TrimSpace(input.SecretKey))
		if encryptErr != nil {
			return nil, apperrors.NewSystemError("ENTERTAINMENT_SECRET_SAVE_FAILED", "保存娱乐平台失败", encryptErr)
		}
		row = entertainment.Platform{
			Code: code, Name: name, Category: defaultString(strings.TrimSpace(input.Category), "其他"),
			MerchantNo: strings.TrimSpace(input.MerchantNo), APIBase: strings.TrimSpace(input.APIBase),
			LaunchPath: strings.TrimSpace(input.LaunchPath), SecretKey: encryptedSecret,
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
			encryptedSecret, encryptErr := utils.EncryptSensitive(strings.TrimSpace(input.SecretKey))
			if encryptErr != nil {
				return nil, apperrors.NewSystemError("ENTERTAINMENT_SECRET_SAVE_FAILED", "保存娱乐平台失败", encryptErr)
			}
			row.SecretKey = encryptedSecret
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
		HasSecret: strings.TrimSpace(row.SecretKey) != "", Status: row.Status, Remark: row.Remark, SortOrder: row.SortOrder,
	}
}

func (s *EntertainmentAdminService) ensureDefaults() error {
	defaults := []entertainment.Platform{
		{Code: "kaiyuan", Name: "开元棋牌", Category: "棋牌", Status: "maintenance", SortOrder: 1, Remark: "等待配置供应商接口"},
		{Code: "pg", Name: "PG电子", Category: "电子", MerchantNo: "DEMO002", LaunchPath: "/portal", SecretKey: "demo", Status: "maintenance", SortOrder: 2, Remark: "维护中"},
		{Code: "ag", Name: "AG真人", Category: "真人", MerchantNo: "DEMO003", LaunchPath: "/portal", SecretKey: "demo", Status: "disabled", SortOrder: 3},
		{Code: "im", Name: "IM电竞", Category: "电竞", MerchantNo: "DEMO004", LaunchPath: "/portal", SecretKey: "demo", Status: "disabled", SortOrder: 4},
		{Code: "fish-lobby", Name: "捕鱼大厅", Category: "捕鱼", Status: "maintenance", SortOrder: 101, Remark: "第三方捕鱼线路接入中"},
		{Code: "fish-king-3d", Name: "捕鱼王3D", Category: "捕鱼", Status: "maintenance", SortOrder: 102, Remark: "第三方捕鱼线路接入中"},
		{Code: "fb-sports", Name: "FB体育", Category: "体育", Status: "maintenance", SortOrder: 201, Remark: "等待配置现有 FB 体育接口"},
		{Code: "im-sports", Name: "IM体育", Category: "体育", Status: "maintenance", SortOrder: 202, Remark: "第三方体育线路接入中"},
		{Code: "live-baccarat", Name: "百家乐", Category: "真人", Status: "maintenance", SortOrder: 301, Remark: "真人线路接入中"},
		{Code: "live-dragon-tiger", Name: "龙虎", Category: "真人", Status: "maintenance", SortOrder: 302, Remark: "真人线路接入中"},
		{Code: "live-golden-flower", Name: "炸金花", Category: "真人", Status: "maintenance", SortOrder: 303, Remark: "真人线路接入中"},
		{Code: "live-texas", Name: "德州扑克", Category: "真人", Status: "maintenance", SortOrder: 304, Remark: "真人线路接入中"},
		{Code: "slot-bounty-captain", Name: "赏金船长", Category: "电子", Status: "maintenance", SortOrder: 401, Remark: "电子线路接入中"},
		{Code: "slot-mahjong-ways", Name: "麻将胡了", Category: "电子", Status: "maintenance", SortOrder: 402, Remark: "电子线路接入中"},
		{Code: "esports-honor", Name: "王者荣耀", Category: "电竞", Status: "maintenance", SortOrder: 501, Remark: "电竞线路接入中"},
		{Code: "esports-hearthstone", Name: "炉石传说", Category: "电竞", Status: "maintenance", SortOrder: 502, Remark: "电竞线路接入中"},
		{Code: "esports-lol", Name: "英雄联盟", Category: "电竞", Status: "maintenance", SortOrder: 503, Remark: "电竞线路接入中"},
	}
	for _, template := range defaults {
		row := template
		if strings.TrimSpace(row.SecretKey) != "" {
			encryptedSecret, err := utils.EncryptSensitive(row.SecretKey)
			if err != nil {
				return apperrors.NewSystemError("ENTERTAINMENT_SECRET_SAVE_FAILED", "保存娱乐平台失败", err)
			}
			row.SecretKey = encryptedSecret
		}
		if err := s.db.Where("code = ?", row.Code).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	// Earlier local versions created a working-looking bridge with DEMO
	// credentials. Keep genuinely configured providers untouched, but migrate
	// those legacy placeholders to the honest unavailable state.
	var rows []entertainment.Platform
	if err := s.db.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		secret, err := utils.DecryptSensitive(row.SecretKey)
		if err != nil {
			return apperrors.NewSystemError("ENTERTAINMENT_SECRET_READ_FAILED", "读取娱乐平台配置失败", err)
		}
		placeholderMerchant := strings.HasPrefix(strings.ToUpper(strings.TrimSpace(row.MerchantNo)), "DEMO")
		placeholderSecret := strings.TrimSpace(secret) == "" || strings.EqualFold(strings.TrimSpace(secret), "demo")
		missingEndpoint := strings.TrimSpace(row.APIBase) == "" && (strings.TrimSpace(row.LaunchPath) == "" || strings.TrimSpace(row.LaunchPath) == "/portal")
		if row.Status == "enabled" && (placeholderSecret || placeholderMerchant) {
			if err := s.db.Model(&entertainment.Platform{}).Where("id = ?", row.ID).
				Updates(map[string]any{"status": "maintenance", "remark": "等待配置供应商接口"}).Error; err != nil {
				return err
			}
			continue
		}
		if missingEndpoint && placeholderSecret {
			if err := s.db.Model(&entertainment.Platform{}).Where("id = ?", row.ID).
				Updates(map[string]any{"status": "maintenance", "remark": "暂未开放，等待供应商接口"}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
