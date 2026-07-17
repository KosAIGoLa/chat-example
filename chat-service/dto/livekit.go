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
	// Group meeting media mode (audio | video).
	Media string `json:"media,omitempty"`
}

// CallEvent is pushed over WebSocket for private 1:1 call signaling (not media).
// type: "call" — group uses MeetingEvent instead (meeting mode, not ring).
type CallEvent struct {
	Type     string `json:"type"`   // "call"
	Action   string `json:"action"` // invite | accept | reject | end | cancel
	Room     string `json:"room"`
	CallType string `json:"call_type"` // private | group (legacy group; prefer MeetingEvent)
	// Media: "audio" = voice-only, "video" = camera + mic.
	Media     string `json:"media,omitempty"`
	From      string `json:"from"`
	FromName  string `json:"from_name,omitempty"`
	To        string `json:"to,omitempty"`       // private peer
	GroupID   string `json:"group_id,omitempty"` // group meeting
	Timestamp int64  `json:"timestamp,omitempty"`
}

// MeetingActionRequest is POST /api/livekit/meeting
// Group conference mode: start / join / leave / end (not a private ring-call).
type MeetingActionRequest struct {
	GroupID string `json:"group_id"`
	// Action: start | join | leave | end
	Action string `json:"action"`
	// Media for start only: audio | video (default audio).
	Media string `json:"media,omitempty"`
}

// MeetingStatus is the active group meeting snapshot (+ optional LiveKit token on start/join).
type MeetingStatus struct {
	Active           bool   `json:"active"`
	GroupID          string `json:"group_id,omitempty"`
	Room             string `json:"room,omitempty"`
	Media            string `json:"media,omitempty"` // audio | video
	StartedBy        string `json:"started_by,omitempty"`
	StartedByName    string `json:"started_by_name,omitempty"`
	StartedAt        int64  `json:"started_at,omitempty"`
	ParticipantCount int    `json:"participant_count"`
	// Token fields only present after start/join.
	Token    string `json:"token,omitempty"`
	URL      string `json:"url,omitempty"`
	Identity string `json:"identity,omitempty"`
	// True when this request created a brand-new meeting.
	Created bool `json:"created,omitempty"`
	// True when leave/end closed the meeting for everyone.
	Ended bool `json:"ended,omitempty"`
}

// MeetingEvent is pushed over WebSocket for group conference lifecycle.
// type: "meeting" — distinct from private "call" ring signaling.
type MeetingEvent struct {
	Type             string `json:"type"`   // "meeting"
	Action           string `json:"action"` // started | ended | joined | left
	Room             string `json:"room"`
	Media            string `json:"media,omitempty"` // audio | video
	From             string `json:"from"`
	FromName         string `json:"from_name,omitempty"`
	GroupID          string `json:"group_id"`
	ParticipantCount int    `json:"participant_count,omitempty"`
	Timestamp        int64  `json:"timestamp,omitempty"`
}
