package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// nowFunc is the clock the HTTP middleware times requests against. It is a
// package var (not a direct time.Now call) so tests can substitute a
// deterministic clock.
var nowFunc = time.Now

// HTTP server metrics. These instrument the inbound API + UI surface. The route
// label is the matched ServeMux *pattern* (e.g. "GET /api/apps/{app}/rings"),
// NOT the raw path — so `{app}`, `{id}`, `{ring}` wildcards collapse to a small,
// bounded set of series instead of one per app/id/ring value.

var (
	httpRequestsTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests handled, by method, matched route, and status code.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency in seconds, by method and matched route.",
			Buckets:   httpDurationBuckets,
		},
		[]string{"method", "route"},
	)

	httpRequestsInFlight = factory.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		},
	)

	httpResponseSize = factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response body size in bytes, by method and matched route.",
			Buckets:   sizeBuckets,
		},
		[]string{"method", "route"},
	)

	httpRequestSize = factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_size_bytes",
			Help:      "HTTP request body size in bytes (Content-Length), by method and matched route.",
			Buckets:   sizeBuckets,
		},
		[]string{"method", "route"},
	)
)

// responseRecorder captures the status code and response body size that the
// existing statusRecorder in the API package does not expose. It is deliberately
// minimal and allocation-free per request beyond the struct itself.
type responseRecorder struct {
	http.ResponseWriter
	status  int
	written int64
	wrote   bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		// Implicit 200 on first write without an explicit WriteHeader.
		r.status = http.StatusOK
		r.wrote = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

// Flush/Unwrap keep the recorder transparent to streaming handlers (the job
// progress endpoints stream). Unwrap lets http.ResponseController reach the
// underlying writer for Flush/Hijack.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// HTTPMiddleware instruments every request: count, duration, in-flight, request
// and response size, labelled by method + matched route + status.
//
// It is designed to wrap the OUTERMOST handler (the whole mux). Go 1.22's
// ServeMux populates Request.Pattern during routing, so reading r.Pattern()
// AFTER next.ServeHTTP returns yields the matched pattern for the route label.
// Unmatched requests (404s) report route="<unmatched>", keeping cardinality flat.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		startAt := nowFunc()

		next.ServeHTTP(rec, r)

		elapsed := nowFunc().Sub(startAt).Seconds()
		route := r.Pattern
		if route == "" {
			route = "<unmatched>"
		}
		method := r.Method
		code := strconv.Itoa(rec.status)

		httpRequestsTotal.WithLabelValues(method, route, code).Inc()
		httpRequestDuration.WithLabelValues(method, route).Observe(elapsed)
		httpResponseSize.WithLabelValues(method, route).Observe(float64(rec.written))
		if r.ContentLength > 0 {
			httpRequestSize.WithLabelValues(method, route).Observe(float64(r.ContentLength))
		}
	})
}
