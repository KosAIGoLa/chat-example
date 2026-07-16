package service

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ws-ex/dto"
	"ws-ex/model"
)

// ChatService contains the business logic for chat operations.
type ChatService struct {
	hub    *Hub
	nats   *NATSService
	crypto *MsgCrypto
}

// NewChatService creates a new ChatService.
func NewChatService(hub *Hub, ns *NATSService, crypto *MsgCrypto) *ChatService {
	if crypto == nil {
		crypto = NewMsgCrypto("")
	}
	return &ChatService{hub: hub, nats: ns, crypto: crypto}
}

// HandleMessage processes an incoming WebSocket message from a client.
// It determines whether it's a private or group message and routes accordingly.
func (s *ChatService) HandleMessage(client *model.Client, raw []byte) {
	var msg dto.ChatMessageDTO
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Printf("[Chat] failed to unmarshal message from %s: %v", client.UserID, err)
		return
	}

	// Ensure the sender is set correctly.
	msg.From = client.UserID
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}
	// Normalize media messages.
	if msg.ContentType == "voice" {
		if msg.MediaURL == "" {
			log.Printf("[Chat] voice message from %s missing media_url", client.UserID)
			return
		}
		if msg.Content == "" {
			msg.Content = "🎤 Voice message"
		}
		if msg.Duration < 0 {
			msg.Duration = 0
		}
		if msg.Duration > MaxVoiceDurationSec {
			msg.Duration = MaxVoiceDurationSec
		}
	} else if msg.ContentType == "" {
		msg.ContentType = "text"
	}

	// Encrypt message body for private/group chat before relay + history.
	// Clients may already send ciphertext; EnsureEncrypted is a no-op then.
	if msg.Type == "private" || msg.Type == "group" {
		if err := s.encryptMessageContent(&msg); err != nil {
			log.Printf("[Chat] encrypt content from %s: %v", client.UserID, err)
			return
		}
	}

	switch msg.Type {
	case "private":
		s.sendPrivate(&msg)
	case "group":
		s.sendGroup(&msg)
	case "join_group":
		s.joinGroup(client, &msg)
	case "leave_group":
		s.leaveGroup(client, &msg)
	case "history":
		s.sendHistory(client, &msg)
	case "presence":
		s.sendPresence(client, &msg)
	default:
		log.Printf("[Chat] unknown message type %q from %s", msg.Type, client.UserID)
	}
}

// encryptMessageContent ensures Content is AES-GCM ciphertext on the wire and in history.
func (s *ChatService) encryptMessageContent(msg *dto.ChatMessageDTO) error {
	if msg.Content == "" {
		return nil
	}
	enc, err := s.crypto.EnsureEncrypted(msg.Content)
	if err != nil {
		return err
	}
	msg.Content = enc
	msg.Encrypted = IsEncrypted(enc)
	return nil
}

// sendPrivate always publishes via NATS (Core for real-time + JetStream for history).
// Local delivery is handled by the NATS subscription so we never skip persistence.
func (s *ChatService) sendPrivate(msg *dto.ChatMessageDTO) {
	if msg.To == "" {
		return
	}

	if err := s.nats.PublishPrivate(msg); err != nil {
		log.Printf("[Chat] failed to publish private message via NATS: %v", err)
	}
}

// sendGroup publishes a group message via NATS. All instances (including this one)
// receive it and deliver to their local group members.
func (s *ChatService) sendGroup(msg *dto.ChatMessageDTO) {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	if groupID == "" {
		return
	}
	msg.GroupID = groupID

	if err := s.nats.PublishGroup(msg); err != nil {
		log.Printf("[Chat] failed to publish group message via NATS: %v", err)
	}
}

// joinGroup adds the client to a group locally.
func (s *ChatService) joinGroup(client *model.Client, msg *dto.ChatMessageDTO) {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	if groupID == "" {
		return
	}
	s.hub.JoinGroup(groupID, client)
	log.Printf("[Chat] user %s joined group %s", client.UserID, groupID)
}

// leaveGroup removes the client from a group locally.
func (s *ChatService) leaveGroup(client *model.Client, msg *dto.ChatMessageDTO) {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	if groupID == "" {
		return
	}
	s.hub.LeaveGroup(groupID, client)
	log.Printf("[Chat] user %s left group %s", client.UserID, groupID)
}

// sendHistory retrieves message history from JetStream and sends it to the client.
// Private: {"type":"history","to":"<peerUserID>"}
// Group:   {"type":"history","group_id":"<groupID>"}
func (s *ChatService) sendHistory(client *model.Client, msg *dto.ChatMessageDTO) {
	var (
		messages []dto.ChatMessageDTO
		err      error
	)

	if msg.GroupID != "" {
		subject := fmt.Sprintf("chat.group.%s", msg.GroupID)
		messages, err = s.nats.GetMessageHistory(subject, 50)
	} else if msg.To != "" {
		messages, err = s.nats.GetPrivateHistory(client.UserID, msg.To, 50)
	} else {
		return
	}

	if err != nil {
		log.Printf("[Chat] failed to get history for user %s: %v", client.UserID, err)
		return
	}

	for _, m := range messages {
		data, _ := json.Marshal(m)
		select {
		case client.Send <- data:
		default:
			log.Printf("[Chat] client %s buffer full while sending history", client.UserID)
			return
		}
	}
}

// sendPresence queries user presence via KV (local) or RPC (cluster-wide).
// The client requests with: {"type":"presence","to":"<userID>"}
func (s *ChatService) sendPresence(client *model.Client, msg *dto.ChatMessageDTO) {
	if msg.To == "" {
		return
	}

	p, err := s.nats.GetPresence(msg.To)
	if err != nil {
		// User not in KV — try RPC to check if any instance has them online.
		rpcResp, rpcErr := s.nats.CallRPC("user.online", msg.To, 3*time.Second)
		if rpcErr != nil {
			log.Printf("[Chat] presence RPC failed for %s: %v", msg.To, rpcErr)
			return
		}
		online, _ := rpcResp.Data.(bool)
		p = &dto.PresenceDTO{UserID: msg.To, Online: online}
	}
	p.Type = "presence"

	data, _ := json.Marshal(p)
	select {
	case client.Send <- data:
	default:
		log.Printf("[Chat] client %s buffer full while sending presence", client.UserID)
	}
}
