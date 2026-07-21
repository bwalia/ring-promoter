package metrics

import "github.com/prometheus/client_golang/prometheus"

// Promotion metrics. A "promotion" here is any Ring Promoter operation that
// deploys a version into a ring and health-checks it — seed, promote, and
// rollback all funnel through the promoter and are counted the same way, keyed
// by the `action` label. Labels are bounded: application/ring come from config,
// action is one of a fixed set, and result is success|failure.

const (
	// ActionSeed / ActionPromote / ActionRollback are the fixed `action` label
	// values, matching Result.Action so dashboards line up with the API.
	ActionSeed     = "seed"
	ActionPromote  = "promote"
	ActionRollback = "rollback"
)

var (
	promotionsTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "promotion",
			Name:      "operations_total",
			Help:      "Promoter operations (seed/promote/rollback) by application, ring, action and result.",
		},
		[]string{"application", "ring", "action", "result"},
	)

	promotionDuration = factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "promotion",
			Name:      "duration_seconds",
			Help:      "Wall-clock duration of a promoter operation (deploy + health check).",
			Buckets:   promotionDurationBuckets,
		},
		[]string{"application", "ring", "action"},
	)
)

// ObservePromotion records one completed promoter operation. `ring` should be
// the affected (target) ring; pass "unknown" if it could not be determined
// (e.g. a validation error before a target was resolved) to keep cardinality
// bounded. `success` distinguishes the `result` label (success|failure).
func ObservePromotion(app, ring, action string, success bool, seconds float64) {
	app = orUnknown(app)
	ring = orUnknown(ring)
	action = orUnknown(action)
	result := "failure"
	if success {
		result = "success"
	}
	promotionsTotal.WithLabelValues(app, ring, action, result).Inc()
	promotionDuration.WithLabelValues(app, ring, action).Observe(seconds)
}
