package model

import "time"

const (
	RedPacketTypePrivate = "private"
	RedPacketTypeGroup   = "group"

	RedPacketStatusOpen     = "open"
	RedPacketStatusFinished = "finished"
	RedPacketStatusExpired  = "expired"
	RedPacketStatusRefunded = "refunded"
)

// RedPacket is a private (full claim) or group (random claim) packet.
type RedPacket struct {
	ID              string    `gorm:"primaryKey;size:36" json:"id"`
	Type            string    `gorm:"size:16;index;not null" json:"type"` // private | group
	FromUserID      string    `gorm:"size:32;index;not null" json:"from_user_id"`
	ToUserID        string    `gorm:"size:32;index" json:"to_user_id"`
	GroupID         string    `gorm:"size:64;index" json:"group_id"`
	TotalAmount     int64     `gorm:"not null" json:"total_amount"`
	TotalCount      int       `gorm:"not null" json:"total_count"`
	RemainingAmount int64     `gorm:"not null" json:"remaining_amount"`
	RemainingCount  int       `gorm:"not null" json:"remaining_count"`
	Greeting        string    `gorm:"size:200" json:"greeting"`
	Status          string    `gorm:"size:16;index;not null" json:"status"`
	MessageID       string    `gorm:"size:36;index" json:"message_id"`
	ExpiresAt       time.Time `gorm:"index;not null" json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// RedPacketClaim records one user's grab.
type RedPacketClaim struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PacketID  string    `gorm:"size:36;uniqueIndex:idx_packet_user;not null" json:"packet_id"`
	UserID    string    `gorm:"size:32;uniqueIndex:idx_packet_user;not null" json:"user_id"`
	Username  string    `gorm:"size:50" json:"username"`
	Amount    int64     `gorm:"not null" json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}
