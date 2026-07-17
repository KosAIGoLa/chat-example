package model

import "time"

// Friend request status values.
const (
	FriendPending  = "pending"
	FriendAccepted = "accepted"
	FriendRejected = "rejected"
)

// FriendRequest is a one-way invite that becomes a Friendship only after accept.
type FriendRequest struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	FromUserID uint      `gorm:"index;not null;uniqueIndex:idx_friend_req_pair" json:"from_user_id"`
	ToUserID   uint      `gorm:"index;not null;uniqueIndex:idx_friend_req_pair" json:"to_user_id"`
	Status     string    `gorm:"size:16;index;not null;default:pending" json:"status"` // pending|accepted|rejected
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Friendship is a bidirectional accepted pair (UserAID < UserBID always).
type Friendship struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserAID   uint      `gorm:"index;not null;uniqueIndex:idx_friendship_pair" json:"user_a_id"`
	UserBID   uint      `gorm:"index;not null;uniqueIndex:idx_friendship_pair" json:"user_b_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Blacklist is one-way: UserID blocked BlockedUserID.
// Does NOT remove Friendship — only hides the peer from my friend list UI and
// blocks private chat / invites while active. Unblock restores friend list entry.
type Blacklist struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"index;not null;uniqueIndex:idx_blacklist_pair" json:"user_id"`
	BlockedUserID uint      `gorm:"index;not null;uniqueIndex:idx_blacklist_pair" json:"blocked_user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// PrivateConvCutoff hides private history between a pair after unfriend.
// UserAID < UserBID always (same ordering as Friendship).
// History API only returns private messages with Timestamp > CutAt.
// Re-adding as friends does not clear CutAt — conversation starts fresh.
type PrivateConvCutoff struct {
	UserAID uint  `gorm:"primaryKey;not null" json:"user_a_id"`
	UserBID uint  `gorm:"primaryKey;not null" json:"user_b_id"`
	CutAt   int64 `gorm:"not null" json:"cut_at"` // unix seconds
}
