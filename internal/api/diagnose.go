package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Diagnoser produces a plain-language explanation of a failure report
// (implemented by internal/diagnose against an Ollama server). nil = the
// feature is not configured.
type Diagnoser interface {
	Diagnose(ctx context.Context, report string) (string, error)
}

// Caps applied to the report sent to the model so a log-heavy job cannot blow
// its context window. Logs are truncated from the front: the tail (where the
// failure surfaces) is the diagnostic signal.
const (
	maxLogLinesPerStep = 40
	maxReportBytes     = 16_000
)

// diagnoseTimeout bounds one detached diagnosis end-to-end. It must exceed the
// diagnose client's own HTTP timeout so the client, not this context, decides.
const diagnoseTimeout = 4 * time.Minute

// handleDiagnoseJob explains a FAILED job: it sends the job's error and step
// logs to the configured LLM and stores the plain-language answer on the job.
// The generation runs DETACHED from the request — a client disconnect or proxy
// timeout must not abort (and waste) a minutes-long model call — and is
// single-flight per job, so concurrent clicks share one call. The handler
// returns 202 while the diagnosis runs; the UI polls the job for the result.
func (s *Server) handleDiagnoseJob(w http.ResponseWriter, r *http.Request) {
	if s.diag == nil {
		writeError(w, http.StatusNotImplemented, errors.New("AI diagnosis is not configured on this server"))
		return
	}
	job, ok := s.jobs.get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("job not found"))
		return
	}
	snap := job.snapshot()
	if snap.App != r.PathValue("app") {
		writeError(w, http.StatusNotFound, errors.New("job not found"))
		return
	}
	if snap.Status != jobFailed {
		writeError(w, http.StatusConflict, errors.New("only failed jobs can be diagnosed"))
		return
	}
	// A second click (or another user) reuses the stored diagnosis.
	if snap.Diagnosis != "" {
		writeJSON(w, http.StatusOK, map[string]string{"diagnosis": snap.Diagnosis})
		return
	}
	if !job.startDiagnosis() {
		// Lost the single-flight race: a diagnosis is being generated right
		// now (or just landed) — the job poll picks it up either way.
		writeJSON(w, http.StatusAccepted, map[string]string{"diagnosis_status": diagRunning})
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), diagnoseTimeout)
	go func() {
		defer cancel()
		text, err := s.diag.Diagnose(ctx, failureReport(snap))
		if err != nil {
			s.log.Error("ai diagnosis failed", "job", snap.ID, "err", err)
		}
		job.finishDiagnosis(text, err)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"diagnosis_status": diagRunning})
}

// failureReport renders a failed job as the plain-text report handed to the
// model: what ran, the terminal error, then each step with its last log lines.
func failureReport(js jobState) string {
	var steps strings.Builder
	for i, st := range js.Steps {
		fmt.Fprintf(&steps, "%d. [%s] %s\n", i+1, st.Status, st.Title)
		logs := st.Logs
		if len(logs) > maxLogLinesPerStep {
			fmt.Fprintf(&steps, "   (... %d earlier log lines omitted)\n", len(logs)-maxLogLinesPerStep)
			logs = logs[len(logs)-maxLogLinesPerStep:]
		}
		for _, line := range logs {
			fmt.Fprintf(&steps, "   %s\n", line)
		}
	}
	stepsText := steps.String()
	if len(stepsText) > maxReportBytes {
		stepsText = "(... report truncated, showing the end)\n" + stepsText[len(stepsText)-maxReportBytes:]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Application: %s\nAction: %s\n", js.App, js.Action)
	if js.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", js.Error)
	}
	if js.Result != nil {
		if js.Result.Message != "" {
			fmt.Fprintf(&b, "Outcome: %s\n", js.Result.Message)
		}
		if js.Result.Ring != "" {
			fmt.Fprintf(&b, "Target ring: %s\n", js.Result.Ring)
		}
		if js.Result.Version != "" {
			fmt.Fprintf(&b, "Version: %s\n", js.Result.Version)
		}
		if js.Result.RolledBack {
			b.WriteString("Note: the ring was automatically rolled back to the previous version.\n")
		}
	}
	b.WriteString("\nSteps:\n")
	b.WriteString(stepsText)
	return b.String()
}
