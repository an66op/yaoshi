package api

import (
	"backend/controller/admin"
	"backend/controller/user"
	"backend/lotteryfeed"

	"gorm.io/gorm"
)

type HandlerRegistry struct {
	AuthHandler             user.AuthHandler // 登录注册
	MemberHandler           user.MemberHandler
	DashboardHandler        *admin.DashboardHandler
	UserAdminHandler        *admin.UserAdminHandler
	ApplicationAdminHandler *admin.ApplicationAdminHandler
	ReportHandler           *admin.ReportHandler
	SettingsHandler         *admin.SettingsHandler
	OddsHandler             *admin.OddsHandler
	WalletHandler           *admin.WalletHandler
	BetHandler              *admin.BetHandler
	OpsHandler              *admin.OpsHandler
	AgentHandler            *admin.AgentHandler
}

func InitHandlers(db *gorm.DB, scheduler *lotteryfeed.Scheduler) *HandlerRegistry {
	return &HandlerRegistry{
		AuthHandler:             user.NewAuthHandler(db),
		MemberHandler:           user.NewMemberHandler(db),
		DashboardHandler:        admin.NewDashboardHandler(db),
		UserAdminHandler:        admin.NewUserAdminHandler(db),
		ApplicationAdminHandler: admin.NewApplicationAdminHandler(db),
		ReportHandler:           admin.NewReportHandler(db),
		SettingsHandler:         admin.NewSettingsHandler(db),
		OddsHandler:             admin.NewOddsHandler(db),
		WalletHandler:           admin.NewWalletHandler(db),
		BetHandler:              admin.NewBetHandler(db),
		OpsHandler:              admin.NewOpsHandler(db),
		AgentHandler:            admin.NewAgentHandler(db),
	}
}
