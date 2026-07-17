package deployer

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/progress"
)

// SpecFunc maps the promotion vocabulary (a Target and a version) onto the
// execution vocabulary (an executor.Spec). It is the only place that knows
// both: the promoter above sees Deployer, the backend below sees Spec.
type SpecFunc func(t Target, version string) (executor.Spec, error)

// ExecDeployer adapts an executor.Executor into a Deployer: Deploy starts an
// execution, streams its logs into the operation's progress reporter, polls
// its status until a terminal phase, and maps the outcome onto Deploy's
// error contract (nil only on success) — so the promotion engine's health
// checks, auto-rollback and history apply to every backend unchanged.
type ExecDeployer struct {
	log  *slog.Logger
	exec executor.Executor
	spec SpecFunc
	poll time.Duration
}

// FromExecutor builds the adapter. poll is the status poll cadence.
func FromExecutor(log *slog.Logger, exec executor.Executor, spec SpecFunc, poll time.Duration) *ExecDeployer {
	if log == nil {
		log = slog.Default()
	}
	if poll <= 0 {
		poll = 3 * time.Second
	}
	return &ExecDeployer{log: log, exec: exec, spec: spec, poll: poll}
}

// Deploy implements Deployer.
func (d *ExecDeployer) Deploy(ctx context.Context, t Target, version string) error {
	spec, err := d.spec(t, version)
	if err != nil {
		return err
	}
	ex, err := d.exec.Start(ctx, spec)
	if err != nil {
		return err
	}

	rep := progress.FromContext(ctx)

	// Pump log lines into the reporter while we wait. The pump stops when
	// Deploy returns (stopPump runs before pump.Wait — defers are LIFO).
	pumpCtx, stopPump := context.WithCancel(ctx)
	var pump sync.WaitGroup
	pump.Add(1)
	go func() {
		defer pump.Done()
		d.pumpLogs(pumpCtx, ex, rep)
	}()
	defer pump.Wait()
	defer stopPump()

	var last executor.Phase
	for {
		st, err := ex.Status(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return d.cancelled(ctx, ex, rep)
			}
			return err
		}
		if st.Phase != last {
			last = st.Phase
			line := "execution " + string(st.Phase)
			if st.Message != "" {
				line += ": " + st.Message
			}
			rep.Log(line)
			d.log.Info("execution phase",
				"app", t.App, "ring", t.Ring, "execution", ex.ID(),
				"phase", st.Phase, "msg", st.Message)
		}
		if st.Phase.Terminal() {
			d.cleanup(ctx, ex)
			if st.Phase == executor.PhaseSucceeded {
				return nil
			}
			msg := st.Message
			if msg == "" {
				msg = "execution " + string(st.Phase)
			}
			return errors.New(msg)
		}
		if err := sleepCtx(ctx, d.poll); err != nil {
			return d.cancelled(ctx, ex, rep)
		}
	}
}

// cancelled tears the execution down after ctx was cancelled (user cancel or
// operation timeout): best-effort Cancel under a detached deadline, then the
// context's own error so the caller records the right cause.
func (d *ExecDeployer) cancelled(ctx context.Context, ex executor.Execution, rep progress.Reporter) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := ex.Cancel(cctx); err != nil {
		d.log.Warn("cancel execution", "execution", ex.ID(), "err", err)
	} else {
		rep.Log("execution cancelled")
	}
	return ctx.Err()
}

// cleanup releases execution resources under a detached deadline (the
// operation context may already be expiring).
func (d *ExecDeployer) cleanup(ctx context.Context, ex executor.Execution) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := ex.Cleanup(cctx); err != nil {
		d.log.Warn("cleanup execution", "execution", ex.ID(), "err", err)
	}
}

// pumpLogs streams execution logs into the reporter, reconnecting (resuming
// from the last seen timestamp) if the stream drops while the execution still
// runs — e.g. a Kubernetes Job retrying with a fresh pod.
func (d *ExecDeployer) pumpLogs(ctx context.Context, ex executor.Execution, rep progress.Reporter) {
	since := ""
	for ctx.Err() == nil {
		rc, err := ex.Logs(ctx, executor.LogOptions{Follow: true, SinceTime: since, Timestamps: true})
		if err != nil {
			if errors.Is(err, executor.ErrLogsUnsupported) || ctx.Err() != nil {
				return
			}
			_ = sleepCtx(ctx, d.poll) // transient (pod not up yet): retry
			continue
		}
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if ts := leadingTimestamp(line); ts != "" {
				since = ts
			}
			rep.Log(line)
		}
		rc.Close()
		if ctx.Err() != nil {
			return
		}
		_ = sleepCtx(ctx, d.poll)
	}
}

// leadingTimestamp returns the RFC3339 timestamp prefixing a log line (as
// emitted by `kubectl logs --timestamps`), or "". It becomes the resume point
// on reconnect; the boundary line may repeat (since-time is inclusive), which
// is preferred over losing lines.
func leadingTimestamp(line string) string {
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, line[:i]); err != nil {
		return ""
	}
	return line[:i]
}

// sleepCtx waits for d or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
