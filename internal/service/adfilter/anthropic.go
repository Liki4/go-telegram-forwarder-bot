package adfilter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go-telegram-forwarder-bot/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.uber.org/zap"
)

// adVerdictSchema constrains the model to output a structured JSON verdict.
// The BetaJSONSchemaOutputFormat helper strips unsupported JSON Schema keywords
// so the schema sent to the API is always compatible.
var adVerdictSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verdict": map[string]any{
			"type":        "string",
			"enum":        []string{"AD", "NORMAL"},
			"description": "AD if the text is unsolicited advertising/promotion, NORMAL otherwise",
		},
		"reason": map[string]any{
			"type":        "string",
			"description": "One short sentence (<=10 words) explaining why it is AD; empty string when NORMAL",
		},
	},
	"required":             []string{"verdict", "reason"},
	"additionalProperties": false,
}

// verdictResponse is the structured JSON the model returns.
type verdictResponse struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

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

	resp, err := j.client.Beta.Messages.New(callCtx, anthropic.BetaMessageNewParams{
		Model:       anthropic.Model(j.model),
		MaxTokens:   256,
		Temperature: anthropic.Float(0),
		Thinking: anthropic.BetaThinkingConfigParamUnion{
			OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
		},
		OutputFormat: anthropic.BetaJSONSchemaOutputFormat(adVerdictSchema),
		System: []anthropic.BetaTextBlockParam{{
			Text: systemPrompt,
		}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text)),
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("anthropic messages.new: %w", err)
	}

	// Fail-open: truncated response means the verdict is unreliable.
	if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		j.logger.Warn("llm ad judge: response truncated by max_tokens, treating as NORMAL",
			zap.String("model", j.model))
		return Result{IsAd: false}, nil
	}

	// Fail-open: model refused the request.
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		j.logger.Warn("llm ad judge: model refused, treating as NORMAL",
			zap.String("model", j.model))
		return Result{IsAd: false}, nil
	}

	// Extract the JSON text block.
	var rawJSON string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			rawJSON = tb.Text
			break
		}
	}
	if rawJSON == "" {
		j.logger.Warn("llm ad judge: no text block in response, treating as NORMAL",
			zap.String("model", j.model),
			zap.String("stop_reason", string(resp.StopReason)))
		return Result{IsAd: false}, nil
	}

	// Try structured JSON first.
	var vr verdictResponse
	if err := json.Unmarshal([]byte(rawJSON), &vr); err != nil {
		// Fall back to free-text parsing for models/gateways that don't
		// support structured output (e.g. DeepSeek via proxy).
		result := parseFreeTextVerdict(rawJSON)
		j.logger.Warn("llm ad judge: failed to parse response JSON, fell back to free-text",
			zap.String("model", j.model),
			zap.String("raw", rawJSON),
			zap.Bool("is_ad", result.IsAd),
			zap.Error(err))
		return result, nil
	}

	if vr.Verdict == "AD" {
		return Result{IsAd: true, Reason: vr.Reason, Raw: rawJSON}, nil
	}
	return Result{IsAd: false, Raw: rawJSON}, nil
}
