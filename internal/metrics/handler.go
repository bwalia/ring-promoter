package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler returns the HTTP handler that renders the Prometheus text exposition
// for Ring Promoter's registry. Mount it (unauthenticated) at GET /metrics.
//
// It serves ONLY this package's dedicated Registry — the HTTP, promotion,
// runtime, and build collectors — never prometheus's global default registry.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		// A scrape failure should surface as an HTTP error the scraper can see,
		// not a partial body.
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}
