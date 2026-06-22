package adfilter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go-telegram-forwarder-bot/internal/config"

	"go.uber.org/zap"
)

type openAIJudge struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	model      string
	maxChars   int
	timeout    time.Duration
	logger     *zap.Logger
}

func newOpenAIJudge(cfg *config.LLMAdFilterConfig, httpClient *http.Client, logger *zap.Logger) Judge {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &openAIJudge{
		httpClient: httpClient,
		endpoint:   base + "/chat/completions",
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		maxChars:   cfg.MaxTextChars,
		timeout:    timeout,
		logger:     logger,
	}
}

type openAIRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	Messages    []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (j *openAIJudge) Judge(ctx context.Context, text string) (Result, error) {
	text = truncate(text, j.maxChars)

	body, err := json.Marshal(openAIRequest{
		Model:       j.model,
		MaxTokens:   64,
		Temperature: 0,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("marshal openai request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, j.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("build openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read openai response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("openai http %d: %s", resp.StatusCode, string(rawBody))
	}

	var decoded openAIResponse
	if err := json.Unmarshal(rawBody, &decoded); err != nil {
		return Result{}, fmt.Errorf("decode openai response: %w", err)
	}
	if decoded.Error != nil {
		return Result{}, fmt.Errorf("openai error: %s (%s)", decoded.Error.Message, decoded.Error.Type)
	}
	if len(decoded.Choices) == 0 {
		return Result{}, fmt.Errorf("openai response had no choices")
	}

	return parseDecision(decoded.Choices[0].Message.Content), nil
}
