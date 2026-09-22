// Package deployer performs the real work of rolling an application version out
// to a ring. Implementations are swappable: KubectlDeployer talks to k3s, while
// LogDeployer is a no-op used for local development and tests.
package deployer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	// Restart re-creates the target's running instances on the version they
	// already run (e.g. so pods pick up a rotated Secret) and waits for them to
	// become available again. It must not change the deployed version. It
	// returns ErrRestartUnsupported (wrapped) when the backend has no notion of
	// restarting in place.
	Restart(ctx context.Context, t Target, req RestartRequest) error
	// ValidateRestart checks, without side effects, that Restart could run for
	// this target and request: ErrRestartUnsupported when the backend cannot
	// restart, ErrInvalidDeployments when a named Deployment is not one this
	// deployer may restart. The promoter calls it before taking the lock, so
	// the async API path rejects such a request with a 4xx instead of a job.
	ValidateRestart(t Target, req RestartRequest) error
}

// RestartRequest carries what a restart needs beyond the Target.
type RestartRequest struct {
	// Version is the ring's current version — what is running and stays
	// running. Deployers that run a script pass it on (RP_VERSION).
	Version string
	// Deployments optionally narrows the restart to these Deployment names.
	// Empty = every Deployment the target covers.
	Deployments []string
}

// ErrRestartUnsupported is returned by Deployer.Restart when the deployment
// backend cannot restart a target without redeploying it (e.g. a CI workflow,
// or a k8sjob app with no restart script configured).
var ErrRestartUnsupported = errors.New("restart is not supported by this app's deployer")

// ErrInvalidDeployments rejects a restart's Deployment list: a name that is not
// a DNS-1123 label, too many names, or (kubectl) a name outside the ring's
// configured target.
var ErrInvalidDeployments = errors.New("invalid deployments")

// MaxRestartDeployments caps how many Deployments one restart may name.
const MaxRestartDeployments = 20

// dns1123Label matches a Kubernetes object name that is a DNS-1123 label.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateDeploymentNames checks a restart's Deployment list independent of any
// deployer: at most MaxRestartDeployments names, each a DNS-1123 label (so a
// name can never smuggle anything into a kubectl argument or a shell word).
func ValidateDeploymentNames(names []string) error {
	if len(names) > MaxRestartDeployments {
		return fmt.Errorf("%w: at most %d deployments per restart, got %d", ErrInvalidDeployments, MaxRestartDeployments, len(names))
	}
	for _, n := range names {
		if len(n) > 63 || !dns1123Label.MatchString(n) {
			return fmt.Errorf("%w: %q is not a valid deployment name (DNS-1123 label)", ErrInvalidDeployments, n)
		}
	}
	return nil
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
