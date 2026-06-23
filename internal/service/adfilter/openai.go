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

// adVerdictSchemaOpenAI is the JSON Schema passed to OpenAI's response_format.
// OpenAI strict mode forbids "description" inside the schema object, so we omit
// it here (unlike the Anthropic variant which supports it).
var adVerdictSchemaOpenAI = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verdict": map[string]any{
			"type": "string",
			"enum": []string{"AD", "NORMAL"},
		},
		"reason": map[string]any{
			"type": "string",
		},
	},
	"required":             []string{"verdict", "reason"},
	"additionalProperties": false,
}

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

type openAIResponseFormat struct {
	Type       string              `json:"type"` // "json_schema"
	JSONSchema openAIJSONSchemaDef `json:"json_schema"`
}

type openAIJSONSchemaDef struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIRequest struct {
	Model          string                `json:"model"`
	MaxTokens      int                   `json:"max_tokens"`
	Temperature    float64               `json:"temperature"`
	Seed           *int                  `json:"seed,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
	Messages       []openAIMessage       `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (j *openAIJudge) Judge(ctx context.Context, text string) (Result, error) {
	text = truncate(text, j.maxChars)

	seed := 0
	body, err := json.Marshal(openAIRequest{
		Model:       j.model,
		MaxTokens:   256,
		Temperature: 0,
		Seed:        &seed,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_schema",
			JSONSchema: openAIJSONSchemaDef{
				Name:   "ad_verdict",
				Strict: true,
				Schema: adVerdictSchemaOpenAI,
			},
		},
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

	choice := decoded.Choices[0]

	// Fail-open: truncated response means the verdict is unreliable.
	if choice.FinishReason == "length" {
		j.logger.Warn("llm ad judge (openai): response truncated by max_tokens, treating as NORMAL",
			zap.String("model", j.model))
		return Result{IsAd: false}, nil
	}

	rawJSON := strings.TrimSpace(choice.Message.Content)
	if rawJSON == "" {
		j.logger.Warn("llm ad judge (openai): empty message content, treating as NORMAL",
			zap.String("model", j.model),
			zap.String("finish_reason", choice.FinishReason))
		return Result{IsAd: false}, nil
	}

	var vr verdictResponse
	if err := json.Unmarshal([]byte(rawJSON), &vr); err != nil {
		j.logger.Warn("llm ad judge (openai): failed to parse response JSON, treating as NORMAL",
			zap.String("model", j.model),
			zap.String("raw", rawJSON),
			zap.Error(err))
		return Result{IsAd: false}, nil
	}

	if vr.Verdict == "AD" {
		return Result{IsAd: true, Reason: vr.Reason, Raw: rawJSON}, nil
	}
	return Result{IsAd: false, Raw: rawJSON}, nil
}
