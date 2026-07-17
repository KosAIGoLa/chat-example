package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ws-ex/model"
)

// PoolConfig controls database/sql connection pool sizing.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultPool returns conservative defaults for a single chat-service instance.
func DefaultPool() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 10 * time.Minute,
	}
}

// Init opens Postgres, applies pool settings, and runs migrations.
// Prefer InitWithConfig from app bootstrap so pool knobs are centralized.
func Init() *gorm.DB {
	return InitWithConfig(
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "chat"),
		getEnv("DB_PASSWORD", "chatpass"),
		getEnv("DB_NAME", "chatdb"),
		DefaultPool(),
	)
}

// InitWithConfig connects with explicit DSN parts and pool settings.
func InitWithConfig(host, port, user, password, dbname string, pool PoolConfig) *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Taipei",
		host, port, user, password, dbname,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Avoid flooding logs under load; errors still print.
		Logger: logger.Default.LogMode(logger.Warn),
		// Prepared statements help pool reuse under repeated list queries.
		PrepareStmt: true,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get sql.DB: %v", err)
	}

	pool = normalizePool(pool)
	sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.MessageRecord{},
		&model.FriendRequest{},
		&model.Friendship{},
		&model.Blacklist{},
		&model.PrivateConvCutoff{},
		&model.PrivatePin{},
		&model.Group{},
		&model.GroupMember{},
		&model.GroupAnnouncement{},
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

	log.Printf(
		"[DB] connected pool max_open=%d max_idle=%d max_lifetime=%s max_idle_time=%s",
		pool.MaxOpenConns, pool.MaxIdleConns, pool.ConnMaxLifetime, pool.ConnMaxIdleTime,
	)
	return db
}

func normalizePool(p PoolConfig) PoolConfig {
	if p.MaxOpenConns <= 0 {
		p.MaxOpenConns = 50
	}
	if p.MaxIdleConns <= 0 {
		p.MaxIdleConns = 10
	}
	// Idle should not exceed open.
	if p.MaxIdleConns > p.MaxOpenConns {
		p.MaxIdleConns = p.MaxOpenConns
	}
	if p.ConnMaxLifetime <= 0 {
		p.ConnMaxLifetime = time.Hour
	}
	if p.ConnMaxIdleTime <= 0 {
		p.ConnMaxIdleTime = 10 * time.Minute
	}
	return p
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
