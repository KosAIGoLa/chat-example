package database

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ws-ex/model"
)

func Init() *gorm.DB {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "chat")
	password := getEnv("DB_PASSWORD", "chatpass")
	dbname := getEnv("DB_NAME", "chatdb")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Taipei",
		host, port, user, password, dbname)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.MessageRecord{},
		&model.FriendRequest{},
		&model.Friendship{},
		&model.Blacklist{},
		&model.Group{},
		&model.GroupMember{},
		&model.OfflineMessage{},
		&model.WalletLedger{},
		&model.RedPacket{},
		&model.RedPacketClaim{},
	); err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}

	// Backfill balance for existing users that still have 0 (registered before wallet).
	initBal := int64(1000)
	if v := os.Getenv("INITIAL_BALANCE"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &initBal); err != nil || n != 1 {
			initBal = 1000
		}
	}
	if err := db.Model(&model.User{}).
		Where("balance = 0").
		Update("balance", initBal).Error; err != nil {
		log.Printf("[DB] balance backfill skip: %v", err)
	}

	// Global message sequence for incremental history (since_seq).
	if err := db.Exec(`CREATE SEQUENCE IF NOT EXISTS message_global_seq START WITH 1 INCREMENT BY 1`).Error; err != nil {
		log.Printf("[DB] message_global_seq create skip: %v", err)
	}
	// Backfill seq=0 rows so order is stable (id order as proxy for creation).
	if err := db.Exec(`
		WITH numbered AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY created_at ASC, id ASC) AS rn
			FROM message_records
			WHERE seq = 0 OR seq IS NULL
		)
		UPDATE message_records m
		SET seq = numbered.rn
		FROM numbered
		WHERE m.id = numbered.id
	`).Error; err != nil {
		log.Printf("[DB] message seq backfill skip: %v", err)
	}
	// Advance sequence past any existing seq values so new messages stay monotonic.
	if err := db.Exec(`
		SELECT setval(
			'message_global_seq',
			(SELECT GREATEST(COALESCE(MAX(seq), 1), 1) FROM message_records)
		)
	`).Error; err != nil {
		log.Printf("[DB] message_global_seq setval skip: %v", err)
	}

	log.Println("[DB] connected and migrated successfully")
	return db
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
