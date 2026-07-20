// Package metrics is Ring Promoter's observability layer. It owns a single
// Prometheus registry, the metric vectors for every subsystem (HTTP, promotion,
// deployment, queue, workers, Git providers, Kubernetes, storage, business), and
// the /metrics HTTP handler.
//
// Design goals:
//
//   - One registry, registered once. Everything hangs off the package-level
//     Registry so callers never thread a *prometheus.Registry through their
//     constructors — they import this package and call the typed helpers.
//   - Low cardinality. Labels are bounded (application, ring, provider, strategy,
//     status, normalized route) — never raw paths, commit SHAs, user IDs, or
//     free-form error strings. See label helpers in labels.go.
//   - Thread-safe and cheap. All metrics are prometheus vectors/collectors, which
//     are safe for concurrent use; instrumentation adds a map lookup + atomic add.
//   - Extensible toward OpenTelemetry. Call sites use small verbs on this package
//     (metrics.ObservePromotion, metrics.HTTPMiddleware, ...) rather than touching
//     prometheus types directly, so an OTLP/bridge backend can be swapped in
//     behind these functions without changing any call site.
//
// The metric vectors themselves are declared in the per-subsystem files
// (http.go, promotions.go, deployments.go, queue.go, providers.go,
// kubernetes.go, storage.go, business.go). This file owns the registry, the
// factory, build info, and the Go runtime collectors.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// namespace prefixes every metric: `ringpromoter_...`.
const namespace = "ringpromoter"

// Registry is the single registry Ring Promoter exposes on /metrics. It is a
// dedicated registry (not the global default) so tests can build an isolated
// instance and so we control exactly which collectors are present.
var Registry = prometheus.NewRegistry()

// factory builds and auto-registers metrics against Registry. Using promauto
// keeps declarations declarative (a var block per subsystem) while guaranteeing
// every metric is registered exactly once at package-init time.
var factory = promauto.With(Registry)

func init() {
	// Standard Go runtime + process collectors: goroutines, memory, GC, heap,
	// threads, CPU, and (on Linux) file descriptors. These satisfy the
	// "Runtime Metrics" section using the official collectors.
	Registry.MustRegister(
		collectors.NewGoCollector(
			collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
		),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// BuildInfo carries the build metadata surfaced as the ringpromoter_build_info
// gauge. It mirrors the fields the API already tracks so main can pass the same
// struct to both.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
	Platform  string
	Arch      string
}

// buildInfo is a constant-1 gauge whose label set carries the build metadata —
// the standard Prometheus "info metric" pattern. Build fields are low
// cardinality (they change only on redeploy), so exposing them as labels is
// safe and lets dashboards join on version.
var buildInfo = factory.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Ring Promoter build metadata. Always 1; read the labels.",
	},
	[]string{"version", "commit", "build_date", "go_version", "platform", "arch"},
)

// SetBuildInfo publishes the build metadata. Call once at startup.
func SetBuildInfo(b BuildInfo) {
	buildInfo.WithLabelValues(
		orUnknown(b.Version),
		orUnknown(b.Commit),
		orUnknown(b.BuildDate),
		orUnknown(b.GoVersion),
		orUnknown(b.Platform),
		orUnknown(b.Arch),
	).Set(1)
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
