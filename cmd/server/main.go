package main

import (
	"jetbrainsai2api/internal/config"
	logpkg "jetbrainsai2api/internal/log"
	"jetbrainsai2api/internal/server"
	"jetbrainsai2api/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		println("No .env file found, using system environment variables")
	}

	logger := logpkg.CreateLogger()
	logger.Info("Logger initialized")

	storageInstance, err := storage.InitStorage()
	if err != nil {
		logger.Fatal("Failed to initialize storage: %v", err)
	}
	defer func() { _ = storageInstance.Close() }()

	cfg, err := config.LoadServerConfigFromEnv(logger)
	if err != nil {
		logger.Fatal("Failed to load server configuration: %v", err)
	}

	cfg.Storage = storageInstance
	cfg.Logger = logger

	srv, err := server.NewServer(cfg)
	if err != nil {
		logger.Fatal("Failed to create server: %v", err)
	}

	logger.Info("Starting server on port %s", cfg.Port)
	if err := srv.Run(); err != nil {
		logger.Fatal("Server error: %v", err)
	}
}
