package deployer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/example/ring-promoter/internal/executor"
	ghexec "github.com/example/ring-promoter/internal/executor/github"
)

// httpDoer is the subset of *http.Client used by the GitHub backend; it lets
// tests substitute a fake transport.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// GitHubActionsConfig configures a GitHubActionsDeployer. The configuration
// lives with the GitHub execution backend; this alias keeps the deployer-level
// name stable.
type GitHubActionsConfig = ghexec.Config

// GitHubActionsDeployer deploys an application by triggering an existing
// GitHub Actions workflow and waiting for that run to complete. It is a thin
// composition over the execution abstraction: the GitHub backend
// (internal/executor/github) dispatches and tracks the run, and the embedded
// ExecDeployer adapts it to the Deployer contract — Deploy returns a non-nil
// error unless the run concludes "success", so Ring Promoter's health-check +
// auto-rollback logic works unchanged.
//
// It additionally implements VersionSource: the deployable versions of a
// github-deployed app are the repository's git refs.
type GitHubActionsDeployer struct {
	*ExecDeployer
	gh    *ghexec.Executor
	owner string
	repo  string
}

// NewGitHubActionsDeployer returns a GitHubActionsDeployer, filling defaults.
func NewGitHubActionsDeployer(log *slog.Logger, cfg GitHubActionsConfig, client httpDoer) *GitHubActionsDeployer {
	if log == nil {
		log = slog.Default()
	}
	ex := ghexec.New(log, cfg, client)

	// Map the promotion vocabulary onto the runner contract: the GitHub
	// backend reads the target environment and version from the Spec's env.
	specFor := func(t Target, version string) (executor.Spec, error) {
		return executor.Spec{
			App:  t.App,
			Ring: t.Ring,
			Env: map[string]string{
				executor.EnvTargetEnv: t.TargetEnv,
				executor.EnvVersion:   version,
			},
		}, nil
	}

	return &GitHubActionsDeployer{
		ExecDeployer: FromExecutor(log, ex, specFor, ex.PollInterval()),
		gh:           ex,
		owner:        cfg.Owner,
		repo:         cfg.Repo,
	}
}

// ListVersions implements VersionSource: the deployable versions are the
// repository's branches and tags (branches first).
func (d *GitHubActionsDeployer) ListVersions(ctx context.Context) ([]Version, error) {
	refs, err := d.gh.ListRefs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Version, 0, len(refs))
	for _, r := range refs {
		out = append(out, Version{Name: r.Name, Type: r.Type})
	}
	return out, nil
}

// ValidateVersion implements VersionSource: a version is valid when GitHub can
// resolve it to a commit — which covers branches, tags and (abbreviated) SHAs.
func (d *GitHubActionsDeployer) ValidateVersion(ctx context.Context, version string) error {
	err := d.gh.ResolveRef(ctx, version)
	if errors.Is(err, ghexec.ErrRefNotFound) {
		return fmt.Errorf("%w: %q does not resolve in %s/%s",
			ErrVersionNotFound, version, d.owner, d.repo)
	}
	return err
}
