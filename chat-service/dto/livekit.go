package dto

// LiveKitTokenRequest is POST /api/livekit/token
// type: "private" | "group"
type LiveKitTokenRequest struct {
	Type    string `json:"type"`               // private | group
	PeerID  string `json:"peer_id,omitempty"`  // private call peer
	GroupID string `json:"group_id,omitempty"` // group meeting
	// Room is optional; server derives a stable room name if empty.
	// For private call invites, callee passes the same room from the invite.
	Room string `json:"room,omitempty"`
}

// LiveKitTokenResponse is returned to the client to join a LiveKit room.
type LiveKitTokenResponse struct {
	Token    string `json:"token"`
	URL      string `json:"url"` // e.g. ws://localhost:3000 (nginx /rtc → livekit)
	Room     string `json:"room"`
	Identity string `json:"identity"`
	// Call metadata for UI
	CallType string `json:"call_type"` // private | group
	PeerID   string `json:"peer_id,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
}

// CallEvent is pushed over WebSocket for call signaling (not media).
// type: "call"
type CallEvent struct {
	Type     string `json:"type"`   // "call"
	Action   string `json:"action"` // invite | accept | reject | end | cancel
	Room     string `json:"room"`
	CallType string `json:"call_type"` // private | group
	// Media: "audio" = voice-only, "video" = camera + mic (default video for group).
	Media     string `json:"media,omitempty"`
	From      string `json:"from"`
	FromName  string `json:"from_name,omitempty"`
	To        string `json:"to,omitempty"`       // private peer
	GroupID   string `json:"group_id,omitempty"` // group meeting
	Timestamp int64  `json:"timestamp,omitempty"`
}
