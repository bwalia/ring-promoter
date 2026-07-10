package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ghFake is an in-memory stand-in for the GitHub Actions REST API sufficient to
// exercise the dispatch → find-run → poll-run flow.
type ghFake struct {
	mu sync.Mutex

	// dispatched captures the inputs from the last dispatch call.
	dispatched   bool
	dispatchRef  string
	dispatchBody map[string]string

	// run status served by the run endpoint.
	conclusion string // "success" | "failure"
	// completeAfter is how many status polls return "in_progress" before
	// the run reports "completed".
	completeAfter int
	statusPolls   int
	// omitRun makes the runs list empty (simulating the run never appearing).
	omitRun bool
}

func newGHServer(t *testing.T, f *ghFake) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/o/r/actions/workflows/wf.yml/dispatches", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Ref    string            `json:"ref"`
			Inputs map[string]string `json:"inputs"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		f.mu.Lock()
		f.dispatched = true
		f.dispatchRef = payload.Ref
		f.dispatchBody = payload.Inputs
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/repos/o/r/actions/workflows/wf.yml/runs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		omit := f.omitRun
		f.mu.Unlock()
		resp := ghRunsResponse{}
		if !omit {
			resp.WorkflowRuns = []ghRun{{
				ID:        42,
				Status:    "queued",
				Event:     "workflow_dispatch",
				CreatedAt: time.Now(),
				HTMLURL:   "https://github.com/o/r/actions/runs/42",
			}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/repos/o/r/actions/runs/42", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.statusPolls++
		status := "completed"
		if f.statusPolls <= f.completeAfter {
			status = "in_progress"
		}
		concl := ""
		if status == "completed" {
			concl = f.conclusion
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(ghRun{
			ID: 42, Status: status, Conclusion: concl,
			HTMLURL: "https://github.com/o/r/actions/runs/42",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testGHDeployer(t *testing.T, baseURL string) *GitHubActionsDeployer {
	return NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml",
		Ref: "build", DeployMode: "full",
		Token:            "secret-token",
		APIBaseURL:       baseURL,
		PollInterval:     time.Millisecond,
		RunLookupTimeout: 2 * time.Second,
		ClockSkew:        time.Minute,
	}, nil)
}

func TestGitHub_Deploy_HappyPath(t *testing.T) {
	f := &ghFake{conclusion: "success", completeAfter: 2}
	srv := newGHServer(t, f)
	d := testGHDeployer(t, srv.URL)

	tgt := Target{App: "wslproxy", Ring: "acc", TargetEnv: "prod"}
	if err := d.Deploy(context.Background(), tgt, "release-1.0.10"); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.dispatched {
		t.Fatal("workflow was not dispatched")
	}
	if f.dispatchRef != "build" {
		t.Fatalf("dispatch ref = %q, want build", f.dispatchRef)
	}
	if f.dispatchBody["DEPLOY_BRANCH"] != "release-1.0.10" {
		t.Fatalf("DEPLOY_BRANCH = %q", f.dispatchBody["DEPLOY_BRANCH"])
	}
	if f.dispatchBody["ENV"] != "prod" {
		t.Fatalf("ENV = %q, want prod", f.dispatchBody["ENV"])
	}
	if f.dispatchBody["DEPLOY_MODE"] != "full" {
		t.Fatalf("unexpected inputs: %+v", f.dispatchBody)
	}
	if _, ok := f.dispatchBody["TARGET_HOST"]; ok {
		t.Fatalf("TARGET_HOST should not be sent: %+v", f.dispatchBody)
	}
	if f.statusPolls < 3 {
		t.Fatalf("expected the run to be polled to completion, polls=%d", f.statusPolls)
	}
}

// TestGitHub_Deploy_OmitsSentinelInputs verifies the "-" sentinel drops the
// version and mode inputs from the dispatch (as spectoncr requires, whose
// workflow declares neither) while still sending the env input and any
// extra_inputs verbatim. GitHub 422s on undeclared inputs, so omission is the
// point.
func TestGitHub_Deploy_OmitsSentinelInputs(t *testing.T) {
	f := &ghFake{conclusion: "success"}
	srv := newGHServer(t, f)
	d := NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml",
		Ref:          "main",
		EnvInput:     "TARGET_ENV",
		VersionInput: "-", // omit: workflow has no version input
		ModeInput:    "-", // omit: workflow has no mode input
		ExtraInputs:  map[string]string{"FORCE": "true"},
		Token:        "secret-token",
		APIBaseURL:   srv.URL,
		PollInterval: time.Millisecond, RunLookupTimeout: 2 * time.Second,
		ClockSkew: time.Minute,
	}, nil)

	if err := d.Deploy(context.Background(), Target{App: "spectoncr", Ring: "acc", TargetEnv: "acc"}, "main"); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.dispatchBody["TARGET_ENV"]; got != "acc" {
		t.Fatalf("TARGET_ENV = %q, want acc", got)
	}
	if got := f.dispatchBody["FORCE"]; got != "true" {
		t.Fatalf("FORCE = %q, want true", got)
	}
	if _, ok := f.dispatchBody["DEPLOY_BRANCH"]; ok {
		t.Fatalf("DEPLOY_BRANCH should be omitted: %+v", f.dispatchBody)
	}
	if _, ok := f.dispatchBody["DEPLOY_MODE"]; ok {
		t.Fatalf("DEPLOY_MODE should be omitted: %+v", f.dispatchBody)
	}
	if len(f.dispatchBody) != 2 {
		t.Fatalf("expected exactly TARGET_ENV + FORCE, got %+v", f.dispatchBody)
	}
}

func TestGitHub_Deploy_FailedRunReturnsError(t *testing.T) {
	f := &ghFake{conclusion: "failure"}
	srv := newGHServer(t, f)
	d := testGHDeployer(t, srv.URL)

	err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "abc123")
	if err == nil {
		t.Fatal("expected an error when the run concludes failure")
	}
	if !strings.Contains(err.Error(), "failure") {
		t.Fatalf("error should mention the conclusion: %v", err)
	}
}

func TestGitHub_Deploy_MissingTargetEnv(t *testing.T) {
	d := testGHDeployer(t, "http://unused.invalid")
	err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "target_env") {
		t.Fatalf("expected target_env error, got %v", err)
	}
}

func TestGitHub_Deploy_MissingToken(t *testing.T) {
	d := NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml", APIBaseURL: "http://unused.invalid",
	}, nil)
	err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected token error, got %v", err)
	}
}

func TestGitHub_Deploy_RunNeverAppears(t *testing.T) {
	f := &ghFake{omitRun: true}
	srv := newGHServer(t, f)
	d := NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml",
		Token: "secret-token", APIBaseURL: srv.URL,
		PollInterval: time.Millisecond, RunLookupTimeout: 50 * time.Millisecond,
	}, nil)
	err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "locate dispatched run") {
		t.Fatalf("expected run-lookup timeout error, got %v", err)
	}
}

// flakyDoer returns a transport error for the first failN calls, then delegates
// to the wrapped transport (simulating transient TLS/connection failures).
type flakyDoer struct {
	failN int
	calls int
	inner httpDoer
}

func (f *flakyDoer) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.failN {
		return nil, fmt.Errorf("net/http: TLS handshake timeout")
	}
	return f.inner.Do(req)
}

func TestGitHub_Deploy_RetriesTransientErrors(t *testing.T) {
	f := &ghFake{conclusion: "success"}
	srv := newGHServer(t, f)
	// First 2 API calls fail at the transport layer, then succeed.
	flaky := &flakyDoer{failN: 2, inner: &http.Client{Timeout: 5 * time.Second}}
	d := NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml", Token: "secret-token",
		APIBaseURL:       srv.URL,
		PollInterval:     time.Millisecond,
		RunLookupTimeout: 2 * time.Second,
		ClockSkew:        time.Minute,
		MaxRetries:       3,
		RetryBackoff:     time.Millisecond,
	}, flaky)

	if err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "v1"); err != nil {
		t.Fatalf("deploy should succeed after transient retries: %v", err)
	}
	if !f.dispatched {
		t.Fatal("dispatch never reached the server despite retries")
	}
	if flaky.calls < 3 {
		t.Fatalf("expected retries (>=3 calls), got %d", flaky.calls)
	}
}

func TestGitHub_Deploy_GivesUpAfterMaxRetries(t *testing.T) {
	f := &ghFake{conclusion: "success"}
	srv := newGHServer(t, f)
	flaky := &flakyDoer{failN: 100, inner: &http.Client{}} // never recovers
	d := NewGitHubActionsDeployer(nil, GitHubActionsConfig{
		Owner: "o", Repo: "r", Workflow: "wf.yml", Token: "secret-token",
		APIBaseURL: srv.URL, MaxRetries: 2, RetryBackoff: time.Millisecond,
	}, flaky)

	err := d.Deploy(context.Background(), Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "TLS handshake timeout") {
		t.Fatalf("expected transport error after exhausting retries, got %v", err)
	}
	if flaky.calls != 3 { // 1 initial + 2 retries
		t.Fatalf("expected exactly 3 dispatch attempts, got %d", flaky.calls)
	}
}

func TestGitHub_Deploy_ContextCancelled(t *testing.T) {
	f := &ghFake{conclusion: "success", completeAfter: 1_000_000} // never completes
	srv := newGHServer(t, f)
	d := testGHDeployer(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := d.Deploy(ctx, Target{App: "wslproxy", Ring: "int", TargetEnv: "int"}, "v1")
	if err == nil {
		t.Fatal("expected an error when the context is cancelled mid-poll")
	}
}

// ---- VersionSource ----

// versionsGHServer serves the ref-listing and commit-resolution endpoints used
// by ListVersions / ValidateVersion.
func versionsGHServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/o/r/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"name":"main"},{"name":"release"}]`))
	})
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"name":"v1.2.3"}]`))
	})
	mux.HandleFunc("/repos/o/r/commits/", func(w http.ResponseWriter, r *http.Request) {
		ref := strings.TrimPrefix(r.URL.Path, "/repos/o/r/commits/")
		switch ref {
		case "main", "release", "v1.2.3", "abc1234":
			_, _ = w.Write([]byte(`{"sha":"abc1234def"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHub_ListVersions_BranchesThenTags(t *testing.T) {
	srv := versionsGHServer(t)
	d := testGHDeployer(t, srv.URL)

	got, err := d.ListVersions(context.Background())
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	want := []Version{
		{Name: "main", Type: "branch"},
		{Name: "release", Type: "branch"},
		{Name: "v1.2.3", Type: "tag"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d versions, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("version[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGitHub_ValidateVersion(t *testing.T) {
	srv := versionsGHServer(t)
	d := testGHDeployer(t, srv.URL)
	ctx := context.Background()

	for _, ok := range []string{"main", "v1.2.3", "abc1234"} {
		if err := d.ValidateVersion(ctx, ok); err != nil {
			t.Fatalf("ValidateVersion(%q): unexpected error %v", ok, err)
		}
	}
	err := d.ValidateVersion(ctx, "does-not-exist")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("ValidateVersion(unknown) = %v, want ErrVersionNotFound", err)
	}
}
