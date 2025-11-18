package message

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/config"
	"go.uber.org/zap"
)

func TestRateLimiter_AllowTelegramAPI(t *testing.T) {
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			TelegramAPI:  5, // 5 per second
			GuestMessage: 1,
		},
	}
	logger := zap.NewNop()
	limiter := NewRateLimiter(nil, cfg, logger)

	ctx := context.Background()

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !limiter.AllowTelegramAPI(ctx) {
			t.Fatalf("Should allow request %d", i+1)
		}
	}

	// 6th request should be rate limited
	if limiter.AllowTelegramAPI(ctx) {
		t.Fatal("Should rate limit 6th request")
	}

	// Wait a bit and should allow again
	time.Sleep(1100 * time.Millisecond)
	if !limiter.AllowTelegramAPI(ctx) {
		t.Fatal("Should allow request after waiting")
	}
}

func TestRateLimiter_AllowGuestMessage(t *testing.T) {
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			TelegramAPI:  25,
			GuestMessage: 1, // 1 per second
		},
	}
	logger := zap.NewNop()
	limiter := NewRateLimiter(nil, cfg, logger)

	ctx := context.Background()
	botID := uuid.New()
	guestID := int64(123456)

	// First request should be allowed
	if !limiter.AllowGuestMessage(ctx, botID, guestID) {
		t.Fatal("Should allow first guest message")
	}

	// Second request immediately should be rate limited
	if limiter.AllowGuestMessage(ctx, botID, guestID) {
		t.Fatal("Should rate limit second guest message")
	}

	// Different guest should be allowed
	if !limiter.AllowGuestMessage(ctx, botID, int64(789012)) {
		t.Fatal("Should allow message from different guest")
	}

	// Wait and should allow again
	time.Sleep(1100 * time.Millisecond)
	if !limiter.AllowGuestMessage(ctx, botID, guestID) {
		t.Fatal("Should allow guest message after waiting")
	}
}

func TestRateLimiter_DifferentBots(t *testing.T) {
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			TelegramAPI:  25,
			GuestMessage: 1,
		},
	}
	logger := zap.NewNop()
	limiter := NewRateLimiter(nil, cfg, logger)

	ctx := context.Background()
	botID1 := uuid.New()
	botID2 := uuid.New()
	guestID := int64(123456)

	// Should allow for different bots
	if !limiter.AllowGuestMessage(ctx, botID1, guestID) {
		t.Fatal("Should allow message for bot 1")
	}
	if !limiter.AllowGuestMessage(ctx, botID2, guestID) {
		t.Fatal("Should allow message for bot 2")
	}
}
