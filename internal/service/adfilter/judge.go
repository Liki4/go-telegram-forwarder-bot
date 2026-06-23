package adfilter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/utils"

	"go.uber.org/zap"
)

// Judge classifies a piece of text as advertising content or not.
// Implementations must be safe for concurrent use.
type Judge interface {
	Judge(ctx context.Context, text string) (Result, error)
}

type Result struct {
	IsAd   bool
	Reason string // model-supplied short reason when IsAd=true; empty otherwise
	Raw    string // raw first line returned by the model, for logging
}

// NewJudge builds a Judge from configuration. Returns (nil, nil) when the
// feature is not configured — callers should treat nil as "disabled globally".
func NewJudge(cfg *config.LLMAdFilterConfig, proxy *config.ProxyConfig, logger *zap.Logger) (Judge, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, nil
	}
	if cfg.APIKey == "" {
		return nil, errors.New("llm_ad_filter.api_key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("llm_ad_filter.model is required")
	}

	httpClient, err := utils.CreateHTTPClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("build http client for llm judge: %w", err)
	}

	switch strings.ToLower(cfg.Provider) {
	case "anthropic", "claude":
		return newAnthropicJudge(cfg, httpClient, logger)
	case "openai":
		return newOpenAIJudge(cfg, httpClient, logger), nil
	default:
		return nil, fmt.Errorf("unknown llm_ad_filter.provider %q (want \"anthropic\" or \"openai\")", cfg.Provider)
	}
}

// truncate returns s clipped to max runes (not bytes) to avoid breaking UTF-8.
func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
