// Command operator is a minimal Kubernetes operator for the Ring Promoter
// training academy. It manages a single CustomResource — Greeting
// (group training.ringpromoter.io, version v1, namespaced) — and, for each
// Greeting, ensures a ConfigMap of the same name exists in the same namespace
// carrying the greeting's spec.message.
//
// It is deliberately dependency-free (standard library only): instead of
// controller-runtime/client-go it talks to the Kubernetes API directly over
// net/http using the in-cluster REST config — the API server URL from the
// KUBERNETES_SERVICE_HOST/PORT environment, the CA bundle and the projected
// ServiceAccount token from /var/run/secrets/kubernetes.io/serviceaccount. This
// keeps the image tiny and the build hermetic while still demonstrating the
// operator reconcile pattern.
//
// It reconciles on a poll loop (no informers/watches) — the simplest thing that
// is genuinely level-triggered: every RECONCILE_INTERVAL it lists all Greetings
// and drives each one's ConfigMap toward the desired state.
//
// Endpoints on the health port (default :8080):
//
//	GET /healthz  -> {"status":"ok","version":"..."}   (liveness + version)
//	GET /metrics  -> Prometheus text exposition          (observability)
//
// The version comes from RP_VERSION (Ring Promoter / the Helm chart set it to
// the deployed tag) or the -X ldflag, falling back to "dev". Because /healthz
// echoes the version, a ring configured with `health_version_field: version`
// only passes once the promoted build is actually live.
package main

import (
	"github.com/bwalia/ring-promoter/training/operator/internal/controller"
)

// version is overridable at build time: go build -ldflags "-X main.version=v1.2.3".
// At runtime RP_VERSION wins so the same image reports the tag it was deployed as.
var version = "dev"

func main() {
	controller.Run(version)
}
