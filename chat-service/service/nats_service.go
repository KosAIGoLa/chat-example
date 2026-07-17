package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/nats-io/nats.go"

	"ws-ex/dto"
)

// NATSService wraps the NATS connection and provides:
//   - Core NATS: low-latency pub/sub for real-time chat delivery
//   - JetStream: reliable message persistence and replay
//   - KV: presence management
//   - Request/Reply: inter-service RPC
type NATSService struct {
	nc         *nats.Conn
	js         nats.JetStreamContext
	kv         nats.KeyValue
	hub        *Hub
	offline    *OfflineService
	instanceID string
}

// SetOffline wires the offline inbox used when private deliver fails.
func (ns *NATSService) SetOffline(o *OfflineService) {
	ns.offline = o
}

const (
	streamName     = "CHAT_MESSAGES"
	streamSubjects = "chat.>"
	kvBucket       = "presence"
	rpcSubject     = "rpc.chat.>"
)

// NewNATSService connects to NATS, sets up Core NATS subscriptions,
// initializes JetStream, creates the KV presence bucket, and registers RPC handlers.
func NewNATSService(url string, hub *Hub) (*NATSService, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = fmt.Sprintf("instance-%d", time.Now().UnixNano()%100000)
	}

	ns := &NATSService{nc: nc, hub: hub, instanceID: instanceID}

	// --- JetStream: reliable message persistence ---
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream init: %w", err)
	}
	ns.js = js

	// Ensure the CHAT_MESSAGES stream exists for durable message storage.
	// Retain several months so clients can scroll up for older history.
	const chatHistoryRetention = 180 * 24 * time.Hour // 6 months
	streamCfg := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{streamSubjects},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    chatHistoryRetention,
		MaxMsgs:   5_000_000,
		MaxBytes:  8 * 1024 * 1024 * 1024, // 8 GiB soft cap
	}
	_, err = js.AddStream(streamCfg)
	if err != nil {
		// Stream may already exist — try updating retention / limits.
		_, uerr := js.UpdateStream(streamCfg)
		if uerr != nil {
			return nil, fmt.Errorf("jetstream add/update stream: %w (update: %v)", err, uerr)
		}
	}

	// --- KV: presence management ---
	// Short TTL so crash leftovers disappear quickly; hub heartbeat refreshes every 30s.
	// 45s TTL: offline without clean Unregister cannot stay "online" longer than this.
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      kvBucket,
		Description: "user presence status",
		TTL:         45 * time.Second,
	})
	if err != nil {
		// Bucket may already exist — try fetching it.
		// Note: existing bucket TTL cannot be changed without recreate.
		kv, err = js.KeyValue(kvBucket)
		if err != nil {
			return nil, fmt.Errorf("kv create/get bucket: %w", err)
		}
	}
	ns.kv = kv

	// --- Core NATS: low-latency real-time subscriptions ---
	_, err = nc.Subscribe("chat.private.*", ns.handlePrivateMessage)
	if err != nil {
		return nil, fmt.Errorf("nats subscribe private: %w", err)
	}

	_, err = nc.Subscribe("chat.group.*", ns.handleGroupMessage)
	if err != nil {
		return nil, fmt.Errorf("nats subscribe group: %w", err)
	}

	// Presence change fan-out (online/offline) to all connected clients.
	_, err = nc.Subscribe("presence.event", ns.handlePresenceEvent)
	if err != nil {
		return nil, fmt.Errorf("nats subscribe presence: %w", err)
	}

	// Cross-instance recall / control events (not stored in JetStream history).
	_, err = nc.Subscribe("chat.event.>", ns.handleChatEvent)
	if err != nil {
		return nil, fmt.Errorf("nats subscribe chat events: %w", err)
	}

	// --- Request/Reply: inter-service RPC ---
	_, err = nc.Subscribe(rpcSubject, ns.handleRPC)
	if err != nil {
		return nil, fmt.Errorf("nats subscribe rpc: %w", err)
	}

	log.Printf("[NATS] connected (instance=%s) — Core NATS pub/sub, JetStream stream=%s, KV bucket=%s, RPC on %s",
		instanceID, streamName, kvBucket, rpcSubject)
	return ns, nil
}

// PublishPrivate publishes a private message to subject chat.private.<to>.
// A single JetStream publish both stores the message (for history) and
// delivers it to Core NATS subscribers (real-time). Do NOT also nc.Publish —
// subjects under chat.> are captured by the CHAT_MESSAGES stream, so dual
// publish would duplicate history entries.
func (ns *NATSService) PublishPrivate(msg *dto.ChatMessageDTO) error {
	subject := fmt.Sprintf("chat.private.%s", msg.To)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := ns.js.Publish(subject, data); err != nil {
		return fmt.Errorf("jetstream publish private: %w", err)
	}
	return nil
}

// PublishGroup publishes a group message to subject chat.group.<groupID>.
// Same single-publish rule as PublishPrivate (see comment there).
func (ns *NATSService) PublishGroup(msg *dto.ChatMessageDTO) error {
	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}
	subject := fmt.Sprintf("chat.group.%s", groupID)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := ns.js.Publish(subject, data); err != nil {
		return fmt.Errorf("jetstream publish group: %w", err)
	}
	return nil
}

// PublishEvent publishes a Core NATS event that is NOT captured by JetStream (outside chat.>).
func (ns *NATSService) PublishEvent(subject string, data []byte) error {
	return ns.nc.Publish(subject, data)
}

// handleChatEvent fans out control frames (e.g. recall) to local clients.
func (ns *NATSService) handleChatEvent(m *nats.Msg) {
	var peek struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		From    string `json:"from"`
		To      string `json:"to"`
		GroupID string `json:"group_id"`
	}
	if err := json.Unmarshal(m.Data, &peek); err != nil {
		return
	}
	switch peek.Type {
	case "recall":
		if peek.GroupID != "" {
			for _, c := range ns.hub.GetGroupMembers(peek.GroupID) {
				select {
				case c.Send <- m.Data:
				default:
				}
			}
			return
		}
		if peek.From != "" {
			ns.hub.DeliverToUser(peek.From, m.Data)
		}
		if peek.To != "" {
			ns.hub.DeliverToUser(peek.To, m.Data)
		}
	default:
		// ignore unknown events
	}
}

// handlePrivateMessage is the NATS subscription handler for private messages.
// It delivers to local clients, and always persists to offline inbox until ACK/delete.
func (ns *NATSService) handlePrivateMessage(m *nats.Msg) {
	var msg dto.ChatMessageDTO
	if err := json.Unmarshal(m.Data, &msg); err != nil {
		log.Printf("[NATS] failed to unmarshal private message: %v", err)
		return
	}

	data, _ := json.Marshal(msg)
	// Live deliver on this instance; if recipient is offline here, park in offline inbox.
	if ns.hub.DeliverToUser(msg.To, data) {
		return
	}
	if ns.offline != nil && msg.ID != "" && msg.To != "" {
		if err := ns.offline.Save(msg.ID, msg.To, msg.From, string(data)); err != nil {
			log.Printf("[NATS] offline save failed: %v", err)
		}
	}
}

// handleGroupMessage is the NATS subscription handler for group messages.
// It delivers the message to all local group members.
func (ns *NATSService) handleGroupMessage(m *nats.Msg) {
	var msg dto.ChatMessageDTO
	if err := json.Unmarshal(m.Data, &msg); err != nil {
		log.Printf("[NATS] failed to unmarshal group message: %v", err)
		return
	}

	groupID := msg.GroupID
	if groupID == "" {
		groupID = msg.To
	}

	members := ns.hub.GetGroupMembers(groupID)
	data, _ := json.Marshal(msg)
	// System notices (join/leave) are delivered to everyone including the actor.
	// Normal chat messages skip the sender (optimistic local echo on client).
	isSystem := msg.ContentType == "system"
	for _, client := range members {
		if !isSystem && client.UserID == msg.From {
			continue
		}
		select {
		case client.Send <- data:
		default:
			log.Printf("[NATS] client %s send buffer full, dropping group message", client.UserID)
		}
	}
}

// --- KV: Presence management ---

// SetPresence puts a user's presence status into the KV bucket.
// Offline uses Purge (hard delete) so Keys() cannot return ghost entries.
func (ns *NATSService) SetPresence(userID, username string, online bool) error {
	if !online {
		// Hard-delete so offline users never linger in Keys().
		if err := ns.kv.Purge(userID); err != nil && !errors.Is(err, nats.ErrKeyNotFound) {
			// Fall back to soft delete if purge fails on older servers.
			if delErr := ns.kv.Delete(userID); delErr != nil && !errors.Is(delErr, nats.ErrKeyNotFound) {
				return delErr
			}
		}
		return nil
	}

	p := dto.PresenceDTO{
		UserID:   userID,
		Username: username,
		Online:   true,
		Instance: ns.instanceID,
		LastSeen: time.Now().Unix(),
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = ns.kv.Put(userID, data)
	return err
}

// PublishPresence publishes a real-time presence event so every instance can
// push the online/offline change to its local WebSocket clients immediately.
func (ns *NATSService) PublishPresence(userID, username string, online bool) error {
	p := dto.PresenceDTO{
		Type:     "presence",
		UserID:   userID,
		Username: username,
		Online:   online,
		Instance: ns.instanceID,
		LastSeen: time.Now().Unix(),
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return ns.nc.Publish("presence.event", data)
}

// handlePresenceEvent fans out a presence change to all local WS clients.
func (ns *NATSService) handlePresenceEvent(m *nats.Msg) {
	// Ensure type is set for WS clients even if publisher omitted it.
	var p dto.PresenceDTO
	if err := json.Unmarshal(m.Data, &p); err != nil {
		log.Printf("[NATS] failed to unmarshal presence event: %v", err)
		return
	}
	p.Type = "presence"
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	ns.hub.BroadcastAll(data)
}

// GetPresence retrieves a user's presence status from the KV bucket.
func (ns *NATSService) GetPresence(userID string) (*dto.PresenceDTO, error) {
	entry, err := ns.kv.Get(userID)
	if err != nil {
		return nil, err
	}
	var p dto.PresenceDTO
	if err := json.Unmarshal(entry.Value(), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetAllPresence returns the presence status of all users in the KV bucket.
func (ns *NATSService) GetAllPresence() ([]dto.PresenceDTO, error) {
	keys, err := ns.kv.Keys()
	if err != nil {
		// Empty bucket is normal when nobody is online.
		if errors.Is(err, nats.ErrNoKeysFound) {
			return []dto.PresenceDTO{}, nil
		}
		return nil, err
	}
	var result []dto.PresenceDTO
	for _, key := range keys {
		entry, err := ns.kv.Get(key)
		if err != nil {
			continue
		}
		var p dto.PresenceDTO
		if err := json.Unmarshal(entry.Value(), &p); err == nil {
			result = append(result, p)
		}
	}
	return result, nil
}

// GetOnlineUserIDs returns cluster-wide online user IDs from the presence KV.
func (ns *NATSService) GetOnlineUserIDs() ([]string, error) {
	users, err := ns.GetFreshOnlineUsers(90 * time.Second)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.UserID)
	}
	return ids, nil
}

// GetFreshOnlineUsers returns presence entries that are online and recently heartbeated.
// Stale KV leftovers (server crash before purge) are filtered out by maxAge.
func (ns *NATSService) GetFreshOnlineUsers(maxAge time.Duration) ([]dto.OnlineUserDTO, error) {
	return ns.collectOnlineUsers(maxAge, false)
}

// GetRemoteOnlineUsers is like GetFreshOnlineUsers but skips entries owned by this instance.
// Used so local offline users are never reintroduced from stale local KV keys.
func (ns *NATSService) GetRemoteOnlineUsers(maxAge time.Duration) ([]dto.OnlineUserDTO, error) {
	return ns.collectOnlineUsers(maxAge, true)
}

func (ns *NATSService) collectOnlineUsers(maxAge time.Duration, remoteOnly bool) ([]dto.OnlineUserDTO, error) {
	presences, err := ns.GetAllPresence()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	maxAgeSec := int64(maxAge.Seconds())
	out := make([]dto.OnlineUserDTO, 0, len(presences))
	for _, p := range presences {
		if !p.Online {
			continue
		}
		if remoteOnly && p.Instance == ns.instanceID {
			// Local hub is authoritative; do not trust local KV for listing.
			continue
		}
		if p.LastSeen > 0 && now-p.LastSeen > maxAgeSec {
			// Stale — purge so it does not reappear.
			_ = ns.kv.Purge(p.UserID)
			continue
		}
		name := p.Username
		if name == "" {
			name = p.UserID
		}
		uid := p.UserID
		if uid == "" {
			continue
		}
		out = append(out, dto.OnlineUserDTO{UserID: uid, Username: name})
	}
	return out, nil
}

// --- Request/Reply: inter-service RPC ---

// handleRPC processes incoming RPC requests on the rpc.chat.> subject.
func (ns *NATSService) handleRPC(m *nats.Msg) {
	var req dto.RPCRequest
	if err := json.Unmarshal(m.Data, &req); err != nil {
		ns.reply(m, dto.RPCResponse{Success: false, Error: "invalid rpc request"})
		return
	}

	switch req.Action {
	case "presence.get":
		userID, ok := req.Payload.(string)
		if !ok {
			ns.reply(m, dto.RPCResponse{Success: false, Error: "invalid payload for presence.get"})
			return
		}
		p, err := ns.GetPresence(userID)
		if err != nil {
			ns.reply(m, dto.RPCResponse{Success: false, Error: err.Error()})
			return
		}
		ns.reply(m, dto.RPCResponse{Success: true, Data: p})

	case "presence.all":
		all, err := ns.GetAllPresence()
		if err != nil {
			ns.reply(m, dto.RPCResponse{Success: false, Error: err.Error()})
			return
		}
		ns.reply(m, dto.RPCResponse{Success: true, Data: all})

	case "user.online":
		userID, ok := req.Payload.(string)
		if !ok {
			ns.reply(m, dto.RPCResponse{Success: false, Error: "invalid payload for user.online"})
			return
		}
		_, ok = ns.hub.GetClient(userID)
		ns.reply(m, dto.RPCResponse{Success: true, Data: ok})

	case "message.deliver":
		// Direct message delivery to a local client via RPC.
		payloadBytes, err := json.Marshal(req.Payload)
		if err != nil {
			ns.reply(m, dto.RPCResponse{Success: false, Error: err.Error()})
			return
		}
		var msg dto.ChatMessageDTO
		if err := json.Unmarshal(payloadBytes, &msg); err != nil {
			ns.reply(m, dto.RPCResponse{Success: false, Error: err.Error()})
			return
		}
		data, _ := json.Marshal(msg)
		if !ns.hub.DeliverToUser(msg.To, data) {
			ns.reply(m, dto.RPCResponse{Success: false, Error: "user not on this instance"})
			return
		}
		ns.reply(m, dto.RPCResponse{Success: true})

	default:
		ns.reply(m, dto.RPCResponse{Success: false, Error: fmt.Sprintf("unknown rpc action: %s", req.Action)})
	}
}

// reply sends an RPC response.
func (ns *NATSService) reply(m *nats.Msg, resp dto.RPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = m.Respond(data)
}

// CallRPC sends a Request/Reply RPC call to the cluster and waits for a response.
func (ns *NATSService) CallRPC(action string, payload interface{}, timeout time.Duration) (*dto.RPCResponse, error) {
	req := dto.RPCRequest{Action: action, Payload: payload}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := ns.nc.Request(rpcSubject, data, timeout)
	if err != nil {
		return nil, err
	}
	var rpcResp dto.RPCResponse
	if err := json.Unmarshal(resp.Data, &rpcResp); err != nil {
		return nil, err
	}
	return &rpcResp, nil
}

// --- JetStream: message history ---

// maxHistoryPull hard-caps how many subject-matching messages we scan per request.
// Prevents multi-month full-stream reads from blocking the API for seconds.
const maxHistoryPull = 2500

// GetMessageHistory retrieves messages from JetStream for a subject.
// count <= 0: up to maxHistoryPull most recent messages for the subject.
// count > 0: only the most recent `count` (still scans subject history once, keeps a ring buffer).
func (ns *NATSService) GetMessageHistory(subject string, count int) ([]dto.ChatMessageDTO, error) {
	keep := count
	if keep <= 0 || keep > maxHistoryPull {
		keep = maxHistoryPull
	}

	// Ephemeral pull consumer bound to CHAT_MESSAGES (filtered by subject).
	// Skip per-message Ack to avoid ACK RTT on large history scans.
	sub, err := ns.js.PullSubscribe(subject, "", nats.BindStream(streamName))
	if err != nil {
		return nil, fmt.Errorf("pull subscribe: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Ring buffer of last `keep` messages (stream order is chronological).
	buf := make([]dto.ChatMessageDTO, 0, keep)
	scanned := 0
	idleRounds := 0
	for scanned < maxHistoryPull {
		batch := 500
		if maxHistoryPull-scanned < batch {
			batch = maxHistoryPull - scanned
		}
		// First batches wait longer; after data starts, fail fast on empty.
		wait := 400 * time.Millisecond
		if scanned > 0 {
			wait = 120 * time.Millisecond
		}
		msgs, err := sub.Fetch(batch, nats.MaxWait(wait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				idleRounds++
				if scanned == 0 || idleRounds >= 2 {
					break
				}
				continue
			}
			return nil, fmt.Errorf("fetch history: %w", err)
		}
		if len(msgs) == 0 {
			idleRounds++
			if idleRounds >= 2 {
				break
			}
			continue
		}
		idleRounds = 0
		for _, m := range msgs {
			scanned++
			var msg dto.ChatMessageDTO
			if err := json.Unmarshal(m.Data, &msg); err == nil {
				if len(buf) < keep {
					buf = append(buf, msg)
				} else {
					// Drop oldest, append newest — O(keep) mem, keeps tail.
					copy(buf[0:], buf[1:])
					buf[keep-1] = msg
				}
			}
			// AckNone consumer: Ack is a no-op / optional.
		}
		if scanned >= maxHistoryPull {
			break
		}
	}

	if count > 0 && len(buf) > count {
		return buf[len(buf)-count:], nil
	}
	return buf, nil
}

// GetPrivateHistory returns the conversation between two users.
// Private messages are stored on chat.private.<recipient>, so we load both
// directions and keep only messages exchanged between a and b.
// count <= 0 uses a capped window (maxHistoryPull per inbox).
func (ns *NATSService) GetPrivateHistory(userA, userB string, count int) ([]dto.ChatMessageDTO, error) {
	// Per-inbox scan budget: enough to find the conversation pair without full 6-month dump.
	perSide := count * 8
	if perSide < 200 {
		perSide = 200
	}
	if perSide > maxHistoryPull {
		perSide = maxHistoryPull
	}
	if count <= 0 {
		perSide = maxHistoryPull
	}

	type sideResult struct {
		msgs []dto.ChatMessageDTO
		err  error
	}
	chB := make(chan sideResult, 1)
	chA := make(chan sideResult, 1)
	go func() {
		m, e := ns.GetMessageHistory(fmt.Sprintf("chat.private.%s", userB), perSide)
		chB <- sideResult{m, e}
	}()
	go func() {
		m, e := ns.GetMessageHistory(fmt.Sprintf("chat.private.%s", userA), perSide)
		chA <- sideResult{m, e}
	}()
	rb := <-chB
	ra := <-chA
	if rb.err != nil {
		return nil, rb.err
	}
	if ra.err != nil {
		return nil, ra.err
	}

	var merged []dto.ChatMessageDTO
	for _, m := range rb.msgs {
		if m.Type == "private" && m.From == userA && m.To == userB {
			merged = append(merged, m)
		}
	}
	for _, m := range ra.msgs {
		if m.Type == "private" && m.From == userB && m.To == userA {
			merged = append(merged, m)
		}
	}

	sortChatMessages(merged)

	if count > 0 && len(merged) > count {
		merged = merged[len(merged)-count:]
	}
	return merged, nil
}

func sortChatMessages(msgs []dto.ChatMessageDTO) {
	SortChatMessages(msgs)
}

// SortChatMessages orders by seq (prefer), then timestamp, then id.
func SortChatMessages(msgs []dto.ChatMessageDTO) {
	if len(msgs) < 2 {
		return
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		return chatMsgLess(msgs[i], msgs[j])
	})
}

// chatMsgLess reports whether a should appear before b (ascending order).
func chatMsgLess(a, b dto.ChatMessageDTO) bool {
	// Prefer seq when both non-zero.
	if a.Seq > 0 && b.Seq > 0 {
		return a.Seq < b.Seq
	}
	if a.Seq > 0 && b.Seq == 0 {
		// sequenced messages are newer than legacy — sort by timestamp fallback
		if a.Timestamp != b.Timestamp {
			return a.Timestamp < b.Timestamp
		}
		return a.Seq < b.Seq
	}
	if a.Seq == 0 && b.Seq > 0 {
		if a.Timestamp != b.Timestamp {
			return a.Timestamp < b.Timestamp
		}
		return true
	}
	if a.Timestamp != b.Timestamp {
		return a.Timestamp < b.Timestamp
	}
	// Stable tie-break by id.
	return a.ID < b.ID
}

// MaxSeq returns the highest seq in the slice (0 if empty / all zero).
func MaxSeq(msgs []dto.ChatMessageDTO) int64 {
	var max int64
	for _, m := range msgs {
		if m.Seq > max {
			max = m.Seq
		}
	}
	return max
}

// MinSeq returns the lowest positive seq in the slice (0 if none).
func MinSeq(msgs []dto.ChatMessageDTO) int64 {
	var min int64
	for _, m := range msgs {
		if m.Seq <= 0 {
			continue
		}
		if min == 0 || m.Seq < min {
			min = m.Seq
		}
	}
	return min
}

// FilterSinceSeq keeps messages with Seq > since (messages with seq=0 kept only when since=0).
func FilterSinceSeq(msgs []dto.ChatMessageDTO, since int64) []dto.ChatMessageDTO {
	if since <= 0 {
		return msgs
	}
	out := make([]dto.ChatMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		if m.Seq > since {
			out = append(out, m)
		}
	}
	return out
}

// FilterBeforeSeq keeps messages strictly older than before (by seq when present).
// Messages without seq fall back to timestamp < beforeTS when beforeTS > 0.
func FilterBeforeSeq(msgs []dto.ChatMessageDTO, beforeSeq, beforeTS int64) []dto.ChatMessageDTO {
	if beforeSeq <= 0 && beforeTS <= 0 {
		return msgs
	}
	out := make([]dto.ChatMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		if m.Seq > 0 && beforeSeq > 0 {
			if m.Seq < beforeSeq {
				out = append(out, m)
			}
			continue
		}
		// Legacy / missing seq: use timestamp cursor.
		if beforeTS > 0 && m.Timestamp > 0 && m.Timestamp < beforeTS {
			out = append(out, m)
		}
	}
	return out
}

// --- Lifecycle ---

// Close closes the NATS connection.
func (ns *NATSService) Close() {
	if ns.nc != nil {
		ns.nc.Close()
	}
}
