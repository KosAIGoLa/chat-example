package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
	"ws-ex/validate"
)

// FriendController handles friend invite / accept / list REST APIs.
type FriendController struct {
	friends *service.FriendService
}

func NewFriendController(friends *service.FriendService) *FriendController {
	return &FriendController{friends: friends}
}

func (ctrl *FriendController) me(c *gin.Context) uint {
	raw, _ := c.Get("user_id")
	return raw.(uint)
}

// ListFriends GET /api/friends
func (ctrl *FriendController) ListFriends(c *gin.Context) {
	list, err := ctrl.friends.ListFriends(ctrl.me(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"friends": list, "count": len(list)},
	})
}

// ListIncoming GET /api/friends/requests/incoming
func (ctrl *FriendController) ListIncoming(c *gin.Context) {
	list, err := ctrl.friends.ListIncoming(ctrl.me(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"requests": list, "count": len(list)},
	})
}

// ListOutgoing GET /api/friends/requests/outgoing
func (ctrl *FriendController) ListOutgoing(c *gin.Context) {
	list, err := ctrl.friends.ListOutgoing(ctrl.me(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"requests": list, "count": len(list)},
	})
}

// SendRequest POST /api/friends/request  {username} or {user_id}
func (ctrl *FriendController) SendRequest(c *gin.Context) {
	var body dto.FriendInviteRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: validate.JSONBody(err).Error()})
		return
	}
	username := ""
	userID := ""
	var err error
	if body.Username != "" {
		username, err = validate.Username(body.Username)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
	}
	if body.UserID != "" {
		userID, err = validate.UserIDStr(body.UserID, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
	}
	if username == "" && userID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "username or user_id is required"})
		return
	}
	req, err := ctrl.friends.SendRequest(ctrl.me(c), username, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	// Real-time notify invitee.
	ctrl.friends.PushFriendEvent(req.ToUserID, dto.FriendEvent{
		Action:       "request",
		RequestID:    req.ID,
		FromUserID:   req.FromUserID,
		FromUsername: req.FromUsername,
		ToUserID:     req.ToUserID,
		ToUsername:   req.ToUsername,
	})
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "invite sent", Data: req})
}

// AcceptRequest POST /api/friends/requests/:id/accept
func (ctrl *FriendController) AcceptRequest(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid id"})
		return
	}
	req, err := ctrl.friends.AcceptRequest(ctrl.me(c), uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	// Notify both sides so friend lists refresh.
	ev := dto.FriendEvent{
		Action:       "accepted",
		RequestID:    req.ID,
		FromUserID:   req.FromUserID,
		FromUsername: req.FromUsername,
		ToUserID:     req.ToUserID,
		ToUsername:   req.ToUsername,
	}
	ctrl.friends.PushFriendEvent(req.FromUserID, ev)
	ctrl.friends.PushFriendEvent(req.ToUserID, ev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "friend added", Data: req})
}

// RejectRequest POST /api/friends/requests/:id/reject
func (ctrl *FriendController) RejectRequest(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid id"})
		return
	}
	req, err := ctrl.friends.RejectRequest(ctrl.me(c), uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	ctrl.friends.PushFriendEvent(req.FromUserID, dto.FriendEvent{
		Action:       "rejected",
		RequestID:    req.ID,
		FromUserID:   req.FromUserID,
		FromUsername: req.FromUsername,
		ToUserID:     req.ToUserID,
		ToUsername:   req.ToUsername,
	})
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "request rejected", Data: req})
}

// RemoveFriend DELETE /api/friends/:user_id — 解除好友关系
// Also clears private chat history between the pair (server + both clients via event).
// Re-adding as friends starts with an empty conversation.
func (ctrl *FriendController) RemoveFriend(c *gin.Context) {
	peer, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid user_id"})
		return
	}
	me := ctrl.me(c)
	peerID, err := ctrl.friends.RemoveFriend(me, uint(peer))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	meStr := strconv.FormatUint(uint64(me), 10)
	ev := dto.FriendEvent{
		Type:       "friend_event",
		Action:     "removed",
		FromUserID: meStr,
		ToUserID:   peerID,
	}
	ctrl.friends.PushFriendEvent(meStr, ev)
	ctrl.friends.PushFriendEvent(peerID, ev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "friend removed; private history cleared"})
}

// ListBlacklist GET /api/friends/blacklist
func (ctrl *FriendController) ListBlacklist(c *gin.Context) {
	list, err := ctrl.friends.ListBlacklist(ctrl.me(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"blacklist": list, "count": len(list)},
	})
}

// BlockUser POST /api/friends/blacklist  {username|user_id}
// 拉黑：保留好友关系；好友列表隐藏对方；屏蔽私聊与邀请；取消拉黑后回到好友列表。
// （与「解除好友」不同：解除会删除 Friendship。）
func (ctrl *FriendController) BlockUser(c *gin.Context) {
	var body dto.BlockUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: validate.JSONBody(err).Error()})
		return
	}
	username := ""
	userID := ""
	var err error
	if body.Username != "" {
		username, err = validate.Username(body.Username)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
	}
	if body.UserID != "" {
		userID, err = validate.UserIDStr(body.UserID, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
	}
	if username == "" && userID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "username or user_id is required"})
		return
	}
	me := ctrl.me(c)
	entry, err := ctrl.friends.BlockUser(me, username, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	meStr := strconv.FormatUint(uint64(me), 10)
	ev := dto.FriendEvent{
		Type:       "friend_event",
		Action:     "blocked",
		FromUserID: meStr,
		ToUserID:   entry.UserID,
	}
	// Me: refresh friend list (peer hidden) + blacklist. Peer: no friendship loss.
	ctrl.friends.PushFriendEvent(meStr, ev)
	ctrl.friends.PushFriendEvent(entry.UserID, ev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "user blocked", Data: entry})
}

// UnblockUser DELETE /api/friends/blacklist/:user_id
// 取消拉黑：好友关系若仍在则重新出现在好友列表。
func (ctrl *FriendController) UnblockUser(c *gin.Context) {
	peer, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid user_id"})
		return
	}
	me := ctrl.me(c)
	if err := ctrl.friends.UnblockUser(me, uint(peer)); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	meStr := strconv.FormatUint(uint64(me), 10)
	peerStr := strconv.FormatUint(peer, 10)
	ev := dto.FriendEvent{
		Type:       "friend_event",
		Action:     "unblocked",
		FromUserID: meStr,
		ToUserID:   peerStr,
	}
	ctrl.friends.PushFriendEvent(meStr, ev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "user unblocked"})
}
