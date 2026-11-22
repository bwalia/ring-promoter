package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// On 2026-11-22 daybook-website's acc deploy matched test's run: test's whole
// workflow took 49 seconds, so its run was still inside the 60s clock-skew
// window when acc was dispatched. Ring Promoter followed that finished run,
// health-checked acc before acc had deployed, and recorded a failure. A run that
// already existed before the dispatch must never be the one we follow.
func TestStartIgnoresARunThatExistedBeforeTheDispatch(t *testing.T) {
	prev := time.Now().Add(-20 * time.Second).UTC().Format(time.RFC3339)
	var dispatched, listsAfterDispatch int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dispatches"):
			atomic.StoreInt32(&dispatched, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/runs"):
			old := fmt.Sprintf(`{"id":100,"status":"completed","conclusion":"success","created_at":%q,"event":"workflow_dispatch"}`, prev)
			// GitHub lists the new run a moment after the dispatch is accepted.
			if atomic.LoadInt32(&dispatched) == 1 && atomic.AddInt32(&listsAfterDispatch, 1) > 1 {
				now := time.Now().UTC().Format(time.RFC3339)
				fmt.Fprintf(w, `{"workflow_runs":[{"id":101,"status":"queued","created_at":%q,"event":"workflow_dispatch"},%s]}`, now, old)
				return
			}
			fmt.Fprintf(w, `{"workflow_runs":[%s]}`, old)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	e := New(nil, Config{
		Token: "t", APIBaseURL: srv.URL, Owner: "o", Repo: "r", Workflow: "deploy.yml", Ref: "main",
		EnvInput: "TARGET_ENV", VersionInput: "IMAGE_TAG", ModeInput: "-",
		PollInterval: time.Millisecond, RunLookupTimeout: 5 * time.Second, RetryBackoff: time.Millisecond,
	}, srv.Client())

	x, err := e.Start(context.Background(), executor.Spec{
		App: "daybook-website", Ring: "acc",
		Env: map[string]string{executor.EnvTargetEnv: "acc", executor.EnvVersion: "96ce96ac10a0"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := x.(*execution).runID; got != 101 {
		t.Fatalf("followed run %d, want 101: run 100 existed before the dispatch (the previous ring's)", got)
	}
}
