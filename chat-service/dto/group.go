package dto

// CreateGroupRequest is POST /api/groups body.
// Name is required (display name). GroupID is optional custom public id.
type CreateGroupRequest struct {
	// Required display name (2–40 chars).
	Name string `json:"name"`
	// Optional custom group id (3–64 chars: letters, digits, _ -). Auto-generated if empty.
	GroupID string `json:"group_id,omitempty"`
}

// GroupDTO is returned for list / create.
type GroupDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OwnerUserID   string `json:"owner_user_id"`
	OwnerUsername string `json:"owner_username,omitempty"`
	Role          string `json:"role,omitempty"` // caller's role: owner | admin | member
	MemberCount   int    `json:"member_count,omitempty"`
	CreatedAt     int64  `json:"created_at,omitempty"`
	// IsMember is set on search results (whether the caller already joined).
	IsMember bool `json:"is_member,omitempty"`
	// Group icon (optional).
	Avatar    string `json:"avatar,omitempty"`
	AvatarRev int64  `json:"avatar_rev,omitempty"`
}

// UpdateGroupRequest is PATCH /api/groups/:group_id
type UpdateGroupRequest struct {
	Name string `json:"name,omitempty"`
}

// UpdateMemberRoleRequest is PATCH /api/groups/:group_id/members/:user_id
// role: admin | member (owner cannot be changed via this endpoint)
type UpdateMemberRoleRequest struct {
	Role string `json:"role"` // admin | member
}

// GroupDissolvedEvent is pushed over WebSocket when a group is dissolved.
// type: "group_dissolved"
type GroupDissolvedEvent struct {
	Type    string `json:"type"` // "group_dissolved"
	GroupID string `json:"group_id"`
	Name    string `json:"name,omitempty"`
	ByUser  string `json:"by_user_id,omitempty"`
}

// GroupMemberDTO is one durable group member with presence + role.
type GroupMemberDTO struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	// Role: owner | admin | member
	Role string `json:"role"`
	// Online: true when the user has an active WebSocket connection (any instance).
	Online bool `json:"online"`
}

// GroupAnnouncementDTO is one pinned group announcement (from a chat message).
type GroupAnnouncementDTO struct {
	ID           uint   `json:"id"`
	GroupID      string `json:"group_id"`
	MessageID    string `json:"message_id"`
	Content      string `json:"content"`
	ContentType  string `json:"content_type,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	SetByUserID  string `json:"set_by_user_id,omitempty"`
	MessageTS    int64  `json:"message_ts,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
}

// AddAnnouncementRequest is POST /api/groups/:id/announcements body (single or bulk).
type AddAnnouncementRequest struct {
	// Single
	MessageID    string `json:"message_id,omitempty"`
	Content      string `json:"content,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	MessageTS    int64  `json:"message_ts,omitempty"`
	// Bulk: multiple message_ids (content snapshots in items preferred).
	MessageIDs []string              `json:"message_ids,omitempty"`
	Items      []AddAnnouncementItem `json:"items,omitempty"`
}

// AddAnnouncementItem is one message to pin in a bulk request.
type AddAnnouncementItem struct {
	MessageID    string `json:"message_id"`
	Content      string `json:"content,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	FromUserID   string `json:"from_user_id,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	MessageTS    int64  `json:"message_ts,omitempty"`
}

// AnnouncementEvent is WS fan-out when announcements change.
// type: "group_announcement"
type AnnouncementEvent struct {
	Type      string                 `json:"type"`   // "group_announcement"
	Action    string                 `json:"action"` // "set" | "remove" | "set_bulk"
	GroupID   string                 `json:"group_id"`
	ByUserID  string                 `json:"by_user_id,omitempty"`
	Items     []GroupAnnouncementDTO `json:"items,omitempty"`
	MessageID string                 `json:"message_id,omitempty"` // remove
}
