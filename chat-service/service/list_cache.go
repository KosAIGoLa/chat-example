package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// ListCache is a thin Redis helper for list read-through caching.
// Pattern: list reads hit Redis; create/update/delete invalidate keys;
// next list misses, loads DB, and re-fills Redis.
//
// When rdb is nil (Redis disabled/unavailable), all ops no-op and lists always hit DB.
type ListCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// Cache key helpers (stable, namespaced).
const (
	cacheKeyFriends      = "list:friends:%d"          // userID
	cacheKeyIncoming     = "list:friends:incoming:%d" // userID
	cacheKeyOutgoing     = "list:friends:outgoing:%d" // userID
	cacheKeyBlacklist    = "list:blacklist:%d"        // userID
	cacheKeyGroupsMine   = "list:groups:mine:%d"      // userID
	cacheKeyGroupMembers = "list:group:members:%s"    // groupID
	cacheKeyGroupAnn     = "list:group:ann:%s"        // groupID
	cacheKeyPrivatePins  = "list:private:pins:%d:%d"  // userA,userB ordered
)

// RedisPoolOptions controls go-redis connection pool and I/O timeouts.
type RedisPoolOptions struct {
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

// DefaultRedisPool returns pool defaults for a chat API instance.
func DefaultRedisPool() RedisPoolOptions {
	return RedisPoolOptions{
		PoolSize:        50,
		MinIdleConns:    5,
		MaxIdleConns:    20,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnMaxLifetime: 30 * time.Minute,
		DialTimeout:     3 * time.Second,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
	}
}

// NewListCache connects to Redis. Empty addr → disabled cache (nil client).
func NewListCache(addr, password string, db int, ttl time.Duration) (*ListCache, error) {
	return NewListCacheWithPool(addr, password, db, ttl, DefaultRedisPool())
}

// NewListCacheWithPool connects with explicit pool tuning.
func NewListCacheWithPool(addr, password string, db int, ttl time.Duration, pool RedisPoolOptions) (*ListCache, error) {
	if addr == "" {
		log.Printf("[Cache] REDIS_ADDR empty — list cache disabled")
		return &ListCache{ttl: ttl}, nil
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	pool = normalizeRedisPool(pool)
	if password == "" {
		log.Printf("[Cache] REDIS_PASSWORD empty — connecting without AUTH (ok only if redis has no requirepass)")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// Pool
		PoolSize:        pool.PoolSize,
		MinIdleConns:    pool.MinIdleConns,
		MaxIdleConns:    pool.MaxIdleConns,
		PoolTimeout:     pool.PoolTimeout,
		ConnMaxIdleTime: pool.ConnMaxIdleTime,
		ConnMaxLifetime: pool.ConnMaxLifetime,

		// I/O
		DialTimeout:  pool.DialTimeout,
		ReadTimeout:  pool.ReadTimeout,
		WriteTimeout: pool.WriteTimeout,

		// Prefer reusing idle conns; go-redis also handles reconnect.
	})

	// Use dial timeout for bootstrap ping.
	ctx, cancel := context.WithTimeout(context.Background(), pool.DialTimeout)
	if pool.DialTimeout <= 0 {
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	}
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		authHint := "no-auth"
		if password != "" {
			authHint = "with-password"
		}
		return nil, fmt.Errorf("redis ping %s (%s): %w", addr, authHint, err)
	}

	auth := "auth=off"
	if password != "" {
		auth = "auth=on"
	}
	log.Printf(
		"[Cache] redis connected %s db=%d ttl=%s %s pool_size=%d min_idle=%d max_idle=%d",
		addr, db, ttl, auth, pool.PoolSize, pool.MinIdleConns, pool.MaxIdleConns,
	)
	return &ListCache{rdb: rdb, ttl: ttl}, nil
}

func normalizeRedisPool(p RedisPoolOptions) RedisPoolOptions {
	d := DefaultRedisPool()
	if p.PoolSize <= 0 {
		p.PoolSize = d.PoolSize
	}
	if p.MinIdleConns < 0 {
		p.MinIdleConns = 0
	}
	if p.MinIdleConns > p.PoolSize {
		p.MinIdleConns = p.PoolSize
	}
	if p.MaxIdleConns <= 0 {
		p.MaxIdleConns = d.MaxIdleConns
	}
	if p.MaxIdleConns > p.PoolSize {
		p.MaxIdleConns = p.PoolSize
	}
	if p.MinIdleConns > p.MaxIdleConns {
		p.MinIdleConns = p.MaxIdleConns
	}
	if p.PoolTimeout <= 0 {
		p.PoolTimeout = d.PoolTimeout
	}
	if p.ConnMaxIdleTime <= 0 {
		p.ConnMaxIdleTime = d.ConnMaxIdleTime
	}
	if p.ConnMaxLifetime < 0 {
		p.ConnMaxLifetime = 0 // 0 = no limit in go-redis
	}
	if p.DialTimeout <= 0 {
		p.DialTimeout = d.DialTimeout
	}
	if p.ReadTimeout <= 0 {
		p.ReadTimeout = d.ReadTimeout
	}
	if p.WriteTimeout <= 0 {
		p.WriteTimeout = d.WriteTimeout
	}
	return p
}

// Enabled reports whether Redis is active.
func (c *ListCache) Enabled() bool {
	return c != nil && c.rdb != nil
}

// Close releases the Redis client.
func (c *ListCache) Close() {
	if c == nil || c.rdb == nil {
		return
	}
	_ = c.rdb.Close()
	c.rdb = nil
}

// GetJSON unmarshals a cached value. ok=false on miss or disabled.
func (c *ListCache) GetJSON(key string, dest any) (ok bool) {
	if !c.Enabled() || key == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		// Corrupt entry — drop it.
		_ = c.rdb.Del(ctx, key).Err()
		return false
	}
	return true
}

// SetJSON stores a value with TTL. Best-effort; errors are logged only.
func (c *ListCache) SetJSON(key string, v any) {
	if !c.Enabled() || key == "" {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.rdb.Set(ctx, key, raw, c.ttl).Err(); err != nil {
		log.Printf("[Cache] SET %s: %v", key, err)
	}
}

// Del removes one or more keys (invalidate on write).
func (c *ListCache) Del(keys ...string) {
	if !c.Enabled() || len(keys) == 0 {
		return
	}
	clean := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			clean = append(clean, k)
		}
	}
	if len(clean) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.rdb.Del(ctx, clean...).Err(); err != nil {
		log.Printf("[Cache] DEL: %v", err)
	}
}

// --- key builders ---

func KeyFriends(userID uint) string    { return fmt.Sprintf(cacheKeyFriends, userID) }
func KeyIncoming(userID uint) string   { return fmt.Sprintf(cacheKeyIncoming, userID) }
func KeyOutgoing(userID uint) string   { return fmt.Sprintf(cacheKeyOutgoing, userID) }
func KeyBlacklist(userID uint) string  { return fmt.Sprintf(cacheKeyBlacklist, userID) }
func KeyGroupsMine(userID uint) string { return fmt.Sprintf(cacheKeyGroupsMine, userID) }
func KeyGroupMembers(groupID string) string {
	return fmt.Sprintf(cacheKeyGroupMembers, groupID)
}
func KeyGroupAnnouncements(groupID string) string {
	return fmt.Sprintf(cacheKeyGroupAnn, groupID)
}
func KeyPrivatePins(userA, userB uint) string {
	if userA > userB {
		userA, userB = userB, userA
	}
	return fmt.Sprintf(cacheKeyPrivatePins, userA, userB)
}
