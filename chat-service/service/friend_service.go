package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ws-ex/dto"
	"ws-ex/model"
)

// FriendService manages invites and the accepted friend list.
type FriendService struct {
	db      *gorm.DB
	hub     *Hub
	offline *OfflineService
	nats    *NATSService
	store   *MessageStore
}

func NewFriendService(db *gorm.DB, hub *Hub) *FriendService {
	return &FriendService{db: db, hub: hub}
}

// SetOffline wires offline inbox cleanup on unfriend.
func (s *FriendService) SetOffline(o *OfflineService) {
	s.offline = o
}

// SetNATS wires JetStream private history purge on unfriend.
func (s *FriendService) SetNATS(ns *NATSService) {
	s.nats = ns
}

// SetMessageStore wires message metadata cleanup on unfriend.
func (s *FriendService) SetMessageStore(ms *MessageStore) {
	s.store = ms
}

func uidStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func parseUID(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user id")
	}
	return uint(n), nil
}

// orderedPair returns (min, max) user ids for a unique friendship row.
func orderedPair(a, b uint) (uint, uint) {
	if a < b {
		return a, b
	}
	return b, a
}

// AreFriends reports whether two users have an accepted friendship.
func (s *FriendService) AreFriends(a, b uint) bool {
	if a == b {
		return false
	}
	ua, ub := orderedPair(a, b)
	var n int64
	s.db.Model(&model.Friendship{}).Where("user_a_id = ? AND user_b_id = ?", ua, ub).Count(&n)
	return n > 0
}

// AreFriendsStr is AreFriends with string user ids (WS layer).
func (s *FriendService) AreFriendsStr(a, b string) bool {
	ua, err1 := parseUID(a)
	ub, err2 := parseUID(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return s.AreFriends(ua, ub)
}

// resolveUser finds a user by username or numeric id string.
func (s *FriendService) resolveUser(username, userIDStr string) (*model.User, error) {
	var u model.User
	switch {
	case username != "":
		if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
			return nil, errors.New("user not found")
		}
	case userIDStr != "":
		id, err := parseUID(userIDStr)
		if err != nil {
			return nil, err
		}
		if err := s.db.First(&u, id).Error; err != nil {
			return nil, errors.New("user not found")
		}
	default:
		return nil, errors.New("username or user_id is required")
	}
	return &u, nil
}

// IsBlocked reports whether either side has blocked the other.
func (s *FriendService) IsBlocked(a, b uint) bool {
	if a == b {
		return false
	}
	var n int64
	s.db.Model(&model.Blacklist{}).
		Where("(user_id = ? AND blocked_user_id = ?) OR (user_id = ? AND blocked_user_id = ?)", a, b, b, a).
		Count(&n)
	return n > 0
}

// IsBlockedStr is IsBlocked with string user ids.
func (s *FriendService) IsBlockedStr(a, b string) bool {
	ua, err1 := parseUID(a)
	ub, err2 := parseUID(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return s.IsBlocked(ua, ub)
}

// IBlocked reports whether me blocked peer (one-way).
func (s *FriendService) IBlocked(me, peer uint) bool {
	var n int64
	s.db.Model(&model.Blacklist{}).Where("user_id = ? AND blocked_user_id = ?", me, peer).Count(&n)
	return n > 0
}

// SendRequest creates a pending invite from → to (by username or user id).
func (s *FriendService) SendRequest(fromID uint, username, toUserIDStr string) (*dto.FriendRequestDTO, error) {
	to, err := s.resolveUser(username, toUserIDStr)
	if err != nil {
		return nil, err
	}
	if to.ID == fromID {
		return nil, errors.New("cannot invite yourself")
	}
	if s.IsBlocked(fromID, to.ID) {
		return nil, errors.New("cannot invite: user is blocked")
	}
	if s.AreFriends(fromID, to.ID) {
		return nil, errors.New("already friends")
	}

	var existing model.FriendRequest
	err = s.db.Where(
		"((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)) AND status = ?",
		fromID, to.ID, to.ID, fromID, model.FriendPending,
	).First(&existing).Error
	if err == nil {
		return nil, errors.New("a pending request already exists")
	}

	var prev model.FriendRequest
	if err := s.db.Where("from_user_id = ? AND to_user_id = ?", fromID, to.ID).First(&prev).Error; err == nil {
		if prev.Status == model.FriendAccepted {
			return nil, errors.New("already friends")
		}
		prev.Status = model.FriendPending
		if err := s.db.Save(&prev).Error; err != nil {
			return nil, err
		}
		return s.toRequestDTO(&prev)
	}

	req := model.FriendRequest{
		FromUserID: fromID,
		ToUserID:   to.ID,
		Status:     model.FriendPending,
	}
	if err := s.db.Create(&req).Error; err != nil {
		return nil, err
	}
	return s.toRequestDTO(&req)
}

// AcceptRequest accepts a pending invite addressed to me.
func (s *FriendService) AcceptRequest(me uint, requestID uint) (*dto.FriendRequestDTO, error) {
	var req model.FriendRequest
	if err := s.db.First(&req, requestID).Error; err != nil {
		return nil, errors.New("request not found")
	}
	if req.ToUserID != me {
		return nil, errors.New("not your request to accept")
	}
	if req.Status != model.FriendPending {
		return nil, fmt.Errorf("request is already %s", req.Status)
	}
	if s.IsBlocked(me, req.FromUserID) {
		return nil, errors.New("cannot accept: user is blocked")
	}
	req.Status = model.FriendAccepted
	if err := s.db.Save(&req).Error; err != nil {
		return nil, err
	}
	ua, ub := orderedPair(req.FromUserID, req.ToUserID)
	var n int64
	s.db.Model(&model.Friendship{}).Where("user_a_id = ? AND user_b_id = ?", ua, ub).Count(&n)
	if n == 0 {
		if err := s.db.Create(&model.Friendship{UserAID: ua, UserBID: ub}).Error; err != nil {
			return nil, err
		}
	}
	_ = s.db.Model(&model.FriendRequest{}).
		Where("from_user_id = ? AND to_user_id = ? AND status = ?", req.ToUserID, req.FromUserID, model.FriendPending).
		Update("status", model.FriendAccepted).Error

	return s.toRequestDTO(&req)
}

// RejectRequest rejects a pending invite addressed to me.
func (s *FriendService) RejectRequest(me uint, requestID uint) (*dto.FriendRequestDTO, error) {
	var req model.FriendRequest
	if err := s.db.First(&req, requestID).Error; err != nil {
		return nil, errors.New("request not found")
	}
	if req.ToUserID != me {
		return nil, errors.New("not your request to reject")
	}
	if req.Status != model.FriendPending {
		return nil, fmt.Errorf("request is already %s", req.Status)
	}
	req.Status = model.FriendRejected
	if err := s.db.Save(&req).Error; err != nil {
		return nil, err
	}
	return s.toRequestDTO(&req)
}

// ListFriends returns accepted friends with online flag.
// Peers I blocked are omitted here (they appear on the blacklist instead);
// friendship rows are kept so Unblock restores them to this list.
func (s *FriendService) ListFriends(me uint) ([]dto.FriendUserDTO, error) {
	var rows []model.Friendship
	if err := s.db.Where("user_a_id = ? OR user_b_id = ?", me, me).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		if r.UserAID == me {
			ids = append(ids, r.UserBID)
		} else {
			ids = append(ids, r.UserAID)
		}
	}
	if len(ids) == 0 {
		return []dto.FriendUserDTO{}, nil
	}
	// One-way: hide people I blocked (not people who blocked me).
	var blockedRows []model.Blacklist
	_ = s.db.Select("blocked_user_id").Where("user_id = ?", me).Find(&blockedRows).Error
	blocked := make(map[uint]bool, len(blockedRows))
	for _, b := range blockedRows {
		blocked[b.BlockedUserID] = true
	}
	var users []model.User
	if err := s.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]model.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]dto.FriendUserDTO, 0, len(ids))
	for _, id := range ids {
		if blocked[id] {
			continue
		}
		u, ok := byID[id]
		if !ok {
			continue
		}
		uid := uidStr(id)
		online := s.hub != nil && s.hub.IsUserOnline(uid)
		out = append(out, dto.FriendUserDTO{
			UserID:   uid,
			Username: u.Username,
			Online:   online,
		})
	}
	return out, nil
}

// ListIncoming returns pending requests where I am the target.
func (s *FriendService) ListIncoming(me uint) ([]dto.FriendRequestDTO, error) {
	var rows []model.FriendRequest
	if err := s.db.Where("to_user_id = ? AND status = ?", me, model.FriendPending).
		Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.mapRequests(rows)
}

// ListOutgoing returns pending requests I sent.
func (s *FriendService) ListOutgoing(me uint) ([]dto.FriendRequestDTO, error) {
	var rows []model.FriendRequest
	if err := s.db.Where("from_user_id = ? AND status = ?", me, model.FriendPending).
		Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.mapRequests(rows)
}

// RemoveFriend deletes the friendship pair, cleans request rows, and clears
// private conversation history so re-adding friends starts with an empty chat.
// Returns peer user id string for live notify.
func (s *FriendService) RemoveFriend(me, peer uint) (peerID string, err error) {
	if me == peer {
		return "", errors.New("invalid peer")
	}
	ua, ub := orderedPair(me, peer)
	res := s.db.Where("user_a_id = ? AND user_b_id = ?", ua, ub).Delete(&model.Friendship{})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", errors.New("not friends")
	}
	// Clear any friend_request rows between the pair so they can re-invite.
	_ = s.db.Where(
		"(from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)",
		me, peer, peer, me,
	).Delete(&model.FriendRequest{}).Error

	// Wipe private history between the pair (server + offline inbox).
	s.ClearPrivateConversation(me, peer)
	return uidStr(peer), nil
}

// ClearPrivateConversation marks a history cutoff and purges durable private data
// so neither side sees old messages after unfriend / re-friend.
func (s *FriendService) ClearPrivateConversation(a, b uint) {
	if s == nil || s.db == nil || a == 0 || b == 0 || a == b {
		return
	}
	ua, ub := orderedPair(a, b)
	cutAt := time.Now().Unix()
	row := model.PrivateConvCutoff{UserAID: ua, UserBID: ub, CutAt: cutAt}
	_ = s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_a_id"}, {Name: "user_b_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"cut_at"}),
	}).Create(&row).Error

	aStr, bStr := uidStr(a), uidStr(b)
	// Drop shared pins with the history wipe.
	_ = s.db.Where("user_a_id = ? AND user_b_id = ?", ua, ub).Delete(&model.PrivatePin{}).Error
	if s.offline != nil {
		s.offline.ClearBetween(aStr, bStr)
	}
	if s.nats != nil {
		s.nats.PurgePrivatePair(aStr, bStr)
	}
	if s.store != nil {
		s.store.DeletePrivatePair(aStr, bStr)
	}
}

// PrivateCutoffUnix returns CutAt for the pair (0 if none).
func (s *FriendService) PrivateCutoffUnix(a, b string) int64 {
	if s == nil || s.db == nil {
		return 0
	}
	ua, err1 := parseUID(a)
	ub, err2 := parseUID(b)
	if err1 != nil || err2 != nil || ua == ub {
		return 0
	}
	min, max := orderedPair(ua, ub)
	var row model.PrivateConvCutoff
	if err := s.db.Where("user_a_id = ? AND user_b_id = ?", min, max).First(&row).Error; err != nil {
		return 0
	}
	return row.CutAt
}

// BlockUser adds peer to my blacklist.
// Friendship is KEPT (unlike RemoveFriend). Pending invites between the pair are cancelled.
// Private messaging is blocked while blacklisted; ListFriends hides the peer until Unblock.
func (s *FriendService) BlockUser(me uint, username, userIDStr string) (*dto.BlacklistUserDTO, error) {
	peer, err := s.resolveUser(username, userIDStr)
	if err != nil {
		return nil, err
	}
	if peer.ID == me {
		return nil, errors.New("cannot block yourself")
	}
	if s.IBlocked(me, peer.ID) {
		return s.toBlacklistDTO(me, peer.ID)
	}
	// Keep friendship. Only cancel open invites so neither side can re-invite while blocked.
	_ = s.db.Where(
		"(from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)",
		me, peer.ID, peer.ID, me,
	).Delete(&model.FriendRequest{}).Error

	row := model.Blacklist{UserID: me, BlockedUserID: peer.ID}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &dto.BlacklistUserDTO{
		UserID:    uidStr(peer.ID),
		Username:  peer.Username,
		CreatedAt: row.CreatedAt.Unix(),
	}, nil
}

// UnblockUser removes peer from my blacklist.
// If they were friends, they reappear on ListFriends (friendship was never deleted).
func (s *FriendService) UnblockUser(me, peer uint) error {
	if me == peer {
		return errors.New("invalid peer")
	}
	res := s.db.Where("user_id = ? AND blocked_user_id = ?", me, peer).Delete(&model.Blacklist{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("user is not in your blacklist")
	}
	return nil
}

// ListBlacklist returns users I blocked.
func (s *FriendService) ListBlacklist(me uint) ([]dto.BlacklistUserDTO, error) {
	var rows []model.Blacklist
	if err := s.db.Where("user_id = ?", me).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []dto.BlacklistUserDTO{}, nil
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BlockedUserID)
	}
	var users []model.User
	_ = s.db.Where("id IN ?", ids).Find(&users).Error
	byID := make(map[uint]model.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]dto.BlacklistUserDTO, 0, len(rows))
	for _, r := range rows {
		name := uidStr(r.BlockedUserID)
		if u, ok := byID[r.BlockedUserID]; ok {
			name = u.Username
		}
		out = append(out, dto.BlacklistUserDTO{
			UserID:    uidStr(r.BlockedUserID),
			Username:  name,
			CreatedAt: r.CreatedAt.Unix(),
		})
	}
	return out, nil
}

func (s *FriendService) toBlacklistDTO(me, peerID uint) (*dto.BlacklistUserDTO, error) {
	var row model.Blacklist
	if err := s.db.Where("user_id = ? AND blocked_user_id = ?", me, peerID).First(&row).Error; err != nil {
		return nil, err
	}
	var u model.User
	_ = s.db.First(&u, peerID).Error
	name := u.Username
	if name == "" {
		name = uidStr(peerID)
	}
	return &dto.BlacklistUserDTO{
		UserID:    uidStr(peerID),
		Username:  name,
		CreatedAt: row.CreatedAt.Unix(),
	}, nil
}

func (s *FriendService) mapRequests(rows []model.FriendRequest) ([]dto.FriendRequestDTO, error) {
	out := make([]dto.FriendRequestDTO, 0, len(rows))
	for i := range rows {
		d, err := s.toRequestDTO(&rows[i])
		if err != nil {
			continue
		}
		out = append(out, *d)
	}
	return out, nil
}

func (s *FriendService) toRequestDTO(req *model.FriendRequest) (*dto.FriendRequestDTO, error) {
	var fromU, toU model.User
	_ = s.db.First(&fromU, req.FromUserID).Error
	_ = s.db.First(&toU, req.ToUserID).Error
	return &dto.FriendRequestDTO{
		ID:           req.ID,
		FromUserID:   uidStr(req.FromUserID),
		FromUsername: fromU.Username,
		ToUserID:     uidStr(req.ToUserID),
		ToUsername:   toU.Username,
		Status:       req.Status,
		CreatedAt:    req.CreatedAt.Unix(),
	}, nil
}

// PushFriendEvent delivers a friend_event to all local connections of userID.
func (s *FriendService) PushFriendEvent(userID string, ev dto.FriendEvent) {
	if s.hub == nil {
		return
	}
	ev.Type = "friend_event"
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.hub.DeliverToUser(userID, data)
}

// ListPrivatePins returns all pins for the private conversation with peer (newest first).
// Either friend may list; blocked pairs still may list existing pins (messaging is blocked separately).
func (s *FriendService) ListPrivatePins(me uint, peerIDStr string) ([]dto.PrivatePinDTO, error) {
	peer, err := parseUID(peerIDStr)
	if err != nil {
		return nil, err
	}
	if me == peer {
		return nil, errors.New("invalid peer")
	}
	if !s.AreFriends(me, peer) {
		return nil, errors.New("not friends")
	}
	ua, ub := orderedPair(me, peer)
	var rows []model.PrivatePin
	if err := s.db.Where("user_a_id = ? AND user_b_id = ?", ua, ub).
		Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.PrivatePinDTO, 0, len(rows))
	peerStr := uidStr(peer)
	for _, r := range rows {
		out = append(out, toPrivatePinDTO(r, peerStr))
	}
	return out, nil
}

// AddPrivatePins pins one or more private messages (either friend). Duplicate message_ids upsert.
func (s *FriendService) AddPrivatePins(me uint, peerIDStr string, items []dto.AddPrivatePinItem) ([]dto.PrivatePinDTO, error) {
	peer, err := parseUID(peerIDStr)
	if err != nil {
		return nil, err
	}
	if me == peer {
		return nil, errors.New("invalid peer")
	}
	if !s.AreFriends(me, peer) {
		return nil, errors.New("not friends")
	}
	if s.IsBlocked(me, peer) {
		return nil, errors.New("blocked")
	}
	if len(items) == 0 {
		return nil, errors.New("no messages to pin")
	}
	if len(items) > 50 {
		return nil, errors.New("too many pins at once (max 50)")
	}
	ua, ub := orderedPair(me, peer)
	setBy := uidStr(me)
	peerStr := uidStr(peer)
	out := make([]dto.PrivatePinDTO, 0, len(items))
	for _, it := range items {
		mid := strings.TrimSpace(it.MessageID)
		if mid == "" || len(mid) > 36 {
			continue
		}
		content := strings.TrimSpace(it.Content)
		if content == "" {
			content = "[消息]"
		}
		if len(content) > 2000 {
			content = content[:2000]
		}
		ct := strings.TrimSpace(it.ContentType)
		if ct == "" {
			ct = "text"
		}
		row := model.PrivatePin{
			UserAID:      ua,
			UserBID:      ub,
			MessageID:    mid,
			Content:      content,
			ContentType:  ct,
			FromUserID:   strings.TrimSpace(it.FromUserID),
			FromUsername: strings.TrimSpace(it.FromUsername),
			SetByUserID:  setBy,
			MessageTS:    it.MessageTS,
		}
		var existing model.PrivatePin
		err := s.db.Where("user_a_id = ? AND user_b_id = ? AND message_id = ?", ua, ub, mid).First(&existing).Error
		if err == nil {
			_ = s.db.Model(&existing).Updates(map[string]interface{}{
				"content":        row.Content,
				"content_type":   row.ContentType,
				"from_user_id":   row.FromUserID,
				"from_username":  row.FromUsername,
				"set_by_user_id": setBy,
				"message_ts":     row.MessageTS,
			}).Error
			existing.Content = row.Content
			existing.ContentType = row.ContentType
			existing.FromUserID = row.FromUserID
			existing.FromUsername = row.FromUsername
			existing.SetByUserID = setBy
			existing.MessageTS = row.MessageTS
			out = append(out, toPrivatePinDTO(existing, peerStr))
			continue
		}
		if err := s.db.Create(&row).Error; err != nil {
			continue
		}
		out = append(out, toPrivatePinDTO(row, peerStr))
	}
	if len(out) == 0 {
		return nil, errors.New("failed to pin any message")
	}
	return out, nil
}

// RemovePrivatePin unpins one message (either friend).
func (s *FriendService) RemovePrivatePin(me uint, peerIDStr, messageID string) error {
	peer, err := parseUID(peerIDStr)
	if err != nil {
		return err
	}
	if me == peer {
		return errors.New("invalid peer")
	}
	if !s.AreFriends(me, peer) {
		return errors.New("not friends")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("message_id required")
	}
	ua, ub := orderedPair(me, peer)
	res := s.db.Where("user_a_id = ? AND user_b_id = ? AND message_id = ?", ua, ub, messageID).
		Delete(&model.PrivatePin{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("pin not found")
	}
	return nil
}

// NotifyPrivatePin pushes private_pin WS events to both peers (peer_id relative to each recipient).
func (s *FriendService) NotifyPrivatePin(me uint, peerIDStr, action string, items []dto.PrivatePinDTO, messageID string) {
	if s.hub == nil {
		return
	}
	peer, err := parseUID(peerIDStr)
	if err != nil {
		return
	}
	meStr := uidStr(me)
	peerStr := uidStr(peer)
	// Deliver to me: peer is peer
	s.pushPrivatePinEvent(meStr, peerStr, meStr, action, items, messageID)
	// Deliver to peer: peer is me
	s.pushPrivatePinEvent(peerStr, meStr, meStr, action, items, messageID)
}

func (s *FriendService) pushPrivatePinEvent(toUserID, peerID, byUserID, action string, items []dto.PrivatePinDTO, messageID string) {
	// Clone items with peer_id set for this recipient.
	cloned := make([]dto.PrivatePinDTO, len(items))
	for i, it := range items {
		it.PeerID = peerID
		cloned[i] = it
	}
	ev := dto.PrivatePinEvent{
		Type:      "private_pin",
		Action:    action,
		PeerID:    peerID,
		ByUserID:  byUserID,
		Items:     cloned,
		MessageID: messageID,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.hub.DeliverToUser(toUserID, data)
}

func toPrivatePinDTO(r model.PrivatePin, peerID string) dto.PrivatePinDTO {
	return dto.PrivatePinDTO{
		ID:           r.ID,
		PeerID:       peerID,
		MessageID:    r.MessageID,
		Content:      r.Content,
		ContentType:  r.ContentType,
		FromUserID:   r.FromUserID,
		FromUsername: r.FromUsername,
		SetByUserID:  r.SetByUserID,
		MessageTS:    r.MessageTS,
		CreatedAt:    r.CreatedAt.Unix(),
	}
}
