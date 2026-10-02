// Package llm defines the provider-neutral interface Ring Promoter uses to
// talk to a large language model. Consumers (AI failure diagnosis today, the
// Ring Agent later) depend on this interface only; concrete providers live in
// subpackages (ollama now, others later) and are selected in main.
//
// Providers return plain text. Callers that need structured output pass a
// Format hint AND decode strictly with DecodeStrict — the typed decode is the
// real guarantee, the hint is advisory (not every backend enforces it).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnavailable wraps transport-level and server-side failures: the provider
// could not be reached or could not serve the request. Callers use it to
// distinguish "the model said something unusable" from "there is no model
// right now" — the latter must degrade safely (the agent falls back to
// deterministic-only behaviour; it never guesses).
var ErrUnavailable = errors.New("llm provider unavailable")

// Message is one turn of a conversation.
type Message struct {
	Role    string // "user" | "assistant"
	Content string
}

// Request is one completion call.
type Request struct {
	// System is the system prompt framing the task.
	System string
	// Messages is the conversation, oldest first.
	Messages []Message
	// Format, when non-empty, asks the provider for structured output (an
	// Ollama-style format value: a JSON Schema object, or the string "json").
	// Advisory — see the package comment.
	Format json.RawMessage
	// Temperature is the sampling temperature (0 = deterministic-ish).
	Temperature float64
	// MaxTokens caps the response length; 0 means the provider's default.
	MaxTokens int
}

// Response is a completion result.
type Response struct {
	Text string
}

// Provider is one way to reach a model.
type Provider interface {
	// Name identifies the provider for logs and audit detail (e.g. "ollama").
	Name() string
	// Complete performs one completion. Transport/server failures are wrapped
	// in ErrUnavailable; an empty answer is an error.
	Complete(ctx context.Context, req Request) (Response, error)
}

// DecodeStrict unmarshals a model's text answer into dst, rejecting unknown
// fields — the same convention the API layer applies to request bodies. It
// tolerates a Markdown code fence around the JSON, a common model tic.
func DecodeStrict(text string, dst any) error {
	trimmed := stripFence([]byte(text))
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode model output: %w", err)
	}
	// Anything after the first JSON value means the model kept talking.
	if dec.More() {
		return errors.New("decode model output: trailing content after JSON value")
	}
	return nil
}

// stripFence removes a surrounding ```/```json fence, if present.
func stripFence(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if !bytes.HasPrefix(b, []byte("```")) {
		return b
	}
	b = bytes.TrimPrefix(b, []byte("```"))
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:] // drop the language tag line ("json", ...)
	}
	b = bytes.TrimSuffix(bytes.TrimSpace(b), []byte("```"))
	return bytes.TrimSpace(b)
}
