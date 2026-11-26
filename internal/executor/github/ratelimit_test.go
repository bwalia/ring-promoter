package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() { rateLimitFloor = 10 * time.Millisecond }

// On 2026-11-26 the shared PAT's quota ran dry and deploys failed with
// "github api: status 403: API rate limit exceeded". A rate-limited request did
// nothing, so the executor waits for the reset and asks again — the dispatch
// POST included.
func TestRateLimitedRequestWaitsForResetAndRetries(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if atomic.AddInt32(&calls, 1) == 1 {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Unix(), 10))
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for user ID 1."}`))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			// MaxRetries 1 proves the wait is not spent from the retry budget.
			e := New(nil, Config{Token: "rl-" + method, APIBaseURL: srv.URL, MaxRetries: 1, RetryBackoff: time.Millisecond}, srv.Client())
			resp, err := e.do(context.Background(), method, srv.URL+"/x", nil)
			if err != nil {
				t.Fatalf("rate limit was not waited out: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", resp.StatusCode)
			}
			if got := atomic.LoadInt32(&calls); got != 2 {
				t.Fatalf("made %d calls, want 2", got)
			}
		})
	}
}

// When the reset is further off than the executor will wait, the error says
// when the quota comes back, and the other apps sharing the token learn it
// without spending a request.
func TestRateLimitBeyondPatienceFailsWithResetTimeAndIsShared(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cfg := Config{Token: "rl-shared", APIBaseURL: srv.URL, RateLimitMaxWait: time.Minute}
	a := New(nil, cfg, srv.Client())
	_, err := a.do(context.Background(), http.MethodGet, srv.URL+"/x", nil)
	if err == nil || !strings.Contains(err.Error(), "rate limit exhausted") || !strings.Contains(err.Error(), "resets at") {
		t.Fatalf("err = %v, want a rate-limit error naming the reset", err)
	}
	b := New(nil, cfg, srv.Client())
	if _, err := b.do(context.Background(), http.MethodGet, srv.URL+"/y", nil); err == nil {
		t.Fatal("second executor on the same token was not held back")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("GitHub was asked %d times, want 1", got)
	}
}

// A 403 for a missing permission is not rate limiting and must not be waited on.
func TestPlainForbiddenIsNotRateLimiting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
	}))
	defer srv.Close()
	e := New(nil, Config{Token: "forbidden", APIBaseURL: srv.URL}, srv.Client())
	resp, err := e.do(context.Background(), http.MethodGet, srv.URL+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(b), "not accessible") {
		t.Fatalf("got %d %q, want the 403 and its message passed through", resp.StatusCode, b)
	}
}

// Polls carry the last ETag; GitHub's 304 (free against the quota) is served
// from the cached body.
func TestGetsAreConditionalAndReplay304(t *testing.T) {
	var conditional int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			atomic.AddInt32(&conditional, 1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"id":7,"status":"in_progress"}`))
	}))
	defer srv.Close()

	e := New(nil, Config{Token: "etag", APIBaseURL: srv.URL, Owner: "o", Repo: "r"}, srv.Client())
	x := &execution{e: e, runID: 7}
	for i := 0; i < 3; i++ {
		st, err := x.Status(context.Background())
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		if st.Phase != "running" {
			t.Fatalf("poll %d: phase = %q, want running", i, st.Phase)
		}
	}
	if got := atomic.LoadInt32(&conditional); got != 2 {
		t.Fatalf("%d conditional polls, want 2", got)
	}
}
