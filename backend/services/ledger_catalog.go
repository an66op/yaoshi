package services

import "strings"

var ledgerCategories = map[string]string{
	"manual":                "finance",
	"application_credit":    "finance",
	"application_debit":     "finance",
	"system_topup":          "finance",
	"opening_balance":       "finance",
	"seed_reconciliation":   "finance",
	"bet":                   "betting",
	"bet_cancel":            "betting",
	"reconciliation_refund": "betting",
	"settlement":            "betting",
	"rebate":                "welfare",
	"checkin":               "welfare",
	"redpacket":             "welfare",
	"redpacket_reserve":     "finance",
	"redpacket_refund":      "finance",
	"invite":                "welfare",
	"agent_share":           "share",
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
