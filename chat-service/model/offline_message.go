package model

import "time"

// OfflineMessage holds a private chat payload until the recipient comes online.
type OfflineMessage struct {
	ID          string     `gorm:"primaryKey;size:36" json:"id"`
	ToUserID    string     `gorm:"size:32;index;not null" json:"to_user_id"`
	FromUserID  string     `gorm:"size:32;index;not null" json:"from_user_id"`
	Payload     string     `gorm:"type:text;not null" json:"payload"` // full ChatMessageDTO JSON
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}
