package metrics

import "strings"

// Label helpers keep cardinality bounded and values consistent. The cardinal
// rule: a label value must come from a small, closed set (app names, ring names,
// provider kinds, gate names, statuses) — never from unbounded input like commit
// SHAs, image tags, user IDs, or raw URL paths.

// status collapses a Go error into a two-value outcome label. Prefer this over
// embedding err.Error() (unbounded) as a label.
func status(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

// boolStatus maps a success bool to the same closed outcome set.
func boolStatus(ok bool) string {
	if ok {
		return "success"
	}
	return "failure"
}

// Provider names the deployment backend behind an app/ring. Ring Promoter has no
// canary/blue-green/helm/kustomize strategy engine — the "strategy" is which
// deployer executes the rollout — so provider is derived from the configured
// deployer kind. Keeping this in one place means every metric agrees on the
// vocabulary.
func Provider(deployerKind string) string {
	switch strings.ToLower(strings.TrimSpace(deployerKind)) {
	case "kubectl":
		return "kubectl"
	case "github", "github_actions", "gha":
		return "github_actions"
	case "k8sjob", "k8s_job", "job":
		return "k8sjob"
	case "log", "":
		return "log"
	default:
		return "other"
	}
}

// Strategy maps a deployer kind to the rollout strategy it implements. Since the
// codebase models strategy implicitly (backend choice + per-ring ref pinning),
// this is a deliberate, documented mapping rather than a free-form label.
func Strategy(deployerKind string) string {
	switch Provider(deployerKind) {
	case "kubectl":
		return "rolling" // `kubectl set image` + `kubectl rollout status`
	case "github_actions":
		return "workflow" // GitHub Actions workflow_dispatch
	case "k8sjob":
		return "job" // one-shot Kubernetes Job
	case "log":
		return "noop"
	default:
		return "other"
	}
}

// clip bounds a would-be label to a known set, mapping anything unexpected to
// "other" so a surprise value can never explode series count.
func clip(value string, allowed ...string) string {
	for _, a := range allowed {
		if value == a {
			return value
		}
	}
	return "other"
}
