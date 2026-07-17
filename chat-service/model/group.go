package model

import "time"

// Group is a persisted chat room. GroupID is the public id used on the wire.
type Group struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"` // public group_id
	Name        string    `gorm:"size:100;not null" json:"name"`
	OwnerUserID uint      `gorm:"index;not null" json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GroupMember is a durable membership (survives reconnect).
type GroupMember struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GroupID   string    `gorm:"size:64;index;not null;uniqueIndex:idx_group_member" json:"group_id"`
	UserID    uint      `gorm:"index;not null;uniqueIndex:idx_group_member" json:"user_id"`
	Role      string    `gorm:"size:16;not null;default:member" json:"role"` // owner | member
	CreatedAt time.Time `json:"created_at"`
}

const (
	GroupRoleOwner  = "owner"
	GroupRoleMember = "member"
)
