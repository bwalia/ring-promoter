package api

import (
	"context"
	"net/http"

	"github.com/example/ring-promoter/internal/promoter"
)

// ControlBriefer rewrites a hop scorecard in plain language. diagnose.Client
// implements it when an LLM is configured.
type ControlBriefer interface {
	BriefControls(ctx context.Context, report string) (string, error)
}

func (s *Server) briefer() ControlBriefer {
	if s.diag == nil {
		return nil
	}
	b, _ := s.diag.(ControlBriefer)
	return b
}

func (s *Server) handleAppControls(w http.ResponseWriter, r *http.Request) {
	view, err := s.prom.AppControls(r.Context(), r.PathValue("app"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	view.AIEnabled = s.briefer() != nil
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleControlsBrief(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !decode(w, r, &body) {
		return
	}
	hop, err := s.prom.HopControls(r.Context(), r.PathValue("app"), body.From, body.To)
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	text := promoter.ControlsBriefText(hop)
	source := "rules"
	if b := s.briefer(); b != nil {
		if ai, err := b.BriefControls(r.Context(), promoter.MarshalHop(hop)); err == nil && ai != "" {
			text, source = ai, "ai"
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"briefing": text, "source": source})
}
