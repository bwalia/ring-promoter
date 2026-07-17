// Package k8sjob implements the Kubernetes Jobs execution backend: each
// deployment task runs as one batch/v1 Job in a dedicated namespace. The
// cluster is driven through kubectl — matching KubectlDeployer's deliberate
// choice (small binary, ServiceAccount auth for free) — behind a small runner
// interface so a client-go implementation can replace it later without
// touching the executor logic.
package k8sjob

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// Options configures the executor.
type Options struct {
	// Kubectl is the kubectl binary. Default "kubectl".
	Kubectl string
}

// Executor implements executor.Executor for Kubernetes Jobs. One instance is
// shared by all k8sjob-configured apps (it is stateless; everything
// per-execution lives on the Spec and the returned Execution).
type Executor struct {
	log *slog.Logger
	run runner
	// retryBackoff spaces the attempts of a transient-error API retry.
	retryBackoff time.Duration
}

// New returns a Kubernetes Jobs executor.
func New(log *slog.Logger, opts Options) *Executor {
	if log == nil {
		log = slog.Default()
	}
	bin := opts.Kubectl
	if bin == "" {
		bin = "kubectl"
	}
	return &Executor{log: log, run: &kubectlRunner{bin: bin}, retryBackoff: 2 * time.Second}
}

// Start implements executor.Executor: generate the Job manifest and create it.
func (e *Executor) Start(ctx context.Context, spec executor.Spec) (executor.Execution, error) {
	if spec.Image == "" {
		return nil, fmt.Errorf("k8sjob deployer: no image configured for %s", spec.App)
	}
	if spec.Namespace == "" {
		return nil, fmt.Errorf("k8sjob deployer: no namespace configured for %s", spec.App)
	}

	id := spec.ID
	if id == "" {
		id = newID()
	}
	name := jobName(spec.App, spec.Ring, id)

	manifest, err := json.Marshal(buildManifest(spec, name, id))
	if err != nil {
		return nil, err
	}
	if _, err := e.run.output(ctx, manifest, "-n", spec.Namespace, "create", "-f", "-"); err != nil {
		return nil, fmt.Errorf("create job %s: %w", name, err)
	}
	e.log.Info("k8s job created",
		"app", spec.App, "ring", spec.Ring, "namespace", spec.Namespace,
		"job", name, "execution", id)

	return &execution{e: e, id: id, name: name, spec: spec}, nil
}

// execution is a handle on one Job.
type execution struct {
	e    *Executor
	id   string
	name string
	spec executor.Spec
}

func (x *execution) ID() string { return x.id }

// Status implements executor.Execution: read the Job and its pods, and map
// their raw state onto a phase and an actionable message.
func (x *execution) Status(ctx context.Context) (executor.Status, error) {
	out, err := x.getWithRetry(ctx, "-n", x.spec.Namespace, "get", "job", x.name, "-o", "json")
	if err != nil {
		return executor.Status{}, fmt.Errorf("get job %s: %w", x.name, err)
	}
	var job jobDoc
	if err := json.Unmarshal(out, &job); err != nil {
		return executor.Status{}, fmt.Errorf("parse job %s: %w", x.name, err)
	}

	// Pod details enrich the status; a transient pod-list failure must not
	// fail the whole status read.
	var pods podList
	if pout, perr := x.getWithRetry(ctx, "-n", x.spec.Namespace,
		"get", "pods", "-l", labelExecutionID+"="+x.id, "-o", "json"); perr == nil {
		_ = json.Unmarshal(pout, &pods)
	}

	st := mapStatus(x.spec, job, pods.Items, time.Now())
	st.Details["job_name"] = x.name
	st.Details["namespace"] = x.spec.Namespace
	st.Details["execution_id"] = x.id
	return st, nil
}

// Logs implements executor.Execution by following the Job's pod logs.
// `kubectl logs job/...` targets the Job's current pod, so after a retry a
// reconnecting caller (see deployer.ExecDeployer.pumpLogs) follows the fresh
// attempt automatically.
func (x *execution) Logs(ctx context.Context, opts executor.LogOptions) (io.ReadCloser, error) {
	args := []string{"-n", x.spec.Namespace, "logs", "job/" + x.name}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	if opts.SinceTime != "" {
		args = append(args, "--since-time="+opts.SinceTime)
	}
	return x.e.run.stream(ctx, args...)
}

// Cancel implements executor.Execution: delete the Job with foreground
// propagation so its pods are terminated too. Idempotent via --ignore-not-found.
func (x *execution) Cancel(ctx context.Context) error {
	_, err := x.e.run.output(ctx, nil, "-n", x.spec.Namespace,
		"delete", "job", x.name, "--ignore-not-found", "--wait=false", "--cascade=foreground")
	if err != nil {
		return fmt.Errorf("cancel job %s: %w", x.name, err)
	}
	x.e.log.Info("k8s job cancelled", "job", x.name, "namespace", x.spec.Namespace)
	return nil
}

// Cleanup implements executor.Execution as a no-op: retention is the Job's
// ttlSecondsAfterFinished (from Spec.TTLAfterFinish), which keeps finished
// Jobs — and their logs — inspectable for a while instead of vanishing the
// moment the promotion moves on. Without a TTL the Job is kept indefinitely,
// which is a configuration choice, not a leak.
func (x *execution) Cleanup(context.Context) error { return nil }

// getWithRetry runs a read-only kubectl command, retrying transient failures
// (API blips, brief network loss) so a monitoring hiccup doesn't fail a
// deploy that is actually progressing.
func (x *execution) getWithRetry(ctx context.Context, args ...string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * x.e.retryBackoff):
			}
		}
		out, err := x.e.run.output(ctx, nil, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

// newID returns a fresh execution id, e.g. "exc-9f3a1c2e".
func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "exc-" + hex.EncodeToString(b)
}

// runner abstracts process execution so tests can script kubectl responses,
// and so the kubectl transport can later be swapped for client-go behind the
// same two calls.
type runner interface {
	// output runs the command to completion and returns stdout.
	output(ctx context.Context, stdin []byte, args ...string) ([]byte, error)
	// stream starts the command and returns its stdout as a stream; closing
	// the reader stops the command.
	stream(ctx context.Context, args ...string) (io.ReadCloser, error)
}

type kubectlRunner struct {
	bin string
}

func (r *kubectlRunner) output(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("kubectl %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (r *kubectlRunner) stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, r.bin, args...)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &procStream{Reader: stdout, stop: sync.OnceValue(func() error {
		cancel()
		_ = cmd.Wait() // reap; the error is expected after cancel
		return nil
	})}, nil
}

// procStream is a subprocess's stdout; Close terminates and reaps the process.
type procStream struct {
	io.Reader
	stop func() error
}

func (p *procStream) Close() error { return p.stop() }
