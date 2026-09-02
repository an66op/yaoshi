package vo

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Email    string `json:"email" binding:"required,email"`
	Nickname string `json:"nickname" binding:"omitempty"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username    string `json:"username" binding:"required,max=50"`
	Password    string `json:"password" binding:"required"`
	Workspace   string `json:"workspace" binding:"omitempty,max=80"`
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}
