package dto

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string   `json:"token"`
	User  UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Balance  int64  `json:"balance"`
}

// UpdateProfileRequest updates the signed-in user's profile.
// Username is required; password is optional (leave empty to keep current).
type UpdateProfileRequest struct {
	Username        string `json:"username" binding:"required,min=3,max=50"`
	Password        string `json:"password" binding:"omitempty,min=6,max=72"`
	CurrentPassword string `json:"current_password" binding:"omitempty"`
}
