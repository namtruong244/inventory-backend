package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"inventory_backend/internal/response"

	"github.com/gin-gonic/gin"
)

// RateLimiter is an in-memory fixed-window counter. State is per process, so on
// Lambda it only limits within one warm instance; pair it with API Gateway
// throttling or a WAF rate-based rule for a hard global limit.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, buckets: make(map[string]*bucket), now: time.Now}
}

// Allow records one attempt for key and reports whether it is within the limit.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if len(l.buckets) > 10000 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		b = &bucket{resetAt: now.Add(l.window)}
		l.buckets[key] = b
	}
	b.count++
	return b.count <= l.limit
}

// Blocked reports whether key has already used up its attempts, without
// counting a new one.
func (l *RateLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	return ok && !l.now().After(b.resetAt) && b.count >= l.limit
}

// Reset forgets key's attempts, e.g. after a successful login.
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// Middleware limits requests per client IP.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			Reject(c)
			return
		}
		c.Next()
	}
}

// Reject aborts the request with 429 RATE_LIMITED.
func Reject(c *gin.Context) {
	response.SendError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many attempts, please try again later", nil)
	c.Abort()
}

// EmailKey normalises an email for use as a limiter key.
func EmailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// EmailIPKey scopes an email key to the client IP, so someone elsewhere cannot
// exhaust the real owner's attempts.
func EmailIPKey(email, ip string) string {
	return EmailKey(email) + "|" + ip
}
