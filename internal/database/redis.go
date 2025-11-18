package database

import (
	"context"
	"fmt"
	"time"

	"go-telegram-forwarder-bot/internal/config"

	"github.com/redis/go-redis/v9"
)

func ConnectRedis(cfg config.RedisConfig) (*redis.Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return rdb, nil
}

func RetryRedisConnection(cfg config.RedisConfig, maxRetries int, interval time.Duration) (*redis.Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	for i := 0; i < maxRetries; i++ {
		client, err := ConnectRedis(cfg)
		if err == nil {
			return client, nil
		}

		if i < maxRetries-1 {
			time.Sleep(interval)
		}
	}

	return nil, fmt.Errorf("failed to connect to Redis after %d retries", maxRetries)
}
