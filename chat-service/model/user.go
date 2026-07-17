package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Username string `gorm:"uniqueIndex;size:50;not null" json:"username"`
	Password string `gorm:"size:255;not null" json:"-"`
	// Balance is virtual coin balance (integer units).
	Balance int64 `gorm:"not null;default:0" json:"balance"`
	// Avatar is public URL path e.g. /api/avatar/11 (empty = default letter avatar).
	Avatar string `gorm:"size:255" json:"avatar,omitempty"`
	// AvatarRev bumps on each upload for cache-busting (?v=).
	AvatarRev int64          `gorm:"not null;default:0" json:"avatar_rev,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
