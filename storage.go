package main

import (
	"context"
	"os"

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

// NewFileStorage 创建新的文件存储
func NewFileStorage(filePath string) *FileStorage {
	if filePath == "" {
		filePath = StatsFilePath
	}
	return &FileStorage{
		filePath: filePath,
	}
}

func (fs *FileStorage) SaveStats(stats *RequestStats) error {
	data, err := sonic.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fs.filePath, data, FilePermissionReadWrite)
}

func (fs *FileStorage) LoadStats() (*RequestStats, error) {
	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty stats if file doesn't exist
			return &RequestStats{
				RequestHistory: []RequestRecord{},
			}, nil
		}
		return nil, err
	}

	var stats RequestStats
	if err := sonic.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	// Ensure history is not nil
	if stats.RequestHistory == nil {
		stats.RequestHistory = []RequestRecord{}
	}

	return &stats, nil
}

func (fs *FileStorage) Close() error {
	return nil // File storage doesn't need cleanup
}

// RedisStorage implements persistence using Redis
type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
	key    string
}

// RedisStorageConfig Redis 存储配置
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

	// Test connection
	_, err = client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	key := config.Key
	if key == "" {
		key = statsRedisKey
	}

	Info("Successfully connected to Redis")
	return &RedisStorage{
		client: client,
		ctx:    ctx,
		key:    key,
	}, nil
}

func (rs *RedisStorage) SaveStats(stats *RequestStats) error {
	data, err := marshalJSON(stats)
	if err != nil {
		return err
	}

	return rs.client.Set(rs.ctx, rs.key, data, 0).Err()
}

func (rs *RedisStorage) LoadStats() (*RequestStats, error) {
	val, err := rs.client.Get(rs.ctx, rs.key).Result()
	if err != nil {
		if err == redis.Nil {
			// Return empty stats if key doesn't exist
			return &RequestStats{
				RequestHistory: []RequestRecord{},
			}, nil
		}
		return nil, err
	}

	var stats RequestStats
	if err := sonic.Unmarshal([]byte(val), &stats); err != nil {
		return nil, err
	}

	// Ensure history is not nil
	if stats.RequestHistory == nil {
		stats.RequestHistory = []RequestRecord{}
	}

	return &stats, nil
}

func (rs *RedisStorage) Close() error {
	return rs.client.Close()
}

// initStorage 初始化存储（返回 StorageInterface，不使用全局变量）
func initStorage() (StorageInterface, error) {
	redisURL := os.Getenv("REDIS_URL")

	if redisURL != "" {
		// Use Redis storage
		redisStorage, err := NewRedisStorage(RedisStorageConfig{
			URL: redisURL,
			Key: statsRedisKey,
		})
		if err != nil {
			Error("Failed to initialize Redis storage: %v, falling back to file storage", err)
			return NewFileStorage(StatsFilePath), nil
		}
		Info("Using Redis storage")
		return redisStorage, nil
	}

	// Use file storage
	Info("Using file storage")
	return NewFileStorage(StatsFilePath), nil
}

// 全局变量（向后兼容，逐步迁移到依赖注入）
var storage StorageInterface

// initStorageGlobal 初始化全局存储（向后兼容）
func initStorageGlobal() error {
	var err error
	storage, err = initStorage()
	return err
}

// saveStatsWithStorage saves stats using the configured storage
func saveStatsWithStorage() {
	stats := atomicStats.ToRequestStats()
	if err := storage.SaveStats(&stats); err != nil {
		Error("Error saving stats: %v", err)
	}
}

// loadStatsWithStorage loads stats using the configured storage
func loadStatsWithStorage() *RequestStats {
	stats, err := storage.LoadStats()
	if err != nil {
		Error("Error loading stats: %v", err)
		// Return empty stats if loading fails
		return &RequestStats{
			RequestHistory: []RequestRecord{},
		}
	}

	Info("Successfully loaded %d request records", len(stats.RequestHistory))
	return stats
}
