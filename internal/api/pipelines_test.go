package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/pipeline"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// newPipelineServer configures db, web and ios in every ring (db pinned to a
// ref on int) and a release pipeline: data → qa gate → approval → web → ios.
func newPipelineServer(t *testing.T, prodPass string) (http.Handler, store.Store) {
	t.Helper()
	app := func(name, ref string) config.AppConfig {
		rings := map[string]config.RingConfig{}
		for _, r := range ring.Names() {
			rc := config.RingConfig{
				Namespace: r, Deployment: name, Container: name,
				Image: "repo/" + name, HealthURL: "health://" + name + "/" + r,
			}
			if r == "int" {
				rc.Ref = ref
			}
			rings[r] = rc
		}
		return config.AppConfig{Name: name, Rings: rings}
	}
	zero := 0
	delay := config.Duration(time.Millisecond)
	cfg := &config.Config{
		APIToken: "tok",
		Retry:    config.RetryConfig{Count: &zero, Delay: &delay},
		Apps:     []config.AppConfig{app("db", "main"), app("web", ""), app("ios", "")},
		Pipelines: []config.PipelineConfig{{
			Name: "release", DisplayName: "Release train", Description: "db, then web, then the iOS app",
			Stages: []config.PipelineStage{
				{Name: "data", DisplayName: "Database", Apps: []string{"db"}},
				{Name: "qa", Type: config.StageGate, Needs: []string{"data"}},
				{Name: "hold", Type: config.StageApproval, Needs: []string{"qa"}},
				{Name: "frontend", Apps: []string{"web"}, Needs: []string{"hold"}},
				{Name: "ios", Apps: []string{"ios"}, Needs: []string{"frontend"}},
			},
		}},
	}
	st := store.NewMemory()
	prom := promoter.New(cfg, st, nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)
	srv := NewServer(prom, "tok", prodPass, http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, nil)
	srv.pipes.Poll = 5 * time.Millisecond
	return srv.Handler(), st
}

func decodeRun(t *testing.T, body []byte) pipeline.Run {
	t.Helper()
	var r pipeline.Run
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode run: %v: %s", err, body)
	}
	return r
}

// pollRun re-reads a run until cond holds.
func pollRun(t *testing.T, h http.Handler, id string, cond func(pipeline.Run) bool) pipeline.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := doJSON(t, h, http.MethodGet, "/api/pipeline-runs/"+id, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get run: %d %s", rec.Code, rec.Body)
		}
		r := decodeRun(t, rec.Body.Bytes())
		if cond(r) {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %+v", r)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPipelines_List(t *testing.T) {
	h, _ := newPipelineServer(t, "")
	rec := doJSON(t, h, http.MethodGet, "/api/pipelines", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["name"] != "release" || got[0]["display_name"] != "Release train" {
		t.Fatalf("pipelines = %v", got)
	}
	if v, ok := got[0]["last_run"]; !ok || v != nil {
		t.Fatalf("last_run should be present and null before any run: %v", got[0])
	}
	stages := got[0]["stages"].([]any)
	qa := stages[1].(map[string]any)
	if qa["type"] != "gate" || qa["gate"] != "qa" || qa["timeout_seconds"].(float64) != 600 {
		t.Fatalf("qa stage = %v", qa)
	}
	if hold := stages[2].(map[string]any); hold["timeout_seconds"].(float64) != 86400 {
		t.Fatalf("approval stage = %v", hold)
	}
}

func TestPipelines_StartValidation(t *testing.T) {
	h, _ := newPipelineServer(t, "secret")
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/pipelines/nope/runs", `{"ring":"test"}`, http.StatusNotFound},
		{"/api/pipelines/release/runs", `{"ring":"moon"}`, http.StatusBadRequest},
		// web and ios have no int ref, so a seed run needs their versions.
		{"/api/pipelines/release/runs", `{"ring":"int","versions":{"web":"v1"}}`, http.StatusBadRequest},
		{"/api/pipelines/release/runs", `{"ring":"prod"}`, http.StatusForbidden},
		{"/api/pipelines/release/runs", `{"ring":"test","bogus":1}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := doJSON(t, h, http.MethodPost, c.path, c.body); rec.Code != c.want {
			t.Errorf("POST %s %s = %d, want %d (%s)", c.path, c.body, rec.Code, c.want, rec.Body)
		}
	}
	for _, path := range []string{"/api/pipeline-runs/run-9", "/api/pipelines/nope/runs"} {
		if rec := doJSON(t, h, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestPipelines_RunEndToEnd(t *testing.T) {
	h, st := newPipelineServer(t, "")
	rec := doJSON(t, h, http.MethodPost, "/api/pipelines/release/runs",
		`{"ring":"int","versions":{"web":"w1","ios":"i1"},"triggered_by":"ci"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	run := decodeRun(t, rec.Body.Bytes())
	if run.ID == "" || run.Ring != "int" || run.TriggeredBy != "ci" || len(run.Stages) != 5 {
		t.Fatalf("run = %+v", run)
	}
	if run.Stages[0].Items[0].Version != "main" {
		t.Fatalf("db version should come from its int ref: %+v", run.Stages[0].Items)
	}

	// The data stage deploys, then the QA gate waits for a verdict.
	r := pollRun(t, h, run.ID, func(r pipeline.Run) bool { return r.Stages[1].Status == pipeline.StatusWaiting })
	if r.Status != pipeline.StatusWaiting || r.Stages[0].Status != pipeline.StatusSuccess {
		t.Fatalf("run = %+v", r)
	}
	if jobID := r.Stages[0].Items[0].JobID; jobID == "" {
		t.Fatal("deploy item should link its job")
	} else if rec := doJSON(t, h, http.MethodGet, "/api/apps/db/jobs/"+jobID, ""); rec.Code != http.StatusOK ||
		!json.Valid(rec.Body.Bytes()) {
		t.Fatalf("job %s: %d", jobID, rec.Code)
	}

	// Approving before the gate passes is refused.
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+run.ID+"/stages/hold/approve", ""); rec.Code != http.StatusConflict {
		t.Fatalf("early approve = %d", rec.Code)
	}
	if err := st.UpsertQAReport(t.Context(), store.QAReport{App: "db", Ring: "int", WorkflowVerdict: "go", Source: "qa"}); err != nil {
		t.Fatal(err)
	}
	r = pollRun(t, h, run.ID, func(r pipeline.Run) bool { return r.Stages[2].Status == pipeline.StatusWaiting })
	if r.Stages[1].Items[0].Verdict != "go" {
		t.Fatalf("gate items = %+v", r.Stages[1].Items)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+run.ID+"/stages/nope/approve", "{}"); rec.Code != http.StatusNotFound {
		t.Fatalf("approve unknown stage = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+run.ID+"/stages/hold/approve", `{"by":"rel-eng","note":"ship it"}`); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body)
	}
	r = pollRun(t, h, run.ID, func(r pipeline.Run) bool { return r.FinishedAt != nil })
	if r.Status != pipeline.StatusSuccess || r.Stages[2].ApprovedBy != "rel-eng" {
		t.Fatalf("run = %+v", r)
	}
	if v := r.Stages[4].Items[0].Version; v != "i1" {
		t.Fatalf("ios version = %q", v)
	}

	// The jobs carry the ring they deployed to.
	var jobs struct {
		Jobs []jobState `json:"jobs"`
	}
	_ = json.Unmarshal(doJSON(t, h, http.MethodGet, "/api/jobs", "").Body.Bytes(), &jobs)
	for _, j := range jobs.Jobs {
		if j.Ring != "int" {
			t.Fatalf("job %s ring = %q", j.ID, j.Ring)
		}
	}

	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+run.ID+"/cancel", ""); rec.Code != http.StatusConflict {
		t.Fatalf("cancel finished = %d", rec.Code)
	}
	var list []pipeline.Run
	_ = json.Unmarshal(doJSON(t, h, http.MethodGet, "/api/pipelines/release/runs", "").Body.Bytes(), &list)
	if len(list) != 1 || list[0].ID != run.ID {
		t.Fatalf("runs = %+v", list)
	}
}

func TestPipelines_RejectCancelAndRerun(t *testing.T) {
	h, st := newPipelineServer(t, "")
	if err := st.UpsertQAReport(t.Context(), store.QAReport{App: "db", Ring: "int", WorkflowVerdict: "go", Source: "qa"}); err != nil {
		t.Fatal(err)
	}
	start := func() pipeline.Run {
		rec := doJSON(t, h, http.MethodPost, "/api/pipelines/release/runs", `{"ring":"int","versions":{"web":"w1","ios":"i1"}}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("start: %d %s", rec.Code, rec.Body)
		}
		return decodeRun(t, rec.Body.Bytes())
	}
	waitingHold := func(r pipeline.Run) bool { return r.Stages[2].Status == pipeline.StatusWaiting }

	first := start()
	pollRun(t, h, first.ID, waitingHold)
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+first.ID+"/stages/hold/reject", `{"note":"not today"}`); rec.Code != http.StatusOK {
		t.Fatalf("reject = %d %s", rec.Code, rec.Body)
	}
	r := pollRun(t, h, first.ID, func(r pipeline.Run) bool { return r.FinishedAt != nil })
	if r.Status != pipeline.StatusFailed || r.Stages[3].Status != pipeline.StatusSkipped {
		t.Fatalf("rejected run = %+v", r)
	}

	rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+first.ID+"/rerun", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rerun = %d %s", rec.Code, rec.Body)
	}
	rerun := decodeRun(t, rec.Body.Bytes())
	if rerun.RerunOf != first.ID || rerun.Stages[0].Status != pipeline.StatusReused || rerun.Stages[1].Status != pipeline.StatusReused {
		t.Fatalf("rerun = %+v", rerun)
	}
	pollRun(t, h, rerun.ID, waitingHold)
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/"+rerun.ID+"/cancel", ""); rec.Code != http.StatusOK {
		t.Fatalf("cancel = %d %s", rec.Code, rec.Body)
	}
	r = pollRun(t, h, rerun.ID, func(r pipeline.Run) bool { return r.FinishedAt != nil })
	if r.Status != pipeline.StatusCancelled || r.Stages[2].Status != pipeline.StatusCancelled {
		t.Fatalf("cancelled run = %+v", r)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/pipeline-runs/run-99/rerun", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("rerun unknown = %d", rec.Code)
	}

	// Lifecycle events land in the audit ledger.
	events, err := st.ListAudit(t.Context(), store.AuditFilter{Category: store.AuditPipeline})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Action] = true
	}
	for _, a := range []string{"start", "reject", "cancel"} {
		if !seen[a] {
			t.Errorf("no %q audit event in %+v", a, events)
		}
	}
}
