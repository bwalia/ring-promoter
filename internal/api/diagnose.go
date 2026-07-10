package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/store"
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

// ---- history diagnosis ----
//
// History entries persist in the store (they outlive restarts and job
// eviction) but carry no step logs, so their diagnosis works from the recorded
// summary. The finished answer is stored on the entry itself — shared by every
// user and durable; only the transient running/failed state lives here.

// historyDiagnoses tracks in-flight and failed history diagnoses per entry id.
type historyDiagnoses struct {
	mu    sync.Mutex
	state map[int64]historyDiagState
}

type historyDiagState struct {
	Status string // diagRunning or diagFailed
	Err    string
}

// start marks entry id as being diagnosed; false means one is already running.
// A previously failed diagnosis can be restarted.
func (h *historyDiagnoses) start(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state[id].Status == diagRunning {
		return false
	}
	h.state[id] = historyDiagState{Status: diagRunning}
	return true
}

// finish clears a successful run (the store now holds the answer) or records
// the failure so the UI can show it and offer a retry.
func (h *historyDiagnoses) finish(id int64, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.state[id] = historyDiagState{Status: diagFailed, Err: err.Error()}
		return
	}
	delete(h.state, id)
}

func (h *historyDiagnoses) get(id int64) (historyDiagState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.state[id]
	return st, ok
}

// historyEntryFor resolves the {app}/{id} path into a history entry, writing
// the error response itself when the entry cannot be diagnosed.
func (s *Server) historyEntryFor(w http.ResponseWriter, r *http.Request) (store.HistoryEntry, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid history id"))
		return store.HistoryEntry{}, false
	}
	entry, err := s.prom.HistoryEntry(r.Context(), r.PathValue("app"), id)
	if err != nil {
		writeError(w, statusForErr(err), err)
		return store.HistoryEntry{}, false
	}
	return entry, true
}

// handleDiagnoseHistory starts (or reuses) the AI diagnosis of a FAILED
// history entry. Same contract as job diagnosis: detached, single-flight,
// 202 while running; the answer is persisted on the entry.
func (s *Server) handleDiagnoseHistory(w http.ResponseWriter, r *http.Request) {
	if s.diag == nil {
		writeError(w, http.StatusNotImplemented, errors.New("AI diagnosis is not configured on this server"))
		return
	}
	entry, ok := s.historyEntryFor(w, r)
	if !ok {
		return
	}
	if entry.Result != store.ResultFailure {
		writeError(w, http.StatusConflict, errors.New("only failed entries can be diagnosed"))
		return
	}
	if entry.Diagnosis != "" {
		writeJSON(w, http.StatusOK, map[string]string{"diagnosis_status": diagDone, "diagnosis": entry.Diagnosis})
		return
	}
	if !s.histDiag.start(entry.ID) {
		writeJSON(w, http.StatusAccepted, map[string]string{"diagnosis_status": diagRunning})
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), diagnoseTimeout)
	go func() {
		defer cancel()
		text, err := s.diag.Diagnose(ctx, historyReport(entry))
		if err == nil {
			err = s.prom.SetHistoryDiagnosis(ctx, entry.ID, text)
		}
		if err != nil {
			s.log.Error("ai history diagnosis failed", "entry", entry.ID, "err", err)
		}
		s.histDiag.finish(entry.ID, err)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"diagnosis_status": diagRunning})
}

// handleGetHistoryDiagnosis reports where a history entry's diagnosis stands:
// none, running, failed (with the error) or done (with the stored answer).
func (s *Server) handleGetHistoryDiagnosis(w http.ResponseWriter, r *http.Request) {
	entry, ok := s.historyEntryFor(w, r)
	if !ok {
		return
	}
	if entry.Diagnosis != "" {
		writeJSON(w, http.StatusOK, map[string]string{"diagnosis_status": diagDone, "diagnosis": entry.Diagnosis})
		return
	}
	if st, ok := s.histDiag.get(entry.ID); ok {
		out := map[string]string{"diagnosis_status": st.Status}
		if st.Err != "" {
			out["diagnosis_error"] = st.Err
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"diagnosis_status": "none"})
}

// historyReport renders a history entry as the report handed to the model.
// Unlike a live job it has no step logs, and the prompt says so.
func historyReport(e store.HistoryEntry) string {
	var b strings.Builder
	b.WriteString("This failure comes from the deployment history. The detailed step logs have expired, so explain from this recorded summary.\n\n")
	fmt.Fprintf(&b, "Application: %s\nAction: %s\nTarget ring: %s\n", e.App, e.Action, e.Ring)
	if e.FromVersion != "" {
		fmt.Fprintf(&b, "From version: %s\n", e.FromVersion)
	}
	if e.ToVersion != "" {
		fmt.Fprintf(&b, "To version: %s\n", e.ToVersion)
	}
	fmt.Fprintf(&b, "When: %s\nFailure message: %s\n", e.CreatedAt.UTC().Format(time.RFC3339), e.Message)
	return b.String()
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
