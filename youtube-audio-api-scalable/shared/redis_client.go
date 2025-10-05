package shared

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// NewRedisClient constructs a go-redis client from Config
func NewRedisClient(cfg *Config) *redis.Client {
	if cfg == nil || cfg.RedisAddr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		// Reasonable timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
}

// PingRedis validates the connection.
func PingRedis(client *redis.Client) error {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.Ping(ctx).Err()
}
