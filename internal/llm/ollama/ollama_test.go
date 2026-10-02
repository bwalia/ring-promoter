package ollama

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/llm"
)

const testSecret = "s3cret"

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// verifyJWT re-derives the HS256 signature the way the gateway would and
// returns the decoded payload claims.
func verifyJWT(t *testing.T, token, secret string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3: %q", len(parts), token)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)); parts[2] != want {
		t.Fatalf("signature mismatch: got %q want %q", parts[2], want)
	}
	var header map[string]string
	decodeSegment(t, parts[0], &header)
	if header["alg"] != "HS256" || header["typ"] != "JWT" {
		t.Fatalf("header = %v, want alg HS256 typ JWT", header)
	}
	var claims map[string]any
	decodeSegment(t, parts[1], &claims)
	return claims
}

func decodeSegment(t *testing.T, seg string, dst any) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		t.Fatalf("decode segment %q: %v", seg, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal segment: %v", err)
	}
}

func TestSignJWT(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token, err := signJWT(testSecret, now)
	if err != nil {
		t.Fatal(err)
	}
	claims := verifyJWT(t, token, testSecret)
	if claims["app"] != "ring promoter" {
		t.Errorf("app = %v", claims["app"])
	}
	if iat := claims["iat"].(float64); int64(iat) != now.Unix() {
		t.Errorf("iat = %v, want %d", iat, now.Unix())
	}
	if exp := claims["exp"].(float64); int64(exp) != now.Add(10*time.Minute).Unix() {
		t.Errorf("exp = %v, want iat+10m", exp)
	}

	other, _ := signJWT("different", now)
	if strings.Split(other, ".")[2] == strings.Split(token, ".")[2] {
		t.Error("signature does not depend on the secret")
	}
}

func TestCompleteSendsRequest(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("got %s %s, want POST /api/chat", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		verifyJWT(t, r.Header.Get("x-api-key"), testSecret)
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		io.WriteString(w, `{"message":{"role":"assistant","content":"  hello  "}}`)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "qwen3:30b", testSecret, quietLog())
	if c.Name() != "ollama" {
		t.Errorf("Name() = %q", c.Name())
	}
	res, err := c.Complete(context.Background(), llm.Request{
		System:      "be terse",
		Messages:    []llm.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}},
		Format:      json.RawMessage(`"json"`),
		Temperature: 0.2,
		MaxTokens:   256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello" {
		t.Errorf("Text = %q, want trimmed %q", res.Text, "hello")
	}

	if got.Model != "qwen3:30b" || got.Stream {
		t.Errorf("model/stream = %q/%v", got.Model, got.Stream)
	}
	wantMsgs := []chatMessage{{"system", "be terse"}, {"user", "hi"}, {"assistant", "yo"}}
	if len(got.Messages) != len(wantMsgs) {
		t.Fatalf("messages = %+v, want %+v", got.Messages, wantMsgs)
	}
	for i := range wantMsgs {
		if got.Messages[i] != wantMsgs[i] {
			t.Errorf("message %d = %+v, want %+v", i, got.Messages[i], wantMsgs[i])
		}
	}
	if string(got.Format) != `"json"` {
		t.Errorf("format = %s", got.Format)
	}
	if got.Options["temperature"] != 0.2 || got.Options["num_predict"] != float64(256) {
		t.Errorf("options = %v", got.Options)
	}
}

func TestCompleteOmitsOptionalFields(t *testing.T) {
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&raw)
		io.WriteString(w, `{"message":{"content":"ok"}}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "m", testSecret, nil).Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["format"]; ok {
		t.Error("format sent although empty")
	}
	var msgs []chatMessage
	json.Unmarshal(raw["messages"], &msgs)
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Errorf("messages = %+v, want only the user turn (no empty system)", msgs)
	}
	var opts map[string]any
	json.Unmarshal(raw["options"], &opts)
	if _, ok := opts["num_predict"]; ok {
		t.Error("num_predict sent although MaxTokens is 0")
	}
}

func TestCompleteErrors(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		unavailable bool
		wantErr     string
	}{
		{name: "5xx json error", status: 502, body: `{"error":"model loading"}`, unavailable: true, wantErr: "status 502: model loading"},
		{name: "5xx plain body", status: 503, body: "upstream down", unavailable: true, wantErr: "status 503: upstream down"},
		{name: "401 bad token", status: 401, body: `{"error":"invalid jwt"}`, wantErr: "status 401: invalid jwt"},
		{name: "404 unknown model", status: 404, body: "not found", wantErr: "status 404: not found"},
		{name: "200 garbage", status: 200, body: "<html>", wantErr: "decode ollama response"},
		{name: "200 empty answer", status: 200, body: `{"message":{"content":"   "}}`, wantErr: "empty answer"},
		{name: "4xx json without error field", status: 400, body: `{"message":{}}`, wantErr: `status 400: {"message":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := New(srv.URL, "m", testSecret, quietLog()).Complete(context.Background(), llm.Request{})
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
	url := srv.URL
	srv.Close() // nothing listens any more

	_, err := New(url, "m", testSecret, quietLog()).Complete(context.Background(), llm.Request{})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestCompleteRespectsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := New(srv.URL, "m", testSecret, quietLog()).Complete(ctx, llm.Request{})
	if !errors.Is(err, llm.ErrUnavailable) || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrUnavailable after deadline", err)
	}
}
