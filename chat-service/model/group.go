package model

import "time"

// Group is a persisted chat room. GroupID is the public id used on the wire.
type Group struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"` // public group_id
	Name        string `gorm:"size:100;not null" json:"name"`
	OwnerUserID uint   `gorm:"index;not null" json:"owner_user_id"`
	// Avatar is public path e.g. /api/group-avatar/{group_id}
	Avatar string `gorm:"size:255" json:"avatar,omitempty"`
	// AvatarRev bumps on each upload for cache busting.
	AvatarRev int64     `gorm:"not null;default:0" json:"avatar_rev,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupMember is a durable membership (survives reconnect).
type GroupMember struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GroupID   string    `gorm:"size:64;index;not null;uniqueIndex:idx_group_member" json:"group_id"`
	UserID    uint      `gorm:"index;not null;uniqueIndex:idx_group_member" json:"user_id"`
	Role      string    `gorm:"size:16;not null;default:member" json:"role"` // owner | admin | member
	CreatedAt time.Time `json:"created_at"`
}

const (
	GroupRoleOwner  = "owner"
	GroupRoleAdmin  = "admin"  // 管理者
	GroupRoleMember = "member" // 一般成员
)

// GroupAnnouncement pins one group chat message as an announcement.
// Multiple announcements per group are allowed (ordered by CreatedAt desc by default).
type GroupAnnouncement struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	GroupID   string `gorm:"size:64;index;not null;uniqueIndex:idx_group_ann_msg" json:"group_id"`
	MessageID string `gorm:"size:36;not null;uniqueIndex:idx_group_ann_msg" json:"message_id"`
	// Snapshot of message body for display if original is gone / recalled.
	Content      string `gorm:"type:text" json:"content"`
	ContentType  string `gorm:"size:32" json:"content_type,omitempty"` // text | voice | …
	FromUserID   string `gorm:"size:32" json:"from_user_id,omitempty"`
	FromUsername string `gorm:"size:100" json:"from_username,omitempty"`
	// Who set it as announcement (owner/admin).
	SetByUserID string    `gorm:"size:32" json:"set_by_user_id,omitempty"`
	MessageTS   int64     `gorm:"index" json:"message_ts,omitempty"` // original message unix ts
	CreatedAt   time.Time `json:"created_at"`
}
