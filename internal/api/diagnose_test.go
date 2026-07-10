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
type fakeDiagnoser struct {
	calls  atomic.Int64
	report atomic.Value // string
}

func (f *fakeDiagnoser) Diagnose(_ context.Context, report string) (string, error) {
	f.calls.Add(1)
	f.report.Store(report)
	return "It failed because there was nothing to roll back.\n- Seed the ring first", nil
}

// failJob starts an async rollback on an empty ring (guaranteed to fail) and
// waits for the job to reach the failed state, returning its id.
func failJob(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/apps/web/rollback?async=1", `{"ring":"int"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback: status %d body %s", rec.Code, rec.Body)
	}
	var accepted struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("parse job id: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := doJSON(t, h, "GET", "/api/apps/web/jobs/"+accepted.JobID, "")
		var job struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
			t.Fatalf("parse job: %v", err)
		}
		if job.Status == "failed" {
			return accepted.JobID
		}
		if job.Status == "success" {
			t.Fatal("job unexpectedly succeeded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not fail in time")
	return ""
}

func TestDiagnoseJob_ReturnsAndCachesDiagnosis(t *testing.T) {
	diag := &fakeDiagnoser{}
	h := newTestServerWithDiag(t, "", diag)
	id := failJob(t, h)

	rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnose: status %d body %s", rec.Code, rec.Body)
	}
	var out struct {
		Diagnosis string `json:"diagnosis"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse diagnosis: %v", err)
	}
	if !strings.Contains(out.Diagnosis, "nothing to roll back") {
		t.Errorf("unexpected diagnosis: %q", out.Diagnosis)
	}

	// The report handed to the model names the app and the action.
	report, _ := diag.report.Load().(string)
	for _, want := range []string{"Application: web", "Action: rollback"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}

	// A second request is served from the cache: no extra model call, and the
	// diagnosis also appears on the job itself for reloaded UIs.
	rec = doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("second diagnose: status %d", rec.Code)
	}
	if got := diag.calls.Load(); got != 1 {
		t.Errorf("model called %d times, want 1", got)
	}
	rec = doJSON(t, h, "GET", "/api/apps/web/jobs/"+id, "")
	if !strings.Contains(rec.Body.String(), "nothing to roll back") {
		t.Errorf("job JSON missing cached diagnosis: %s", rec.Body)
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
	rec := doJSON(t, h, "POST", "/api/apps/web/seed?async=1", `{"ring":"int","version":"v1"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("seed: status %d body %s", rec.Code, rec.Body)
	}
	var accepted struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("parse job id: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := doJSON(t, h, "GET", "/api/apps/web/jobs/"+accepted.JobID, "")
		if strings.Contains(rec.Body.String(), `"status":"success"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("seed job did not finish: %s", rec.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+accepted.JobID+"/diagnose", ""); rec.Code != http.StatusConflict {
		t.Errorf("successful job: status %d, want 409", rec.Code)
	}

	// Feature not configured.
	h = newTestServer(t, "")
	id := failJob(t, h)
	if rec := doJSON(t, h, "POST", "/api/apps/web/jobs/"+id+"/diagnose", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("no diagnoser: status %d, want 501", rec.Code)
	}
}
