package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"ws-ex/dto"
	"ws-ex/model"
)

// FriendService manages invites and the accepted friend list.
type FriendService struct {
	db  *gorm.DB
	hub *Hub
}

func NewFriendService(db *gorm.DB, hub *Hub) *FriendService {
	return &FriendService{db: db, hub: hub}
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

// SendRequest creates a pending invite from → to (by username or user id).
func (s *FriendService) SendRequest(fromID uint, username, toUserIDStr string) (*dto.FriendRequestDTO, error) {
	var to model.User
	switch {
	case username != "":
		if err := s.db.Where("username = ?", username).First(&to).Error; err != nil {
			return nil, errors.New("user not found")
		}
	case toUserIDStr != "":
		toID, err := parseUID(toUserIDStr)
		if err != nil {
			return nil, err
		}
		if err := s.db.First(&to, toID).Error; err != nil {
			return nil, errors.New("user not found")
		}
	default:
		return nil, errors.New("username or user_id is required")
	}
	if to.ID == fromID {
		return nil, errors.New("cannot invite yourself")
	}
	if s.AreFriends(fromID, to.ID) {
		return nil, errors.New("already friends")
	}

	var existing model.FriendRequest
	err := s.db.Where(
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
		u, ok := byID[id]
		if !ok {
			continue
		}
		online := false
		if s.hub != nil {
			_, online = s.hub.GetClient(uidStr(id))
		}
		out = append(out, dto.FriendUserDTO{
			UserID:   uidStr(id),
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

// RemoveFriend deletes the friendship pair.
func (s *FriendService) RemoveFriend(me, peer uint) error {
	if me == peer {
		return errors.New("invalid peer")
	}
	ua, ub := orderedPair(me, peer)
	res := s.db.Where("user_a_id = ? AND user_b_id = ?", ua, ub).Delete(&model.Friendship{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("not friends")
	}
	return nil
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
