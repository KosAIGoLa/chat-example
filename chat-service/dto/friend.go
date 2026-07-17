package dto

// FriendUserDTO is a friend list entry.
type FriendUserDTO struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

// FriendRequestDTO is an invite shown in the inbox / outbox.
type FriendRequestDTO struct {
	ID           uint   `json:"id"`
	FromUserID   string `json:"from_user_id"`
	FromUsername string `json:"from_username"`
	ToUserID     string `json:"to_user_id"`
	ToUsername   string `json:"to_username"`
	Status       string `json:"status"` // pending | accepted | rejected
	CreatedAt    int64  `json:"created_at"`
}

// FriendInviteRequest is POST /api/friends/request body.
type FriendInviteRequest struct {
	// Username of the user to invite (preferred).
	Username string `json:"username,omitempty"`
	// UserID alternative when known.
	UserID string `json:"user_id,omitempty"`
}

// FriendEvent is pushed over WebSocket for friend request / accept updates.
// type: "friend_event"
type FriendEvent struct {
	Type         string `json:"type"`   // "friend_event"
	Action       string `json:"action"` // "request" | "accepted" | "rejected" | "removed" | "blocked"
	RequestID    uint   `json:"request_id,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	ToUserID     string `json:"to_user_id,omitempty"`
	ToUsername   string `json:"to_username,omitempty"`
}

// BlacklistUserDTO is one entry in my block list.
type BlacklistUserDTO struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"created_at"`
}

// BlockUserRequest is POST /api/friends/blacklist body.
type BlockUserRequest struct {
	Username string `json:"username,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}
