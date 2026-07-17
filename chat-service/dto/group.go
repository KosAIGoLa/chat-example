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
