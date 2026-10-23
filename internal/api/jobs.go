package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/progress"
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
	// Stages is the step's breakdown into the jobs a backend ran for it (the
	// GitHub Actions jobs of a workflow run, the phases of a Kubernetes Job).
	Stages []stageView `json:"stages,omitempty"`
}

// stageView is the JSON view of one stage of a step.
type stageView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	URL        string          `json:"url,omitempty"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Steps      []stageStepView `json:"steps,omitempty"`
}

// stageStepView is the JSON view of one step inside a stage.
type stageStepView struct {
	Number     int        `json:"number"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// gateView is the JSON view of one promotion-policy gate verdict.
type gateView struct {
	Gate    string `json:"gate"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
	Ring    string `json:"ring,omitempty"`
	Version string `json:"version,omitempty"`
}

// jobState is the JSON view of a job (no mutex, safe to marshal).
type jobState struct {
	ID     string `json:"id"`
	App    string `json:"app"`
	Action string `json:"action"`
	// Ring is the ring the operation deploys to, when known.
	Ring   string           `json:"ring,omitempty"`
	Status string           `json:"status"`
	Steps  []stepView       `json:"steps"`
	Result *promoter.Result `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
	// Gates are the promotion-policy verdicts the operation was checked
	// against, in evaluation order.
	Gates []gateView `json:"gates,omitempty"`
	// AI diagnosis of a failed job (see handleDiagnoseJob). The generation
	// runs detached from the request, so the UI polls the job for these:
	// DiagnosisStatus moves "" → running → done|failed, and Diagnosis carries
	// the answer once done (also lets a reloaded UI show it without a second
	// model call).
	Diagnosis       string     `json:"diagnosis,omitempty"`
	DiagnosisStatus string     `json:"diagnosis_status,omitempty"`
	DiagnosisError  string     `json:"diagnosis_error,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

// Diagnosis status values.
const (
	diagRunning = "running"
	diagDone    = "done"
	diagFailed  = "failed"
)

// Job tracks the live progress of one operation. It implements promoter.Reporter.
type Job struct {
	mu sync.Mutex
	st jobState
	// done is closed once the job reaches a terminal status.
	done chan struct{}
}

// wait blocks until the job has finished and returns its final state.
func (j *Job) wait() jobState {
	<-j.done
	return j.snapshot()
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

// SetStages implements progress.StageSink: stages replace the current step's
// stage breakdown wholesale.
func (j *Job) SetStages(stages []progress.Stage) {
	views := make([]stageView, len(stages))
	for i, st := range stages {
		v := stageView{
			ID: st.ID, Name: st.Name, Status: st.Status, URL: st.URL,
			StartedAt: copyTime(st.StartedAt), FinishedAt: copyTime(st.FinishedAt),
		}
		for _, sp := range st.Steps {
			v.Steps = append(v.Steps, stageStepView{
				Number: sp.Number, Name: sp.Name, Status: sp.Status,
				StartedAt: copyTime(sp.StartedAt), FinishedAt: copyTime(sp.FinishedAt),
			})
		}
		views[i] = v
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if n := len(j.st.Steps); n > 0 {
		j.st.Steps[n-1].Stages = views
	}
}

// AddGate implements progress.GateSink.
func (j *Job) AddGate(g progress.GateVerdict) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.st.Gates = append(j.st.Gates, gateView{Gate: g.Gate, Verdict: g.Verdict, Detail: g.Detail, Ring: g.Ring, Version: g.Version})
}

// setRing records the ring the operation deploys to.
func (j *Job) setRing(r string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.st.Ring = r
}

// copyTime returns a copy of t so a view never aliases its source.
func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// StepLogs implements promoter.StepLogsProvider: it renders the steps and logs
// collected so far (capped like a diagnosis report). The promoter persists
// this with a failure's history entry so the failure can still be properly
// diagnosed after this in-memory job is gone.
func (j *Job) StepLogs() string {
	return stepsReport(j.snapshot().Steps)
}

// startDiagnosis marks a diagnosis as in flight and reports whether the caller
// won the right to run it. It returns false when one is already running or an
// answer is already stored — the single-flight guard that stops concurrent
// clicks from firing duplicate model calls. A previously failed diagnosis can
// be restarted.
func (j *Job) startDiagnosis() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.st.DiagnosisStatus == diagRunning || j.st.Diagnosis != "" {
		return false
	}
	j.st.DiagnosisStatus = diagRunning
	j.st.DiagnosisError = ""
	return true
}

// finishDiagnosis records the outcome of an in-flight diagnosis.
func (j *Job) finishDiagnosis(text string, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err != nil {
		j.st.DiagnosisStatus = diagFailed
		j.st.DiagnosisError = err.Error()
		return
	}
	j.st.DiagnosisStatus = diagDone
	j.st.Diagnosis = text
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
	defer close(j.done)
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
		// Stage views are never mutated in place (SetStages swaps the whole
		// slice), so sharing the slice is safe; copy the header anyway so the
		// snapshot does not observe a later swap.
		if s.Stages != nil {
			sc.Stages = append([]stageView(nil), s.Stages...)
		}
		cp.Steps[i] = sc
	}
	if j.st.Gates != nil {
		cp.Gates = append([]gateView(nil), j.st.Gates...)
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
	}, done: make(chan struct{})}
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

// latestPerApp returns the newest job of every app, most recent first — the
// shared view the UI polls so one user's operation is visible to all users.
func (m *JobManager) latestPerApp() []jobState {
	m.mu.Lock()
	jobs := make([]*Job, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if j, ok := m.jobs[m.order[i]]; ok {
			jobs = append(jobs, j)
		}
	}
	m.mu.Unlock()

	seen := map[string]bool{}
	out := []jobState{}
	for _, j := range jobs {
		st := j.snapshot()
		if seen[st.App] {
			continue
		}
		seen[st.App] = true
		out = append(out, st)
	}
	return out
}

// run starts fn in the background under a request-detached, timeout-bounded
// context that carries the Job as the progress Reporter. It returns immediately
// with the Job so the caller can hand back its ID.
func (m *JobManager) run(baseCtx context.Context, timeout time.Duration, app, action string, fn func(ctx context.Context) (promoter.Result, error)) *Job {
	return m.runOn(baseCtx, timeout, app, action, "", fn)
}

// runOn is run for an operation whose target ring is known up front; the ring
// is recorded on the job so views can place it without parsing step titles.
func (m *JobManager) runOn(baseCtx context.Context, timeout time.Duration, app, action, targetRing string, fn func(ctx context.Context) (promoter.Result, error)) *Job {
	j := m.create(app, action)
	j.setRing(targetRing)
	ctx, cancel := context.WithTimeout(promoter.WithReporter(context.WithoutCancel(baseCtx), j), timeout)
	go func() {
		defer cancel()
		j.markRunning()
		res, err := fn(ctx)
		j.finish(res, err)
	}()
	return j
}
