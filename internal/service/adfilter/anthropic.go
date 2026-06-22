package adfilter

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go-telegram-forwarder-bot/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.uber.org/zap"
)

type anthropicJudge struct {
	client   anthropic.Client
	model    string
	maxChars int
	timeout  time.Duration
	logger   *zap.Logger
}

func newAnthropicJudge(cfg *config.LLMAdFilterConfig, httpClient *http.Client, logger *zap.Logger) (Judge, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(httpClient),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &anthropicJudge{
		client:   anthropic.NewClient(opts...),
		model:    cfg.Model,
		maxChars: cfg.MaxTextChars,
		timeout:  timeout,
		logger:   logger,
	}, nil
}

func (j *anthropicJudge) Judge(ctx context.Context, text string) (Result, error) {
	text = truncate(text, j.maxChars)

	callCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	resp, err := j.client.Messages.New(callCtx, anthropic.MessageNewParams{
		Model:     anthropic.Model(j.model),
		MaxTokens: 64,
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("anthropic messages.new: %w", err)
	}

	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return parseDecision(tb.Text), nil
		}
	}
	return Result{}, fmt.Errorf("anthropic response had no text block (stop_reason=%s)", resp.StopReason)
}
