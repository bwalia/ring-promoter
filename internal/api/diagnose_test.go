package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDiagnoser records the reports it receives and returns a canned answer.
// A non-nil block channel makes Diagnose wait until it is closed, so tests can
// hold a diagnosis in flight.
type fakeDiagnoser struct {
	calls  atomic.Int64
	report atomic.Value // string
	block  chan struct{}
}

func (f *fakeDiagnoser) Diagnose(_ context.Context, report string) (string, error) {
	f.calls.Add(1)
	f.report.Store(report)
	if f.block != nil {
		<-f.block
	}
	return "It failed because there was nothing to roll back.\n- Seed the ring first", nil
}

// startJob POSTs an async operation and returns the accepted job id.
func startJob(t *testing.T, h http.Handler, path, body string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", path, body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%s: status %d body %s", path, rec.Code, rec.Body)
	}
	var accepted struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("parse job id: %v", err)
	}
	return accepted.JobID
}

// pollJob polls the job until done returns true for its JSON body, failing the
// test on timeout.
func pollJob(t *testing.T, h http.Handler, id, what string, done func(body string) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := doJSON(t, h, "GET", "/api/apps/web/jobs/"+id, "")
		if done(rec.Body.String()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s: %s did not happen in time", id, what)
}

// waitForJob polls until the job reaches wantStatus, failing the test if it
// lands on the other terminal status first.
func waitForJob(t *testing.T, h http.Handler, id, wantStatus string) {
	t.Helper()
	pollJob(t, h, id, "status "+wantStatus, func(body string) bool {
		var job struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(body), &job); err != nil {
			t.Fatalf("parse job: %v", err)
		}
		if (job.Status == jobSuccess || job.Status == jobFailed) && job.Status != wantStatus {
			t.Fatalf("job reached %q, want %q", job.Status, wantStatus)
		}
		return job.Status == wantStatus
	})
}

// failJob starts an async rollback on an empty ring (guaranteed to fail) and
// waits for the job to reach the failed state, returning its id.
func failJob(t *testing.T, h http.Handler) string {
	t.Helper()
	id := startJob(t, h, "/api/apps/web/rollback?async=1", `{"ring":"int"}`)
	waitForJob(t, h, id, jobFailed)
	return id
}

func TestDiagnoseJob_RunsDetachedAndCaches(t *testing.T) {
	diag := &fakeDiagnoser{}
	h := newTestServerWithDiag(t, "", diag)
	id := failJob(t, h)

	// The first request starts the diagnosis and returns immediately.
	rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("diagnose: status %d body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"diagnosis_status":"running"`) {
		t.Fatalf("diagnose: body %s, want running status", rec.Body)
	}

	// The answer lands on the job (which the UI polls) once the model returns.
	pollJob(t, h, id, "diagnosis", func(body string) bool {
		return strings.Contains(body, "nothing to roll back") &&
			strings.Contains(body, `"diagnosis_status":"done"`)
	})

	// The report handed to the model names the app and the action.
	report, _ := diag.report.Load().(string)
	for _, want := range []string{"Application: web", "Action: rollback"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}

	// A later request is served straight from the cache: 200, no extra call.
	rec = doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "nothing to roll back") {
		t.Fatalf("cached diagnose: status %d body %s", rec.Code, rec.Body)
	}
	if got := diag.calls.Load(); got != 1 {
		t.Errorf("model called %d times, want 1", got)
	}
}

func TestDiagnoseJob_SingleFlight(t *testing.T) {
	diag := &fakeDiagnoser{block: make(chan struct{})}
	h := newTestServerWithDiag(t, "", diag)
	id := failJob(t, h)

	// Concurrent clicks while the model is thinking share ONE generation.
	for i := 0; i < 3; i++ {
		rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", "")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("diagnose %d: status %d body %s", i, rec.Code, rec.Body)
		}
	}
	close(diag.block)
	pollJob(t, h, id, "diagnosis", func(body string) bool {
		return strings.Contains(body, `"diagnosis_status":"done"`)
	})
	if got := diag.calls.Load(); got != 1 {
		t.Errorf("model called %d times, want 1", got)
	}
}

func TestDiagnoseJob_Guards(t *testing.T) {
	diag := &fakeDiagnoser{}
	h := newTestServerWithDiag(t, "", diag)

	// Unknown job.
	if rec := doJSON(t, h, "POST", "/api/apps/web/jobs/job-999/diagnose", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown job: status %d, want 404", rec.Code)
	}

	// Successful job: not diagnosable.
	id := startJob(t, h, "/api/apps/web/seed?async=1", `{"ring":"int","version":"v1"}`)
	waitForJob(t, h, id, jobSuccess)
	if rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", ""); rec.Code != http.StatusConflict {
		t.Errorf("successful job: status %d, want 409", rec.Code)
	}
	if got := diag.calls.Load(); got != 0 {
		t.Errorf("model called %d times, want 0", got)
	}

	// Feature not configured.
	h = newTestServer(t, "")
	id = failJob(t, h)
	if rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("no diagnoser: status %d, want 501", rec.Code)
	}
}
