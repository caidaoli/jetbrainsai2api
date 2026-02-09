package storage

import (
	"context"
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

func NewFileStorage(filePath string) *FileStorage {
	if filePath == "" {
		filePath = core.StatsFilePath
	}
	return &FileStorage{filePath: filePath}
}

func (fs *FileStorage) SaveStats(stats *core.RequestStats) error {
	data, err := sonic.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fs.filePath, data, core.FilePermissionReadWrite)
}

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

func NewRedisStorage(config RedisStorageConfig) (*RedisStorage, error) {
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

	println("Successfully connected to Redis")
	return &RedisStorage{client: client, ctx: ctx, key: key}, nil
}

func (rs *RedisStorage) SaveStats(stats *core.RequestStats) error {
	data, err := util.MarshalJSON(stats)
	if err != nil {
		return err
	}
	return rs.client.Set(rs.ctx, rs.key, data, 0).Err()
}

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

func (rs *RedisStorage) Close() error {
	return rs.client.Close()
}

// InitStorage initializes storage (returns StorageInterface)
func InitStorage() (core.StorageInterface, error) {
	redisURL := os.Getenv("REDIS_URL")

	if redisURL != "" {
		redisStorage, err := NewRedisStorage(RedisStorageConfig{
			URL: redisURL,
			Key: statsRedisKey,
		})
		if err != nil {
			println("Failed to initialize Redis storage:", err.Error(), ", falling back to file storage")
			return NewFileStorage(core.StatsFilePath), nil
		}
		println("Using Redis storage")
		return redisStorage, nil
	}

	println("Using file storage")
	return NewFileStorage(core.StatsFilePath), nil
}
