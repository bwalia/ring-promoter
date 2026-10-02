// Package ollama implements llm.Provider against an Ollama server sitting
// behind an auth gateway that expects an `x-api-key` header carrying a JWT
// signed with HS256 and a shared secret (the gateway verifies it with
// lua-resty-jwt). The token is minted fresh per request.
package ollama

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

	"github.com/example/ring-promoter/internal/llm"
)

// Client talks to one Ollama server. It implements llm.Provider.
type Client struct {
	baseURL string
	model   string
	secret  string
	http    *http.Client
	log     *slog.Logger
}

// New returns a provider for the Ollama server at baseURL (scheme + host, no
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

// Name implements llm.Provider.
func (c *Client) Name() string { return "ollama" }

// chat request/response wire types for POST /api/chat (stream=false).
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error"`
}

// Complete implements llm.Provider.
func (c *Client) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	token, err := signJWT(c.secret, time.Now())
	if err != nil {
		return llm.Response{}, fmt.Errorf("sign api token: %w", err)
	}

	msgs := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	options := map[string]any{"temperature": req.Temperature}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: msgs,
		Stream:   false,
		Format:   req.Format,
		Options:  options,
	})
	if err != nil {
		return llm.Response{}, fmt.Errorf("encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", token)

	start := time.Now()
	res, err := c.http.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("%w: call ollama: %v", llm.ErrUnavailable, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return llm.Response{}, fmt.Errorf("%w: read ollama response: %v", llm.ErrUnavailable, err)
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		if res.StatusCode != http.StatusOK {
			return llm.Response{}, statusErr(res.StatusCode, strings.TrimSpace(string(raw)))
		}
		return llm.Response{}, fmt.Errorf("decode ollama response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return llm.Response{}, statusErr(res.StatusCode, msg)
	}

	answer := strings.TrimSpace(out.Message.Content)
	if answer == "" {
		return llm.Response{}, fmt.Errorf("ollama returned an empty answer")
	}
	c.log.Info("llm completion produced", "provider", "ollama", "model", c.model,
		"duration_ms", time.Since(start).Milliseconds())
	return llm.Response{Text: answer}, nil
}

// statusErr maps an HTTP failure to an error, marking server-side (5xx)
// failures as ErrUnavailable — the gateway/model is down, not the request
// wrong. 4xx (bad auth, bad model name) stays a plain error: retrying or
// degrading will not help, configuration must change.
func statusErr(code int, msg string) error {
	if code >= 500 {
		return fmt.Errorf("%w: ollama returned status %d: %s", llm.ErrUnavailable, code, msg)
	}
	return fmt.Errorf("ollama returned status %d: %s", code, msg)
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
