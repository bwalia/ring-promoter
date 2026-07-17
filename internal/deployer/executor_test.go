package deployer

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/progress"
)

// fakeExecution is a scripted executor.Execution: Status returns the scripted
// statuses in order (the last repeats); Logs serves a fixed body once, then
// reports streaming unsupported so the pump stops.
type fakeExecution struct {
	mu          sync.Mutex
	statuses    []executor.Status
	statusCalls int
	logs        string
	logsServed  bool
	cancelled   bool
	cleaned     bool
}

func (f *fakeExecution) ID() string { return "exc-test" }

func (f *fakeExecution) Status(ctx context.Context) (executor.Status, error) {
	if err := ctx.Err(); err != nil {
		return executor.Status{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.statusCalls
	if i >= len(f.statuses) {
		i = len(f.statuses) - 1
	}
	f.statusCalls++
	return f.statuses[i], nil
}

func (f *fakeExecution) Logs(ctx context.Context, opts executor.LogOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == "" || f.logsServed {
		return nil, executor.ErrLogsUnsupported
	}
	f.logsServed = true
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

func (f *fakeExecution) Cancel(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = true
	return nil
}

func (f *fakeExecution) Cleanup(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleaned = true
	return nil
}

type fakeExecutor struct {
	ex       *fakeExecution
	startErr error
	spec     executor.Spec
}

func (f *fakeExecutor) Start(ctx context.Context, spec executor.Spec) (executor.Execution, error) {
	f.spec = spec
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.ex, nil
}

// recordingReporter collects Log lines (concurrency-safe: the log pump runs in
// its own goroutine).
type recordingReporter struct {
	mu    sync.Mutex
	lines []string
}

func (r *recordingReporter) StartStep(string, string)  {}
func (r *recordingReporter) FinishStep(string, string) {}
func (r *recordingReporter) Log(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}
func (r *recordingReporter) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func specOK(t Target, version string) (executor.Spec, error) {
	return executor.Spec{App: t.App, Ring: t.Ring, Env: map[string]string{executor.EnvVersion: version}}, nil
}

func newAdapter(ex executor.Executor) *ExecDeployer {
	return FromExecutor(nil, ex, specOK, time.Millisecond)
}

func TestExecDeployer_SuccessStreamsLogsAndCleansUp(t *testing.T) {
	fx := &fakeExecution{
		statuses: []executor.Status{
			{Phase: executor.PhasePending},
			{Phase: executor.PhaseRunning},
			{Phase: executor.PhaseSucceeded},
		},
		logs: "pulling image\napplying manifests\n",
	}
	f := &fakeExecutor{ex: fx}
	rep := &recordingReporter{}
	ctx := progress.WithReporter(context.Background(), rep)

	if err := newAdapter(f).Deploy(ctx, Target{App: "a", Ring: "int"}, "v1"); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	out := rep.joined()
	for _, want := range []string{"pulling image", "applying manifests", "execution running", "execution succeeded"} {
		if !strings.Contains(out, want) {
			t.Fatalf("reporter missing %q in:\n%s", want, out)
		}
	}
	if !fx.cleaned {
		t.Fatal("cleanup was not called")
	}
	if fx.cancelled {
		t.Fatal("cancel should not be called on success")
	}
	if f.spec.Env[executor.EnvVersion] != "v1" {
		t.Fatalf("spec env: %+v", f.spec.Env)
	}
}

func TestExecDeployer_FailureReturnsStatusMessage(t *testing.T) {
	fx := &fakeExecution{
		statuses: []executor.Status{
			{Phase: executor.PhaseRunning},
			{Phase: executor.PhaseFailed, Message: "deploy script exited with code 3"},
		},
	}
	err := newAdapter(&fakeExecutor{ex: fx}).Deploy(context.Background(), Target{App: "a", Ring: "int"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "exited with code 3") {
		t.Fatalf("expected the status message as the error, got %v", err)
	}
	if !fx.cleaned {
		t.Fatal("cleanup was not called on failure")
	}
}

func TestExecDeployer_ContextCancelTriggersCancel(t *testing.T) {
	fx := &fakeExecution{
		statuses: []executor.Status{{Phase: executor.PhaseRunning}}, // never terminal
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := newAdapter(&fakeExecutor{ex: fx}).Deploy(ctx, Target{App: "a", Ring: "int"}, "v1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the context error, got %v", err)
	}
	fx.mu.Lock()
	defer fx.mu.Unlock()
	if !fx.cancelled {
		t.Fatal("expected Cancel to be called when the operation context ends")
	}
}

func TestExecDeployer_StartAndSpecErrors(t *testing.T) {
	boom := errors.New("no capacity")
	err := newAdapter(&fakeExecutor{startErr: boom}).Deploy(context.Background(), Target{App: "a"}, "v1")
	if !errors.Is(err, boom) {
		t.Fatalf("start error not propagated: %v", err)
	}

	bad := FromExecutor(nil, &fakeExecutor{}, func(Target, string) (executor.Spec, error) {
		return executor.Spec{}, errors.New("ring has no image configured")
	}, time.Millisecond)
	err = bad.Deploy(context.Background(), Target{App: "a"}, "v1")
	if err == nil || !strings.Contains(err.Error(), "no image configured") {
		t.Fatalf("spec error not propagated: %v", err)
	}
}

func TestLeadingTimestamp(t *testing.T) {
	if got := leadingTimestamp("2026-07-17T12:04:11.5Z pulling image"); got != "2026-07-17T12:04:11.5Z" {
		t.Fatalf("leadingTimestamp = %q", got)
	}
	for _, line := range []string{"no timestamp here", "12:04 short", ""} {
		if got := leadingTimestamp(line); got != "" {
			t.Fatalf("leadingTimestamp(%q) = %q, want empty", line, got)
		}
	}
}
