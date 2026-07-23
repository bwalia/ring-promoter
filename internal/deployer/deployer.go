// Package deployer performs the real work of rolling an application version out
// to a ring. Implementations are swappable: KubectlDeployer talks to k3s, while
// LogDeployer is a no-op used for local development and tests.
package deployer

import (
	"context"
	"errors"
)

// Target identifies what to deploy and where. It is derived from the
// per-(app, ring) configuration. The Kubernetes-oriented fields
// (Namespace/Deployment/Container/Image) are used by KubectlDeployer; VM/CI
// deployers such as GitHubActionsDeployer use TargetEnv instead. A given
// deployer simply ignores the fields that do not apply to it.
type Target struct {
	App        string
	Ring       string
	Namespace  string
	Deployment string
	Container  string
	// Image is the repository without a tag; the version is applied as the tag.
	Image string
	// TargetEnv is the deployment environment name a non-Kubernetes deployer
	// ships to (e.g. "int", "test", "prod"). It maps a ring onto the real
	// environment understood by the target system (for wslproxy, the
	// TARGET_ENV input of its CI/CD pipeline).
	TargetEnv string
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

// ErrVersionNotFound is returned by VersionSource.ValidateVersion when the
// requested version does not exist in the application's source repository.
var ErrVersionNotFound = errors.New("version not found in source repository")

// Version is one deployable version known to an application's source
// repository (a git branch or tag).
type Version struct {
	Name string `json:"name"`
	Type string `json:"type"` // "branch" | "tag"
}

// VersionSource is an optional capability: enumerating the versions that exist
// in the application's source repository and validating that a given version
// exists before it is deployed. Only deployers whose "version" maps onto a
// verifiable source (e.g. GitHubActionsDeployer, whose versions are git refs)
// implement it; for the rest the UI falls back to free-form input.
type VersionSource interface {
	// ListVersions returns the known branches and tags, branches first.
	ListVersions(ctx context.Context) ([]Version, error)
	// ValidateVersion returns nil when version resolves in the source repository
	// (a branch, tag or commit SHA), ErrVersionNotFound when it does not, and
	// any other error when the source could not be consulted.
	ValidateVersion(ctx context.Context, version string) error
}

// WithoutVersionSource returns a Deployer that deploys exactly like d but does
// not advertise the VersionSource capability, so callers skip both the version
// dropdown and the pre-deploy "does this version exist?" check.
//
// For an app whose promoted version is NOT a ref of the deployer's repository,
// that check is not a safety net — it is a guaranteed false negative that
// rejects every legitimate deploy. diytaxreturn-opsapi is the case: its
// workflow lives in diy-tax-return-uk while its version is a docker image tag
// built from bwalia/opsapi, so validating against the workflow's repo failed
// 100% of the time.
//
// The wrapper embeds the Deployer INTERFACE rather than the concrete type, so
// every optional capability is hidden, not just VersionSource. That is correct
// for GitHubActionsDeployer, which implements no other. Re-check this if a
// wrapped deployer ever gains one (e.g. LiveVersioner).
func WithoutVersionSource(d Deployer) Deployer { return versionOpaque{d} }

type versionOpaque struct{ Deployer }
