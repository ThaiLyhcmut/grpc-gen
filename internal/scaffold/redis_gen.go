package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

// generateRedisClient generates the Redis client file
func generateRedisClient(serverDir string, modulePath string) error {
	content := fmt.Sprintf(`package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Address      string
	Password     string
	DB           int
	DialTimeout  int
	ReadTimeout  int
	WriteTimeout int
	PoolSize     int
	MinIdleConns int
}

// RedisClient wraps the Redis client
type RedisClient struct {
	client *redis.Client
	config *RedisConfig
}

// NewRedisClient creates a new Redis client from environment variables
// Returns nil if Redis is not configured (REDIS_ENABLED != "true")
func NewRedisClient() (*RedisClient, error) {
	// Check if Redis is enabled
	enabled := os.Getenv("REDIS_ENABLED")
	if enabled != "true" {
		log.Println("Redis is disabled, caching will be skipped")
		return nil, nil
	}

	cfg := RedisConfig{
		Address:      getEnvOrDefault("REDIS_ADDRESS", "localhost:6379"),
		Password:     os.Getenv("REDIS_PASSWORD"),
		DB:           getEnvAsInt("REDIS_DB", 0),
		DialTimeout:  getEnvAsInt("REDIS_DIAL_TIMEOUT", 5),
		ReadTimeout:  getEnvAsInt("REDIS_READ_TIMEOUT", 3),
		WriteTimeout: getEnvAsInt("REDIS_WRITE_TIMEOUT", 3),
		PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 10),
		MinIdleConns: getEnvAsInt("REDIS_MIN_IDLE_CONNS", 5),
	}

	// Create Redis client options
	opts := &redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	}

	// Create Redis client
	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Failed to connect to Redis at %%s: %%v", cfg.Address, err)
		log.Println("Continuing without Redis caching...")
		return nil, nil
	}

	log.Printf("Connected to Redis at %%s (DB: %%d)", cfg.Address, cfg.DB)

	return &RedisClient{
		client: client,
		config: &cfg,
	}, nil
}

// GetClient returns the underlying Redis client (can be nil)
func (r *RedisClient) GetClient() *redis.Client {
	if r == nil {
		return nil
	}
	return r.client
}

// Ping checks if Redis connection is alive
func (r *RedisClient) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Ping(ctx).Err()
}

// Set sets a key-value pair with optional expiration
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	if r == nil || r.client == nil {
		return "", redis.Nil
	}
	return r.client.Get(ctx, key).Result()
}

// Del deletes one or more keys
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

// ============================================
// CACHE HELPER FUNCTIONS
// ============================================

// GenerateCacheKey creates a unique cache key from prefix and parameters
func GenerateCacheKey(prefix string, params interface{}) string {
	data, _ := json.Marshal(params)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%%s%%x", prefix, hash[:16])
}

// GetCachedProto retrieves cached protobuf message from Redis
// Returns (true, nil) on cache hit, (false, nil) on cache miss or if Redis is disabled
func GetCachedProto(ctx context.Context, redisClient *redis.Client, key string, dest proto.Message) (bool, error) {
	if redisClient == nil {
		return false, nil
	}

	data, err := redisClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil // Cache miss
	}
	if err != nil {
		log.Printf("Redis get error for key %%s: %%v", key, err)
		return false, nil // Don't fail on cache errors
	}

	if err := proto.Unmarshal(data, dest); err != nil {
		log.Printf("Proto unmarshal error for key %%s: %%v", key, err)
		return false, nil
	}

	return true, nil
}

// SetCachedProto stores protobuf message in Redis
func SetCachedProto(ctx context.Context, redisClient *redis.Client, key string, msg proto.Message, ttl time.Duration) {
	if redisClient == nil {
		return
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("Proto marshal error for key %%s: %%v", key, err)
		return
	}

	if err := redisClient.Set(ctx, key, data, ttl).Err(); err != nil {
		log.Printf("Redis set error for key %%s: %%v", key, err)
	}
}

// InvalidateCacheByPattern invalidates cache keys matching a pattern
func InvalidateCacheByPattern(ctx context.Context, redisClient *redis.Client, pattern string) error {
	if redisClient == nil {
		return nil
	}

	iter := redisClient.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := redisClient.Del(ctx, iter.Val()).Err(); err != nil {
			log.Printf("Failed to delete cache key %%s: %%v", iter.Val(), err)
		}
	}
	return iter.Err()
}

// InvalidateCacheByKey invalidates a specific cache key
func InvalidateCacheByKey(ctx context.Context, redisClient *redis.Client, key string) error {
	if redisClient == nil {
		return nil
	}

	if err := redisClient.Del(ctx, key).Err(); err != nil {
		log.Printf("Failed to delete cache key %%s: %%v", key, err)
		return err
	}
	return nil
}

// ============================================
// INTERNAL HELPERS
// ============================================

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
`, modulePath)

	redisFile := filepath.Join(serverDir, "client", "redis.go")
	return os.WriteFile(redisFile, []byte(content), 0644)
}
