package message

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"go-telegram-forwarder-bot/internal/config"
	"go.uber.org/zap"
)

func TestRetryHandler_Success(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	err := handler.Retry(ctx, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("Should succeed, got error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("Should succeed on first attempt, got %d attempts", attempts)
	}
}

func TestRetryHandler_RetryableError(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	// Create a proper network error that implements net.Error
	netErr := &net.OpError{
		Op:  "write",
		Net: "tcp",
		Err: &net.DNSError{
			Err:         "connection reset",
			Name:        "example.com",
			Server:      "8.8.8.8",
			IsTimeout:   false,
			IsTemporary: true,
		},
	}

	err := handler.Retry(ctx, func() error {
		attempts++
		if attempts < 3 {
			return netErr
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Should succeed after retries, got error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("Should succeed on 3rd attempt, got %d attempts", attempts)
	}
}

func TestRetryHandler_NonRetryableError(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	nonRetryableErr := errors.New("invalid input")
	err := handler.Retry(ctx, func() error {
		attempts++
		return nonRetryableErr
	})

	if err == nil {
		t.Fatal("Should return error for non-retryable error")
	}
	if attempts != 1 {
		t.Fatalf("Should not retry non-retryable error, got %d attempts", attempts)
	}
}

func TestRetryHandler_MaxAttemptsExceeded(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	// Use 5xx error which is retryable
	retryableErr := errors.New("500 Internal Server Error")
	err := handler.Retry(ctx, func() error {
		attempts++
		return retryableErr
	})

	if err == nil {
		t.Fatal("Should return error after max attempts")
	}
	if attempts != 3 {
		t.Fatalf("Should retry 3 times, got %d attempts", attempts)
	}
}

func TestRetryHandler_429Error(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	err := handler.Retry(ctx, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("429 Too Many Requests")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Should succeed after retrying 429 error, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("Should succeed on 2nd attempt, got %d attempts", attempts)
	}
}

func TestRetryHandler_5xxError(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     3,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx := context.Background()
	attempts := 0

	err := handler.Retry(ctx, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("500 Internal Server Error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Should succeed after retrying 5xx error, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("Should succeed on 2nd attempt, got %d attempts", attempts)
	}
}

func TestRetryHandler_ContextCancellation(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts:     10,
			IntervalSeconds: 1,
		},
	}
	logger := zap.NewNop()
	handler := NewRetryHandler(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	// Cancel context after first attempt
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := handler.Retry(ctx, func() error {
		attempts++
		return errors.New("500 Internal Server Error")
	})

	if err == nil {
		t.Fatal("Should return error when context is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Should return context.Canceled error, got: %v", err)
	}
}
