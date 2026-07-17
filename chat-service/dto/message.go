package dto

// ChatMessageDTO is the DTO for all chat messages exchanged over WebSocket.
type ChatMessageDTO struct {
	// ID is a server-issued stable id (required for recall). Client may omit; server fills it.
	ID          string  `json:"id,omitempty"`
	Type        string  `json:"type"`                   // "private" | "group" | "ping" | "recall" | …
	From        string  `json:"from"`                   // sender user id
	To          string  `json:"to"`                     // recipient user id (private) or group id (group)
	Content     string  `json:"content"`                // text body (plaintext or enc:v1:… ciphertext)
	GroupID     string  `json:"group_id,omitempty"`     // group id for group chat
	Timestamp   int64   `json:"timestamp,omitempty"`    // unix seconds when sent (server-set)
	ContentType string  `json:"content_type,omitempty"` // "text" (default) | "voice" | "system"
	MediaURL    string  `json:"media_url,omitempty"`    // e.g. /api/voice/<id>.webm
	Duration    float64 `json:"duration,omitempty"`     // voice duration in seconds
	// Encrypted is true when Content is AES-GCM ciphertext (prefix enc:v1:).
	Encrypted bool `json:"encrypted,omitempty"`
	// Recalled is true after a successful recall (history + live updates).
	Recalled bool `json:"recalled,omitempty"`
}

// RecallEvent is pushed over WebSocket when a message is recalled.
// type: "recall"
type RecallEvent struct {
	Type      string `json:"type"` // "recall"
	ID        string `json:"id"`   // message id
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
	GroupID   string `json:"group_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// HeartbeatDTO is the application-level WebSocket heartbeat.
// Client → server: {"type":"ping","ts":<ms>}
// Server → client: {"type":"pong","ts":<echo>,"server_ts":<unix>}
type HeartbeatDTO struct {
	Type     string `json:"type"`                // "ping" | "pong"
	TS       int64  `json:"ts,omitempty"`        // client timestamp (echoed on pong)
	ServerTS int64  `json:"server_ts,omitempty"` // server unix seconds (pong only)
}

// CryptoKeyResponse is returned to authenticated clients for message encryption.
type CryptoKeyResponse struct {
	Algorithm string `json:"algorithm"` // AES-GCM
	Key       string `json:"key"`       // base64 raw 32-byte key
	Version   int    `json:"version"`   // wire format version
}

// VoiceUploadResponse is returned after a successful voice upload.
type VoiceUploadResponse struct {
	ID       string  `json:"id"`
	URL      string  `json:"url"`
	MimeType string  `json:"mime_type"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
}

// JoinDTO represents a client joining a group.
type JoinDTO struct {
	GroupID string `json:"group_id"`
}

// PresenceDTO represents a user's presence status stored in NATS KV,
// and is also pushed to WebSocket clients as a real-time event.
type PresenceDTO struct {
	Type     string `json:"type,omitempty"` // "presence" when pushed over WS
	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"`
	Online   bool   `json:"online"`
	Instance string `json:"instance,omitempty"`
	LastSeen int64  `json:"last_seen,omitempty"`
}

// OnlineUserDTO is a user currently connected (for the online list UI).
type OnlineUserDTO struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// GroupMembersEvent is pushed over WebSocket when a group's roster changes.
type GroupMembersEvent struct {
	Type    string          `json:"type"` // "group_members"
	GroupID string          `json:"group_id"`
	Members []OnlineUserDTO `json:"members"`
}

// RPCRequest is the generic Request/Reply payload for inter-service RPC.
type RPCRequest struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"payload,omitempty"`
}

// RPCResponse is the generic Request/Reply reply payload.
type RPCResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// APIResponseDTO is the standard REST API response wrapper.
type APIResponseDTO struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
