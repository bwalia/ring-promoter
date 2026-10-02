package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/example/ring-promoter/internal/llm"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func message(stop, text string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",` +
		`"content":[{"type":"text","text":` + mustJSON(text) + `}],"stop_reason":"` + stop + `",` +
		`"usage":{"input_tokens":10,"output_tokens":5}}`
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// newTestClient points a provider at srv with SDK retries off, so error
// mapping is observed on the first response.
func newTestClient(srv *httptest.Server, model string) *Client {
	return New("test-key", model, quietLog(), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

func TestCompleteSendsRequest(t *testing.T) {
	var body map[string]json.RawMessage
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("got %s %s, want POST /v1/messages", r.Method, r.URL.Path)
		}
		hdr = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, message("end_turn", "  The health check failed.\n- Check the URL  "))
	}))
	defer srv.Close()

	c := newTestClient(srv, "")
	if c.Name() != "claude" {
		t.Errorf("Name() = %q", c.Name())
	}
	res, err := c.Complete(context.Background(), llm.Request{
		System:      "be terse",
		Messages:    []llm.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}, {Role: "user", Content: "why?"}},
		Temperature: 0.2,
		MaxTokens:   512,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "The health check failed.\n- Check the URL" {
		t.Errorf("Text = %q", res.Text)
	}

	if got := hdr.Get("X-Api-Key"); got != "test-key" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := hdr.Get("Anthropic-Beta"); !strings.Contains(got, "server-side-fallback-2026-07-01") {
		t.Errorf("anthropic-beta = %q, want the server-side fallback beta", got)
	}
	if string(body["fallbacks"]) != `"default"` {
		t.Errorf("fallbacks = %s", body["fallbacks"])
	}
	if string(body["model"]) != `"`+DefaultModel+`"` || string(body["max_tokens"]) != "512" {
		t.Errorf("model/max_tokens = %s/%s", body["model"], body["max_tokens"])
	}
	if _, ok := body["temperature"]; ok {
		t.Error("temperature sent; current models reject sampling parameters")
	}
	if _, ok := body["output_config"]; ok {
		t.Error("output_config sent without a schema")
	}
	var system []struct{ Text string }
	json.Unmarshal(body["system"], &system)
	if len(system) != 1 || system[0].Text != "be terse" {
		t.Errorf("system = %s", body["system"])
	}
	var msgs []struct {
		Role    string
		Content []struct{ Type, Text string }
	}
	json.Unmarshal(body["messages"], &msgs)
	if len(msgs) != 3 || msgs[0].Role != "user" || msgs[1].Role != "assistant" || msgs[2].Content[0].Text != "why?" {
		t.Errorf("messages = %s", body["messages"])
	}
}

func TestCompleteDefaultsAndFormat(t *testing.T) {
	var body map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, message("end_turn", `{"ok":true}`))
	}))
	defer srv.Close()
	c := newTestClient(srv, "claude-sonnet-5-5")
	user := []llm.Message{{Role: "user", Content: "hi"}}

	// The bare "json" hint has no API equivalent: nothing is sent for it.
	if _, err := c.Complete(context.Background(), llm.Request{Messages: user, Format: json.RawMessage(`"json"`)}); err != nil {
		t.Fatal(err)
	}
	if string(body["model"]) != `"claude-sonnet-5-5"` || string(body["max_tokens"]) != "16000" {
		t.Errorf("model/max_tokens = %s/%s, want configured model and default cap", body["model"], body["max_tokens"])
	}
	if _, ok := body["output_config"]; ok {
		t.Error(`output_config sent for the "json" hint`)
	}
	if _, ok := body["system"]; ok {
		t.Error("empty system prompt sent")
	}

	// A JSON Schema becomes a structured-output format.
	schema := `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`
	if _, err := c.Complete(context.Background(), llm.Request{Messages: user, Format: json.RawMessage(schema)}); err != nil {
		t.Fatal(err)
	}
	var oc struct {
		Format struct {
			Type   string         `json:"type"`
			Schema map[string]any `json:"schema"`
		} `json:"format"`
	}
	json.Unmarshal(body["output_config"], &oc)
	if oc.Format.Type != "json_schema" || oc.Format.Schema["type"] != "object" {
		t.Errorf("output_config = %s", body["output_config"])
	}
}

func TestCompleteRejectsUnknownRole(t *testing.T) {
	c := New("k", "", quietLog(), option.WithBaseURL("http://127.0.0.1:1"))
	_, err := c.Complete(context.Background(), llm.Request{Messages: []llm.Message{{Role: "tool", Content: "x"}}})
	if err == nil || !strings.Contains(err.Error(), `unsupported message role "tool"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompleteErrors(t *testing.T) {
	apiErr := func(typ, msg string) string {
		return `{"type":"error","error":{"type":"` + typ + `","message":"` + msg + `"}}`
	}
	cases := []struct {
		name        string
		status      int
		body        string
		unavailable bool
		wantErr     string
	}{
		{name: "overloaded 529", status: 529, body: apiErr("overloaded_error", "Overloaded"), unavailable: true, wantErr: "status 529"},
		{name: "rate limited 429", status: 429, body: apiErr("rate_limit_error", "slow down"), unavailable: true, wantErr: "status 429"},
		{name: "server error 500", status: 500, body: apiErr("api_error", "boom"), unavailable: true, wantErr: "status 500"},
		{name: "bad key 401", status: 401, body: apiErr("authentication_error", "invalid x-api-key"), wantErr: "status 401"},
		{name: "unknown model 404", status: 404, body: apiErr("not_found_error", "model: nope"), wantErr: "status 404"},
		{name: "refusal", status: 200, body: `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],` +
			`"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"x"},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantErr: "declined the request (cyber)"},
		{name: "empty answer", status: 200, body: message("end_turn", "   "), wantErr: "empty answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := newTestClient(srv, "").Complete(context.Background(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "hi"}}})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if got := errors.Is(err, llm.ErrUnavailable); got != tc.unavailable {
				t.Errorf("errors.Is(ErrUnavailable) = %v, want %v", got, tc.unavailable)
			}
		})
	}
}

func TestCompleteUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := newTestClient(srv, "").Complete(context.Background(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
