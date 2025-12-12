package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// HTTP 中间件
// SRP: 单一职责 - 每个中间件只负责一件事
// ============================================================================

// corsMiddleware CORS中间件
// 允许跨域请求，支持常见的 HTTP 方法和头部
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		c.Header("Access-Control-Max-Age", CORSMaxAge)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// authenticateClient 客户端认证中间件
// 支持两种认证方式：
// 1. Authorization: Bearer <token>
// 2. x-api-key: <token>
func (s *Server) authenticateClient(c *gin.Context) {
	if len(s.validClientKeys) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service unavailable: no client API keys configured"})
		c.Abort()
		return
	}

	authHeader := c.GetHeader(HeaderAuthorization)
	apiKey := c.GetHeader(HeaderXAPIKey)

	// Check x-api-key first
	if apiKey != "" {
		if s.validClientKeys[apiKey] {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (x-api-key)"})
		c.Abort()
		return
	}

	// Check Authorization header
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, AuthBearerPrefix)
		if s.validClientKeys[token] {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (Bearer token)"})
		c.Abort()
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "API key required in Authorization header (Bearer) or x-api-key header"})
	c.Abort()
}
