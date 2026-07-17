package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"ws-ex/model"
)

// RecallWindow is how long a sender may recall or edit a message after send.
const RecallWindow = 2 * time.Minute

// EditWindow matches recall window (edit own text within 2 minutes).
const EditWindow = RecallWindow

// MessageStore persists message metadata for recall authorization + seq allocation.
type MessageStore struct {
	db *gorm.DB
}

func NewMessageStore(db *gorm.DB) *MessageStore {
	return &MessageStore{db: db}
}

// EnsureSeqSequence creates the Postgres sequence used by NextSeq (idempotent).
func EnsureSeqSequence(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.Exec(`CREATE SEQUENCE IF NOT EXISTS message_global_seq START WITH 1 INCREMENT BY 1`).Error
}

// NewMessageID returns a random 32-char hex id.
func NewMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NextSeq returns the next global monotonic sequence number.
func (s *MessageStore) NextSeq() int64 {
	if s == nil || s.db == nil {
		return time.Now().UnixNano()
	}
	var seq int64
	if err := s.db.Raw(`SELECT nextval('message_global_seq')`).Scan(&seq).Error; err != nil {
		// Fallback: time-based unique-ish value (dev only if sequence missing).
		return time.Now().UnixNano()
	}
	return seq
}

// Save records a newly sent private/group chat message.
// Assigns Seq via NextSeq when rec.Seq == 0.
func (s *MessageStore) Save(rec *model.MessageRecord) error {
	if s == nil || s.db == nil || rec == nil || rec.ID == "" {
		return nil
	}
	if rec.Seq == 0 {
		rec.Seq = s.NextSeq()
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

// Edit updates message body (ciphertext) if sender + within window + not recalled.
func (s *MessageStore) Edit(msgID, fromUserID, content string) (*model.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("message store unavailable")
	}
	if content == "" {
		return nil, errors.New("content is required")
	}
	var rec model.MessageRecord
	if err := s.db.Where("id = ?", msgID).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("message not found")
		}
		return nil, err
	}
	if rec.FromUserID != fromUserID {
		return nil, errors.New("only the sender can edit this message")
	}
	if rec.Recalled {
		return nil, errors.New("cannot edit a recalled message")
	}
	sentAt := time.Unix(rec.Timestamp, 0)
	if time.Since(sentAt) > EditWindow {
		return nil, errors.New("edit window expired (2 minutes)")
	}
	now := time.Now()
	if err := s.db.Model(&rec).Updates(map[string]interface{}{
		"edited":         true,
		"edited_at":      now,
		"edited_content": content,
	}).Error; err != nil {
		return nil, err
	}
	rec.Edited = true
	rec.EditedAt = &now
	rec.EditedContent = content
	return &rec, nil
}

// EditedBodies returns id → edited ciphertext for messages that were edited.
func (s *MessageStore) EditedBodies(ids []string) (map[string]string, error) {
	out := make(map[string]string)
	if s == nil || s.db == nil || len(ids) == 0 {
		return out, nil
	}
	var rows []model.MessageRecord
	if err := s.db.Select("id", "edited_content").
		Where("id IN ? AND edited = ? AND recalled = ?", ids, true, false).
		Find(&rows).Error; err != nil {
		return out, err
	}
	for _, r := range rows {
		if r.EditedContent != "" {
			out[r.ID] = r.EditedContent
		}
	}
	return out, nil
}

// DeletePrivatePair removes message_records for private chat between two users.
func (s *MessageStore) DeletePrivatePair(userA, userB string) {
	if s == nil || s.db == nil || userA == "" || userB == "" {
		return
	}
	_ = s.db.Where(
		"type = ? AND ((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))",
		"private", userA, userB, userB, userA,
	).Delete(&model.MessageRecord{}).Error
}

// SeqByIDs returns id → seq for the given message ids (only rows with seq > 0).
func (s *MessageStore) SeqByIDs(ids []string) (map[string]int64, error) {
	out := make(map[string]int64)
	if s == nil || s.db == nil || len(ids) == 0 {
		return out, nil
	}
	var rows []model.MessageRecord
	if err := s.db.Select("id", "seq").Where("id IN ? AND seq > 0", ids).Find(&rows).Error; err != nil {
		return out, err
	}
	for _, r := range rows {
		out[r.ID] = r.Seq
	}
	return out, nil
}
