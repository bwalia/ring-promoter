package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
)

// fakeRunner scripts deploys and QA verdicts. A deploy blocks until its app's
// release channel is closed (when one is registered), so tests can observe
// what runs concurrently.
type fakeRunner struct {
	mu       sync.Mutex
	fail     map[string]bool
	verdicts map[string]string
	release  map[string]chan struct{}
	started  []string
	running  int
	peak     int
	audits   []string
	seq      int
}

func newFake() *fakeRunner {
	return &fakeRunner{fail: map[string]bool{}, verdicts: map[string]string{}, release: map[string]chan struct{}{}}
}

func (f *fakeRunner) Deploy(_ context.Context, app, ringName, version string) (Deployment, error) {
	f.mu.Lock()
	f.seq++
	id := fmt.Sprintf("job-%d", f.seq)
	f.started = append(f.started, app)
	f.running++
	if f.running > f.peak {
		f.peak = f.running
	}
	gate := f.release[app]
	fail := f.fail[app]
	f.mu.Unlock()
	return Deployment{JobID: id, Wait: func() (bool, string, string) {
		if gate != nil {
			<-gate
		}
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
		if fail {
			return false, version, "deploy failed"
		}
		return true, version, "ok"
	}}, nil
}

func (f *fakeRunner) QAVerdict(_ context.Context, app, _ string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := f.verdicts[app]
	if v == "" {
		v = VerdictUnknown
	}
	return v, "summary for " + app, nil
}

func (f *fakeRunner) Audit(_ context.Context, _, action string, _ map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, action)
}

func (f *fakeRunner) setVerdict(app, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verdicts[app] = v
}

func newTestManager(f *fakeRunner) *Manager {
	m := NewManager(f)
	m.Poll = time.Millisecond
	return m
}

// waitFor polls the run until cond holds or the test times out.
func waitFor(t *testing.T, m *Manager, id string, cond func(Run) bool) Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if cond(r) {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out; run = %+v", r)
		}
		time.Sleep(time.Millisecond)
	}
}

func finished(r Run) bool { return r.FinishedAt != nil }

func stageStatus(r Run, name string) string {
	for _, s := range r.Stages {
		if s.Name == name {
			return s.Status
		}
	}
	return "missing"
}

func pipe(stages ...config.PipelineStage) config.PipelineConfig {
	return config.PipelineConfig{Name: "release", DisplayName: "Release", Stages: stages}
}

func req() Request {
	return Request{Ring: "test", TriggeredBy: "tester"}
}

func TestRun_ParallelStageAndNeedsOrdering(t *testing.T) {
	f := newFake()
	f.release["db"] = make(chan struct{})
	f.release["cache"] = make(chan struct{})
	m := newTestManager(f)
	p := pipe(
		config.PipelineStage{Name: "data", Apps: []string{"db", "cache"}},
		config.PipelineStage{Name: "frontend", Apps: []string{"web"}, Needs: []string{"data"}},
	)
	run, err := m.Start(context.Background(), p, req(), "")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != "run-1" || run.DisplayName != "Release" {
		t.Fatalf("run = %+v", run)
	}
	waitFor(t, m, run.ID, func(r Run) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.running == 2
	})
	r, _ := m.Get(run.ID)
	if r.Status != StatusRunning || stageStatus(r, "frontend") != StatusQueued {
		t.Fatalf("frontend must wait for data: %+v", r)
	}
	if r.Stages[0].Items[0].JobID == "" {
		t.Fatalf("item should carry its job id: %+v", r.Stages[0].Items)
	}
	close(f.release["db"])
	close(f.release["cache"])
	r = waitFor(t, m, run.ID, finished)
	if r.Status != StatusSuccess {
		t.Fatalf("status = %s: %+v", r.Status, r)
	}
	if f.peak != 2 {
		t.Fatalf("data apps should deploy in parallel, peak = %d", f.peak)
	}
	if got := f.started[len(f.started)-1]; got != "web" {
		t.Fatalf("web must deploy last, order = %v", f.started)
	}
}

func TestRun_FailureSkipsDependents(t *testing.T) {
	f := newFake()
	f.fail["db"] = true
	m := newTestManager(f)
	p := pipe(
		config.PipelineStage{Name: "data", Apps: []string{"db"}},
		config.PipelineStage{Name: "api", Apps: []string{"api"}, Needs: []string{"data"}},
		config.PipelineStage{Name: "web", Apps: []string{"web"}, Needs: []string{"api"}},
		config.PipelineStage{Name: "docs", Apps: []string{"docs"}},
	)
	run, _ := m.Start(context.Background(), p, req(), "")
	r := waitFor(t, m, run.ID, finished)
	if r.Status != StatusFailed {
		t.Fatalf("status = %s", r.Status)
	}
	want := map[string]string{"data": StatusFailed, "api": StatusSkipped, "web": StatusSkipped, "docs": StatusSuccess}
	for n, w := range want {
		if got := stageStatus(r, n); got != w {
			t.Errorf("stage %s = %s, want %s", n, got, w)
		}
	}
	if r.Stages[1].Items[0].Status != StatusSkipped {
		t.Fatalf("skipped stage items should be skipped: %+v", r.Stages[1].Items)
	}
}

func TestRun_QAGate(t *testing.T) {
	cases := []struct {
		name    string
		verdict string
		timeout time.Duration
		want    string
		item    string
	}{
		{"go", VerdictGo, time.Minute, StatusSuccess, StatusSuccess},
		{"no_go", VerdictNoGo, time.Minute, StatusFailed, StatusFailed},
		{"timeout", VerdictCheck, 20 * time.Millisecond, StatusFailed, StatusWaiting},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake()
			f.setVerdict("db", c.verdict)
			m := newTestManager(f)
			p := pipe(
				config.PipelineStage{Name: "data", Apps: []string{"db"}},
				config.PipelineStage{Name: "qa", Type: config.StageGate, Needs: []string{"data"}, Timeout: config.Duration(c.timeout)},
				config.PipelineStage{Name: "web", Apps: []string{"web"}, Needs: []string{"qa"}},
			)
			run, _ := m.Start(context.Background(), p, req(), "")
			r := waitFor(t, m, run.ID, finished)
			if got := stageStatus(r, "qa"); got != c.want {
				t.Fatalf("gate = %s, want %s (%+v)", got, c.want, r.Stages[1])
			}
			items := r.Stages[1].Items
			if len(items) != 1 || items[0].App != "db" || items[0].Verdict != c.verdict || items[0].Status != c.item {
				t.Fatalf("gate items = %+v (gate checks the apps its needs deployed)", items)
			}
			if c.want == StatusFailed && stageStatus(r, "web") != StatusSkipped {
				t.Fatalf("web should be skipped behind a NO-GO")
			}
		})
	}
}

func TestRun_QAGateWaitsThenGoes(t *testing.T) {
	f := newFake()
	f.setVerdict("db", VerdictCheck)
	m := newTestManager(f)
	p := pipe(config.PipelineStage{Name: "qa", Type: config.StageGate, Apps: []string{"db"}})
	run, _ := m.Start(context.Background(), p, req(), "")
	r := waitFor(t, m, run.ID, func(r Run) bool { return r.Status == StatusWaiting })
	if r.Stages[0].Status != StatusWaiting {
		t.Fatalf("gate should be waiting: %+v", r.Stages[0])
	}
	f.setVerdict("db", VerdictGo)
	r = waitFor(t, m, run.ID, finished)
	if r.Status != StatusSuccess {
		t.Fatalf("status = %s", r.Status)
	}
}

func TestRun_Approval(t *testing.T) {
	for _, approve := range []bool{true, false} {
		t.Run(fmt.Sprint("approve=", approve), func(t *testing.T) {
			f := newFake()
			m := newTestManager(f)
			p := pipe(
				config.PipelineStage{Name: "hold", Type: config.StageApproval},
				config.PipelineStage{Name: "ios", Apps: []string{"ios"}, Needs: []string{"hold"}},
			)
			run, _ := m.Start(context.Background(), p, req(), "")
			waitFor(t, m, run.ID, func(r Run) bool { return r.Status == StatusWaiting })
			if _, err := m.Decide(context.Background(), run.ID, "ios", true, "", ""); !errors.Is(err, ErrNotWaiting) {
				t.Fatalf("deciding a non-approval stage: err = %v", err)
			}
			if _, err := m.Decide(context.Background(), run.ID, "nope", true, "", ""); !errors.Is(err, ErrStageNotFound) {
				t.Fatalf("unknown stage: err = %v", err)
			}
			if _, err := m.Decide(context.Background(), run.ID, "hold", approve, "rel-eng", "lgtm"); err != nil {
				t.Fatal(err)
			}
			r := waitFor(t, m, run.ID, finished)
			if r.Stages[0].ApprovedBy != "rel-eng" {
				t.Fatalf("approved_by = %q", r.Stages[0].ApprovedBy)
			}
			want, ios := StatusSuccess, StatusSuccess
			if !approve {
				want, ios = StatusFailed, StatusSkipped
			}
			if r.Status != want || stageStatus(r, "ios") != ios {
				t.Fatalf("run = %s ios = %s", r.Status, stageStatus(r, "ios"))
			}
			if _, err := m.Decide(context.Background(), run.ID, "hold", true, "", ""); !errors.Is(err, ErrNotWaiting) {
				t.Fatalf("second decision: err = %v", err)
			}
		})
	}
}

func TestRun_ApprovalTimeout(t *testing.T) {
	m := newTestManager(newFake())
	p := pipe(config.PipelineStage{Name: "hold", Type: config.StageApproval, Timeout: config.Duration(10 * time.Millisecond)})
	run, _ := m.Start(context.Background(), p, req(), "")
	r := waitFor(t, m, run.ID, finished)
	if r.Status != StatusFailed || r.Stages[0].Message == "" {
		t.Fatalf("run = %+v", r)
	}
}

func TestRun_Cancel(t *testing.T) {
	f := newFake()
	f.release["db"] = make(chan struct{})
	m := newTestManager(f)
	p := pipe(
		config.PipelineStage{Name: "data", Apps: []string{"db"}},
		config.PipelineStage{Name: "hold", Type: config.StageApproval},
		config.PipelineStage{Name: "web", Apps: []string{"web"}, Needs: []string{"data"}},
	)
	run, _ := m.Start(context.Background(), p, req(), "")
	waitFor(t, m, run.ID, func(r Run) bool {
		return stageStatus(r, "hold") == StatusWaiting && stageStatus(r, "data") == StatusRunning
	})
	if _, err := m.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	// The running deploy is not interrupted: it finishes on its own.
	close(f.release["db"])
	r := waitFor(t, m, run.ID, finished)
	if r.Status != StatusCancelled {
		t.Fatalf("status = %s", r.Status)
	}
	want := map[string]string{"data": StatusSuccess, "hold": StatusCancelled, "web": StatusCancelled}
	for n, w := range want {
		if got := stageStatus(r, n); got != w {
			t.Errorf("stage %s = %s, want %s", n, got, w)
		}
	}
	if _, err := m.Cancel(context.Background(), run.ID); !errors.Is(err, ErrRunFinished) {
		t.Fatalf("cancel finished run: err = %v", err)
	}
	if f.audits[0] != "start" || f.audits[1] != "cancel" {
		t.Fatalf("audits = %v", f.audits)
	}
}

func TestRun_RerunFromFailed(t *testing.T) {
	f := newFake()
	f.fail["web"] = true
	m := newTestManager(f)
	p := pipe(
		config.PipelineStage{Name: "data", Apps: []string{"db"}},
		config.PipelineStage{Name: "frontend", Apps: []string{"web"}, Needs: []string{"data"}},
		config.PipelineStage{Name: "ios", Apps: []string{"ios"}, Needs: []string{"frontend"}},
	)
	first, _ := m.Start(context.Background(), p, req(), "")
	waitFor(t, m, first.ID, finished)

	f.mu.Lock()
	f.fail["web"] = false
	f.started = nil
	f.mu.Unlock()
	_, rq, err := m.Request(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Start(context.Background(), p, rq, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := waitFor(t, m, second.ID, finished)
	if r.RerunOf != first.ID || r.Status != StatusSuccess {
		t.Fatalf("rerun = %+v", r)
	}
	if stageStatus(r, "data") != StatusReused || r.Stages[0].Items[0].Status != StatusReused {
		t.Fatalf("data should be reused: %+v", r.Stages[0])
	}
	if len(f.started) != 2 || f.started[0] != "web" || f.started[1] != "ios" {
		t.Fatalf("rerun deployed %v, want [web ios]", f.started)
	}

	if _, err := m.Start(context.Background(), p, rq, "run-99"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("rerun of unknown run: err = %v", err)
	}
	if got := m.List("release", 1); len(got) != 1 || got[0].ID != second.ID {
		t.Fatalf("list = %+v", got)
	}
}

func TestValidate(t *testing.T) {
	p := pipe(config.PipelineStage{Name: "data", Apps: []string{"db", "web"}})
	refs := func(app string) string {
		if app == "db" {
			return "main"
		}
		return ""
	}
	if err := Validate(p, Request{Ring: "nope"}, refs); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("bad ring: %v", err)
	}
	if err := Validate(p, Request{Ring: "int"}, refs); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("missing web version: %v", err)
	}
	if err := Validate(p, Request{Ring: "int", Versions: map[string]string{"web": "v1"}}, refs); err != nil {
		t.Fatalf("db from ref, web from request: %v", err)
	}
	if err := Validate(p, Request{Ring: "test"}, refs); err != nil {
		t.Fatalf("promotions need no versions: %v", err)
	}
}
