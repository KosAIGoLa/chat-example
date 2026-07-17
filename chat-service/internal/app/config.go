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
