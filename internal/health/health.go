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
// healthy.
type Checker interface {
	Check(ctx context.Context, url string) error
}

// AlwaysHealthy is a Checker that always reports healthy.
type AlwaysHealthy struct{}

// Check implements Checker.
func (AlwaysHealthy) Check(context.Context, string) error { return nil }

// HTTPChecker performs an HTTP GET and treats any 2xx response as healthy.
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
func (c *HTTPChecker) Check(ctx context.Context, url string) error {
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
