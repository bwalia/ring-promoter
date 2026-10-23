package deployer

import (
	"context"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/progress"
)

// stagedExecution is a fakeExecution that also reports stages.
type stagedExecution struct {
	*fakeExecution
	stages []progress.Stage
}

func (s *stagedExecution) Stages(context.Context) ([]progress.Stage, error) {
	return s.stages, nil
}

type stagedExecutor struct{ ex *stagedExecution }

func (f *stagedExecutor) Start(context.Context, executor.Spec) (executor.Execution, error) {
	return f.ex, nil
}

func TestExecDeployer_ReportsStages(t *testing.T) {
	ex := &stagedExecution{
		fakeExecution: &fakeExecution{statuses: []executor.Status{
			{Phase: executor.PhaseRunning},
			{Phase: executor.PhaseSucceeded},
		}},
		stages: []progress.Stage{
			{ID: "1", Name: "build", Status: progress.StageSuccess},
			{ID: "2", Name: "deploy", Status: progress.StageRunning},
		},
	}
	d := FromExecutor(nil, &stagedExecutor{ex: ex}, specOK, time.Millisecond)
	rep := &recordingReporter{}
	ctx := progress.WithReporter(context.Background(), rep)
	if err := d.Deploy(ctx, Target{App: "a", Ring: "int"}, "v1"); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	got := rep.lastStages()
	if len(got) != 2 || got[1].Name != "deploy" {
		t.Fatalf("stages = %+v", got)
	}
	// Once per status poll: running and the terminal one.
	if len(rep.stages) < 2 {
		t.Fatalf("stages pushed %d times, want one per poll", len(rep.stages))
	}
}

func TestGitHub_Deploy_ReportsJobsAsStages(t *testing.T) {
	f := &ghFake{conclusion: "success", completeAfter: 1, jobs: `{"jobs":[
		{"id":7,"name":"Build & Push Docker Image","status":"completed","conclusion":"success",
		 "html_url":"https://github.com/o/r/actions/runs/42/job/7",
		 "started_at":"2026-10-22T10:00:00Z","completed_at":"2026-10-22T10:08:34Z",
		 "steps":[{"number":1,"name":"Set up job","status":"completed","conclusion":"success"},
		          {"number":2,"name":"Push","status":"completed","conclusion":"skipped"}]},
		{"id":8,"name":"INT → K3S Cluster","status":"in_progress","conclusion":null,"started_at":"2026-10-22T10:08:40Z"},
		{"id":9,"name":"PROD → K3S Cluster","status":"queued","conclusion":null},
		{"id":10,"name":"Cut Release Tag","status":"completed","conclusion":"failure"},
		{"id":11,"name":"Notify","status":"completed","conclusion":"cancelled"}]}`}
	srv := newGHServer(t, f)
	d := testGHDeployer(t, srv.URL)
	rep := &recordingReporter{}
	ctx := progress.WithReporter(context.Background(), rep)
	if err := d.Deploy(ctx, Target{App: "web", Ring: "int", TargetEnv: "int"}, "v1"); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	got := rep.lastStages()
	if len(got) != 5 {
		t.Fatalf("got %d stages: %+v", len(got), got)
	}
	want := []string{progress.StageSuccess, progress.StageRunning, progress.StageQueued, progress.StageFailed, progress.StageCancelled}
	for i, w := range want {
		if got[i].Status != w {
			t.Errorf("stage %d (%s) status = %q, want %q", i, got[i].Name, got[i].Status, w)
		}
	}
	b := got[0]
	if b.ID != "7" || b.URL != "https://github.com/o/r/actions/runs/42/job/7" || b.StartedAt == nil || b.FinishedAt == nil {
		t.Fatalf("build stage = %+v", b)
	}
	if b.FinishedAt.Sub(*b.StartedAt) != 8*time.Minute+34*time.Second {
		t.Fatalf("build duration = %s", b.FinishedAt.Sub(*b.StartedAt))
	}
	if len(b.Steps) != 2 || b.Steps[1].Status != progress.StageSkipped || b.Steps[1].Number != 2 {
		t.Fatalf("build steps = %+v", b.Steps)
	}
}
