package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/promoter"
)

// Job status values.
const (
	jobPending = "pending"
	jobRunning = "running"
	jobSuccess = "success"
	jobFailed  = "failed"
)

// stepView is the JSON view of a single step.
type stepView struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Logs       []string   `json:"logs"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// jobState is the JSON view of a job (no mutex, safe to marshal).
type jobState struct {
	ID         string           `json:"id"`
	App        string           `json:"app"`
	Action     string           `json:"action"`
	Status     string           `json:"status"`
	Steps      []stepView       `json:"steps"`
	Result     *promoter.Result `json:"result,omitempty"`
	Error      string           `json:"error,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
}

// Job tracks the live progress of one operation. It implements promoter.Reporter.
type Job struct {
	mu sync.Mutex
	st jobState
}

func (j *Job) id() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.st.ID
}

// StartStep implements promoter.Reporter.
func (j *Job) StartStep(id, title string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.st.Steps = append(j.st.Steps, stepView{
		ID: id, Title: title, Status: promoter.StepRunning,
		StartedAt: time.Now().UTC(), Logs: []string{},
	})
}

// Log implements promoter.Reporter (appends to the current step).
func (j *Job) Log(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if n := len(j.st.Steps); n > 0 {
		j.st.Steps[n-1].Logs = append(j.st.Steps[n-1].Logs, line)
	}
}

// FinishStep implements promoter.Reporter.
func (j *Job) FinishStep(status, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if n := len(j.st.Steps); n > 0 {
		s := &j.st.Steps[n-1]
		s.Status = status
		if message != "" {
			s.Logs = append(s.Logs, message)
		}
		t := time.Now().UTC()
		s.FinishedAt = &t
	}
}

func (j *Job) markRunning() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.st.Status = jobRunning
}

// finish records the terminal outcome.
func (j *Job) finish(res promoter.Result, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	// Defensively close a dangling running step.
	if n := len(j.st.Steps); n > 0 && j.st.Steps[n-1].Status == promoter.StepRunning {
		t := time.Now().UTC()
		j.st.Steps[n-1].Status = promoter.StepFailed
		j.st.Steps[n-1].FinishedAt = &t
	}
	t := time.Now().UTC()
	j.st.FinishedAt = &t
	if err != nil {
		j.st.Status = jobFailed
		j.st.Error = err.Error()
		return
	}
	j.st.Result = &res
	if res.Success {
		j.st.Status = jobSuccess
	} else {
		j.st.Status = jobFailed
	}
}

// snapshot returns a deep copy safe to marshal without holding the lock.
func (j *Job) snapshot() jobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	cp := j.st
	cp.Steps = make([]stepView, len(j.st.Steps))
	for i, s := range j.st.Steps {
		sc := s
		// make (not append to nil) so a step with no logs marshals as [] —
		// append([]string(nil), ...) of an empty slice yields nil → JSON null.
		sc.Logs = make([]string, len(s.Logs))
		copy(sc.Logs, s.Logs)
		cp.Steps[i] = sc
	}
	if j.st.Result != nil {
		r := *j.st.Result
		cp.Result = &r
	}
	if j.st.FinishedAt != nil {
		t := *j.st.FinishedAt
		cp.FinishedAt = &t
	}
	return cp
}

// JobManager stores recent jobs in memory and runs operations asynchronously.
type JobManager struct {
	mu    sync.Mutex
	jobs  map[string]*Job
	order []string
	seq   int64
	max   int
}

// NewJobManager returns a JobManager retaining the most recent jobs.
func NewJobManager() *JobManager {
	return &JobManager{jobs: make(map[string]*Job), max: 200}
}

func (m *JobManager) create(app, action string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("job-%d", m.seq)
	j := &Job{st: jobState{
		ID: id, App: app, Action: action, Status: jobPending,
		Steps: []stepView{}, StartedAt: time.Now().UTC(),
	}}
	m.jobs[id] = j
	m.order = append(m.order, id)
	for len(m.order) > m.max {
		delete(m.jobs, m.order[0])
		m.order = m.order[1:]
	}
	return j
}

func (m *JobManager) get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

// run starts fn in the background under a request-detached, timeout-bounded
// context that carries the Job as the progress Reporter. It returns immediately
// with the Job so the caller can hand back its ID.
func (m *JobManager) run(baseCtx context.Context, timeout time.Duration, app, action string, fn func(ctx context.Context) (promoter.Result, error)) *Job {
	j := m.create(app, action)
	ctx, cancel := context.WithTimeout(promoter.WithReporter(context.WithoutCancel(baseCtx), j), timeout)
	go func() {
		defer cancel()
		j.markRunning()
		res, err := fn(ctx)
		j.finish(res, err)
	}()
	return j
}
