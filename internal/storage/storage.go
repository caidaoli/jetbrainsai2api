package storage

import (
	"context"
	"fmt"
	"os"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

const (
	statsRedisKey = "jetbrainsai2api:stats"
)

// FileStorage implements persistence using JSON files
type FileStorage struct {
	filePath string
}

// NewFileStorage creates a new file-based storage instance.
func NewFileStorage(filePath string) *FileStorage {
	if filePath == "" {
		filePath = core.StatsFilePath
	}
	return &FileStorage{filePath: filePath}
}

// SaveStats persists request statistics to the JSON file atomically.
func (fs *FileStorage) SaveStats(stats *core.RequestStats) error {
	data, err := sonic.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}
	tmpFile := fs.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, core.FilePermissionReadWrite); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmpFile, fs.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// LoadStats reads request statistics from the JSON file.
func (fs *FileStorage) LoadStats() (*core.RequestStats, error) {
	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &core.RequestStats{RequestHistory: []core.RequestRecord{}}, nil
		}
		return nil, err
	}

	var stats core.RequestStats
	if err := sonic.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	if stats.RequestHistory == nil {
		stats.RequestHistory = []core.RequestRecord{}
	}

	return &stats, nil
}

// Close is a no-op for file storage (no resources to release).
func (fs *FileStorage) Close() error {
	return nil
}

// RedisStorage implements persistence using Redis
type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
	key    string
}

// RedisStorageConfig Redis storage config
type RedisStorageConfig struct {
	URL string
	Key string
}

func logStorageInfo(logger core.Logger, format string, args ...any) {
	if logger != nil {
		logger.Info(format, args...)
	}
}

func logStorageWarn(logger core.Logger, format string, args ...any) {
	if logger != nil {
		logger.Warn(format, args...)
	}
}

// NewRedisStorage creates a new Redis-based storage instance.
func NewRedisStorage(config RedisStorageConfig, logger core.Logger) (*RedisStorage, error) {
	opts, err := redis.ParseURL(config.URL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	ctx := context.Background()

	_, err = client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	key := config.Key
	if key == "" {
		key = statsRedisKey
	}

	logStorageInfo(logger, "Successfully connected to Redis")
	return &RedisStorage{client: client, ctx: ctx, key: key}, nil
}

// SaveStats persists request statistics to Redis.
func (rs *RedisStorage) SaveStats(stats *core.RequestStats) error {
	data, err := util.MarshalJSON(stats)
	if err != nil {
		return err
	}
	return rs.client.Set(rs.ctx, rs.key, data, 0).Err()
}

// LoadStats reads request statistics from Redis.
func (rs *RedisStorage) LoadStats() (*core.RequestStats, error) {
	val, err := rs.client.Get(rs.ctx, rs.key).Result()
	if err != nil {
		if err == redis.Nil {
			return &core.RequestStats{RequestHistory: []core.RequestRecord{}}, nil
		}
		return nil, err
	}

	var stats core.RequestStats
	if err := sonic.Unmarshal([]byte(val), &stats); err != nil {
		return nil, err
	}

	if stats.RequestHistory == nil {
		stats.RequestHistory = []core.RequestRecord{}
	}

	return &stats, nil
}

// Close closes the Redis connection.
func (rs *RedisStorage) Close() error {
	return rs.client.Close()
}

// InitStorage initializes storage (returns StorageInterface).
func InitStorage(logger core.Logger) (core.StorageInterface, error) {
	redisURL := os.Getenv("REDIS_URL")

	if redisURL != "" {
		redisStorage, err := NewRedisStorage(RedisStorageConfig{
			URL: redisURL,
			Key: statsRedisKey,
		}, logger)
		if err != nil {
			logStorageWarn(logger, "Failed to initialize Redis storage: %v, falling back to file storage", err)
			return NewFileStorage(core.StatsFilePath), nil
		}
		logStorageInfo(logger, "Using Redis storage")
		return redisStorage, nil
	}

	logStorageInfo(logger, "Using file storage")
	return NewFileStorage(core.StatsFilePath), nil
}
