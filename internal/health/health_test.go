package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// serveHealthz returns a test server answering 200 with the given body and,
// when version is non-empty, an X-App-Version header.
func serveHealthz(t *testing.T, body, headerVersion string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if headerVersion != "" {
			w.Header().Set("X-App-Version", headerVersion)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPChecker_AnyTwoXX(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	ok := serveStatus(t, http.StatusNoContent) // 204
	if err := c.Check(context.Background(), Probe{URL: ok.URL}); err != nil {
		t.Fatalf("204 should be healthy with expect=0 (any 2xx): %v", err)
	}
	bad := serveStatus(t, http.StatusUnauthorized) // 401
	if err := c.Check(context.Background(), Probe{URL: bad.URL}); err == nil {
		t.Fatal("401 should be unhealthy with expect=0 (any 2xx)")
	}
}

// TestHTTPChecker_ExpectExactStatus covers the spectoncr case: a healthy
// registry answers /v2/ with 401, so only that exact code is healthy and a
// 2xx (or anything else) is treated as unhealthy.
func TestHTTPChecker_ExpectExactStatus(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)

	got401 := serveStatus(t, http.StatusUnauthorized)
	if err := c.Check(context.Background(), Probe{URL: got401.URL, ExpectStatus: http.StatusUnauthorized}); err != nil {
		t.Fatalf("401 should be healthy when expect=401: %v", err)
	}

	got200 := serveStatus(t, http.StatusOK)
	if err := c.Check(context.Background(), Probe{URL: got200.URL, ExpectStatus: http.StatusUnauthorized}); err == nil {
		t.Fatal("200 should be unhealthy when expect=401")
	}

	got404 := serveStatus(t, http.StatusNotFound)
	if err := c.Check(context.Background(), Probe{URL: got404.URL, ExpectStatus: http.StatusUnauthorized}); err == nil {
		t.Fatal("404 should be unhealthy when expect=401")
	}
}

func TestHTTPChecker_EmptyURL(t *testing.T) {
	c := NewHTTPChecker(time.Second)
	if err := c.Check(context.Background(), Probe{ExpectStatus: 401}); err == nil {
		t.Fatal("empty url should error")
	}
}

// TestHTTPChecker_VersionFromJSON is the core post-deploy scenario: a healthy
// status is NOT enough when the endpoint reports a different (old) version.
func TestHTTPChecker_VersionFromJSON(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	srv := serveHealthz(t, `{"status":"ok","version":"v2"}`, "")

	match := Probe{URL: srv.URL, VersionField: "version", WantVersion: "v2"}
	if err := c.Check(context.Background(), match); err != nil {
		t.Fatalf("matching version should be healthy: %v", err)
	}

	stale := Probe{URL: srv.URL, VersionField: "version", WantVersion: "v3"}
	err := c.Check(context.Background(), stale)
	if err == nil {
		t.Fatal("endpoint reporting v2 must fail a check wanting v3")
	}
	if !strings.Contains(err.Error(), `reports "v2"`) || !strings.Contains(err.Error(), `want "v3"`) {
		t.Fatalf("mismatch error should name both versions, got: %v", err)
	}
}

func TestHTTPChecker_VersionFromNestedJSON(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	srv := serveHealthz(t, `{"build":{"version":"abc123"}}`, "")
	p := Probe{URL: srv.URL, VersionField: "build.version", WantVersion: "abc123"}
	if err := c.Check(context.Background(), p); err != nil {
		t.Fatalf("nested field should resolve: %v", err)
	}
}

func TestHTTPChecker_VersionFromHeader(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	srv := serveHealthz(t, "ok", "v5")

	good := Probe{URL: srv.URL, VersionHeader: "X-App-Version", WantVersion: "v5"}
	if err := c.Check(context.Background(), good); err != nil {
		t.Fatalf("matching header version should be healthy: %v", err)
	}
	bad := Probe{URL: srv.URL, VersionHeader: "X-App-Version", WantVersion: "v6"}
	if err := c.Check(context.Background(), bad); err == nil {
		t.Fatal("header reporting v5 must fail a check wanting v6")
	}
}

// TestHTTPChecker_VersionSourceErrors: an endpoint that cannot prove its
// version (missing field/header, non-JSON body) fails a version-checked probe
// rather than silently passing.
func TestHTTPChecker_VersionSourceErrors(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)

	noField := serveHealthz(t, `{"status":"ok"}`, "")
	if err := c.Check(context.Background(), Probe{URL: noField.URL, VersionField: "version", WantVersion: "v1"}); err == nil {
		t.Fatal("missing JSON field should fail the check")
	}

	notJSON := serveHealthz(t, "plain ok", "")
	if err := c.Check(context.Background(), Probe{URL: notJSON.URL, VersionField: "version", WantVersion: "v1"}); err == nil {
		t.Fatal("non-JSON body should fail the check")
	}

	noHeader := serveHealthz(t, "ok", "")
	if err := c.Check(context.Background(), Probe{URL: noHeader.URL, VersionHeader: "X-App-Version", WantVersion: "v1"}); err == nil {
		t.Fatal("missing header should fail the check")
	}
}

// TestHTTPChecker_ReportedVersion: reading back which version the endpoint
// claims to run (used for ref-pinned rings after a deploy).
func TestHTTPChecker_ReportedVersion(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)

	srv := serveHealthz(t, `{"status":"ok","version":"v1.0.36"}`, "")
	got, err := c.ReportedVersion(context.Background(), Probe{URL: srv.URL, VersionField: "version"})
	if err != nil || got != "v1.0.36" {
		t.Fatalf("ReportedVersion = %q, %v; want v1.0.36", got, err)
	}

	down := serveStatus(t, http.StatusBadGateway)
	if _, err := c.ReportedVersion(context.Background(), Probe{URL: down.URL, VersionField: "version"}); err == nil {
		t.Fatal("unhealthy status must fail ReportedVersion")
	}
}

// TestHTTPChecker_VersionIgnoredWhenNotWanted: without a WantVersion (live
// dashboard checks) or without a configured source, only the status matters.
func TestHTTPChecker_VersionIgnoredWhenNotWanted(t *testing.T) {
	c := NewHTTPChecker(2 * time.Second)
	srv := serveHealthz(t, `{"version":"v1"}`, "")

	if err := c.Check(context.Background(), Probe{URL: srv.URL, VersionField: "version"}); err != nil {
		t.Fatalf("no WantVersion: status-only check should pass: %v", err)
	}
	if err := c.Check(context.Background(), Probe{URL: srv.URL, WantVersion: "v9"}); err != nil {
		t.Fatalf("no version source configured: status-only check should pass: %v", err)
	}
}
