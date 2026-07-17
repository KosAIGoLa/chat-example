package model

import "time"

// MessageRecord stores send metadata so the server can authorize recalls / edits.
// Full body still lives in NATS JetStream; this table is the recall/edit source of truth.
// Seq is a monotonic global order key used by clients for incremental history.
type MessageRecord struct {
	ID string `gorm:"primaryKey;size:36" json:"id"`
	// Seq is assigned from message_global_seq on insert; not unique-indexed so legacy
	// rows can share 0 until backfilled.
	Seq        int64      `gorm:"index;not null;default:0" json:"seq"`
	Type       string     `gorm:"size:16;index;not null" json:"type"` // private | group
	FromUserID string     `gorm:"size:32;index;not null" json:"from_user_id"`
	ToUserID   string     `gorm:"size:64;index" json:"to_user_id"` // private peer or empty
	GroupID    string     `gorm:"size:64;index" json:"group_id"`
	Timestamp  int64      `gorm:"index;not null" json:"timestamp"` // unix seconds
	Recalled   bool       `gorm:"not null;default:false" json:"recalled"`
	RecalledAt *time.Time `json:"recalled_at,omitempty"`
	// Edited body (ciphertext or plaintext) applied when loading history after an edit.
	Edited        bool       `gorm:"not null;default:false" json:"edited"`
	EditedAt      *time.Time `json:"edited_at,omitempty"`
	EditedContent string     `gorm:"type:text" json:"edited_content,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
