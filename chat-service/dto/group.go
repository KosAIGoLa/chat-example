package dto

// CreateGroupRequest is POST /api/groups body.
type CreateGroupRequest struct {
	// Optional display name; defaults to the generated id.
	Name string `json:"name,omitempty"`
	// Optional custom group id (3–64 chars). Auto-generated if empty.
	GroupID string `json:"group_id,omitempty"`
}

// GroupDTO is returned for list / create.
type GroupDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OwnerUserID   string `json:"owner_user_id"`
	OwnerUsername string `json:"owner_username,omitempty"`
	Role          string `json:"role,omitempty"` // caller's role: owner | member
	MemberCount   int    `json:"member_count,omitempty"`
	CreatedAt     int64  `json:"created_at,omitempty"`
}

// GroupDissolvedEvent is pushed over WebSocket when a group is dissolved.
// type: "group_dissolved"
type GroupDissolvedEvent struct {
	Type    string `json:"type"` // "group_dissolved"
	GroupID string `json:"group_id"`
	Name    string `json:"name,omitempty"`
	ByUser  string `json:"by_user_id,omitempty"`
}
