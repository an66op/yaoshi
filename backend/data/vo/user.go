package vo

// RegisterResponse 用户注册后的返回对象
type RegisterResponse struct {
	ID       uint64 `json:"id"`
	PublicID uint64 `json:"public_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname,omitempty"`
	Status   int    `json:"status"`
}

// UserResponse 登录时返回的用户信息（去掉敏感字段）
type UserResponse struct {
	ID       uint64 `json:"id"`
	PublicID uint64 `json:"public_id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Title    string `json:"public_title,omitempty"`
	Badge    string `json:"badge,omitempty"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}

// LoginResponse 用户登录响应
type LoginResponse struct {
	// Token is carried to the HTTP handler so it can set the HttpOnly cookie.
	// Browser responses clear it before serialization; omitempty prevents the
	// bearer credential from becoming readable JavaScript state.
	Token   string       `json:"token,omitempty"`
	User    UserResponse `json:"user"`
	Message string       `json:"message,omitempty"`
}

// MemberProfileResponse 会员端资料（含余额与房间）
type MemberProfileResponse struct {
	UserResponse
	Balance       float64 `json:"balance"`
	ParentAgentID *uint64 `json:"parent_agent_id,omitempty"`
	RoomCode      string  `json:"room_code,omitempty"`
	RoomName      string  `json:"room_name,omitempty"`
	RoomLogo      string  `json:"room_logo,omitempty"`
}

// JoinRoomRequest 会员进入代理房间
type JoinRoomRequest struct {
	RoomCode  string `json:"room_code" binding:"required"`
	RequestID string `json:"request_id" binding:"omitempty,max=96"`
}
