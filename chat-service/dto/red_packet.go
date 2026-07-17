package dto

// CreateRedPacketRequest is POST /api/red-packets body.
type CreateRedPacketRequest struct {
	Type          string   `json:"type" binding:"required"` // private | group | designated
	PeerID        string   `json:"peer_id"`
	GroupID       string   `json:"group_id"`
	TargetUserIDs []string `json:"target_user_ids"` // designated: who may claim
	TotalAmount   int64    `json:"total_amount" binding:"required"`
	TotalCount    int      `json:"total_count"` // group only; private=1; designated=len(targets)
	Greeting      string   `json:"greeting"`
}

// RedPacketClaimDTO is one claim entry.
type RedPacketClaimDTO struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Amount    int64  `json:"amount"`
	CreatedAt int64  `json:"created_at"`
}

// RedPacketDTO is the public packet view.
type RedPacketDTO struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	FromUserID      string   `json:"from_user_id"`
	ToUserID        string   `json:"to_user_id,omitempty"`
	GroupID         string   `json:"group_id,omitempty"`
	TargetUserIDs   []string `json:"target_user_ids,omitempty"`
	TotalAmount     int64    `json:"total_amount"`
	TotalCount      int      `json:"total_count"`
	RemainingAmount int64    `json:"remaining_amount"`
	RemainingCount  int      `json:"remaining_count"`
	Greeting        string   `json:"greeting"`
	Status          string   `json:"status"`
	MessageID       string   `json:"message_id,omitempty"`
	ExpiresAt       int64    `json:"expires_at"`
	CreatedAt       int64    `json:"created_at"`
	MyClaimAmount   int64    `json:"my_claim_amount,omitempty"`
	// CanClaim hints whether the viewer is allowed to grab (designated filter).
	CanClaim *bool               `json:"can_claim,omitempty"`
	Claims   []RedPacketClaimDTO `json:"claims,omitempty"`
}

// ClaimRedPacketResponse is returned after a successful grab.
type ClaimRedPacketResponse struct {
	PacketID       string `json:"packet_id"`
	Amount         int64  `json:"amount"`
	RemainingCount int    `json:"remaining_count"`
	Finished       bool   `json:"finished"`
	Balance        int64  `json:"balance"`
	Status         string `json:"status"`
}

// RedPacketClaimedEvent is pushed over WebSocket.
type RedPacketClaimedEvent struct {
	Type           string `json:"type"` // red_packet_claimed
	PacketID       string `json:"packet_id"`
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	Amount         int64  `json:"amount"`
	RemainingCount int    `json:"remaining_count"`
	Finished       bool   `json:"finished"`
	Timestamp      int64  `json:"timestamp"`
}

// WalletDTO is GET /api/wallet/me.
type WalletDTO struct {
	Balance int64 `json:"balance"`
}
