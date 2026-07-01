package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubActionsDeployer deploys an application by triggering an existing GitHub
// Actions workflow via the workflow-dispatch API and then waiting for that run
// to complete. It is the deploy mechanism for applications that are NOT on
// Kubernetes but already have a CI/CD pipeline that ships to their VMs — for
// example wslproxy, whose delivery pipeline builds the requested ref and rolls
// it out to the OpenResty hosts of a given environment (reloading OpenResty on
// each host).
//
// The version acted upon is passed as the workflow's DEPLOY_BRANCH input (a git
// branch, tag or commit SHA that the pipeline checks out). The ring is mapped
// to the environment via Target.TargetEnv, sent as the TARGET_ENV input.
//
// Deploy returns a non-nil error unless the dispatched run concludes with
// "success", so Ring Promoter's health-check + auto-rollback logic works
// unchanged: a failed pipeline (or a failed post-deploy health check) triggers
// a rollback that re-dispatches the pipeline for the previous version.
type GitHubActionsDeployer struct {
	log  *slog.Logger
	http httpDoer
	cfg  GitHubActionsConfig
}

// httpDoer is the subset of *http.Client used here; it lets tests substitute a
// fake transport.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// GitHubActionsConfig configures a GitHubActionsDeployer.
type GitHubActionsConfig struct {
	// Owner and Repo identify the repository hosting the workflow.
	Owner string
	Repo  string
	// Workflow is the workflow file name (e.g.
	// "deploy-wslproxy-delivery-pipeline.yml") or its numeric id.
	Workflow string
	// Ref is the git ref the workflow itself runs FROM (must be a branch or
	// tag, e.g. "build"). The version being deployed is a separate input.
	Ref string
	// Input names carried on the dispatch. They default to the wslproxy
	// deploy-single-environment.yml schema but are configurable so this
	// deployer can drive any workflow-dispatch pipeline.
	//   EnvInput     <- Target.TargetEnv   (default "ENV")
	//   VersionInput <- version            (default "DEPLOY_BRANCH")
	//   ModeInput    <- DeployMode         (default "DEPLOY_MODE"; omitted if empty)
	EnvInput     string
	VersionInput string
	ModeInput    string
	// DeployMode is the value sent as ModeInput (e.g. "full").
	DeployMode string
	// ExtraInputs are additional static inputs sent verbatim on every dispatch.
	ExtraInputs map[string]string
	// Token authenticates to the GitHub API (a PAT or App token). It should
	// come from a secret, never the config file.
	Token string
	// APIBaseURL is the GitHub API base (default https://api.github.com).
	APIBaseURL string
	// PollInterval is how often the run's status is polled (default 15s).
	PollInterval time.Duration
	// RunLookupTimeout bounds how long Deploy waits for the dispatched run to
	// become visible via the API before giving up (default 60s).
	RunLookupTimeout time.Duration
	// ClockSkew is subtracted from the dispatch time when matching the run, to
	// tolerate clock differences with GitHub (default 60s).
	ClockSkew time.Duration
}

// NewGitHubActionsDeployer returns a GitHubActionsDeployer, filling defaults.
func NewGitHubActionsDeployer(log *slog.Logger, cfg GitHubActionsConfig, client httpDoer) *GitHubActionsDeployer {
	if log == nil {
		log = slog.Default()
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.github.com"
	}
	cfg.APIBaseURL = strings.TrimRight(cfg.APIBaseURL, "/")
	if cfg.Ref == "" {
		cfg.Ref = "build"
	}
	if cfg.EnvInput == "" {
		cfg.EnvInput = "ENV"
	}
	if cfg.VersionInput == "" {
		cfg.VersionInput = "DEPLOY_BRANCH"
	}
	if cfg.ModeInput == "" {
		cfg.ModeInput = "DEPLOY_MODE"
	}
	if cfg.DeployMode == "" {
		cfg.DeployMode = "full"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 15 * time.Second
	}
	if cfg.RunLookupTimeout <= 0 {
		cfg.RunLookupTimeout = 60 * time.Second
	}
	if cfg.ClockSkew <= 0 {
		cfg.ClockSkew = 60 * time.Second
	}
	return &GitHubActionsDeployer{log: log, http: client, cfg: cfg}
}

// Deploy implements Deployer: dispatch the workflow for (TargetEnv, version)
// and wait for the resulting run to conclude successfully.
func (d *GitHubActionsDeployer) Deploy(ctx context.Context, t Target, version string) error {
	if t.TargetEnv == "" {
		return fmt.Errorf("github deployer: no target_env configured for %s ring %s", t.App, t.Ring)
	}
	if d.cfg.Token == "" {
		return fmt.Errorf("github deployer: no API token configured for %s", t.App)
	}

	// Record when we dispatch so we can find the run we just created. Subtract
	// the skew to tolerate clock differences with GitHub's servers.
	since := time.Now().Add(-d.cfg.ClockSkew)

	d.log.Info("github deploy: dispatching workflow",
		"app", t.App, "ring", t.Ring, "env", t.TargetEnv, "version", version,
		"workflow", d.cfg.Workflow, "ref", d.cfg.Ref)

	if err := d.dispatch(ctx, t.TargetEnv, version); err != nil {
		return fmt.Errorf("dispatch workflow: %w", err)
	}

	run, err := d.findRun(ctx, since)
	if err != nil {
		return fmt.Errorf("locate dispatched run: %w", err)
	}
	d.log.Info("github deploy: run started",
		"app", t.App, "ring", t.Ring, "run_id", run.ID, "url", run.HTMLURL)

	if err := d.waitForRun(ctx, run.ID); err != nil {
		return err
	}
	d.log.Info("github deploy: run succeeded",
		"app", t.App, "ring", t.Ring, "run_id", run.ID, "url", run.HTMLURL)
	return nil
}

// dispatch triggers the workflow via the workflow-dispatch API.
func (d *GitHubActionsDeployer) dispatch(ctx context.Context, env, version string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		d.cfg.APIBaseURL, d.cfg.Owner, d.cfg.Repo, url.PathEscape(d.cfg.Workflow))

	inputs := map[string]string{
		d.cfg.VersionInput: version,
		d.cfg.EnvInput:     env,
	}
	if d.cfg.ModeInput != "" && d.cfg.DeployMode != "" {
		inputs[d.cfg.ModeInput] = d.cfg.DeployMode
	}
	for k, v := range d.cfg.ExtraInputs {
		inputs[k] = v
	}

	body, err := json.Marshal(map[string]any{
		"ref":    d.cfg.Ref,
		"inputs": inputs,
	})
	if err != nil {
		return err
	}

	resp, err := d.do(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// A successful dispatch returns 204 No Content.
	if resp.StatusCode != http.StatusNoContent {
		return apiError("workflow dispatch", resp)
	}
	return nil
}

// findRun polls the workflow's run list until a workflow_dispatch run created
// at/after `since` appears, returning the newest such run.
func (d *GitHubActionsDeployer) findRun(ctx context.Context, since time.Time) (ghRun, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?event=workflow_dispatch&per_page=20",
		d.cfg.APIBaseURL, d.cfg.Owner, d.cfg.Repo, url.PathEscape(d.cfg.Workflow))

	deadline := time.Now().Add(d.cfg.RunLookupTimeout)
	// Poll a little faster than the run poll while we wait for the run to appear.
	lookupDelay := d.cfg.PollInterval
	if lookupDelay > 5*time.Second {
		lookupDelay = 5 * time.Second
	}

	for {
		var latest ghRun
		found := false

		resp, err := d.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return ghRun{}, err
		}
		var runs ghRunsResponse
		derr := decode(resp, &runs)
		if derr != nil {
			return ghRun{}, derr
		}
		for _, r := range runs.WorkflowRuns {
			if r.CreatedAt.Before(since) {
				continue
			}
			if !found || r.CreatedAt.After(latest.CreatedAt) || (r.CreatedAt.Equal(latest.CreatedAt) && r.ID > latest.ID) {
				latest = r
				found = true
			}
		}
		if found {
			return latest, nil
		}

		if time.Now().After(deadline) {
			return ghRun{}, fmt.Errorf("no workflow_dispatch run appeared within %s", d.cfg.RunLookupTimeout)
		}
		if err := sleep(ctx, lookupDelay); err != nil {
			return ghRun{}, err
		}
	}
}

// waitForRun polls a run until it completes, returning nil only when its
// conclusion is "success".
func (d *GitHubActionsDeployer) waitForRun(ctx context.Context, runID int64) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d",
		d.cfg.APIBaseURL, d.cfg.Owner, d.cfg.Repo, runID)

	for {
		resp, err := d.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		var run ghRun
		if derr := decode(resp, &run); derr != nil {
			return derr
		}
		if run.Status == "completed" {
			if run.Conclusion == "success" {
				return nil
			}
			return fmt.Errorf("workflow run concluded %q (see %s)", run.Conclusion, run.HTMLURL)
		}
		if err := sleep(ctx, d.cfg.PollInterval); err != nil {
			return err
		}
	}
}

// do performs an authenticated GitHub API request.
func (d *GitHubActionsDeployer) do(ctx context.Context, method, endpoint string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return d.http.Do(req)
}

// ghRun is the subset of a GitHub Actions workflow run we consume.
type ghRun struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	Event      string    `json:"event"`
	HeadBranch string    `json:"head_branch"`
}

type ghRunsResponse struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
}

// decode reads and JSON-decodes a 2xx response body, or turns a non-2xx into an
// error. It always closes the body.
func decode(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError("github api", resp)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// apiError builds an error including the status and a snippet of the body.
func apiError(what string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		return fmt.Errorf("%s: unexpected status %d", what, resp.StatusCode)
	}
	return fmt.Errorf("%s: status %d: %s", what, resp.StatusCode, msg)
}

// sleep waits for d or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
