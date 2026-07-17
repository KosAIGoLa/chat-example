package service

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"ws-ex/dto"
	"ws-ex/model"
)

// Hub manages all local WebSocket connections on this instance.
// A user may have multiple concurrent connections (multiple tabs).
// It integrates with NATS KV for cluster-wide presence management.
type Hub struct {
	// userID -> set of live connections
	clients map[string]map[*model.Client]struct{}
	// groupID -> set of connections currently in the group
	groups    map[string]map[*model.Client]struct{}
	nats      *NATSService
	presenceT *time.Ticker
	mu        sync.RWMutex
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*model.Client]struct{}),
		groups:  make(map[string]map[*model.Client]struct{}),
	}
}

// SetNATS links the NATS service for KV presence management and starts a heartbeat.
func (h *Hub) SetNATS(ns *NATSService) {
	h.nats = ns
	// Refresh often enough that stale KV entries cannot outlive a dead connection long.
	h.presenceT = time.NewTicker(30 * time.Second)
	go func() {
		for range h.presenceT.C {
			h.refreshPresence()
		}
	}()
}

// refreshPresence re-publishes all local users' presence to KV to prevent TTL expiry.
func (h *Hub) refreshPresence() {
	if h.nats == nil {
		return
	}
	type userRef struct {
		id, name string
	}
	h.mu.RLock()
	users := make([]userRef, 0, len(h.clients))
	for uid, conns := range h.clients {
		if len(conns) == 0 {
			continue
		}
		name := uid
		for c := range conns {
			if c.Username != "" {
				name = c.Username
			}
			break
		}
		users = append(users, userRef{id: uid, name: name})
	}
	h.mu.RUnlock()

	for _, u := range users {
		if err := h.nats.SetPresence(u.id, u.name, true); err != nil {
			log.Printf("[Hub] failed to refresh presence for %s: %v", u.id, err)
		}
	}
}

// Register adds a client to the hub without dropping other connections for the same user.
// Multiple tabs may stay online simultaneously (previously they kicked each other → reconnect loop).
func (h *Hub) Register(client *model.Client) {
	h.mu.Lock()
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*model.Client]struct{})
	}
	// First connection for this user on this instance → publish online.
	wasOffline := len(h.clients[client.UserID]) == 0
	h.clients[client.UserID][client] = struct{}{}
	h.mu.Unlock()

	if h.nats != nil && wasOffline {
		if err := h.nats.SetPresence(client.UserID, client.Username, true); err != nil {
			log.Printf("[Hub] failed to set presence for %s: %v", client.UserID, err)
		}
		if err := h.nats.PublishPresence(client.UserID, client.Username, true); err != nil {
			log.Printf("[Hub] failed to publish online presence for %s: %v", client.UserID, err)
		}
	}
}

// Unregister removes a client from the hub and its groups.
// Presence goes offline only when the user's last connection on this instance is gone.
func (h *Hub) Unregister(client *model.Client) {
	h.mu.Lock()
	conns, ok := h.clients[client.UserID]
	if !ok {
		h.mu.Unlock()
		return
	}
	if _, present := conns[client]; !present {
		h.mu.Unlock()
		return
	}
	delete(conns, client)
	wentOffline := len(conns) == 0
	if wentOffline {
		delete(h.clients, client.UserID)
	}

	affectedGroups := make([]string, 0)
	for _, groupID := range client.GetGroups() {
		if members, ok := h.groups[groupID]; ok {
			if _, in := members[client]; in {
				delete(members, client)
				if len(members) == 0 {
					delete(h.groups, groupID)
				}
				// Roster may change for others even if this user still has another tab in the group.
				affectedGroups = append(affectedGroups, groupID)
			}
		}
	}
	// If user still has another connection in a group, member list user-identity is unchanged;
	// still rebroadcast so connection-level fans stay consistent.
	username := client.Username
	userID := client.UserID
	h.mu.Unlock()

	if wentOffline && h.nats != nil {
		if err := h.nats.SetPresence(userID, username, false); err != nil {
			log.Printf("[Hub] failed to clear presence for %s: %v", userID, err)
		}
		if err := h.nats.PublishPresence(userID, username, false); err != nil {
			log.Printf("[Hub] failed to publish offline presence for %s: %v", userID, err)
		}
	}
	for _, gid := range affectedGroups {
		h.broadcastGroupMembers(gid)
	}
}

// BroadcastAll sends a raw message payload to every local WebSocket connection.
func (h *Hub) BroadcastAll(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conns := range h.clients {
		for client := range conns {
			select {
			case client.Send <- data:
			default:
				log.Printf("[Hub] client %s buffer full, dropping broadcast", client.UserID)
			}
		}
	}
}

// DeliverToUser sends a payload to every local connection for userID.
// Returns true if at least one connection accepted the message.
func (h *Hub) DeliverToUser(userID string, data []byte) bool {
	h.mu.RLock()
	conns := h.clients[userID]
	clients := make([]*model.Client, 0, len(conns))
	for c := range conns {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	delivered := false
	for _, client := range clients {
		select {
		case client.Send <- data:
			delivered = true
		default:
			log.Printf("[Hub] client %s buffer full, dropping direct message", userID)
		}
	}
	return delivered
}

// GetClient returns any one live connection for userID (for APIs that only need "is online").
func (h *Hub) GetClient(userID string) (*model.Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.clients[userID]
	if !ok || len(conns) == 0 {
		return nil, false
	}
	for c := range conns {
		return c, true
	}
	return nil, false
}

// GetClients returns all live connections for userID.
func (h *Hub) GetClients(userID string) []*model.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.clients[userID]
	if !ok || len(conns) == 0 {
		return nil
	}
	out := make([]*model.Client, 0, len(conns))
	for c := range conns {
		out = append(out, c)
	}
	return out
}

// JoinGroup adds a connection to a group and notifies members of the new roster.
// Returns true when this is the first connection of that user in the group
// (true "enter group" — not a multi-tab / reconnect re-join).
func (h *Hub) JoinGroup(groupID string, client *model.Client) (isNewMember bool) {
	h.mu.Lock()
	if _, ok := h.groups[groupID]; !ok {
		h.groups[groupID] = make(map[*model.Client]struct{})
	}
	already := false
	for c := range h.groups[groupID] {
		if c.UserID == client.UserID {
			already = true
			break
		}
	}
	h.groups[groupID][client] = struct{}{}
	h.mu.Unlock()
	client.JoinGroup(groupID)
	h.broadcastGroupMembers(groupID)
	return !already
}

// JoinGroupAll adds every local connection of userID to the group (REST join).
// isNewMember is true if the user was not already in the group on any connection.
func (h *Hub) JoinGroupAll(groupID, userID string) (ok bool, isNewMember bool) {
	clients := h.GetClients(userID)
	if len(clients) == 0 {
		return false, false
	}
	for _, c := range clients {
		if h.JoinGroup(groupID, c) {
			isNewMember = true
		}
	}
	return true, isNewMember
}

// LeaveGroup removes a connection from a group and notifies remaining members.
// Returns true when the user has no remaining connections in the group
// (true "leave group" — last tab left).
func (h *Hub) LeaveGroup(groupID string, client *model.Client) (fullyLeft bool) {
	h.mu.Lock()
	if members, ok := h.groups[groupID]; ok {
		delete(members, client)
		if len(members) == 0 {
			delete(h.groups, groupID)
		}
	}
	// Still in group if another connection of same user remains.
	stillIn := false
	if members, ok := h.groups[groupID]; ok {
		for c := range members {
			if c.UserID == client.UserID {
				stillIn = true
				break
			}
		}
	}
	h.mu.Unlock()
	client.LeaveGroup(groupID)
	h.broadcastGroupMembers(groupID)
	return !stillIn
}

// LeaveGroupAll removes every local connection of userID from the group (REST leave).
// fullyLeft is true if the user no longer has any connection in the group.
func (h *Hub) LeaveGroupAll(groupID, userID string) (ok bool, fullyLeft bool) {
	clients := h.GetClients(userID)
	if len(clients) == 0 {
		return false, false
	}
	// Was the user in the group before we remove them?
	wasIn := false
	h.mu.RLock()
	if members, ok := h.groups[groupID]; ok {
		for c := range members {
			if c.UserID == userID {
				wasIn = true
				break
			}
		}
	}
	h.mu.RUnlock()

	for _, c := range clients {
		h.LeaveGroup(groupID, c)
	}
	return true, wasIn
}

// broadcastGroupMembers pushes a personalized member list to each connection in the group
// (each client sees the roster without themselves).
func (h *Hub) broadcastGroupMembers(groupID string) {
	for _, c := range h.GetGroupMembers(groupID) {
		members := h.GetGroupMemberInfos(groupID, c.UserID)
		ev := dto.GroupMembersEvent{
			Type:    "group_members",
			GroupID: groupID,
			Members: members,
		}
		if ev.Members == nil {
			ev.Members = []dto.OnlineUserDTO{}
		}
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		select {
		case c.Send <- data:
		default:
			log.Printf("[Hub] client %s buffer full, dropping group_members", c.UserID)
		}
	}
}

// GetGroupMembers returns all connections currently in a group on this instance.
func (h *Hub) GetGroupMembers(groupID string) []*model.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[groupID]
	if !ok {
		return nil
	}
	clients := make([]*model.Client, 0, len(members))
	for c := range members {
		clients = append(clients, c)
	}
	return clients
}

// GetGroupMemberInfos returns unique id+username for members currently in a group.
// excludeUserID, when non-empty, is omitted (typically the caller themselves).
func (h *Hub) GetGroupMemberInfos(groupID string, excludeUserID string) []dto.OnlineUserDTO {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[groupID]
	if !ok {
		return []dto.OnlineUserDTO{}
	}
	seen := make(map[string]dto.OnlineUserDTO, len(members))
	for c := range members {
		if excludeUserID != "" && c.UserID == excludeUserID {
			continue
		}
		if _, exists := seen[c.UserID]; exists {
			continue
		}
		name := c.Username
		if name == "" {
			name = c.UserID
		}
		seen[c.UserID] = dto.OnlineUserDTO{
			UserID:   c.UserID,
			Username: name,
		}
	}
	users := make([]dto.OnlineUserDTO, 0, len(seen))
	for _, u := range seen {
		users = append(users, u)
	}
	return users
}

// GetOnlineUsers returns the user IDs of all connected users on this instance.
func (h *Hub) GetOnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]string, 0, len(h.clients))
	for uid, conns := range h.clients {
		if len(conns) > 0 {
			users = append(users, uid)
		}
	}
	return users
}

// GetOnlineUserInfos returns id+username for every local connected user (one entry per user).
// excludeUserID, when non-empty, is omitted from the list (typically the caller).
func (h *Hub) GetOnlineUserInfos(excludeUserID string) []dto.OnlineUserDTO {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]dto.OnlineUserDTO, 0, len(h.clients))
	for uid, conns := range h.clients {
		if len(conns) == 0 {
			continue
		}
		if excludeUserID != "" && uid == excludeUserID {
			continue
		}
		name := uid
		for c := range conns {
			if c.Username != "" {
				name = c.Username
			}
			break
		}
		users = append(users, dto.OnlineUserDTO{
			UserID:   uid,
			Username: name,
		})
	}
	return users
}

// UpdateClientUsername updates the cached display name for all connections of a user.
func (h *Hub) UpdateClientUsername(userID, username string) {
	h.mu.Lock()
	if conns, ok := h.clients[userID]; ok {
		for c := range conns {
			c.Username = username
		}
	}
	h.mu.Unlock()
	if h.nats != nil && username != "" {
		_ = h.nats.SetPresence(userID, username, true)
		_ = h.nats.PublishPresence(userID, username, true)
	}
}
