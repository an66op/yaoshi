package constants

// 系统相关错误消息
const (
	ErrDatabaseConnectionFailed  = "数据库连接失败"
	ErrDatabaseMigrationFailed   = "数据库迁移失败"
	ErrInitDependenciesFailed    = "无法初始化依赖"
	ErrServerStartFailed         = "服务器启动失败"
	ErrGetWorkDirFailed          = "无法获取当前工作目录"
	ErrReadConfigFailed          = "读取配置文件失败"
	ErrParseConfigFailed         = "解析配置文件失败"
	ErrCreateAdminPasswordFailed = "创建默认管理员密码失败"
	ErrCreateAdminUserFailed     = "创建默认管理员失败"
)

// 系统相关成功消息
const (
	DatabaseConnectionSuccess = "数据库连接成功"
	ServerStartMessage        = "服务器启动，监听端口 %d"
)

// 默认管理员配置
const (
	DefaultAdminUsername = "admin"
	DefaultAdminPassword = "Admin8801!"
	DefaultAdminNickname = "系统管理员"
	DefaultAdminEmail    = "admin@example.com"
)
