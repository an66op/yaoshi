package vo

// MemberRegisterRequest 会员自助注册
type MemberRegisterRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=20"`
	Password   string `json:"password" binding:"required,min=6"`
	Nickname   string `json:"nickname"`
	InviteCode string `json:"invite_code"`
	RoomCode   string `json:"room_code"`
}
