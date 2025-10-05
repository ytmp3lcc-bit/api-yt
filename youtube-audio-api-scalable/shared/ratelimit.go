package shared

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RateLimiter provides per-IP rate limiting with optional Redis backend
type RateLimiter struct {
	cfg        *Config
	redis      *redis.Client
	inMemMu    sync.Mutex
	inMemCount map[string]int
	inMemTTL   time.Time
}

func NewRateLimiter(cfg *Config, redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{cfg: cfg, redis: redisClient, inMemCount: map[string]int{}}
}

// key for the current minute window
func minuteKey(ip string) string {
	return fmt.Sprintf("ratelimit:%s:%d", ip, time.Now().Unix()/60)
}

// Allow returns whether the request is allowed and remaining quota (best-effort)
func (r *RateLimiter) Allow(ip string) (bool, int) {
	rpm := r.cfg.RateLimitRPM
	if rpm <= 0 {
		return true, rpm
	}
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		key := minuteKey(ip)
		n, err := r.redis.Incr(ctx, key).Result()
		if err != nil {
			// Fallback to in-memory on error
			return r.allowInMem(ip, rpm)
		}
		// Ensure expiry ~65 seconds for the rolling window minute
		if n == 1 {
			_ = r.redis.Expire(ctx, key, 65*time.Second).Err()
		}
		remaining := rpm - int(n)
		return int(n) <= rpm, remaining
	}
	return r.allowInMem(ip, rpm)
}

func (r *RateLimiter) allowInMem(ip string, rpm int) (bool, int) {
	now := time.Now()
	// Reset counts on minute boundary
	r.inMemMu.Lock()
	defer r.inMemMu.Unlock()
	if now.Sub(r.inMemTTL) > 60*time.Second {
		r.inMemCount = map[string]int{}
		r.inMemTTL = now
	}
	r.inMemCount[ip]++
	n := r.inMemCount[ip]
	remaining := rpm - n
	return n <= rpm, remaining
}

// GetClientIP extracts client IP from headers or RemoteAddr
func GetClientIP(r *http.Request) string {
	// Try common proxy headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the first IP in the list
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
