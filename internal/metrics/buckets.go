package metrics

// Histogram bucket definitions, kept in one place so every duration/size metric
// uses a deliberate, documented bucket set rather than ad-hoc literals. Buckets
// are the main cardinality/cost lever on a histogram (each bucket is a series),
// so they are chosen per measurement domain.

// httpDurationBuckets covers sub-millisecond health checks up to slow API calls.
// Ring Promoter's HTTP surface is a small JSON API + an embedded SPA; most
// responses are single-digit milliseconds, with the occasional multi-second
// promote/rollback request.
var httpDurationBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
}

// sizeBuckets (bytes) for request/response bodies: 100B .. 10MB, powers-of-~10.
var sizeBuckets = []float64{
	100, 1_000, 10_000, 100_000, 1_000_000, 10_000_000,
}

// promotionDurationBuckets: a promotion hop is a deploy + a health-check retry
// loop. These run from a few seconds (fast rollout) to several minutes (image
// pull + readiness gates), so the buckets span 1s .. 30m.
var promotionDurationBuckets = []float64{
	1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800,
}

// deploymentDurationBuckets mirror promotions but bias slightly longer — a
// GitHub Actions workflow deploy includes runner scheduling latency.
var deploymentDurationBuckets = []float64{
	1, 5, 15, 30, 60, 120, 300, 600, 900, 1800, 3600,
}

// queueProcessingBuckets: end-to-end async job processing time, same order of
// magnitude as a promotion since a job usually wraps one.
var queueProcessingBuckets = []float64{
	0.5, 1, 5, 15, 30, 60, 120, 300, 600, 1800,
}

// apiClientBuckets for outbound Git-provider / Kubernetes API calls: fast
// control-plane calls (ms) up to long polls (60s+ workflow-run lookups).
var apiClientBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
}
