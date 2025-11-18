package message

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"go-telegram-forwarder-bot/internal/config"
	"go.uber.org/zap"
)

type RetryHandler struct {
	config *config.Config
	logger *zap.Logger
}

func NewRetryHandler(cfg *config.Config, logger *zap.Logger) *RetryHandler {
	return &RetryHandler{
		config: cfg,
		logger: logger,
	}
}

func (rh *RetryHandler) Retry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < rh.config.Retry.MaxAttempts; i++ {
		err := fn()
		if err == nil {
			if i > 0 {
				rh.logger.Info("Operation succeeded after retries",
					zap.Int("attempt", i+1))
			}
			return nil
		}

		lastErr = err

		if !rh.isRetryableError(err) {
			rh.logger.Warn("Non-retryable error encountered",
				zap.Error(err))
			return err
		}

		if i < rh.config.Retry.MaxAttempts-1 {
			rh.logger.Debug("Retrying operation",
				zap.Int("attempt", i+1),
				zap.Int("max_attempts", rh.config.Retry.MaxAttempts),
				zap.Error(err))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rh.config.Retry.IntervalSeconds) * time.Second):
			}
		}
	}

	rh.logger.Warn("Max retries exceeded",
		zap.Int("attempts", rh.config.Retry.MaxAttempts),
		zap.Error(lastErr))
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (rh *RetryHandler) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	errStr := err.Error()
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") {
		return true
	}

	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
		return true
	}

	return false
}
