package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"ws-ex/model"
)

// RecallWindow is how long a sender may recall a message after send.
const RecallWindow = 2 * time.Minute

// MessageStore persists message metadata for recall authorization.
type MessageStore struct {
	db *gorm.DB
}

func NewMessageStore(db *gorm.DB) *MessageStore {
	return &MessageStore{db: db}
}

// NewMessageID returns a random 32-char hex id.
func NewMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Save records a newly sent private/group chat message.
func (s *MessageStore) Save(rec *model.MessageRecord) error {
	if s == nil || s.db == nil || rec == nil || rec.ID == "" {
		return nil
	}
	return s.db.Create(rec).Error
}

// Recall marks a message as recalled if the caller is the sender and within the window.
func (s *MessageStore) Recall(msgID, fromUserID string) (*model.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("message store unavailable")
	}
	var rec model.MessageRecord
	if err := s.db.Where("id = ?", msgID).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("message not found")
		}
		return nil, err
	}
	if rec.FromUserID != fromUserID {
		return nil, errors.New("only the sender can recall this message")
	}
	if rec.Recalled {
		return &rec, nil // idempotent
	}
	sentAt := time.Unix(rec.Timestamp, 0)
	if time.Since(sentAt) > RecallWindow {
		return nil, errors.New("recall window expired (2 minutes)")
	}
	now := time.Now()
	if err := s.db.Model(&rec).Updates(map[string]interface{}{
		"recalled":    true,
		"recalled_at": now,
	}).Error; err != nil {
		return nil, err
	}
	rec.Recalled = true
	rec.RecalledAt = &now
	return &rec, nil
}

// RecalledIDs returns the subset of ids that are recalled.
func (s *MessageStore) RecalledIDs(ids []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if s == nil || s.db == nil || len(ids) == 0 {
		return out, nil
	}
	var rows []model.MessageRecord
	if err := s.db.Select("id").Where("id IN ? AND recalled = ?", ids, true).Find(&rows).Error; err != nil {
		return out, err
	}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out, nil
}
