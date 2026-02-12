package server

import (
	"jetbrainsai2api/internal/metrics"

	"github.com/gin-gonic/gin"
)

func (s *Server) setupRoutes() {
	gin.SetMode(s.ginMode)
	s.router = gin.New()

	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())
	s.router.Use(s.corsMiddleware())
	s.router.Use(s.maxBodySizeMiddleware())
	s.router.Use(s.rateLimitMiddleware())

	// Public routes (no auth)
	s.router.GET("/", metrics.ShowStatsPage)
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/api/stats", s.getStatsData)

	// Protected admin routes (auth required)
	admin := s.router.Group("/")
	admin.Use(s.authenticateClient)
	admin.GET("/log", metrics.StreamLog)

	// API routes (auth required)
	api := s.router.Group("/v1")
	api.Use(s.authenticateClient)
	{
		api.GET("/models", s.listModels)
		api.POST("/chat/completions", s.chatCompletions)
		api.POST("/messages", s.anthropicMessages)
	}
}
