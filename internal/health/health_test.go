package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// serveStatus returns a test server that always responds with the given status.
func serveStatus(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPChecker_AnyTwoXX(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	ok := serveStatus(t, http.StatusNoContent) // 204
	if err := c.Check(context.Background(), ok.URL, 0); err != nil {
		t.Fatalf("204 should be healthy with expect=0 (any 2xx): %v", err)
	}
	bad := serveStatus(t, http.StatusUnauthorized) // 401
	if err := c.Check(context.Background(), bad.URL, 0); err == nil {
		t.Fatal("401 should be unhealthy with expect=0 (any 2xx)")
	}
}

// TestHTTPChecker_ExpectExactStatus covers the spectoncr case: a healthy
// registry answers /v2/ with 401, so only that exact code is healthy and a
// 2xx (or anything else) is treated as unhealthy.
func TestHTTPChecker_ExpectExactStatus(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)

	got401 := serveStatus(t, http.StatusUnauthorized)
	if err := c.Check(context.Background(), got401.URL, http.StatusUnauthorized); err != nil {
		t.Fatalf("401 should be healthy when expect=401: %v", err)
	}

	got200 := serveStatus(t, http.StatusOK)
	if err := c.Check(context.Background(), got200.URL, http.StatusUnauthorized); err == nil {
		t.Fatal("200 should be unhealthy when expect=401")
	}

	got404 := serveStatus(t, http.StatusNotFound)
	if err := c.Check(context.Background(), got404.URL, http.StatusUnauthorized); err == nil {
		t.Fatal("404 should be unhealthy when expect=401")
	}
}

func TestHTTPChecker_EmptyURL(t *testing.T) {
	c := NewHTTPChecker(time.Second)
	if err := c.Check(context.Background(), "", 401); err == nil {
		t.Fatal("empty url should error")
	}
}
