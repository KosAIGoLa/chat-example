package controller

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"ws-ex/dto"
	"ws-ex/model"
	"ws-ex/service"
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
// GET /api/crypto/key  (auth required)
func (ctrl *ChatController) GetCryptoKey(c *gin.Context) {
	if ctrl.crypto == nil {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{
			Code:    503,
			Message: "message crypto not configured",
		})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: dto.CryptoKeyResponse{
			Algorithm: ctrl.crypto.Algorithm(),
			Key:       ctrl.crypto.KeyBase64(),
			Version:   ctrl.crypto.Version(),
		},
	})
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
	groupID := c.Query("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: "group_id is required",
		})
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
	groupID := c.Query("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: "group_id is required",
		})
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

// GetGroupMembers returns users currently in a group (connected + joined).
// The caller is excluded so the list never contains yourself.
// GET /api/groups/:group_id/members
func (ctrl *ChatController) GetGroupMembers(c *gin.Context) {
	groupID := c.Param("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: "group_id is required",
		})
		return
	}
	me := ""
	if userIDRaw, ok := c.Get("user_id"); ok {
		me = strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
	}
	members := ctrl.hub.GetGroupMemberInfos(groupID, me)
	if members == nil {
		members = []dto.OnlineUserDTO{}
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"group_id": groupID,
			"members":  members,
			"count":    len(members),
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
	if remote, err := ctrl.natsSvc.GetRemoteOnlineUsers(90 * time.Second); err == nil {
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

// GetMessageHistory retrieves recent message history from JetStream.
//
// Group:   GET /api/history?type=group&group_id=<id>&count=50
// Private: GET /api/history?type=private&peer_id=<userID>&count=50
// Legacy:  GET /api/history?subject=chat.group.<id>&count=50
func (ctrl *ChatController) GetMessageHistory(c *gin.Context) {
	count := 50
	if n := c.Query("count"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 && v <= 500 {
			count = v
		}
	}

	var (
		messages []dto.ChatMessageDTO
		err      error
	)

	switch c.Query("type") {
	case "private":
		peerID := c.Query("peer_id")
		if peerID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "peer_id is required for private history"})
			return
		}
		userIDRaw, _ := c.Get("user_id")
		myID := strconv.FormatUint(uint64(userIDRaw.(uint)), 10)
		messages, err = ctrl.natsSvc.GetPrivateHistory(myID, peerID, count)

	case "group":
		groupID := c.Query("group_id")
		if groupID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id is required for group history"})
			return
		}
		messages, err = ctrl.natsSvc.GetMessageHistory(fmt.Sprintf("chat.group.%s", groupID), count)

	default:
		// Backward-compatible subject filter.
		subject := c.Query("subject")
		if subject == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
				Code:    400,
				Message: "type=private|group (with peer_id/group_id) or subject is required",
			})
			return
		}
		messages, err = ctrl.natsSvc.GetMessageHistory(subject, count)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	if messages == nil {
		messages = []dto.ChatMessageDTO{}
	}
	// Mask recalled messages for history viewers.
	messages = ctrl.chatSvc.ApplyRecalls(messages)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data:    gin.H{"messages": messages, "count": len(messages)},
	})
}
