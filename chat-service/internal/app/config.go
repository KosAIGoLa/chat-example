package app

import (
	"os"
	"strconv"
	"time"
)

// Config holds process configuration from the environment.
type Config struct {
	Addr            string
	NATSURL         string
	JWTSecret       string
	MsgCryptoKey    string
	MediaDir        string
	ShutdownTimeout time.Duration

	// Postgres
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBPool     DBPoolConfig

	// Redis list cache (empty RedisAddr disables caching).
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisTTL      time.Duration
	RedisPool     RedisPoolConfig
}

// DBPoolConfig is database/sql pool settings applied after gorm.Open.
type DBPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// RedisPoolConfig is go-redis connection pool + I/O timeouts.
type RedisPoolConfig struct {
	PoolSize        int
	MinIdleConns    int
	MaxIdleConns    int
	PoolTimeout     time.Duration
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
}

// LoadConfig reads env vars with sensible local-dev defaults.
func LoadConfig() Config {
	return Config{
		Addr:            envOr("SERVER_ADDR", ":8080"),
		NATSURL:         envOr("NATS_URL", "nats://127.0.0.1:4222"),
		JWTSecret:       envOr("JWT_SECRET", "change-me-in-production"),
		MsgCryptoKey:    os.Getenv("MSG_CRYPTO_KEY"),
		MediaDir:        envOr("MEDIA_DIR", "./data/voice"),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),

		DBHost:     envOr("DB_HOST", "localhost"),
		DBPort:     envOr("DB_PORT", "5432"),
		DBUser:     envOr("DB_USER", "chat"),
		DBPassword: envOr("DB_PASSWORD", "chatpass"),
		DBName:     envOr("DB_NAME", "chatdb"),
		DBPool: DBPoolConfig{
			// MaxOpen ≈ concurrent DB-heavy requests; keep under Postgres max_connections.
			MaxOpenConns: envInt("DB_MAX_OPEN_CONNS", 50),
			// Idle pool for reuse under bursty chat traffic.
			MaxIdleConns: envInt("DB_MAX_IDLE_CONNS", 10),
			// Recycle long-lived connections (NAT / load balancer friendliness).
			ConnMaxLifetime: envDuration("DB_CONN_MAX_LIFETIME", time.Hour),
			// Drop idle conns so Postgres is not holding unused backends.
			ConnMaxIdleTime: envDuration("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
		},

		RedisAddr:     envOr("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       envInt("REDIS_DB", 0),
		RedisTTL:      envDuration("REDIS_LIST_TTL", 10*time.Minute),
		RedisPool: RedisPoolConfig{
			PoolSize:        envInt("REDIS_POOL_SIZE", 50),
			MinIdleConns:    envInt("REDIS_MIN_IDLE_CONNS", 5),
			MaxIdleConns:    envInt("REDIS_MAX_IDLE_CONNS", 20),
			PoolTimeout:     envDuration("REDIS_POOL_TIMEOUT", 4*time.Second),
			ConnMaxIdleTime: envDuration("REDIS_CONN_MAX_IDLE_TIME", 5*time.Minute),
			ConnMaxLifetime: envDuration("REDIS_CONN_MAX_LIFETIME", 30*time.Minute),
			DialTimeout:     envDuration("REDIS_DIAL_TIMEOUT", 3*time.Second),
			ReadTimeout:     envDuration("REDIS_READ_TIMEOUT", 2*time.Second),
			WriteTimeout:    envDuration("REDIS_WRITE_TIMEOUT", 2*time.Second),
		},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	// Accept Go durations ("15s", "1m") or plain seconds ("15").
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
