package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/promoter"
)

// TestPlanner turns a deployed version's test kit into an AI test plan (raw
// JSON, validated by promoter.ParseTestPlan). The diagnose client implements
// it; the server finds it on the configured Diagnoser.
type TestPlanner interface {
	TestPlan(ctx context.Context, report string) (string, error)
}

func (s *Server) planner() TestPlanner {
	if s.diag == nil {
		return nil
	}
	tp, _ := s.diag.(TestPlanner)
	return tp
}

// testPlans tracks in-flight and failed plan generations per (app, ring,
// version). A finished plan lives in the store; only transient state is here.
type testPlans struct {
	mu    sync.Mutex
	state map[string]historyDiagState
}

func planKey(app, ring, version string) string { return app + "\x00" + ring + "\x00" + version }

func (t *testPlans) start(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state[key].Status == diagRunning {
		return false
	}
	t.state[key] = historyDiagState{Status: diagRunning}
	return true
}

func (t *testPlans) finish(key string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		t.state[key] = historyDiagState{Status: diagFailed, Err: err.Error()}
		return
	}
	delete(t.state, key)
}

func (t *testPlans) get(key string) (historyDiagState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.state[key]
	return st, ok
}

// testKitResponse is the test kit plus where its AI plan stands.
type testKitResponse struct {
	promoter.TestKitView
	AIEnabled  bool   `json:"ai_enabled"`
	PlanStatus string `json:"plan_status"` // none | running | failed | done
	PlanError  string `json:"plan_error,omitempty"`
}

// handleTestKit returns where to test a ring's current version: its links
// (config, health host, deploy outputs) and the AI test plan, if any.
func (s *Server) handleTestKit(w http.ResponseWriter, r *http.Request) {
	view, _, err := s.prom.TestKit(r.Context(), r.PathValue("app"), r.PathValue("ring"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, s.testKitResponse(view))
}

func (s *Server) testKitResponse(view promoter.TestKitView) testKitResponse {
	out := testKitResponse{TestKitView: view, AIEnabled: s.planner() != nil, PlanStatus: "none"}
	if view.Plan != nil {
		out.PlanStatus = diagDone
	}
	if st, ok := s.plans.get(planKey(view.App, view.Ring, view.Version)); ok {
		out.PlanStatus = st.Status
		out.PlanError = st.Err
	}
	return out
}

// testPlanTimeout bounds one detached plan generation end-to-end.
const testPlanTimeout = 4 * time.Minute

// handleTestPlan starts (or reuses) the AI test plan for a ring's current
// version. Same contract as diagnosis: detached from the request,
// single-flight per version, 202 while running; the UI polls the test kit.
// POST with ?refresh=1 regenerates an existing plan.
func (s *Server) handleTestPlan(w http.ResponseWriter, r *http.Request) {
	tp := s.planner()
	if tp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("AI test plans are not configured on this server"))
		return
	}
	view, kit, err := s.prom.TestKit(r.Context(), r.PathValue("app"), r.PathValue("ring"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	if view.Plan != nil && r.URL.Query().Get("refresh") == "" {
		writeJSON(w, http.StatusOK, s.testKitResponse(view))
		return
	}
	key := planKey(view.App, view.Ring, view.Version)
	if !s.plans.start(key) {
		writeJSON(w, http.StatusAccepted, s.testKitResponse(view))
		return
	}

	report := promoter.TestPlanReport(view, kit)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), testPlanTimeout)
	go func() {
		defer cancel()
		raw, err := tp.TestPlan(ctx, report)
		if err == nil {
			plan, perr := promoter.ParseTestPlan(raw, view.Links, time.Now())
			if err = perr; err == nil {
				err = s.prom.SetTestPlan(ctx, view.App, view.Ring, view.Version, plan)
			}
		}
		if err != nil {
			s.log.Error("ai test plan failed", "app", view.App, "ring", view.Ring, "version", view.Version, "err", err)
		}
		s.plans.finish(key, err)
	}()
	writeJSON(w, http.StatusAccepted, s.testKitResponse(view))
}
