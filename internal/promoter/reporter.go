package promoter

import "context"

// Step statuses reported during an operation.
const (
	StepRunning = "running"
	StepSuccess = "success"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Reporter receives fine-grained, step-by-step progress during a seed / promote
// / rollback so the API layer can surface a live view (à la GitHub Actions).
//
// The promoter drives a single operation from one goroutine, so calls are
// ordered; implementations must still be safe for concurrent reads (the API
// serves job status while the operation runs).
type Reporter interface {
	// StartStep begins a new step and makes it the current step.
	StartStep(id, title string)
	// Log appends a line to the current step.
	Log(line string)
	// FinishStep completes the current step with a status (StepSuccess/StepFailed/
	// StepSkipped) and an optional closing message.
	FinishStep(status, message string)
}

// StepLogsProvider is optionally implemented by a Reporter that can render the
// step-by-step logs collected so far. When an operation fails, the promoter
// persists these logs with the failure's history entry so AI diagnosis has
// real evidence even after the in-memory job is gone.
type StepLogsProvider interface {
	StepLogs() string
}

type reporterKey struct{}

// WithReporter attaches a Reporter to ctx so promoter operations emit progress.
func WithReporter(ctx context.Context, r Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, r)
}

// reporterFrom returns the Reporter in ctx, or a no-op if none is set (so the
// synchronous/tested code paths need no reporter).
func reporterFrom(ctx context.Context) Reporter {
	if r, ok := ctx.Value(reporterKey{}).(Reporter); ok && r != nil {
		return r
	}
	return noopReporter{}
}

type noopReporter struct{}

func (noopReporter) StartStep(string, string)  {}
func (noopReporter) Log(string)                {}
func (noopReporter) FinishStep(string, string) {}
