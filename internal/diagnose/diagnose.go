// Package diagnose asks an LLM (an Ollama server) to explain, in simple
// language, why a seed/promote/rollback failed and how to fix it.
//
// The Ollama server sits behind an auth gateway that expects an `x-api-key`
// header carrying a JWT signed with HS256 and a shared secret (the gateway
// verifies it with lua-resty-jwt). The token is minted fresh per request.
package diagnose

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// systemPrompt frames the model as a deployment-failure explainer. Plain text
// is requested because the UI renders the answer verbatim (no markdown).
const systemPrompt = `You are a deployment assistant for Ring Promoter, a tool that promotes application versions through deployment rings (int -> test -> acc -> prod). A deployment operation (seed, promote or rollback) has FAILED and you are given its failure report: the action, the error, and the step-by-step logs.

Explain to an operator who is not a deployment expert:
1. WHY it failed, in simple language (one or two short sentences naming the most likely root cause found in the logs).
2. HOW to fix it (2-4 concrete suggestions, most likely fix first).

Rules: be brief and specific to the evidence in the report. Plain text only - no markdown symbols like **, #, or backticks. Start fix suggestions on new lines prefixed with "- ".`

// Client talks to one Ollama server.
type Client struct {
	baseURL string
	model   string
	secret  string
	http    *http.Client
	log     *slog.Logger
}

// New returns a client for the Ollama server at baseURL (scheme + host, no
// trailing path) using the given model. secret signs the per-request JWT.
func New(baseURL, model, secret string, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		secret:  secret,
		// A 30B model on a shared workstation can take a while to load and
		// answer; the caller's context can always cancel earlier.
		http: &http.Client{Timeout: 3 * time.Minute},
		log:  log,
	}
}

// chat request/response wire types for POST /api/chat (stream=false).
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error"`
}

// Diagnose sends the failure report to the model and returns its plain-text
// explanation.
func (c *Client) Diagnose(ctx context.Context, report string) (string, error) {
	token, err := signJWT(c.secret, time.Now())
	if err != nil {
		return "", fmt.Errorf("sign api token: %w", err)
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: report},
		},
		Stream: false,
		// Low temperature: we want a grounded explanation, not creativity.
		Options: map[string]any{"temperature": 0.2},
	})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", token)

	start := time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("ollama returned status %d", res.StatusCode)
		}
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return "", fmt.Errorf("ollama returned status %d: %s", res.StatusCode, msg)
	}

	answer := strings.TrimSpace(out.Message.Content)
	if answer == "" {
		return "", fmt.Errorf("ollama returned an empty answer")
	}
	c.log.Info("ai diagnosis produced", "model", c.model, "duration_ms", time.Since(start).Milliseconds())
	return answer, nil
}

// signJWT mints a short-lived HS256 JWT identifying this service — the value
// the auth gateway in front of Ollama expects in the x-api-key header.
func signJWT(secret string, now time.Time) (string, error) {
	header := map[string]string{"typ": "JWT", "alg": "HS256"}
	payload := map[string]any{
		"app": "ring promoter",
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
	}

	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(h) + "." + enc.EncodeToString(p)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + enc.EncodeToString(mac.Sum(nil)), nil
}
