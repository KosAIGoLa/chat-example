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
// It integrates with NATS KV for cluster-wide presence management.
type Hub struct {
	clients   map[string]*model.Client            // userID -> client
	groups    map[string]map[string]*model.Client // groupID -> userID -> client
	nats      *NATSService                        // for KV presence updates
	presenceT *time.Ticker                        // presence heartbeat ticker
	mu        sync.RWMutex
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*model.Client),
		groups:  make(map[string]map[string]*model.Client),
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
	h.mu.RLock()
	clients := make([]*model.Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if err := h.nats.SetPresence(c.UserID, c.Username, true); err != nil {
			log.Printf("[Hub] failed to refresh presence for %s: %v", c.UserID, err)
		}
	}
}

// Register adds a client to the hub, sets their presence in KV, and broadcasts online.
func (h *Hub) Register(client *model.Client) {
	h.mu.Lock()
	// Replace an existing connection for the same user (e.g. re-login).
	if old, ok := h.clients[client.UserID]; ok && old != client {
		old.Close()
	}
	h.clients[client.UserID] = client
	h.mu.Unlock()
	if h.nats != nil {
		if err := h.nats.SetPresence(client.UserID, client.Username, true); err != nil {
			log.Printf("[Hub] failed to set presence for %s: %v", client.UserID, err)
		}
		// Notify all instances so online lists refresh immediately.
		if err := h.nats.PublishPresence(client.UserID, client.Username, true); err != nil {
			log.Printf("[Hub] failed to publish online presence for %s: %v", client.UserID, err)
		}
	}
}

// Unregister removes a client from the hub and all its groups, and clears their presence in KV.
func (h *Hub) Unregister(client *model.Client) {
	h.mu.Lock()
	// Only mutate hub state if this is still the active connection for the user
	// (avoids racing a re-login that already replaced us).
	current, stillCurrent := h.clients[client.UserID]
	isActive := stillCurrent && current == client
	affectedGroups := make([]string, 0)
	if isActive {
		delete(h.clients, client.UserID)
		for _, groupID := range client.GetGroups() {
			if members, ok := h.groups[groupID]; ok {
				// Only drop membership if this exact connection holds the seat.
				if members[client.UserID] == client {
					delete(members, client.UserID)
					if len(members) == 0 {
						delete(h.groups, groupID)
					}
					affectedGroups = append(affectedGroups, groupID)
				}
			}
		}
	}
	h.mu.Unlock()

	if isActive && h.nats != nil {
		if err := h.nats.SetPresence(client.UserID, client.Username, false); err != nil {
			log.Printf("[Hub] failed to clear presence for %s: %v", client.UserID, err)
		}
		// Notify all instances so offline status refreshes immediately.
		if err := h.nats.PublishPresence(client.UserID, client.Username, false); err != nil {
			log.Printf("[Hub] failed to publish offline presence for %s: %v", client.UserID, err)
		}
	}
	// Push updated rosters so group member lists drop the offline user.
	for _, gid := range affectedGroups {
		h.broadcastGroupMembers(gid)
	}
}

// BroadcastAll sends a raw message payload to every local WebSocket client.
func (h *Hub) BroadcastAll(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		select {
		case client.Send <- data:
		default:
			log.Printf("[Hub] client %s buffer full, dropping broadcast", client.UserID)
		}
	}
}

// GetClient returns a client by user id.
func (h *Hub) GetClient(userID string) (*model.Client, bool) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()
	return client, ok
}

// JoinGroup adds a client to a group and notifies members of the new roster.
func (h *Hub) JoinGroup(groupID string, client *model.Client) {
	h.mu.Lock()
	if _, ok := h.groups[groupID]; !ok {
		h.groups[groupID] = make(map[string]*model.Client)
	}
	h.groups[groupID][client.UserID] = client
	h.mu.Unlock()
	client.JoinGroup(groupID)
	h.broadcastGroupMembers(groupID)
}

// LeaveGroup removes a client from a group and notifies remaining members.
func (h *Hub) LeaveGroup(groupID string, client *model.Client) {
	h.mu.Lock()
	if members, ok := h.groups[groupID]; ok {
		delete(members, client.UserID)
		if len(members) == 0 {
			delete(h.groups, groupID)
		}
	}
	h.mu.Unlock()
	client.LeaveGroup(groupID)
	h.broadcastGroupMembers(groupID)
}

// broadcastGroupMembers pushes a personalized member list to each member
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

// GetGroupMembers returns all clients in a group on this instance.
func (h *Hub) GetGroupMembers(groupID string) []*model.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[groupID]
	if !ok {
		return nil
	}
	clients := make([]*model.Client, 0, len(members))
	for _, c := range members {
		clients = append(clients, c)
	}
	return clients
}

// GetGroupMemberInfos returns id+username for members currently in a group on this instance.
// excludeUserID, when non-empty, is omitted (typically the caller themselves).
func (h *Hub) GetGroupMemberInfos(groupID string, excludeUserID string) []dto.OnlineUserDTO {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members, ok := h.groups[groupID]
	if !ok {
		return []dto.OnlineUserDTO{}
	}
	users := make([]dto.OnlineUserDTO, 0, len(members))
	for _, c := range members {
		if excludeUserID != "" && c.UserID == excludeUserID {
			continue
		}
		name := c.Username
		if name == "" {
			name = c.UserID
		}
		users = append(users, dto.OnlineUserDTO{
			UserID:   c.UserID,
			Username: name,
		})
	}
	return users
}

// GetOnlineUsers returns the user IDs of all connected clients on this instance.
func (h *Hub) GetOnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]string, 0, len(h.clients))
	for uid := range h.clients {
		users = append(users, uid)
	}
	return users
}

// GetOnlineUserInfos returns id+username for every local connected client.
// This is the source of truth for who is actually online on this instance.
// excludeUserID, when non-empty, is omitted from the list (typically the caller).
func (h *Hub) GetOnlineUserInfos(excludeUserID string) []dto.OnlineUserDTO {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]dto.OnlineUserDTO, 0, len(h.clients))
	for _, c := range h.clients {
		if excludeUserID != "" && c.UserID == excludeUserID {
			continue
		}
		name := c.Username
		if name == "" {
			name = c.UserID
		}
		users = append(users, dto.OnlineUserDTO{
			UserID:   c.UserID,
			Username: name,
		})
	}
	return users
}

// UpdateClientUsername updates the cached display name for a connected user.
func (h *Hub) UpdateClientUsername(userID, username string) {
	h.mu.Lock()
	if c, ok := h.clients[userID]; ok {
		c.Username = username
	}
	h.mu.Unlock()
	if h.nats != nil && username != "" {
		_ = h.nats.SetPresence(userID, username, true)
		_ = h.nats.PublishPresence(userID, username, true)
	}
}
