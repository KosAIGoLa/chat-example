package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

// LiveKitController issues tokens and helps signal calls / meetings over the chat hub.
type LiveKitController struct {
	lk       *service.LiveKitService
	meetings *service.MeetingService
	hub      *service.Hub
	friends  *service.FriendService
	groups   *service.GroupService
}

func NewLiveKitController(
	lk *service.LiveKitService,
	hub *service.Hub,
	friends *service.FriendService,
	groups *service.GroupService,
	meetings *service.MeetingService,
) *LiveKitController {
	return &LiveKitController{
		lk:       lk,
		hub:      hub,
		friends:  friends,
		groups:   groups,
		meetings: meetings,
	}
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
// Prefer POST /api/livekit/meeting for group conference mode.
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
// Relays invite/accept/reject/end/cancel over WebSocket (hub).
// Private 1:1 only for ring flow; group conference should use /livekit/meeting.
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
		// Legacy path: still fan-out, but clients treat group as meeting (no ring).
		if body.GroupID == "" {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id required"})
			return
		}
		for _, client := range ctrl.hub.GetGroupMembers(body.GroupID) {
			if client.UserID == uidStr {
				continue
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

// MeetingAction POST /api/livekit/meeting
// Group conference: start | join | leave | end (meeting mode, not private ring-call).
func (ctrl *LiveKitController) MeetingAction(c *gin.Context) {
	if ctrl.lk == nil || !ctrl.lk.Enabled() {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{
			Code: 503, Message: "livekit not configured",
		})
		return
	}
	if ctrl.meetings == nil {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{
			Code: 503, Message: "meeting service not configured",
		})
		return
	}

	var body dto.MeetingActionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid body"})
		return
	}
	uid, uidStr, username := ctrl.me(c)
	groupID := strings.TrimSpace(body.GroupID)
	if groupID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id is required"})
		return
	}
	if ctrl.groups != nil && !ctrl.groups.IsMember(uid, groupID) {
		c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "not a group member"})
		return
	}

	action := strings.ToLower(strings.TrimSpace(body.Action))
	switch action {
	case "start":
		ctrl.meetingStart(c, groupID, uidStr, username, body.Media)
	case "join":
		ctrl.meetingJoin(c, groupID, uidStr, username)
	case "leave":
		ctrl.meetingLeave(c, groupID, uidStr, username)
	case "end":
		ctrl.meetingEnd(c, groupID, uidStr, username)
	default:
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code: 400, Message: "action must be start, join, leave, or end",
		})
	}
}

// GetMeeting GET /api/livekit/meeting/:group_id
func (ctrl *LiveKitController) GetMeeting(c *gin.Context) {
	uid, _, _ := ctrl.me(c)
	groupID := strings.TrimSpace(c.Param("group_id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "group_id required"})
		return
	}
	if ctrl.groups != nil && !ctrl.groups.IsMember(uid, groupID) {
		c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "not a group member"})
		return
	}
	if ctrl.meetings == nil {
		c.JSON(http.StatusOK, dto.APIResponseDTO{
			Code: 200, Message: "success",
			Data: dto.MeetingStatus{Active: false, GroupID: groupID},
		})
		return
	}
	m := ctrl.meetings.Get(groupID)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data:    meetingToStatus(m, groupID),
	})
}

func (ctrl *LiveKitController) meetingStart(c *gin.Context, groupID, uidStr, username, media string) {
	media = strings.ToLower(strings.TrimSpace(media))
	if media != "video" {
		media = "audio"
	}
	room := service.GroupRoomName(groupID)
	m, created, err := ctrl.meetings.Start(groupID, room, media, uidStr, username)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	// If joining an already-open meeting, use its media mode.
	if !created && m != nil {
		media = m.Media
		room = m.Room
	}

	token, err := ctrl.lk.MintToken(uidStr, username, room, true, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	lkURL := ctrl.lk.ClientURL(c.Request)
	st := meetingToStatus(m, groupID)
	st.Token = token
	st.URL = lkURL
	st.Identity = uidStr
	st.Created = created

	if created {
		ctrl.broadcastMeeting(groupID, uidStr, username, "started", m)
		// Persist a group system line so members see "会议已开启 · 可加入" in history.
		ctrl.postMeetingNotice(groupID, username, media, true)
	} else {
		ctrl.broadcastMeeting(groupID, uidStr, username, "joined", m)
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: st})
}

// postMeetingNotice writes a system message into the group stream (best-effort).
func (ctrl *LiveKitController) postMeetingNotice(groupID, username, media string, started bool) {
	if ctrl.groups == nil {
		return
	}
	kind := "语音"
	if media == "video" {
		kind = "视讯"
	}
	name := username
	if name == "" {
		name = "成员"
	}
	var text string
	if started {
		text = fmt.Sprintf("%s 开启了群%s会议，成员可点击「加入会议」一起沟通", name, kind)
	} else {
		text = fmt.Sprintf("群%s会议已结束", kind)
	}
	ctrl.groups.BroadcastMeetingNotice(groupID, text)
}

func (ctrl *LiveKitController) meetingJoin(c *gin.Context, groupID, uidStr, username string) {
	m, err := ctrl.meetings.Join(groupID, uidStr, username)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	token, err := ctrl.lk.MintToken(uidStr, username, m.Room, true, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	lkURL := ctrl.lk.ClientURL(c.Request)
	st := meetingToStatus(m, groupID)
	st.Token = token
	st.URL = lkURL
	st.Identity = uidStr

	ctrl.broadcastMeeting(groupID, uidStr, username, "joined", m)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: st})
}

func (ctrl *LiveKitController) meetingLeave(c *gin.Context, groupID, uidStr, username string) {
	m, ended := ctrl.meetings.Leave(groupID, uidStr)
	if ended {
		ctrl.broadcastMeeting(groupID, uidStr, username, "ended", m)
		media := ""
		if m != nil {
			media = m.Media
		}
		ctrl.postMeetingNotice(groupID, username, media, false)
		c.JSON(http.StatusOK, dto.APIResponseDTO{
			Code: 200, Message: "success",
			Data: dto.MeetingStatus{
				Active:  false,
				GroupID: groupID,
				Room:    m.Room,
				Media:   m.Media,
				Ended:   true,
			},
		})
		return
	}
	if m != nil {
		ctrl.broadcastMeeting(groupID, uidStr, username, "left", m)
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: meetingToStatus(m, groupID),
	})
}

func (ctrl *LiveKitController) meetingEnd(c *gin.Context, groupID, uidStr, username string) {
	m := ctrl.meetings.End(groupID)
	if m == nil {
		c.JSON(http.StatusOK, dto.APIResponseDTO{
			Code: 200, Message: "success",
			Data: dto.MeetingStatus{Active: false, GroupID: groupID, Ended: true},
		})
		return
	}
	ctrl.broadcastMeeting(groupID, uidStr, username, "ended", m)
	ctrl.postMeetingNotice(groupID, username, m.Media, false)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: dto.MeetingStatus{
			Active:  false,
			GroupID: groupID,
			Room:    m.Room,
			Media:   m.Media,
			Ended:   true,
		},
	})
}

func meetingToStatus(m *service.ActiveMeeting, groupID string) dto.MeetingStatus {
	if m == nil {
		return dto.MeetingStatus{Active: false, GroupID: groupID}
	}
	return dto.MeetingStatus{
		Active:           true,
		GroupID:          m.GroupID,
		Room:             m.Room,
		Media:            m.Media,
		StartedBy:        m.StartedBy,
		StartedByName:    m.StartedByName,
		StartedAt:        m.StartedAt,
		ParticipantCount: m.ParticipantCount,
	}
}

func (ctrl *LiveKitController) broadcastMeeting(
	groupID, from, fromName, action string,
	m *service.ActiveMeeting,
) {
	if ctrl.hub == nil {
		return
	}
	ev := dto.MeetingEvent{
		Type:      "meeting",
		Action:    action,
		From:      from,
		FromName:  fromName,
		GroupID:   groupID,
		Timestamp: time.Now().Unix(),
	}
	if m != nil {
		ev.Room = m.Room
		ev.Media = m.Media
		ev.ParticipantCount = m.ParticipantCount
	}
	if ev.Room == "" {
		ev.Room = service.GroupRoomName(groupID)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}

	// Notify ALL durable group members who are online — not only clients currently
	// viewing this group room. Otherwise members on the private tab never see
	// "join meeting" and cannot join the conference.
	seen := map[string]struct{}{}
	if ctrl.groups != nil {
		for _, uid := range ctrl.groups.MemberUserIDs(groupID) {
			if uid == from && (action == "started" || action == "joined") {
				continue // starter/joiner already has REST response
			}
			seen[uid] = struct{}{}
			ctrl.hub.DeliverToUser(uid, data)
		}
	}
	// Also fan-out to hub room members (covers any session not in durable list edge cases).
	for _, client := range ctrl.hub.GetGroupMembers(groupID) {
		if client.UserID == from && (action == "started" || action == "joined") {
			continue
		}
		if _, ok := seen[client.UserID]; ok {
			continue // already delivered per-user
		}
		select {
		case client.Send <- data:
		default:
		}
	}
}
