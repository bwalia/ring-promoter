// Package queue holds the small pieces of contract shared by the API producer
// and the worker consumer: the Redis key names and the job status vocabulary.
// Keeping them in one place stops the two binaries from drifting apart.
package queue

// Key is the Redis list the API pushes jobs onto and the worker pops from.
const Key = "imageproc:jobs"

// StatusKey returns the Redis string key holding the status of job id.
func StatusKey(id string) string {
	return "imageproc:job:" + id
}

// Job statuses, written by the API on submit and by the worker as it processes.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusDone       = "done"
)
