package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSPreflightIsAnsweredBeforeAuth(t *testing.T) {
	h := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodOptions, "/api/apps/web/seed", nil)
	req.Header.Set("Origin", "https://rp-other.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://rp-other.example.com" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("credentials must never be allowed, got %q", got)
	}
}

func TestCORSStillRequiresTheToken(t *testing.T) {
	h := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	req.Header.Set("Origin", "https://rp-other.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a token", rec.Code)
	}
	// Readable by the other console, so it can tell a bad token from a dead host.
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("401 should still carry Allow-Origin")
	}

	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the token", rec.Code)
	}
}

func TestCORSPolicyParsing(t *testing.T) {
	cases := []struct {
		env, origin string
		want        bool
	}{
		{"", "https://a.example.com", true},
		{"*", "https://a.example.com", true},
		{"off", "https://a.example.com", false},
		{"https://a.example.com, https://b.example.com/", "https://b.example.com", true},
		{"https://a.example.com", "https://evil.example.com", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := ParseCORSOrigins(c.env).allows(c.origin); got != c.want {
			t.Errorf("ParseCORSOrigins(%q).allows(%q) = %v, want %v", c.env, c.origin, got, c.want)
		}
	}
}

func TestCORSLeavesTheUIAlone(t *testing.T) {
	h := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://rp-other.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("the UI itself should not be cross-origin readable")
	}
}
