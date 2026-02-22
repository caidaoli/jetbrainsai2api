package main

import (
	"jetbrainsai2api/internal/config"
	logpkg "jetbrainsai2api/internal/log"
	"jetbrainsai2api/internal/server"
	"jetbrainsai2api/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	dotenvErr := godotenv.Load()

	logger := logpkg.CreateLogger()
	defer func() {
		if appLog, ok := logger.(*logpkg.AppLogger); ok {
			_ = appLog.Close()
		}
	}()

	if dotenvErr != nil {
		logger.Warn("No .env file found, using system environment variables")
	}
	logger.Info("Logger initialized")

	storageInstance, err := storage.InitStorage(logger)
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
	defer func() { _ = srv.Close() }()

	logger.Info("Starting server on port %s", cfg.Port)
	if err := srv.Run(); err != nil {
		logger.Fatal("Server error: %v", err)
	}
}
