// Package claude implements llm.Provider against the Claude Messages API using
// the official Anthropic Go SDK.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/example/ring-promoter/internal/llm"
)

// DefaultModel is used when no model is configured.
const DefaultModel = "claude-opus-5-5"

// defaultMaxTokens caps a response when the request does not. The API requires
// max_tokens; this keeps a non-streaming call well inside the SDK's timeouts.
const defaultMaxTokens = 16000

// Client talks to the Claude API. It implements llm.Provider.
type Client struct {
	api   anthropic.Client
	model string
	log   *slog.Logger
}

// New returns a provider for model (DefaultModel when empty) authenticated
// with apiKey. Extra options (e.g. option.WithBaseURL in tests) are passed to
// the SDK client.
func New(apiKey, model string, log *slog.Logger, opts ...option.RequestOption) *Client {
	if model == "" {
		model = DefaultModel
	}
	if log == nil {
		log = slog.Default()
	}
	opts = append([]option.RequestOption{
		option.WithAPIKey(apiKey),
		// If a safety classifier declines the request, let the API re-serve it
		// on a fallback model instead of stopping. "default" routes by refusal
		// category, so there is no model list to maintain.
		option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
		option.WithJSONSet("fallbacks", "default"),
	}, opts...)
	return &Client{api: anthropic.NewClient(opts...), model: model, log: log}
}

// Name implements llm.Provider.
func (c *Client) Name() string { return "claude" }

// Complete implements llm.Provider.
//
// Request.Temperature is not sent: current Claude models reject sampling
// parameters. Request.Format is honoured when it is a JSON Schema object
// (structured outputs); the bare "json" hint has no API equivalent, so it is
// left to the prompt and the caller's decode.
func (c *Client) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		block := anthropic.NewTextBlock(m.Content)
		switch m.Role {
		case "user":
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		case "assistant":
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		default:
			return llm.Response{}, fmt.Errorf("claude: unsupported message role %q", m.Role)
		}
	}
	if schema, ok := jsonSchema(req.Format); ok {
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: schema}
	}

	start := time.Now()
	msg, err := c.api.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, classify(err)
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return llm.Response{}, fmt.Errorf("claude declined the request (%s)", msg.StopDetails.Category)
	}

	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	answer := strings.TrimSpace(b.String())
	if answer == "" {
		return llm.Response{}, fmt.Errorf("claude returned an empty answer (stop reason %q)", msg.StopReason)
	}
	c.log.Info("llm completion produced", "provider", "claude", "model", msg.Model,
		"stop_reason", msg.StopReason, "input_tokens", msg.Usage.InputTokens,
		"output_tokens", msg.Usage.OutputTokens, "duration_ms", time.Since(start).Milliseconds())
	return llm.Response{Text: answer}, nil
}

// jsonSchema returns format as a JSON Schema object, if it is one.
func jsonSchema(format json.RawMessage) (map[string]any, bool) {
	var schema map[string]any
	if len(format) == 0 || json.Unmarshal(format, &schema) != nil || len(schema) == 0 {
		return nil, false
	}
	return schema, true
}

// classify maps an SDK error onto the llm conventions: rate limits (429),
// overload (529) and other server-side failures, plus transport errors, wrap
// llm.ErrUnavailable. Other 4xx (bad key, unknown model, invalid request)
// stay plain errors — only a config change fixes them. The SDK has already
// retried the retryable ones by the time we see them.
func classify(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: call claude: %v", llm.ErrUnavailable, err)
	}
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%w: call claude: %v", llm.ErrUnavailable, err)
	}
	if apiErr.StatusCode == 429 || apiErr.StatusCode >= 500 {
		return fmt.Errorf("%w: claude returned status %d: %v", llm.ErrUnavailable, apiErr.StatusCode, err)
	}
	return fmt.Errorf("claude returned status %d: %v", apiErr.StatusCode, err)
}
