package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

// LiveKitController issues tokens and helps signal calls over the chat hub.
type LiveKitController struct {
	lk      *service.LiveKitService
	hub     *service.Hub
	friends *service.FriendService
	groups  *service.GroupService
}

func NewLiveKitController(
	lk *service.LiveKitService,
	hub *service.Hub,
	friends *service.FriendService,
	groups *service.GroupService,
) *LiveKitController {
	return &LiveKitController{lk: lk, hub: hub, friends: friends, groups: groups}
}

func (ctrl *LiveKitController) me(c *gin.Context) (uint, string, string) {
	raw, _ := c.Get("user_id")
	uid := raw.(uint)
	username, _ := c.Get("username")
	name, _ := username.(string)
	return uid, strconv.FormatUint(uint64(uid), 10), name
}

// CreateToken POST /api/livekit/token
// Authorizes private (must be friends) or group (must be member) and returns JWT + room.
func (ctrl *LiveKitController) CreateToken(c *gin.Context) {
	if ctrl.lk == nil || !ctrl.lk.Enabled() {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{
			Code: 503, Message: "livekit not configured",
		})
		return
	}

	var body dto.LiveKitTokenRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid body"})
		return
	}
	uid, uidStr, username := ctrl.me(c)
	callType := strings.ToLower(strings.TrimSpace(body.Type))
	if callType == "" {
		callType = "private"
	}

	var room, peerID, groupID string
	switch callType {
	case "private":
		peerID = strings.TrimSpace(body.PeerID)
		if peerID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "peer_id is required for private call"})
			return
		}
		if peerID == uidStr {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "cannot call yourself"})
			return
		}
		if ctrl.friends != nil {
			if ctrl.friends.IsBlockedStr(uidStr, peerID) {
				c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "cannot call: user is blocked"})
				return
			}
			if !ctrl.friends.AreFriendsStr(uidStr, peerID) {
				c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "can only call accepted friends"})
				return
			}
		}
		room = strings.TrimSpace(body.Room)
		if room == "" {
			room = service.PrivateRoomName(uidStr, peerID)
		}

	case "group":
		groupID = strings.TrimSpace(body.GroupID)
		if groupID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id is required for group meeting"})
			return
		}
		if ctrl.groups != nil && !ctrl.groups.IsMember(uid, groupID) {
			c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "not a group member"})
			return
		}
		room = strings.TrimSpace(body.Room)
		if room == "" {
			room = service.GroupRoomName(groupID)
		}

	default:
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "type must be private or group"})
		return
	}

	token, err := ctrl.lk.MintToken(uidStr, username, room, true, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}

	// Same origin as the page (e.g. ws://host:3000). Browser never hits :7880;
	// SPA nginx proxies /rtc → livekit:7880.
	lkURL := ctrl.lk.ClientURL(c.Request)

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: dto.LiveKitTokenResponse{
			Token:    token,
			URL:      lkURL,
			Room:     room,
			Identity: uidStr,
			CallType: callType,
			PeerID:   peerID,
			GroupID:  groupID,
		},
	})
}

// SignalCall POST /api/livekit/signal
// Relays invite/accept/reject/end/cancel over WebSocket (hub) to the target user(s).
func (ctrl *LiveKitController) SignalCall(c *gin.Context) {
	var body dto.CallEvent
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid body"})
		return
	}
	_, uidStr, username := ctrl.me(c)
	body.Type = "call"
	body.From = uidStr
	if body.FromName == "" {
		body.FromName = username
	}
	if body.Timestamp == 0 {
		body.Timestamp = time.Now().Unix()
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "action is required"})
		return
	}
	body.Action = action
	if body.Room == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "room is required"})
		return
	}

	data, err := json.Marshal(body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: "marshal failed"})
		return
	}

	// Fan-out targets.
	switch strings.ToLower(body.CallType) {
	case "group":
		if body.GroupID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id required"})
			return
		}
		for _, client := range ctrl.hub.GetGroupMembers(body.GroupID) {
			if client.UserID == uidStr {
				continue // don't echo invite to self
			}
			select {
			case client.Send <- data:
			default:
			}
		}
	default: // private
		to := strings.TrimSpace(body.To)
		if to == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "to is required for private call"})
			return
		}
		if ctrl.friends != nil {
			if ctrl.friends.IsBlockedStr(uidStr, to) {
				c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "user is blocked"})
				return
			}
		}
		ctrl.hub.DeliverToUser(to, data)
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "signaled", Data: body})
}
