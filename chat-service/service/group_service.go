package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ws-ex/dto"
	"ws-ex/model"
	"ws-ex/validate"
)

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
// name is required and validated; customID optional (validated or auto-generated).
func (s *GroupService) Create(ownerID uint, name, customID string) (*dto.GroupDTO, error) {
	name, err := validate.GroupName(name)
	if err != nil {
		return nil, err
	}
	id, err := validate.GroupID(customID, false)
	if err != nil {
		return nil, err
	}
	if id == "" {
		// Retry a few times on rare auto-id collision.
		for i := 0; i < 5; i++ {
			cand := genGroupID()
			var n int64
			s.db.Model(&model.Group{}).Where("id = ?", cand).Count(&n)
			if n == 0 {
				id = cand
				break
			}
		}
		if id == "" {
			return nil, errors.New("failed to allocate group id")
		}
	} else {
		var existing model.Group
		if err := s.db.Where("id = ?", id).First(&existing).Error; err == nil {
			return nil, errors.New("该群 ID 已被占用，请换一个")
		}
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
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
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
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return err
	}
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
	groupID, err = validate.GroupID(groupID, true)
	if err != nil {
		return nil, "", err
	}
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

// Search finds groups by fuzzy id / name (ILIKE). Used for join autocomplete.
// q empty returns recent groups (limited). Marks is_member for the caller.
func (s *GroupService) Search(userID uint, q string, limit int) ([]dto.GroupDTO, error) {
	q, err := validate.SearchQuery(q)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > validate.MaxLimit {
		limit = validate.DefaultLimit
	}

	var groups []model.Group
	tx := s.db.Model(&model.Group{})
	if q != "" {
		// Escape ILIKE wildcards in user input.
		like := "%" + escapeLike(q) + "%"
		tx = tx.Where("id ILIKE ? ESCAPE '\\' OR name ILIKE ? ESCAPE '\\'", like, like)
	}
	if err := tx.Order("created_at DESC").Limit(limit).Find(&groups).Error; err != nil {
		return nil, err
	}

	// Caller's membership set.
	memberOf := map[string]string{} // group_id → role
	if userID > 0 && len(groups) > 0 {
		ids := make([]string, 0, len(groups))
		for _, g := range groups {
			ids = append(ids, g.ID)
		}
		var rows []model.GroupMember
		_ = s.db.Where("user_id = ? AND group_id IN ?", userID, ids).Find(&rows).Error
		for _, r := range rows {
			memberOf[r.GroupID] = r.Role
		}
	}

	out := make([]dto.GroupDTO, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		role := memberOf[g.ID]
		d := s.toDTO(g, role, s.memberCount(g.ID))
		d.IsMember = role != ""
		out = append(out, *d)
	}

	// Prefer not-yet-joined matches, then name/id prefix closeness.
	if q != "" {
		ql := strings.ToLower(q)
		sort.SliceStable(out, func(i, j int) bool {
			ai, aj := out[i], out[j]
			if ai.IsMember != aj.IsMember {
				return !ai.IsMember // non-members first for join UI
			}
			si := scoreGroupMatch(ql, ai.ID, ai.Name)
			sj := scoreGroupMatch(ql, aj.ID, aj.Name)
			if si != sj {
				return si > sj
			}
			return strings.ToLower(ai.Name) < strings.ToLower(aj.Name)
		})
	}
	return out, nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// Higher = better match for autocomplete ranking.
func scoreGroupMatch(q, id, name string) int {
	idL := strings.ToLower(id)
	nameL := strings.ToLower(name)
	score := 0
	if idL == q || nameL == q {
		score += 100
	}
	if strings.HasPrefix(idL, q) {
		score += 40
	}
	if strings.HasPrefix(nameL, q) {
		score += 35
	}
	if strings.Contains(idL, q) {
		score += 15
	}
	if strings.Contains(nameL, q) {
		score += 10
	}
	return score
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

// MemberUserIDs returns durable member user ids as strings (for typing fan-out etc.).
func (s *GroupService) MemberUserIDs(groupID string) []string {
	var rows []model.GroupMember
	if err := s.db.Where("group_id = ?", groupID).Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, strconv.FormatUint(uint64(r.UserID), 10))
	}
	return out
}

// ListMembers returns the full durable roster with username, role, and online flag.
// isOnline may be nil (all offline). Sorted: owner first, then online, then username.
func (s *GroupService) ListMembers(groupID string, isOnline func(userID string) bool) ([]dto.GroupMemberDTO, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}
	if !s.Exists(groupID) {
		return nil, errors.New("group not found")
	}

	var rows []model.GroupMember
	if err := s.db.Where("group_id = ?", groupID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []dto.GroupMemberDTO{}, nil
	}

	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	var users []model.User
	_ = s.db.Where("id IN ?", ids).Find(&users).Error
	nameByID := make(map[uint]string, len(users))
	for _, u := range users {
		nameByID[u.ID] = u.Username
	}

	out := make([]dto.GroupMemberDTO, 0, len(rows))
	for _, r := range rows {
		uid := strconv.FormatUint(uint64(r.UserID), 10)
		name := nameByID[r.UserID]
		if name == "" {
			name = uid
		}
		role := r.Role
		if role == "" {
			role = model.GroupRoleMember
		}
		online := false
		if isOnline != nil {
			online = isOnline(uid)
		} else if s.hub != nil {
			online = s.hub.IsUserOnline(uid)
		}
		out = append(out, dto.GroupMemberDTO{
			UserID:   uid,
			Username: name,
			Role:     role,
			Online:   online,
		})
	}

	// owner → admin → member; online first within band.
	roleRank := func(role string) int {
		switch role {
		case model.GroupRoleOwner:
			return 0
		case model.GroupRoleAdmin:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i], out[j]
		if roleRank(ri.Role) != roleRank(rj.Role) {
			return roleRank(ri.Role) < roleRank(rj.Role)
		}
		if ri.Online != rj.Online {
			return ri.Online
		}
		return strings.ToLower(ri.Username) < strings.ToLower(rj.Username)
	})
	return out, nil
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
		Avatar:        g.Avatar,
		AvatarRev:     g.AvatarRev,
	}
}

// MemberRole returns durable role for user in group (empty if not a member).
func (s *GroupService) MemberRole(userID uint, groupID string) string {
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return ""
	}
	if g.OwnerUserID == userID {
		return model.GroupRoleOwner
	}
	var m model.GroupMember
	if err := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&m).Error; err != nil {
		return ""
	}
	if m.Role == "" {
		return model.GroupRoleMember
	}
	return m.Role
}

// CanManageGroup: owner or admin may edit name / avatar.
func (s *GroupService) CanManageGroup(userID uint, groupID string) bool {
	r := s.MemberRole(userID, groupID)
	return r == model.GroupRoleOwner || r == model.GroupRoleAdmin
}

// IsOwner reports whether userID is the group owner.
func (s *GroupService) IsOwner(userID uint, groupID string) bool {
	return s.MemberRole(userID, groupID) == model.GroupRoleOwner
}

// UpdateName renames the group (owner or admin).
func (s *GroupService) UpdateName(actorID uint, groupID, name string) (*dto.GroupDTO, error) {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
	}
	name, err = validate.GroupName(name)
	if err != nil {
		return nil, err
	}
	if !s.CanManageGroup(actorID, groupID) {
		return nil, errors.New("only owner or admin can rename the group")
	}
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found")
	}
	if err := s.db.Model(&g).Update("name", name).Error; err != nil {
		return nil, err
	}
	g.Name = name
	role := s.MemberRole(actorID, groupID)
	return s.toDTO(&g, role, s.memberCount(groupID)), nil
}

// SetAvatar updates group icon (owner or admin).
func (s *GroupService) SetAvatar(actorID uint, groupID, publicPath string) (*dto.GroupDTO, error) {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
	}
	if !s.CanManageGroup(actorID, groupID) {
		return nil, errors.New("only owner or admin can update the group icon")
	}
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found")
	}
	rev := g.AvatarRev + 1
	if err := s.db.Model(&g).Updates(map[string]interface{}{
		"avatar":     publicPath,
		"avatar_rev": rev,
	}).Error; err != nil {
		return nil, err
	}
	g.Avatar = publicPath
	g.AvatarRev = rev
	role := s.MemberRole(actorID, groupID)
	return s.toDTO(&g, role, s.memberCount(groupID)), nil
}

// SetMemberRole promotes/demotes a member (owner only).
// targetRole: admin | member. Cannot change owner; cannot set role to owner.
func (s *GroupService) SetMemberRole(actorID uint, groupID string, targetUserID uint, targetRole string) (*dto.GroupMemberDTO, error) {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
	}
	targetRole = strings.ToLower(strings.TrimSpace(targetRole))
	if targetRole != model.GroupRoleAdmin && targetRole != model.GroupRoleMember {
		return nil, errors.New("role must be admin or member")
	}
	if !s.IsOwner(actorID, groupID) {
		return nil, errors.New("only the group owner can change member roles")
	}
	if targetUserID == actorID {
		return nil, errors.New("cannot change your own role")
	}
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found")
	}
	if g.OwnerUserID == targetUserID {
		return nil, errors.New("cannot change the owner's role")
	}
	var m model.GroupMember
	if err := s.db.Where("group_id = ? AND user_id = ?", groupID, targetUserID).First(&m).Error; err != nil {
		return nil, errors.New("user is not a group member")
	}
	if err := s.db.Model(&m).Update("role", targetRole).Error; err != nil {
		return nil, err
	}
	m.Role = targetRole
	var u model.User
	_ = s.db.First(&u, targetUserID).Error
	name := u.Username
	uid := strconv.FormatUint(uint64(targetUserID), 10)
	if name == "" {
		name = uid
	}
	online := s.hub != nil && s.hub.IsUserOnline(uid)
	return &dto.GroupMemberDTO{
		UserID:   uid,
		Username: name,
		Role:     targetRole,
		Online:   online,
	}, nil
}

// ListAnnouncements returns all pinned announcements for a group (newest first).
func (s *GroupService) ListAnnouncements(groupID string) ([]dto.GroupAnnouncementDTO, error) {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
	}
	var rows []model.GroupAnnouncement
	if err := s.db.Where("group_id = ?", groupID).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.GroupAnnouncementDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAnnouncementDTO(r))
	}
	return out, nil
}

// AddAnnouncements pins one or more messages as group announcements (owner/admin).
// Duplicate message_ids are ignored (idempotent).
func (s *GroupService) AddAnnouncements(actorID uint, groupID string, items []dto.AddAnnouncementItem) ([]dto.GroupAnnouncementDTO, error) {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return nil, err
	}
	if !s.CanManageGroup(actorID, groupID) {
		return nil, errors.New("only owner or admin can set announcements")
	}
	if len(items) == 0 {
		return nil, errors.New("no messages to pin")
	}
	if len(items) > 50 {
		return nil, errors.New("too many announcements at once (max 50)")
	}
	var g model.Group
	if err := s.db.Where("id = ?", groupID).First(&g).Error; err != nil {
		return nil, errors.New("group not found")
	}
	setBy := strconv.FormatUint(uint64(actorID), 10)
	out := make([]dto.GroupAnnouncementDTO, 0, len(items))
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
		row := model.GroupAnnouncement{
			GroupID:      groupID,
			MessageID:    mid,
			Content:      content,
			ContentType:  ct,
			FromUserID:   strings.TrimSpace(it.FromUserID),
			FromUsername: strings.TrimSpace(it.FromUsername),
			SetByUserID:  setBy,
			MessageTS:    it.MessageTS,
		}
		// Upsert on (group_id, message_id)
		var existing model.GroupAnnouncement
		err := s.db.Where("group_id = ? AND message_id = ?", groupID, mid).First(&existing).Error
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
			out = append(out, toAnnouncementDTO(existing))
			continue
		}
		if err := s.db.Create(&row).Error; err != nil {
			continue
		}
		out = append(out, toAnnouncementDTO(row))
	}
	if len(out) == 0 {
		return nil, errors.New("failed to pin any announcement")
	}
	return out, nil
}

// RemoveAnnouncement unpins a message (owner/admin).
func (s *GroupService) RemoveAnnouncement(actorID uint, groupID, messageID string) error {
	groupID, err := validate.GroupID(groupID, true)
	if err != nil {
		return err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("message_id required")
	}
	if !s.CanManageGroup(actorID, groupID) {
		return errors.New("only owner or admin can remove announcements")
	}
	res := s.db.Where("group_id = ? AND message_id = ?", groupID, messageID).Delete(&model.GroupAnnouncement{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("announcement not found")
	}
	return nil
}

// NotifyAnnouncement pushes a WS event to group members (best-effort via hub room + durable members).
func (s *GroupService) NotifyAnnouncement(groupID, byUserID, action string, items []dto.GroupAnnouncementDTO, messageID string) {
	if s.hub == nil || groupID == "" {
		return
	}
	ev := dto.AnnouncementEvent{
		Type:      "group_announcement",
		Action:    action,
		GroupID:   groupID,
		ByUserID:  byUserID,
		Items:     items,
		MessageID: messageID,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	// Deliver once per durable member (all their tabs on this instance).
	for _, uid := range s.MemberUserIDs(groupID) {
		s.hub.DeliverToUser(uid, data)
	}
}

func toAnnouncementDTO(r model.GroupAnnouncement) dto.GroupAnnouncementDTO {
	return dto.GroupAnnouncementDTO{
		ID:           r.ID,
		GroupID:      r.GroupID,
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

// BroadcastMeetingNotice posts a system line into the group chat stream (meeting open/close).
func (s *GroupService) BroadcastMeetingNotice(groupID, text string) {
	if s.chatSvc == nil || strings.TrimSpace(text) == "" {
		return
	}
	s.chatSvc.BroadcastPlainGroupNotice(groupID, text)
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
