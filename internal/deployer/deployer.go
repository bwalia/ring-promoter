// Package deployer performs the real work of rolling an application version out
// to a ring. Implementations are swappable: KubectlDeployer talks to k3s, while
// LogDeployer is a no-op used for local development and tests.
package deployer

import "context"

// Target identifies what to deploy and where. It is derived from the
// per-(app, ring) configuration.
type Target struct {
	App        string
	Ring       string
	Namespace  string
	Deployment string
	Container  string
	// Image is the repository without a tag; the version is applied as the tag.
	Image string
}

// Deployer rolls a version out to a target.
type Deployer interface {
	// Deploy sets the target Deployment's container image to Image:version and
	// waits for the rollout to become available. It returns an error if the
	// rollout does not succeed.
	Deploy(ctx context.Context, t Target, version string) error
}

// LiveVersioner is an optional capability: reporting the version currently
// running in the cluster (as opposed to the version we believe we deployed).
// Deployers that cannot introspect the cluster simply do not implement it.
type LiveVersioner interface {
	// LiveVersion returns the image tag currently set on the target Deployment.
	// An empty string means "unknown".
	LiveVersion(ctx context.Context, t Target) (string, error)
}
