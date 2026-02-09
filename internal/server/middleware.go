package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"jetbrainsai2api/internal/core"

	"github.com/gin-gonic/gin"
)

func (s *Server) isValidClientKey(providedKey string) bool {
	providedBytes := []byte(providedKey)
	for validKey := range s.validClientKeys {
		validBytes := []byte(validKey)
		if subtle.ConstantTimeCompare(providedBytes, validBytes) == 1 {
			return true
		}
	}
	return false
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
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
