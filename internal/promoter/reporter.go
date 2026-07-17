package promoter

import (
	"context"

	"github.com/example/ring-promoter/internal/progress"
)

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
// It is an alias for progress.Reporter: the interface lives in the leaf
// progress package so execution backends can stream log lines into the current
// step without importing the promoter (which would be an import cycle).
type Reporter = progress.Reporter

// StepLogsProvider is optionally implemented by a Reporter that can render the
// step-by-step logs collected so far. When an operation fails, the promoter
// persists these logs with the failure's history entry so AI diagnosis has
// real evidence even after the in-memory job is gone.
type StepLogsProvider interface {
	StepLogs() string
}

// WithReporter attaches a Reporter to ctx so promoter operations emit progress.
func WithReporter(ctx context.Context, r Reporter) context.Context {
	return progress.WithReporter(ctx, r)
}

// reporterFrom returns the Reporter in ctx, or a no-op if none is set (so the
// synchronous/tested code paths need no reporter).
func reporterFrom(ctx context.Context) Reporter {
	return progress.FromContext(ctx)
}
