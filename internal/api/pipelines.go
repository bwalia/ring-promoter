package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/pipeline"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
)

// pipelineRunner adapts the server to pipeline.Runner: every app deploy runs
// as an ordinary async job, so it shows in /api/jobs with its own steps.
type pipelineRunner struct{ s *Server }

func (p pipelineRunner) Deploy(ctx context.Context, app, ringName, version string) (pipeline.Deployment, error) {
	var job *Job
	if ringName == ring.All()[0].Name {
		job = p.s.jobs.runOn(ctx, p.s.opTimeout, app, "seed", ringName, func(ctx context.Context) (promoter.Result, error) {
			return p.s.prom.Seed(ctx, app, ringName, version)
		})
	} else {
		prev, ok := ring.Prev(ringName)
		if !ok {
			return pipeline.Deployment{}, errors.New("unknown ring " + ringName)
		}
		job = p.s.jobs.runOn(ctx, p.s.opTimeout, app, "promote", ringName, func(ctx context.Context) (promoter.Result, error) {
			return p.s.prom.Promote(ctx, app, prev.Name)
		})
	}
	return pipeline.Deployment{JobID: job.id(), Wait: func() (bool, string, string) {
		st := job.wait()
		switch {
		case st.Error != "":
			return false, "", st.Error
		case st.Result == nil:
			return false, "", "job ended without a result"
		}
		return st.Status == jobSuccess, st.Result.Version, st.Result.Message
	}}, nil
}

func (p pipelineRunner) QAVerdict(ctx context.Context, app, ringName string) (string, string, error) {
	return p.s.prom.QAVerdict(ctx, app, ringName)
}

func (p pipelineRunner) Audit(ctx context.Context, ringName, action string, detail map[string]string) {
	p.s.prom.AuditPipeline(ctx, ringName, action, detail)
}

// pipelineStageView is the JSON view of a configured stage.
type pipelineStageView struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name"`
	Type           string   `json:"type"`
	Apps           []string `json:"apps"`
	Needs          []string `json:"needs"`
	Gate           string   `json:"gate,omitempty"`
	TimeoutSeconds int64    `json:"timeout_seconds,omitempty"`
}

// pipelineView is the JSON view of a configured pipeline.
type pipelineView struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"display_name"`
	Description string              `json:"description,omitempty"`
	Stages      []pipelineStageView `json:"stages"`
	LastRun     *pipeline.Run       `json:"last_run"`
}

func (s *Server) handleListPipelines(w http.ResponseWriter, _ *http.Request) {
	out := []pipelineView{}
	for _, p := range s.prom.Pipelines() {
		v := pipelineView{Name: p.Name, DisplayName: p.Title(), Description: p.Description, Stages: []pipelineStageView{}}
		for _, st := range p.Stages {
			sv := pipelineStageView{
				Name: st.Name, DisplayName: st.Title(), Type: st.ResolvedType(),
				Apps: append([]string{}, st.Apps...), Needs: append([]string{}, st.Needs...),
			}
			if sv.Type != config.StageDeploy {
				sv.TimeoutSeconds = int64(st.ResolvedTimeout().Seconds())
			}
			if sv.Type == config.StageGate {
				sv.Gate = st.ResolvedGate()
			}
			v.Stages = append(v.Stages, sv)
		}
		if runs := s.pipes.List(p.Name, 1); len(runs) > 0 {
			v.LastRun = &runs[0]
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStartPipeline(w http.ResponseWriter, r *http.Request) {
	p, ok := s.prom.Pipeline(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("pipeline not found"))
		return
	}
	var body struct {
		pipeline.Request
		// Password is the production password, required for a run into the
		// production ring exactly as for a single-app deploy there.
		Password string `json:"password,omitempty"`
	}
	if !decode(w, r, &body) {
		return
	}
	req := body.Request
	if err := pipeline.Validate(p, req, func(app string) string { return s.prom.RingRef(app, req.Ring) }); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Ring == prodRing() && !s.checkProdPassword(w, body.Password) {
		return
	}
	s.startPipeline(w, r, p, req, "")
}

// startPipeline resolves first-ring versions from ring refs and starts the run.
func (s *Server) startPipeline(w http.ResponseWriter, r *http.Request, p config.PipelineConfig, req pipeline.Request, rerunOf string) {
	versions := map[string]string{}
	for k, v := range req.Versions {
		versions[k] = v
	}
	if req.Ring == ring.All()[0].Name {
		for _, st := range p.Stages {
			for _, app := range st.Apps {
				if versions[app] == "" && st.ResolvedType() == config.StageDeploy {
					versions[app] = s.prom.RingRef(app, req.Ring)
				}
			}
		}
	}
	req.Versions = versions
	if req.TriggeredBy == "" {
		req.TriggeredBy = promoter.ActorFrom(r.Context()).Name
	}
	ctx := promoter.WithGateInputs(r.Context(), promoter.GateInputs{ChangeRequestCode: req.ChangeRequestCode})
	run, err := s.pipes.Start(ctx, p, req, rerunOf)
	if err != nil {
		writeError(w, statusForPipelineErr(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleListPipelineRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.prom.Pipeline(name); !ok {
		writeError(w, http.StatusNotFound, errors.New("pipeline not found"))
		return
	}
	writeJSON(w, http.StatusOK, s.pipes.List(name, 50))
}

func (s *Server) handleGetPipelineRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.pipes.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, statusForPipelineErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleApprovePipelineStage(w http.ResponseWriter, r *http.Request) {
	s.decidePipelineStage(w, r, true)
}

func (s *Server) handleRejectPipelineStage(w http.ResponseWriter, r *http.Request) {
	s.decidePipelineStage(w, r, false)
}

func (s *Server) decidePipelineStage(w http.ResponseWriter, r *http.Request, approve bool) {
	var body struct {
		By   string `json:"by,omitempty"`
		Note string `json:"note,omitempty"`
	}
	if !decodeOptional(w, r, &body) {
		return
	}
	if body.By == "" {
		body.By = promoter.ActorFrom(r.Context()).Name
	}
	run, err := s.pipes.Decide(r.Context(), r.PathValue("id"), r.PathValue("stage"), approve, body.By, body.Note)
	if err != nil {
		writeError(w, statusForPipelineErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleCancelPipelineRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.pipes.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusForPipelineErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// handleRerunPipeline starts a new run with the same ring and versions,
// reusing every stage that succeeded in the source run (rerun from failed).
func (s *Server) handleRerunPipeline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password,omitempty"`
	}
	if !decodeOptional(w, r, &body) {
		return
	}
	id := r.PathValue("id")
	p, req, err := s.pipes.Request(id)
	if err != nil {
		writeError(w, statusForPipelineErr(err), err)
		return
	}
	if req.Ring == prodRing() && !s.checkProdPassword(w, body.Password) {
		return
	}
	req.TriggeredBy = ""
	s.startPipeline(w, r, p, req, id)
}

// decodeOptional is decode for endpoints whose body may be empty.
func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body: "+err.Error()))
		return false
	}
	return true
}

func statusForPipelineErr(err error) int {
	switch {
	case errors.Is(err, pipeline.ErrRunNotFound), errors.Is(err, pipeline.ErrStageNotFound):
		return http.StatusNotFound
	case errors.Is(err, pipeline.ErrNotWaiting), errors.Is(err, pipeline.ErrRunFinished):
		return http.StatusConflict
	case errors.Is(err, pipeline.ErrBadRequest):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
