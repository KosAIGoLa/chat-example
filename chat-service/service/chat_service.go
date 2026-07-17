package service

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ws-ex/dto"
	"ws-ex/model"
)

// ChatService contains the business logic for chat operations.
type ChatService struct {
	hub     *Hub
	nats    *NATSService
	crypto  *MsgCrypto
	store   *MessageStore
	friends *FriendService
	groups  *GroupService
}

// NewChatService creates a new ChatService.
func NewChatService(hub *Hub, ns *NATSService, crypto *MsgCrypto) *ChatService {
	if crypto == nil {
		crypto = NewMsgCrypto("")
	}
	return &ChatService{hub: hub, nats: ns, crypto: crypto}
}

// SetMessageStore attaches Postgres message metadata (recall).
func (s *ChatService) SetMessageStore(store *MessageStore) {
	s.store = store
}

// SetFriends attaches friendship checks for private chat.
func (s *ChatService) SetFriends(f *FriendService) {
	s.friends = f
}

// SetGroups attaches durable group membership service.
func (s *ChatService) SetGroups(g *GroupService) {
	s.groups = g
}

// Groups returns the durable group service (may be nil).
func (s *ChatService) Groups() *GroupService {
	return s.groups
}

// ListGroupMembers returns durable members with role + online (local hub).
// Caller may enrich online with remote presence.
func (s *ChatService) ListGroupMembers(groupID string, isOnline func(userID string) bool) ([]dto.GroupMemberDTO, error) {
	if s.groups == nil {
		return nil, fmt.Errorf("group service unavailable")
	}
	return s.groups.ListMembers(groupID, isOnline)
}

// HandleMessage processes an incoming WebSocket message from a client.
// It determines whether it's a private or group message and routes accordingly.
func (s *ChatService) HandleMessage(client *model.Client, raw []byte) {
	// Fast path: application heartbeat (client → server ping, server → client pong).
	// Handled before full DTO normalize so heartbeats stay cheap.
	var peek struct {
		Type string `json:"type"`
		TS   int64  `json:"ts,omitempty"`
	}
	if err := json.Unmarshal(raw, &peek); err == nil && peek.Type == "ping" {
		s.replyPong(client, peek.TS)
		return
	}

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

	// Typing indicators are ephemeral control frames — no encrypt / no history.
	if msg.Type == "typing" {
		s.handleTyping(client, &msg)
		return
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
	} else if msg.ContentType == "" && (msg.Type == "private" || msg.Type == "group") {
		msg.ContentType = "text"
	}

	// Encrypt message body for private/group chat before relay + history.
	// Clients may already send ciphertext; EnsureEncrypted is a no-op then.
	// System notices stay plaintext so every client can render them without keys.
	if (msg.Type == "private" || msg.Type == "group") && msg.ContentType != "system" {
		if err := s.encryptMessageContent(&msg); err != nil {
			log.Printf("[Chat] encrypt content from %s: %v", client.UserID, err)
			return
		}
	}

	switch msg.Type {
	case "private":
		s.sendPrivate(client, &msg)
	case "group":
		s.sendGroup(&msg)
	case "recall":
		s.recallMessage(client, &msg)
	case "edit":
		s.editMessage(client, &msg)
	case "join_group":
		s.joinGroup(client, &msg)
	case "leave_group":
		s.leaveGroup(client, &msg)
	case "history":
		s.sendHistory(client, &msg)
	case "presence":
		s.sendPresence(client, &msg)
	case "ping":
		// Defensive: should already be handled above.
		s.replyPong(client, msg.Timestamp)
	default:
		log.Printf("[Chat] unknown message type %q from %s", msg.Type, client.UserID)
	}
}

// replyPong answers a client application heartbeat.
// Wire format: {"type":"pong","ts":<client_ts>,"server_ts":<unix>}
func (s *ChatService) replyPong(client *model.Client, clientTS int64) {
	resp := dto.HeartbeatDTO{
		Type:     "pong",
		TS:       clientTS,
		ServerTS: time.Now().Unix(),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	select {
	case client.Send <- data:
	default:
		log.Printf("[Chat] client %s buffer full while sending pong", client.UserID)
	}
}

// handleTyping fans out ephemeral typing start/stop to peer or group members.
// Wire: {"type":"typing","to":"<peer>","content":"start"|"stop"} or group_id set.
func (s *ChatService) handleTyping(client *model.Client, msg *dto.ChatMessageDTO) {
	action := msg.Content
	if action != "start" && action != "stop" {
		if action == "1" || action == "true" || action == "" {
			action = "start"
		} else {
			action = "stop"
		}
	}

	ev := dto.TypingEvent{
		Type:      "typing",
		From:      client.UserID,
		FromName:  client.Username,
		Content:   action,
		Timestamp: time.Now().Unix(),
	}

	// Group typing: push to all durable members currently connected (any tab),
	// not only hub "joined room" — users may be viewing the group without re-join race.
	if msg.GroupID != "" {
		ev.GroupID = msg.GroupID
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		delivered := map[string]bool{client.UserID: true}
		// Hub room members (actively in group on this instance).
		for _, c := range s.hub.GetGroupMembers(msg.GroupID) {
			if delivered[c.UserID] {
				continue
			}
			select {
			case c.Send <- data:
				delivered[c.UserID] = true
			default:
			}
		}
		// Durable members who are online on this instance (even if not in hub room).
		if s.groups != nil {
			for _, uid := range s.groups.MemberUserIDs(msg.GroupID) {
				if delivered[uid] {
					continue
				}
				if s.hub.DeliverToUser(uid, data) {
					delivered[uid] = true
				}
			}
		}
		return
	}

	// Private typing
	peer := msg.To
	if peer == "" || peer == client.UserID {
		return
	}
	// Soft check: allow typing only between friends when friend service is wired.
	if s.friends != nil && !s.friends.AreFriendsStr(client.UserID, peer) {
		return
	}
	ev.To = peer
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.hub.DeliverToUser(peer, data)
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

// ensureMessageID assigns a stable id + monotonic seq and records metadata for recall.
func (s *ChatService) ensureMessageID(msg *dto.ChatMessageDTO) {
	if msg.ID == "" {
		msg.ID = NewMessageID()
	}
	if len(msg.ID) > 36 {
		msg.ID = msg.ID[:36]
	}
	// System notices still get a seq so clients can order them in history.
	if msg.Seq == 0 && s.store != nil {
		msg.Seq = s.store.NextSeq()
	}
	if s.store == nil {
		return
	}
	if msg.ContentType == "system" {
		// Persist system notices too (optional metadata); skip if store Save fails.
	}
	to := msg.To
	gid := msg.GroupID
	if msg.Type == "group" {
		if gid == "" {
			gid = msg.To
		}
		to = ""
	}
	_ = s.store.Save(&model.MessageRecord{
		ID:         msg.ID,
		Seq:        msg.Seq,
		Type:       msg.Type,
		FromUserID: msg.From,
		ToUserID:   to,
		GroupID:    gid,
		Timestamp:  msg.Timestamp,
	})
}

// sendPrivate always publishes via NATS (Core for real-time + JetStream for history).
// Local delivery is handled by the NATS subscription so we never skip persistence.
// Private chat requires an accepted friendship.
func (s *ChatService) sendPrivate(client *model.Client, msg *dto.ChatMessageDTO) {
	if msg.To == "" {
		return
	}
	if s.friends != nil {
		if s.friends.IsBlockedStr(msg.From, msg.To) {
			s.sendError(client, "blocked", "Cannot message: user is blocked")
			return
		}
		if !s.friends.AreFriendsStr(msg.From, msg.To) {
			s.sendError(client, "not_friends", "You can only message accepted friends")
			return
		}
	}
	s.ensureMessageID(msg)
	if err := s.nats.PublishPrivate(msg); err != nil {
		log.Printf("[Chat] failed to publish private message via NATS: %v", err)
	}
	// Echo id-bearing copy to sender (other tabs + confirm id after server assign).
	if data, err := json.Marshal(msg); err == nil {
		s.hub.DeliverToUser(msg.From, data)
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
	if msg.ContentType != "system" {
		s.ensureMessageID(msg)
	}
	if err := s.nats.PublishGroup(msg); err != nil {
		log.Printf("[Chat] failed to publish group message via NATS: %v", err)
	}
}

// recallMessage handles type=recall — only sender, within RecallWindow.
// Expects msg.ID (message id); To/GroupID optional (filled from store).
func (s *ChatService) recallMessage(client *model.Client, msg *dto.ChatMessageDTO) {
	if s.store == nil {
		s.sendError(client, "recall_unavailable", "recall not available")
		return
	}
	if msg.ID == "" {
		s.sendError(client, "recall_bad_id", "message id is required")
		return
	}
	rec, err := s.store.Recall(msg.ID, client.UserID)
	if err != nil {
		s.sendError(client, "recall_denied", err.Error())
		return
	}
	ev := dto.RecallEvent{
		Type:      "recall",
		ID:        rec.ID,
		From:      rec.FromUserID,
		To:        rec.ToUserID,
		GroupID:   rec.GroupID,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	// Core NATS (not JetStream) so every instance including this one fans out once.
	_ = s.nats.PublishEvent("chat.event.recall", data)
}

// editMessage handles type=edit — sender may change text body within EditWindow.
// Client should send encrypted Content (same as normal messages).
func (s *ChatService) editMessage(client *model.Client, msg *dto.ChatMessageDTO) {
	if s.store == nil {
		s.sendError(client, "edit_unavailable", "edit not available")
		return
	}
	if msg.ID == "" {
		s.sendError(client, "edit_bad_id", "message id is required")
		return
	}
	if strings.TrimSpace(msg.Content) == "" {
		s.sendError(client, "edit_empty", "content is required")
		return
	}
	// Encrypt if client sent plaintext (EnsureEncrypted no-op on already ciphertext).
	if err := s.encryptMessageContent(msg); err != nil {
		s.sendError(client, "edit_encrypt_failed", "failed to encrypt edited content")
		return
	}
	rec, err := s.store.Edit(msg.ID, client.UserID, msg.Content)
	if err != nil {
		s.sendError(client, "edit_denied", err.Error())
		return
	}
	ev := dto.EditEvent{
		Type:      "edit",
		ID:        rec.ID,
		From:      rec.FromUserID,
		To:        rec.ToUserID,
		GroupID:   rec.GroupID,
		Content:   msg.Content,
		Encrypted: true,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_ = s.nats.PublishEvent("chat.event.edit", data)
}

// sendError pushes a small system error frame to the client.
func (s *ChatService) sendError(client *model.Client, code, message string) {
	payload := map[string]string{
		"type":    "error",
		"code":    code,
		"message": message,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	select {
	case client.Send <- data:
	default:
	}
}

// EnrichSeq fills msg.Seq from Postgres when JetStream payload lacks seq (legacy).
func (s *ChatService) EnrichSeq(messages []dto.ChatMessageDTO) []dto.ChatMessageDTO {
	if s.store == nil || len(messages) == 0 {
		return messages
	}
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.ID != "" && m.Seq == 0 {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return messages
	}
	byID, err := s.store.SeqByIDs(ids)
	if err != nil || len(byID) == 0 {
		return messages
	}
	for i := range messages {
		if messages[i].Seq == 0 {
			if seq, ok := byID[messages[i].ID]; ok {
				messages[i].Seq = seq
			}
		}
	}
	return messages
}

// FilterPrivateAfterCutoff drops private messages at-or-before unfriend cut-off.
func (s *ChatService) FilterPrivateAfterCutoff(myID, peerID string, messages []dto.ChatMessageDTO) []dto.ChatMessageDTO {
	if s == nil || s.friends == nil || len(messages) == 0 {
		return messages
	}
	cut := s.friends.PrivateCutoffUnix(myID, peerID)
	if cut <= 0 {
		return messages
	}
	out := make([]dto.ChatMessageDTO, 0, len(messages))
	for _, m := range messages {
		if m.Timestamp > cut {
			out = append(out, m)
		}
	}
	return out
}

// ApplyRecalls marks recalled flags on history messages using the store.
func (s *ChatService) ApplyRecalls(messages []dto.ChatMessageDTO) []dto.ChatMessageDTO {
	if s.store == nil || len(messages) == 0 {
		return messages
	}
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	recalled, err := s.store.RecalledIDs(ids)
	if err != nil || len(recalled) == 0 {
		// still try edits
	} else {
		for i := range messages {
			if recalled[messages[i].ID] {
				messages[i].Recalled = true
				messages[i].Content = ""
				messages[i].MediaURL = ""
				messages[i].Encrypted = false
			}
		}
	}
	// Overlay latest edited ciphertext (not for recalled).
	edited, err := s.store.EditedBodies(ids)
	if err != nil || len(edited) == 0 {
		return messages
	}
	for i := range messages {
		if messages[i].Recalled {
			continue
		}
		if body, ok := edited[messages[i].ID]; ok && body != "" {
			messages[i].Content = body
			messages[i].Encrypted = true
			messages[i].Edited = true
		}
	}
	return messages
}

// joinGroup adds the client to a group locally.
// First membership of this user in the group → broadcast "加入到群"
// (unless content is "rejoin", used after reconnect / multi-tab sync).
func (s *ChatService) joinGroup(client *model.Client, msg *dto.ChatMessageDTO) {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	if groupID == "" {
		return
	}
	// Durable groups must exist; reject free-form ids that were never created.
	if s.groups != nil {
		if !s.groups.Exists(groupID) {
			s.sendError(client, "group_not_found", "group not found — create it first")
			return
		}
		if uid, err := parseUID(client.UserID); err == nil {
			if _, err := s.groups.Join(uid, groupID); err != nil {
				s.sendError(client, "group_join_failed", err.Error())
				return
			}
		}
	}
	isNew := s.hub.JoinGroup(groupID, client)
	rejoin := msg.Content == "rejoin"
	log.Printf("[Chat] user %s joined group %s (new=%v rejoin=%v)", client.UserID, groupID, isNew, rejoin)
	if isNew && !rejoin {
		s.broadcastSystemGroupNotice(client, groupID, "join")
	}
}

// leaveGroup removes the client from a group locally.
// When the user's last connection leaves → broadcast "退出群"
// (unless content is "silent").
func (s *ChatService) leaveGroup(client *model.Client, msg *dto.ChatMessageDTO) {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	if groupID == "" {
		return
	}
	fullyLeft := s.hub.LeaveGroup(groupID, client)
	silent := msg.Content == "silent" || msg.Content == "rejoin"
	log.Printf("[Chat] user %s left group %s (fully=%v silent=%v)", client.UserID, groupID, fullyLeft, silent)
	if fullyLeft && !silent {
		s.broadcastSystemGroupNotice(client, groupID, "leave")
	}
}

// JoinGroupForUser joins every local connection of userID (REST path).
// announce=true broadcasts "加入到群" when this is a true first join.
// Requires the group to exist in DB when GroupService is configured.
func (s *ChatService) JoinGroupForUser(userID, groupID string, announce bool) error {
	if s.groups != nil {
		uid, err := parseUID(userID)
		if err != nil {
			return err
		}
		if _, err := s.groups.Join(uid, groupID); err != nil {
			return err
		}
	}
	ok, isNew := s.hub.JoinGroupAll(groupID, userID)
	if !ok {
		// Durable membership already saved; offline join is OK.
		return nil
	}
	if announce && isNew {
		if c, found := s.hub.GetClient(userID); found {
			s.broadcastSystemGroupNotice(c, groupID, "join")
		}
	}
	return nil
}

// LeaveGroupForUser leaves every local connection of userID (REST path).
// announce=true broadcasts "退出群" when the user fully left.
func (s *ChatService) LeaveGroupForUser(userID, groupID string, announce bool) error {
	if s.groups != nil {
		uid, err := parseUID(userID)
		if err != nil {
			return err
		}
		if err := s.groups.Leave(uid, groupID); err != nil {
			return err
		}
	}
	ok, fullyLeft := s.hub.LeaveGroupAll(groupID, userID)
	if !ok {
		// Offline leave of durable membership is fine.
		return nil
	}
	if announce && fullyLeft {
		if c, found := s.hub.GetClient(userID); found {
			s.broadcastSystemGroupNotice(c, groupID, "leave")
		} else {
			s.broadcastSystemGroupNotice(&model.Client{UserID: userID, Username: userID}, groupID, "leave")
		}
	}
	return nil
}

// broadcastSystemGroupNotice publishes a system notice into the group stream
// so every online member (all instances) sees e.g. "Alice 加入到群" / "Alice 退出群".
// action: "join" | "leave"
func (s *ChatService) broadcastSystemGroupNotice(client *model.Client, groupID, action string) {
	if client == nil || groupID == "" {
		return
	}
	name := client.Username
	if name == "" {
		name = client.UserID
	}
	var content string
	switch action {
	case "leave":
		content = fmt.Sprintf("%s 退出群", name)
	default:
		content = fmt.Sprintf("%s 加入到群", name)
	}
	msg := &dto.ChatMessageDTO{
		Type:        "group",
		From:        client.UserID,
		To:          groupID,
		GroupID:     groupID,
		Content:     content,
		ContentType: "system",
		Timestamp:   time.Now().Unix(),
	}
	if err := s.nats.PublishGroup(msg); err != nil {
		log.Printf("[Chat] failed to publish group notice (%s) for %s in %s: %v",
			action, client.UserID, groupID, err)
	}
}

// BroadcastPlainGroupNotice publishes a system line (e.g. group dissolved).
func (s *ChatService) BroadcastPlainGroupNotice(groupID, content string) {
	if groupID == "" || content == "" {
		return
	}
	msg := &dto.ChatMessageDTO{
		Type:        "group",
		From:        "system",
		To:          groupID,
		GroupID:     groupID,
		Content:     content,
		ContentType: "system",
		Timestamp:   time.Now().Unix(),
	}
	if err := s.nats.PublishGroup(msg); err != nil {
		log.Printf("[Chat] failed to publish plain group notice in %s: %v", groupID, err)
	}
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
