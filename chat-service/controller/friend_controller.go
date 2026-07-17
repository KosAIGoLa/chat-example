package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
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
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid body"})
		return
	}
	req, err := ctrl.friends.SendRequest(ctrl.me(c), body.Username, body.UserID)
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

// RemoveFriend DELETE /api/friends/:user_id
func (ctrl *FriendController) RemoveFriend(c *gin.Context) {
	peer, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid user_id"})
		return
	}
	if err := ctrl.friends.RemoveFriend(ctrl.me(c), uint(peer)); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "friend removed"})
}
