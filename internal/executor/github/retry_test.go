package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A 502 while polling a run used to end the deploy. On 2026-10-12 that
// reported "deploy to target failed: github api: status 502" for an acc deploy
// that had succeeded: the workflow was green, the pods had the new image, and
// the promoter recorded a failure.
func TestGetRetriesGitHubs5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"Server Error"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	}))
	defer srv.Close()

	e := New(nil, Config{Token: "t", APIBaseURL: srv.URL, RetryBackoff: time.Millisecond}, srv.Client())
	resp, err := e.do(context.Background(), http.MethodGet, srv.URL+"/runs/1", nil)
	if err != nil {
		t.Fatalf("gave up on a transient 502: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("made %d attempts, want 3", got)
	}
}

// The other half, and the reason this is not simply "retry every 5xx": a 502
// can come back after GitHub has already accepted the dispatch. Asking again
// would start a second deploy.
func TestDispatchPostIsNotRetriedOnA5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"Server Error"}`))
	}))
	defer srv.Close()

	e := New(nil, Config{Token: "t", APIBaseURL: srv.URL, RetryBackoff: time.Millisecond}, srv.Client())
	resp, err := e.do(context.Background(), http.MethodPost, srv.URL+"/dispatches", []byte(`{}`))
	if err != nil {
		t.Fatalf("do returned an error rather than the response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want the 502 handed back", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("dispatched %d times, want exactly 1 — a retry here is a second deploy", got)
	}
}

// Giving up still has to say what GitHub said, or the operator is told nothing.
func TestARelentless5xxStillReportsWhatGitHubSaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"Server Error"}`))
	}))
	defer srv.Close()

	e := New(nil, Config{Token: "t", APIBaseURL: srv.URL, RetryBackoff: time.Millisecond}, srv.Client())
	_, err := e.do(context.Background(), http.MethodGet, srv.URL+"/runs/1", nil)
	if err == nil {
		t.Fatal("a 503 that never clears must still fail")
	}
	for _, want := range []string{"503", "Server Error"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}
