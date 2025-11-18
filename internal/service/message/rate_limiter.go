package message

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go-telegram-forwarder-bot/internal/config"
	"go.uber.org/zap"
)

type RateLimiter struct {
	redisClient *redis.Client
	memoryStore map[string]*tokenBucket
	mutex       sync.RWMutex
	config      *config.Config
	logger      *zap.Logger
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	capacity   float64
	rate       float64
}

func NewRateLimiter(redisClient *redis.Client, cfg *config.Config, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redisClient: redisClient,
		memoryStore: make(map[string]*tokenBucket),
		config:      cfg,
		logger:      logger,
	}
}

func (rl *RateLimiter) AllowTelegramAPI(ctx context.Context) bool {
	key := "rate_limit:telegram_api"
	return rl.allow(ctx, key, rl.config.RateLimit.TelegramAPI)
}

func (rl *RateLimiter) AllowGuestMessage(ctx context.Context, botID uuid.UUID, guestUserID int64) bool {
	key := fmt.Sprintf("rate_limit:guest:%s:%d", botID.String(), guestUserID)
	return rl.allow(ctx, key, rl.config.RateLimit.GuestMessage)
}

func (rl *RateLimiter) allow(ctx context.Context, key string, limitPerSecond int) bool {
	if rl.redisClient != nil {
		return rl.allowWithRedis(ctx, key, limitPerSecond)
	}
	return rl.allowWithMemory(key, limitPerSecond)
}

func (rl *RateLimiter) allowWithRedis(ctx context.Context, key string, limitPerSecond int) bool {
	now := time.Now()
	window := time.Second

	pipe := rl.redisClient.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now.Add(-window).UnixNano()))
	pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	pipe.Expire(ctx, key, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		rl.logger.Warn("Redis rate limit check failed, falling back to memory",
			zap.Error(err))
		return rl.allowWithMemory(key, limitPerSecond)
	}

	count, err := rl.redisClient.ZCard(ctx, key).Result()
	if err != nil {
		rl.logger.Warn("Redis rate limit check failed, falling back to memory",
			zap.Error(err))
		return rl.allowWithMemory(key, limitPerSecond)
	}

	return int64(count) <= int64(limitPerSecond)
}

func (rl *RateLimiter) allowWithMemory(key string, limitPerSecond int) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	bucket, exists := rl.memoryStore[key]

	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(limitPerSecond),
			lastUpdate: now,
			capacity:   float64(limitPerSecond),
			rate:       float64(limitPerSecond),
		}
		rl.memoryStore[key] = bucket
	}

	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	tokensToAdd := elapsed * bucket.rate
	bucket.tokens = min(bucket.capacity, bucket.tokens+tokensToAdd)
	bucket.lastUpdate = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}

	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
