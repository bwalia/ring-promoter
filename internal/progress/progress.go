// Package progress carries step-by-step progress reporting through a context.
// It is a leaf package so that both the promotion engine (which starts and
// finishes steps) and lower layers such as execution backends (which stream
// log lines into the current step) can emit progress without import cycles.
package progress

import "context"

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
