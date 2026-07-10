// Package health checks whether an application is healthy in a ring.
// Implementations are swappable: HTTPChecker performs a real HTTP GET, while
// AlwaysHealthy always succeeds and is used for local development and tests.
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Checker reports whether the endpoint at url is healthy. A nil error means
// healthy. expectStatus, when non-zero, is the exact HTTP status code that
// means healthy; zero means "any 2xx".
type Checker interface {
	Check(ctx context.Context, url string, expectStatus int) error
}

// AlwaysHealthy is a Checker that always reports healthy.
type AlwaysHealthy struct{}

// Check implements Checker.
func (AlwaysHealthy) Check(context.Context, string, int) error { return nil }

// HTTPChecker performs an HTTP GET and treats the response as healthy when it
// matches the expected status (any 2xx by default).
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

// Check implements Checker. When expectStatus is non-zero, only that exact
// status code is healthy; otherwise any 2xx is healthy.
func (c *HTTPChecker) Check(ctx context.Context, url string, expectStatus int) error {
	if url == "" {
		return fmt.Errorf("no health url configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()
	if expectStatus != 0 {
		if resp.StatusCode != expectStatus {
			return fmt.Errorf("unhealthy: status %d (want %d)", resp.StatusCode, expectStatus)
		}
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
