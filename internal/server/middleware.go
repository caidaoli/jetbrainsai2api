package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"jetbrainsai2api/internal/core"

	"github.com/gin-gonic/gin"
)

// MaxBodySize is the maximum allowed request body size (50MB).
const MaxBodySize = 50 << 20

func (s *Server) maxBodySizeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodySize)
		c.Next()
	}
}

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitorInfo
	rate     int
	cleanup  time.Duration
}

type visitorInfo struct {
	count    int
	lastSeen time.Time
}

func newRateLimiter(ratePerMinute int) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*visitorInfo),
		rate:     ratePerMinute,
		cleanup:  5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	v, exists := rl.visitors[ip]
	if !exists || time.Since(v.lastSeen) > time.Minute {
		rl.visitors[ip] = &visitorInfo{count: 1, lastSeen: time.Now()}
		return true
	}
	v.count++
	v.lastSeen = time.Now()
	return v.count <= rl.rate
}

func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !s.rateLimiter.allow(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) isValidClientKey(providedKey string) bool {
	providedBytes := []byte(providedKey)
	for validKey := range s.validClientKeys {
		validBytes := []byte(validKey)
		if len(providedBytes) == len(validBytes) && subtle.ConstantTimeCompare(providedBytes, validBytes) == 1 {
			return true
		}
	}
	return false
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	allowOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
	if allowOrigin == "" {
		allowOrigin = "*"
	}

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		c.Header("Access-Control-Max-Age", core.CORSMaxAge)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (s *Server) authenticateClient(c *gin.Context) {
	if len(s.validClientKeys) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service unavailable: no client API keys configured"})
		c.Abort()
		return
	}

	authHeader := c.GetHeader(core.HeaderAuthorization)
	apiKey := c.GetHeader(core.HeaderXAPIKey)

	if apiKey != "" {
		if s.isValidClientKey(apiKey) {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (x-api-key)"})
		c.Abort()
		return
	}

	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, core.AuthBearerPrefix)
		if s.isValidClientKey(token) {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (Bearer token)"})
		c.Abort()
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "API key required in Authorization header (Bearer) or x-api-key header"})
	c.Abort()
}
