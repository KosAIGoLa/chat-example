package model

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a single WebSocket connection for a user.
type Client struct {
	UserID   string
	Username string
	Conn     *websocket.Conn
	Send     chan []byte
	Groups   map[string]struct{}
	// Crypto seals/opens entire WS frames when set (server ↔ client transport encryption).
	Crypto  FrameCrypto
	mu      sync.RWMutex
	closed  bool
	closeMu sync.Mutex
}

// NewClient creates a new Client for the given user id and websocket connection.
func NewClient(userID, username string, conn *websocket.Conn, crypto FrameCrypto) *Client {
	return &Client{
		UserID:   userID,
		Username: username,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Groups:   make(map[string]struct{}),
		Crypto:   crypto,
	}
}

// JoinGroup adds the client to a group.
func (c *Client) JoinGroup(groupID string) {
	c.mu.Lock()
	c.Groups[groupID] = struct{}{}
	c.mu.Unlock()
}

// LeaveGroup removes the client from a group.
func (c *Client) LeaveGroup(groupID string) {
	c.mu.Lock()
	delete(c.Groups, groupID)
	c.mu.Unlock()
}

// IsInGroup checks if the client is in a group.
func (c *Client) IsInGroup(groupID string) bool {
	c.mu.RLock()
	_, ok := c.Groups[groupID]
	c.mu.RUnlock()
	return ok
}

// GetGroups returns a slice of group ids the client belongs to.
func (c *Client) GetGroups() []string {
	c.mu.RLock()
	groups := make([]string, 0, len(c.Groups))
	for g := range c.Groups {
		groups = append(groups, g)
	}
	c.mu.RUnlock()
	return groups
}

// Close safely closes the client's send channel and websocket connection.
func (c *Client) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.Send)
	_ = c.Conn.Close()
}

// IsClosed returns whether the client has been closed.
func (c *Client) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

// WritePump pumps messages from the Send channel to the websocket connection.
// Every application frame is sealed with AES-GCM when Crypto is configured.
// On exit it closes the client so ReadPump unregisters promptly (no ghost online).
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			out := msg
			if c.Crypto != nil {
				sealed, err := c.Crypto.SealFrame(msg)
				if err != nil {
					log.Printf("[WS] seal frame for %s: %v", c.UserID, err)
					return
				}
				out = sealed
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readWait is how long the server waits for any client activity (app data or
// WebSocket pong). Client application heartbeats should be shorter than this.
const readWait = 60 * time.Second

// ReadPump reads messages from the websocket connection and forwards them
// to the provided handler. Encrypted frames are opened before the handler runs.
// Any successful read (including app-level {"type":"ping"}) extends the deadline.
func (c *Client) ReadPump(handler func([]byte)) {
	defer c.Close()

	_ = c.Conn.SetReadDeadline(time.Now().Add(readWait))
	// Protocol-level pong (reply to server Ping frames) also keeps the connection alive.
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(readWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}
		// Client is alive — extend idle deadline on every frame.
		_ = c.Conn.SetReadDeadline(time.Now().Add(readWait))

		plain := message
		if c.Crypto != nil {
			opened, err := c.Crypto.OpenFrame(message)
			if err != nil {
				log.Printf("[WS] open frame from %s: %v — dropping", c.UserID, err)
				continue
			}
			plain = opened
		}
		handler(plain)
	}
}
