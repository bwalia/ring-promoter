package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/store"
)

// handleQAStatus returns the QA agent registration and latest reports.
// When no qa_agent is configured the response is {"enabled":false} so clients
// can hide the strip without treating it as an error.
func (s *Server) handleQAStatus(w http.ResponseWriter, r *http.Request) {
	if !s.prom.QAAgentEnabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	app := strings.TrimSpace(r.URL.Query().Get("app"))
	reports, err := s.prom.ListQAReports(r.Context(), app)
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	if reports == nil {
		reports = []store.QAReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"name":    s.prom.QAAgentName(),
		"url":     s.prom.QAAgentURL(),
		"reports": reports,
	})
}

// handleCreateQAReport records a push from the external QA agent.
func (s *Server) handleCreateQAReport(w http.ResponseWriter, r *http.Request) {
	if !s.prom.QAAgentEnabled() {
		writeError(w, http.StatusConflict, promoter.ErrQAAgentDisabled)
		return
	}
	var body struct {
		App             string     `json:"app"`
		Ring            string     `json:"ring"`
		WorkflowVerdict string     `json:"workflow_verdict"`
		EnvHealthy      *bool      `json:"env_healthy"`
		Summary         string     `json:"summary"`
		Detail          string     `json:"detail"`
		CheckedAt       *time.Time `json:"checked_at"`
	}
	if !decode(w, r, &body) {
		return
	}
	var checkedAt time.Time
	if body.CheckedAt != nil {
		checkedAt = *body.CheckedAt
	}
	report, err := s.prom.RecordQAReport(r.Context(),
		strings.TrimSpace(body.App), body.Ring, body.WorkflowVerdict,
		body.EnvHealthy, body.Summary, body.Detail, checkedAt)
	if err != nil {
		writeError(w, statusForQAErr(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, report)
}

func statusForQAErr(err error) int {
	switch {
	case errors.Is(err, promoter.ErrQAAgentDisabled):
		return http.StatusConflict
	case errors.Is(err, promoter.ErrQAAgentScope):
		return http.StatusForbidden
	case errors.Is(err, promoter.ErrInvalidQAReport):
		return http.StatusBadRequest
	default:
		return statusForErr(err)
	}
}
