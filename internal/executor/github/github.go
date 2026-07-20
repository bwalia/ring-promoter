// Package github implements the GitHub Actions execution backend: it starts a
// deployment by dispatching an existing workflow (workflow-dispatch API) and
// reports the run's lifecycle by polling its status. It is the backend for
// applications that are NOT on Kubernetes but already have a CI/CD pipeline
// that ships to their VMs — for example wslproxy, whose delivery pipeline
// builds the requested ref and rolls it out to the OpenResty hosts of a given
// environment.
//
// The version acted upon is passed as the workflow's version input (a git
// branch, tag or commit SHA that the pipeline checks out); the target
// environment comes from the RP_TARGET_ENV contract variable on the Spec.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// HTTPDoer is the subset of *http.Client used here; it lets tests substitute a
// fake transport.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures the GitHub Actions executor.
type Config struct {
	// Owner and Repo identify the repository hosting the workflow.
	Owner string
	Repo  string
	// Workflow is the workflow file name (e.g.
	// "deploy-wslproxy-delivery-pipeline.yml") or its numeric id.
	Workflow string
	// Ref is the git ref the workflow itself runs FROM (must be a branch or
	// tag, e.g. "build"). The version being deployed is a separate input.
	Ref string
	// VersionAsRef makes Start dispatch the workflow ON the deployed
	// version's git ref (branch or tag) instead of the static Ref. For
	// workflows that declare no version input and simply build whatever ref
	// they run from — e.g. ios_release.yml, where dispatching on a v* tag
	// releases that tag and dispatching on main releases main. The workflow
	// file must exist on every ref that can be deployed.
	VersionAsRef bool
	// Input names carried on the dispatch. They default to the wslproxy
	// deploy-single-environment.yml schema but are configurable so this
	// backend can drive any workflow-dispatch pipeline.
	//   EnvInput     <- RP_TARGET_ENV      (default "ENV")
	//   VersionInput <- RP_VERSION         (default "DEPLOY_BRANCH")
	//   ModeInput    <- DeployMode         (default "DEPLOY_MODE"; omitted if empty)
	//
	// Set any of these to the sentinel "-" to OMIT that input from the
	// dispatch entirely. This is needed for workflows whose schema does not
	// declare the input — GitHub rejects a dispatch carrying an undeclared
	// input with HTTP 422. For example spectoncr's deploy-spectoncr.yml has no
	// version or mode input, so it sets version_input and mode_input to "-".
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
	// RunLookupTimeout bounds how long Start waits for the dispatched run to
	// become visible via the API before giving up (default 60s).
	RunLookupTimeout time.Duration
	// ClockSkew is subtracted from the dispatch time when matching the run, to
	// tolerate clock differences with GitHub (default 60s).
	ClockSkew time.Duration
	// MaxRetries is how many extra attempts each API request makes on a
	// transport error (DNS/TLS/connection failure) before giving up. Transport
	// errors mean no response was received, so retrying is safe (default 3).
	MaxRetries int
	// RetryBackoff is the base linear backoff between transport-error retries
	// (attempt N waits N*RetryBackoff; default 2s).
	RetryBackoff time.Duration
}

// Executor implements executor.Executor for GitHub Actions.
type Executor struct {
	log  *slog.Logger
	http HTTPDoer
	cfg  Config
}

// New returns a GitHub Actions executor, filling defaults.
func New(log *slog.Logger, cfg Config, client HTTPDoer) *Executor {
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
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 2 * time.Second
	}
	return &Executor{log: log, http: client, cfg: cfg}
}

// PollInterval returns the resolved status poll cadence, for the adapter.
func (e *Executor) PollInterval() time.Duration { return e.cfg.PollInterval }

// Start implements executor.Executor: dispatch the workflow for the Spec's
// target environment and version, and locate the run it created.
func (e *Executor) Start(ctx context.Context, spec executor.Spec) (executor.Execution, error) {
	env := spec.Env[executor.EnvTargetEnv]
	version := spec.Env[executor.EnvVersion]
	if env == "" {
		return nil, fmt.Errorf("github deployer: no target_env configured for %s ring %s", spec.App, spec.Ring)
	}
	if e.cfg.Token == "" {
		return nil, fmt.Errorf("github deployer: no API token configured for %s", spec.App)
	}

	// Record when we dispatch so we can find the run we just created. Subtract
	// the skew to tolerate clock differences with GitHub's servers.
	since := time.Now().Add(-e.cfg.ClockSkew)

	e.log.Info("github deploy: dispatching workflow",
		"app", spec.App, "ring", spec.Ring, "env", env, "version", version,
		"workflow", e.cfg.Workflow, "ref", e.cfg.Ref)

	if err := e.dispatch(ctx, env, version); err != nil {
		return nil, fmt.Errorf("dispatch workflow: %w", err)
	}

	run, err := e.findRun(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("locate dispatched run: %w", err)
	}
	e.log.Info("github deploy: run started",
		"app", spec.App, "ring", spec.Ring, "run_id", run.ID, "url", run.HTMLURL)

	id := spec.ID
	if id == "" {
		id = "gh-" + strconv.FormatInt(run.ID, 10)
	}
	return &execution{e: e, id: id, runID: run.ID, url: run.HTMLURL}, nil
}

// execution is a handle on one dispatched workflow run.
type execution struct {
	e     *Executor
	id    string
	runID int64
	url   string
}

func (x *execution) ID() string { return x.id }

// Status implements executor.Execution by reading the run once and mapping
// GitHub's status/conclusion pair onto a phase.
func (x *execution) Status(ctx context.Context) (executor.Status, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d",
		x.e.cfg.APIBaseURL, x.e.cfg.Owner, x.e.cfg.Repo, x.runID)

	resp, err := x.e.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return executor.Status{}, err
	}
	var run Run
	if derr := decode(resp, &run); derr != nil {
		return executor.Status{}, derr
	}

	details := map[string]string{
		"run_id":  strconv.FormatInt(x.runID, 10),
		"run_url": x.url,
	}
	if run.Status != "completed" {
		phase := executor.PhaseRunning
		if run.Status == "queued" {
			phase = executor.PhasePending
		}
		return executor.Status{Phase: phase, Details: details}, nil
	}
	switch run.Conclusion {
	case "success":
		return executor.Status{Phase: executor.PhaseSucceeded, Details: details}, nil
	case "cancelled":
		return executor.Status{
			Phase:   executor.PhaseCancelled,
			Message: fmt.Sprintf("workflow run concluded %q (see %s)", run.Conclusion, run.HTMLURL),
			Details: details,
		}, nil
	default:
		return executor.Status{
			Phase:   executor.PhaseFailed,
			Message: fmt.Sprintf("workflow run concluded %q (see %s)", run.Conclusion, run.HTMLURL),
			Details: details,
		}, nil
	}
}

// Logs implements executor.Execution. GitHub exposes run logs only as a zip
// archive after the fact, so live streaming is not supported; step-level
// progress still reaches the UI via the operation's reporter.
func (x *execution) Logs(context.Context, executor.LogOptions) (io.ReadCloser, error) {
	return nil, executor.ErrLogsUnsupported
}

// Cancel implements executor.Execution via the run-cancel API (best effort).
func (x *execution) Cancel(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/cancel",
		x.e.cfg.APIBaseURL, x.e.cfg.Owner, x.e.cfg.Repo, x.runID)
	resp, err := x.e.do(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError("cancel run", resp)
	}
	return nil
}

// Cleanup implements executor.Execution; GitHub retains runs itself.
func (x *execution) Cleanup(context.Context) error { return nil }

// sendInput reports whether a dispatch input with the given configured name
// should be sent. The "-" sentinel means "omit". (An empty name is also
// treated as omit defensively, but note New defaults empty Env/Version/Mode
// input names to ENV/DEPLOY_BRANCH/DEPLOY_MODE, so a name only reaches here
// empty if a caller sets one explicitly — normal config uses "-".)
func sendInput(name string) bool { return name != "" && name != "-" }

// dispatch triggers the workflow via the workflow-dispatch API.
func (e *Executor) dispatch(ctx context.Context, env, version string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		e.cfg.APIBaseURL, e.cfg.Owner, e.cfg.Repo, url.PathEscape(e.cfg.Workflow))

	// An input whose configured name is the "-" sentinel is omitted: GitHub
	// 422s on any input the target workflow does not declare, so a workflow
	// lacking a version/mode input (e.g. spectoncr) sets those names to "-" and
	// only its declared inputs get sent. (Blank names were already defaulted by
	// the constructor, so they are sent, not omitted.)
	inputs := map[string]string{}
	if name := e.cfg.EnvInput; sendInput(name) {
		inputs[name] = env
	}
	if name := e.cfg.VersionInput; sendInput(name) {
		inputs[name] = version
	}
	if name := e.cfg.ModeInput; sendInput(name) && e.cfg.DeployMode != "" {
		inputs[name] = e.cfg.DeployMode
	}
	for k, v := range e.cfg.ExtraInputs {
		inputs[k] = v
	}

	// The ref the workflow runs from: static by default; the version itself
	// for VersionAsRef workflows (they build whatever ref they run on).
	ref := e.cfg.Ref
	if e.cfg.VersionAsRef && version != "" {
		ref = version
	}
	body, err := json.Marshal(map[string]any{
		"ref":    ref,
		"inputs": inputs,
	})
	if err != nil {
		return err
	}

	resp, err := e.do(ctx, http.MethodPost, endpoint, body)
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
func (e *Executor) findRun(ctx context.Context, since time.Time) (Run, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?event=workflow_dispatch&per_page=20",
		e.cfg.APIBaseURL, e.cfg.Owner, e.cfg.Repo, url.PathEscape(e.cfg.Workflow))

	deadline := time.Now().Add(e.cfg.RunLookupTimeout)
	// Poll a little faster than the run poll while we wait for the run to appear.
	lookupDelay := e.cfg.PollInterval
	if lookupDelay > 5*time.Second {
		lookupDelay = 5 * time.Second
	}

	for {
		var latest Run
		found := false

		resp, err := e.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return Run{}, err
		}
		var runs RunsResponse
		derr := decode(resp, &runs)
		if derr != nil {
			return Run{}, derr
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
			return Run{}, fmt.Errorf("no workflow_dispatch run appeared within %s", e.cfg.RunLookupTimeout)
		}
		if err := sleep(ctx, lookupDelay); err != nil {
			return Run{}, err
		}
	}
}

// do performs an authenticated GitHub API request, retrying on transport
// errors (DNS/TLS handshake/connection failures) with linear backoff. A
// transport error means the request never got a response, so retrying is safe
// even for the dispatch POST — e.g. a "TLS handshake timeout" reaching
// api.github.com is retried rather than failing the whole deploy.
func (e *Executor) do(ctx context.Context, method, endpoint string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
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
		req.Header.Set("Authorization", "Bearer "+e.cfg.Token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := e.http.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt >= e.cfg.MaxRetries {
			return nil, lastErr
		}
		backoff := time.Duration(attempt+1) * e.cfg.RetryBackoff
		e.log.Warn("github api transient error, retrying",
			"method", method, "attempt", attempt+1, "max", e.cfg.MaxRetries,
			"backoff", backoff.String(), "err", err)
		if serr := sleep(ctx, backoff); serr != nil {
			return nil, serr
		}
	}
}

// ---- git refs: the versions of a github-deployed app ----

// ErrRefNotFound is returned by ResolveRef when the ref does not exist in the
// repository.
var ErrRefNotFound = errors.New("ref not found in repository")

// Ref is one git ref (branch or tag) known to the repository.
type Ref struct {
	Name string
	Type string // "branch" | "tag"
}

// ListRefs returns the repository's branches and tags (branches first).
// Paginated up to a sane cap; a repo with more refs than that should be
// seeded by exact name/SHA instead.
func (e *Executor) ListRefs(ctx context.Context) ([]Ref, error) {
	if e.cfg.Token == "" {
		return nil, fmt.Errorf("github deployer: no API token configured")
	}
	branches, err := e.listRefs(ctx, "branches", "branch")
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	tags, err := e.listRefs(ctx, "tags", "tag")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return append(branches, tags...), nil
}

// listRefs pages through /repos/{owner}/{repo}/{branches|tags}.
func (e *Executor) listRefs(ctx context.Context, endpoint, typ string) ([]Ref, error) {
	const perPage, maxPages = 100, 3
	var out []Ref
	for page := 1; page <= maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/%s/%s?per_page=%d&page=%d",
			e.cfg.APIBaseURL, e.cfg.Owner, e.cfg.Repo, endpoint, perPage, page)
		resp, err := e.do(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		var refs []struct {
			Name string `json:"name"`
		}
		if err := decode(resp, &refs); err != nil {
			return nil, err
		}
		for _, r := range refs {
			out = append(out, Ref{Name: r.Name, Type: typ})
		}
		if len(refs) < perPage {
			break
		}
	}
	return out, nil
}

// ResolveRef returns nil when the ref resolves to a commit (covering branches,
// tags and abbreviated SHAs), ErrRefNotFound when it does not, and any other
// error when the repository could not be consulted.
func (e *Executor) ResolveRef(ctx context.Context, ref string) error {
	if e.cfg.Token == "" {
		return fmt.Errorf("github deployer: no API token configured")
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s",
		e.cfg.APIBaseURL, e.cfg.Owner, e.cfg.Repo, url.PathEscape(ref))
	resp, err := e.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	// GitHub answers 404 for unknown refs and 422 for malformed ones.
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity:
		return ErrRefNotFound
	default:
		return apiError("resolve version", resp)
	}
}

// Run is the subset of a GitHub Actions workflow run we consume.
type Run struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	Event      string    `json:"event"`
	HeadBranch string    `json:"head_branch"`
}

// RunsResponse is the run-list envelope.
type RunsResponse struct {
	WorkflowRuns []Run `json:"workflow_runs"`
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
