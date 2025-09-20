package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RateLimiter struct {
	clients    map[string]*ClientBucket
	mutex      sync.RWMutex
	cleanup    *time.Ticker
	logger     *zap.Logger
	defaultRPS int
	burst      int
}

type ClientBucket struct {
	tokens     int
	lastUpdate time.Time
	mutex      sync.Mutex
}

func NewRateLimiter(defaultRPS, burst int, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		clients:    make(map[string]*ClientBucket),
		defaultRPS: defaultRPS,
		burst:      burst,
		logger:     logger,
	}

	rl.cleanup = time.NewTicker(5 * time.Minute)
	go rl.cleanupExpiredClients()

	return rl
}

func (rl *RateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		if !rl.allowRequest(clientIP) {
			rl.logger.Warn("Rate limit exceeded",
				zap.String("client_ip", clientIP),
				zap.String("path", c.Request.URL.Path))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": 60, 
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) RateLimitWithConfig(rps int, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		if !rl.allowRequestWithConfig(clientIP, rps, burst) {
			rl.logger.Warn("Rate limit exceeded with custom config",
				zap.String("client_ip", clientIP),
				zap.String("path", c.Request.URL.Path),
				zap.Int("rps", rps))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) allowRequest(clientIP string) bool {
	return rl.allowRequestWithConfig(clientIP, rl.defaultRPS, rl.burst)
}

func (rl *RateLimiter) allowRequestWithConfig(clientIP string, rps, burst int) bool {
	rl.mutex.Lock()
	bucket, exists := rl.clients[clientIP]
	if !exists {
		bucket = &ClientBucket{
			tokens:     burst,
			lastUpdate: time.Now(),
		}
		rl.clients[clientIP] = bucket
	}
	rl.mutex.Unlock()

	return bucket.allowRequest(rps, burst)
}

func (cb *ClientBucket) allowRequest(rps, burst int) bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(cb.lastUpdate)

	tokensToAdd := int(elapsed.Seconds() * float64(rps))
	cb.tokens += tokensToAdd

	if cb.tokens > burst {
		cb.tokens = burst
	}

	cb.lastUpdate = now

	if cb.tokens > 0 {
		cb.tokens--
		return true
	}

	return false
}

func (rl *RateLimiter) cleanupExpiredClients() {
	for range rl.cleanup.C {
		rl.mutex.Lock()
		now := time.Now()
		for ip, bucket := range rl.clients {
			bucket.mutex.Lock()
			if now.Sub(bucket.lastUpdate) > 10*time.Minute {
				delete(rl.clients, ip)
			}
			bucket.mutex.Unlock()
		}
		rl.mutex.Unlock()
	}
}

func (rl *RateLimiter) GetClientStats(clientIP string) (tokens int, lastUpdate time.Time, exists bool) {
	rl.mutex.RLock()
	bucket, exists := rl.clients[clientIP]
	rl.mutex.RUnlock()

	if !exists {
		return 0, time.Time{}, false
	}

	bucket.mutex.Lock()
	defer bucket.mutex.Unlock()
	return bucket.tokens, bucket.lastUpdate, true
}

func (rl *RateLimiter) GetGlobalStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	return map[string]interface{}{
		"active_clients": len(rl.clients),
		"default_rps":    rl.defaultRPS,
		"burst_capacity": rl.burst,
	}
}

func (rl *RateLimiter) Shutdown() {
	if rl.cleanup != nil {
		rl.cleanup.Stop()
	}
}
