package model

import "time"

// MessageRecord stores send metadata so the server can authorize recalls.
// Full body still lives in NATS JetStream; this table is the recall source of truth.
type MessageRecord struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	Type       string     `gorm:"size:16;index;not null" json:"type"` // private | group
	FromUserID string     `gorm:"size:32;index;not null" json:"from_user_id"`
	ToUserID   string     `gorm:"size:64;index" json:"to_user_id"` // private peer or empty
	GroupID    string     `gorm:"size:64;index" json:"group_id"`
	Timestamp  int64      `gorm:"index;not null" json:"timestamp"` // unix seconds
	Recalled   bool       `gorm:"not null;default:false" json:"recalled"`
	RecalledAt *time.Time `json:"recalled_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
