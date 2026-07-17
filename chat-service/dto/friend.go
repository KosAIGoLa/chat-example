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
	Action       string `json:"action"` // "request" | "accepted" | "rejected" | "removed" | "blocked" | "unblocked"
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

// PrivatePinDTO is one pinned private-chat message (shared by both peers).
type PrivatePinDTO struct {
	ID           uint   `json:"id"`
	PeerID       string `json:"peer_id,omitempty"` // conversation peer (from caller's view)
	MessageID    string `json:"message_id"`
	Content      string `json:"content"`
	ContentType  string `json:"content_type,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	SetByUserID  string `json:"set_by_user_id,omitempty"`
	MessageTS    int64  `json:"message_ts,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
}

// AddPrivatePinRequest is POST /api/private/:peer_id/pins body (single or bulk).
type AddPrivatePinRequest struct {
	MessageID    string              `json:"message_id,omitempty"`
	Content      string              `json:"content,omitempty"`
	ContentType  string              `json:"content_type,omitempty"`
	FromUserID   string              `json:"from_user_id,omitempty"`
	FromUsername string              `json:"from_username,omitempty"`
	MessageTS    int64               `json:"message_ts,omitempty"`
	MessageIDs   []string            `json:"message_ids,omitempty"`
	Items        []AddPrivatePinItem `json:"items,omitempty"`
}

// AddPrivatePinItem is one message to pin in a bulk request.
type AddPrivatePinItem struct {
	MessageID    string `json:"message_id"`
	Content      string `json:"content,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	MessageTS    int64  `json:"message_ts,omitempty"`
}

// PrivatePinEvent is WS fan-out when private pins change.
// type: "private_pin"
type PrivatePinEvent struct {
	Type      string          `json:"type"`    // "private_pin"
	Action    string          `json:"action"`  // "set" | "remove" | "set_bulk"
	PeerID    string          `json:"peer_id"` // the other party from recipient's view
	ByUserID  string          `json:"by_user_id,omitempty"`
	Items     []PrivatePinDTO `json:"items,omitempty"`
	MessageID string          `json:"message_id,omitempty"` // remove
}
