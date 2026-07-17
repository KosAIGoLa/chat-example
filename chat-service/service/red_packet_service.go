package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ws-ex/dto"
	"ws-ex/model"
)

// RedPacketService creates and claims private/group/designated red packets.
type RedPacketService struct {
	db       *gorm.DB
	wallet   *WalletService
	friends  *FriendService
	groups   *GroupService
	hub      *Hub
	nats     *NATSService
	msgStore *MessageStore
}

func NewRedPacketService(
	db *gorm.DB,
	wallet *WalletService,
	friends *FriendService,
	groups *GroupService,
	hub *Hub,
	nats *NATSService,
	msgStore *MessageStore,
) *RedPacketService {
	return &RedPacketService{
		db: db, wallet: wallet, friends: friends, groups: groups,
		hub: hub, nats: nats, msgStore: msgStore,
	}
}

// Create deducts balance, stores packet, publishes chat message.
func (s *RedPacketService) Create(fromUID uint, fromName string, req dto.CreateRedPacketRequest) (*dto.RedPacketDTO, *dto.ChatMessageDTO, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Type))
	if kind != model.RedPacketTypePrivate && kind != model.RedPacketTypeGroup && kind != model.RedPacketTypeDesignated {
		return nil, nil, errors.New("type must be private, group, or designated")
	}
	if req.TotalAmount <= 0 {
		return nil, nil, errors.New("total_amount must be positive")
	}
	greeting := strings.TrimSpace(req.Greeting)
	if greeting == "" {
		greeting = "恭喜发财，大吉大利"
	}
	if len([]rune(greeting)) > 100 {
		return nil, nil, errors.New("greeting too long")
	}

	fromStr := fmt.Sprintf("%d", fromUID)
	totalCount := 1
	peerID := strings.TrimSpace(req.PeerID)
	groupID := strings.TrimSpace(req.GroupID)
	targetIDs := normalizeTargetIDs(req.TargetUserIDs)
	targetsJSON := ""

	switch kind {
	case model.RedPacketTypePrivate:
		if peerID == "" {
			return nil, nil, errors.New("peer_id is required for private red packet")
		}
		if peerID == fromStr {
			return nil, nil, errors.New("cannot send red packet to yourself")
		}
		if s.friends != nil {
			if s.friends.IsBlockedStr(fromStr, peerID) {
				return nil, nil, errors.New("cannot send: user is blocked")
			}
			if !s.friends.AreFriendsStr(fromStr, peerID) {
				return nil, nil, errors.New("can only send red packets to friends")
			}
		}
		totalCount = 1
	case model.RedPacketTypeGroup:
		if groupID == "" {
			return nil, nil, errors.New("group_id is required for group red packet")
		}
		if s.groups != nil && !s.groups.IsMember(fromUID, groupID) {
			return nil, nil, errors.New("not a group member")
		}
		totalCount = req.TotalCount
		if totalCount < 1 {
			return nil, nil, errors.New("total_count must be >= 1")
		}
		if req.TotalAmount < int64(totalCount) {
			return nil, nil, errors.New("total_amount must be >= total_count")
		}
	case model.RedPacketTypeDesignated:
		// 指定红包：群聊中指定成员均分领取
		if groupID == "" {
			return nil, nil, errors.New("group_id is required for designated red packet")
		}
		if s.groups != nil && !s.groups.IsMember(fromUID, groupID) {
			return nil, nil, errors.New("not a group member")
		}
		if len(targetIDs) < 1 {
			return nil, nil, errors.New("target_user_ids required for designated red packet")
		}
		if len(targetIDs) > 200 {
			return nil, nil, errors.New("too many designated recipients")
		}
		memberSet := map[string]struct{}{}
		if s.groups != nil {
			for _, mid := range s.groups.MemberUserIDs(groupID) {
				memberSet[mid] = struct{}{}
			}
		}
		for _, tid := range targetIDs {
			if tid == "" {
				return nil, nil, errors.New("invalid target_user_id")
			}
			if s.groups != nil {
				if _, ok := memberSet[tid]; !ok {
					return nil, nil, fmt.Errorf("user %s is not a group member", tid)
				}
			}
		}
		totalCount = len(targetIDs)
		if req.TotalAmount < int64(totalCount) {
			return nil, nil, errors.New("total_amount must be >= number of recipients")
		}
		b, err := json.Marshal(targetIDs)
		if err != nil {
			return nil, nil, errors.New("invalid target_user_ids")
		}
		targetsJSON = string(b)
	}

	packetID := NewMessageID()
	msgID := NewMessageID()
	now := time.Now()
	expires := now.Add(24 * time.Hour)

	var packet model.RedPacket
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if _, err := s.wallet.AdjustInTx(tx, fromUID, -req.TotalAmount, "send_red_packet", "red_packet", packetID); err != nil {
			return err
		}
		packet = model.RedPacket{
			ID:              packetID,
			Type:            kind,
			FromUserID:      fromStr,
			ToUserID:        peerID,
			GroupID:         groupID,
			TargetUserIDs:   targetsJSON,
			TotalAmount:     req.TotalAmount,
			TotalCount:      totalCount,
			RemainingAmount: req.TotalAmount,
			RemainingCount:  totalCount,
			Greeting:        greeting,
			Status:          model.RedPacketStatusOpen,
			MessageID:       msgID,
			ExpiresAt:       expires,
			CreatedAt:       now,
		}
		return tx.Create(&packet).Error
	})
	if err != nil {
		return nil, nil, err
	}

	// Chat card payload (plaintext; not encrypted).
	bodyMap := map[string]interface{}{
		"greeting":     greeting,
		"total_amount": req.TotalAmount,
		"total_count":  totalCount,
		"packet_type":  kind,
	}
	if kind == model.RedPacketTypeDesignated {
		bodyMap["target_user_ids"] = targetIDs
	}
	body, _ := json.Marshal(bodyMap)

	chatMsg := &dto.ChatMessageDTO{
		ID:          msgID,
		Type:        kind, // private | group | designated (normalized below)
		From:        fromStr,
		Content:     string(body),
		Timestamp:   now.Unix(),
		ContentType: "red_packet",
		RedPacketID: packetID,
		Encrypted:   false,
	}
	if kind == model.RedPacketTypePrivate {
		chatMsg.To = peerID
	} else {
		// group + designated both land in the group conversation
		chatMsg.To = groupID
		chatMsg.GroupID = groupID
		chatMsg.Type = "group"
	}

	// Assign seq + persist metadata (recall / ordering).
	if s.msgStore != nil {
		chatMsg.Seq = s.msgStore.NextSeq()
		to := chatMsg.To
		gid := chatMsg.GroupID
		if chatMsg.Type == "group" {
			to = ""
		}
		_ = s.msgStore.Save(&model.MessageRecord{
			ID:         msgID,
			Seq:        chatMsg.Seq,
			Type:       chatMsg.Type,
			FromUserID: fromStr,
			ToUserID:   to,
			GroupID:    gid,
			Timestamp:  chatMsg.Timestamp,
		})
	}

	// Publish for real-time + JetStream + offline path.
	var pubErr error
	if kind == model.RedPacketTypePrivate {
		pubErr = s.nats.PublishPrivate(chatMsg)
		if data, err := json.Marshal(chatMsg); err == nil {
			s.hub.DeliverToUser(fromStr, data)
		}
	} else {
		pubErr = s.nats.PublishGroup(chatMsg)
		if data, err := json.Marshal(chatMsg); err == nil {
			s.hub.DeliverToUser(fromStr, data)
		}
	}
	if pubErr != nil {
		log.Printf("[RedPacket] publish chat failed: %v", pubErr)
	}

	return s.toDTO(&packet, fromStr, nil), chatMsg, nil
}

// Claim grabs a packet. Private = full; group = random; designated = equal among targets.
func (s *RedPacketService) Claim(userID uint, username, packetID string) (*dto.ClaimRedPacketResponse, error) {
	uidStr := fmt.Sprintf("%d", userID)
	if username == "" {
		username = uidStr
	}

	var result dto.ClaimRedPacketResponse
	var notify dto.RedPacketClaimedEvent
	var packet model.RedPacket

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", packetID).First(&packet).Error; err != nil {
			return errors.New("red packet not found")
		}

		// Lazy expiry / refund.
		if packet.Status == model.RedPacketStatusOpen && time.Now().After(packet.ExpiresAt) {
			if err := s.refundInTx(tx, &packet); err != nil {
				return err
			}
			return errors.New("red packet expired")
		}
		if packet.Status != model.RedPacketStatusOpen {
			return fmt.Errorf("red packet is %s", packet.Status)
		}
		if packet.RemainingCount <= 0 || packet.RemainingAmount <= 0 {
			return errors.New("red packet already finished")
		}

		// Already claimed?
		var existing model.RedPacketClaim
		if err := tx.Where("packet_id = ? AND user_id = ?", packetID, uidStr).First(&existing).Error; err == nil {
			return errors.New("already claimed")
		}

		// Authorization.
		switch packet.Type {
		case model.RedPacketTypePrivate:
			if packet.ToUserID != uidStr {
				return errors.New("only the recipient can claim this red packet")
			}
		case model.RedPacketTypeGroup:
			if s.groups != nil && !s.groups.IsMember(userID, packet.GroupID) {
				return errors.New("not a group member")
			}
		case model.RedPacketTypeDesignated:
			if s.groups != nil && !s.groups.IsMember(userID, packet.GroupID) {
				return errors.New("not a group member")
			}
			targets := parseTargetIDs(packet.TargetUserIDs)
			if !containsStr(targets, uidStr) {
				return errors.New("you are not a designated recipient")
			}
		default:
			return errors.New("invalid packet type")
		}

		amount := int64(0)
		switch {
		case packet.Type == model.RedPacketTypePrivate || packet.RemainingCount == 1:
			amount = packet.RemainingAmount
		case packet.Type == model.RedPacketTypeDesignated:
			// Equal split among remaining designated shares.
			amount = packet.RemainingAmount / int64(packet.RemainingCount)
			if amount < 1 {
				amount = 1
			}
			// Last share takes remainder.
			if packet.RemainingCount == 1 {
				amount = packet.RemainingAmount
			} else {
				// Leave at least 1 per remaining slot after this grab.
				minLeft := int64(packet.RemainingCount - 1)
				if packet.RemainingAmount-amount < minLeft {
					amount = packet.RemainingAmount - minLeft
				}
			}
		default:
			// Double-average method (WeChat-style luck red packet).
			max := 2 * packet.RemainingAmount / int64(packet.RemainingCount)
			if max < 1 {
				max = 1
			}
			amount = int64(rand.Intn(int(max))) + 1
			if amount > packet.RemainingAmount {
				amount = packet.RemainingAmount
			}
			minLeft := int64(packet.RemainingCount - 1)
			if packet.RemainingAmount-amount < minLeft {
				amount = packet.RemainingAmount - minLeft
			}
			if amount < 1 {
				amount = 1
			}
		}

		claim := model.RedPacketClaim{
			PacketID:  packetID,
			UserID:    uidStr,
			Username:  username,
			Amount:    amount,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&claim).Error; err != nil {
			return err
		}

		if _, err := s.wallet.AdjustInTx(tx, userID, amount, "claim_red_packet", "red_packet", packetID); err != nil {
			return err
		}

		packet.RemainingAmount -= amount
		packet.RemainingCount--
		if packet.RemainingCount <= 0 || packet.RemainingAmount <= 0 {
			packet.RemainingCount = 0
			packet.RemainingAmount = 0
			packet.Status = model.RedPacketStatusFinished
		}
		if err := tx.Save(&packet).Error; err != nil {
			return err
		}

		var u model.User
		_ = tx.Select("balance").First(&u, userID).Error
		bal := u.Balance

		result = dto.ClaimRedPacketResponse{
			PacketID:       packetID,
			Amount:         amount,
			RemainingCount: packet.RemainingCount,
			Finished:       packet.Status == model.RedPacketStatusFinished,
			Balance:        bal,
			Status:         packet.Status,
		}
		notify = dto.RedPacketClaimedEvent{
			Type:           "red_packet_claimed",
			PacketID:       packetID,
			UserID:         uidStr,
			Username:       username,
			Amount:         amount,
			RemainingCount: packet.RemainingCount,
			Finished:       result.Finished,
			Timestamp:      time.Now().Unix(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.broadcastClaim(packet, notify)
	return &result, nil
}

func (s *RedPacketService) refundInTx(tx *gorm.DB, packet *model.RedPacket) error {
	if packet.RemainingAmount <= 0 {
		packet.Status = model.RedPacketStatusExpired
		return tx.Save(packet).Error
	}
	fromUID, err := ParseUserID(packet.FromUserID)
	if err != nil {
		return err
	}
	if _, err := s.wallet.AdjustInTx(tx, fromUID, packet.RemainingAmount, "refund_red_packet", "red_packet", packet.ID); err != nil {
		return err
	}
	packet.RemainingAmount = 0
	packet.RemainingCount = 0
	packet.Status = model.RedPacketStatusRefunded
	return tx.Save(packet).Error
}

// Get returns packet detail + claims.
func (s *RedPacketService) Get(viewerUID uint, packetID string) (*dto.RedPacketDTO, error) {
	var packet model.RedPacket
	if err := s.db.Where("id = ?", packetID).First(&packet).Error; err != nil {
		return nil, errors.New("red packet not found")
	}
	// Lazy expire on read.
	if packet.Status == model.RedPacketStatusOpen && time.Now().After(packet.ExpiresAt) {
		_ = s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", packetID).First(&packet).Error; err != nil {
				return err
			}
			if packet.Status == model.RedPacketStatusOpen && time.Now().After(packet.ExpiresAt) {
				return s.refundInTx(tx, &packet)
			}
			return nil
		})
		_ = s.db.Where("id = ?", packetID).First(&packet).Error
	}

	var claims []model.RedPacketClaim
	_ = s.db.Where("packet_id = ?", packetID).Order("created_at ASC").Find(&claims).Error
	return s.toDTO(&packet, fmt.Sprintf("%d", viewerUID), claims), nil
}

func (s *RedPacketService) toDTO(p *model.RedPacket, viewer string, claims []model.RedPacketClaim) *dto.RedPacketDTO {
	out := &dto.RedPacketDTO{
		ID:              p.ID,
		Type:            p.Type,
		FromUserID:      p.FromUserID,
		ToUserID:        p.ToUserID,
		GroupID:         p.GroupID,
		TotalAmount:     p.TotalAmount,
		TotalCount:      p.TotalCount,
		RemainingAmount: p.RemainingAmount,
		RemainingCount:  p.RemainingCount,
		Greeting:        p.Greeting,
		Status:          p.Status,
		MessageID:       p.MessageID,
		ExpiresAt:       p.ExpiresAt.Unix(),
		CreatedAt:       p.CreatedAt.Unix(),
	}
	if p.Type == model.RedPacketTypeDesignated {
		targets := parseTargetIDs(p.TargetUserIDs)
		out.TargetUserIDs = targets
		can := containsStr(targets, viewer)
		out.CanClaim = &can
	}
	for _, c := range claims {
		out.Claims = append(out.Claims, dto.RedPacketClaimDTO{
			UserID:    c.UserID,
			Username:  c.Username,
			Amount:    c.Amount,
			CreatedAt: c.CreatedAt.Unix(),
		})
		if c.UserID == viewer {
			out.MyClaimAmount = c.Amount
		}
	}
	return out
}

func (s *RedPacketService) broadcastClaim(packet model.RedPacket, ev dto.RedPacketClaimedEvent) {
	data, err := json.Marshal(ev)
	if err != nil || s.hub == nil {
		return
	}
	switch packet.Type {
	case model.RedPacketTypePrivate:
		s.hub.DeliverToUser(packet.FromUserID, data)
		s.hub.DeliverToUser(packet.ToUserID, data)
	case model.RedPacketTypeGroup, model.RedPacketTypeDesignated:
		for _, c := range s.hub.GetGroupMembers(packet.GroupID) {
			select {
			case c.Send <- data:
			default:
			}
		}
		// Also notify sender if not in hub group room.
		s.hub.DeliverToUser(packet.FromUserID, data)
	}
}

func normalizeTargetIDs(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func parseTargetIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		// Fallback: comma-separated
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				ids = append(ids, p)
			}
		}
		return ids
	}
	return normalizeTargetIDs(ids)
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
