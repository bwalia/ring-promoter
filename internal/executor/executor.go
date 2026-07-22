// Package executor defines the execution-backend abstraction: a backend runs
// one deployment task somewhere (a GitHub Actions workflow, a Kubernetes Job,
// ...) and reports its lifecycle. Backends know nothing about rings or
// promotions; the promotion vocabulary is mapped onto a Spec by the deployer
// layer (see deployer.FromExecutor).
package executor

import (
	"context"
	"errors"
	"io"
	"time"
)

// Environment variable names of the runner contract: every execution receives
// these, so any image/script that reads RP_* variables and exits 0 on success
// is a valid deployment task regardless of which backend runs it.
const (
	EnvApp         = "RP_APP"
	EnvRing        = "RP_RING"
	EnvVersion     = "RP_VERSION"
	EnvTargetEnv   = "RP_TARGET_ENV"
	EnvExecutionID = "RP_EXECUTION_ID"
)

// ErrLogsUnsupported is returned by Execution.Logs when the backend cannot
// stream logs (e.g. GitHub Actions); callers treat it as "nothing to pump",
// not as a failure.
var ErrLogsUnsupported = errors.New("executor: log streaming not supported by this backend")

// Spec is everything a backend needs to run one deployment task. It is
// backend-agnostic: each backend ignores the fields that do not apply to it,
// exactly as deployer.Target does today. Kubernetes-flavored fields
// (Namespace, Resources, ...) are populated from the app's k8sjob config.
type Spec struct {
	// ID uniquely identifies the execution; backends label their work with it
	// so it can be found again. Empty = the backend generates one on Start.
	ID     string
	App    string
	Ring   string
	Action string // seed | promote | rollback (informational; may be empty)

	// What to run.
	Image   string
	Command []string
	Args    []string
	// Env is the full environment for the task: the RP_* contract variables
	// plus any custom variables from configuration.
	Env map[string]string

	// Secret/config material is referenced by name, never inlined.
	EnvFromSecrets    []string
	EnvFromConfigMaps []string
	ImagePullSecrets  []string

	// Placement and limits.
	Namespace      string
	ServiceAccount string
	Resources      Resources
	NodeSelector   map[string]string
	Tolerations    []Toleration
	// SecurityContext, when set, is applied to the task container. Most
	// deployment tasks need none (the default hardened context is fine), but a
	// task that builds container images in-cluster (e.g. a BuildKit-based deploy
	// runner) needs elevated privileges. Nil = the backend's default.
	SecurityContext *SecurityContext
	// Affinity is an optional raw Kubernetes affinity object (as parsed from
	// YAML config), passed through to the pod spec verbatim.
	Affinity map[string]any

	// Lifecycle.
	Timeout        time.Duration // overall deadline for the task (0 = none)
	Retries        int           // extra attempts after a failure
	TTLAfterFinish time.Duration // how long finished work is retained (0 = keep)

	Labels      map[string]string
	Annotations map[string]string
}

// Executor starts executions. One instance per backend is built at startup.
type Executor interface {
	Start(ctx context.Context, spec Spec) (Execution, error)
}

// Execution is a handle on one running task.
type Execution interface {
	// ID returns the execution id (Spec.ID, or the generated one).
	ID() string
	// Status reports the current phase. It is safe to call repeatedly.
	Status(ctx context.Context) (Status, error)
	// Logs opens a log stream. Backends without log access return
	// ErrLogsUnsupported.
	Logs(ctx context.Context, opts LogOptions) (io.ReadCloser, error)
	// Cancel stops the execution (best effort, idempotent).
	Cancel(ctx context.Context) error
	// Cleanup releases any resources the execution left behind. Backends with
	// their own retention (e.g. a Kubernetes TTL) may make this a no-op.
	Cleanup(ctx context.Context) error
}

// Phase is the backend-agnostic lifecycle state of an execution.
type Phase string

const (
	PhasePending   Phase = "pending"   // accepted, not yet scheduled/started
	PhaseScheduled Phase = "scheduled" // placed, waiting to start
	PhaseRunning   Phase = "running"
	PhaseRetrying  Phase = "retrying" // an attempt failed, more attempts remain
	PhaseSucceeded Phase = "succeeded"
	PhaseFailed    Phase = "failed"
	PhaseCancelled Phase = "cancelled"
	PhaseTimedOut  Phase = "timed_out"
)

// Terminal reports whether the phase is final.
func (p Phase) Terminal() bool {
	switch p {
	case PhaseSucceeded, PhaseFailed, PhaseCancelled, PhaseTimedOut:
		return true
	}
	return false
}

// Status is a point-in-time view of an execution.
type Status struct {
	Phase Phase
	// Message is a human-actionable description of the current state, e.g.
	// "image pull failed for ghcr.io/x:v1: not found". Required for failures.
	Message string
	// ExitCode is the task's exit code when known.
	ExitCode *int
	// Details carries backend-specific facts for display and records:
	// job_name, pod_name, node, run_url, ...
	Details map[string]string
}

// LogOptions controls a log stream.
type LogOptions struct {
	// Follow keeps the stream open while the execution runs.
	Follow bool
	// SinceTime resumes from an RFC3339 timestamp (for reconnects). Empty =
	// from the start.
	SinceTime string
	// Timestamps asks the backend to prefix each line with its timestamp.
	Timestamps bool
}

// Resources are the compute requests/limits for the task, as Kubernetes
// quantity strings (e.g. "250m", "512Mi"). Empty fields are omitted.
type Resources struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

// Toleration mirrors a Kubernetes toleration.
type Toleration struct {
	Key      string
	Operator string
	Value    string
	Effect   string
}

// SecurityContext is the subset of a Kubernetes container securityContext the
// executor exposes. Pointer fields distinguish "unset" (omitted) from an
// explicit false/zero. It exists so an image-building deploy runner can request
// the privileges BuildKit needs; ordinary tasks leave it nil.
type SecurityContext struct {
	Privileged             *bool
	RunAsUser              *int64
	RunAsGroup             *int64
	RunAsNonRoot           *bool
	ReadOnlyRootFilesystem *bool
}
