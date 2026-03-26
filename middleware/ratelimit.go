package middleware

import (
	"net"
	"sync"
	"time"

	qf "github.com/twaritam/quicframe"
	"github.com/twaritam/quicframe/protocol"
	"golang.org/x/time/rate"
)

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained request rate allowed per client IP.
	RequestsPerSecond float64

	// Burst is the maximum burst size.  Defaults to RequestsPerSecond.
	Burst int

	// KeyFunc extracts the rate-limit key from a context.
	// Defaults to the client IP address.
	KeyFunc func(ctx *qf.Context) string

	// CleanupInterval controls how often idle limiters are evicted.
	// Defaults to 5 minutes.
	CleanupInterval time.Duration

	// IdleTimeout is how long a limiter must be idle before eviction.
	// Defaults to 10 minutes.
	IdleTimeout time.Duration
}

func (c *RateLimitConfig) setDefaults() {
	if c.Burst == 0 {
		c.Burst = int(c.RequestsPerSecond)
		if c.Burst < 1 {
			c.Burst = 1
		}
	}
	if c.KeyFunc == nil {
		c.KeyFunc = func(ctx *qf.Context) string {
			addr := ctx.RemoteAddr()
			if addr == nil {
				return "unknown"
			}
			host, _, err := net.SplitHostPort(addr.String())
			if err != nil {
				return addr.String()
			}
			return host
		}
	}
	if c.CleanupInterval == 0 {
		c.CleanupInterval = 5 * time.Minute
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 10 * time.Minute
	}
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit returns middleware that enforces per-key token-bucket rate limits.
// Idle limiters are evicted in the background to prevent unbounded memory growth.
func RateLimit(cfg RateLimitConfig) qf.MiddlewareFunc {
	cfg.setDefaults()

	var (
		mu      sync.Mutex
		entries = make(map[string]*entry)
	)

	// Background cleanup goroutine.
	go func() {
		ticker := time.NewTicker(cfg.CleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for k, e := range entries {
				if time.Since(e.lastSeen) > cfg.IdleTimeout {
					delete(entries, k)
				}
			}
			mu.Unlock()
		}
	}()

	getLimiter := func(key string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		e, ok := entries[key]
		if !ok {
			e = &entry{
				limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst),
			}
			entries[key] = e
		}
		e.lastSeen = time.Now()
		return e.limiter
	}

	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(ctx *qf.Context) error {
			key := cfg.KeyFunc(ctx)
			if !getLimiter(key).Allow() {
				return ctx.Error(protocol.StatusTooManyRequests, "rate limit exceeded")
			}
			return next(ctx)
		}
	}
}

// RateLimitSimple is a convenience wrapper for IP-based rate limiting.
//
//	mw := middleware.RateLimitSimple(100) // 100 req/s per IP
func RateLimitSimple(rps float64) qf.MiddlewareFunc {
	return RateLimit(RateLimitConfig{RequestsPerSecond: rps})
}
