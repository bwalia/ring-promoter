// Package progress carries step-by-step progress reporting through a context.
// It is a leaf package so that both the promotion engine (which starts and
// finishes steps) and lower layers such as execution backends (which stream
// log lines into the current step) can emit progress without import cycles.
package progress

import (
	"context"
	"time"
)

// Reporter receives fine-grained progress during an operation. The promoter
// drives a single operation from one goroutine, so StartStep/FinishStep calls
// are ordered; Log may additionally be called from an execution backend's
// log-streaming goroutine, so implementations must be safe for concurrent use.
type Reporter interface {
	// StartStep begins a new step and makes it the current step.
	StartStep(id, title string)
	// Log appends a line to the current step.
	Log(line string)
	// FinishStep completes the current step with a status and an optional
	// closing message.
	FinishStep(status, message string)
}

type reporterKey struct{}

// WithReporter attaches a Reporter to ctx.
func WithReporter(ctx context.Context, r Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, r)
}

// FromContext returns the Reporter in ctx, or a no-op if none is set, so
// callers never need a nil check.
func FromContext(ctx context.Context) Reporter {
	if r, ok := ctx.Value(reporterKey{}).(Reporter); ok && r != nil {
		return r
	}
	return noopReporter{}
}

type noopReporter struct{}

func (noopReporter) StartStep(string, string)  {}
func (noopReporter) Log(string)                {}
func (noopReporter) FinishStep(string, string) {}

// Link is a URL an operation produced: a workflow run, a build artifact, a
// release. Kind is a coarse category ("ci", "artifact", "release", "ios", ...).
type Link struct {
	Label string
	URL   string
	Kind  string
}

// OutputSink is optionally implemented by a Reporter that wants to know what a
// deploy produced — links, and log text that was not streamed line by line.
// Lower layers emit through AddLink/AddOutput and never need to know whether
// anyone is listening.
type OutputSink interface {
	AddLink(l Link)
	AddOutput(text string)
}

// AddLink hands l to the context's Reporter when it is an OutputSink.
func AddLink(ctx context.Context, l Link) {
	if s, ok := FromContext(ctx).(OutputSink); ok {
		s.AddLink(l)
	}
}

// AddOutput hands log text to the context's Reporter when it is an OutputSink.
func AddOutput(ctx context.Context, text string) {
	if s, ok := FromContext(ctx).(OutputSink); ok {
		s.AddOutput(text)
	}
}

// Stage is one unit of work inside a step that a backend runs as its own job
// — a GitHub Actions job, a phase of a Kubernetes Job. Status is one of
// queued|running|success|failed|skipped|cancelled. Stages are a snapshot: each
// SetStages call replaces the previous set wholesale.
type Stage struct {
	ID         string
	Name       string
	Status     string
	URL        string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Steps      []StageStep
}

// StageStep is one step of a Stage (a GitHub Actions job step).
type StageStep struct {
	Number     int
	Name       string
	Status     string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Stage statuses.
const (
	StageQueued    = "queued"
	StageRunning   = "running"
	StageSuccess   = "success"
	StageFailed    = "failed"
	StageSkipped   = "skipped"
	StageCancelled = "cancelled"
)

// StageSink is optionally implemented by a Reporter that wants the stage
// breakdown of the current step.
type StageSink interface {
	SetStages(stages []Stage)
}

// SetStages hands stages to the context's Reporter when it is a StageSink.
func SetStages(ctx context.Context, stages []Stage) {
	if s, ok := FromContext(ctx).(StageSink); ok {
		s.SetStages(stages)
	}
}

// GateVerdict is one promotion-policy gate's verdict for an operation (see
// promoter.Gate): Verdict is pass|fail|overridden.
type GateVerdict struct {
	Gate    string
	Verdict string
	Detail  string
	Ring    string
	// Version is the version the gate checked.
	Version string
}

// GateSink is optionally implemented by a Reporter that records gate verdicts.
type GateSink interface {
	AddGate(g GateVerdict)
}

// AddGate hands g to the context's Reporter when it is a GateSink.
func AddGate(ctx context.Context, g GateVerdict) {
	if s, ok := FromContext(ctx).(GateSink); ok {
		s.AddGate(g)
	}
}
