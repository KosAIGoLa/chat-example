package model

import "time"

// WalletLedger is an append-only balance change log.
type WalletLedger struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"user_id"`
	Delta        int64     `gorm:"not null" json:"delta"`
	BalanceAfter int64     `gorm:"not null" json:"balance_after"`
	Reason       string    `gorm:"size:64;not null" json:"reason"`
	RefType      string    `gorm:"size:32;index" json:"ref_type"`
	RefID        string    `gorm:"size:64;index" json:"ref_id"`
	CreatedAt    time.Time `json:"created_at"`
}
