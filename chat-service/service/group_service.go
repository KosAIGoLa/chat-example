package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ws-ex/dto"
	"ws-ex/model"
)

var groupIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

// GroupService persists groups and durable memberships.
type GroupService struct {
	db      *gorm.DB
	hub     *Hub
	chatSvc *ChatService
}

func NewGroupService(db *gorm.DB, hub *Hub) *GroupService {
	return &GroupService{db: db, hub: hub}
}

// SetChatService links system notices (optional).
func (s *GroupService) SetChatService(cs *ChatService) {
	s.chatSvc = cs
}

func genGroupID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "g_" + hex.EncodeToString(b)
}

// Create creates a group; creator becomes owner and durable member.
func (s *GroupService) Create(ownerID uint, name, customID string) (*dto.GroupDTO, error) {
	name = strings.TrimSpace(name)
	customID = strings.TrimSpace(customID)

	var id string
	if customID != "" {
		if !groupIDPattern.MatchString(customID) {
			return nil, errors.New("group_id must be 3–64 chars: letters, digits, _ or -")
		}
		id = customID
	} else {
		id = genGroupID()
	}
	if name == "" {
		name = id
	}
	if len(name) > 100 {
		return nil, errors.New("name too long (max 100)")
	}

	var existing model.Group
	if err := s.db.Where("id = ?", id).First(&existing).Error; err == nil {
		return nil, errors.New("group id already exists")
	}

	g := model.Group{
		ID:          id,
		Name:        name,
		OwnerUserID: ownerID,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		m := model.GroupMember{
			GroupID: id,
			UserID:  ownerID,
			Role:    model.GroupRoleOwner,
		}
		return tx.Create(&m).Error
	}); err != nil {
		return nil, err
	}

	// Online connections join hub immediately.
	uid := strconv.FormatUint(uint64(ownerID), 10)
	if s.hub != nil {
		s.hub.JoinGroupAll(id, uid)
	}

	return s.toDTO(&g, model.GroupRoleOwner, 1), nil
}

// Join adds durable membership; fails if group does not exist or is dissolved.
func (s *GroupService) Join(userID uint, groupID string) (*dto.GroupDTO, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found (create it first or check the id)")
	}
	var n int64
	s.db.Model(&model.GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Count(&n)
	if n == 0 {
		m := model.GroupMember{
			GroupID: groupID,
			UserID:  userID,
			Role:    model.GroupRoleMember,
		}
		if err := s.db.Create(&m).Error; err != nil {
			return nil, err
		}
	}
	// Hub online join is handled by ChatService / controller.
	role := model.GroupRoleMember
	if g.OwnerUserID == userID {
		role = model.GroupRoleOwner
	}
	count := s.memberCount(groupID)
	return s.toDTO(&g, role, count), nil
}

// Leave removes durable membership. Owner must dissolve instead.
func (s *GroupService) Leave(userID uint, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		// Allow leaving hub-only ghost groups.
		_ = s.db.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&model.GroupMember{}).Error
		return nil
	}
	if g.OwnerUserID == userID {
		return errors.New("owner cannot leave — dissolve the group instead")
	}
	res := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&model.GroupMember{})
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// Dissolve deletes the group; only the owner may call this.
// Returns member user ids (string) that were in the group for live notify.
func (s *GroupService) Dissolve(ownerID uint, groupID string) (memberIDs []string, name string, err error) {
	groupID = strings.TrimSpace(groupID)
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, "", errors.New("group not found")
	}
	if g.OwnerUserID != ownerID {
		return nil, "", errors.New("only the owner can dissolve the group")
	}
	name = g.Name

	var members []model.GroupMember
	_ = s.db.Where("group_id = ?", groupID).Find(&members).Error
	memberIDs = make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, strconv.FormatUint(uint64(m.UserID), 10))
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&model.GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", groupID).Delete(&model.Group{}).Error
	}); err != nil {
		return nil, "", err
	}
	return memberIDs, name, nil
}

// ListMine returns groups the user belongs to.
func (s *GroupService) ListMine(userID uint) ([]dto.GroupDTO, error) {
	var memberships []model.GroupMember
	if err := s.db.Where("user_id = ?", userID).Find(&memberships).Error; err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return []dto.GroupDTO{}, nil
	}
	ids := make([]string, 0, len(memberships))
	roleBy := make(map[string]string, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.GroupID)
		roleBy[m.GroupID] = m.Role
	}
	var groups []model.Group
	if err := s.db.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]dto.GroupDTO, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		role := roleBy[g.ID]
		if role == "" {
			role = model.GroupRoleMember
		}
		out = append(out, *s.toDTO(g, role, s.memberCount(g.ID)))
	}
	return out, nil
}

// IsMember reports durable membership.
func (s *GroupService) IsMember(userID uint, groupID string) bool {
	var n int64
	s.db.Model(&model.GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Count(&n)
	return n > 0
}

// Exists reports whether a group row is present.
func (s *GroupService) Exists(groupID string) bool {
	var n int64
	s.db.Model(&model.Group{}).Where("id = ?", groupID).Count(&n)
	return n > 0
}

// Get returns group info for a member.
func (s *GroupService) Get(userID uint, groupID string) (*dto.GroupDTO, error) {
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found")
	}
	role := model.GroupRoleMember
	var m model.GroupMember
	if err := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&m).Error; err == nil {
		role = m.Role
	} else if g.OwnerUserID == userID {
		role = model.GroupRoleOwner
	} else {
		return nil, errors.New("not a member")
	}
	return s.toDTO(&g, role, s.memberCount(groupID)), nil
}

func (s *GroupService) memberCount(groupID string) int {
	var n int64
	s.db.Model(&model.GroupMember{}).Where("group_id = ?", groupID).Count(&n)
	return int(n)
}

func (s *GroupService) toDTO(g *model.Group, role string, count int) *dto.GroupDTO {
	var owner model.User
	_ = s.db.First(&owner, g.OwnerUserID).Error
	return &dto.GroupDTO{
		ID:            g.ID,
		Name:          g.Name,
		OwnerUserID:   strconv.FormatUint(uint64(g.OwnerUserID), 10),
		OwnerUsername: owner.Username,
		Role:          role,
		MemberCount:   count,
		CreatedAt:     g.CreatedAt.Unix(),
	}
}

// NotifyDissolved kicks hub members and pushes group_dissolved events.
func (s *GroupService) NotifyDissolved(groupID, name, byUserID string, memberIDs []string) {
	if s.hub == nil {
		return
	}
	ev := dto.GroupDissolvedEvent{
		Type:    "group_dissolved",
		GroupID: groupID,
		Name:    name,
		ByUser:  byUserID,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}

	// System notice in stream before members are gone (best-effort).
	if s.chatSvc != nil {
		s.chatSvc.BroadcastPlainGroupNotice(groupID, fmt.Sprintf("群「%s」已解散", name))
	}

	// Push event + force leave every local connection still in the group.
	for _, c := range s.hub.GetGroupMembers(groupID) {
		select {
		case c.Send <- data:
		default:
		}
		s.hub.LeaveGroup(groupID, c)
	}
	// Also notify offline members' other tabs if they were only durable members:
	for _, uid := range memberIDs {
		s.hub.DeliverToUser(uid, data)
	}
	log.Printf("[Group] dissolved %s by %s members=%d", groupID, byUserID, len(memberIDs))
}
