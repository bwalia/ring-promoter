package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/store"
)

// maxScanBody bounds a scan upload: SARIF from a large dependency tree runs
// to megabytes.
const maxScanBody = 10 << 20

// handleCreateSecurityScan records a scanner's report for one version: SARIF,
// a findings list, or bare severity counts.
func (s *Server) handleCreateSecurityScan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		App       string                  `json:"app"`
		Version   string                  `json:"version"`
		Kind      string                  `json:"kind"`
		Scanner   string                  `json:"scanner"`
		URL       string                  `json:"url"`
		Summary   string                  `json:"summary"`
		ScannedAt *time.Time              `json:"scanned_at"`
		Critical  int                     `json:"critical"`
		High      int                     `json:"high"`
		Medium    int                     `json:"medium"`
		Low       int                     `json:"low"`
		Findings  []store.SecurityFinding `json:"findings"`
		SARIF     json.RawMessage         `json:"sarif"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScanBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body: "+err.Error()))
		return
	}
	in := promoter.SecurityScanInput{
		App: strings.TrimSpace(body.App), Version: body.Version, Kind: body.Kind, Scanner: body.Scanner,
		URL: body.URL, Summary: body.Summary, Findings: body.Findings, SARIF: body.SARIF,
		Counts: promoter.SeverityCounts{Critical: body.Critical, High: body.High, Medium: body.Medium, Low: body.Low},
	}
	if body.ScannedAt != nil {
		in.ScannedAt = *body.ScannedAt
	}
	scan, err := s.prom.RecordSecurityScan(r.Context(), in)
	if err != nil {
		writeError(w, statusForSecurityErr(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, scan)
}

// handleAppSecurity returns the app's security policy, per-ring posture,
// newest scans and waivers.
func (s *Server) handleAppSecurity(w http.ResponseWriter, r *http.Request) {
	sec, err := s.prom.AppSecurity(r.Context(), r.PathValue("app"))
	if err != nil {
		writeError(w, statusForSecurityErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, sec)
}

// handleEvaluateSecurity evaluates one version (optionally for a ring).
func (s *Server) handleEvaluateSecurity(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if version == "" {
		writeError(w, http.StatusBadRequest, errors.New("version is required"))
		return
	}
	ev, err := s.prom.EvaluateSecurity(r.Context(), r.PathValue("app"), strings.TrimSpace(r.URL.Query().Get("ring")), version)
	if err != nil {
		writeError(w, statusForSecurityErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleCreateSecurityWaiver accepts the risk of one finding for a while.
func (s *Server) handleCreateSecurityWaiver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Finding   string     `json:"finding"`
		Ring      string     `json:"ring"`
		Reason    string     `json:"reason"`
		ExpiresAt *time.Time `json:"expires_at"`
		Days      int        `json:"days"`
	}
	if !decode(w, r, &body) {
		return
	}
	var exp time.Time
	if body.ExpiresAt != nil {
		exp = *body.ExpiresAt
	}
	wv, err := s.prom.CreateSecurityWaiver(r.Context(), r.PathValue("app"), body.Finding, body.Ring, body.Reason,
		promoter.ActorFrom(r.Context()).Name, exp, body.Days)
	if err != nil {
		writeError(w, statusForSecurityErr(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, wv)
}

// handleDeleteSecurityWaiver revokes a waiver.
func (s *Server) handleDeleteSecurityWaiver(w http.ResponseWriter, r *http.Request) {
	if err := s.prom.DeleteSecurityWaiver(r.Context(), r.PathValue("app"), r.PathValue("id")); err != nil {
		writeError(w, statusForSecurityErr(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func statusForSecurityErr(err error) int {
	switch {
	case errors.Is(err, promoter.ErrInvalidSecurityScan), errors.Is(err, promoter.ErrInvalidWaiver):
		return http.StatusBadRequest
	}
	return statusForErr(err)
}
