package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func serve(t *testing.T, mux http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func readyFlag(v bool) *atomic.Bool {
	b := &atomic.Bool{}
	b.Store(v)
	return b
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, rec.Body.String())
	}
	return body
}

// /healthz is what Ring Promoter's version-aware health check reads, so the
// version it reports must be exactly the one the server was started with.
func TestHealthzReportsVersion(t *testing.T) {
	rec := serve(t, newMux("v1.2.3", readyFlag(true)), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decode(t, rec)
	if body["status"] != "ok" || body["version"] != "v1.2.3" {
		t.Fatalf("body = %v, want status ok and version v1.2.3", body)
	}
}

// /healthz answers during warm-up too: liveness must not depend on readiness.
func TestHealthzDuringWarmUp(t *testing.T) {
	rec := serve(t, newMux("v1", readyFlag(false)), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 while warming up", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	cases := []struct {
		name   string
		ready  bool
		code   int
		status string
	}{
		{"warming up", false, http.StatusServiceUnavailable, "warming-up"},
		{"ready", true, http.StatusOK, "ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, newMux("v1", readyFlag(tc.ready)), http.MethodGet, "/readyz")
			if rec.Code != tc.code {
				t.Fatalf("status = %d, want %d", rec.Code, tc.code)
			}
			if got := decode(t, rec)["status"]; got != tc.status {
				t.Fatalf("status field = %q, want %q", got, tc.status)
			}
		})
	}
}

// Readiness flips without rebuilding the mux — the warm-up goroutine in main
// relies on the handler reading the flag on every request.
func TestReadyzFollowsFlag(t *testing.T) {
	ready := readyFlag(false)
	mux := newMux("v1", ready)
	if rec := serve(t, mux, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before warm-up: status = %d, want 503", rec.Code)
	}
	ready.Store(true)
	if rec := serve(t, mux, http.MethodGet, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("after warm-up: status = %d, want 200", rec.Code)
	}
}

func TestRootGreetsWithVersion(t *testing.T) {
	rec := serve(t, newMux("v9.9.9", readyFlag(true)), http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Hello from ring-promoter training", "version: v9.9.9", "host: "} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q does not contain %q", body, want)
		}
	}
}

func TestMetricsCountsRequests(t *testing.T) {
	mux := newMux("v2", readyFlag(true))
	for range 3 {
		serve(t, mux, http.MethodGet, "/")
	}
	// Probes are not traffic: health and readiness must not move the counter.
	serve(t, mux, http.MethodGet, "/healthz")
	serve(t, mux, http.MethodGet, "/readyz")

	rec := serve(t, mux, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want Prometheus text", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE helloworld_requests_total counter",
		"helloworld_requests_total 3\n",
		`helloworld_build_info{version="v2"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestUnknownMethodRejected(t *testing.T) {
	rec := serve(t, newMux("v1", readyFlag(true)), http.MethodPost, "/healthz")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz status = %d, want 405", rec.Code)
	}
}

func TestResolveVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })
	version = "v0.0.1-ldflag"

	t.Setenv("RP_VERSION", "")
	if got := resolveVersion(); got != "v0.0.1-ldflag" {
		t.Fatalf("without RP_VERSION: got %q, want the ldflag version", got)
	}
	t.Setenv("RP_VERSION", "v3.0.0")
	if got := resolveVersion(); got != "v3.0.0" {
		t.Fatalf("with RP_VERSION: got %q, want v3.0.0 (runtime wins)", got)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("HW_TEST_PORT", "")
	if got := envOr("HW_TEST_PORT", "8080"); got != "8080" {
		t.Fatalf("unset: got %q, want default", got)
	}
	t.Setenv("HW_TEST_PORT", "9090")
	if got := envOr("HW_TEST_PORT", "8080"); got != "9090" {
		t.Fatalf("set: got %q, want 9090", got)
	}
}
