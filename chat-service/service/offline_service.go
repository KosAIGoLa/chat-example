package service

import (
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ws-ex/model"
)

// OfflineService persists private messages for offline recipients and flushes on reconnect.
type OfflineService struct {
	db  *gorm.DB
	hub *Hub
}

func NewOfflineService(db *gorm.DB, hub *Hub) *OfflineService {
	return &OfflineService{db: db, hub: hub}
}

// Save stores or updates a pending private message by message id.
func (s *OfflineService) Save(id, toUserID, fromUserID, payload string) error {
	if s == nil || s.db == nil || id == "" || toUserID == "" {
		return nil
	}
	rec := model.OfflineMessage{
		ID:         id,
		ToUserID:   toUserID,
		FromUserID: fromUserID,
		Payload:    payload,
		CreatedAt:  time.Now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"to_user_id", "from_user_id", "payload"}),
	}).Create(&rec).Error
}

// Delete removes a delivered offline message.
func (s *OfflineService) Delete(id string) error {
	if s == nil || s.db == nil || id == "" {
		return nil
	}
	return s.db.Where("id = ?", id).Delete(&model.OfflineMessage{}).Error
}

// ClearBetween deletes offline private messages between two users (either direction).
func (s *OfflineService) ClearBetween(userA, userB string) {
	if s == nil || s.db == nil || userA == "" || userB == "" {
		return
	}
	_ = s.db.Where(
		"(from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)",
		userA, userB, userB, userA,
	).Delete(&model.OfflineMessage{}).Error
}

// Flush delivers all pending messages for userID (chronological) and deletes them.
// Returns how many messages were pushed.
func (s *OfflineService) Flush(userID string) int {
	if s == nil || s.db == nil || s.hub == nil || userID == "" {
		return 0
	}
	var rows []model.OfflineMessage
	if err := s.db.Where("to_user_id = ? AND delivered_at IS NULL", userID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		log.Printf("[Offline] list for %s: %v", userID, err)
		return 0
	}
	if len(rows) == 0 {
		return 0
	}

	now := time.Now()
	n := 0
	for _, row := range rows {
		ok := s.hub.DeliverToUser(userID, []byte(row.Payload))
		if !ok {
			// Still offline / buffer full — stop; keep remainder.
			log.Printf("[Offline] flush stopped for %s after %d (buffer/offline)", userID, n)
			break
		}
		_ = s.db.Model(&model.OfflineMessage{}).Where("id = ?", row.ID).
			Updates(map[string]interface{}{"delivered_at": now}).Error
		_ = s.db.Where("id = ?", row.ID).Delete(&model.OfflineMessage{}).Error
		n++
	}
	if n > 0 {
		log.Printf("[Offline] flushed %d messages to user %s", n, userID)
	}
	return n
}
