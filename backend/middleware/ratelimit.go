package middleware

import (
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/logoes0/peeriodic.git/utils"
)

// clientBucket tracks token bucket rate limit state for a single client IP
type clientBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter implements a token bucket rate limiter keyed by client IP
type IPRateLimiter struct {
	mu          sync.Mutex
	clients     map[string]*clientBucket
	rate        float64 // tokens per second
	burst       float64 // max burst capacity
	sweepTicker *time.Ticker
	stopSweeper chan struct{}
	closeOnce   sync.Once
}

// NewIPRateLimiter creates a new rate limiter
// requestsPerMinute: average allowed requests per minute
// burst: maximum burst tokens allowed
func NewIPRateLimiter(requestsPerMinute int, burst int) *IPRateLimiter {
	if requestsPerMinute <= 0 || burst <= 0 {
		panic("rate and burst must be positive")
	}
	limiter := &IPRateLimiter{
		clients:     make(map[string]*clientBucket),
		rate:        float64(requestsPerMinute) / 60.0,
		burst:       float64(burst),
		sweepTicker: time.NewTicker(5 * time.Minute),
		stopSweeper: make(chan struct{}),
	}

	go limiter.startCleaner()
	return limiter
}

// startCleaner periodically removes stale IP entries to prevent memory leaks
func (l *IPRateLimiter) startCleaner() {
	for {
		select {
		case <-l.sweepTicker.C:
			l.mu.Lock()
			now := time.Now()
			for ip, bucket := range l.clients {
				// If client hasn't been active for 10 minutes, evict it
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(l.clients, ip)
				}
			}
			l.mu.Unlock()
		case <-l.stopSweeper:
			l.sweepTicker.Stop()
			return
		}
	}
}

// Close stops the background cleaner
func (l *IPRateLimiter) Close() {
	l.closeOnce.Do(func() { close(l.stopSweeper) })
}

// Allow checks if the client IP is allowed to make a request
func (l *IPRateLimiter) Allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	bucket, exists := l.clients[ip]
	if !exists {
		l.clients[ip] = &clientBucket{
			tokens:     l.burst - 1,
			lastRefill: now,
		}
		return true, 0
	}

	// Calculate tokens added since last refill
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * l.rate
	if bucket.tokens > l.burst {
		bucket.tokens = l.burst
	}
	bucket.lastRefill = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, 0
	}

	// Calculate retry after in seconds
	missing := 1.0 - bucket.tokens
	retryAfter := int(math.Ceil(missing / l.rate))
	if retryAfter < 1 {
		retryAfter = 1
	}

	return false, retryAfter
}

// ExtractIP uses the transport peer; forwarding headers from clients are untrusted.
func ExtractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// Middleware creates an http middleware that rate limits by IP
func (l *IPRateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := ExtractIP(r)
		allowed, retryAfter := l.Allow(ip)
		if !allowed {
			utils.TooManyRequests(w, retryAfter)
			return
		}
		next(w, r)
	}
}

// Pre-configured global rate limiters
var (
	// GeneralAPILimiter: 120 req/min with burst of 40
	GeneralAPILimiter = NewIPRateLimiter(120, 40)
	// AuthAPILimiter: 15 req/min with burst of 5 (protects against brute-force)
	AuthAPILimiter = NewIPRateLimiter(15, 5)
)
