package services

import "strings"

var ledgerCategories = map[string]string{
	"manual":             "finance",
	"application_credit": "finance",
	"application_debit":  "finance",
	"bet":                "betting",
	"bet_cancel":         "betting",
	"settlement":         "betting",
	"rebate":             "welfare",
	"checkin":            "welfare",
	"redpacket":          "welfare",
	"invite":             "welfare",
	"agent_share":        "share",
}

func ledgerCategory(ledgerType string) string {
	if category, ok := ledgerCategories[strings.TrimSpace(ledgerType)]; ok {
		return category
	}
	return "other"
}

func ledgerCategoryLabel(category string) string {
	switch category {
	case "finance":
		return "资金操作"
	case "betting":
		return "投注结算"
	case "welfare":
		return "福利活动"
	case "share":
		return "代理分账"
	default:
		return "其他"
	}
}
