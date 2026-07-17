package controller

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"ws-ex/dto"
	"ws-ex/model"
	"ws-ex/service"
	"ws-ex/validate"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ChatController handles WebSocket and REST endpoints for chat.
type ChatController struct {
	hub     *service.Hub
	chatSvc *service.ChatService
	natsSvc *service.NATSService
	crypto  *service.MsgCrypto
}

// NewChatController creates a new ChatController.
func NewChatController(hub *service.Hub, chatSvc *service.ChatService, natsSvc *service.NATSService) *ChatController {
	return &ChatController{hub: hub, chatSvc: chatSvc, natsSvc: natsSvc}
}

// ChatService returns the underlying chat service (history recall helpers).
func (ctrl *ChatController) ChatService() *service.ChatService {
	return ctrl.chatSvc
}

// SetCrypto attaches the message encryption helper (for client key distribution).
func (ctrl *ChatController) SetCrypto(c *service.MsgCrypto) {
	ctrl.crypto = c
}

// GetCryptoKey returns the AES key used to encrypt WebSocket frames and content.
// The raw key is JWT-wrapped (AES-GCM) and field names are obfuscated — never plaintext.
// GET /api/crypto/key  (auth required)
func (ctrl *ChatController) GetCryptoKey(c *gin.Context) {
	if ctrl.crypto == nil {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{
			Code:    503,
			Message: "message crypto not configured",
		})
		return
	}
	// Prefer raw Authorization bearer so wrap key matches what the browser holds.
	token := bearerToken(c)
	if token == "" {
		// Fallback: query token (rarely used for this endpoint).
		token = c.Query("token")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
			Code:    401,
			Message: "authorization required to unwrap crypto key",
		})
		return
	}
	wrapped, err := ctrl.crypto.WrapKeyForToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{
			Code:    500,
			Message: "failed to wrap crypto key",
		})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		// Opaque envelope only — no algorithm / scheme labels for attackers to read.
		Data: dto.CryptoKeyResponse{
			V: service.WrapFormatVersion,
			W: wrapped,
		},
	})
}

// bearerToken extracts the JWT from Authorization: Bearer <token>.
func bearerToken(c *gin.Context) string {
	h := strings.TrimSpace(c.GetHeader("Authorization"))
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and registers the client.
// GET /ws?token=<JWT>
func (ctrl *ChatController) HandleWebSocket(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
			Code:    401,
			Message: "user not authenticated",
		})
		return
	}
	userID := strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = userID
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] upgrade error for user %s (%s): %v", usernameStr, userID, err)
		return
	}

	// Wire AES frame crypto so all server→client / client→server WS payloads are sealed.
	var frameCrypto model.FrameCrypto
	if ctrl.crypto != nil {
		frameCrypto = ctrl.crypto
	}
	client := model.NewClient(userID, usernameStr, conn, frameCrypto)
	ctrl.hub.Register(client)

	log.Printf("[WS] user %s (%s) connected", usernameStr, userID)

	go client.WritePump()

	defer func() {
		ctrl.hub.Unregister(client)
		log.Printf("[WS] user %s (%s) disconnected", usernameStr, userID)
	}()

	client.ReadPump(func(message []byte) {
		ctrl.chatSvc.HandleMessage(client, message)
	})
}

// JoinGroup handles a REST request to join a group.
// POST /api/groups/join?group_id=<groupID>
func (ctrl *ChatController) JoinGroup(c *gin.Context) {
	userIDRaw, _ := c.Get("user_id")
	userID := strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
	groupID, err := validate.GroupID(c.Query("group_id"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	// rejoin=1 / silent=1: membership restore after reconnect — no "加入到群" notice.
	announce := c.Query("rejoin") != "1" && c.Query("silent") != "1"
	// Durable join + online hub join; first-time join may broadcast "加入到群".
	if err := ctrl.chatSvc.JoinGroupForUser(userID, groupID, announce); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "joined group successfully",
		Data:    gin.H{"group_id": groupID, "user_id": userID},
	})
}

// LeaveGroup handles a REST request to leave a group.
// POST /api/groups/leave?group_id=<groupID>
func (ctrl *ChatController) LeaveGroup(c *gin.Context) {
	userIDRaw, _ := c.Get("user_id")
	userID := strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
	groupID, err := validate.GroupID(c.Query("group_id"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	// silent=1: membership cleanup without "退出群" notice.
	announce := c.Query("silent") != "1"
	// Durable leave + hub leave; full leave may broadcast "退出群".
	if err := ctrl.chatSvc.LeaveGroupForUser(userID, groupID, announce); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "left group successfully",
		Data:    gin.H{"group_id": groupID, "user_id": userID},
	})
}

// GetGroupMembers returns the full durable member roster for a group:
// user_id, username, role (owner|member), online (WS connected).
// Includes the caller so the UI can show "我 · 群主 · 在线".
// GET /api/groups/:group_id/members
func (ctrl *ChatController) GetGroupMembers(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	// Only members may list the roster.
	if userIDRaw, ok := c.Get("user_id"); ok {
		uid := userIDRaw.(uint)
		if gs := ctrl.chatSvc.Groups(); gs != nil && !gs.IsMember(uid, groupID) {
			c.JSON(http.StatusForbidden, dto.APIResponseDTO{
				Code: 403, Message: "not a group member",
			})
			return
		}
	}

	// Build online set: local hub (live WS only) + fresh remote presence.
	onlineSet := map[string]bool{}
	for _, uid := range ctrl.hub.GetOnlineUsers() {
		onlineSet[uid] = true
	}
	if ctrl.natsSvc != nil {
		if remote, err := ctrl.natsSvc.GetRemoteOnlineUsers(45 * time.Second); err == nil {
			for _, u := range remote {
				if u.UserID != "" {
					onlineSet[u.UserID] = true
				}
			}
		}
	}

	members, err := ctrl.chatSvc.ListGroupMembers(groupID, func(uid string) bool {
		return onlineSet[uid]
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	if members == nil {
		members = []dto.GroupMemberDTO{}
	}

	onlineCount := 0
	for _, m := range members {
		if m.Online {
			onlineCount++
		}
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"group_id":     groupID,
			"members":      members,
			"count":        len(members),
			"online_count": onlineCount,
		},
	})
}

// GetOnlineUsers returns currently online users (user_id + username).
// Local Hub is the source of truth (actual WS connections only).
// The caller is never included.
// GET /api/users/online
func (ctrl *ChatController) GetOnlineUsers(c *gin.Context) {
	me := ""
	if userIDRaw, ok := c.Get("user_id"); ok {
		me = strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
	}

	// Hub = real connections on this instance (never includes offline ghosts).
	byID := make(map[string]dto.OnlineUserDTO)
	for _, u := range ctrl.hub.GetOnlineUserInfos(me) {
		byID[u.UserID] = u
	}

	// Merge only remote-instance presence; never re-add local KV leftovers.
	// maxAge 45s matches presence heartbeat (30s) + small slack.
	if remote, err := ctrl.natsSvc.GetRemoteOnlineUsers(45 * time.Second); err == nil {
		for _, u := range remote {
			if u.UserID == me {
				continue
			}
			if _, exists := byID[u.UserID]; exists {
				continue
			}
			byID[u.UserID] = u
		}
	}

	users := make([]dto.OnlineUserDTO, 0, len(byID))
	for _, u := range byID {
		users = append(users, u)
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data:    gin.H{"online_users": users, "count": len(users)},
	})
}

// GetPresence returns a user's presence status from KV.
// GET /api/presence/:user_id
func (ctrl *ChatController) GetPresence(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "user_id is required"})
		return
	}

	p, err := ctrl.natsSvc.GetPresence(userID)
	if err != nil {
		c.JSON(http.StatusOK, dto.APIResponseDTO{
			Code:    200,
			Message: "user is offline",
			Data:    dto.PresenceDTO{UserID: userID, Online: false},
		})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: p})
}

// GetAllPresence returns the presence status of all online users from KV.
// GET /api/presence
func (ctrl *ChatController) GetAllPresence(c *gin.Context) {
	presences, err := ctrl.natsSvc.GetAllPresence()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data:    gin.H{"presences": presences, "count": len(presences)},
	})
}

// GetMessageHistory retrieves message history from JetStream (up to stream MaxAge ≈ 6 months).
//
// Group:   GET /api/history?type=group&group_id=<id>&count=50
// Private: GET /api/history?type=private&peer_id=<userID>&count=50
//
// Cursors:
//   - since_seq / since_ts  → only newer messages (incremental sync)
//   - before_seq / before_ts → only older messages (scroll-up pagination)
//
// Response includes has_more so the client can keep loading older pages.
func (ctrl *ChatController) GetMessageHistory(c *gin.Context) {
	count, err := validate.Limit(c.Query("count"), 80, 500)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	var messages []dto.ChatMessageDTO

	sinceTS, err := validate.NonNegInt64(c.Query("since_ts"), "since_ts")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	sinceSeq, err := validate.NonNegInt64(c.Query("since_seq"), "since_seq")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	beforeSeq, err := validate.NonNegInt64(c.Query("before_seq"), "before_seq")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	beforeTS, err := validate.NonNegInt64(c.Query("before_ts"), "before_ts")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	// Pull budget: never full multi-month dump. Enough for page + filter headroom.
	// before_seq needs a wider window; latest/delta stay small and fast.
	pullLimit := count * 4
	if beforeSeq > 0 || beforeTS > 0 {
		pullLimit = count * 20
		if pullLimit < 400 {
			pullLimit = 400
		}
	} else if sinceSeq > 0 || sinceTS > 0 {
		pullLimit = count * 8
		if pullLimit < 200 {
			pullLimit = 200
		}
	}
	if pullLimit > 2500 {
		pullLimit = 2500
	}
	if pullLimit < count {
		pullLimit = count
	}

	histType, err := validate.HistoryType(c.Query("type"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	switch histType {
	case "private":
		peerID, err := validate.PeerID(c.Query("peer_id"), true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
		userIDRaw, _ := c.Get("user_id")
		myID := strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
		// Private scans two inboxes then filters to the pair — budget handled inside.
		messages, err = ctrl.natsSvc.GetPrivateHistory(myID, peerID, pullLimit)
		// After unfriend, hide history at-or-before cut-off (re-friend starts clean).
		if err == nil && ctrl.chatSvc != nil {
			messages = ctrl.chatSvc.FilterPrivateAfterCutoff(myID, peerID, messages)
		}

	case "group":
		groupID, err := validate.GroupID(c.Query("group_id"), true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
		messages, err = ctrl.natsSvc.GetMessageHistory(fmt.Sprintf("chat.group.%s", groupID), pullLimit)

	default:
		// Backward-compatible subject filter — only allow known chat subjects.
		subject := validate.CleanSingleLine(c.Query("subject"))
		if subject == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
				Code:    400,
				Message: "type=private|group (with peer_id/group_id) or subject is required",
			})
			return
		}
		if !strings.HasPrefix(subject, "chat.private.") && !strings.HasPrefix(subject, "chat.group.") {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid subject"})
			return
		}
		if len(subject) > 128 {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "subject too long"})
			return
		}
		messages, err = ctrl.natsSvc.GetMessageHistory(subject, pullLimit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	if messages == nil {
		messages = []dto.ChatMessageDTO{}
	}
	// Fill seq from Postgres for JetStream payloads that predate seq-on-wire,
	// then sort + filter so since_seq / before_seq work for backfilled records.
	messages = ctrl.chatSvc.EnrichSeq(messages)
	service.SortChatMessages(messages)

	// Mutually exclusive cursors: before_* for older pages; since_* for new deltas.
	if beforeSeq > 0 || beforeTS > 0 {
		messages = service.FilterBeforeSeq(messages, beforeSeq, beforeTS)
	} else if sinceSeq > 0 {
		messages = service.FilterSinceSeq(messages, sinceSeq)
	} else if sinceTS > 0 {
		filtered := make([]dto.ChatMessageDTO, 0, len(messages))
		for _, m := range messages {
			if m.Timestamp > sinceTS {
				filtered = append(filtered, m)
			}
		}
		messages = filtered
	}

	// Cap to page size: always the latest `count` of the filtered set
	// (for before_seq this is the newest slice of older history — correct for prepend).
	hasMore := false
	if len(messages) > count {
		hasMore = true
		messages = messages[len(messages)-count:]
	}

	// Mask recalled messages for history viewers.
	messages = ctrl.chatSvc.ApplyRecalls(messages)
	maxSeq := service.MaxSeq(messages)
	minSeq := service.MinSeq(messages)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"messages":   messages,
			"count":      len(messages),
			"max_seq":    maxSeq,
			"min_seq":    minSeq,
			"since_seq":  sinceSeq,
			"before_seq": beforeSeq,
			"has_more":   hasMore,
		},
	})
}
