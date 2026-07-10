package diagnose

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatalf("signature mismatch: got %q want %q", parts[2], want)
	}

	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header["alg"] != "HS256" || header["typ"] != "JWT" {
		t.Fatalf("unexpected header: %v", header)
	}

	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(rawPayload, &claims); err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	return claims
}

func TestDiagnoseSendsSignedJWTAndReturnsAnswer(t *testing.T) {
	const secret = "test-secret"
	var gotPath, gotToken string
	var gotReq chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"role": "assistant", "content": "  The health check failed.\n- Check the URL  "},
			"done":    true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "qwen3-coder:30b", secret, nil)
	answer, err := c.Diagnose(context.Background(), "promote failed: health check timeout")
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if answer != "The health check failed.\n- Check the URL" {
		t.Errorf("unexpected answer: %q", answer)
	}
	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}

	claims := verifyJWT(t, gotToken, secret)
	if claims["app"] != "ring promoter" {
		t.Errorf("app claim = %v", claims["app"])
	}
	exp, ok := claims["exp"].(float64)
	if !ok || time.Unix(int64(exp), 0).Before(time.Now()) {
		t.Errorf("exp claim missing or already expired: %v", claims["exp"])
	}

	if gotReq.Model != "qwen3-coder:30b" {
		t.Errorf("model = %q", gotReq.Model)
	}
	if gotReq.Stream {
		t.Error("stream should be false")
	}
	if len(gotReq.Messages) != 2 || gotReq.Messages[1].Content != "promote failed: health check timeout" {
		t.Errorf("unexpected messages: %+v", gotReq.Messages)
	}
}

func TestDiagnoseSurfacesGatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid api key"})
	}))
	defer srv.Close()

	c := New(srv.URL, "m", "s", nil)
	_, err := c.Diagnose(context.Background(), "report")
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("want gateway error, got %v", err)
	}
}

func TestDiagnoseRejectsEmptyAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "   "}})
	}))
	defer srv.Close()

	c := New(srv.URL, "m", "s", nil)
	if _, err := c.Diagnose(context.Background(), "report"); err == nil {
		t.Fatal("want error for empty answer")
	}
}
