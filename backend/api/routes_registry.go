package api

import (
	"backend/controller/admin"
	agentctrl "backend/controller/agent"
	tenantctrl "backend/controller/tenant"
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
	TenantHandler           *admin.TenantHandler
	AgentWorkspaceHandler   *agentctrl.WorkspaceHandler
	TenantWorkspaceHandler  *tenantctrl.WorkspaceHandler
	SystemAuditHandler      *admin.SystemAuditHandler
	PlanHandler             *admin.PlanHandler
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
		TenantHandler:           admin.NewTenantHandler(db),
		AgentWorkspaceHandler:   agentctrl.NewWorkspaceHandler(db),
		TenantWorkspaceHandler:  tenantctrl.NewWorkspaceHandler(db),
		SystemAuditHandler:      admin.NewSystemAuditHandler(db),
		PlanHandler:             admin.NewPlanHandler(db),
	}
}
