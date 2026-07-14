// Package health checks whether an application is healthy in a ring.
// Implementations are swappable: HTTPChecker performs a real HTTP GET, while
// AlwaysHealthy always succeeds and is used for local development and tests.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxVersionBody bounds how much of a health response body is read when
// extracting the reported version.
const maxVersionBody = 1 << 20 // 1 MiB

// Probe describes one health check: where to ask and what a healthy answer
// looks like. The zero values of ExpectStatus and the version fields mean
// "status-only": any 2xx passes and no version is verified.
type Probe struct {
	// URL is the endpoint to GET.
	URL string
	// ExpectStatus, when non-zero, is the exact HTTP status code that means
	// healthy; zero means "any 2xx".
	ExpectStatus int
	// WantVersion, when non-empty, is the version the endpoint must REPORT for
	// the check to pass — this is what catches an old version still serving
	// "200 OK" after a deploy. It is compared against the version extracted via
	// VersionField or VersionHeader; when neither is set it is ignored.
	WantVersion string
	// VersionField names the JSON field in the response body holding the
	// running version. Nested fields use a dotted path, e.g. "build.version"
	// for {"build":{"version":"v1"}}.
	VersionField string
	// VersionHeader names a response header holding the running version, as an
	// alternative to VersionField.
	VersionHeader string
}

// wantsVersion reports whether the probe verifies the reported version.
func (p Probe) wantsVersion() bool {
	return p.WantVersion != "" && (p.VersionField != "" || p.VersionHeader != "")
}

// Checker reports whether the endpoint described by the probe is healthy.
// A nil error means healthy (and, when the probe carries a version
// expectation, that the endpoint reports that version).
type Checker interface {
	Check(ctx context.Context, p Probe) error
}

// VersionReporter is an optional Checker capability: fetching the version the
// health endpoint reports itself to be running. Used for ref-pinned rings,
// where the concrete deployed version is only knowable AFTER the deploy (the
// pipeline decides what the ref ships), so instead of enforcing an expected
// version the promoter records the reported one.
type VersionReporter interface {
	// ReportedVersion checks the endpoint is healthy (status) and returns the
	// version extracted per the probe's VersionField / VersionHeader.
	ReportedVersion(ctx context.Context, p Probe) (string, error)
}

// AlwaysHealthy is a Checker that always reports healthy.
type AlwaysHealthy struct{}

// Check implements Checker.
func (AlwaysHealthy) Check(context.Context, Probe) error { return nil }

// HTTPChecker performs an HTTP GET and treats the response as healthy when it
// matches the expected status (any 2xx by default) and — when the probe asks
// for it — reports the wanted version.
type HTTPChecker struct {
	client *http.Client
}

// NewHTTPChecker returns an HTTPChecker with the given per-request timeout.
func NewHTTPChecker(timeout time.Duration) *HTTPChecker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPChecker{client: &http.Client{Timeout: timeout}}
}

// Check implements Checker.
func (c *HTTPChecker) Check(ctx context.Context, p Probe) error {
	resp, err := c.fetch(ctx, p)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !p.wantsVersion() {
		return nil
	}

	got, err := reportedVersion(resp, p)
	if err != nil {
		return fmt.Errorf("verify running version: %w", err)
	}
	if got != p.WantVersion {
		return fmt.Errorf("wrong version live: endpoint reports %q, want %q", got, p.WantVersion)
	}
	return nil
}

// ReportedVersion implements VersionReporter: a healthy-status GET whose
// result is the version the endpoint claims to be running.
func (c *HTTPChecker) ReportedVersion(ctx context.Context, p Probe) (string, error) {
	resp, err := c.fetch(ctx, p)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return reportedVersion(resp, p)
}

// fetch GETs the probe URL and validates the status; the caller must close the
// returned body.
func (c *HTTPChecker) fetch(ctx context.Context, p Probe) (*http.Response, error) {
	if p.URL == "" {
		return nil, fmt.Errorf("no health url configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("health request: %w", err)
	}
	if p.ExpectStatus != 0 {
		if resp.StatusCode != p.ExpectStatus {
			resp.Body.Close()
			return nil, fmt.Errorf("unhealthy: status %d (want %d)", resp.StatusCode, p.ExpectStatus)
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return resp, nil
}

// reportedVersion extracts the version the endpoint claims to be running,
// from the configured response header or JSON body field.
func reportedVersion(resp *http.Response, p Probe) (string, error) {
	if p.VersionHeader != "" {
		v := strings.TrimSpace(resp.Header.Get(p.VersionHeader))
		if v == "" {
			return "", fmt.Errorf("response header %q is missing or empty", p.VersionHeader)
		}
		return v, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVersionBody))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("response body is not JSON: %w", err)
	}
	cur := doc
	for _, part := range strings.Split(p.VersionField, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field %q not found in response body", p.VersionField)
		}
		if cur, ok = obj[part]; !ok {
			return "", fmt.Errorf("field %q not found in response body", p.VersionField)
		}
	}
	switch v := cur.(type) {
	case string:
		return strings.TrimSpace(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("field %q is not a string", p.VersionField)
	}
}
