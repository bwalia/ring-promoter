// Package pipeline runs multi-app release pipelines: a DAG of stages from
// config (see config.PipelineConfig) that deploy several applications into one
// ring, check QA and security go/no-go gates and hold for human approvals — the CircleCI
// workflow / GitHub Actions jobs model, across services. Each app deploy runs
// as an ordinary job (so it keeps its own steps, stages and gate verdicts);
// this package only sequences them. Runs live in memory, like jobs.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/ring"
)

// Run, stage and item statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusWaiting   = "waiting"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
	StatusCancelled = "cancelled"
	StatusReused    = "reused"
)

// Gate verdicts on gate items.
const (
	VerdictGo      = "go"
	VerdictCheck   = "check"
	VerdictNoGo    = "no_go"
	VerdictUnknown = "unknown"
)

// Security evaluation verdicts a Runner returns (promoter.Security*).
const (
	SecurityGo      = "go"
	SecurityNoGo    = "no_go"
	SecurityPending = "pending"
	SecurityNone    = "none"
)

// SecurityResult is one app's security evaluation for a security gate.
type SecurityResult struct {
	// Verdict is go, no_go, pending (a required scan has not reported) or
	// none (no version to check).
	Verdict string
	// Version is the version that was checked.
	Version string
	Detail  string
	// Checks is the per-scan-kind breakdown, already JSON-encoded.
	Checks json.RawMessage
}

// Errors the API maps to status codes.
var (
	ErrRunNotFound   = errors.New("pipeline run not found")
	ErrStageNotFound = errors.New("pipeline stage not found")
	ErrNotWaiting    = errors.New("stage is not waiting for approval")
	ErrRunFinished   = errors.New("pipeline run has finished")
	ErrBadRequest    = errors.New("invalid pipeline run request")
)

// Deployment is one app operation the Runner started.
type Deployment struct {
	// JobID identifies the tracked job, so views can link to its steps.
	JobID string
	// Wait blocks until the operation ends and reports its outcome; version
	// is what was deployed when the runner knows it.
	Wait func() (ok bool, version, message string)
}

// Runner is what a pipeline needs from the rest of Ring Promoter.
type Runner interface {
	// Deploy starts app's operation into ring: a seed of version into the
	// first ring, a promotion from the previous ring otherwise.
	Deploy(ctx context.Context, app, ringName, version string) (Deployment, error)
	// QAVerdict is the QA agent's current verdict for app on ring.
	QAVerdict(ctx context.Context, app, ringName string) (verdict, detail string, err error)
	// SecurityVerdict evaluates app's security scans for entering ring.
	// version is the run's requested version ("" lets the runner resolve
	// what the ring would receive).
	SecurityVerdict(ctx context.Context, app, ringName, version string) (SecurityResult, error)
	// Audit records a run lifecycle event.
	Audit(ctx context.Context, ringName, action string, detail map[string]string)
}

// Request starts a run.
type Request struct {
	Ring              string            `json:"ring"`
	Versions          map[string]string `json:"versions,omitempty"`
	ChangeRequestCode string            `json:"change_request_code,omitempty"`
	TriggeredBy       string            `json:"triggered_by,omitempty"`
}

// Item is one app within a stage.
type Item struct {
	App     string `json:"app"`
	Status  string `json:"status"`
	JobID   string `json:"job_id,omitempty"`
	Version string `json:"version,omitempty"`
	Verdict string `json:"verdict,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// Checks is a security gate item's per-scan-kind breakdown.
	Checks json.RawMessage `json:"checks,omitempty"`
}

// Stage is one stage of a run.
type Stage struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Type        string     `json:"type"`
	Gate        string     `json:"gate,omitempty"`
	Needs       []string   `json:"needs"`
	Status      string     `json:"status"`
	Message     string     `json:"message,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	ApprovedBy  string     `json:"approved_by,omitempty"`
	Items       []Item     `json:"items"`
}

// Run is the JSON view of one pipeline run.
type Run struct {
	ID          string     `json:"id"`
	Pipeline    string     `json:"pipeline"`
	DisplayName string     `json:"display_name"`
	Ring        string     `json:"ring"`
	Status      string     `json:"status"`
	TriggeredBy string     `json:"triggered_by,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	RerunOf     string     `json:"rerun_of,omitempty"`
	Stages      []Stage    `json:"stages"`
}

// decision is an approval stage's outcome.
type decision struct {
	approve bool
	by      string
	note    string
}

// run is one live run. All fields below mu are guarded by it.
type run struct {
	cfg    config.PipelineConfig
	req    Request
	cancel context.CancelFunc
	// wake nudges the scheduler after a stage changes state.
	wake chan struct{}

	mu        sync.Mutex
	st        Run
	cancelled bool
	// decisions delivers approve/reject to a waiting approval stage.
	decisions map[string]chan decision
}

// Manager starts and tracks pipeline runs.
type Manager struct {
	runner Runner
	// Poll is how often a QA gate re-reads a pending verdict.
	Poll time.Duration
	now  func() time.Time

	mu    sync.Mutex
	runs  map[string]*run
	order []string
	seq   int64
	max   int
}

// NewManager returns a Manager retaining the newest 100 runs.
func NewManager(r Runner) *Manager {
	return &Manager{runner: r, Poll: 5 * time.Second, now: func() time.Time { return time.Now().UTC() },
		runs: map[string]*run{}, max: 100}
}

// Validate checks a run request against the pipeline before anything starts:
// a known ring, and for the first ring a version for every deployed app
// (from the request, else resolved by version — typically the ring's ref).
func Validate(p config.PipelineConfig, req Request, version func(app string) string) error {
	if !ring.IsValid(req.Ring) {
		return fmt.Errorf("%w: unknown ring %q", ErrBadRequest, req.Ring)
	}
	if req.Ring != ring.All()[0].Name {
		return nil
	}
	for _, s := range p.Stages {
		if s.ResolvedType() != config.StageDeploy {
			continue
		}
		for _, app := range s.Apps {
			if req.Versions[app] == "" && version(app) == "" {
				return fmt.Errorf("%w: a version is required for %s (no ref pinned on ring %s)", ErrBadRequest, app, req.Ring)
			}
		}
	}
	return nil
}

// Start begins a run of p. versions must already be resolved for a first-ring
// run (see Validate). When rerunOf names a previous run, stages that
// succeeded there are reused and the rest run again (rerun from failed).
func (m *Manager) Start(ctx context.Context, p config.PipelineConfig, req Request, rerunOf string) (Run, error) {
	var source *Run
	if rerunOf != "" {
		src, err := m.Get(rerunOf)
		if err != nil {
			return Run{}, err
		}
		source = &src
	}

	r := &run{cfg: p, req: req, wake: make(chan struct{}, 1), decisions: map[string]chan decision{}}
	r.st = Run{
		Pipeline: p.Name, DisplayName: p.Title(), Ring: req.Ring, Status: StatusQueued,
		TriggeredBy: req.TriggeredBy, StartedAt: m.now(), RerunOf: rerunOf,
	}
	for _, s := range p.Stages {
		st := Stage{
			Name: s.Name, DisplayName: s.Title(), Type: s.ResolvedType(),
			Needs: append([]string{}, s.Needs...), Status: StatusQueued, Items: []Item{},
		}
		if st.Type == config.StageGate {
			st.Gate = s.ResolvedGate()
		}
		for _, app := range stageApps(p, s) {
			st.Items = append(st.Items, Item{App: app, Status: StatusQueued, Version: req.Versions[app]})
		}
		if source != nil {
			if prev := findStage(source.Stages, s.Name); prev != nil &&
				(prev.Status == StatusSuccess || prev.Status == StatusReused) {
				st.Status = StatusReused
				st.Message = "reused from " + source.ID
				st.Items = make([]Item, len(prev.Items))
				for i, it := range prev.Items {
					it.Status = StatusReused
					st.Items[i] = it
				}
			}
		}
		r.st.Stages = append(r.st.Stages, st)
	}

	m.mu.Lock()
	m.seq++
	r.st.ID = fmt.Sprintf("run-%d", m.seq)
	m.runs[r.st.ID] = r
	m.order = append(m.order, r.st.ID)
	for len(m.order) > m.max {
		delete(m.runs, m.order[0])
		m.order = m.order[1:]
	}
	m.mu.Unlock()

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel = cancel
	detail := map[string]string{"pipeline": p.Name, "run": r.st.ID}
	if rerunOf != "" {
		detail["rerun_of"] = rerunOf
	}
	m.runner.Audit(ctx, req.Ring, "start", detail)
	go m.schedule(runCtx, r)
	return r.view(), nil
}

// stageApps lists the apps a stage acts on. A gate without apps checks every
// app its direct needs deploy.
func stageApps(p config.PipelineConfig, s config.PipelineStage) []string {
	if len(s.Apps) > 0 || s.ResolvedType() != config.StageGate {
		return append([]string{}, s.Apps...)
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range s.Needs {
		for _, ns := range p.Stages {
			if ns.Name != n || ns.ResolvedType() != config.StageDeploy {
				continue
			}
			for _, a := range ns.Apps {
				if !seen[a] {
					seen[a] = true
					out = append(out, a)
				}
			}
		}
	}
	return out
}

func findStage(stages []Stage, name string) *Stage {
	for i := range stages {
		if stages[i].Name == name {
			return &stages[i]
		}
	}
	return nil
}

// Get returns a run by id.
func (m *Manager) Get(id string) (Run, error) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return Run{}, ErrRunNotFound
	}
	return r.view(), nil
}

// Request returns the request a run was started with (for a rerun).
func (m *Manager) Request(id string) (config.PipelineConfig, Request, error) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return config.PipelineConfig{}, Request{}, ErrRunNotFound
	}
	return r.cfg, r.req, nil
}

// List returns the newest runs of a pipeline (all pipelines when name is ""),
// newest first, at most limit.
func (m *Manager) List(name string, limit int) []Run {
	m.mu.Lock()
	runs := make([]*run, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if r, ok := m.runs[m.order[i]]; ok && (name == "" || r.cfg.Name == name) {
			runs = append(runs, r)
		}
		if limit > 0 && len(runs) >= limit {
			break
		}
	}
	m.mu.Unlock()
	out := make([]Run, len(runs))
	for i, r := range runs {
		out[i] = r.view()
	}
	return out
}

// Decide approves or rejects an approval stage that is waiting.
func (m *Manager) Decide(ctx context.Context, id, stage string, approve bool, by, note string) (Run, error) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return Run{}, ErrRunNotFound
	}
	r.mu.Lock()
	st := findStage(r.st.Stages, stage)
	if st == nil {
		r.mu.Unlock()
		return Run{}, ErrStageNotFound
	}
	ch, waiting := r.decisions[stage]
	if !waiting || st.Status != StatusWaiting {
		r.mu.Unlock()
		return Run{}, ErrNotWaiting
	}
	delete(r.decisions, stage)
	r.mu.Unlock()

	action := "reject"
	if approve {
		action = "approve"
	}
	m.runner.Audit(ctx, r.req.Ring, action, map[string]string{
		"pipeline": r.cfg.Name, "run": id, "stage": stage, "by": by, "note": note,
	})
	ch <- decision{approve: approve, by: by, note: note}
	return r.view(), nil
}

// Cancel stops scheduling a run: queued and waiting stages are cancelled.
// Deploys already running are not interrupted — they finish, and their stage
// records the outcome.
func (m *Manager) Cancel(ctx context.Context, id string) (Run, error) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return Run{}, ErrRunNotFound
	}
	r.mu.Lock()
	if r.st.FinishedAt != nil {
		r.mu.Unlock()
		return Run{}, ErrRunFinished
	}
	r.cancelled = true
	r.mu.Unlock()
	m.runner.Audit(ctx, r.req.Ring, "cancel", map[string]string{"pipeline": r.cfg.Name, "run": id})
	r.cancel()
	r.nudge()
	return r.view(), nil
}

func (r *run) nudge() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// view returns a deep copy of the run, with its status derived.
func (r *run) view() Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := r.st
	cp.Status = r.statusLocked()
	cp.Stages = make([]Stage, len(r.st.Stages))
	for i, s := range r.st.Stages {
		s.Needs = append([]string{}, s.Needs...)
		s.Items = append([]Item{}, s.Items...)
		cp.Stages[i] = s
	}
	return cp
}

// update applies fn to the named stage under the lock.
func (r *run) update(name string, fn func(s *Stage)) {
	r.mu.Lock()
	if s := findStage(r.st.Stages, name); s != nil {
		fn(s)
	}
	r.mu.Unlock()
	r.nudge()
}

// schedule drives a run: it starts every stage whose needs have succeeded,
// skips stages behind a failure, and finishes once nothing can move.
func (m *Manager) schedule(ctx context.Context, r *run) {
	defer r.cancel()
	var wg sync.WaitGroup
	for {
		r.mu.Lock()
		var ready []config.PipelineStage
		active := false
		for _, cs := range r.cfg.Stages {
			s := findStage(r.st.Stages, cs.Name)
			switch s.Status {
			case StatusRunning, StatusWaiting:
				active = true
				continue
			case StatusQueued:
			default:
				continue
			}
			if r.cancelled {
				s.Status = StatusCancelled
				s.Message = "run cancelled"
				continue
			}
			blocked, pending := false, false
			for _, n := range cs.Needs {
				switch findStage(r.st.Stages, n).Status {
				case StatusSuccess, StatusReused:
				case StatusFailed, StatusSkipped, StatusCancelled:
					blocked = true
				default:
					pending = true
				}
			}
			switch {
			case blocked:
				s.Status = StatusSkipped
				s.Message = "a stage it needs did not succeed"
				for i := range s.Items {
					s.Items[i].Status = StatusSkipped
				}
				// A newly skipped stage can unblock-to-skip its dependents.
				r.nudge()
			case !pending:
				now := m.now()
				s.Status = StatusRunning
				s.StartedAt = &now
				ready = append(ready, cs)
				active = true
			}
		}
		r.st.Status = r.statusLocked()
		if !active && len(ready) == 0 && len(r.wake) == 0 {
			now := m.now()
			r.st.FinishedAt = &now
			r.st.Status = r.statusLocked()
			r.mu.Unlock()
			wg.Wait()
			return
		}
		r.mu.Unlock()

		for _, cs := range ready {
			wg.Add(1)
			go func(cs config.PipelineStage) {
				defer wg.Done()
				m.runStage(ctx, r, cs)
			}(cs)
		}
		<-r.wake
	}
}

// statusLocked derives the run status from its stages. Callers hold r.mu.
func (r *run) statusLocked() string {
	running, waiting, failed := false, false, false
	for _, s := range r.st.Stages {
		switch s.Status {
		case StatusRunning:
			running = true
		case StatusWaiting:
			waiting = true
		case StatusFailed:
			failed = true
		}
	}
	switch {
	case running:
		return StatusRunning
	case waiting:
		return StatusWaiting
	case r.st.FinishedAt == nil:
		if r.cancelled {
			return StatusRunning
		}
		for _, s := range r.st.Stages {
			if s.Status != StatusQueued {
				return StatusRunning
			}
		}
		return StatusQueued
	case r.cancelled:
		return StatusCancelled
	case failed:
		return StatusFailed
	}
	for _, s := range r.st.Stages {
		if s.Status == StatusSkipped || s.Status == StatusCancelled {
			return StatusFailed
		}
	}
	return StatusSuccess
}

// finish closes a stage with a status and message.
func (m *Manager) finish(r *run, name, status, msg string) {
	now := m.now()
	r.update(name, func(s *Stage) {
		s.Status = status
		s.Message = msg
		s.FinishedAt = &now
	})
}

func (m *Manager) runStage(ctx context.Context, r *run, cs config.PipelineStage) {
	switch cs.ResolvedType() {
	case config.StageDeploy:
		m.runDeploy(ctx, r, cs)
	case config.StageGate:
		m.runGate(ctx, r, cs)
	case config.StageApproval:
		m.runApproval(ctx, r, cs)
	}
}

// runDeploy deploys every app of the stage in parallel.
func (m *Manager) runDeploy(ctx context.Context, r *run, cs config.PipelineStage) {
	type outcome struct {
		ok  bool
		msg string
	}
	results := make([]outcome, len(cs.Apps))
	var wg sync.WaitGroup
	for i, app := range cs.Apps {
		r.mu.Lock()
		version := r.req.Versions[app]
		r.mu.Unlock()
		// Deploys are deliberately not tied to the run's cancellation: a
		// half-cancelled deploy is worse than a finished one.
		d, err := m.runner.Deploy(context.WithoutCancel(ctx), app, r.req.Ring, version)
		if err != nil {
			results[i] = outcome{msg: fmt.Sprintf("%s: %v", app, err)}
			setItem(r, cs.Name, app, func(it *Item) { it.Status = StatusFailed; it.Detail = err.Error() })
			continue
		}
		setItem(r, cs.Name, app, func(it *Item) { it.Status = StatusRunning; it.JobID = d.JobID })
		wg.Add(1)
		go func(i int, app string) {
			defer wg.Done()
			ok, v, msg := d.Wait()
			results[i] = outcome{ok: ok, msg: fmt.Sprintf("%s: %s", app, msg)}
			setItem(r, cs.Name, app, func(it *Item) {
				it.Status = StatusFailed
				if ok {
					it.Status = StatusSuccess
				}
				if v != "" {
					it.Version = v
				}
				it.Detail = msg
			})
		}(i, app)
	}
	wg.Wait()
	for _, o := range results {
		if !o.ok {
			m.finish(r, cs.Name, StatusFailed, o.msg)
			return
		}
	}
	msg := fmt.Sprintf("deployed %d app(s) to %s", len(cs.Apps), r.req.Ring)
	if ctx.Err() != nil {
		msg += " (finished after the run was cancelled)"
	}
	m.finish(r, cs.Name, StatusSuccess, msg)
}

// gateCheck is one poll of one app for a gate: its verdict (go, no_go or
// anything else for pending) and what to record on the item.
type gateCheck struct {
	verdict, detail, version string
	checks                   json.RawMessage
}

// gateKind holds what differs between gate kinds.
type gateKind struct {
	check   func(ctx context.Context, app, version string) gateCheck
	noGo    func(app string, c gateCheck) string
	waiting string // "waiting for %d app(s)" format
	timeout string // "NO-GO: ... within %s" format
	pass    string
}

func (m *Manager) gateKind(r *run, cs config.PipelineStage) gateKind {
	if cs.ResolvedGate() == config.GateSecurity {
		return gateKind{
			check: func(ctx context.Context, app, version string) gateCheck {
				res, err := m.runner.SecurityVerdict(ctx, app, r.req.Ring, version)
				if err != nil {
					return gateCheck{verdict: VerdictUnknown, detail: err.Error(), version: version}
				}
				c := gateCheck{detail: res.Detail, version: res.Version, checks: res.Checks}
				switch res.Verdict {
				case SecurityGo:
					c.verdict = VerdictGo
				case SecurityNoGo:
					c.verdict = VerdictNoGo
				default:
					c.verdict = VerdictUnknown
				}
				return c
			},
			noGo: func(app string, c gateCheck) string {
				return fmt.Sprintf("NO-GO: security policy for %s %s: %s", app, c.version, c.detail)
			},
			waiting: "waiting for security scans on %d app(s)",
			timeout: "NO-GO: security scans incomplete within %s",
			pass:    "GO: security policy satisfied for every app",
		}
	}
	return gateKind{
		check: func(ctx context.Context, app, _ string) gateCheck {
			v, detail, err := m.runner.QAVerdict(ctx, app, r.req.Ring)
			if err != nil {
				v, detail = VerdictUnknown, err.Error()
			}
			return gateCheck{verdict: v, detail: detail}
		},
		noGo:    func(app string, _ gateCheck) string { return fmt.Sprintf("NO-GO: QA verdict for %s is no_go", app) },
		waiting: "waiting for a QA verdict on %d app(s)",
		timeout: "NO-GO: no QA verdict within %s",
		pass:    "GO: QA verdict is go for every app",
	}
}

// runGate polls every app's verdict (QA or security, by gate kind) until all
// are go (pass), any is no_go (fail), the timeout passes (fail) or the run is
// cancelled.
func (m *Manager) runGate(ctx context.Context, r *run, cs config.PipelineStage) {
	apps := stageApps(r.cfg, cs)
	if len(apps) == 0 {
		m.finish(r, cs.Name, StatusSuccess, "no apps to check")
		return
	}
	kind := m.gateKind(r, cs)
	deadline := time.NewTimer(cs.ResolvedTimeout())
	defer deadline.Stop()
	for {
		pending := 0
		for _, app := range apps {
			r.mu.Lock()
			version := r.req.Versions[app]
			r.mu.Unlock()
			c := kind.check(ctx, app, version)
			status := StatusWaiting
			switch c.verdict {
			case VerdictGo:
				status = StatusSuccess
			case VerdictNoGo:
				status = StatusFailed
			default:
				pending++
			}
			setItem(r, cs.Name, app, func(it *Item) {
				it.Status = status
				it.Verdict = c.verdict
				it.Detail = c.detail
				if c.version != "" {
					it.Version = c.version
				}
				if c.checks != nil {
					it.Checks = c.checks
				}
			})
			if c.verdict == VerdictNoGo {
				m.finish(r, cs.Name, StatusFailed, kind.noGo(app, c))
				return
			}
		}
		if pending == 0 {
			m.finish(r, cs.Name, StatusSuccess, kind.pass)
			return
		}
		r.update(cs.Name, func(s *Stage) {
			s.Status = StatusWaiting
			s.Message = fmt.Sprintf(kind.waiting, pending)
		})
		select {
		case <-ctx.Done():
			m.finish(r, cs.Name, StatusCancelled, "run cancelled")
			return
		case <-deadline.C:
			m.finish(r, cs.Name, StatusFailed, fmt.Sprintf(kind.timeout, cs.ResolvedTimeout()))
			return
		case <-time.After(m.Poll):
		}
	}
}

// runApproval holds until someone approves or rejects, the timeout passes or
// the run is cancelled.
func (m *Manager) runApproval(ctx context.Context, r *run, cs config.PipelineStage) {
	ch := make(chan decision, 1)
	r.mu.Lock()
	r.decisions[cs.Name] = ch
	if s := findStage(r.st.Stages, cs.Name); s != nil {
		s.Status = StatusWaiting
		s.Message = "waiting for approval"
	}
	r.mu.Unlock()
	r.nudge()

	deadline := time.NewTimer(cs.ResolvedTimeout())
	defer deadline.Stop()
	select {
	case d := <-ch:
		by := d.by
		if by == "" {
			by = "anonymous"
		}
		r.update(cs.Name, func(s *Stage) { s.ApprovedBy = by })
		msg := ""
		if d.note != "" {
			msg = ": " + d.note
		}
		if d.approve {
			m.finish(r, cs.Name, StatusSuccess, "approved by "+by+msg)
		} else {
			m.finish(r, cs.Name, StatusFailed, "rejected by "+by+msg)
		}
	case <-ctx.Done():
		m.forget(r, cs.Name)
		m.finish(r, cs.Name, StatusCancelled, "run cancelled")
	case <-deadline.C:
		m.forget(r, cs.Name)
		m.finish(r, cs.Name, StatusFailed, fmt.Sprintf("not approved within %s", cs.ResolvedTimeout()))
	}
}

// forget withdraws a stage's pending approval so a late decision is refused.
func (m *Manager) forget(r *run, stage string) {
	r.mu.Lock()
	delete(r.decisions, stage)
	r.mu.Unlock()
}

func setItem(r *run, stage, app string, fn func(it *Item)) {
	r.update(stage, func(s *Stage) {
		for i := range s.Items {
			if s.Items[i].App == app {
				fn(&s.Items[i])
			}
		}
	})
}
