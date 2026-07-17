package service

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ws-ex/model"
)

// WalletService manages virtual coin balance and ledgers.
type WalletService struct {
	db *gorm.DB
}

func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{db: db}
}

// InitialBalance for new registrations (default 1000).
func InitialBalance() int64 {
	if v := os.Getenv("INITIAL_BALANCE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return 1000
}

// GetBalance returns the user's current balance.
func (s *WalletService) GetBalance(userID uint) (int64, error) {
	var u model.User
	if err := s.db.Select("id", "balance").First(&u, userID).Error; err != nil {
		return 0, errors.New("user not found")
	}
	return u.Balance, nil
}

// AdjustInTx changes balance inside an existing transaction. delta may be negative.
func (s *WalletService) AdjustInTx(tx *gorm.DB, userID uint, delta int64, reason, refType, refID string) (int64, error) {
	if tx == nil {
		tx = s.db
	}
	var u model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID).Error; err != nil {
		return 0, errors.New("user not found")
	}
	next := u.Balance + delta
	if next < 0 {
		return 0, fmt.Errorf("insufficient balance (have %d, need %d)", u.Balance, -delta)
	}
	if err := tx.Model(&u).Update("balance", next).Error; err != nil {
		return 0, err
	}
	led := model.WalletLedger{
		UserID:       userID,
		Delta:        delta,
		BalanceAfter: next,
		Reason:       reason,
		RefType:      refType,
		RefID:        refID,
	}
	if err := tx.Create(&led).Error; err != nil {
		return 0, err
	}
	return next, nil
}

// ParseUserID converts string user id to uint.
func ParseUserID(id string) (uint, error) {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user id")
	}
	return uint(n), nil
}
